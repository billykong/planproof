package kernel

import (
	"encoding/json"
	"fmt"
)

// KernelVersion is recorded in every verdict; the executor checks it (SEC-05).
const KernelVersion = "0.1.0"

type Result string

const (
	Proved  Result = "Proved"
	Refuted Result = "Refuted"
	Unknown Result = "Unknown"
)

// Rules, in the order of spec 04. Every verdict has one check fact per rule.
var Rules = []string{
	"input", "scope", "protected", "limits", "assertions", "cordon-first", "no-orphans",
	"budget", "never-zero", "capacity", "settled", "frame", "rollback",
}

// Verdict is the kernel's answer (spec 05).
type Verdict struct {
	APIVersion       string `json:"apiVersion"`
	Kind             string `json:"kind"`
	Result           Result `json:"result"`
	KernelVersion    string `json:"kernelVersion"`
	PlanSHA256       string `json:"planSha256"`
	SnapshotSHA256   string `json:"snapshotSha256"`
	PolicySHA256     string `json:"policySha256"`
	ApprovalRequired bool   `json:"approvalRequired"`
	Facts            []Fact `json:"facts"`
}

// Fact is one typed record of what the kernel found. Only the fields that
// belong to its Type are set.
type Fact struct {
	Type     string         `json:"type"`
	Rule     string         `json:"rule,omitempty"`
	Outcome  string         `json:"outcome,omitempty"`
	Step     *StepRef       `json:"step,omitempty"`
	Phase    string         `json:"phase,omitempty"`
	Trigger  string         `json:"trigger,omitempty"`
	Object   string         `json:"object,omitempty"`
	Evidence map[string]any `json:"evidence,omitempty"`
}

// StepRef names where a fact applies: before the steps (0), a step (1..n),
// after the last step ("end"), or a rollback step ("rollback:n").
type StepRef struct {
	Index    int
	End      bool
	Rollback bool
}

func (s StepRef) MarshalJSON() ([]byte, error) {
	switch {
	case s.End:
		return json.Marshal("end")
	case s.Rollback:
		return json.Marshal(fmt.Sprintf("rollback:%d", s.Index))
	}
	return json.Marshal(s.Index)
}

// Outcomes of a check fact.
const (
	outcomePass          = "pass"
	outcomeFail          = "fail"
	outcomeIncomplete    = "incomplete"
	outcomeNotReached    = "not-reached"
	outcomeNotApplicable = "not-applicable"
)

// checkFacts returns one check fact per rule, with the given outcomes and
// every other rule not reached.
func checkFacts(outcomes map[string]string) []Fact {
	facts := make([]Fact, 0, len(Rules))
	for _, r := range Rules {
		o, ok := outcomes[r]
		if !ok {
			o = outcomeNotReached
		}
		facts = append(facts, Fact{Type: "check", Rule: r, Outcome: o})
	}
	return facts
}
