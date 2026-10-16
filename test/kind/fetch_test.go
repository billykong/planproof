package kind

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	authv1 "k8s.io/api/authorization/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	"github.com/billykong/planproof/fetch"
	"github.com/billykong/planproof/kernel"
)

// TestFetcherRBAC: the fetcher can read what the snapshot needs and
// nothing else (C-02). Each "can" is paired with a "cannot".
func TestFetcherRBAC(t *testing.T) {
	admin, _ := clients(t)
	type access struct {
		verb, group, resource, sub string
		want                       bool
	}
	for _, a := range []access{
		{"list", "", "nodes", "", true}, {"list", "", "pods", "", true}, {"get", "apps", "deployments", "", true},
		{"list", "apps", "replicasets", "", true}, {"list", "policy", "poddisruptionbudgets", "", true},
		{"list", "autoscaling", "horizontalpodautoscalers", "", true},
		{"get", "", "secrets", "", false}, {"list", "", "configmaps", "", false}, {"patch", "", "nodes", "", false},
		{"create", "", "pods", "eviction", false}, {"delete", "", "pods", "", false}, {"patch", "apps", "deployments", "", false},
		{"update", "apps", "deployments", "scale", false}, {"impersonate", "", "users", "", false},
	} {
		r, err := admin.AuthorizationV1().SubjectAccessReviews().Create(ctx(t), &authv1.SubjectAccessReview{Spec: authv1.SubjectAccessReviewSpec{
			User: fetcherSA, Groups: []string{"system:serviceaccounts", "system:serviceaccounts:planproof", "system:authenticated"},
			ResourceAttributes: &authv1.ResourceAttributes{Verb: a.verb, Group: a.group, Resource: a.resource, Subresource: a.sub},
		}}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if r.Status.Allowed != a.want {
			t.Errorf("fetcher %s %s/%s %s: allowed=%v, want %v", a.verb, a.group, a.resource, a.sub, r.Status.Allowed, a.want)
		}
	}
}

func pauseDeployment(ns, name string, replicas int32, cpu, mem string, spec func(*corev1.PodSpec)) *appsv1.Deployment {
	labels := map[string]string{"app": name}
	ps := corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: pause, ImagePullPolicy: corev1.PullNever,
		Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(cpu), corev1.ResourceMemory: resource.MustParse(mem)}}}},
		TerminationGracePeriodSeconds: new(int64)}
	if spec != nil {
		spec(&ps)
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Spec: appsv1.DeploymentSpec{Replicas: &replicas, Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: ps}},
	}
}

// fixture creates a namespace with deployments and budgets, waits for the
// deployments to be ready, and deletes the namespace when the test ends.
func fixture(t *testing.T, c *kubernetes.Clientset, ns string, objs ...any) {
	t.Helper()
	cx := ctx(t)
	if _, err := c.CoreV1().Namespaces().Create(cx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Wait until it's gone, so its pods can't leak into the next test's snapshot.
		cx := context.Background()
		c.CoreV1().Namespaces().Delete(cx, ns, metav1.DeleteOptions{})
		wait.PollUntilContextTimeout(cx, time.Second, 2*time.Minute, true, func(cx context.Context) (bool, error) {
			_, err := c.CoreV1().Namespaces().Get(cx, ns, metav1.GetOptions{})
			return apierrors.IsNotFound(err), nil
		})
	})
	var deps []string
	for _, o := range objs {
		var err error
		switch o := o.(type) {
		case *appsv1.Deployment:
			_, err = c.AppsV1().Deployments(ns).Create(cx, o, metav1.CreateOptions{})
			deps = append(deps, o.Name)
		case *policyv1.PodDisruptionBudget:
			_, err = c.PolicyV1().PodDisruptionBudgets(ns).Create(cx, o, metav1.CreateOptions{})
		case *autoscalingv2.HorizontalPodAutoscaler:
			_, err = c.AutoscalingV2().HorizontalPodAutoscalers(ns).Create(cx, o, metav1.CreateOptions{})
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range deps {
		waitReady(t, c, ns, d)
	}
}

func waitReady(t *testing.T, c *kubernetes.Clientset, ns, name string) {
	t.Helper()
	err := wait.PollUntilContextTimeout(ctx(t), time.Second, 2*time.Minute, true, func(cx context.Context) (bool, error) {
		d, err := c.AppsV1().Deployments(ns).Get(cx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		r := *d.Spec.Replicas
		return d.Status.ObservedGeneration >= d.Generation && d.Status.ReadyReplicas == r && d.Status.UpdatedReplicas == r && d.Status.Replicas == r, nil
	})
	if err != nil {
		t.Fatalf("%s/%s not ready: %v", ns, name, err)
	}
}

// TestFetchFixture: known objects on kind give a known snapshot (C-03),
// fetched with the fetcher's own identity.
func TestFetchFixture(t *testing.T) {
	admin, cfg := clients(t)
	ns := unique("pp-fetch")
	two := intstr.FromInt32(2)
	half := intstr.FromString("50%")
	one, eighty := int32(1), int32(80)
	fixture(t, admin, ns,
		pauseDeployment(ns, "web", 3, "100m", "32Mi", nil),
		pauseDeployment(ns, "api", 2, "50m", "16Mi", func(s *corev1.PodSpec) {
			s.Affinity = &corev1.Affinity{PodAntiAffinity: &corev1.PodAntiAffinity{RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{TopologyKey: "kubernetes.io/hostname", LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}}}}}
		}),
		&policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "web"}, Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}}, MinAvailable: &two}},
		&policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "api"}, Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}, MaxUnavailable: &half}},
		&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: "api"}, Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "Deployment", Name: "api"}, MinReplicas: &one, MaxReplicas: 3,
			Metrics: []autoscalingv2.MetricSpec{{Type: autoscalingv2.ResourceMetricSourceType, Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU, Target: autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType, AverageUtilization: &eighty}}}}}},
	)

	s, err := fetch.Snapshot(ctx(t), as(t, cfg, fetcherSA), "planproof", time.Now())
	if err != nil {
		t.Fatalf("fetching as the fetcher: %v", err)
	}
	deps, pdbs := map[string]kernel.Deployment{}, map[string]kernel.PDB{}
	for _, d := range s.Deployments {
		deps[d.Namespace+"/"+d.Name] = d
	}
	for _, b := range s.PDBs {
		pdbs[b.Namespace+"/"+b.Name] = b
	}
	web, api := deps[ns+"/web"], deps[ns+"/api"]
	if *web.Replicas != 3 || *web.ReadyReplicas != 3 || *web.UpdatedReplicas != 3 || *web.Strategy.MaxSurge != 1 || *web.Strategy.MaxUnavailable != 0 ||
		*web.PodRequests.CPUMilli != 100 || *web.PodRequests.MemoryMiB != 32 || *web.PlacementRules || *web.HPA {
		t.Errorf("web: %+v", web)
	}
	if !*api.PlacementRules || !*api.HPA {
		t.Errorf("api: placementRules %v, hpa %v; want both true", *api.PlacementRules, *api.HPA)
	}
	if b := pdbs[ns+"/web"]; b.Deployment == nil || *b.Deployment != "web" || *b.MinAvailable != 2 {
		t.Errorf("web budget: %+v", b)
	}
	if b := pdbs[ns+"/api"]; b.Unresolved != "percentage" {
		t.Errorf("api budget: %+v", b)
	}
	webPods := 0
	for _, p := range s.Pods {
		if p.Namespace == ns && p.Owner == (kernel.Owner{Kind: "Deployment", Name: "web"}) && *p.Ready {
			webPods++
		}
	}
	if webPods != 3 {
		t.Errorf("%d ready web pods owned by Deployment web, want 3", webPods)
	}

	// The kernel's strict parser accepts the live snapshot.
	b, _ := fetch.Marshal(s)
	plan := `{"apiVersion":"planproof/v0","kind":"Plan","metadata":{"name":"p"},"snapshot":"` + kernel.Hash(b) +
		`","goal":[{"node":"planproof-worker","field":"unschedulable","from":false,"to":true}],"steps":[{"op":"cordon","node":"planproof-worker"}]}`
	policy := `{"apiVersion":"planproof/v0","kind":"Policy","managedNamespaces":["` + ns + `"],"managedNodes":["planproof-worker"],"protectedDeployments":[],"criticalDeployments":[],"approvalRequiredFor":[],
"limits":{"maxReplicas":10,"maxUnschedulableNodes":1,"minSchedulableNodes":0,"maxWaitSeconds":60,"requireImageDigest":false,"allowScaleToZero":false}}`
	if v := kernel.Check(b, []byte(plan), []byte(policy)); v.Result != kernel.Proved {
		t.Errorf("cordon on the live snapshot: %s %+v", v.Result, v.Facts)
	}
}
