// Package planproofshim re-exports the safe-ops approval controller's review
// functions and its policy gate, which live in internal packages, for
// planproof's comparison harness (PRD E-20). It is copied into an exported
// copy of safe-ops' approval-gate module (git archive of a commit) at
// bin/safeops/approval-gate/planproofshim by `make safeops-compare`; safe-ops'
// own checkout is never changed. It adds no logic.
package planproofshim

import (
	"github.com/billykong/safe-ops-agent-k8s/projects/02-approval-gate/internal/controller"
	"github.com/billykong/safe-ops-agent-k8s/projects/02-approval-gate/internal/gate"
)

var (
	Plan          = controller.Plan
	Observe       = controller.Observe
	BlastRadius   = controller.BlastRadius
	Findings      = controller.Findings
	Diff          = controller.Diff
	RollbackCheck = controller.RollbackCheck
	PolicyInput   = controller.PolicyInput
	CheckScope    = controller.CheckScope
	LoadGate      = gate.Load
	DiffRoots     = controller.DiffRoots
)

type Gate = gate.Gate
