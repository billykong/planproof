// Package kernel is planproof's trusted checker. Check takes a snapshot of
// the cluster, a plan and a policy, all as JSON, and returns a Verdict:
// Proved, Refuted or Unknown, with the facts behind it.
//
// The kernel is the part of planproof you have to trust, so it is kept small
// enough to read: it imports only the standard library packages in
// scripts/kernel-budget.sh, does no I/O, reads no clock, environment or
// randomness, and starts no goroutines. CI enforces all of that and a size
// budget.
//
// The normative definition is docs/spec/. Comments here cite it as
// "spec 02" and so on; requirement IDs (K-10) are from docs/prd/.
package kernel
