package kernel

import (
	"crypto/sha256"
	"encoding/hex"
)

// inputs is the three parsed and validated inputs.
type inputs struct {
	snapshot Snapshot
	plan     Plan
	policy   Policy
}

// Check is the kernel: a pure, deterministic function of its three inputs
// (spec 05). Every path that isn't a completed check returns Refuted or
// Unknown, never Proved (SEC-04, K-31).
func Check(snapshot, plan, policy []byte) Verdict {
	v := Verdict{
		APIVersion:     apiVersion,
		Kind:           "Verdict",
		KernelVersion:  KernelVersion,
		PlanSHA256:     hash(plan),
		SnapshotSHA256: hash(snapshot),
		PolicySHA256:   hash(policy),
	}
	in, err := parse(snapshot, plan, policy)
	if err == nil && in.plan.Snapshot != v.SnapshotSHA256 {
		err = &inputError{"plan", reasonSnapshotMismatch, "snapshot"}
	}
	if err != nil {
		v.Result = Refuted
		v.Facts = append(checkFacts(map[string]string{"input": outcomeFail}), Fact{
			Type: "failure", Rule: "input", Step: &StepRef{}, Phase: "static", Object: err.input,
			Evidence: map[string]any{"reason": err.reason, "path": err.path},
		})
		return v
	}
	// Phase 1 milestone 3 adds the model and the rules. Until then the
	// kernel can parse a plan but not check it, so it says so.
	v.Result = Unknown
	v.Facts = append(checkFacts(map[string]string{"input": outcomePass}), Fact{
		Type: "unknown", Trigger: "kernel-incomplete", Step: &StepRef{},
	})
	return v
}

// parse checks all sizes first, then parses and validates each input.
func parse(snapshot, plan, policy []byte) (*inputs, *inputError) {
	switch {
	case len(plan) > maxPlanBytes:
		return nil, &inputError{"plan", reasonTooLarge, ""}
	case len(snapshot) > maxSnapshotBytes:
		return nil, &inputError{"snapshot", reasonTooLarge, ""}
	case len(policy) > maxPolicyBytes:
		return nil, &inputError{"policy", reasonTooLarge, ""}
	}
	in := &inputs{}
	if err := decodeStrict("plan", plan, maxPlanBytes, &in.plan); err != nil {
		return nil, err
	}
	if err := validatePlan(&in.plan); err != nil {
		return nil, err
	}
	if err := decodeStrict("snapshot", snapshot, maxSnapshotBytes, &in.snapshot); err != nil {
		return nil, err
	}
	if err := validateSnapshot(&in.snapshot); err != nil {
		return nil, err
	}
	if err := decodeStrict("policy", policy, maxPolicyBytes, &in.policy); err != nil {
		return nil, err
	}
	if err := validatePolicy(&in.policy); err != nil {
		return nil, err
	}
	return in, nil
}

func hash(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}
