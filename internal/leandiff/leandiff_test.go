// Package leandiff checks that the Go kernel and the Lean checker
// (lean/, PRD area L) give the same verdicts (L-21, L-22): the result, the
// failing rules, the Unknown trigger and the approval reasons. The Lean
// checker's theorems are about the Lean code; these tests are what link
// them to the Go kernel that actually runs.
//
// It needs the built Lean executable (`make lean`). Without it the tests
// skip, unless PLANPROOF_LEAN_REQUIRED is set, as in CI.
package leandiff

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/billykong/planproof/agent"
	"github.com/billykong/planproof/evals"
	"github.com/billykong/planproof/internal/testcases"
	"github.com/billykong/planproof/internal/yamljson"
	"github.com/billykong/planproof/kernel"
)

// input is one check: the three inputs as the kernel receives them.
type input struct {
	name            string
	snap, plan, pol []byte
}

// summary is what both checkers report.
type summary struct {
	Result   string   `json:"result"`
	Rules    []string `json:"rules"`
	Trigger  string   `json:"trigger"`
	Approval []string `json:"approval"`
	Error    string   `json:"error,omitempty"`
}

func goSummary(v kernel.Verdict) summary {
	s := summary{Result: string(v.Result), Rules: []string{}, Approval: []string{}}
	for _, f := range v.Facts {
		switch f.Type {
		case "failure":
			if !slices.Contains(s.Rules, f.Rule) {
				s.Rules = append(s.Rules, f.Rule)
			}
		case "unknown":
			s.Trigger = f.Trigger
		case "approval":
			s.Approval = append(s.Approval, f.Reasons...)
		}
	}
	slices.SortFunc(s.Rules, func(a, b string) int { return slices.Index(kernel.Rules, a) - slices.Index(kernel.Rules, b) })
	return s
}

func leanBinary(t *testing.T) string {
	bin := os.Getenv("PLANPROOF_LEAN")
	if bin == "" {
		bin = filepath.Join(evals.Root(), "lean", ".lake", "build", "bin", "planproof-lean")
	}
	if _, err := os.Stat(bin); err != nil {
		if os.Getenv("PLANPROOF_LEAN_REQUIRED") != "" {
			t.Fatalf("Lean checker not built: %v (run make lean)", err)
		}
		t.Skipf("Lean checker not built (%v); run make lean", err)
	}
	return bin
}

// compare runs every input the Go kernel accepts through both checkers and
// fails on any disagreement. It returns the Go kernel's summaries of the
// inputs compared.
func compare(t *testing.T, ins []input) []summary {
	t.Helper()
	bin := leanBinary(t)
	var lines bytes.Buffer
	var want []summary
	var names []string
	for _, in := range ins {
		v := kernel.Check(in.snap, in.plan, in.pol)
		if slices.ContainsFunc(v.Facts, func(f kernel.Fact) bool { return f.Type == "failure" && f.Rule == "input" }) {
			continue // the Lean checker only sees inputs the Go kernel accepted
		}
		line, err := json.Marshal(map[string]json.RawMessage{"snapshot": in.snap, "plan": in.plan, "policy": in.pol})
		if err != nil {
			t.Fatal(err)
		}
		lines.Write(line)
		lines.WriteByte('\n')
		want = append(want, goSummary(v))
		names = append(names, in.name)
	}
	cmd := exec.Command(bin)
	cmd.Stdin = &lines
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("planproof-lean: %v", err)
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	i, bad := 0, 0
	for ; sc.Scan(); i++ {
		var got summary
		if err := json.Unmarshal(sc.Bytes(), &got); err != nil {
			t.Fatalf("%s: bad output %q", names[i], sc.Text())
		}
		if got.Error != "" || got.Result != want[i].Result || !slices.Equal(got.Rules, want[i].Rules) ||
			got.Trigger != want[i].Trigger || !slices.Equal(got.Approval, want[i].Approval) {
			if bad++; bad <= 20 {
				t.Errorf("%s:\n  go:   %+v\n  lean: %+v", names[i], want[i], got)
			}
		}
	}
	if i != len(want) {
		t.Fatalf("planproof-lean answered %d of %d inputs", i, len(want))
	}
	if bad > 0 {
		t.Errorf("%d of %d disagree", bad, len(want))
	}
	return want
}

// spread logs how many inputs reached each result, rule and trigger, so a
// corpus that only exercises a few outcomes shows.
func spread(t *testing.T, ss []summary) map[string]int {
	n := map[string]int{}
	for _, s := range ss {
		n[s.Result]++
		for _, r := range s.Rules {
			n["rule "+r]++
		}
		if s.Trigger != "" {
			n["trigger "+s.Trigger]++
		}
		for _, a := range s.Approval {
			n["approval "+a]++
		}
	}
	var b strings.Builder
	for _, k := range slices.Sorted(maps.Keys(n)) {
		fmt.Fprintf(&b, "%s=%d ", k, n[k])
	}
	t.Logf("%d inputs: %s", len(ss), b.String())
	return n
}

func mustConvert(t *testing.T, b []byte) []byte {
	j, err := yamljson.Convert(b)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

// corpus is every plan the project has (L-21): the shared case table, the
// worked examples, the tasks' reference and naive plans, the seeded plans
// and every plan submitted in the stored agent runs.
func corpus(t *testing.T) []input {
	root := evals.Root()
	var ins []input
	for _, c := range testcases.Cases {
		s, p, q := testcases.Inputs(c)
		ins = append(ins, input{"case " + c.Name, s, p, q})
	}
	ex := filepath.Join(root, "examples", "drain-two-nodes")
	exSnap, _ := os.ReadFile(filepath.Join(ex, "snapshot.json"))
	exPolY, _ := os.ReadFile(filepath.Join(ex, "policy.yaml"))
	for _, p := range []string{"proved", "no-wait", "extra-change"} {
		b, err := os.ReadFile(filepath.Join(ex, "plan-"+p+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		ins = append(ins, input{"example " + p, exSnap, mustConvert(t, b), mustConvert(t, exPolY)})
	}
	specs, err := evals.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	tasks := map[string]agent.Task{}
	for _, s := range specs {
		task, err := s.Task(root)
		if err != nil {
			t.Fatal(err)
		}
		tasks[s.ID] = task
		for kind, body := range map[string]json.RawMessage{"reference": s.Reference, "naive": s.Naive} {
			if body != nil {
				ins = append(ins, input{"task " + s.ID + " " + kind, task.Snapshot, agent.Plan(task, body), task.Policy})
			}
		}
	}
	seeded, err := evals.LoadSeeded(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range seeded {
		task := tasks[s.Task]
		ins = append(ins, input{"seeded " + s.ID, task.Snapshot, agent.Plan(task, s.Plan), task.Policy})
	}
	runs, _ := filepath.Glob(filepath.Join(root, "results", "agent-*.jsonl"))
	for _, path := range runs {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for n := 1; sc.Scan(); n++ {
			var tr agent.Transcript
			if err := json.Unmarshal(sc.Bytes(), &tr); err != nil {
				t.Fatal(err)
			}
			task := tasks[tr.Task]
			for i, a := range tr.Attempts {
				name := fmt.Sprintf("%s run %d attempt %d", filepath.Base(path), n, i+1)
				ins = append(ins, input{name, task.Snapshot, agent.Plan(task, a.Plan), task.Policy})
			}
		}
		f.Close()
	}
	return ins
}

func TestCorpus(t *testing.T) {
	ss := compare(t, corpus(t))
	spread(t, ss)
	if len(ss) < 300 {
		t.Errorf("only %d inputs compared; the corpus should have more than 300", len(ss))
	}
}

// TestGenerated compares plans made at random over the evals' snapshots and
// policies (L-22). Each plan is checked once to learn its effect from the
// kernel's frame failure, and that effect becomes its goal, so most plans
// get past the static rules and reach the simulation, the end checks and
// the rollback.
func TestGenerated(t *testing.T) {
	count := 10000
	if testing.Short() {
		count = 300
	}
	root := evals.Root()
	read := func(p string) []byte {
		b, err := os.ReadFile(filepath.Join(root, p))
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(p, ".yaml") {
			return mustConvert(t, b)
		}
		return b
	}
	snaps := [][]byte{read("examples/drain-two-nodes/snapshot.json"), read("evals/snapshots/tight.json"), read("evals/snapshots/hpa.json")}
	pols := [][]byte{read("examples/drain-two-nodes/policy.yaml"), read("evals/policies/relaxed.yaml")}
	rng := rand.New(rand.NewPCG(1, 2))
	var ins []input
	for i := range count {
		snap, pol := snaps[rng.IntN(len(snaps))], pols[rng.IntN(len(pols))]
		body := randomPlan(rng, snap, pol)
		plan := withSnapshot(body, snap, fmt.Sprintf("gen-%d", i))
		// Learn the effect and make it the goal.
		v := kernel.Check(snap, plan, pol)
		if goal := effect(v); goal != nil && rng.IntN(4) > 0 {
			body["goal"] = goal
			plan = withSnapshot(body, snap, fmt.Sprintf("gen-%d", i))
		}
		ins = append(ins, input{fmt.Sprintf("generated %d: %s", i, plan), snap, plan, pol})
	}
	n := spread(t, compare(t, ins))
	// Every result and every simulated rule must be reached, or the
	// generator isn't testing them.
	for _, k := range []string{"Proved", "Refuted", "Unknown", "rule limits", "rule assertions", "rule cordon-first",
		"rule budget", "rule never-zero", "rule capacity", "rule settled", "rule frame", "rule rollback",
		"approval drain", "approval critical", "approval scale-down"} {
		if n[k] == 0 {
			t.Errorf("no generated input reached %s", k)
		}
	}
}

func withSnapshot(body map[string]any, snap []byte, name string) []byte {
	m := map[string]any{"apiVersion": "planproof/v0", "kind": "Plan", "snapshot": kernel.Hash(snap),
		"metadata": map[string]any{"name": name}}
	for k, v := range body {
		m[k] = v
	}
	b, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return b
}

// effect turns a frame failure's extra changes into goal changes.
func effect(v kernel.Verdict) []any {
	for _, f := range v.Facts {
		if f.Type != "failure" || f.Rule != "frame" || f.Phase != "end" {
			continue
		}
		extra, _ := f.Evidence["extra"].([]any)
		var goal []any
		for _, e := range extra {
			e := e.(map[string]any)
			kind, name, _ := strings.Cut(e["object"].(string), "/")
			c := map[string]any{"field": e["field"], "from": e["from"], "to": e["to"]}
			c[kind] = name
			if ct, ok := e["container"]; ok {
				c["container"] = ct
			}
			goal = append(goal, c)
		}
		return goal
	}
	return nil
}

// randomPlan makes a plan over the snapshot's nodes and deployments: a
// third are well-formed drains (cordon the nodes, then drain each and
// wait), the rest up to 8 random steps with values near the policies'
// limits, an occasional assertion and an occasional rollback. Protected
// deployments are picked only now and then, or most plans would stop at
// rule protected.
func randomPlan(rng *rand.Rand, snap, pol []byte) map[string]any {
	var p struct {
		Protected []string `json:"protectedDeployments"`
	}
	if err := json.Unmarshal(pol, &p); err != nil {
		panic(err)
	}
	var s struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
		Deployments []struct {
			Namespace  string `json:"namespace"`
			Name       string `json:"name"`
			Replicas   int    `json:"replicas"`
			Containers []struct {
				Name string `json:"name"`
			} `json:"containers"`
		} `json:"deployments"`
	}
	if err := json.Unmarshal(snap, &s); err != nil {
		panic(err)
	}
	node := func() string { return s.Nodes[rng.IntN(len(s.Nodes))].Name }
	dep := func() (string, string, int) {
		for {
			d := s.Deployments[rng.IntN(len(s.Deployments))]
			name := d.Namespace + "/" + d.Name
			if !slices.Contains(p.Protected, name) || rng.IntN(10) == 0 {
				return name, d.Containers[0].Name, d.Replicas
			}
		}
	}
	var all []string
	for _, d := range s.Deployments {
		all = append(all, d.Namespace+"/"+d.Name)
	}
	images := []string{
		"registry.example/x@sha256:" + strings.Repeat("d", 64),
		"registry.example/x@sha256:" + strings.Repeat("e", 65),
		"registry.example/x:latest",
	}
	var touched []string
	step := func() map[string]any {
		switch rng.IntN(8) {
		case 0, 1:
			return map[string]any{"op": "cordon", "node": node()}
		case 2:
			return map[string]any{"op": "uncordon", "node": node()}
		case 3:
			n := node()
			st := map[string]any{"op": "drain", "node": n}
			if rng.IntN(5) == 0 {
				st["post"] = map[string]any{"podsOnNode": rng.IntN(3)}
			}
			return st
		case 4:
			d, _, r := dep()
			touched = append(touched, d)
			st := map[string]any{"op": "scale", "deployment": d, "replicas": max(0, r+rng.IntN(5)-2+rng.IntN(2)*rng.IntN(20))}
			if rng.IntN(5) == 0 {
				st["pre"] = map[string]any{"replicas": r + rng.IntN(2)}
			}
			return st
		case 5:
			d, c, _ := dep()
			touched = append(touched, d)
			return map[string]any{"op": "set-image", "deployment": d, "container": c, "image": images[rng.IntN(len(images))]}
		case 6:
			d, _, _ := dep()
			touched = append(touched, d)
			return map[string]any{"op": "rollout-restart", "deployment": d}
		default:
			d, _, _ := dep()
			deps := []string{d}
			if len(touched) > 0 && rng.IntN(2) == 0 {
				deps = []string{touched[rng.IntN(len(touched))]}
			}
			return map[string]any{"op": "wait", "deployments": deps, "timeoutSeconds": 100 + rng.IntN(700)}
		}
	}
	var steps []any
	if rng.IntN(3) == 0 {
		nodes := []string{node()}
		if n := node(); rng.IntN(2) == 0 && n != nodes[0] {
			nodes = append(nodes, n)
		}
		for _, n := range nodes {
			steps = append(steps, map[string]any{"op": "cordon", "node": n})
		}
		for _, n := range nodes {
			steps = append(steps, map[string]any{"op": "drain", "node": n},
				map[string]any{"op": "wait", "deployments": all, "timeoutSeconds": 300})
		}
	} else {
		for range 1 + rng.IntN(8) {
			steps = append(steps, step())
		}
	}
	if len(touched) > 0 && rng.IntN(3) > 0 {
		steps = append(steps, map[string]any{"op": "wait", "deployments": compact(touched), "timeoutSeconds": 300})
	}
	// A goal is required; this one is replaced by the plan's effect once the
	// kernel has reported it (TestGenerated).
	placeholder := []any{map[string]any{"node": s.Nodes[0].Name, "field": "drained", "from": false, "to": true}}
	plan := map[string]any{"goal": placeholder, "steps": steps}
	if rng.IntN(2) == 0 {
		var rb []any
		var scaled []string
		for _, st := range steps {
			switch st := st.(map[string]any); st["op"] {
			case "cordon":
				rb = append(rb, map[string]any{"op": "uncordon", "node": st["node"]})
			case "scale":
				for _, d := range s.Deployments {
					if name := d.Namespace + "/" + d.Name; name == st["deployment"] && !slices.Contains(scaled, name) {
						scaled = append(scaled, name)
						rb = append(rb, map[string]any{"op": "scale", "deployment": name, "replicas": d.Replicas})
					}
				}
			}
		}
		if len(scaled) > 0 {
			rb = append(rb, map[string]any{"op": "wait", "deployments": scaled, "timeoutSeconds": 300})
		}
		if len(rb) > 0 {
			plan["rollback"] = rb
		}
	}
	return plan
}

func compact(xs []string) []string {
	xs = slices.Clone(xs)
	slices.Sort(xs)
	return slices.Compact(xs)
}
