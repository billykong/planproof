package kernel

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"slices"
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
	v.PlanName = in.plan.Metadata.Name
	r := newRun(in)
	stage, at, restored := r.execute()
	switch {
	case r.unknown != nil:
		v.Result = Unknown
	case len(r.failures) > 0:
		v.Result = Refuted
	default:
		v.Result = Proved
	}
	v.Facts = r.facts(stage, at, restored)
	if v.Result == Proved {
		reasons := r.approvalReasons()
		v.ApprovalRequired = len(reasons) > 0
		approval := Fact{Type: "approval", Required: &v.ApprovalRequired, Reasons: reasons}
		if critical := r.criticalInvolved(); slices.Contains(reasons, "critical") {
			approval.Evidence = map[string]any{"critical": critical}
		}
		v.Facts = append(v.Facts, approval)
	}
	for _, t := range []struct {
		field string
		text  *string
	}{{"ticket", in.plan.Metadata.Ticket}, {"rationale", in.plan.Rationale}} {
		if t.text != nil {
			v.Facts = append(v.Facts, Fact{Type: "unchecked-text", Field: t.field, Text: *t.text})
		}
	}
	return v
}

// Stages a check can stop at.
const (
	stageStatic   = "static"
	stageStep     = "step"
	stageEnd      = "end"
	stageRollback = "rollback"
	stageDone     = "done"
)

// execute runs the algorithm of spec 05, steps 3-6, and says where it
// stopped. restored counts the goal fields the rollback restores.
func (r *run) execute() (stage string, at StepRef, restored int) {
	if r.static(); len(r.failures) > 0 {
		return stageStatic, StepRef{}, 0
	}
	for i, s := range r.in.plan.Steps {
		if ref := (StepRef{Index: i + 1}); !r.do(ref, s) {
			return stageStep, ref, 0
		}
	}
	if r.end(); len(r.failures) > 0 {
		return stageEnd, StepRef{End: true}, 0
	}
	if len(r.in.plan.Rollback) == 0 {
		return stageDone, StepRef{}, 0
	}
	r.rollback = true
	for i, s := range r.in.plan.Rollback {
		if ref := (StepRef{Index: i + 1, Rollback: true}); !r.do(ref, s) {
			return stageRollback, ref, 0
		}
	}
	r.rollback = false
	if n := r.restored(); len(r.failures) == 0 {
		return stageDone, StepRef{}, n
	}
	return stageRollback, StepRef{End: true}, 0
}

// eachStateRules are checked at every state; when the check stops at a
// step, those that haven't failed are incomplete.
var eachStateRules = []string{"limits", "assertions", "cordon-first", "no-orphans", "budget", "never-zero", "capacity"}

// facts lists everything found, in the order of spec 05.
func (r *run) facts(stage string, at StepRef, restored int) []Fact {
	plan := &r.in.plan
	var facts []Fact
	for _, c := range plan.Goal {
		f := changeField(c)
		facts = append(facts, Fact{Type: "goal-change", Object: f.object, Field: c.Field, Container: c.Container, From: &c.From, To: &c.To})
	}
	for i := range plan.Steps {
		facts = append(facts, Fact{Type: "step", Index: i + 1, Params: &plan.Steps[i]})
	}
	for i := range plan.Rollback {
		facts = append(facts, Fact{Type: "rollback-step", Index: i + 1, Params: &plan.Rollback[i]})
	}
	for _, c := range plan.Goal {
		if len(plan.Rollback) > 0 && (c.Field == "drained" || c.Field == "restarted") {
			facts = append(facts, Fact{Type: "not-reversible", Object: changeField(c).object, Field: c.Field})
		}
	}

	failed := map[string]bool{}
	for _, f := range r.failures {
		failed[f.Rule] = true
	}
	for _, c := range checkFacts(nil) {
		c.Outcome = r.outcome(c.Rule, stage, failed)
		if c.Outcome == outcomeIncomplete {
			c.Step = &at
		}
		if c.Outcome == outcomePass || c.Outcome == outcomeFail {
			c.Evidence = r.evidence(c.Rule, restored)
		}
		facts = append(facts, c)
	}

	slices.SortStableFunc(r.failures, func(a, b Fact) int {
		return cmp.Or(cmp.Compare(slices.Index(Rules, a.Rule), slices.Index(Rules, b.Rule)), cmp.Compare(a.Object, b.Object))
	})
	facts = append(facts, r.failures...)
	if r.unknown != nil {
		facts = append(facts, *r.unknown)
	}
	for _, id := range []string{"A1", "A2", "A3", "A4", "A5"} {
		if id == "A2" || id == "A3" {
			if len(r.creators) == 0 {
				continue
			}
			facts = append(facts, Fact{Type: "assumption", ID: id, Steps: r.creators})
		} else {
			facts = append(facts, Fact{Type: "assumption", ID: id})
		}
	}
	return facts
}

func (r *run) outcome(rule, stage string, failed map[string]bool) string {
	switch {
	case failed[rule]:
		return outcomeFail
	case rule == "input" || rule == "scope" || rule == "protected":
		return outcomePass
	case rule == "rollback" && len(r.in.plan.Rollback) == 0:
		return outcomeNotApplicable
	case rule == "rollback" && stage == stageRollback:
		return outcomeIncomplete
	case rule == "rollback":
		return map[bool]string{true: outcomePass, false: outcomeNotReached}[stage == stageDone]
	case slices.Contains(eachStateRules, rule) && stage == stageStatic:
		return outcomeNotReached
	case slices.Contains(eachStateRules, rule) && stage == stageStep:
		return outcomeIncomplete
	case slices.Contains(eachStateRules, rule):
		return outcomePass
	case stage == stageStatic || stage == stageStep || rule == "frame" && failed["settled"]:
		return outcomeNotReached
	}
	return outcomePass
}

// evidence holds the numbers behind a check's outcome (spec 05).
func (r *run) evidence(rule string, restored int) map[string]any {
	lim := r.in.policy.Limits
	switch rule {
	case "limits":
		if r.maxUnsched == nil {
			return nil
		}
		return map[string]any{
			"maxUnschedulable": r.maxUnsched.val, "maxUnschedulableStep": r.maxUnsched.step, "limitUnschedulable": *lim.MaxUnschedulableNodes,
			"minSchedulable": r.minSched.val, "minSchedulableStep": r.minSched.step, "limitSchedulable": *lim.MinSchedulableNodes,
		}
	case "assertions":
		return map[string]any{"count": r.asserts}
	case "budget":
		var items []any
		for _, b := range r.st.pdbs {
			m := r.low[b.name]
			item := map[string]any{"deployment": b.dep, "lowestAvailable": m.val, "needs": m.need, "phase": "during", "step": nil}
			if m.step > 0 {
				item["step"] = m.step
			}
			items = append(items, item)
		}
		return map[string]any{"items": items}
	case "never-zero":
		var items []any
		for _, f := range r.failures {
			if f.Rule == "never-zero" {
				items = append(items, map[string]any{"deployment": f.Evidence["deployment"], "lowestAvailable": 0, "step": f.Step, "phase": f.Phase})
			}
		}
		if items == nil {
			return nil
		}
		return map[string]any{"items": items}
	case "capacity":
		if t := r.tight; t != nil {
			return map[string]any{"node": t.node, "cpuFreeMilli": t.cpu, "memoryFreeMiB": t.mem, "step": t.step}
		}
	case "frame":
		if !slices.ContainsFunc(r.failures, func(f Fact) bool { return f.Rule == "frame" }) {
			return map[string]any{"changes": len(r.in.plan.Goal)}
		}
	case "rollback":
		if len(r.failures) == 0 && r.unknown == nil {
			return map[string]any{"restored": restored}
		}
	}
	return nil
}

// approvalReasons lists the policy's approval conditions this plan meets,
// in the policy's order (spec 04).
func (r *run) approvalReasons() []string {
	pol := &r.in.policy
	met := map[string]bool{"always": true, "drain": r.drains, "critical": len(r.criticalInvolved()) > 0, "scale-down": r.scaleDown, "image": r.setsImage}
	reasons := []string{}
	for _, reason := range pol.ApprovalRequiredFor {
		if met[reason] {
			reasons = append(reasons, reason)
		}
	}
	return reasons
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

// criticalInvolved lists the critical deployments the plan names, or whose
// pods it moves, replaces or removes.
func (r *run) criticalInvolved() []string {
	named := map[string]bool{}
	for _, c := range r.in.plan.Goal {
		named[c.Deployment] = true
	}
	for _, s := range r.in.plan.Steps {
		named[s.Deployment] = true
	}
	var out []string
	for _, d := range r.in.policy.CriticalDeployments {
		if named[d] || r.disrupted[d] {
			out = append(out, d)
		}
	}
	return out
}
