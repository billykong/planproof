package kernel

import (
	"maps"
	"slices"
	"strings"
)

// static checks the rules that need no simulation: scope, protected, the
// static part of limits, and the static part of frame (spec 04).
func (r *run) static() {
	plan, pol, st, ref := &r.in.plan, &r.in.policy, r.st, StepRef{}
	steps := append(slices.Clone(plan.Steps), plan.Rollback...)
	var deps, nodes []string
	for _, c := range plan.Goal {
		if c.Deployment != "" {
			deps = append(deps, c.Deployment)
		} else {
			nodes = append(nodes, c.Node)
		}
	}
	for _, s := range steps {
		deps = append(append(deps, s.Deployments...), s.Deployment)
		nodes = append(nodes, s.Node)
	}
	slices.Sort(deps)
	slices.Sort(nodes)
	for _, d := range slices.Compact(deps) {
		ns, _, _ := strings.Cut(d, "/")
		switch {
		case d == "":
		case !slices.Contains(pol.ManagedNamespaces, ns):
			r.fail("scope", ref, "static", "deployment/"+d, map[string]any{"reason": "not-managed"})
		case st.deps[d] == nil:
			r.fail("scope", ref, "static", "deployment/"+d, map[string]any{"reason": "not-found"})
		}
		if slices.Contains(pol.ProtectedDeployments, d) {
			r.fail("protected", ref, "static", "deployment/"+d, nil)
		}
	}
	for _, n := range slices.Compact(nodes) {
		switch {
		case n == "":
		case !slices.Contains(pol.ManagedNodes, n):
			r.fail("scope", ref, "static", "node/"+n, map[string]any{"reason": "not-managed"})
		case st.nodes[n] == nil:
			r.fail("scope", ref, "static", "node/"+n, map[string]any{"reason": "not-found"})
		}
	}
	if len(r.failures) > 0 {
		return // the rest needs the objects to exist
	}

	lim := pol.Limits
	limit := func(field string, value any, limit int64) {
		r.fail("limits", ref, "static", "", map[string]any{"field": field, "value": value, "limit": limit})
	}
	noDigest := func(image string) bool { return *lim.RequireImageDigest && !strings.Contains(image, "@sha256:") }
	for _, s := range steps {
		switch {
		case s.Op == "scale" && *s.Replicas > *lim.MaxReplicas:
			limit("replicas", *s.Replicas, *lim.MaxReplicas)
		case s.Op == "wait" && *s.TimeoutSeconds > *lim.MaxWaitSeconds:
			limit("timeoutSeconds", *s.TimeoutSeconds, *lim.MaxWaitSeconds)
		case s.Op == "set-image" && noDigest(s.Image):
			r.fail("limits", ref, "static", "deployment/"+s.Deployment, map[string]any{"field": "image", "value": s.Image})
		}
		if s.Op == "set-image" {
			if _, ok := st.deps[s.Deployment].images[s.Container]; !ok {
				r.fail("scope", ref, "static", "deployment/"+s.Deployment, map[string]any{"reason": "not-found", "container": s.Container})
			}
		}
	}

	seen := map[string]bool{}
	for _, c := range plan.Goal {
		f := changeField(c)
		cur, ok := r.initDesired[f.key()]
		switch {
		case !ok:
			r.fail("scope", ref, "static", f.object, map[string]any{"reason": "not-found", "container": c.Container})
		case seen[f.key()]:
			r.fail("frame", ref, "static", f.object, map[string]any{"reason": "repeated", "field": c.Field})
		case cur.val != c.From:
			r.fail("frame", ref, "static", f.object, map[string]any{"reason": "from-mismatch", "field": c.Field, "from": c.From, "actual": cur.val})
		case c.From == c.To:
			r.fail("frame", ref, "static", f.object, map[string]any{"reason": "no-change", "field": c.Field})
		case c.Field == "replicas" && c.To.Int > *lim.MaxReplicas:
			limit("replicas", c.To.Int, *lim.MaxReplicas)
		case c.Field == "image" && noDigest(c.To.Str):
			r.fail("limits", ref, "static", f.object, map[string]any{"field": "image", "value": c.To.Str})
		}
		seen[f.key()] = true
	}
}

// end checks settled, then frame: the plan's effect is exactly its goal.
func (r *run) end() {
	ref := StepRef{End: true}
	for _, dep := range r.depNames {
		if r.st.deps[dep].rolling || slices.ContainsFunc(r.st.podsOf(dep), func(p *mpod) bool { return p.created && !p.ready }) {
			r.fail("settled", ref, "end", "deployment/"+dep, map[string]any{"deployment": dep})
		}
	}
	if len(r.failures) > 0 {
		return
	}
	final, goal := r.st.desired(), map[string]field{}
	for _, c := range r.in.plan.Goal {
		f := changeField(c)
		goal[f.key()] = f
	}
	extra, missing := []any{}, []any{}
	for _, k := range slices.Sorted(maps.Keys(final)) {
		if g, inGoal := goal[k]; inGoal && final[k].val != g.val {
			missing = append(missing, g.record(r.initDesired[k].val))
		} else if !inGoal && final[k].val != r.initDesired[k].val {
			extra = append(extra, final[k].record(r.initDesired[k].val))
		}
	}
	if len(extra)+len(missing) > 0 {
		r.fail("frame", ref, "end", "", map[string]any{"extra": extra, "missing": missing})
	}
}

// restored checks, after the rollback, that each reversible goal field is
// back to its from value (spec 01).
func (r *run) restored() int {
	final, n := r.st.desired(), 0
	var notRestored []any
	for _, c := range r.in.plan.Goal {
		if c.Field == "drained" || c.Field == "restarted" {
			continue
		}
		f := changeField(c)
		if final[f.key()].val == c.From {
			n++
		} else {
			notRestored = append(notRestored, final[f.key()].record(c.From))
		}
	}
	if notRestored != nil {
		r.fail("rollback", StepRef{End: true}, "end", "", map[string]any{"notRestored": notRestored})
	}
	return n
}
