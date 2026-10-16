// Package render turns a kernel verdict into the review packet a person
// reads (spec 06). It is a pure function with fixed templates: no model
// writes any part of the packet, and free text from the plan appears only
// quoted, in the section that says the kernel did not check it.
//
// The renderer is trusted like the kernel: a wrong sentence misleads as
// badly as a wrong verdict. Tests check that every fact produces a sentence
// and every sentence comes from a fact or from the fixed structure.
package render

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/billykong/planproof/kernel"
)

// Packet renders a verdict as plain text.
func Packet(v kernel.Verdict) string { return build(v).text(false) }

// Debug renders a verdict with each line tagged by the fact that produced
// it ([f12]) or the structure template ([head.goal]).
func Debug(v kernel.Verdict) string { return build(v).text(true) }

type line struct {
	fact   int    // index into the verdict's facts, or -1 for structure
	key    string // the template
	indent int
	text   string
}

type packet struct {
	v     kernel.Verdict
	lines []line
}

func (p *packet) text(debug bool) string {
	var b strings.Builder
	for _, l := range p.lines {
		if debug && l.key != "" {
			if l.fact >= 0 {
				fmt.Fprintf(&b, "[f%d] ", l.fact)
			} else {
				fmt.Fprintf(&b, "[%s] ", l.key)
			}
		}
		b.WriteString(strings.Repeat(" ", l.indent) + l.text + "\n")
	}
	return b.String()
}

// trace, when set by a test, sees every template used.
var trace func(key string)

var placeholder = regexp.MustCompile(`\{(\w+)\}`)

// fill fills a template. Every placeholder must be given. Replacement is a
// single pass, so text inside a value is never treated as a placeholder.
func fill(key string, kv ...string) string {
	if trace != nil {
		trace(key)
	}
	t, ok := templates[key]
	if !ok {
		panic("render: no template " + key)
	}
	kv, given := slices.Clone(kv), map[string]bool{}
	for i := 0; i < len(kv); i += 2 {
		given[kv[i]] = true
		kv[i] = "{" + kv[i] + "}"
	}
	for _, m := range placeholder.FindAllStringSubmatch(t, -1) {
		if !given[m[1]] {
			panic("render: template " + key + " needs {" + m[1] + "}")
		}
	}
	return strings.NewReplacer(kv...).Replace(t)
}

// say adds the lines of one template, rendered for one fact (or -1).
func (p *packet) say(fact, indent int, key string, kv ...string) {
	for _, t := range strings.Split(fill(key, kv...), "\n") {
		p.lines = append(p.lines, line{fact, key, indent, t})
	}
}

func (p *packet) blank() { p.lines = append(p.lines, line{fact: -1}) }

func code(s string) string { return "`" + s + "`" }

// object strips the kind prefix: "deployment/shop/cart" is shown as
// `shop/cart`, "node/n1" as `n1`.
func object(o string) string {
	_, name, _ := strings.Cut(o, "/")
	return code(name)
}

func num(v any) string {
	switch n := v.(type) {
	case float64:
		return strconv.FormatFloat(n, 'f', -1, 64)
	case nil:
		return ""
	}
	return fmt.Sprint(v)
}

// value shows a goal or evidence value: numbers and booleans as words,
// strings (images) in code font.
func value(v any) string {
	switch x := v.(type) {
	case kernel.Value:
		switch x.Kind {
		case "int":
			return strconv.FormatInt(x.Int, 10)
		case "bool":
			return strconv.FormatBool(x.Bool)
		}
		return code(x.Str)
	case *kernel.Value:
		return value(*x)
	case string:
		return code(x)
	case map[string]any: // a Value after a JSON round trip never is; kept for safety
		return code(fmt.Sprint(x))
	}
	return num(v)
}

// where a step reference points, in words.
func stepWords(s *kernel.StepRef) (step, Step string) {
	switch {
	case s == nil || !s.End && !s.Rollback && s.Index == 0:
		return "the start", "The start"
	case s.End:
		return "the last step", "The last step"
	case s.Rollback:
		return "rollback step " + strconv.Itoa(s.Index), "Rollback step " + strconv.Itoa(s.Index)
	}
	return "step " + strconv.Itoa(s.Index), "Step " + strconv.Itoa(s.Index)
}

// when a failure happened: "During step 4", "After rollback step 2".
func when(f kernel.Fact) string {
	step, _ := stepWords(f.Step)
	phase := "During"
	if f.Phase == "after" || f.Phase == "end" {
		phase = "After"
	}
	return phase + " " + step
}

func list(items []string) string {
	for i := range items {
		items[i] = code(items[i])
	}
	if len(items) == 1 {
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

func duration(seconds int64) string {
	switch {
	case seconds == 60:
		return "1 minute"
	case seconds%60 == 0:
		return strconv.FormatInt(seconds/60, 10) + " minutes"
	case seconds == 1:
		return "1 second"
	}
	return strconv.FormatInt(seconds, 10) + " seconds"
}

// unchecked quotes text from the plan: control and bidirectional-override
// characters become spaces, so the text can't fake extra lines or reorder
// what the reader sees (R-07).
func unchecked(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r >= '‪' && r <= '‮' || r >= '⁦' && r <= '⁩' {
			return ' '
		}
		return r
	}, s)
	if r := []rune(s); len(r) > 1000 {
		s = string(r[:1000])
	}
	return `"` + s + `"`
}

func build(v kernel.Verdict) *packet {
	p := &packet{v: v}
	// The name is the agent's: shown as code, an identifier, never prose (W7).
	name := code(v.PlanName)
	if v.PlanName == "" {
		name = "(the kernel cannot read the inputs)"
	}
	short := func(h string) string { h = strings.TrimPrefix(h, "sha256:"); return h[:min(12, len(h))] }
	p.say(-1, 0, "head.title", "name", name)
	p.say(-1, 0, "head.hashes", "plan", short(v.PlanSHA256), "snapshot", short(v.SnapshotSHA256), "policy", short(v.PolicySHA256), "version", v.KernelVersion)
	p.blank()
	p.caution()
	p.goal()
	p.result()
	p.checks()
	p.notChecked()
	p.steps("head.procedure", "step")
	p.steps("head.rollback", "rollback-step")
	return p
}

func (p *packet) each(typ string, fn func(i int, f kernel.Fact)) {
	for i, f := range p.v.Facts {
		if f.Type == typ {
			fn(i, f)
		}
	}
}

func (p *packet) caution() {
	switch p.v.Result {
	case kernel.Refuted:
		p.say(-1, 0, "head.caution")
		p.each("failure", func(i int, f kernel.Fact) { p.failure(i, 0, f) })
	case kernel.Unknown:
		p.say(-1, 0, "head.cautionUnknown")
		p.each("unknown", func(i int, f kernel.Fact) { p.unknown(i, f) })
	default:
		return
	}
	p.blank()
}

func (p *packet) goal() {
	p.say(-1, 0, "head.goal")
	n := 0
	p.each("goal-change", func(i int, f kernel.Fact) {
		n++
		p.say(i, 3, changeKey("goal", f.Field, f.To), changeArgs(f.Object, f.Container, f.From, f.To, "i", strconv.Itoa(n))...)
	})
	if n == 0 {
		p.say(-1, 3, "head.none")
	} else {
		p.say(-1, 3, "goal.nothingElse")
	}
	p.blank()
}

func changeKey(prefix, field string, to any) string {
	if field == "unschedulable" {
		if value(to) == "false" {
			return prefix + ".uncordon"
		}
		return prefix + ".cordon"
	}
	return prefix + "." + field
}

// changeArgs gives the placeholders a change template may use. A goal line
// is numbered, so the number goes in front of the filled template.
func changeArgs(obj, container string, from, to any, extra ...string) []string {
	return append([]string{"n", object(obj), "d", object(obj), "c", code(container), "from", value(from), "to", value(to)}, extra...)
}

func (p *packet) result() {
	p.say(-1, 0, "head.result")
	counts := map[string]int{}
	var failed []string
	p.each("check", func(_ int, f kernel.Fact) {
		counts[f.Outcome]++
		if f.Outcome == "fail" {
			failed = append(failed, f.Rule)
		}
	})
	switch p.v.Result {
	case kernel.Proved:
		p.say(-1, 3, "head.proved", "k", strconv.Itoa(counts["pass"]))
	case kernel.Refuted:
		var at *kernel.StepRef
		p.each("failure", func(_ int, f kernel.Fact) { at = f.Step })
		where := "at " + func() string { s, _ := stepWords(at); return s }()
		switch {
		case at != nil && at.End:
			where = "after the last step"
		case at == nil || at.Index == 0 && !at.Rollback:
			where = "before the first step"
		}
		if slices.Contains(failed, "input") {
			p.say(-1, 3, "head.refutedInput")
			break
		}
		p.say(-1, 3, plural("head.refuted", len(failed)), "k", strconv.Itoa(len(failed)), "where", where)
	case kernel.Unknown:
		p.each("unknown", func(_ int, f kernel.Fact) {
			step, _ := stepWords(f.Step)
			p.say(-1, 3, "head.unknown", "step", step)
		})
	}
	p.each("approval", func(i int, f kernel.Fact) {
		if f.Required == nil || !*f.Required {
			p.say(i, 3, "approval.notNeeded")
			return
		}
		p.say(i, 3, "approval.required")
		for _, r := range f.Reasons {
			if r != "critical" {
				p.say(i, 3, "approval."+r)
				continue
			}
			for _, d := range anyList(f.Evidence["critical"]) {
				p.say(i, 3, "approval.critical", "d", code(fmt.Sprint(d)))
			}
		}
	})
	p.blank()
}

// plural picks key1 for a count of 1.
func plural(key string, n int) string {
	if n == 1 {
		return key + "1"
	}
	return key
}

func anyList(v any) []any {
	switch l := v.(type) {
	case []any:
		return l
	case []string:
		out := make([]any, len(l))
		for i, s := range l {
			out[i] = s
		}
		return out
	}
	return nil
}

func (p *packet) checks() {
	p.say(-1, 0, "head.checks")
	n := 0
	p.each("check", func(i int, f kernel.Fact) {
		n++
		title := templates["title."+f.Rule]
		key := map[string]string{"pass": "check.pass", "fail": "check.fail", "incomplete": "check.incomplete",
			"not-reached": "check.notReached", "not-applicable": "check.notApplicable"}[f.Outcome]
		step, _ := stepWords(f.Step)
		p.say(i, 3, key, "i", strconv.Itoa(n), "title", title, "step", step)
		if f.Outcome == "pass" || f.Outcome == "fail" {
			p.evidence(i, f)
		}
	})
	p.blank()
}

// evidence adds the lines behind a check's outcome.
func (p *packet) evidence(i int, f kernel.Fact) {
	ev := f.Evidence
	switch f.Rule {
	case "limits":
		if ev == nil {
			return
		}
		x := num(ev["maxUnschedulable"])
		p.say(i, 6, plural("pass.limits.max", atoi(x)), "x", x, "lim", num(ev["limitUnschedulable"]))
		p.say(i, 6, "pass.limits.min", "x", num(ev["minSchedulable"]), "lim", num(ev["limitSchedulable"]))
	case "assertions":
		k := num(ev["count"])
		key := plural("pass.assertions", atoi(k))
		if k == "0" {
			key = "pass.assertions0"
		}
		p.say(i, 6, key, "k", k)
	case "budget":
		for _, it := range anyList(ev["items"]) {
			it := it.(map[string]any)
			args := []string{"d", code(fmt.Sprint(it["deployment"])), "x", num(it["lowestAvailable"]), "need", num(it["needs"])}
			if it["step"] == nil {
				p.say(i, 6, "pass.budget.stays", args...)
				continue
			}
			step, _ := stepWords(&kernel.StepRef{Index: atoi(num(it["step"]))})
			p.say(i, 6, "pass.budget", append(args, "phase", fmt.Sprint(it["phase"]), "step", step)...)
		}
	case "never-zero":
		if f.Outcome == "pass" {
			p.say(i, 6, "pass.never-zero")
			return
		}
		for _, it := range anyList(ev["items"]) {
			it := it.(map[string]any)
			step, _ := stepWords(stepRef(it["step"]))
			p.say(i, 6, "fail.never-zero", "d", code(fmt.Sprint(it["deployment"])), "phase", fmt.Sprint(it["phase"]), "step", step)
		}
	case "capacity":
		if ev != nil {
			step, _ := stepWords(&kernel.StepRef{Index: atoi(num(ev["step"]))})
			p.say(i, 6, "pass.capacity", "n", code(fmt.Sprint(ev["node"])), "step", step, "cpu", num(ev["cpuFreeMilli"]), "mem", num(ev["memoryFreeMiB"]))
		}
	case "frame":
		if f.Outcome == "pass" {
			k := num(ev["changes"])
			p.say(i, 6, plural("pass.frame", atoi(k)), "k", k)
			return
		}
		p.each("failure", func(j int, g kernel.Fact) {
			if g.Rule == "frame" {
				p.failure(j, 6, g)
			}
		})
	case "rollback":
		if f.Outcome == "pass" {
			k := num(ev["restored"])
			p.say(i, 6, plural("pass.rollback", atoi(k)), "k", k)
		}
	default:
		if f.Outcome == "pass" {
			p.say(i, 6, "pass."+f.Rule)
		}
	}
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

// stepRef reads a step reference out of evidence: a *StepRef in memory, a
// number or string after a JSON round trip.
func stepRef(v any) *kernel.StepRef {
	switch s := v.(type) {
	case *kernel.StepRef:
		return s
	case float64:
		return &kernel.StepRef{Index: int(s)}
	case string:
		if s == "end" {
			return &kernel.StepRef{End: true}
		}
		return &kernel.StepRef{Index: atoi(strings.TrimPrefix(s, "rollback:")), Rollback: true}
	}
	return nil
}

// failure renders a failure fact's sentences (spec 06, CAUTION block).
func (p *packet) failure(i, indent int, f kernel.Fact) {
	ev := f.Evidence
	step, Step := stepWords(f.Step)
	str := func(k string) string { return fmt.Sprint(ev[k]) }
	rule := f.Rule
	if rule == "rollback" {
		if ev["notRestored"] != nil {
			p.say(i, indent, "failure.rollback.notRestored")
			for _, r := range anyList(ev["notRestored"]) {
				p.change(i, indent, r.(map[string]any))
			}
			return
		}
		rule = str("innerRule") // its sentences already say "rollback step n"
	}
	common := []string{"When", when(f), "Step", Step, "step", step, "object", object(f.Object), "d", object(f.Object), "n", object(f.Object)}
	switch rule {
	case "budget":
		x := num(ev["lowestAvailable"])
		key := plural("failure.budget", atoi(x))
		if x == "0" {
			key = "failure.budget0"
		}
		p.say(i, indent, key, append(common, "x", x, "need", num(ev["needs"]))...)
	case "frame":
		switch {
		case ev["reason"] == "repeated":
			p.say(i, indent, "failure.frame.repeated", append(common, "field", code(str("field")))...)
		case ev["reason"] == "from-mismatch":
			p.say(i, indent, "failure.frame.mismatch", append(common, "field", code(str("field")), "from", value(ev["from"]), "actual", value(ev["actual"]))...)
		case ev["reason"] == "no-change":
			p.say(i, indent, "failure.frame.noChange", append(common, "field", code(str("field")))...)
		default:
			for _, part := range []string{"extra", "missing"} {
				if changes := anyList(ev[part]); len(changes) > 0 {
					p.say(i, indent, "failure.frame."+part)
					for _, c := range changes {
						p.change(i, indent, c.(map[string]any))
					}
				}
			}
		}
	case "no-orphans":
		p.say(i, indent, "failure.no-orphans", append(common, "p", object(f.Object))...)
	case "assertions":
		p.say(i, indent, "failure.assertions", append(common, "key", code(str("key")), "expected", value(ev["expected"]), "actual", value(ev["actual"]))...)
	case "scope":
		key := "failure.scope." + str("reason")
		if c, _ := ev["container"].(string); c != "" {
			key = "failure.scope.container"
			common = append(common, "c", code(c))
		}
		p.say(i, indent, key, common...)
	case "limits":
		p.say(i, indent, "failure.limits."+str("field"), append(common, "x", num(ev["value"]), "lim", num(ev["limit"]), "image", code(str("value")))...)
	case "input":
		key, args := "failure.input", []string{"input", f.Object, "reason", code(str("reason"))}
		if path := str("path"); path != "" {
			key, args = "failure.input.at", append(args, "path", code(path))
		}
		p.say(i, indent, key, args...)
	default: // never-zero, capacity, cordon-first, settled, protected
		if d, ok := ev["deployment"]; ok {
			common = append(common, "d", code(fmt.Sprint(d)))
		}
		p.say(i, indent, "failure."+rule, common...)
	}
}

// change describes one changed field from frame or rollback evidence.
func (p *packet) change(i, indent int, c map[string]any) {
	container, _ := c["container"].(string)
	p.say(i, indent, changeKey("change", fmt.Sprint(c["field"]), c["to"]), changeArgs(fmt.Sprint(c["object"]), container, c["from"], c["to"])...)
}

func (p *packet) unknown(i int, f kernel.Fact) {
	_, Step := stepWords(f.Step)
	o := object(f.Object)
	p.say(i, 0, "unknown."+f.Trigger, "Step", Step, "d", o, "n", o, "p", o)
}

func (p *packet) notChecked() {
	p.say(-1, 0, "head.notChecked")
	p.say(-1, 3, "head.goalNotChecked")
	p.each("assumption", func(i int, f kernel.Fact) {
		var steps []string
		for _, s := range f.Steps {
			steps = append(steps, strconv.Itoa(s))
		}
		words := "step " + strings.Join(steps, "")
		if len(steps) > 1 {
			words = "steps " + strings.Join(steps[:len(steps)-1], ", ") + " and " + steps[len(steps)-1]
		}
		p.say(i, 3, "assumption."+f.ID, "steps", words)
	})
	intro := false
	p.each("unchecked-text", func(i int, f kernel.Fact) {
		if !intro {
			p.say(i, 3, "unchecked.intro")
			intro = true
		}
		key := map[string]string{"ticket": "unchecked.ticket", "rationale": "unchecked.reason"}[f.Field]
		p.say(i, 3, key, "text", unchecked(f.Text))
	})
	p.blank()
}

// steps renders the procedure or the rollback (spec 06).
func (p *packet) steps(head, typ string) {
	p.say(-1, 0, head)
	n := 0
	p.each(typ, func(i int, f kernel.Fact) {
		n++
		s := f.Params
		args := []string{"i", strconv.Itoa(n), "n", code(s.Node), "d", code(s.Deployment), "c", code(s.Container), "image", code(s.Image)}
		switch s.Op {
		case "scale":
			args = append(args, "x", strconv.FormatInt(*s.Replicas, 10))
		case "wait":
			args = append(args, "list", list(slices.Clone(s.Deployments)), "time", duration(*s.TimeoutSeconds))
		}
		key := "step." + s.Op
		if s.Op == "wait" && len(s.Deployments) == 1 {
			key = "step.wait1"
		}
		p.say(i, 3, key, args...)
		for _, a := range []struct {
			key string
			a   *kernel.Assertions
		}{{"pre", s.Pre}, {"post", s.Post}} {
			for _, claim := range claims(s, a.a) {
				p.say(i, 6, a.key, "claim", claim)
			}
		}
	})
	if n == 0 {
		if typ == "step" {
			p.say(-1, 3, "head.none")
		} else {
			p.say(-1, 3, "head.noRollback")
		}
	}
	if typ == "step" {
		p.blank()
		return
	}
	p.each("not-reversible", func(i int, f kernel.Fact) {
		p.say(i, 3, "note."+f.Field, "n", object(f.Object), "d", object(f.Object))
	})
}

// claims turns a step's assertions into the clauses of spec 06.
func claims(s *kernel.Step, a *kernel.Assertions) []string {
	if a == nil {
		return nil
	}
	var out []string
	if a.Replicas != nil {
		out = append(out, fill("claim.replicas", "d", code(s.Deployment), "x", strconv.FormatInt(*a.Replicas, 10)))
	}
	if a.Image != nil {
		c := a.Container
		if c == "" {
			c = s.Container
		}
		out = append(out, fill("claim.image", "c", code(c), "d", code(s.Deployment), "image", code(*a.Image)))
	}
	if a.Unschedulable != nil {
		key := "claim.schedulable"
		if *a.Unschedulable {
			key = "claim.unschedulable"
		}
		out = append(out, fill(key, "n", code(s.Node)))
	}
	if a.PodsOnNode != nil {
		out = append(out, fill("claim.podsOnNode", "n", code(s.Node), "x", strconv.FormatInt(*a.PodsOnNode, 10)))
	}
	return out
}
