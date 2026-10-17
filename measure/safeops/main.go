// Command safeops-compare runs the same plans through planproof's kernel and
// through safe-ops-agent-k8s's approval controller review and policy gate,
// in both directions, and writes results/safeops-comparison.md (PRD E-20).
//
// safe-ops' side is its own code, unchanged: Plan, BlastRadius, Findings,
// Diff, RollbackCheck, PolicyInput and gate.rego, re-exported by
// planproofshim from a copy exported at a commit. Only the server-side dry
// run is simulated: the merge patch is applied locally and the dry run
// reported as accepted, so admission policies are outside this comparison.
//
// Run it with `make safeops-compare`.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	jsonpatch "github.com/evanphx/json-patch/v5"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/billykong/planproof/agent"
	"github.com/billykong/planproof/evals"
	"github.com/billykong/planproof/internal/yamljson"
	"github.com/billykong/planproof/kernel"
	v1 "github.com/billykong/safe-ops-agent-k8s/projects/02-approval-gate/api/v1alpha1"
	shim "github.com/billykong/safe-ops-agent-k8s/projects/02-approval-gate/planproofshim"
)

// proposedAt fixes the restart timestamp, so the output is the same on every run.
var proposedAt = time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)

func main() {
	root := flag.String("root", "../..", "planproof repository root")
	safeops := flag.String("safeops", "../../bin/safeops/repo", "the exported safe-ops-agent-k8s tree")
	commit := flag.String("commit", "", "the safe-ops commit the tree was exported from")
	out := flag.String("out", "", "write the report here instead of stdout")
	flag.Parse()
	report, err := run(*root, *safeops, *commit)
	if err != nil {
		fmt.Fprintln(os.Stderr, "safeops-compare:", err)
		os.Exit(1)
	}
	if *out == "" {
		fmt.Print(report)
		return
	}
	if err := os.WriteFile(*out, []byte(report), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "safeops-compare:", err)
		os.Exit(1)
	}
}

// --- safe-ops' side

// world is the cluster as safe-ops' controller sees it, built from a
// planproof snapshot and policy (translated in safe-ops' favour: see PRD E-20).
type world struct {
	deps map[string]*appsv1.Deployment
	nss  map[string]*corev1.Namespace
	pdbs map[string][]policyv1.PodDisruptionBudget
}

func newWorld(snap kernel.Snapshot, pol kernel.Policy) *world {
	w := &world{deps: map[string]*appsv1.Deployment{}, nss: map[string]*corev1.Namespace{}, pdbs: map[string][]policyv1.PodDisruptionBudget{}}
	ns := func(name string) *corev1.Namespace {
		if n, ok := w.nss[name]; ok {
			return n
		}
		n := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{}}}
		if slices.Contains(pol.ManagedNamespaces, name) {
			n.Labels["safe-ops.io/agent-managed"] = "true"
			if pol.Limits.MaxReplicas != nil {
				n.Labels["safe-ops.io/max-replicas"] = fmt.Sprint(*pol.Limits.MaxReplicas)
			}
		}
		w.nss[name] = n
		return n
	}
	for _, d := range snap.Deployments {
		key := d.Namespace + "/" + d.Name
		ns(d.Namespace)
		labels := map[string]string{}
		if slices.Contains(pol.ProtectedDeployments, key) {
			labels["safe-ops.io/protected"] = "true"
		}
		if slices.Contains(pol.CriticalDeployments, key) {
			labels["safe-ops.io/tier"] = "critical"
		}
		var containers []corev1.Container
		for _, c := range d.Containers {
			containers = append(containers, corev1.Container{Name: c.Name, Image: c.Image})
		}
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: d.Name, Namespace: d.Namespace, Labels: labels, Generation: 1},
			Spec: appsv1.DeploymentSpec{
				Replicas: ptr(int32(deref(d.Replicas))),
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": d.Name}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": d.Name}},
					Spec:       corev1.PodSpec{Containers: containers},
				},
				Strategy: strategy(d.Strategy),
			},
			Status: appsv1.DeploymentStatus{
				ObservedGeneration: 1, Replicas: int32(deref(d.Replicas)),
				ReadyReplicas: int32(deref(d.ReadyReplicas)), UpdatedReplicas: int32(deref(d.UpdatedReplicas)),
			},
		}
		w.deps[key] = dep
	}
	for _, p := range snap.PDBs {
		if p.Deployment == nil {
			continue
		}
		pdb := policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{Name: p.Name, Namespace: p.Namespace},
			Spec:       policyv1.PodDisruptionBudgetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": *p.Deployment}}},
		}
		if p.MinAvailable != nil {
			pdb.Spec.MinAvailable = ptr(intstr.FromInt32(int32(*p.MinAvailable)))
		}
		if p.MaxUnavailable != nil {
			pdb.Spec.MaxUnavailable = ptr(intstr.FromInt32(int32(*p.MaxUnavailable)))
		}
		w.pdbs[p.Namespace] = append(w.pdbs[p.Namespace], pdb)
	}
	return w
}

func strategy(s kernel.Strategy) appsv1.DeploymentStrategy {
	if s.Type == "Recreate" {
		return appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}
	}
	ru := &appsv1.RollingUpdateDeployment{}
	if s.MaxUnavailable != nil {
		ru.MaxUnavailable = ptr(intstr.FromInt32(int32(*s.MaxUnavailable)))
	}
	if s.MaxSurge != nil {
		ru.MaxSurge = ptr(intstr.FromInt32(int32(*s.MaxSurge)))
	}
	return appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType, RollingUpdate: ru}
}

// safeopsResult is the gate's answer for a whole plan.
type safeopsResult struct {
	Result  string   // Deny, Escalate, Allow, or Not expressible
	Reasons []string // the deciding reasons, one per ChangeRequest that wasn't allowed
	CRs     int      // ChangeRequests the plan became
}

var rank = map[string]int{"Allow": 0, "Escalate": 1, "Deny": 2}

// reviewPlan turns each scale and rollout-restart step into a ChangeRequest
// and has safe-ops review it against the state after the earlier ones have
// finished: one change at a time, as safe-ops runs.
func reviewPlan(ctx context.Context, g *shim.Gate, w *world, steps []kernel.Step, intent string) (safeopsResult, error) {
	var none []string
	for _, s := range steps {
		switch s.Op {
		case "scale", "rollout-restart", "wait":
		default:
			if !slices.Contains(none, s.Op) {
				none = append(none, s.Op)
			}
		}
	}
	if len(none) > 0 {
		return safeopsResult{Result: "Not expressible", Reasons: []string{"no ChangeRequest action for " + strings.Join(none, ", ")}}, nil
	}
	r := safeopsResult{Result: "Allow"}
	worse := func(result, reason string) {
		if rank[result] > rank[r.Result] {
			r.Result = result
		}
		if reason != "" {
			r.Reasons = append(r.Reasons, reason)
		}
	}
	for _, s := range steps {
		if s.Op == "wait" {
			continue
		}
		r.CRs++
		a := v1.Action{Type: v1.RestartDeployment}
		if s.Op == "scale" {
			a.Type = v1.ScaleDeployment
			if s.Replicas != nil {
				a.Replicas = ptr(int32(*s.Replicas))
			}
		}
		nsName, name, _ := strings.Cut(s.Deployment, "/")
		a.Target = v1.TargetRef{Kind: "Deployment", Namespace: nsName, Name: name}
		// The ChangeRequest schema (its CRD) bounds replicas to 0-50.
		if a.Replicas != nil && (*a.Replicas < 0 || *a.Replicas > 50) {
			worse("Deny", fmt.Sprintf("%s: rejected by the ChangeRequest schema (replicas 0-50)", s.Deployment))
			continue
		}
		cr := &v1.ChangeRequest{Spec: v1.ChangeRequestSpec{
			Intent: intent, Action: a, Scope: v1.Scope{Namespaces: []string{nsName}, Kinds: []string{"Deployment"}},
		}}
		if err := shim.CheckScope(cr.Spec); err != nil {
			worse("Deny", err.Error())
			continue
		}
		dep, ok := w.deps[s.Deployment]
		if !ok {
			worse("Deny", fmt.Sprintf("%s: target not found; the controller rejects it", s.Deployment))
			continue
		}
		patch, rollback, err := shim.Plan(a, shim.Observe(dep), proposedAt)
		if err != nil {
			return r, err
		}
		live, err := runtime.DefaultUnstructuredConverter.ToUnstructured(dep)
		if err != nil {
			return r, err
		}
		lj, err := json.Marshal(live)
		if err != nil {
			return r, err
		}
		aj, err := jsonpatch.MergePatch(lj, []byte(patch))
		if err != nil {
			return r, err
		}
		var after map[string]any
		if err := json.Unmarshal(aj, &after); err != nil {
			return r, err
		}
		pdbs := w.pdbs[nsName]
		rv := &v1.Review{DryRun: "accepted"} // simulated: see the package comment
		rv.Diff = shim.Diff(live, after, shim.DiffRoots...)
		rv.RollbackCheck = shim.RollbackCheck(live, after, rollback)
		rv.BlastRadius = shim.BlastRadius(dep, a, pdbs)
		rv.Findings = shim.Findings(dep, a, rv.BlastRadius, pdbs, rv.Diff, "")
		in, err := shim.PolicyInput(cr, dep, w.nss[nsName], rv)
		if err != nil {
			return r, err
		}
		d := g.Decide(ctx, in)
		reason := ""
		if d.Result != v1.PolicyAllow && len(d.Reasons) > 0 {
			reason = d.Reasons[0]
		}
		worse(string(d.Result), reason)
		// Applied and settled before the next ChangeRequest is proposed.
		var next appsv1.Deployment
		if err := json.Unmarshal(aj, &next); err != nil {
			return r, err
		}
		next.Generation = dep.Generation + 1
		n := *next.Spec.Replicas
		next.Status = appsv1.DeploymentStatus{ObservedGeneration: next.Generation, Replicas: n, ReadyReplicas: n, UpdatedReplicas: n}
		w.deps[s.Deployment] = &next
	}
	return r, nil
}

// --- planproof's side

type plan struct {
	Rationale string          `json:"rationale"`
	Goal      []kernel.Change `json:"goal"`
	Steps     []kernel.Step   `json:"steps"`
}

type input struct {
	snapshot, policy []byte
	snap             kernel.Snapshot
	pol              kernel.Policy
}

func readInput(root, snapPath, polPath string) (input, error) {
	var in input
	var err error
	if in.snapshot, err = os.ReadFile(filepath.Join(root, snapPath)); err != nil {
		return in, err
	}
	b, err := os.ReadFile(filepath.Join(root, polPath))
	if err != nil {
		return in, err
	}
	if in.policy, err = yamljson.Convert(b); err != nil {
		return in, err
	}
	if err := json.Unmarshal(in.snapshot, &in.snap); err != nil {
		return in, err
	}
	return in, json.Unmarshal(in.policy, &in.pol)
}

// planproofResult is the kernel's verdict, with the failing rules or the
// Unknown trigger, and whether a person must approve.
func check(in input, id, ticket string, body json.RawMessage) (string, bool) {
	t := agent.Task{ID: id, Ticket: ticket, Snapshot: in.snapshot, Policy: in.policy}
	v := kernel.Check(in.snapshot, agent.Plan(t, body), in.policy)
	var why []string
	for _, f := range v.Facts {
		if f.Type == "failure" && !slices.Contains(why, f.Rule) {
			why = append(why, f.Rule)
		}
		if f.Type == "unknown" {
			why = append(why, f.Trigger)
		}
	}
	s := string(v.Result)
	if len(why) > 0 {
		s += " (" + strings.Join(why, ", ") + ")"
	}
	return s, v.ApprovalRequired
}

// --- safe-ops' seeded corpus

type corpus struct {
	Proposals []struct {
		Name     string `json:"name"`
		Expected string `json:"expected"`
		Reason   string `json:"reason"`
		Rules    string `json:"rules"`
		Spec     struct {
			Intent string    `json:"intent"`
			Action v1.Action `json:"action"`
		} `json:"spec"`
	} `json:"proposals"`
}

// claims are the goals the intents state, where they differ from the action.
type claims map[string][]kernel.Change

func readYAML(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	j, err := yamljson.Convert(b)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return json.Unmarshal(j, v)
}

// asPlan is the plan an agent would write for a ChangeRequest's action: the
// step, a wait (planproof's settled rule needs one), and the goal the action
// achieves. The intent goes in the rationale, which the packet shows as the
// agent's unchecked words.
func asPlan(snap kernel.Snapshot, a v1.Action, intent string, goal []kernel.Change) (json.RawMessage, error) {
	key := a.Target.Namespace + "/" + a.Target.Name
	var cur int64
	for _, d := range snap.Deployments {
		if d.Namespace+"/"+d.Name == key {
			cur = deref(d.Replicas)
		}
	}
	step := map[string]any{"op": "rollout-restart", "deployment": key}
	honest := []any{map[string]any{"deployment": key, "field": "restarted", "from": false, "to": true}}
	if a.Type == v1.ScaleDeployment {
		step = map[string]any{"op": "scale", "deployment": key, "replicas": *a.Replicas}
		honest = []any{map[string]any{"deployment": key, "field": "replicas", "from": cur, "to": *a.Replicas}}
	}
	p := map[string]any{
		"rationale": intent,
		"goal":      honest,
		"steps":     []any{step, map[string]any{"op": "wait", "deployments": []string{key}, "timeoutSeconds": 300}},
	}
	if goal != nil {
		p["goal"] = goal
	}
	return json.Marshal(p)
}

// --- the report

func run(root, safeops, commit string) (string, error) {
	ctx := context.Background()
	g, err := shim.LoadGate(ctx, filepath.Join(safeops, "projects/02-approval-gate/policy/gate.rego"))
	if err != nil {
		return "", err
	}
	specs, err := evals.Load(root)
	if err != nil {
		return "", err
	}
	byID := map[string]evals.Spec{}
	for _, s := range specs {
		byID[s.ID] = s
	}
	seeded, err := evals.LoadSeeded(root)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	p := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	p("# planproof and the safe-ops gate, side by side\n\n")
	p("Generated by `make safeops-compare` (PRD E-20); don't edit by hand. Kernel %s; safe-ops-agent-k8s commit `%s`, gate policy `%s`.\n\n", kernel.KernelVersion, commit, short(g.SHA))
	p("safe-ops' side is its own controller code and `gate.rego`, unchanged; only its server-side dry run is simulated (the patch applied locally, the dry run reported as accepted). Each `scale` and `rollout-restart` step is one ChangeRequest, reviewed after the earlier ones have finished; waits are dropped; a plan with any other operation is **not expressible** as ChangeRequests. Outcomes: safe-ops **Deny** (no approval can override), **Escalate** (a person must approve), **Allow**; planproof **Refuted** or **Unknown** (cannot be approved), **Proved** (a person checks the goal; *approval* marks the plans the policy sends to a person). See the [PRD](../docs/prd/areas/e-agent-and-evals.md#head-to-head-with-the-safe-ops-gate-e-20) for how the policy was translated.\n\n")

	count := map[string]int{}

	// 1. planproof's seeded bad plans.
	p("## 1. planproof's seeded bad plans (%d)\n\n", len(seeded))
	p("| Seeded plan | Should be caught by | planproof | safe-ops | safe-ops' reason |\n| --- | --- | --- | --- | --- |\n")
	for _, s := range seeded {
		spec := byID[s.Task]
		in, err := readInput(root, spec.Snapshot, spec.Policy)
		if err != nil {
			return "", err
		}
		var pl plan
		if err := json.Unmarshal(s.Plan, &pl); err != nil {
			return "", fmt.Errorf("%s: %w", s.ID, err)
		}
		pp, approval := check(in, s.ID, spec.Ticket, s.Plan)
		if strings.HasPrefix(pp, "Proved") {
			if evals.SameGoal(s.Plan, spec.Expect.Goal) {
				pp += ", goal matches the ticket"
			} else {
				pp += ", **goal isn't the ticket's**: the reviewer must catch it"
			}
			if approval {
				pp += "; approval"
			}
		}
		so, err := reviewPlan(ctx, g, newWorld(in.snap, in.pol), pl.Steps, pl.Rationale)
		if err != nil {
			return "", fmt.Errorf("%s: %w", s.ID, err)
		}
		count["seeded/planproof/"+first(pp)]++
		count["seeded/safeops/"+so.Result]++
		p("| %s | %s | %s | %s | %s |\n", s.ID, s.Want(), pp, so.Result, cell(so.Reasons))
	}
	p("\n")

	// 2. Good plans: the reference plans of the solvable tasks.
	p("## 2. Good plans: the reference plans of the solvable tasks\n\n")
	p("| Task | planproof | safe-ops | safe-ops' reason |\n| --- | --- | --- | --- |\n")
	for _, s := range specs {
		if s.Reference == nil {
			continue
		}
		in, err := readInput(root, s.Snapshot, s.Policy)
		if err != nil {
			return "", err
		}
		var pl plan
		if err := json.Unmarshal(s.Reference, &pl); err != nil {
			return "", err
		}
		pp, approval := check(in, s.ID, s.Ticket, s.Reference)
		if approval {
			pp += "; approval"
		}
		so, err := reviewPlan(ctx, g, newWorld(in.snap, in.pol), pl.Steps, pl.Rationale)
		if err != nil {
			return "", err
		}
		count["good/planproof/"+first(pp)]++
		count["good/safeops/"+so.Result]++
		p("| %s | %s | %s | %s |\n", s.ID, pp, so.Result, cell(so.Reasons))
	}
	p("\n")

	// 3. safe-ops' seeded corpus through planproof.
	var c corpus
	if err := readYAML(filepath.Join(safeops, "projects/04-observability/evals/seeded/proposals.yaml"), &c); err != nil {
		return "", err
	}
	var cl claims
	if err := readYAML("testdata/safeops-claims.yaml", &cl); err != nil {
		return "", err
	}
	strict, err := readInput(".", "testdata/safeops-snapshot.json", "testdata/safeops-policy.yaml")
	if err != nil {
		return "", err
	}
	zero, err := readInput(".", "testdata/safeops-snapshot.json", "testdata/safeops-policy-zero.yaml")
	if err != nil {
		return "", err
	}
	p("## 3. safe-ops' seeded proposals (%d) through planproof\n\n", len(c.Proposals))
	p("From safe-ops' `projects/04-observability/evals/seeded/proposals.yaml`, on its demo namespace written as a planproof snapshot ([testdata](../measure/safeops/testdata/README.md)). The intent becomes the ticket and the plan's rationale. *safe-ops (here)* is this harness's run of safe-ops' code; *safe-ops' own label* is what its suite expects, so the two columns agreeing checks the harness. planproof runs with scale to zero forbidden and, where it differs, allowed; *claimed goal* is the plan with the goal the intent states instead of the one the action achieves ([claims](../measure/safeops/testdata/safeops-claims.yaml)).\n\n")
	p("| Proposal | Should be | safe-ops' own label | safe-ops (here) | planproof | planproof, zero allowed | planproof, claimed goal |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, e := range c.Proposals {
		body, err := asPlan(strict.snap, e.Spec.Action, e.Spec.Intent, nil)
		if err != nil {
			return "", err
		}
		var pl plan
		if err := json.Unmarshal(body, &pl); err != nil {
			return "", err
		}
		so, err := reviewPlan(ctx, g, newWorld(strict.snap, strict.pol), pl.Steps, e.Spec.Intent)
		if err != nil {
			return "", err
		}
		withApproval := func(in input, body json.RawMessage) string {
			s, a := check(in, e.Name, e.Spec.Intent, body)
			if a {
				s += "; approval"
			}
			return s
		}
		pp := withApproval(strict, body)
		pz := withApproval(zero, body)
		if pz == pp {
			pz = "same"
		}
		claimed := "-"
		if goal, ok := cl[e.Name]; ok {
			cb, err := asPlan(strict.snap, e.Spec.Action, e.Spec.Intent, goal)
			if err != nil {
				return "", err
			}
			claimed = withApproval(strict, cb)
		}
		if got, want := so.Result, e.Rules; !agrees(got, want) {
			return "", fmt.Errorf("%s: this harness's safe-ops review gives %s, but safe-ops' suite expects %s", e.Name, got, want)
		}
		p("| %s | %s | %s | %s | %s | %s | %s |\n", e.Name, e.Expected, e.Rules, so.Result+reasonSuffix(so), pp, pz, claimed)
	}
	p("\n")

	p("## Counts\n\n| Plans | planproof | safe-ops |\n| --- | --- | --- |\n")
	p("| %d seeded bad plans | %s | %s |\n", len(seeded), tally(count, "seeded/planproof/"), tally(count, "seeded/safeops/"))
	p("| good plans | %s | %s |\n", tally(count, "good/planproof/"), tally(count, "good/safeops/"))
	return b.String(), nil
}

// agrees checks this harness's safe-ops result against the label in
// safe-ops' corpus: Caught and FalseAlarm mean escalate or deny; Missed and
// Passed mean allow.
func agrees(result, label string) bool {
	switch label {
	case "Caught", "FalseAlarm":
		return result == "Escalate" || result == "Deny"
	case "Missed", "Passed":
		return result == "Allow"
	}
	return false
}

func reasonSuffix(r safeopsResult) string {
	if len(r.Reasons) == 0 {
		return ""
	}
	return ": " + cell(r.Reasons)
}

func tally(count map[string]int, prefix string) string {
	var parts []string
	for _, k := range []string{"Refuted", "Unknown", "Proved", "Deny", "Escalate", "Allow", "Not expressible"} {
		if n := count[prefix+k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", k, n))
		}
	}
	return strings.Join(parts, ", ")
}

// first is a result's first word: "Proved; approval" is Proved.
func first(s string) string {
	if i := strings.IndexAny(s, " ,;"); i >= 0 {
		return s[:i]
	}
	return s
}

func cell(reasons []string) string {
	s := strings.Join(reasons, "; ")
	return strings.ReplaceAll(s, "|", "\\|")
}

func short(sha string) string {
	if len(sha) > 19 {
		return sha[:19]
	}
	return sha
}

func ptr[T any](v T) *T { return &v }

func deref(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
