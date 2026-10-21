// Package testcases holds the plans the tests check against the worked
// example cluster: the kernel's rule tests (K-21, K-22) and the renderer's
// tests use the same table, so every verdict the kernel can give is also
// rendered and linted.
package testcases

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/billykong/planproof/internal/yamljson"
)

// Case is a variation on the drain-two-nodes example. Want is the expected
// result; Rule a rule that must fail, Trigger the expected Unknown trigger.
type Case struct {
	Name          string
	Snap, Pol     func(m map[string]any)
	Plan          string
	Want          string
	Rule, Trigger string
}

// ExampleDir is examples/drain-two-nodes, wherever the tests run from.
var ExampleDir = func() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "examples", "drain-two-nodes")
}()

// Load reads an example file as the kernel receives it: JSON as is, YAML
// converted to compact JSON with sorted keys, as the CLI does.
func Load(name string) []byte {
	b, err := os.ReadFile(filepath.Join(ExampleDir, name))
	if err != nil {
		panic(err)
	}
	if strings.HasSuffix(name, ".yaml") {
		if b, err = yamljson.Convert(b); err != nil {
			panic(err)
		}
	}
	return b
}

// Edit applies f to a JSON document as a generic value and re-encodes it.
func Edit(doc []byte, f func(m map[string]any)) []byte {
	var m map[string]any
	if err := json.Unmarshal(doc, &m); err != nil {
		panic(err)
	}
	f(m)
	out, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return out
}

// Inputs builds a case's snapshot, plan and policy. The plan is the YAML of
// its goal, steps and rollback; the header and snapshot hash are added.
func Inputs(c Case) (snap, plan, pol []byte) {
	snap, pol = Load("snapshot.json"), Load("policy.yaml")
	if c.Snap != nil {
		snap = Edit(snap, c.Snap)
	}
	if c.Pol != nil {
		pol = Edit(pol, c.Pol)
	}
	body, err := yamljson.Convert([]byte(c.Plan))
	if err != nil {
		panic(err)
	}
	plan = Edit(body, func(m map[string]any) {
		m["apiVersion"], m["kind"], m["snapshot"] = "planproof/v0", "Plan", Hash(snap)
		m["metadata"] = map[string]any{"name": slug(c.Name)}
	})
	return snap, plan, pol
}

// Hash is the kernel's hash of an input: sha256:<hex> of the exact bytes.
func Hash(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func slug(s string) string {
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, strings.ToLower(s))
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")[:min(63, len(strings.Trim(s, "-")))]
}

// named finds the object with the given name in a list in the snapshot.
func Named(m map[string]any, list, name string) map[string]any {
	for _, o := range m[list].([]any) {
		if o := o.(map[string]any); o["name"] == name {
			return o
		}
	}
	panic("no " + name + " in " + list)
}

func AddPod(m map[string]any, name, node, ownerKind, ownerName string) {
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

func scaleTo(n int) string { return fmt.Sprintf(scaleCart, n, n) }

func limits(k string, v any) func(m map[string]any) {
	return func(m map[string]any) { m["limits"].(map[string]any)[k] = v }
}

// Cases: every rule both ways, every Unknown trigger.
var Cases = []Case{

	// The nearby plans the others vary from.
	{Name: "accept: drain n1", Plan: drainN1, Want: "Proved"},
	{Name: "accept: scale cart to the replica limit", Plan: scaleTo(20), Want: "Proved"},
	{Name: "accept: new image with a digest", Plan: fmt.Sprintf(imageCart, newDigest), Want: "Proved"},

	{Name: "scope: node not managed", Pol: func(m map[string]any) { m["managedNodes"] = []any{"n2", "n3", "n4"} }, Plan: drainN1, Want: "Refuted", Rule: "scope"},
	{Name: "scope: deployment not in snapshot", Plan: strings.Replace(drainN1, "shop/cart]", "shop/ghost]", 1), Want: "Refuted", Rule: "scope"},
	{Name: "scope: namespace not managed", Pol: func(m map[string]any) { m["managedNamespaces"] = []any{"other"} }, Plan: scaleTo(3), Want: "Refuted", Rule: "scope"},
	{Name: "scope: container not in deployment", Plan: strings.ReplaceAll(fmt.Sprintf(imageCart, newDigest), "container: web", "container: api"), Want: "Refuted", Rule: "scope"},
	{Name: "accept: drain a node running a protected deployment's pod, and wait for it", Plan: `
goal: [{node: n3, field: unschedulable, from: false, to: true}, {node: n3, field: drained, from: false, to: true}]
steps: [{op: cordon, node: n3}, {op: drain, node: n3}, {op: wait, deployments: [shop/checkout, shop/payments], timeoutSeconds: 300}]
`, Want: "Proved"},
	{Name: "protected", Plan: strings.ReplaceAll(scaleTo(3), "shop/cart", "shop/payments"), Want: "Refuted", Rule: "protected"},

	{Name: "limits: replicas", Plan: scaleTo(21), Want: "Refuted", Rule: "limits"},
	{Name: "limits: wait timeout", Plan: strings.Replace(drainN1, "300", "601", 1), Want: "Refuted", Rule: "limits"},
	{Name: "limits: a digest one character too long", Plan: fmt.Sprintf(imageCart, newDigest+"d"), Want: "Refuted", Rule: "limits"},
	{Name: "limits: a digest that isn't hex", Plan: fmt.Sprintf(imageCart, strings.Replace(newDigest, "d", "g", 1)), Want: "Refuted", Rule: "limits"},
	{Name: "limits: image without digest", Plan: fmt.Sprintf(imageCart, "registry.example/cart:latest"), Want: "Refuted", Rule: "limits"},
	{Name: "limits: too many nodes cordoned", Plan: `
goal: [{node: n1, field: unschedulable, from: false, to: true}, {node: n2, field: unschedulable, from: false, to: true}, {node: n3, field: unschedulable, from: false, to: true}]
steps: [{op: cordon, node: n1}, {op: cordon, node: n2}, {op: cordon, node: n3}]
`, Want: "Refuted", Rule: "limits"},
	{Name: "limits: too few schedulable nodes", Pol: limits("minSchedulableNodes", 4), Plan: drainN1, Want: "Refuted", Rule: "limits"},
	{Name: "limits: already broken in the snapshot, not made worse", Snap: func(m map[string]any) { Named(m, "nodes", "n4")["unschedulable"] = true },
		Pol: limits("maxUnschedulableNodes", 0), Plan: scaleTo(3), Want: "Proved"},

	{Name: "assertions: pre false", Plan: strings.Replace(drainN1, "{op: cordon, node: n1}", "{op: cordon, node: n1, pre: {unschedulable: true}}", 1), Want: "Refuted", Rule: "assertions"},
	{Name: "assertions: post true", Plan: strings.Replace(drainN1, "{op: drain, node: n1}", "{op: drain, node: n1, post: {podsOnNode: 0}}", 1), Want: "Proved"},
	{Name: "assertions: post false", Plan: strings.Replace(drainN1, "{op: drain, node: n1}", "{op: drain, node: n1, post: {podsOnNode: 1}}", 1), Want: "Refuted", Rule: "assertions"},
	{Name: "assertions: replicas and image true", Plan: strings.Replace(fmt.Sprintf(imageCart, newDigest), "image: \""+newDigest+"\"}", "image: \""+newDigest+"\", pre: {replicas: 2}, post: {image: \""+newDigest+"\"}}", 1), Want: "Proved"},

	{Name: "cordon-first", Plan: `
goal: [{node: n1, field: drained, from: false, to: true}]
steps: [{op: drain, node: n1}, {op: wait, deployments: [shop/checkout, shop/cart], timeoutSeconds: 300}]
`, Want: "Refuted", Rule: "cordon-first"},
	{Name: "no-orphans", Snap: func(m map[string]any) { AddPod(m, "debug", "n1", "None", "") }, Plan: drainN1, Want: "Refuted", Rule: "no-orphans"},
	{Name: "no-orphans: a DaemonSet pod stays", Snap: func(m map[string]any) { AddPod(m, "agent", "n1", "DaemonSet", "agent") }, Plan: drainN1, Want: "Proved"},

	{Name: "budget: maxUnavailable form", Snap: func(m map[string]any) {
		b := Named(m, "pdbs", "checkout")
		delete(b, "minAvailable")
		b["maxUnavailable"] = 0
	}, Plan: drainN1, Want: "Refuted", Rule: "budget"},
	{Name: "budget: already broken in the snapshot, not made worse", Snap: func(m map[string]any) { Named(m, "pdbs", "payments")["minAvailable"] = 3 }, Plan: drainN1, Want: "Proved"},
	{Name: "never-zero: no budget", Snap: func(m map[string]any) { m["pdbs"] = m["pdbs"].([]any)[1:] }, Plan: `
goal: [{node: n1, field: unschedulable, from: false, to: true}, {node: n2, field: unschedulable, from: false, to: true}, {node: n1, field: drained, from: false, to: true}, {node: n2, field: drained, from: false, to: true}]
steps:
  - {op: cordon, node: n1}
  - {op: cordon, node: n2}
  - {op: drain, node: n1}
  - {op: wait, deployments: [shop/checkout], timeoutSeconds: 300}
  - {op: drain, node: n2}
  - {op: wait, deployments: [shop/checkout, shop/cart], timeoutSeconds: 300}
`, Want: "Refuted", Rule: "never-zero"},
	{Name: "never-zero: scale to zero allowed", Snap: func(m map[string]any) { m["pdbs"] = m["pdbs"].([]any)[1:] }, Pol: limits("allowScaleToZero", true), Plan: scaleTo(0), Want: "Proved"},
	{Name: "never-zero: scale to zero not allowed", Snap: func(m map[string]any) { m["pdbs"] = m["pdbs"].([]any)[1:] }, Plan: scaleTo(0), Want: "Refuted", Rule: "never-zero"},
	// Found while writing the Lean proof (kernel 0.3.0): the scaled-up pod may be
	// placed on n2, next to cart-b, so draining n1 moves neither and draining n2
	// takes both. Until 0.2.0 the model moved it to "the schedulable nodes" at
	// the n1 drain, forgot it could still be on n2, and proved this plan.
	{Name: "never-zero: a pod that may have stayed on another candidate node", Snap: func(m map[string]any) {
		var pods []any
		for _, p := range m["pods"].([]any) {
			if p.(map[string]any)["name"] != "cart-a" {
				pods = append(pods, p)
			}
		}
		m["pods"], m["pdbs"] = pods, m["pdbs"].([]any)[1:]
		d := Named(m, "deployments", "cart")
		d["replicas"], d["readyReplicas"], d["updatedReplicas"] = 1, 1, 1
	}, Plan: `
goal:
  - {deployment: shop/cart, field: replicas, from: 1, to: 2}
  - {node: n1, field: unschedulable, from: false, to: true}
  - {node: n2, field: unschedulable, from: false, to: true}
  - {node: n1, field: drained, from: false, to: true}
  - {node: n2, field: drained, from: false, to: true}
steps:
  - {op: scale, deployment: shop/cart, replicas: 2}
  - {op: wait, deployments: [shop/cart], timeoutSeconds: 300}
  - {op: cordon, node: n1}
  - {op: cordon, node: n2}
  - {op: drain, node: n1}
  - {op: wait, deployments: [shop/cart, shop/checkout], timeoutSeconds: 300}
  - {op: drain, node: n2}
  - {op: wait, deployments: [shop/cart, shop/checkout, shop/payments], timeoutSeconds: 300}
`, Want: "Refuted", Rule: "never-zero"},
	{Name: "never-zero: Recreate strategy", Snap: func(m map[string]any) {
		Named(m, "deployments", "cart")["strategy"].(map[string]any)["type"] = "Recreate"
		m["pdbs"] = m["pdbs"].([]any)[1:]
	}, Plan: fmt.Sprintf(imageCart, newDigest), Want: "Refuted", Rule: "never-zero"},

	{Name: "capacity: drain", Snap: func(m map[string]any) {
		for _, n := range []string{"n2", "n3", "n4"} {
			Named(m, "nodes", n)["allocatable"].(map[string]any)["cpuMilli"] = 900
		}
	}, Plan: drainN1, Want: "Refuted", Rule: "capacity"},
	{Name: "capacity: scale up", Plan: scaleTo(20), Pol: limits("maxReplicas", 100), Want: "Proved"},
	{Name: "capacity: scale past the cluster", Plan: scaleTo(60), Pol: limits("maxReplicas", 100), Want: "Refuted", Rule: "capacity"},
	{Name: "capacity: surge", Snap: func(m map[string]any) {
		// Every node full: n1-n3 run 3 pods each, n4 runs 2.
		for n, slots := range map[string]int{"n1": 3, "n2": 3, "n3": 3, "n4": 2} {
			Named(m, "nodes", n)["allocatable"].(map[string]any)["pods"] = slots
		}
	}, Plan: fmt.Sprintf(imageCart, newDigest), Want: "Refuted", Rule: "capacity"},

	{Name: "settled", Plan: strings.Replace(drainN1, "  - {op: wait, deployments: [shop/checkout, shop/cart], timeoutSeconds: 300}\n", "", 1), Want: "Refuted", Rule: "settled"},
	{Name: "frame: a goal change that doesn't happen", Plan: strings.Replace(drainN1, "steps:", "  - {node: n2, field: unschedulable, from: false, to: true}\nsteps:", 1), Want: "Refuted", Rule: "frame"},
	{Name: "frame: from differs from the snapshot", Plan: strings.Replace(drainN1, cordonN1, "{node: n1, field: unschedulable, from: true, to: false}", 1), Want: "Refuted", Rule: "frame"},
	{Name: "frame: from equals to", Plan: strings.Replace(drainN1, cordonN1, "{node: n1, field: unschedulable, from: false, to: false}", 1), Want: "Refuted", Rule: "frame"},
	{Name: "frame: field repeated", Plan: strings.Replace(drainN1, cordonN1, cordonN1+"\n  - "+cordonN1, 1), Want: "Refuted", Rule: "frame"},
	{Name: "frame: changes that cancel out", Plan: `
goal: [{deployment: shop/cart, field: replicas, from: 2, to: 3}]
steps:
  - {op: scale, deployment: shop/cart, replicas: 3}
  - {op: wait, deployments: [shop/cart], timeoutSeconds: 300}
  - {op: scale, deployment: shop/checkout, replicas: 4}
  - {op: wait, deployments: [shop/checkout], timeoutSeconds: 300}
  - {op: scale, deployment: shop/checkout, replicas: 3}
`, Want: "Proved"},

	{Name: "frame: an extra drain", Plan: `
goal: [{node: n1, field: unschedulable, from: false, to: true}]
steps: [{op: cordon, node: n1}, {op: drain, node: n1}, {op: wait, deployments: [shop/checkout, shop/cart], timeoutSeconds: 300}]
`, Want: "Refuted", Rule: "frame"},
	{Name: "frame: an extra uncordon", Snap: func(m map[string]any) { Named(m, "nodes", "n4")["unschedulable"] = true }, Plan: `
goal: [{node: n1, field: unschedulable, from: false, to: true}]
steps: [{op: cordon, node: n1}, {op: uncordon, node: n4}]
`, Want: "Refuted", Rule: "frame"},
	{Name: "frame: an extra restart and image", Plan: strings.Replace(scaleTo(3), "  - {op: wait", "  - {op: rollout-restart, deployment: shop/checkout}\n  - {op: wait, deployments: [shop/checkout], timeoutSeconds: 300}\n  - {op: set-image, deployment: shop/checkout, container: web, image: \""+newDigest+"\"}\n  - {op: wait, deployments: [shop/checkout], timeoutSeconds: 300}\n  - {op: wait", 1), Want: "Refuted", Rule: "frame"},
	{Name: "accept: restart, with a rollback that can't undo it", Plan: `
goal: [{deployment: shop/cart, field: restarted, from: false, to: true}]
steps: [{op: rollout-restart, deployment: shop/cart}, {op: wait, deployments: [shop/cart], timeoutSeconds: 300}]
rollback: [{op: wait, deployments: [shop/cart], timeoutSeconds: 60}]
`, Want: "Proved"},
	{Name: "accept: approval for every plan and for images", Pol: func(m map[string]any) { m["approvalRequiredFor"] = []any{"always", "image"} },
		Plan: fmt.Sprintf(imageCart, newDigest), Want: "Proved"},
	{Name: "rollback: restores", Plan: drainN1 + "rollback: [{op: uncordon, node: n1}]\n", Want: "Proved"},
	{Name: "rollback: doesn't restore", Plan: drainN1 + "rollback: [{op: cordon, node: n2}]\n", Want: "Refuted", Rule: "rollback"},
	{Name: "rollback: breaks a rule", Plan: drainN1 + "rollback: [{op: uncordon, node: n1}, {op: scale, deployment: shop/cart, replicas: 0}]\n", Want: "Refuted", Rule: "rollback"},

	{Name: "unknown: hpa", Snap: func(m map[string]any) { Named(m, "deployments", "cart")["hpa"] = true }, Plan: drainN1, Want: "Unknown", Trigger: "hpa"},
	{Name: "unknown: pdb-unresolved", Snap: func(m map[string]any) {
		m["pdbs"] = append(m["pdbs"].([]any), map[string]any{"namespace": "shop", "name": "wide", "deployment": nil, "unresolved": "selects-several"})
	}, Plan: drainN1, Want: "Unknown", Trigger: "pdb-unresolved"},
	{Name: "unknown: two budgets on one deployment", Snap: func(m map[string]any) {
		m["pdbs"] = append(m["pdbs"].([]any), map[string]any{"namespace": "shop", "name": "cart-2", "deployment": "cart", "minAvailable": 1})
	}, Plan: drainN1, Want: "Unknown", Trigger: "pdb-unresolved"},
	{Name: "unknown: placement-rules", Snap: func(m map[string]any) { Named(m, "pods", "cart-a")["placementRules"] = true }, Plan: drainN1, Want: "Unknown", Trigger: "placement-rules"},
	{Name: "accept: a tainted node no pod tolerates is just not a candidate", Snap: func(m map[string]any) { Named(m, "nodes", "n4")["taints"] = true }, Plan: drainN1, Want: "Proved"},
	{Name: "capacity: a tainted node adds no room", Snap: func(m map[string]any) {
		Named(m, "nodes", "n4")["taints"] = true
		for _, n := range []string{"n2", "n3"} {
			Named(m, "nodes", n)["allocatable"].(map[string]any)["cpuMilli"] = 900
		}
	}, Plan: drainN1, Want: "Refuted", Rule: "capacity"},
	{Name: "unknown: stateful-owner", Snap: func(m map[string]any) { AddPod(m, "db-0", "n1", "StatefulSet", "db") }, Plan: drainN1, Want: "Unknown", Trigger: "stateful-owner"},
	{Name: "unknown: local-storage", Snap: func(m map[string]any) { Named(m, "pods", "cart-a")["localStorage"] = true }, Plan: drainN1, Want: "Unknown", Trigger: "local-storage"},
	{Name: "unknown: node-not-ready", Snap: func(m map[string]any) { Named(m, "nodes", "n1")["ready"] = false }, Plan: drainN1, Want: "Unknown", Trigger: "node-not-ready"},
	{Name: "unknown: unsettled-start", Snap: func(m map[string]any) { Named(m, "deployments", "checkout")["readyReplicas"] = 2 }, Plan: drainN1, Want: "Unknown", Trigger: "unsettled-start"},
	{Name: "unknown: overlapping-change", Plan: strings.Replace(fmt.Sprintf(imageCart, newDigest), "  - {op: wait", "  - {op: rollout-restart, deployment: shop/cart}\n  - {op: wait", 1), Want: "Unknown", Trigger: "overlapping-change"},
}
