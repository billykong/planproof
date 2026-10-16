package executor

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/billykong/planproof/kernel"
)

// Inputs are the three files the kernel checked, as the kernel receives
// them (YAML already converted), and optionally the verdict a person read.
type Inputs struct {
	Snapshot, Plan, Policy []byte
	Verdict                []byte // optional; if given, it must match
}

// Admit decides whether a plan may run (C-10). It doesn't trust a verdict
// file: it runs the kernel again (the kernel is deterministic) and requires
// Proved, a matching verdict if one was given, and, if the policy asks for
// approval, an approval of exactly this plan's hash.
func Admit(in Inputs, approvedPlan string) (kernel.Verdict, kernel.Plan, error) {
	v := kernel.Check(in.Snapshot, in.Plan, in.Policy)
	var plan kernel.Plan
	if v.Result != kernel.Proved {
		return v, plan, fmt.Errorf("the kernel says %s, not Proved", v.Result)
	}
	if in.Verdict != nil {
		var given kernel.Verdict
		if err := json.Unmarshal(in.Verdict, &given); err != nil {
			return v, plan, fmt.Errorf("reading the verdict: %w", err)
		}
		if given.Result != v.Result || given.PlanSHA256 != v.PlanSHA256 || given.SnapshotSHA256 != v.SnapshotSHA256 ||
			given.PolicySHA256 != v.PolicySHA256 || given.KernelVersion != v.KernelVersion {
			return v, plan, errors.New("the verdict given is not the kernel's verdict for these inputs")
		}
	}
	if v.ApprovalRequired && approvedPlan != v.PlanSHA256 {
		if approvedPlan == "" {
			return v, plan, errors.New("the policy requires approval, and there is none")
		}
		return v, plan, fmt.Errorf("the approval is for %s, not this plan (%s)", approvedPlan, v.PlanSHA256)
	}
	if err := json.Unmarshal(in.Plan, &plan); err != nil { // the kernel parsed it strictly already
		return v, plan, err
	}
	return v, plan, nil
}
