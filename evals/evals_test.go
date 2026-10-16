package evals

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/billykong/planproof/agent"
	"github.com/billykong/planproof/kernel"
)

func load(t *testing.T) ([]Spec, string) {
	t.Helper()
	root := Root()
	specs, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) < 10 {
		t.Fatalf("%d tasks, want at least 10 (E-10)", len(specs))
	}
	return specs, root
}

// TestTasks: the task set is sound. Every solvable task has a reference
// plan the kernel proves, with the expected goal; every other task has a
// naive plan the kernel doesn't prove.
func TestTasks(t *testing.T) {
	specs, root := load(t)
	seen := map[string]bool{}
	for _, s := range specs {
		t.Run(s.ID, func(t *testing.T) {
			if seen[s.ID] {
				t.Fatal("duplicate id")
			}
			seen[s.ID] = true
			task, err := s.Task(root)
			if err != nil {
				t.Fatal(err)
			}
			if (s.Expect.NoSafePlan == "") == (s.Expect.Goal == nil) {
				t.Fatal("expect needs exactly one of goal and noSafePlan")
			}
			plan, want := s.Reference, kernel.Proved
			if s.Expect.NoSafePlan != "" {
				plan = s.Naive
			}
			if plan == nil {
				t.Fatal("no reference (solvable) or naive (not solvable) plan")
			}
			v := kernel.Check(task.Snapshot, agent.Plan(task, plan), task.Policy)
			switch {
			case s.Expect.NoSafePlan == "" && v.Result != want:
				t.Fatalf("the reference plan is %s, want Proved: %s", v.Result, agent.Feedback(v, 0))
			case s.Expect.NoSafePlan == "" && !SameGoal(plan, s.Expect.Goal):
				t.Fatal("the reference plan's goal isn't the expected goal")
			case s.Expect.NoSafePlan != "" && v.Result == kernel.Proved:
				t.Fatal("the naive plan is Proved: the task is solvable")
			}
		})
	}
}

// TestScriptedRuns: the loop and the grading, with the scripted model.
func TestScriptedRuns(t *testing.T) {
	specs, root := load(t)
	byID := map[string]Spec{}
	for _, s := range specs {
		byID[s.ID] = s
	}
	broken := json.RawMessage(`{"goal": [], "steps": [{"op": "drain", "node": "n1"}]}`)
	wrongGoal := json.RawMessage(`{"goal": [{"deployment": "shop/cart", "field": "replicas", "from": 2, "to": 5}],
	  "steps": [{"op": "scale", "deployment": "shop/cart", "replicas": 5}, {"op": "wait", "deployments": ["shop/cart"], "timeoutSeconds": 300}]}`)
	cases := []struct {
		task    string
		model   agent.Scripted
		outcome string
		tries   int
	}{
		{"drain-two", agent.Scripted{Plans: []json.RawMessage{byID["drain-two"].Reference}}, ProvedCorrect, 1},
		{"drain-two", agent.Scripted{Plans: []json.RawMessage{broken, byID["drain-two"].Reference}}, ProvedCorrect, 2},
		{"scale-cart", agent.Scripted{Plans: []json.RawMessage{wrongGoal}}, ProvedWrongGoal, 1},
		{"scale-cart", agent.Scripted{Plans: []json.RawMessage{broken, broken, broken, broken, broken}}, NotProved, 4},
		{"scale-cart", agent.Scripted{Final: "NO SAFE PLAN: I give up"}, DeclaredWrongly, 0},
		{"payments-up", agent.Scripted{Plans: []json.RawMessage{byID["payments-up"].Naive}, Final: "NO SAFE PLAN: payments is protected."}, Declared, 1},
		{"payments-up", agent.Scripted{Plans: []json.RawMessage{byID["payments-up"].Naive}, Final: "Done."}, NotProved, 1},
	}
	for _, c := range cases {
		task, err := byID[c.task].Task(root)
		if err != nil {
			t.Fatal(err)
		}
		tr := agent.Run(context.Background(), c.model, task, 4)
		g := GradeRun(byID[c.task], tr)
		if g.Outcome != c.outcome || len(tr.Attempts) != c.tries {
			t.Errorf("%s: outcome %s after %d tries, want %s after %d (%+v)", c.task, g.Outcome, len(tr.Attempts), c.outcome, c.tries, tr)
		}
	}
}

// TestSeeded: every seeded bad plan is caught by what its label says (E-11,
// E-21): the kernel for rule, trigger and frame plans, and, for plans whose
// goal isn't what the ticket asked, only the goal check (the kernel proves
// those: it checks a plan against its own goal).
func TestSeeded(t *testing.T) {
	specs, root := load(t)
	byID := map[string]Spec{}
	for _, s := range specs {
		byID[s.ID] = s
	}
	seeded, err := LoadSeeded(root)
	if err != nil {
		t.Fatal(err)
	}
	mismatches := 0
	for _, s := range seeded {
		if w := s.Want(); w == "frame" || w == "ticket" {
			mismatches++
		}
		r, err := CheckSeeded(root, byID, s)
		if err != nil {
			t.Fatal(err)
		}
		if r.CaughtBy == "" {
			t.Errorf("%s: want %s; the kernel said %s %v %s", s.ID, r.Want, r.Kernel, r.Rules, r.Trigger)
		}
	}
	if len(seeded) < 20 || mismatches < 5 {
		t.Fatalf("%d seeded plans, %d intent mismatches; want at least 20 and 5 (E-11)", len(seeded), mismatches)
	}
}
