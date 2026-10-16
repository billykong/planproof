package kind

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	"github.com/billykong/planproof/kernel"
)

// Model fidelity (PRD C-20 to C-22): run plans on kind and compare what
// Kubernetes does with what the kernel predicted. The kernel's model is
// meant to be pessimistic, so the real lowest number of ready pods must
// never be below the predicted lowest; where the kernel says pods fit, the
// scheduler must place them, and where it says they don't, they must stay
// Pending.

// sampler records, for each app in a namespace, the lowest number of pods
// that are ready and not terminating, polling every 100 ms. Polling can miss
// a dip shorter than that; the tests compare lower bounds, so a miss can only
// hide a failure, never invent one.
type sampler struct {
	mu   sync.Mutex
	min  map[string]int
	stop chan struct{}
	done chan struct{}
}

func sample(t *testing.T, c *kubernetes.Clientset, ns string, apps ...string) *sampler {
	s := &sampler{min: map[string]int{}, stop: make(chan struct{}), done: make(chan struct{})}
	for _, a := range apps {
		s.min[a] = 1 << 30
	}
	go func() {
		defer close(s.done)
		for {
			pods, err := c.CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{})
			if err == nil {
				count := map[string]int{}
				for _, p := range pods.Items {
					if p.DeletionTimestamp == nil && ready(&p) {
						count[p.Labels["app"]]++
					}
				}
				s.mu.Lock()
				for _, a := range apps {
					s.min[a] = min(s.min[a], count[a])
				}
				s.mu.Unlock()
			}
			select {
			case <-s.stop:
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}()
	return s
}

func (s *sampler) finish() map[string]int {
	close(s.stop)
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.min
}

func ready(p *corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// predicted reads the kernel's lowest available pods per deployment from
// the budget evidence.
func predicted(v kernel.Verdict) map[string]int {
	out := map[string]int{}
	for _, f := range v.Facts {
		if f.Type == "check" && f.Rule == "budget" && f.Evidence != nil {
			for _, it := range f.Evidence["items"].([]any) {
				it := it.(map[string]any)
				_, name, _ := strings.Cut(fmt.Sprint(it["deployment"]), "/")
				out[name] = int(toInt(it["lowestAvailable"]))
			}
		}
	}
	return out
}

func toInt(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	}
	return -1
}

// rolling makes a pause deployment with an explicit rolling strategy and a
// budget of minAvailable 0: the budget never blocks, and the kernel reports
// its predicted lowest for the deployment.
func rolling(ns, name string, replicas int32, cpu string, surge, unavailable int) []any {
	d := pauseDeployment(ns, name, replicas, cpu, "16Mi", nil)
	s, u := intstr.FromInt(surge), intstr.FromInt(unavailable)
	d.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType, RollingUpdate: &appsv1.RollingUpdateDeployment{MaxSurge: &s, MaxUnavailable: &u}}
	zero := intstr.FromInt32(0)
	return []any{d, &policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: name}, Spec: policyv1.PodDisruptionBudgetSpec{
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}}, MinAvailable: &zero}}}
}

// fidelityRun fetches a fresh snapshot, asks the kernel, runs the plan as
// the executor while sampling, and checks real lowest >= predicted lowest.
func fidelityRun(t *testing.T, l *lab, apps []string, goal, steps []map[string]any) {
	t.Helper()
	l.fetch()
	in, v := planInputs(l.snap, l.ns, nil, goal, steps, nil)
	if v.Result != kernel.Proved {
		t.Fatalf("not Proved: %s %+v", v.Result, v.Facts)
	}
	want := predicted(v)
	s := sample(t, l.admin, l.ns, apps...)
	rep := l.runner().Run(ctx(t), in, "")
	got := s.finish()
	if rep.Result != "done" {
		t.Fatalf("run: %+v", rep)
	}
	for _, a := range apps {
		t.Logf("%-5s predicted lowest %d, real lowest %d", a, want[a], got[a])
		if got[a] < want[a] {
			t.Errorf("%s: real lowest %d is below the kernel's predicted lowest %d: the model is not pessimistic here", a, got[a], want[a])
		}
	}
}

// TestFidelityOperations: each operation that changes availability, run
// for real, never does worse than the kernel predicted (C-20).
func TestFidelityOperations(t *testing.T) {
	admin, cfg := clients(t)
	l := &lab{t: t, admin: admin, cfg: cfg, ns: unique("pp-fid")}
	fixture(t, admin, l.ns, append(rolling(l.ns, "web", 4, "100m", 1, 0), rolling(l.ns, "api", 4, "100m", 0, 1)...)...)
	apps := []string{"web", "api"}
	dep := func(n string) string { return l.ns + "/" + n }
	waitFor := func(names ...string) map[string]any {
		var ds []string
		for _, n := range names {
			ds = append(ds, dep(n))
		}
		return m("op", "wait", "deployments", ds, "timeoutSeconds", 180)
	}

	t.Run("rollout-restart, surge first", func(t *testing.T) {
		fidelityRun(t, l, apps, []map[string]any{m("deployment", dep("web"), "field", "restarted", "from", false, "to", true)},
			[]map[string]any{m("op", "rollout-restart", "deployment", dep("web")), waitFor("web")})
	})
	t.Run("rollout-restart, one unavailable", func(t *testing.T) {
		fidelityRun(t, l, apps, []map[string]any{m("deployment", dep("api"), "field", "restarted", "from", false, "to", true)},
			[]map[string]any{m("op", "rollout-restart", "deployment", dep("api")), waitFor("api")})
	})
	t.Run("scale down", func(t *testing.T) {
		fidelityRun(t, l, apps, []map[string]any{m("deployment", dep("web"), "field", "replicas", "from", 4, "to", 2)},
			[]map[string]any{m("op", "scale", "deployment", dep("web"), "replicas", 2), waitFor("web")})
	})
	t.Run("scale up", func(t *testing.T) {
		fidelityRun(t, l, apps, []map[string]any{m("deployment", dep("web"), "field", "replicas", "from", 2, "to", 5)},
			[]map[string]any{m("op", "scale", "deployment", dep("web"), "replicas", 5), waitFor("web")})
	})
	t.Run("cordon, drain and uncordon", func(t *testing.T) {
		l.fetch()
		node := l.nodeWithApp("api")
		fidelityRun(t, l, apps,
			[]map[string]any{m("node", node, "field", "drained", "from", false, "to", true)},
			[]map[string]any{m("op", "cordon", "node", node), m("op", "drain", "node", node), waitFor("web", "api"), m("op", "uncordon", "node", node)})
	})
}

// nodeWithApp is a worker running a pod of the app, per the snapshot; the
// test leaves it schedulable whatever happens.
func (l *lab) nodeWithApp(app string) string {
	var s kernel.Snapshot
	json.Unmarshal(l.snap, &s)
	for _, p := range s.Pods {
		if p.Namespace == l.ns && p.Owner.Name == app {
			l.t.Cleanup(func() {
				l.admin.CoreV1().Nodes().Patch(context.Background(), p.Node, types.StrategicMergePatchType, []byte(`{"spec":{"unschedulable":false}}`), metav1.PatchOptions{})
			})
			return p.Node
		}
	}
	l.t.Fatalf("no %s pod in the snapshot", app)
	return ""
}

// TestFidelityExample: the worked example's two plans on the real cluster
// (C-21). The proved plan, run for real, never does worse than predicted;
// for the refuted plan (drain the second node without waiting), the
// cluster's own Eviction API refuses evictions the budgets don't allow,
// agreeing with the kernel that the plan is wrong.
func TestFidelityExample(t *testing.T) {
	admin, cfg := clients(t)
	l := &lab{t: t, admin: admin, cfg: cfg, ns: unique("pp-example")}
	two, one := intstr.FromInt32(2), intstr.FromInt32(1)
	budget := func(app string, v *intstr.IntOrString) *policyv1.PodDisruptionBudget {
		return &policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: app}, Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": app}}, MinAvailable: v}}
	}
	fixture(t, admin, l.ns,
		pauseDeployment(l.ns, "checkout", 3, "500m", "64Mi", nil), pauseDeployment(l.ns, "cart", 2, "250m", "32Mi", nil),
		budget("checkout", &two), budget("cart", &one))
	l.fetch()

	// The two workers running the most fixture pods; cordon both first, as
	// the proved example does.
	var s kernel.Snapshot
	json.Unmarshal(l.snap, &s)
	count := map[string]int{}
	for _, p := range s.Pods {
		if p.Namespace == l.ns {
			count[p.Node]++
		}
	}
	nodes := append([]string{}, workers...)
	sort.SliceStable(nodes, func(i, j int) bool { return count[nodes[i]] > count[nodes[j]] })
	n1, n2 := nodes[0], nodes[1]
	for _, n := range []string{n1, n2} {
		n := n
		t.Cleanup(func() {
			admin.CoreV1().Nodes().Patch(context.Background(), n, types.StrategicMergePatchType, []byte(`{"spec":{"unschedulable":false}}`), metav1.PatchOptions{})
		})
	}
	deps := []string{l.ns + "/checkout", l.ns + "/cart"}
	goal := []map[string]any{
		m("node", n1, "field", "unschedulable", "from", false, "to", true), m("node", n2, "field", "unschedulable", "from", false, "to", true),
		m("node", n1, "field", "drained", "from", false, "to", true), m("node", n2, "field", "drained", "from", false, "to", true),
	}
	waitAll := m("op", "wait", "deployments", deps, "timeoutSeconds", 180)
	proved := []map[string]any{m("op", "cordon", "node", n1), m("op", "cordon", "node", n2), m("op", "drain", "node", n1), waitAll, m("op", "drain", "node", n2), waitAll}
	noWait := []map[string]any{m("op", "cordon", "node", n1), m("op", "cordon", "node", n2), m("op", "drain", "node", n1), m("op", "drain", "node", n2), waitAll}

	if _, v := planInputs(l.snap, l.ns, nil, goal, noWait, nil); v.Result != kernel.Refuted {
		t.Fatalf("the no-wait plan on this placement: %s, want Refuted", v.Result)
	}
	// The cluster agrees: cordon both, evict everything on n1, then try n2
	// at once. The Eviction API must refuse at least one eviction (429).
	refused := 0
	for _, n := range []string{n1, n2} {
		admin.CoreV1().Nodes().Patch(ctx(t), n, types.StrategicMergePatchType, []byte(`{"spec":{"unschedulable":true}}`), metav1.PatchOptions{})
	}
	for _, n := range []string{n1, n2} {
		pods, _ := admin.CoreV1().Pods(l.ns).List(ctx(t), metav1.ListOptions{FieldSelector: "spec.nodeName=" + n})
		for _, p := range pods.Items {
			err := admin.PolicyV1().Evictions(l.ns).Evict(ctx(t), &policyv1.Eviction{ObjectMeta: metav1.ObjectMeta{Namespace: l.ns, Name: p.Name}})
			if err != nil && strings.Contains(err.Error(), "disruption budget") {
				refused++
			}
		}
	}
	t.Logf("no-wait plan: the kernel says Refuted; the Eviction API refused %d evictions", refused)
	if refused == 0 {
		t.Errorf("the kernel refuted the no-wait plan, but the cluster allowed every eviction: the model is too pessimistic here, or the placement didn't test it")
	}
	// Put things back for the proved plan.
	for _, n := range []string{n1, n2} {
		admin.CoreV1().Nodes().Patch(ctx(t), n, types.StrategicMergePatchType, []byte(`{"spec":{"unschedulable":false}}`), metav1.PatchOptions{})
	}
	for _, d := range []string{"checkout", "cart"} {
		waitReady(t, admin, l.ns, d)
	}

	fidelityRun(t, l, []string{"checkout", "cart"}, goal, proved)
}

// TestFidelityCapacity: the scheduler-fit assumption (A3) in both
// directions. 900m pods on workers with 2000m allocatable and a 100m
// kindnet pod: two fit on each worker, so 8 fit and a 9th doesn't.
func TestFidelityCapacity(t *testing.T) {
	admin, cfg := clients(t)
	l := &lab{t: t, admin: admin, cfg: cfg, ns: unique("pp-cap")}
	fixture(t, admin, l.ns, rolling(l.ns, "big", 4, "900m", 1, 0)...)
	big := l.ns + "/big"

	// The kernel says 8 fit; the scheduler places all 8.
	fidelityRun(t, l, []string{"big"}, []map[string]any{m("deployment", big, "field", "replicas", "from", 4, "to", 8)},
		[]map[string]any{m("op", "scale", "deployment", big, "replicas", 8), m("op", "wait", "deployments", []string{big}, "timeoutSeconds", 180)})

	// The kernel says a 9th doesn't fit; the scheduler leaves it Pending.
	l.fetch()
	_, v := planInputs(l.snap, l.ns, nil, []map[string]any{m("deployment", big, "field", "replicas", "from", 8, "to", 9)},
		[]map[string]any{m("op", "scale", "deployment", big, "replicas", 9), m("op", "wait", "deployments", []string{big}, "timeoutSeconds", 60)}, nil)
	if v.Result != kernel.Refuted || !hasFailure(v, "capacity") {
		t.Fatalf("scale to 9: %s, want Refuted for capacity", v.Result)
	}
	admin.AppsV1().Deployments(l.ns).Patch(ctx(t), "big", types.StrategicMergePatchType, []byte(`{"spec":{"replicas":9}}`), metav1.PatchOptions{})
	pending := 0
	wait.PollUntilContextTimeout(ctx(t), time.Second, 20*time.Second, true, func(cx context.Context) (bool, error) {
		pods, _ := admin.CoreV1().Pods(l.ns).List(cx, metav1.ListOptions{})
		pending = 0
		for _, p := range pods.Items {
			if p.Spec.NodeName == "" {
				pending++
			}
		}
		return len(pods.Items) == 9 && pending == 1, nil
	})
	t.Logf("scale to 9: the kernel says no room; %d pod(s) Pending on the real cluster", pending)
	if pending != 1 {
		t.Errorf("%d pods Pending, want 1", pending)
	}
}

func hasFailure(v kernel.Verdict, rule string) bool {
	for _, f := range v.Facts {
		if f.Type == "failure" && f.Rule == rule {
			return true
		}
	}
	return false
}

func unknownTrigger(v kernel.Verdict) string {
	for _, f := range v.Facts {
		if f.Type == "unknown" {
			return f.Trigger
		}
	}
	return ""
}

// TestUnknownTriggersAreReal: for two Unknown triggers, the real behaviour
// the model would get wrong (C-22).
func TestUnknownTriggersAreReal(t *testing.T) {
	admin, cfg := clients(t)

	t.Run("hpa: another actor changes replicas", func(t *testing.T) {
		l := &lab{t: t, admin: admin, cfg: cfg, ns: unique("pp-hpa")}
		minR, eighty := int32(3), int32(80)
		fixture(t, admin, l.ns, pauseDeployment(l.ns, "web", 1, "50m", "16Mi", nil))
		l.fetch()
		_, v := planInputs(l.snap, l.ns, nil, []map[string]any{m("deployment", l.ns+"/web", "field", "replicas", "from", 1, "to", 2)},
			[]map[string]any{m("op", "scale", "deployment", l.ns+"/web", "replicas", 2), m("op", "wait", "deployments", []string{l.ns + "/web"}, "timeoutSeconds", 60)}, nil)
		if v.Result != kernel.Proved {
			t.Fatalf("without an HPA: %s, want Proved", v.Result)
		}
		_, err := admin.AutoscalingV2().HorizontalPodAutoscalers(l.ns).Create(ctx(t), &autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: "web"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "Deployment", Name: "web"},
				MinReplicas: &minR, MaxReplicas: 3, Metrics: []autoscalingv2.MetricSpec{{Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{Name: corev1.ResourceCPU, Target: autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType, AverageUtilization: &eighty}}}}}},
			metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		// The HPA may already have acted (it does within seconds when below
		// its minimum), so plan from whatever the snapshot shows now.
		l.fetch()
		var s kernel.Snapshot
		json.Unmarshal(l.snap, &s)
		r := int64(0)
		for _, d := range s.Deployments {
			if d.Namespace == l.ns && d.Name == "web" {
				r = *d.Replicas
			}
		}
		_, v = planInputs(l.snap, l.ns, nil, []map[string]any{m("deployment", l.ns+"/web", "field", "replicas", "from", r, "to", r+1)},
			[]map[string]any{m("op", "scale", "deployment", l.ns+"/web", "replicas", r+1), m("op", "wait", "deployments", []string{l.ns + "/web"}, "timeoutSeconds", 60)}, nil)
		if got := unknownTrigger(v); got != "hpa" {
			t.Fatalf("with an HPA: %s, trigger %q, want Unknown hpa", v.Result, got)
		}
		// Nothing in the plan scales to 3, but the HPA does, within its sync period.
		err = wait.PollUntilContextTimeout(ctx(t), time.Second, 90*time.Second, true, func(cx context.Context) (bool, error) {
			d, err := admin.AppsV1().Deployments(l.ns).Get(cx, "web", metav1.GetOptions{})
			return err == nil && *d.Spec.Replicas == 3, nil
		})
		if err != nil {
			t.Fatalf("the HPA didn't change replicas: %v", err)
		}
		t.Log("the HPA scaled web from 1 to 3 on its own: a plan scaling to 2 would not end at 2")
	})

	t.Run("placement-rules: a moved pod has nowhere to go", func(t *testing.T) {
		l := &lab{t: t, admin: admin, cfg: cfg, ns: unique("pp-spread")}
		fixture(t, admin, l.ns, pauseDeployment(l.ns, "spread", 4, "50m", "16Mi", func(s *corev1.PodSpec) {
			s.Affinity = &corev1.Affinity{PodAntiAffinity: &corev1.PodAntiAffinity{RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{TopologyKey: "kubernetes.io/hostname", LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "spread"}}}}}}
		}))
		l.fetch()
		node := l.nodeWithApp("spread")
		goal := []map[string]any{m("node", node, "field", "unschedulable", "from", false, "to", true), m("node", node, "field", "drained", "from", false, "to", true)}
		steps := []map[string]any{m("op", "cordon", "node", node), m("op", "drain", "node", node), m("op", "wait", "deployments", []string{l.ns + "/spread"}, "timeoutSeconds", 60)}
		if _, v := planInputs(l.snap, l.ns, nil, goal, steps, nil); unknownTrigger(v) != "placement-rules" {
			t.Fatalf("trigger %q, want placement-rules", unknownTrigger(v))
		}
		// Without the rule, the model would say the drain is fine...
		blind := strings.ReplaceAll(string(l.snap), `"placementRules": true`, `"placementRules": false`)
		if _, v := planInputs([]byte(blind), l.ns, nil, goal, steps, nil); v.Result != kernel.Proved {
			t.Fatalf("with the rule hidden: %s, want Proved (to show what it would miss)", v.Result)
		}
		// ...but on the cluster, the moved pod can't be placed anywhere.
		admin.CoreV1().Nodes().Patch(ctx(t), node, types.StrategicMergePatchType, []byte(`{"spec":{"unschedulable":true}}`), metav1.PatchOptions{})
		pods, _ := admin.CoreV1().Pods(l.ns).List(ctx(t), metav1.ListOptions{FieldSelector: "spec.nodeName=" + node})
		for _, p := range pods.Items {
			admin.PolicyV1().Evictions(l.ns).Evict(ctx(t), &policyv1.Eviction{ObjectMeta: metav1.ObjectMeta{Namespace: l.ns, Name: p.Name}})
		}
		pending := 0
		wait.PollUntilContextTimeout(ctx(t), time.Second, 20*time.Second, true, func(cx context.Context) (bool, error) {
			all, _ := admin.CoreV1().Pods(l.ns).List(cx, metav1.ListOptions{})
			pending = 0
			for _, p := range all.Items {
				if p.Spec.NodeName == "" && p.DeletionTimestamp == nil {
					pending++
				}
			}
			return pending == 1, nil
		})
		t.Logf("with the rule hidden the kernel says Proved; on the cluster %d moved pod(s) stay Pending", pending)
		if pending != 1 {
			t.Errorf("%d pods Pending, want 1", pending)
		}
	})
}
