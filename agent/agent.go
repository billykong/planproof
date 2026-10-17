// Package agent is planproof's planning agent (PRD area E): a model writes
// an operations plan from a ticket, a snapshot and a policy, submits it to
// the kernel through one tool, reads the kernel's facts, and repairs the
// plan. It is untrusted: it holds no cluster credentials, and nothing it
// writes runs unless the kernel proves it.
//
// The loop is the same for every model, so results compare (E-04). Each
// model keeps its own native conversation (Claude's thinking blocks must
// go back unchanged, append-only), behind the Model interface.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/billykong/planproof/kernel"
	"github.com/billykong/planproof/render"
)

// Model is a planner the loop can drive.
type Model interface {
	Name() string
	Start(system, user string) Session
}

// Session is one conversation with a model.
type Session interface {
	// Next asks for the model's next turn: a plan submission or a final
	// answer in text.
	Next(ctx context.Context) (Turn, error)
	// Answer gives the model the result of its last submission.
	Answer(callID, content string)
	// Say adds a user message (the loop's one reminder).
	Say(text string)
}

// Turn is what the model did.
type Turn struct {
	Text  string // what it wrote (its final answer, if Call is nil)
	Call  *Call  // a submit_plan call, if any
	Usage Usage
}

// Call is one submit_plan call: the plan body the model wrote.
type Call struct {
	ID    string
	Input json.RawMessage
}

// Usage is the tokens one turn used.
type Usage struct {
	Input, Output, CacheRead, CacheWrite int64
}

func (u *Usage) Add(v Usage) {
	u.Input, u.Output, u.CacheRead, u.CacheWrite = u.Input+v.Input, u.Output+v.Output, u.CacheRead+v.CacheRead, u.CacheWrite+v.CacheWrite
}

// Task is one ticket for the agent, with the inputs the kernel checks
// against. Snapshot and Policy are the kernel's JSON bytes.
type Task struct {
	ID       string
	Ticket   string
	Snapshot []byte
	Policy   []byte
}

// Attempt is one submission and the kernel's answer.
type Attempt struct {
	Plan    json.RawMessage `json:"plan"`
	Result  kernel.Result   `json:"result"`
	Rules   []string        `json:"failedRules,omitempty"`
	Trigger string          `json:"unknownTrigger,omitempty"`
}

// Transcript is one run of the loop on one task.
type Transcript struct {
	Task     string    `json:"task"`
	Model    string    `json:"model"`
	Prompt   string    `json:"prompt"` // PromptVersion
	Kernel   string    `json:"kernel"` // kernel.KernelVersion
	Attempts []Attempt `json:"attempts"`
	Final    string    `json:"final,omitempty"` // the model's last text
	NoPlan   bool      `json:"declaredNoSafePlan"`
	Stopped  string    `json:"stopped"` // "proved", "declared", "gave-up", "tries", "error"
	Nudged   bool      `json:"nudged"`  // the model answered in text once and was reminded to use the tool
	Error    string    `json:"error,omitempty"`
	Usage    Usage     `json:"usage"`
}

// Reminder is the loop's one reminder when the model answers in text.
const Reminder = "To submit a plan, call the submit_plan tool; a plan written in text isn't checked. If no safe plan exists, reply with text starting \"NO SAFE PLAN:\"."

// NoSafePlan is how the model says no plan can be proved: its final text
// starts with this.
const NoSafePlan = "NO SAFE PLAN"

// Run drives one task: the model submits plans until one is Proved, it
// stops, or it has used maxTries submissions (E-01, E-02).
func Run(ctx context.Context, m Model, t Task, maxTries int) Transcript {
	tr := Transcript{Task: t.ID, Model: m.Name(), Prompt: PromptVersion, Kernel: kernel.KernelVersion, Attempts: []Attempt{}}
	s := m.Start(SystemPrompt, UserPrompt(t))
	for {
		turn, err := s.Next(ctx)
		tr.Usage.Add(turn.Usage)
		if err != nil {
			tr.Stopped, tr.Error = "error", err.Error()
			return tr
		}
		if turn.Text != "" {
			tr.Final = turn.Text
		}
		if turn.Call == nil {
			tr.NoPlan = strings.HasPrefix(strings.TrimSpace(strings.TrimLeft(turn.Text, "*#> ")), NoSafePlan)
			if !tr.NoPlan && !tr.Nudged {
				// One reminder, the same for every model: small models often
				// write the plan in text instead of calling the tool.
				tr.Nudged = true
				s.Say(Reminder)
				continue
			}
			tr.Stopped = map[bool]string{true: "declared", false: "gave-up"}[tr.NoPlan]
			return tr
		}
		plan := Plan(t, turn.Call.Input)
		v := kernel.Check(t.Snapshot, plan, t.Policy)
		a := Attempt{Plan: turn.Call.Input, Result: v.Result}
		for _, f := range v.Facts {
			switch f.Type {
			case "failure":
				a.Rules = append(a.Rules, f.Rule)
			case "unknown":
				a.Trigger = f.Trigger
			}
		}
		tr.Attempts = append(tr.Attempts, a)
		if v.Result == kernel.Proved {
			tr.Stopped = "proved"
			return tr
		}
		if len(tr.Attempts) >= maxTries {
			tr.Stopped = "tries"
			return tr
		}
		s.Answer(turn.Call.ID, Feedback(v, maxTries-len(tr.Attempts)))
	}
}

// Plan wraps the body the model wrote (goal, steps, rollback, rationale)
// in the plan header, bound to the task's snapshot. The model never
// computes hashes. Anything that isn't a JSON object is passed on as is,
// for the kernel to reject.
func Plan(t Task, body json.RawMessage) []byte {
	var m map[string]any
	if json.Unmarshal(body, &m) != nil || m == nil {
		return body
	}
	m["apiVersion"], m["kind"], m["snapshot"] = "planproof/v0", "Plan", kernel.Hash(t.Snapshot)
	m["metadata"] = map[string]any{"name": t.ID, "ticket": t.Ticket[:min(200, len(t.Ticket))]}
	b, _ := json.Marshal(m)
	return b
}

// Feedback is the tool result: the verdict's facts the model needs to
// repair a plan, compact, not the packet (E-02).
func Feedback(v kernel.Verdict, triesLeft int) string {
	out := map[string]any{"result": v.Result, "submissionsLeft": triesLeft}
	var failures []any
	for _, f := range v.Facts {
		switch f.Type {
		case "failure":
			x := map[string]any{"rule": f.Rule, "phase": f.Phase}
			if f.Step != nil {
				x["step"] = f.Step
			}
			if f.Object != "" {
				x["object"] = f.Object
			}
			if f.Evidence != nil {
				x["evidence"] = f.Evidence
			}
			failures = append(failures, x)
		case "unknown":
			out["unknown"] = map[string]any{"trigger": f.Trigger, "step": f.Step, "object": f.Object,
				"meaning": "the plan depends on something the kernel can't model; it can never be Proved as it is"}
		}
	}
	if failures != nil {
		out["failures"] = failures
	}
	// The same plain-language sentences the review packet shows a person.
	if explain := render.Explain(v); len(explain) > 0 {
		out["explanation"] = explain
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// UserPrompt is the task: the ticket, then the policy and the snapshot as
// compact JSON (the snapshot is the large part; models cache it).
func UserPrompt(t Task) string {
	return fmt.Sprintf("<ticket>\n%s\n</ticket>\n\n<policy>\n%s\n</policy>\n\n<snapshot>\n%s\n</snapshot>", t.Ticket, compact(t.Policy), compact(t.Snapshot))
}

func compact(b []byte) string {
	var v any
	if json.Unmarshal(b, &v) != nil {
		return string(b)
	}
	c, _ := json.Marshal(v)
	return string(c)
}
