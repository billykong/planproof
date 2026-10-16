package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	"github.com/billykong/planproof/fetch"
	"github.com/billykong/planproof/kernel"
)

// Runner runs admitted plans with the executor's identity.
type Runner struct {
	Client      kubernetes.Interface
	StepTimeout time.Duration // for a drain, whose plan step has no timeout of its own
	Poll        time.Duration
	// BeforeWrite, if set (tests only), runs between reading an object and
	// patching it: the place where a concurrent change must be caught.
	BeforeWrite func(Call)
}

// Report says what happened. On "stopped", nothing after Step ran, and the
// plan's checked rollback is offered to a person, never run (C-15).
type Report struct {
	Result   string        `json:"result"` // "done", "stopped" or "refused"
	Step     int           `json:"step,omitempty"`
	Reason   string        `json:"reason,omitempty"`
	Done     []Done        `json:"done"`
	Rollback []kernel.Step `json:"rollbackOffered,omitempty"`
}

// Done is one API call made, for the record and the translation check.
type Done struct {
	Step      int    `json:"step"`
	Verb      string `json:"verb"`
	Resource  string `json:"resource"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

// Run admits the plan, checks for drift, and runs it step by step.
func (r *Runner) Run(ctx context.Context, in Inputs, approvedPlan string) Report {
	rep := Report{Done: []Done{}}
	v, plan, err := Admit(in, approvedPlan)
	if err != nil {
		return Report{Result: "refused", Reason: err.Error(), Done: []Done{}}
	}
	calls, err := Compile(plan.Steps, v.PlanSHA256)
	if err != nil {
		return Report{Result: "refused", Reason: err.Error(), Done: []Done{}}
	}
	stop := func(step int, format string, args ...any) Report {
		rep.Result, rep.Step, rep.Reason, rep.Rollback = "stopped", step, fmt.Sprintf(format, args...), plan.Rollback
		return rep
	}
	if err := r.drift(ctx, in.Snapshot, plan); err != nil {
		return stop(0, "the cluster changed since the snapshot: %v", err)
	}
	for i, s := range plan.Steps {
		n := i + 1
		if err := r.assert(ctx, s, s.Pre); err != nil {
			return stop(n, "before step %d: %v", n, err)
		}
		if err := r.call(ctx, calls[i], &rep); err != nil {
			return stop(n, "step %d: %v", n, err)
		}
		if err := r.assert(ctx, s, s.Post); err != nil {
			return stop(n, "after step %d: %v", n, err)
		}
	}
	rep.Result = "done"
	return rep
}

func (r *Runner) call(ctx context.Context, c Call, rep *Report) error {
	record := func(verb, resource, ns, name string) {
		rep.Done = append(rep.Done, Done{c.Step, verb, resource, ns, name})
	}
	switch c.Verb {
	case "patch":
		if err := r.patch(ctx, c); err != nil {
			return err
		}
		record("patch", c.Resource, c.Namespace, c.Name)
		return nil
	case "evict-pods-on-node":
		return r.drain(ctx, c.Name, func(ns, name string) { record("create", "pods/eviction", ns, name) })
	case "wait":
		return r.wait(ctx, c.Deployments, time.Duration(c.TimeoutSeconds)*time.Second)
	}
	return fmt.Errorf("unknown call %q", c.Verb)
}

// patch sends a strategic merge patch with the object's resourceVersion as
// a precondition: if anything changed the object since it was read, the API
// server answers 409 Conflict and the run stops (C-13).
func (r *Runner) patch(ctx context.Context, c Call) error {
	var rv string
	var err error
	switch c.Resource {
	case "nodes":
		var n *corev1.Node
		n, err = r.Client.CoreV1().Nodes().Get(ctx, c.Name, metav1.GetOptions{})
		if n != nil {
			rv = n.ResourceVersion
		}
	case "deployments":
		d, e := r.Client.AppsV1().Deployments(c.Namespace).Get(ctx, c.Name, metav1.GetOptions{})
		err = e
		if d != nil {
			rv = d.ResourceVersion
		}
	}
	if err != nil {
		return err
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(c.Patch), &body); err != nil {
		return err
	}
	body["metadata"] = map[string]any{"resourceVersion": rv}
	patch, _ := json.Marshal(body)
	if r.BeforeWrite != nil {
		r.BeforeWrite(c)
	}
	opts := metav1.PatchOptions{FieldManager: "planproof-executor"}
	if c.Resource == "nodes" {
		_, err = r.Client.CoreV1().Nodes().Patch(ctx, c.Name, types.StrategicMergePatchType, patch, opts)
	} else {
		_, err = r.Client.AppsV1().Deployments(c.Namespace).Patch(ctx, c.Name, types.StrategicMergePatchType, patch, opts)
	}
	if apierrors.IsConflict(err) {
		return fmt.Errorf("%s %s changed while the step ran (resourceVersion conflict)", c.Resource, c.Name)
	}
	return err
}

// drain evicts every pod on the node except DaemonSet and static pods, only
// through the Eviction API, which also enforces the cluster's own budgets
// (C-14). An eviction a budget refuses (429) is retried until the step
// timeout. A pod that is not a Deployment's stops the run: the kernel only
// proved moving Deployment pods.
func (r *Runner) drain(ctx context.Context, node string, evicted func(ns, name string)) error {
	deadline := time.Now().Add(r.StepTimeout)
	asked := map[string]bool{}
	for {
		pods, err := r.Client.CoreV1().Pods("").List(ctx, metav1.ListOptions{FieldSelector: "spec.nodeName=" + node})
		if err != nil {
			return err
		}
		left := 0
		for _, p := range pods.Items {
			ref := metav1.GetControllerOf(&p)
			if p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed || ref != nil && (ref.Kind == "DaemonSet" || ref.Kind == "Node") {
				continue
			}
			if ref == nil || ref.Kind != "ReplicaSet" {
				return fmt.Errorf("pod %s/%s on %s is not a Deployment's; the kernel didn't prove moving it", p.Namespace, p.Name, node)
			}
			left++
			if p.DeletionTimestamp != nil {
				continue // evicted, terminating
			}
			err := r.Client.PolicyV1().Evictions(p.Namespace).Evict(ctx, &policyv1.Eviction{ObjectMeta: metav1.ObjectMeta{Namespace: p.Namespace, Name: p.Name}})
			switch {
			case err == nil:
				if key := p.Namespace + "/" + p.Name; !asked[key] {
					asked[key] = true
					evicted(p.Namespace, p.Name)
				}
			case apierrors.IsTooManyRequests(err), apierrors.IsNotFound(err):
				// A budget refused it for now, or it's already gone.
			default:
				return fmt.Errorf("evicting %s/%s: %w", p.Namespace, p.Name, err)
			}
		}
		if left == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("drain of %s not finished after %s (%d pods left; a disruption budget may be refusing evictions)", node, r.StepTimeout, left)
		}
		if err := sleep(ctx, r.Poll); err != nil {
			return err
		}
	}
}

// wait polls until every listed deployment has rolled out completely: the
// controller has seen the latest spec, and all replicas are updated, ready
// and available, with no old pods left (C-15).
func (r *Runner) wait(ctx context.Context, deps []string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		var waiting []string
		for _, ref := range deps {
			ns, name, _ := strings.Cut(ref, "/")
			d, err := r.Client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			want := int32(1)
			if d.Spec.Replicas != nil {
				want = *d.Spec.Replicas
			}
			st := d.Status
			if st.ObservedGeneration < d.Generation || st.UpdatedReplicas != want || st.ReadyReplicas != want || st.AvailableReplicas != want || st.Replicas != want {
				waiting = append(waiting, ref)
			}
		}
		if len(waiting) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("waited %s; not ready: %s", timeout, strings.Join(waiting, ", "))
		}
		if err := sleep(ctx, r.Poll); err != nil {
			return err
		}
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// assert checks a step's assertions against the live cluster (C-13).
func (r *Runner) assert(ctx context.Context, s kernel.Step, a *kernel.Assertions) error {
	if a == nil {
		return nil
	}
	if s.Deployment != "" {
		ns, name, _ := strings.Cut(s.Deployment, "/")
		d, err := r.Client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if a.Replicas != nil && (d.Spec.Replicas == nil || int64(*d.Spec.Replicas) != *a.Replicas) {
			return fmt.Errorf("%s: replicas is not %d", s.Deployment, *a.Replicas)
		}
		if a.Image != nil {
			c := a.Container
			if c == "" {
				c = s.Container
			}
			i := slices.IndexFunc(d.Spec.Template.Spec.Containers, func(x corev1.Container) bool { return x.Name == c })
			if i < 0 || d.Spec.Template.Spec.Containers[i].Image != *a.Image {
				return fmt.Errorf("%s: container %s is not %s", s.Deployment, c, *a.Image)
			}
		}
	}
	if s.Node != "" {
		n, err := r.Client.CoreV1().Nodes().Get(ctx, s.Node, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if a.Unschedulable != nil && n.Spec.Unschedulable != *a.Unschedulable {
			return fmt.Errorf("node %s: unschedulable is not %v", s.Node, *a.Unschedulable)
		}
		if a.PodsOnNode != nil {
			pods, err := r.Client.CoreV1().Pods("").List(ctx, metav1.ListOptions{FieldSelector: "spec.nodeName=" + s.Node})
			if err != nil {
				return err
			}
			var count int64
			for _, p := range pods.Items {
				if ref := metav1.GetControllerOf(&p); (ref == nil || ref.Kind != "DaemonSet") && p.Status.Phase != corev1.PodSucceeded && p.Status.Phase != corev1.PodFailed {
					count++
				}
			}
			if count != *a.PodsOnNode {
				return fmt.Errorf("node %s has %d pods, not %d", s.Node, count, *a.PodsOnNode)
			}
		}
	}
	return nil
}

// drift re-fetches the cluster and compares, for every object the plan
// depends on, the fields the kernel used (C-12). Not resourceVersions: a
// node's changes every few minutes with kubelet status updates alone. The
// window between this check and each write is covered by the write's
// resourceVersion precondition.
func (r *Runner) drift(ctx context.Context, snapshot []byte, plan kernel.Plan) error {
	var old kernel.Snapshot
	if err := json.Unmarshal(snapshot, &old); err != nil {
		return err
	}
	fresh, err := fetch.Snapshot(ctx, r.Client, old.Cluster, time.Now())
	if err != nil {
		return err
	}
	nodes, deps := dependsOn(&old, plan)
	same := func(kind, name string, a, b any) error {
		x, _ := json.Marshal(a)
		y, _ := json.Marshal(b)
		if string(x) != string(y) {
			return fmt.Errorf("%s %s differs", kind, name)
		}
		return nil
	}
	for _, name := range nodes {
		a, b := find(old.Nodes, name, nodeKey), find(fresh.Nodes, name, nodeKey)
		if a != nil {
			a.ResourceVersion = ""
		}
		if b != nil {
			b.ResourceVersion = ""
		}
		if err := same("node", name, a, b); err != nil {
			return err
		}
		if err := same("pods on node", name, podsOn(old.Pods, name), podsOn(fresh.Pods, name)); err != nil {
			return err
		}
	}
	for _, name := range deps {
		a, b := find(old.Deployments, name, depKey), find(fresh.Deployments, name, depKey)
		if a != nil {
			a.ResourceVersion = ""
		}
		if b != nil {
			b.ResourceVersion = ""
		}
		if err := same("deployment", name, a, b); err != nil {
			return err
		}
		ns, _, _ := strings.Cut(name, "/")
		if err := same("disruption budgets in", ns, inNamespace(old.PDBs, ns), inNamespace(fresh.PDBs, ns)); err != nil {
			return err
		}
	}
	return nil
}

func nodeKey(n kernel.Node) string      { return n.Name }
func depKey(d kernel.Deployment) string { return d.Namespace + "/" + d.Name }

func find[T any](list []T, name string, key func(T) string) *T {
	for i := range list {
		if key(list[i]) == name {
			c := list[i]
			return &c
		}
	}
	return nil
}

func podsOn(pods []kernel.Pod, node string) []kernel.Pod {
	var out []kernel.Pod
	for _, p := range pods {
		if p.Node == node {
			out = append(out, p)
		}
	}
	return out
}

func inNamespace(pdbs []kernel.PDB, ns string) []kernel.PDB {
	var out []kernel.PDB
	for _, b := range pdbs {
		if b.Namespace == ns {
			out = append(out, b)
		}
	}
	return out
}

// dependsOn lists the nodes the plan names, and the deployments it names or
// whose pods are on a node it drains.
func dependsOn(s *kernel.Snapshot, plan kernel.Plan) (nodes, deps []string) {
	add := func(list *[]string, v string) {
		if v != "" && !slices.Contains(*list, v) {
			*list = append(*list, v)
		}
	}
	for _, c := range plan.Goal {
		add(&nodes, c.Node)
		add(&deps, c.Deployment)
	}
	for _, st := range append(slices.Clone(plan.Steps), plan.Rollback...) {
		add(&nodes, st.Node)
		add(&deps, st.Deployment)
		for _, d := range st.Deployments {
			add(&deps, d)
		}
		if st.Op == "drain" {
			for _, p := range s.Pods {
				if p.Node == st.Node && p.Owner.Kind == "Deployment" {
					add(&deps, p.Namespace+"/"+p.Owner.Name)
				}
			}
		}
	}
	return nodes, deps
}
