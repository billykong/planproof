package kernel

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// Tests of the rules (K-21), Unknown triggers (K-22) and operations (K-20),
// as small variations on the drain-two-nodes example cluster:
//
//	n1: checkout-a, cart-a     n3: checkout-c, payments-a
//	n2: checkout-b, cart-b     n4: payments-b
//
// Budgets: checkout 2, cart 1, payments 1. payments is protected.

// variant checks a plan, given as the YAML of its goal, steps and rollback,
// against the example cluster and policy after optional edits.
func variant(t *testing.T, snapEdit, polEdit func(m map[string]any), planYAML string) Verdict {
	t.Helper()
	snap, pol := load(t, "snapshot.json"), load(t, "policy.yaml")
	if snapEdit != nil {
		snap = edit(t, snap, snapEdit)
	}
	if polEdit != nil {
		pol = edit(t, pol, polEdit)
	}
	body, err := yaml.YAMLToJSON([]byte(planYAML))
	if err != nil {
		t.Fatal(err)
	}
	plan := edit(t, body, func(m map[string]any) {
		m["apiVersion"], m["kind"], m["snapshot"] = "planproof/v0", "Plan", hash(snap)
		m["metadata"] = map[string]any{"name": "test"}
	})
	return Check(snap, plan, pol)
}

// named finds the object with the given name in a list in the snapshot.
func named(m map[string]any, list, name string) map[string]any {
	for _, o := range m[list].([]any) {
		if o := o.(map[string]any); o["name"] == name {
			return o
		}
	}
	panic("no " + name + " in " + list)
}

func addPod(m map[string]any, name, node, ownerKind, ownerName string) {
	m["pods"] = append(m["pods"].([]any), map[string]any{
		"namespace": "shop", "name": name, "node": node, "ready": true,
		"owner":    map[string]any{"kind": ownerKind, "name": ownerName},
		"requests": map[string]any{"cpuMilli": 100, "memoryMiB": 64, "pods": 1}, "localStorage": false, "placementRules": false,
	})
}

const (
	cordonN1 = `{node: n1, field: unschedulable, from: false, to: true}`
	drainN1  = `
goal:
  - ` + cordonN1 + `
  - {node: n1, field: drained, from: false, to: true}
steps:
  - {op: cordon, node: n1}
  - {op: drain, node: n1}
  - {op: wait, deployments: [shop/checkout, shop/cart], timeoutSeconds: 300}
`
	scaleCart = `
goal:
  - {deployment: shop/cart, field: replicas, from: 2, to: %d}
steps:
  - {op: scale, deployment: shop/cart, replicas: %d}
  - {op: wait, deployments: [shop/cart], timeoutSeconds: 300}
`
	imageCart = `
goal:
  - {deployment: shop/cart, field: image, container: web, from: "registry.example/cart@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", to: "%[1]s"}
steps:
  - {op: set-image, deployment: shop/cart, container: web, image: "%[1]s"}
  - {op: wait, deployments: [shop/cart], timeoutSeconds: 300}
`
	newDigest = "registry.example/cart@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
)

func TestRules(t *testing.T) {
	scaleTo := func(n int) string { return fmt.Sprintf(scaleCart, n, n) }
	limits := func(k string, v any) func(m map[string]any) {
		return func(m map[string]any) { m["limits"].(map[string]any)[k] = v }
	}
	cases := []struct {
		name          string
		snap, pol     func(m map[string]any)
		plan          string
		want          Result
		rule, trigger string
	}{
		// The nearby plans the others vary from.
		{name: "accept: drain n1", plan: drainN1, want: Proved},
		{name: "accept: scale cart to the replica limit", plan: scaleTo(20), want: Proved},
		{name: "accept: new image with a digest", plan: fmt.Sprintf(imageCart, newDigest), want: Proved},

		{name: "scope: node not managed", pol: func(m map[string]any) { m["managedNodes"] = []any{"n2", "n3", "n4"} }, plan: drainN1, want: Refuted, rule: "scope"},
		{name: "scope: deployment not in snapshot", plan: strings.Replace(drainN1, "shop/cart]", "shop/ghost]", 1), want: Refuted, rule: "scope"},
		{name: "scope: namespace not managed", pol: func(m map[string]any) { m["managedNamespaces"] = []any{"other"} }, plan: scaleTo(3), want: Refuted, rule: "scope"},
		{name: "scope: container not in deployment", plan: strings.ReplaceAll(fmt.Sprintf(imageCart, newDigest), "container: web", "container: api"), want: Refuted, rule: "scope"},
		{name: "protected", plan: strings.ReplaceAll(scaleTo(3), "shop/cart", "shop/payments"), want: Refuted, rule: "protected"},

		{name: "limits: replicas", plan: scaleTo(21), want: Refuted, rule: "limits"},
		{name: "limits: wait timeout", plan: strings.Replace(drainN1, "300", "601", 1), want: Refuted, rule: "limits"},
		{name: "limits: image without digest", plan: fmt.Sprintf(imageCart, "registry.example/cart:latest"), want: Refuted, rule: "limits"},
		{name: "limits: too many nodes cordoned", plan: `
goal: [{node: n1, field: unschedulable, from: false, to: true}, {node: n2, field: unschedulable, from: false, to: true}, {node: n3, field: unschedulable, from: false, to: true}]
steps: [{op: cordon, node: n1}, {op: cordon, node: n2}, {op: cordon, node: n3}]
`, want: Refuted, rule: "limits"},
		{name: "limits: too few schedulable nodes", pol: limits("minSchedulableNodes", 4), plan: drainN1, want: Refuted, rule: "limits"},
		{name: "limits: already broken in the snapshot, not made worse", snap: func(m map[string]any) { named(m, "nodes", "n4")["unschedulable"] = true },
			pol: limits("maxUnschedulableNodes", 0), plan: scaleTo(3), want: Proved},

		{name: "assertions: pre false", plan: strings.Replace(drainN1, "{op: cordon, node: n1}", "{op: cordon, node: n1, pre: {unschedulable: true}}", 1), want: Refuted, rule: "assertions"},
		{name: "assertions: post true", plan: strings.Replace(drainN1, "{op: drain, node: n1}", "{op: drain, node: n1, post: {podsOnNode: 0}}", 1), want: Proved},
		{name: "assertions: post false", plan: strings.Replace(drainN1, "{op: drain, node: n1}", "{op: drain, node: n1, post: {podsOnNode: 1}}", 1), want: Refuted, rule: "assertions"},
		{name: "assertions: replicas and image true", plan: strings.Replace(fmt.Sprintf(imageCart, newDigest), "image: \""+newDigest+"\"}", "image: \""+newDigest+"\", pre: {replicas: 2}, post: {image: \""+newDigest+"\"}}", 1), want: Proved},

		{name: "cordon-first", plan: `
goal: [{node: n1, field: drained, from: false, to: true}]
steps: [{op: drain, node: n1}, {op: wait, deployments: [shop/checkout, shop/cart], timeoutSeconds: 300}]
`, want: Refuted, rule: "cordon-first"},
		{name: "no-orphans", snap: func(m map[string]any) { addPod(m, "debug", "n1", "None", "") }, plan: drainN1, want: Refuted, rule: "no-orphans"},
		{name: "no-orphans: a DaemonSet pod stays", snap: func(m map[string]any) { addPod(m, "agent", "n1", "DaemonSet", "agent") }, plan: drainN1, want: Proved},

		{name: "budget: maxUnavailable form", snap: func(m map[string]any) {
			b := named(m, "pdbs", "checkout")
			delete(b, "minAvailable")
			b["maxUnavailable"] = 0
		}, plan: drainN1, want: Refuted, rule: "budget"},
		{name: "budget: already broken in the snapshot, not made worse", snap: func(m map[string]any) { named(m, "pdbs", "payments")["minAvailable"] = 3 }, plan: drainN1, want: Proved},
		{name: "never-zero: no budget", snap: func(m map[string]any) { m["pdbs"] = m["pdbs"].([]any)[1:] }, plan: `
goal: [{node: n1, field: unschedulable, from: false, to: true}, {node: n2, field: unschedulable, from: false, to: true}, {node: n1, field: drained, from: false, to: true}, {node: n2, field: drained, from: false, to: true}]
steps:
  - {op: cordon, node: n1}
  - {op: cordon, node: n2}
  - {op: drain, node: n1}
  - {op: wait, deployments: [shop/checkout], timeoutSeconds: 300}
  - {op: drain, node: n2}
  - {op: wait, deployments: [shop/checkout, shop/cart], timeoutSeconds: 300}
`, want: Refuted, rule: "never-zero"},
		{name: "never-zero: scale to zero allowed", snap: func(m map[string]any) { m["pdbs"] = m["pdbs"].([]any)[1:] }, pol: limits("allowScaleToZero", true), plan: scaleTo(0), want: Proved},
		{name: "never-zero: scale to zero not allowed", snap: func(m map[string]any) { m["pdbs"] = m["pdbs"].([]any)[1:] }, plan: scaleTo(0), want: Refuted, rule: "never-zero"},
		{name: "never-zero: Recreate strategy", snap: func(m map[string]any) {
			named(m, "deployments", "cart")["strategy"].(map[string]any)["type"] = "Recreate"
			m["pdbs"] = m["pdbs"].([]any)[1:]
		}, plan: fmt.Sprintf(imageCart, newDigest), want: Refuted, rule: "never-zero"},

		{name: "capacity: drain", snap: func(m map[string]any) {
			for _, n := range []string{"n2", "n3", "n4"} {
				named(m, "nodes", n)["allocatable"].(map[string]any)["cpuMilli"] = 900
			}
		}, plan: drainN1, want: Refuted, rule: "capacity"},
		{name: "capacity: scale up", plan: scaleTo(20), pol: limits("maxReplicas", 100), want: Proved},
		{name: "capacity: scale past the cluster", plan: scaleTo(60), pol: limits("maxReplicas", 100), want: Refuted, rule: "capacity"},
		{name: "capacity: surge", snap: func(m map[string]any) {
			// Every node full: n1-n3 run 3 pods each, n4 runs 2.
			for n, slots := range map[string]int{"n1": 3, "n2": 3, "n3": 3, "n4": 2} {
				named(m, "nodes", n)["allocatable"].(map[string]any)["pods"] = slots
			}
		}, plan: fmt.Sprintf(imageCart, newDigest), want: Refuted, rule: "capacity"},

		{name: "settled", plan: strings.Replace(drainN1, "  - {op: wait, deployments: [shop/checkout, shop/cart], timeoutSeconds: 300}\n", "", 1), want: Refuted, rule: "settled"},
		{name: "frame: a goal change that doesn't happen", plan: strings.Replace(drainN1, "steps:", "  - {node: n2, field: unschedulable, from: false, to: true}\nsteps:", 1), want: Refuted, rule: "frame"},
		{name: "frame: from differs from the snapshot", plan: strings.Replace(drainN1, cordonN1, "{node: n1, field: unschedulable, from: true, to: false}", 1), want: Refuted, rule: "frame"},
		{name: "frame: from equals to", plan: strings.Replace(drainN1, cordonN1, "{node: n1, field: unschedulable, from: false, to: false}", 1), want: Refuted, rule: "frame"},
		{name: "frame: field repeated", plan: strings.Replace(drainN1, cordonN1, cordonN1+"\n  - "+cordonN1, 1), want: Refuted, rule: "frame"},
		{name: "frame: changes that cancel out", plan: `
goal: [{deployment: shop/cart, field: replicas, from: 2, to: 3}]
steps:
  - {op: scale, deployment: shop/cart, replicas: 3}
  - {op: wait, deployments: [shop/cart], timeoutSeconds: 300}
  - {op: scale, deployment: shop/checkout, replicas: 4}
  - {op: wait, deployments: [shop/checkout], timeoutSeconds: 300}
  - {op: scale, deployment: shop/checkout, replicas: 3}
`, want: Proved},

		{name: "rollback: restores", plan: drainN1 + "rollback: [{op: uncordon, node: n1}]\n", want: Proved},
		{name: "rollback: doesn't restore", plan: drainN1 + "rollback: [{op: cordon, node: n2}]\n", want: Refuted, rule: "rollback"},
		{name: "rollback: breaks a rule", plan: drainN1 + "rollback: [{op: uncordon, node: n1}, {op: scale, deployment: shop/cart, replicas: 0}]\n", want: Refuted, rule: "rollback"},

		{name: "unknown: hpa", snap: func(m map[string]any) { named(m, "deployments", "cart")["hpa"] = true }, plan: drainN1, want: Unknown, trigger: "hpa"},
		{name: "unknown: pdb-unresolved", snap: func(m map[string]any) {
			m["pdbs"] = append(m["pdbs"].([]any), map[string]any{"namespace": "shop", "name": "wide", "deployment": nil, "unresolved": "selects-several"})
		}, plan: drainN1, want: Unknown, trigger: "pdb-unresolved"},
		{name: "unknown: two budgets on one deployment", snap: func(m map[string]any) {
			m["pdbs"] = append(m["pdbs"].([]any), map[string]any{"namespace": "shop", "name": "cart-2", "deployment": "cart", "minAvailable": 1})
		}, plan: drainN1, want: Unknown, trigger: "pdb-unresolved"},
		{name: "unknown: placement-rules", snap: func(m map[string]any) { named(m, "pods", "cart-a")["placementRules"] = true }, plan: drainN1, want: Unknown, trigger: "placement-rules"},
		{name: "unknown: tainted-nodes", snap: func(m map[string]any) { named(m, "nodes", "n4")["taints"] = true }, plan: drainN1, want: Unknown, trigger: "tainted-nodes"},
		{name: "unknown: stateful-owner", snap: func(m map[string]any) { addPod(m, "db-0", "n1", "StatefulSet", "db") }, plan: drainN1, want: Unknown, trigger: "stateful-owner"},
		{name: "unknown: local-storage", snap: func(m map[string]any) { named(m, "pods", "cart-a")["localStorage"] = true }, plan: drainN1, want: Unknown, trigger: "local-storage"},
		{name: "unknown: node-not-ready", snap: func(m map[string]any) { named(m, "nodes", "n1")["ready"] = false }, plan: drainN1, want: Unknown, trigger: "node-not-ready"},
		{name: "unknown: unsettled-start", snap: func(m map[string]any) { named(m, "deployments", "checkout")["readyReplicas"] = 2 }, plan: drainN1, want: Unknown, trigger: "unsettled-start"},
		{name: "unknown: overlapping-change", plan: strings.Replace(fmt.Sprintf(imageCart, newDigest), "  - {op: wait", "  - {op: rollout-restart, deployment: shop/cart}\n  - {op: wait", 1), want: Unknown, trigger: "overlapping-change"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := variant(t, c.snap, c.pol, c.plan)
			var rules []string
			trigger := ""
			for _, f := range v.Facts {
				switch f.Type {
				case "failure":
					rules = append(rules, f.Rule)
					if f.Rule == "input" {
						t.Fatalf("input rejected: %v", f.Evidence)
					}
				case "unknown":
					trigger = f.Trigger
				}
			}
			if v.Result != c.want || c.rule != "" && !slices.Contains(rules, c.rule) || trigger != c.trigger {
				t.Fatalf("got %s, failures %v, trigger %q; want %s %s%s", v.Result, rules, trigger, c.want, c.rule, c.trigger)
			}
		})
	}
}
