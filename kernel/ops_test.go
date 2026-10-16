package kernel

import (
	"slices"
	"testing"
)

// TestOperations checks what each operation does to the model's state
// (spec 03, K-20), step by step on the example cluster.
func TestOperations(t *testing.T) {
	in, err := parse(load(t, "snapshot.json"), load(t, "plan-proved.yaml"), load(t, "policy.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	start := func() *run { return newRun(in) }
	do := func(t *testing.T, r *run, s Step) {
		t.Helper()
		if !r.do(StepRef{Index: 1}, s) {
			t.Fatalf("step %+v stopped: %+v %+v", s, r.failures, r.unknown)
		}
	}
	n := func(v int64) *int64 { return &v }
	pod := func(r *run, name string) *mpod {
		for _, p := range r.st.pods {
			if p.name == name {
				return p
			}
		}
		t.Fatalf("no pod %s", name)
		return nil
	}

	t.Run("cordon and uncordon", func(t *testing.T) {
		r := start()
		do(t, r, Step{Op: "cordon", Node: "n1"})
		if !r.st.nodes["n1"].unsched || slices.Contains(r.st.schedulable(), "n1") {
			t.Fatal("n1 still schedulable")
		}
		do(t, r, Step{Op: "uncordon", Node: "n1"})
		if r.st.nodes["n1"].unsched {
			t.Fatal("n1 still cordoned")
		}
	})

	t.Run("drain", func(t *testing.T) {
		r := start()
		do(t, r, Step{Op: "cordon", Node: "n1"})
		do(t, r, Step{Op: "drain", Node: "n1"})
		a := pod(r, "shop/checkout-a")
		if a.ready || !a.created || !slices.Equal(a.nodes, []string{"n2", "n3", "n4"}) || a.at != "n2" {
			t.Fatalf("moved pod %+v", a)
		}
		if p := pod(r, "kube-system/kube-proxy-n1"); !p.ready || p.at != "n1" {
			t.Fatalf("DaemonSet pod moved: %+v", p)
		}
		if !r.st.nodes["n1"].drained || r.st.avail("shop/checkout") != 2 || r.st.avail("shop/cart") != 1 {
			t.Fatal("drained or availability wrong")
		}
		do(t, r, Step{Op: "wait", Deployments: []string{"shop/checkout", "shop/cart"}, TimeoutSeconds: n(60)})
		if r.st.avail("shop/checkout") != 3 || r.st.avail("shop/cart") != 2 {
			t.Fatal("wait didn't make the moved pods ready")
		}
		// Worst case: the moved checkout pod may be on any of its candidates,
		// so draining n3 counts it as moved again, with checkout-c: 1 left,
		// below the budget of 2.
		do(t, r, Step{Op: "cordon", Node: "n3"})
		if r.do(StepRef{Index: 2}, Step{Op: "drain", Node: "n3"}) || a.ready || r.failures[0].Rule != "budget" {
			t.Fatalf("a pod that might be on n3 wasn't treated as moved: %+v", r.failures)
		}
	})

	t.Run("scale up and down", func(t *testing.T) {
		r := start()
		do(t, r, Step{Op: "scale", Deployment: "shop/cart", Replicas: n(4)})
		pods := r.st.podsOf("shop/cart")
		if len(pods) != 4 || r.st.avail("shop/cart") != 2 || r.st.deps["shop/cart"].replicas != 4 {
			t.Fatalf("scale up: %d pods, %d available", len(pods), r.st.avail("shop/cart"))
		}
		for _, p := range pods[2:] {
			if p.ready || p.at == "" {
				t.Fatalf("new pod %+v", p)
			}
		}
		// Down to 1: the 2 pending pods go first, then 1 ready one; the one
		// left may be on any node the deployment's pods were on.
		do(t, r, Step{Op: "scale", Deployment: "shop/cart", Replicas: n(1)})
		left := r.st.podsOf("shop/cart")
		if len(left) != 1 || r.st.avail("shop/cart") != 1 || len(left[0].nodes) < 2 {
			t.Fatalf("scale down: %d left, %d available, nodes %v", len(left), r.st.avail("shop/cart"), left[0].nodes)
		}
	})

	t.Run("set-image and wait", func(t *testing.T) {
		r := start()
		img := "registry.example/checkout@sha256:" + string(slices.Repeat([]byte("e"), 64))
		do(t, r, Step{Op: "set-image", Deployment: "shop/checkout", Container: "web", Image: img})
		d := r.st.deps["shop/checkout"]
		if !d.rolling || d.floor != 3 || d.images["web"] != img || r.st.avail("shop/checkout") != 3 || len(r.st.podsOf("shop/checkout")) != 3 {
			t.Fatalf("rolling update: %+v", d)
		}
		do(t, r, Step{Op: "wait", Deployments: []string{"shop/checkout"}, TimeoutSeconds: n(60)})
		if d.rolling || r.st.avail("shop/checkout") != 3 {
			t.Fatal("wait didn't end the rolling update")
		}
	})

	t.Run("rollout-restart", func(t *testing.T) {
		r := start()
		do(t, r, Step{Op: "rollout-restart", Deployment: "shop/cart"})
		if d := r.st.deps["shop/cart"]; !d.restarted || !d.rolling {
			t.Fatalf("%+v", d)
		}
	})
}
