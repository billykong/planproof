// Package evals loads the agent's task set (tasks.yaml, PRD E-10) and grades
// the agent's runs on it (E-12).
package evals

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/billykong/planproof/agent"
	"github.com/billykong/planproof/internal/yamljson"
	"github.com/billykong/planproof/kernel"
)

// Spec is one task as tasks.yaml writes it.
type Spec struct {
	ID        string          `json:"id"`
	Ticket    string          `json:"ticket"`
	Snapshot  string          `json:"snapshot"`
	Policy    string          `json:"policy"`
	Expect    Expect          `json:"expect"`
	Reference json.RawMessage `json:"reference"`
	Naive     json.RawMessage `json:"naive"`
}

// Expect is a correct answer: the goal, or that no plan can be proved.
type Expect struct {
	Goal       []GoalItem `json:"goal"`
	NoSafePlan string     `json:"noSafePlan"`
}

// GoalItem is one expected change: object, field, container, to.
type GoalItem struct {
	Node       string `json:"node"`
	Deployment string `json:"deployment"`
	Field      string `json:"field"`
	Container  string `json:"container"`
	To         any    `json:"to"`
	From       any    `json:"from"` // ignored when grading: the kernel checks it
}

func (g GoalItem) key() string {
	to, _ := json.Marshal(g.To)
	return strings.Join([]string{g.Node, g.Deployment, g.Field, g.Container, string(to)}, "|")
}

// Root is the repository root, found from this package's directory.
func Root() string {
	wd, _ := os.Getwd()
	for d := wd; d != filepath.Dir(d); d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
	}
	return wd
}

// Load reads tasks.yaml.
func Load(root string) ([]Spec, error) {
	b, err := os.ReadFile(filepath.Join(root, "evals", "tasks.yaml"))
	if err != nil {
		return nil, err
	}
	// The task file is ours, not the kernel's input: allow it to be larger.
	if len(b) > yamljson.MaxBytes {
		return nil, fmt.Errorf("tasks.yaml is larger than %d bytes", yamljson.MaxBytes)
	}
	j, err := yamljson.Convert(b)
	if err != nil {
		return nil, fmt.Errorf("tasks.yaml: %w", err)
	}
	var specs []Spec
	return specs, json.Unmarshal(j, &specs)
}

// Task reads a spec's snapshot and policy as the kernel receives them.
func (s Spec) Task(root string) (agent.Task, error) {
	read := func(p string) ([]byte, error) {
		b, err := os.ReadFile(filepath.Join(root, p))
		if err != nil || !strings.HasSuffix(p, ".yaml") {
			return b, err
		}
		return yamljson.Convert(b)
	}
	snap, err := read(s.Snapshot)
	if err != nil {
		return agent.Task{}, err
	}
	pol, err := read(s.Policy)
	if err != nil {
		return agent.Task{}, err
	}
	return agent.Task{ID: s.ID, Ticket: s.Ticket, Snapshot: snap, Policy: pol}, nil
}

// Grade is one run's result.
type Grade struct {
	Pass    bool   `json:"pass"`
	Outcome string `json:"outcome"`
}

// Outcomes, from best to worst. "proved-wrong-goal" is the dangerous one:
// the kernel proved a plan that doesn't do what the ticket asked (it
// checks the plan against its own goal, not against the ticket).
const (
	ProvedCorrect   = "proved-correct"
	Declared        = "declared-no-safe-plan"
	ProvedWrongGoal = "proved-wrong-goal"
	DeclaredWrongly = "declared-but-solvable"
	NotProved       = "not-proved"
	RunError        = "error"
)

// GradeRun grades a transcript against the spec.
func GradeRun(s Spec, tr agent.Transcript) Grade {
	if tr.Stopped == "error" {
		return Grade{false, RunError}
	}
	proved := len(tr.Attempts) > 0 && tr.Attempts[len(tr.Attempts)-1].Result == kernel.Proved
	if s.Expect.NoSafePlan != "" {
		switch {
		case proved: // can't happen if the task is sound (TestTasks)
			return Grade{false, ProvedWrongGoal}
		case tr.NoPlan:
			return Grade{true, Declared}
		}
		return Grade{false, NotProved}
	}
	switch {
	case proved && SameGoal(tr.Attempts[len(tr.Attempts)-1].Plan, s.Expect.Goal):
		return Grade{true, ProvedCorrect}
	case proved:
		return Grade{false, ProvedWrongGoal}
	case tr.NoPlan:
		return Grade{false, DeclaredWrongly}
	}
	return Grade{false, NotProved}
}

// SameGoal: the plan's goal is exactly the expected one, in any order.
func SameGoal(plan json.RawMessage, want []GoalItem) bool {
	var p struct {
		Goal []GoalItem `json:"goal"`
	}
	if json.Unmarshal(plan, &p) != nil {
		return false
	}
	keys := func(items []GoalItem) []string {
		var out []string
		for _, g := range items {
			out = append(out, g.key())
		}
		slices.Sort(out)
		return out
	}
	return slices.Equal(keys(p.Goal), keys(want))
}

// Seeded is one seeded bad plan (seeded.yaml, E-11).
type Seeded struct {
	ID    string          `json:"id"`
	Task  string          `json:"task"`
	Catch json.RawMessage `json:"catch"`
	Plan  json.RawMessage `json:"plan"`
}

// Catch is what should catch a seeded plan: "rule:X", "trigger:X",
// "frame" or "ticket".
func (s Seeded) Want() string {
	var label string
	if json.Unmarshal(s.Catch, &label) == nil {
		return label
	}
	var m map[string]string
	json.Unmarshal(s.Catch, &m)
	for k, v := range m {
		return k + ":" + v
	}
	return ""
}

// LoadSeeded reads seeded.yaml.
func LoadSeeded(root string) ([]Seeded, error) {
	b, err := os.ReadFile(filepath.Join(root, "evals", "seeded.yaml"))
	if err != nil {
		return nil, err
	}
	j, err := yamljson.Convert(b)
	if err != nil {
		return nil, fmt.Errorf("seeded.yaml: %w", err)
	}
	var out []Seeded
	return out, json.Unmarshal(j, &out)
}

// SeededResult is what happened to one seeded plan.
type SeededResult struct {
	ID, Want, Kernel string
	Rules            []string
	Trigger          string
	CaughtBy         string // "kernel", "grader (goal vs ticket)", or "" for missed
}

// CheckSeeded runs a seeded plan through the kernel, and the grader for
// goal-vs-ticket plans.
func CheckSeeded(root string, specs map[string]Spec, s Seeded) (SeededResult, error) {
	spec, ok := specs[s.Task]
	if !ok {
		return SeededResult{}, fmt.Errorf("%s: no task %q", s.ID, s.Task)
	}
	task, err := spec.Task(root)
	if err != nil {
		return SeededResult{}, err
	}
	v := kernel.Check(task.Snapshot, agent.Plan(task, s.Plan), task.Policy)
	r := SeededResult{ID: s.ID, Want: s.Want(), Kernel: string(v.Result)}
	for _, f := range v.Facts {
		if f.Type == "failure" {
			r.Rules = append(r.Rules, f.Rule)
		}
		if f.Type == "unknown" {
			r.Trigger = f.Trigger
		}
	}
	switch want := r.Want; {
	case strings.HasPrefix(want, "rule:") && v.Result == kernel.Refuted && slices.Contains(r.Rules, strings.TrimPrefix(want, "rule:")),
		strings.HasPrefix(want, "trigger:") && v.Result == kernel.Unknown && r.Trigger == strings.TrimPrefix(want, "trigger:"),
		want == "frame" && v.Result == kernel.Refuted && slices.Contains(r.Rules, "frame"):
		r.CaughtBy = "kernel"
	case want == "ticket" && v.Result == kernel.Proved && !SameGoal(s.Plan, spec.Expect.Goal):
		r.CaughtBy = "grader (goal vs ticket)"
	}
	return r, nil
}
