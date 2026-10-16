// Package fetch reads a cluster through a read-only identity and writes the
// snapshot the kernel checks against (spec 02, PRD area C). It resolves the
// Kubernetes details the kernel deliberately doesn't know about: owner
// chains, quantities, rollout strategies, disruption-budget selectors,
// autoscalers, taints and placement constraints.
//
// The fetcher is trusted: a wrong snapshot makes a correct kernel wrong
// (assumption A5). Where it can't resolve something exactly, it says so in
// the snapshot (an unresolved budget, a placement rule), and the kernel
// returns Unknown rather than guess.
package fetch

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"

	"github.com/billykong/planproof/kernel"
)

// Snapshot reads the cluster and builds a snapshot. now is recorded as
// fetchedAt; it is a parameter so tests are deterministic.
func Snapshot(ctx context.Context, c kubernetes.Interface, cluster string, now time.Time) (*kernel.Snapshot, error) {
	all := metav1.ListOptions{}
	nodes, err := c.CoreV1().Nodes().List(ctx, all)
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}
	pods, err := c.CoreV1().Pods("").List(ctx, all)
	if err != nil {
		return nil, fmt.Errorf("listing pods: %w", err)
	}
	deps, err := c.AppsV1().Deployments("").List(ctx, all)
	if err != nil {
		return nil, fmt.Errorf("listing deployments: %w", err)
	}
	rss, err := c.AppsV1().ReplicaSets("").List(ctx, all)
	if err != nil {
		return nil, fmt.Errorf("listing replicasets: %w", err)
	}
	pdbs, err := c.PolicyV1().PodDisruptionBudgets("").List(ctx, all)
	if err != nil {
		return nil, fmt.Errorf("listing poddisruptionbudgets: %w", err)
	}
	hpas, err := c.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, all)
	if err != nil {
		return nil, fmt.Errorf("listing horizontalpodautoscalers: %w", err)
	}

	s := &kernel.Snapshot{APIVersion: "planproof/v0", Kind: "Snapshot", Cluster: cluster, FetchedAt: now.UTC().Format(time.RFC3339),
		Nodes: []kernel.Node{}, Deployments: []kernel.Deployment{}, Pods: []kernel.Pod{}, PDBs: []kernel.PDB{}} // [] not null when empty
	for i := range nodes.Items {
		s.Nodes = append(s.Nodes, node(&nodes.Items[i]))
	}

	// Owner chains: a pod's ReplicaSet belongs to a Deployment, or not.
	rsOwner := map[string]string{}
	for _, rs := range rss.Items {
		if ref := metav1.GetControllerOf(&rs); ref != nil && ref.Kind == "Deployment" {
			rsOwner[rs.Namespace+"/"+rs.Name] = ref.Name
		}
	}
	scaled := map[string]bool{}
	for _, h := range hpas.Items {
		// KEDA ScaledObjects work through an HPA too, so this covers them.
		if h.Spec.ScaleTargetRef.Kind == "Deployment" {
			scaled[h.Namespace+"/"+h.Spec.ScaleTargetRef.Name] = true
		}
	}

	podOwner := map[string]string{} // ns/pod -> owning deployment name
	for i := range pods.Items {
		p := &pods.Items[i]
		if !live(p) {
			continue // not on a node, or finished: holds no place and no budget
		}
		kind, name := owner(p, rsOwner)
		if kind == "Deployment" {
			podOwner[p.Namespace+"/"+p.Name] = name
		}
		s.Pods = append(s.Pods, kernel.Pod{
			Namespace: p.Namespace, Name: p.Name, Node: p.Spec.NodeName, Ready: ptr(podReady(p)),
			Owner: kernel.Owner{Kind: kind, Name: name}, Requests: requests(&p.Spec),
			LocalStorage:   ptr(slices.ContainsFunc(p.Spec.Volumes, func(v corev1.Volume) bool { return v.EmptyDir != nil })),
			PlacementRules: ptr(placementRules(&p.Spec, nodes.Items)),
		})
	}

	for i := range deps.Items {
		s.Deployments = append(s.Deployments, deployment(&deps.Items[i], nodes.Items, scaled))
	}
	for i := range pdbs.Items {
		s.PDBs = append(s.PDBs, pdb(&pdbs.Items[i], deps.Items, pods.Items, podOwner))
	}

	slices.SortFunc(s.Nodes, func(a, b kernel.Node) int { return cmp.Compare(a.Name, b.Name) })
	slices.SortFunc(s.Pods, func(a, b kernel.Pod) int {
		return cmp.Or(cmp.Compare(a.Namespace, b.Namespace), cmp.Compare(a.Name, b.Name))
	})
	slices.SortFunc(s.Deployments, func(a, b kernel.Deployment) int {
		return cmp.Or(cmp.Compare(a.Namespace, b.Namespace), cmp.Compare(a.Name, b.Name))
	})
	slices.SortFunc(s.PDBs, func(a, b kernel.PDB) int {
		return cmp.Or(cmp.Compare(a.Namespace, b.Namespace), cmp.Compare(a.Name, b.Name))
	})
	return s, nil
}

// Marshal writes a snapshot as indented JSON with a final newline. The
// plan binds the hash of exactly these bytes.
func Marshal(s *kernel.Snapshot) ([]byte, error) {
	b, err := json.MarshalIndent(s, "", "  ")
	return append(b, '\n'), err
}

func ptr[T any](v T) *T { return &v }

// live: on a node and not finished.
func live(p *corev1.Pod) bool {
	return p.Spec.NodeName != "" && p.Status.Phase != corev1.PodSucceeded && p.Status.Phase != corev1.PodFailed
}

func node(n *corev1.Node) kernel.Node {
	ready := false
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady {
			ready = c.Status == corev1.ConditionTrue
		}
	}
	return kernel.Node{
		Name: n.Name, Ready: &ready, Unschedulable: ptr(n.Spec.Unschedulable), Taints: ptr(len(hardTaints(n)) > 0),
		Allocatable: kernel.Resources{
			CPUMilli:  ptr(n.Status.Allocatable.Cpu().MilliValue()),
			MemoryMiB: ptr(n.Status.Allocatable.Memory().Value() >> 20), // rounded down: less room, conservative
			Pods:      ptr(n.Status.Allocatable.Pods().Value()),
		},
		ResourceVersion: n.ResourceVersion,
	}
}

// hardTaints are the taints that keep pods off a node, apart from the one a
// cordon adds (the kernel models cordons itself).
func hardTaints(n *corev1.Node) []corev1.Taint {
	var out []corev1.Taint
	for _, t := range n.Spec.Taints {
		if (t.Effect == corev1.TaintEffectNoSchedule || t.Effect == corev1.TaintEffectNoExecute) && t.Key != corev1.TaintNodeUnschedulable {
			out = append(out, t)
		}
	}
	return out
}

func podReady(p *corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// owner resolves a pod's controller to the kinds the kernel knows.
func owner(p *corev1.Pod, rsOwner map[string]string) (kind, name string) {
	ref := metav1.GetControllerOf(p)
	switch {
	case ref == nil:
		return "None", ""
	case ref.Kind == "ReplicaSet":
		if d, ok := rsOwner[p.Namespace+"/"+ref.Name]; ok {
			return "Deployment", d
		}
	case ref.Kind == "DaemonSet" || ref.Kind == "StatefulSet" || ref.Kind == "Job":
		return ref.Kind, ref.Name
	}
	return "Other", ref.Name // a bare ReplicaSet, a static pod's Node, anything else
}

// requests is the pod's effective request, as the scheduler counts it: the
// larger of the containers' sum and the largest init container, plus
// overhead. Rounded up: more use, conservative.
func requests(spec *corev1.PodSpec) kernel.Resources {
	sum := corev1.ResourceList{}
	for _, c := range spec.Containers {
		add(sum, c.Resources.Requests)
	}
	for _, c := range spec.InitContainers {
		for name, q := range c.Resources.Requests {
			if cur, ok := sum[name]; !ok || q.Cmp(cur) > 0 {
				sum[name] = q.DeepCopy()
			}
		}
	}
	add(sum, spec.Overhead)
	mem := sum.Memory().Value()
	return kernel.Resources{CPUMilli: ptr(sum.Cpu().MilliValue()), MemoryMiB: ptr((mem + 1<<20 - 1) >> 20), Pods: ptr(int64(1))}
}

func add(sum, more corev1.ResourceList) {
	for name, q := range more {
		cur := sum[name]
		cur.Add(q)
		sum[name] = cur
	}
}

// placementRules: the pod has a hard placement constraint the model doesn't
// evaluate (spec 02). Soft preferences don't count: they change where a pod
// lands, never whether it can.
func placementRules(spec *corev1.PodSpec, nodes []corev1.Node) bool {
	if len(spec.NodeSelector) > 0 {
		sel := labels.SelectorFromSet(spec.NodeSelector)
		for _, n := range nodes {
			if !sel.Matches(labels.Set(n.Labels)) {
				return true
			}
		}
	}
	if a := spec.Affinity; a != nil {
		if a.NodeAffinity != nil && a.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil ||
			a.PodAffinity != nil && len(a.PodAffinity.RequiredDuringSchedulingIgnoredDuringExecution) > 0 ||
			a.PodAntiAffinity != nil && len(a.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution) > 0 {
			return true
		}
	}
	for _, t := range spec.TopologySpreadConstraints {
		if t.WhenUnsatisfiable == corev1.DoNotSchedule {
			return true
		}
	}
	// A pod that tolerates every hard taint of some tainted node could land
	// there, outside the candidates the model gives it.
	for i := range nodes {
		if taints := hardTaints(&nodes[i]); len(taints) > 0 && !slices.ContainsFunc(taints, func(t corev1.Taint) bool {
			return !slices.ContainsFunc(spec.Tolerations, func(tol corev1.Toleration) bool { return tolerates(tol, t) })
		}) {
			return true
		}
	}
	return false
}

// tolerates is Kubernetes' toleration matching. An operator it doesn't
// know counts as tolerating: that makes the pod a placement rule and the
// plan Unknown, the safe direction.
func tolerates(tol corev1.Toleration, t corev1.Taint) bool {
	if tol.Effect != "" && tol.Effect != t.Effect {
		return false
	}
	switch tol.Operator {
	case corev1.TolerationOpExists:
		return tol.Key == "" || tol.Key == t.Key
	case corev1.TolerationOpEqual, "":
		return tol.Key == t.Key && tol.Value == t.Value
	}
	return true
}

func deployment(d *appsv1.Deployment, nodes []corev1.Node, scaled map[string]bool) kernel.Deployment {
	replicas := int64(1)
	if d.Spec.Replicas != nil {
		replicas = int64(*d.Spec.Replicas)
	}
	updated := int64(d.Status.UpdatedReplicas)
	if d.Status.ObservedGeneration < d.Generation {
		updated = 0 // the controller hasn't seen the latest spec: not settled
	}
	strategy := kernel.Strategy{Type: string(d.Spec.Strategy.Type), MaxUnavailable: ptr(int64(0)), MaxSurge: ptr(int64(0))}
	if ru := d.Spec.Strategy.RollingUpdate; d.Spec.Strategy.Type == appsv1.RollingUpdateDeploymentStrategyType && ru != nil {
		// As the deployment controller does: surge rounds up, unavailable down.
		strategy.MaxSurge = ptr(scaledInt(ru.MaxSurge, replicas, true))
		strategy.MaxUnavailable = ptr(scaledInt(ru.MaxUnavailable, replicas, false))
	}
	var containers []kernel.Container
	for _, c := range d.Spec.Template.Spec.Containers {
		containers = append(containers, kernel.Container{Name: c.Name, Image: c.Image})
	}
	key := d.Namespace + "/" + d.Name
	return kernel.Deployment{
		Namespace: d.Namespace, Name: d.Name, Replicas: &replicas,
		ReadyReplicas: ptr(int64(d.Status.ReadyReplicas)), UpdatedReplicas: &updated,
		Strategy: strategy, Containers: containers, PodRequests: requests(&d.Spec.Template.Spec),
		PlacementRules: ptr(placementRules(&d.Spec.Template.Spec, nodes)), HPA: ptr(scaled[key]),
		ResourceVersion: d.ResourceVersion,
	}
}

func scaledInt(v *intstr.IntOrString, total int64, roundUp bool) int64 {
	if v == nil {
		return 0
	}
	n, err := intstr.GetScaledValueFromIntOrPercent(v, int(total), roundUp)
	if err != nil {
		return 0
	}
	return int64(n)
}

// pdb resolves a budget to exactly one deployment, or marks it unresolved
// (spec 02): the kernel then treats every deployment in its namespace as
// possibly covered, and says Unknown for plans that touch them.
func pdb(b *policyv1.PodDisruptionBudget, deps []appsv1.Deployment, pods []corev1.Pod, podOwner map[string]string) kernel.PDB {
	out := kernel.PDB{Namespace: b.Namespace, Name: b.Name}
	unresolved := func(reason string) kernel.PDB { out.Unresolved = reason; return out }
	if b.Spec.Selector == nil {
		return unresolved("no-selector")
	}
	sel, err := metav1.LabelSelectorAsSelector(b.Spec.Selector)
	if err != nil {
		return unresolved("bad-selector")
	}
	var match []string
	for _, d := range deps {
		if d.Namespace == b.Namespace && sel.Matches(labels.Set(d.Spec.Template.Labels)) {
			match = append(match, d.Name)
		}
	}
	if len(match) != 1 {
		return unresolved(fmt.Sprintf("selects-%d-deployments", len(match)))
	}
	for _, p := range pods {
		if p.Namespace == b.Namespace && sel.Matches(labels.Set(p.Labels)) && podOwner[p.Namespace+"/"+p.Name] != match[0] && live(&p) {
			return unresolved("selects-other-pods")
		}
	}
	for _, v := range []*intstr.IntOrString{b.Spec.MinAvailable, b.Spec.MaxUnavailable} {
		if v != nil && v.Type != intstr.Int {
			return unresolved("percentage")
		}
	}
	switch {
	case b.Spec.MinAvailable != nil:
		out.MinAvailable = ptr(int64(b.Spec.MinAvailable.IntValue()))
	case b.Spec.MaxUnavailable != nil:
		out.MaxUnavailable = ptr(int64(b.Spec.MaxUnavailable.IntValue()))
	default:
		return unresolved("no-value")
	}
	out.Deployment = &match[0]
	return out
}
