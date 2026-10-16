package fetch

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/billykong/planproof/kernel"
)

var linux = map[string]string{"kubernetes.io/os": "linux"}

func readyNode(name string, ready bool, taints ...corev1.Taint) *corev1.Node {
	status := corev1.ConditionTrue
	if !ready {
		status = corev1.ConditionFalse
	}
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: linux, ResourceVersion: "7"},
		Spec:       corev1.NodeSpec{Taints: taints},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: status}},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourcePods: resource.MustParse("110"),
				corev1.ResourceMemory: resource.MustParse("3182608Ki"), // 3108.0156 MiB: rounds down to 3108
			},
		},
	}
}

func controller(kind, name string) []metav1.OwnerReference {
	yes := true
	return []metav1.OwnerReference{{Kind: kind, Name: name, Controller: &yes}}
}

func template(app string, spec corev1.PodSpec) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": app}}, Spec: spec}
}

func req(cpu, mem string) corev1.ResourceRequirements {
	return corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(cpu), corev1.ResourceMemory: resource.MustParse(mem)}}
}

func dep(ns, name string, replicas int32, spec corev1.PodSpec) *appsv1.Deployment {
	quarter := intstr.FromString("25%")
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, Generation: 1},
		Spec: appsv1.DeploymentSpec{Replicas: &replicas, Template: template(name, spec),
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType, RollingUpdate: &appsv1.RollingUpdateDeployment{MaxSurge: &quarter, MaxUnavailable: &quarter}}},
		Status: appsv1.DeploymentStatus{ObservedGeneration: 1, Replicas: replicas, ReadyReplicas: replicas, UpdatedReplicas: replicas},
	}
}

func pod(name, node string, owner []metav1.OwnerReference, labels map[string]string, spec corev1.PodSpec) *corev1.Pod {
	spec.NodeName = node
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: name, OwnerReferences: owner, Labels: labels},
		Spec:       spec,
		Status:     corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
	}
}

func budget(name string, sel map[string]string, min, max *intstr.IntOrString) *policyv1.PodDisruptionBudget {
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: name},
		Spec:       policyv1.PodDisruptionBudgetSpec{Selector: &metav1.LabelSelector{MatchLabels: sel}, MinAvailable: min, MaxUnavailable: max},
	}
}

func cluster() []runtime.Object {
	controlPlane := corev1.Taint{Key: "node-role.kubernetes.io/control-plane", Effect: corev1.TaintEffectNoSchedule}
	cordon := corev1.Taint{Key: corev1.TaintNodeUnschedulable, Effect: corev1.TaintEffectNoSchedule}
	n2 := readyNode("n2", true, cordon)
	n2.Spec.Unschedulable = true

	web := dep("shop", "web", 3, corev1.PodSpec{
		Containers:     []corev1.Container{{Name: "app", Image: "web@sha256:1", Resources: req("250m", "100Mi")}},
		InitContainers: []corev1.Container{{Name: "init", Image: "init", Resources: req("500m", "10Mi")}},
		NodeSelector:   linux, // every node has it: not a placement rule
	})
	web.Generation = 2 // not yet observed by the controller: not settled

	sys := dep("kube-system", "dns", 2, corev1.PodSpec{
		Containers:  []corev1.Container{{Name: "dns", Image: "dns"}},
		Tolerations: []corev1.Toleration{{Key: "node-role.kubernetes.io/control-plane", Effect: corev1.TaintEffectNoSchedule}},
		Affinity: &corev1.Affinity{PodAntiAffinity: &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{{Weight: 1}}}},
	})
	api := dep("shop", "api", 2, corev1.PodSpec{
		Containers: []corev1.Container{{Name: "api", Image: "api"}},
		Affinity: &corev1.Affinity{PodAntiAffinity: &corev1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{TopologyKey: "kubernetes.io/hostname"}}}},
	})
	recreate := dep("shop", "batch", 1, corev1.PodSpec{Containers: []corev1.Container{{Name: "b", Image: "b"}}})
	recreate.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}

	rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "web-abc", OwnerReferences: controller("Deployment", "web")}}
	bareRS := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "lonely"}}
	two := intstr.FromInt32(2)
	half := intstr.FromString("50%")
	pending := pod("web-abc-pending", "", controller("ReplicaSet", "web-abc"), map[string]string{"app": "web"}, corev1.PodSpec{})
	done := pod("job-done", "n1", controller("Job", "j"), nil, corev1.PodSpec{})
	done.Status.Phase = corev1.PodSucceeded

	return []runtime.Object{
		readyNode("cp", true, controlPlane), readyNode("n1", true), n2, readyNode("n3", false),
		web, sys, api, recreate, rs, bareRS,
		pod("web-abc-1", "n1", controller("ReplicaSet", "web-abc"), map[string]string{"app": "web"}, corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Resources: req("250m", "100Mi")}},
			Volumes:    []corev1.Volume{{Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
		}),
		pod("bare", "n1", nil, nil, corev1.PodSpec{}),
		pod("lonely-1", "n1", controller("ReplicaSet", "lonely"), nil, corev1.PodSpec{}),
		pod("agent-1", "n1", controller("DaemonSet", "agent"), nil, corev1.PodSpec{}),
		pod("static", "cp", controller("Node", "cp"), nil, corev1.PodSpec{}),
		pending, done,
		budget("web", map[string]string{"app": "web"}, &two, nil),
		budget("web-pct", map[string]string{"app": "web"}, nil, &half),
		budget("nothing", map[string]string{"app": "none"}, &two, nil),
		budget("everything", map[string]string{}, &two, nil),
		&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Namespace: "shop", Name: "api"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "api"}}},
	}
}

func TestSnapshot(t *testing.T) {
	s, err := Snapshot(context.Background(), fake.NewClientset(cluster()...), "test", time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	nodes, deps, pods, pdbs := map[string]kernel.Node{}, map[string]kernel.Deployment{}, map[string]kernel.Pod{}, map[string]kernel.PDB{}
	for _, n := range s.Nodes {
		nodes[n.Name] = n
	}
	for _, d := range s.Deployments {
		deps[d.Namespace+"/"+d.Name] = d
	}
	for _, p := range s.Pods {
		pods[p.Name] = p
	}
	for _, b := range s.PDBs {
		pdbs[b.Name] = b
	}
	check := func(what string, got, want any) {
		t.Helper()
		g, _ := json.Marshal(got)
		w, _ := json.Marshal(want)
		if string(g) != string(w) {
			t.Errorf("%s: got %s, want %s", what, g, w)
		}
	}

	// Nodes: memory rounded down; a cordon's taint is not a hard taint.
	check("n1 allocatable", nodes["n1"].Allocatable, kernel.Resources{CPUMilli: ptr(int64(2000)), MemoryMiB: ptr(int64(3108)), Pods: ptr(int64(110))})
	check("cp taints", *nodes["cp"].Taints, true)
	check("n2 cordoned, no hard taint", []bool{*nodes["n2"].Unschedulable, *nodes["n2"].Taints}, []bool{true, false})
	check("n3 ready", *nodes["n3"].Ready, false)

	// Deployments: effective requests (init container wins), percentages
	// resolved as the controller does, unobserved generation not settled.
	w := deps["shop/web"]
	check("web requests", w.PodRequests, kernel.Resources{CPUMilli: ptr(int64(500)), MemoryMiB: ptr(int64(100)), Pods: ptr(int64(1))})
	check("web strategy", w.Strategy, kernel.Strategy{Type: "RollingUpdate", MaxUnavailable: ptr(int64(0)), MaxSurge: ptr(int64(1))})
	check("web updated", *w.UpdatedReplicas, int64(0))
	check("web placement (selector every node has)", *w.PlacementRules, false)
	check("dns placement (tolerates the control plane)", *deps["kube-system/dns"].PlacementRules, true)
	check("api placement (required anti-affinity)", *deps["shop/api"].PlacementRules, true)
	check("api hpa", *deps["shop/api"].HPA, true)
	check("batch strategy", deps["shop/batch"].Strategy, kernel.Strategy{Type: "Recreate", MaxUnavailable: ptr(int64(0)), MaxSurge: ptr(int64(0))})

	// Pods: owner chains, skipped pods, local storage, memory rounded up.
	check("owners", []kernel.Owner{pods["web-abc-1"].Owner, pods["bare"].Owner, pods["lonely-1"].Owner, pods["agent-1"].Owner, pods["static"].Owner},
		[]kernel.Owner{{Kind: "Deployment", Name: "web"}, {Kind: "None"}, {Kind: "Other", Name: "lonely"}, {Kind: "DaemonSet", Name: "agent"}, {Kind: "Other", Name: "cp"}})
	check("web-abc-1 local storage", *pods["web-abc-1"].LocalStorage, true)
	if _, ok := pods["web-abc-pending"]; ok {
		t.Error("an unscheduled pod is in the snapshot")
	}
	if _, ok := pods["job-done"]; ok {
		t.Error("a finished pod is in the snapshot")
	}

	// Budgets: one exact match resolves; anything else is unresolved.
	check("web budget", []any{*pdbs["web"].Deployment, *pdbs["web"].MinAvailable}, []any{"web", 2})
	check("unresolved", []string{pdbs["web-pct"].Unresolved, pdbs["nothing"].Unresolved, pdbs["everything"].Unresolved},
		[]string{"percentage", "selects-0-deployments", "selects-3-deployments"})

	// And the kernel's strict parser accepts it.
	b, err := Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	plan := `{"apiVersion":"planproof/v0","kind":"Plan","metadata":{"name":"p"},"snapshot":"` + kernel.Hash(b) +
		`","goal":[{"node":"n1","field":"unschedulable","from":false,"to":true}],"steps":[{"op":"cordon","node":"n1"}]}`
	policy := `{"apiVersion":"planproof/v0","kind":"Policy","managedNamespaces":["shop"],"managedNodes":["n1"],"protectedDeployments":[],"criticalDeployments":[],"approvalRequiredFor":[],
"limits":{"maxReplicas":10,"maxUnschedulableNodes":2,"minSchedulableNodes":0,"maxWaitSeconds":60,"requireImageDigest":false,"allowScaleToZero":false}}`
	v := kernel.Check(b, []byte(plan), []byte(policy))
	for _, f := range v.Facts {
		if f.Type == "failure" && f.Rule == "input" {
			t.Fatalf("the kernel rejects the snapshot: %v", f.Evidence)
		}
	}
	if v.Result != kernel.Proved {
		t.Fatalf("cordon n1 on the fetched snapshot: %s", v.Result)
	}
}
