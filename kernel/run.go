package kernel

import (
	"cmp"
	"maps"
	"slices"
)

// run is one check in progress: the state, what has failed, and the
// trackers behind the check facts' evidence.
type run struct {
	in                           *inputs
	st                           *state
	depNames                     []string         // sorted, for deterministic iteration
	initial                      map[string]int64 // available pods per deployment in the snapshot
	initDesired                  map[string]field
	initUnsched, initSched       int
	failures                     []Fact
	unknown                      *Fact
	rollback                     bool // running the rollback: failures are relabelled, trackers paused
	creators                     []int
	asserts                      int
	low                          map[string]*mark
	maxUnsched, minSched         *mark
	tight                        *tightMark
	disrupted                    map[string]bool
	drains, scaleDown, setsImage bool
}

// mark is the extreme value of a number over the plan's states, and the
// first step that reached it (0: the snapshot).
type mark struct {
	val, need int64
	step      int
}

type tightMark struct {
	node     string
	cpu, mem int64
	step     int
}

func newRun(in *inputs) *run {
	st := build(&in.snapshot)
	r := &run{in: in, st: st, initial: map[string]int64{}, low: map[string]*mark{}, disrupted: map[string]bool{}}
	r.depNames = slices.Sorted(maps.Keys(st.deps))
	for _, d := range r.depNames {
		r.initial[d] = st.avail(d)
	}
	for _, b := range st.pdbs {
		r.low[b.name] = &mark{val: r.initial[b.dep], need: st.need(b)}
	}
	r.initUnsched, r.initSched = st.nodeCounts()
	r.initDesired = st.desired()
	return r
}

func (st *state) need(b mpdb) int64 {
	if b.min != nil {
		return *b.min
	}
	return max(0, st.deps[b.dep].replicas-*b.max)
}

func (st *state) nodeCounts() (unsched, sched int) {
	for _, n := range st.nodes {
		if n.unsched {
			unsched++
		} else if n.ready {
			sched++
		}
	}
	return
}

// fail records a broken rule. During the rollback it is recorded as a
// failure of rule rollback, carrying the inner rule (spec 05).
func (r *run) fail(rule string, ref StepRef, phase, object string, ev map[string]any) {
	if r.rollback {
		ev = maps.Clone(ev)
		if ev == nil {
			ev = map[string]any{}
		}
		ev["innerRule"], rule = rule, "rollback"
	}
	r.failures = append(r.failures, Fact{Type: "failure", Rule: rule, Step: &ref, Phase: phase, Object: object, Evidence: ev})
}

// do runs one step (spec 05, step 4) and reports whether to go on.
func (r *run) do(ref StepRef, s Step) bool {
	if t, obj := r.trigger(s); t != "" {
		r.unknown = &Fact{Type: "unknown", Trigger: t, Step: &ref, Object: obj}
		return false
	}
	r.assert(ref, s, s.Pre, "pre")
	if s.Op == "drain" {
		r.drainRequires(ref, s.Node)
	}
	if len(r.failures) > 0 {
		return false
	}
	if r.apply(ref, s) && !r.rollback {
		r.creators = append(r.creators, ref.Index)
	}
	r.eachState(ref)
	r.assert(ref, s, s.Post, "after")
	return len(r.failures) == 0
}

// trigger returns the first Unknown trigger that applies to a step, and the
// object it concerns (spec 02, spec 03).
func (r *run) trigger(s Step) (string, string) {
	st := r.st
	switch s.Op {
	case "cordon", "uncordon", "drain":
		if !st.nodes[s.Node].ready {
			return "node-not-ready", "node/" + s.Node
		}
	}
	switch s.Op {
	case "drain":
		for _, p := range st.pods {
			if !p.mightBeOn(s.Node) || p.owner == "DaemonSet" || p.owner == "None" {
				continue
			}
			switch {
			case p.local:
				return "local-storage", "pod/" + p.name
			case p.dep == "" || st.deps[p.dep] == nil:
				return "stateful-owner", "pod/" + p.name
			case r.depTrigger(p.dep) != "":
				return r.depTrigger(p.dep), "deployment/" + p.dep
			case p.placement:
				return "placement-rules", "deployment/" + p.dep
			}
		}
	case "scale", "set-image", "rollout-restart":
		if t := r.depTrigger(s.Deployment); t != "" {
			return t, "deployment/" + s.Deployment
		}
		d := st.deps[s.Deployment]
		if s.Op == "scale" && *s.Replicas <= d.replicas {
			break
		}
		if d.placement {
			return "placement-rules", "deployment/" + s.Deployment
		}
	}
	return "", ""
}

func (r *run) depTrigger(dep string) string {
	d := r.st.deps[dep]
	switch {
	case d.hpa:
		return "hpa"
	case d.unresolved:
		return "pdb-unresolved"
	case d.unsettled:
		return "unsettled-start"
	case d.rolling:
		return "overlapping-change"
	}
	return ""
}

func (r *run) drainRequires(ref StepRef, node string) {
	if !r.st.nodes[node].unsched {
		r.fail("cordon-first", ref, "pre", "node/"+node, map[string]any{"node": node})
	}
	for _, p := range r.st.pods {
		if p.owner == "None" && p.mightBeOn(node) {
			r.fail("no-orphans", ref, "pre", "pod/"+p.name, map[string]any{"pod": p.name})
		}
	}
}

// apply changes the state as the step's operation does (spec 03) and
// reports whether it created pods.
func (r *run) apply(ref StepRef, s Step) bool {
	st, plan := r.st, !r.rollback
	switch s.Op {
	case "cordon", "uncordon":
		st.nodes[s.Node].unsched = s.Op == "cordon"
	case "drain":
		var moved []*mpod
		sched := st.schedulable()
		for _, p := range st.pods {
			if p.dep != "" && p.mightBeOn(s.Node) {
				// It was on this node and moves to a schedulable one, or it was on
				// one of its other candidates and stays there (kernel 0.3.0).
				nodes := append(slices.Clone(sched), slices.DeleteFunc(slices.Clone(p.nodes), func(n string) bool { return n == s.Node })...)
				slices.Sort(nodes)
				p.ready, p.created, p.nodes = false, true, slices.Compact(nodes)
				moved = append(moved, p)
				r.disrupted[p.dep] = r.disrupted[p.dep] || plan
			}
		}
		st.nodes[s.Node].drained = true
		r.drains = r.drains || plan
		r.placeOrFail(ref, moved)
		return len(moved) > 0
	case "scale":
		d, n := st.deps[s.Deployment], *s.Replicas
		old := d.replicas
		d.replicas = n
		if n > old {
			r.placeOrFail(ref, st.newPods(s.Deployment, n-old))
			return true
		}
		if n < old {
			r.scaleDown = r.scaleDown || plan
			r.disrupted[s.Deployment] = r.disrupted[s.Deployment] || plan
			// Kubernetes removes pods that aren't ready first; the model doesn't
			// know which ready ones go, so the rest may be anywhere they were.
			pods := st.podsOf(s.Deployment)
			slices.SortStableFunc(pods, func(a, b *mpod) int { return cmp.Compare(boolInt(a.ready), boolInt(b.ready)) })
			var where []string
			for _, p := range pods {
				where = append(where, p.nodes...)
			}
			slices.Sort(where)
			where = slices.Compact(where)
			for i, p := range pods {
				if int64(i) < int64(len(pods))-n {
					p.gone, p.ready = true, false
				} else {
					p.nodes = where
				}
			}
		}
	case "set-image", "rollout-restart":
		d := st.deps[s.Deployment]
		r.disrupted[s.Deployment] = r.disrupted[s.Deployment] || plan
		r.setsImage = r.setsImage || plan && s.Op == "set-image"
		d.floor = 0
		if !d.recreate {
			d.floor = max(0, min(st.avail(s.Deployment), d.replicas-d.maxUnavail))
			surge := st.newPods(s.Deployment, d.maxSurge)
			bad := st.place(surge)
			st.remove(surge)
			if bad != nil {
				r.fail("capacity", ref, "during", "deployment/"+s.Deployment, map[string]any{"deployment": s.Deployment})
				return true
			}
		}
		st.remove(st.podsOf(s.Deployment))
		d.rolling = true
		if s.Op == "set-image" {
			d.images[s.Container] = s.Image
		} else {
			d.restarted = true
		}
		r.placeOrFail(ref, st.newPods(s.Deployment, d.replicas))
		return true
	case "wait":
		for _, dep := range s.Deployments {
			st.deps[dep].rolling = false
			for _, p := range st.podsOf(dep) {
				p.ready = true
			}
		}
	}
	return false
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (r *run) placeOrFail(ref StepRef, pods []*mpod) {
	if bad := r.st.place(pods); bad != nil {
		r.fail("capacity", ref, "after", "deployment/"+bad.dep, map[string]any{"deployment": bad.dep})
	}
}

// eachState checks the rules that hold at every state: budget, never-zero
// and limits (spec 04). Capacity is checked where pods are placed. In this
// model a step's During and After states have the same availability, so
// availability is checked once and reported as During.
func (r *run) eachState(ref StepRef) {
	st, lim, plan := r.st, r.in.policy.Limits, !r.rollback
	for _, b := range st.pdbs {
		a, need := st.avail(b.dep), st.need(b)
		if m := r.low[b.name]; plan && a < m.val {
			*m = mark{a, need, ref.Index}
		}
		if a < need && a < r.initial[b.dep] {
			r.fail("budget", ref, "during", "deployment/"+b.dep, map[string]any{"deployment": b.dep, "lowestAvailable": a, "needs": need})
		}
	}
	for _, dep := range r.depNames {
		if r.initial[dep] >= 1 && st.avail(dep) == 0 && !r.zeroAllowed(dep) {
			r.fail("never-zero", ref, "during", "deployment/"+dep, map[string]any{"deployment": dep, "lowestAvailable": 0})
		}
	}
	unsched, sched := st.nodeCounts()
	if plan {
		if r.maxUnsched == nil || int64(unsched) > r.maxUnsched.val {
			r.maxUnsched = &mark{val: int64(unsched), step: ref.Index}
		}
		if r.minSched == nil || int64(sched) < r.minSched.val {
			r.minSched = &mark{val: int64(sched), step: ref.Index}
		}
		u := st.usage()
		for _, name := range st.schedulable() {
			n := st.nodes[name]
			cpu, mem := n.alloc.cpu-u[name].cpu, n.alloc.mem-u[name].mem
			if t := r.tight; t == nil || cpu < t.cpu || cpu == t.cpu && mem < t.mem {
				r.tight = &tightMark{name, cpu, mem, ref.Index}
			}
		}
	}
	// A limit already broken in the snapshot counts only if the plan makes it worse.
	if int64(unsched) > *lim.MaxUnschedulableNodes && unsched > r.initUnsched {
		r.fail("limits", ref, "after", "", map[string]any{"field": "maxUnschedulableNodes", "value": unsched, "limit": *lim.MaxUnschedulableNodes})
	}
	if int64(sched) < *lim.MinSchedulableNodes && sched < r.initSched {
		r.fail("limits", ref, "after", "", map[string]any{"field": "minSchedulableNodes", "value": sched, "limit": *lim.MinSchedulableNodes})
	}
}

// zeroAllowed: the goal scales the deployment to zero and the policy allows it.
func (r *run) zeroAllowed(dep string) bool {
	return *r.in.policy.Limits.AllowScaleToZero && slices.ContainsFunc(r.in.plan.Goal, func(c Change) bool {
		return c.Deployment == dep && c.Field == "replicas" && c.To.Int == 0
	})
}

func (r *run) assert(ref StepRef, s Step, a *Assertions, phase string) {
	if a == nil {
		return
	}
	st := r.st
	check := func(object, key string, want, got any) {
		r.asserts++
		if want != got {
			r.fail("assertions", ref, phase, object, map[string]any{"key": key, "expected": want, "actual": got})
		}
	}
	if d := st.deps[s.Deployment]; d != nil {
		if a.Replicas != nil {
			check("deployment/"+s.Deployment, "replicas", *a.Replicas, d.replicas)
		}
		if a.Image != nil {
			check("deployment/"+s.Deployment, "image", *a.Image, d.images[cmp.Or(a.Container, s.Container)])
		}
	}
	if n := st.nodes[s.Node]; n != nil {
		if a.Unschedulable != nil {
			check("node/"+s.Node, "unschedulable", *a.Unschedulable, n.unsched)
		}
		if a.PodsOnNode != nil {
			var count int64
			for _, p := range st.pods {
				if p.owner != "DaemonSet" && p.mightBeOn(s.Node) {
					count++
				}
			}
			check("node/"+s.Node, "podsOnNode", *a.PodsOnNode, count)
		}
	}
}

// field is one desired field of one object, as in a goal change.
type field struct {
	object, name, container string
	val                     Value
}

func (f field) key() string { return f.object + "|" + f.name + "|" + f.container }

// desired lists every desired field in the state (spec 02), for rule frame.
func (st *state) desired() map[string]field {
	out := map[string]field{}
	add := func(object, name, container string, v Value) {
		f := field{object, name, container, v}
		out[f.key()] = f
	}
	for name, d := range st.deps {
		add("deployment/"+name, "replicas", "", Value{Kind: "int", Int: d.replicas})
		add("deployment/"+name, "restarted", "", Value{Kind: "bool", Bool: d.restarted})
		for c, img := range d.images {
			add("deployment/"+name, "image", c, Value{Kind: "string", Str: img})
		}
	}
	for name, n := range st.nodes {
		add("node/"+name, "unschedulable", "", Value{Kind: "bool", Bool: n.unsched})
		add("node/"+name, "drained", "", Value{Kind: "bool", Bool: n.drained})
	}
	return out
}

func changeField(c Change) field {
	object := "node/" + c.Node
	if c.Deployment != "" {
		object = "deployment/" + c.Deployment
	}
	return field{object, c.Field, c.Container, c.To}
}

func (f field) record(from Value) map[string]any {
	m := map[string]any{"object": f.object, "field": f.name, "from": from, "to": f.val}
	if f.container != "" {
		m["container"] = f.container
	}
	return m
}
