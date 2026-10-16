package kind

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	authv1 "k8s.io/api/authorization/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/billykong/planproof/executor"
	"github.com/billykong/planproof/fetch"
	"github.com/billykong/planproof/kernel"
)

const executorSA = "system:serviceaccount:planproof:executor"

// TestExecutorRBAC: the executor can do what the seven operations need and
// nothing else (C-16): patch, never update, create or delete, apart from
// evictions.
func TestExecutorRBAC(t *testing.T) {
	admin, _ := clients(t)
	type access struct {
		verb, group, resource, sub string
		want                       bool
	}
	for _, a := range []access{
		{"patch", "", "nodes", "", true}, {"patch", "apps", "deployments", "", true}, {"create", "", "pods", "eviction", true},
		{"list", "", "pods", "", true}, {"list", "apps", "replicasets", "", true},
		{"update", "", "nodes", "", false}, {"delete", "", "nodes", "", false}, {"delete", "", "pods", "", false},
		{"create", "", "pods", "", false}, {"update", "apps", "deployments", "", false}, {"delete", "apps", "deployments", "", false},
		{"create", "apps", "deployments", "", false}, {"patch", "apps", "daemonsets", "", false}, {"patch", "apps", "statefulsets", "", false},
		{"get", "", "secrets", "", false}, {"patch", "policy", "poddisruptionbudgets", "", false}, {"impersonate", "", "users", "", false},
	} {
		r, err := admin.AuthorizationV1().SubjectAccessReviews().Create(ctx(t), &authv1.SubjectAccessReview{Spec: authv1.SubjectAccessReviewSpec{
			User: executorSA, Groups: []string{"system:serviceaccounts", "system:serviceaccounts:planproof", "system:authenticated"},
			ResourceAttributes: &authv1.ResourceAttributes{Verb: a.verb, Group: a.group, Resource: a.resource, Subresource: a.sub},
		}}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if r.Status.Allowed != a.want {
			t.Errorf("executor %s %s/%s %s: allowed=%v, want %v", a.verb, a.group, a.resource, a.sub, r.Status.Allowed, a.want)
		}
	}
}

// lab is one executor test: a fixture namespace with "web" (3 replicas,
// budget minAvailable 2), the snapshot, and a runner with the executor's
// identity.
type lab struct {
	t     *testing.T
	admin *kubernetes.Clientset
	cfg   *rest.Config
	ns    string
	snap  []byte
}

func newLab(t *testing.T) *lab {
	admin, cfg := clients(t)
	ns := unique("pp-exec")
	two := intstr.FromInt32(2)
	fixture(t, admin, ns,
		pauseDeployment(ns, "web", 3, "100m", "32Mi", nil),
		&policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}}, MinAvailable: &two}})
	return &lab{t: t, admin: admin, cfg: cfg, ns: ns}
}

// fetch takes the snapshot as the fetcher.
func (l *lab) fetch() []byte {
	s, err := fetch.Snapshot(ctx(l.t), as(l.t, l.cfg, fetcherSA), "planproof", time.Now())
	if err != nil {
		l.t.Fatal(err)
	}
	l.snap, _ = fetch.Marshal(s)
	return l.snap
}

// nodeWithWeb is a worker running a web pod, per the snapshot.
func (l *lab) nodeWithWeb() string {
	var s kernel.Snapshot
	json.Unmarshal(l.snap, &s)
	for _, p := range s.Pods {
		if p.Namespace == l.ns && p.Owner.Name == "web" {
			l.t.Cleanup(func() { // whatever happens, leave the node schedulable
				l.admin.CoreV1().Nodes().Patch(ctx(l.t), p.Node, types.StrategicMergePatchType, []byte(`{"spec":{"unschedulable":false}}`), metav1.PatchOptions{})
			})
			return p.Node
		}
	}
	l.t.Fatal("no web pod in the snapshot")
	return ""
}

// inputs builds a plan (goal, steps, rollback as YAML-like Go values) bound
// to the snapshot, and a policy that manages the fixture and the workers.
func (l *lab) inputs(goal, steps, rollback []map[string]any) executor.Inputs {
	plan, _ := json.Marshal(map[string]any{"apiVersion": "planproof/v0", "kind": "Plan", "metadata": map[string]any{"name": "lab"},
		"snapshot": kernel.Hash(l.snap), "goal": goal, "steps": steps, "rollback": rollback})
	policy, _ := json.Marshal(map[string]any{"apiVersion": "planproof/v0", "kind": "Policy",
		"managedNamespaces": []string{l.ns}, "managedNodes": []string{"planproof-worker", "planproof-worker2", "planproof-worker3", "planproof-worker4"},
		"protectedDeployments": []string{}, "criticalDeployments": []string{}, "approvalRequiredFor": []string{"drain"},
		"limits": map[string]any{"maxReplicas": 10, "maxUnschedulableNodes": 1, "minSchedulableNodes": 0, "maxWaitSeconds": 300,
			"requireImageDigest": false, "allowScaleToZero": false}})
	in := executor.Inputs{Snapshot: l.snap, Plan: plan, Policy: policy}
	if v := kernel.Check(in.Snapshot, in.Plan, in.Policy); v.Result != kernel.Proved {
		l.t.Fatalf("the test plan isn't Proved: %s %+v", v.Result, v.Facts)
	}
	return in
}

func (l *lab) runner() *executor.Runner {
	return &executor.Runner{Client: as(l.t, l.cfg, executorSA), StepTimeout: 90 * time.Second, Poll: time.Second}
}

func (l *lab) web() (replicas, ready int32) {
	d, err := l.admin.AppsV1().Deployments(l.ns).Get(ctx(l.t), "web", metav1.GetOptions{})
	if err != nil {
		l.t.Fatal(err)
	}
	return *d.Spec.Replicas, d.Status.ReadyReplicas
}

func m(kv ...any) map[string]any {
	out := map[string]any{}
	for i := 0; i < len(kv); i += 2 {
		out[kv[i].(string)] = kv[i+1]
	}
	return out
}

// TestExecuteDrain: a proved, approved drain runs as the executor: cordon,
// evictions only, wait (C-13, C-14).
func TestExecuteDrain(t *testing.T) {
	l := newLab(t)
	l.fetch()
	node := l.nodeWithWeb()
	web := l.ns + "/web"
	in := l.inputs(
		[]map[string]any{m("node", node, "field", "unschedulable", "from", false, "to", true), m("node", node, "field", "drained", "from", false, "to", true)},
		[]map[string]any{
			m("op", "cordon", "node", node, "pre", m("unschedulable", false)),
			m("op", "drain", "node", node, "post", m("podsOnNode", 0)),
			m("op", "wait", "deployments", []string{web}, "timeoutSeconds", 120),
		},
		[]map[string]any{m("op", "uncordon", "node", node)})
	rep := l.runner().Run(ctx(t), in, kernel.Hash(in.Plan))
	if rep.Result != "done" {
		t.Fatalf("report: %+v", rep)
	}
	evictions := 0
	for _, d := range rep.Done {
		if d.Resource == "pods/eviction" {
			evictions++
		}
	}
	n, _ := l.admin.CoreV1().Nodes().Get(ctx(t), node, metav1.GetOptions{})
	pods, _ := l.admin.CoreV1().Pods(l.ns).List(ctx(t), metav1.ListOptions{FieldSelector: "spec.nodeName=" + node})
	if replicas, ready := l.web(); evictions == 0 || !n.Spec.Unschedulable || len(pods.Items) != 0 || ready != replicas {
		t.Fatalf("after the run: %d evictions, unschedulable %v, %d web pods on %s, %d/%d ready", evictions, n.Spec.Unschedulable, len(pods.Items), node, ready, replicas)
	}
}

// TestExecuteRefusesWithoutApproval: the policy requires approval for a
// drain; without one, nothing runs (C-10).
func TestExecuteRefusesWithoutApproval(t *testing.T) {
	l := newLab(t)
	l.fetch()
	node := l.nodeWithWeb()
	in := l.inputs([]map[string]any{m("node", node, "field", "unschedulable", "from", false, "to", true), m("node", node, "field", "drained", "from", false, "to", true)},
		[]map[string]any{m("op", "cordon", "node", node), m("op", "drain", "node", node), m("op", "wait", "deployments", []string{l.ns + "/web"}, "timeoutSeconds", 60)}, nil)
	rep := l.runner().Run(ctx(t), in, "")
	n, _ := l.admin.CoreV1().Nodes().Get(ctx(t), node, metav1.GetOptions{})
	if rep.Result != "refused" || len(rep.Done) != 0 || n.Spec.Unschedulable {
		t.Fatalf("report %+v; node cordoned: %v", rep, n.Spec.Unschedulable)
	}
}

func scaleWeb(l *lab, from, to int, pre map[string]any) []map[string]any {
	step := m("op", "scale", "deployment", l.ns+"/web", "replicas", to)
	if pre != nil {
		step["pre"] = pre
	}
	return []map[string]any{step, m("op", "wait", "deployments", []string{l.ns + "/web"}, "timeoutSeconds", 120)}
}

// TestExecuteStopsOnDrift: the cluster changed after the snapshot, so
// nothing runs (C-12).
func TestExecuteStopsOnDrift(t *testing.T) {
	l := newLab(t)
	l.fetch()
	in := l.inputs([]map[string]any{m("deployment", l.ns+"/web", "field", "replicas", "from", 3, "to", 4)}, scaleWeb(l, 3, 4, nil), nil)
	l.admin.AppsV1().Deployments(l.ns).Patch(ctx(t), "web", types.StrategicMergePatchType, []byte(`{"spec":{"replicas":5}}`), metav1.PatchOptions{})
	rep := l.runner().Run(ctx(t), in, "")
	if replicas, _ := l.web(); rep.Result != "stopped" || rep.Step != 0 || len(rep.Done) != 0 || replicas != 5 || !strings.Contains(rep.Reason, "changed since the snapshot") {
		t.Fatalf("report %+v; replicas %d", rep, replicas)
	}
}

// TestExecuteStopsOnConflict: the object changes between the executor's
// read and its write; the resourceVersion precondition refuses the write
// (C-13).
func TestExecuteStopsOnConflict(t *testing.T) {
	l := newLab(t)
	l.fetch()
	in := l.inputs([]map[string]any{m("deployment", l.ns+"/web", "field", "replicas", "from", 3, "to", 4)}, scaleWeb(l, 3, 4, nil), nil)
	r := l.runner()
	r.BeforeWrite = func(executor.Call) {
		l.admin.AppsV1().Deployments(l.ns).Patch(ctx(t), "web", types.StrategicMergePatchType, []byte(`{"metadata":{"annotations":{"someone":"else"}}}`), metav1.PatchOptions{})
	}
	rep := r.Run(ctx(t), in, "")
	if replicas, _ := l.web(); rep.Result != "stopped" || rep.Step != 1 || len(rep.Done) != 0 || replicas != 3 || !strings.Contains(rep.Reason, "conflict") {
		t.Fatalf("report %+v; replicas %d", rep, replicas)
	}
}

// TestExecuteStopsOnLiveAssertion: a step's pre assertion is false on the
// live cluster when the step comes, so the step doesn't run (C-13).
func TestExecuteStopsOnLiveAssertion(t *testing.T) {
	l := newLab(t)
	l.fetch()
	node := l.nodeWithWeb()
	steps := append([]map[string]any{m("op", "cordon", "node", node)}, scaleWeb(l, 3, 4, m("replicas", 3))...)
	in := l.inputs([]map[string]any{m("node", node, "field", "unschedulable", "from", false, "to", true), m("deployment", l.ns+"/web", "field", "replicas", "from", 3, "to", 4)}, steps, nil)
	r := l.runner()
	r.BeforeWrite = func(c executor.Call) {
		if c.Step == 1 { // while step 1 runs, someone scales web
			l.admin.AppsV1().Deployments(l.ns).Patch(ctx(t), "web", types.StrategicMergePatchType, []byte(`{"spec":{"replicas":5}}`), metav1.PatchOptions{})
		}
	}
	rep := r.Run(ctx(t), in, "")
	if replicas, _ := l.web(); rep.Result != "stopped" || rep.Step != 2 || len(rep.Done) != 1 || replicas != 5 || !strings.Contains(rep.Reason, "replicas is not 3") {
		t.Fatalf("report %+v; replicas %d", rep, replicas)
	}
}

// TestExecuteWaitTimeout: a rollout that never becomes ready stops the run
// at its wait, and the rollback is offered, not run (C-15).
func TestExecuteWaitTimeout(t *testing.T) {
	l := newLab(t)
	l.fetch()
	web := l.ns + "/web"
	missing := "registry.k8s.io/pause:does-not-exist" // never pulled: PullNever
	in := l.inputs([]map[string]any{m("deployment", web, "field", "image", "container", "app", "from", pause, "to", missing)},
		[]map[string]any{m("op", "set-image", "deployment", web, "container", "app", "image", missing), m("op", "wait", "deployments", []string{web}, "timeoutSeconds", 10)},
		[]map[string]any{m("op", "set-image", "deployment", web, "container", "app", "image", pause), m("op", "wait", "deployments", []string{web}, "timeoutSeconds", 60)})
	rep := l.runner().Run(ctx(t), in, "")
	d, _ := l.admin.AppsV1().Deployments(l.ns).Get(ctx(t), "web", metav1.GetOptions{})
	if rep.Result != "stopped" || rep.Step != 2 || len(rep.Rollback) != 2 || d.Spec.Template.Spec.Containers[0].Image != missing || d.Status.ReadyReplicas < 2 {
		t.Fatalf("report %+v; image %s, ready %d", rep, d.Spec.Template.Spec.Containers[0].Image, d.Status.ReadyReplicas)
	}
}
