// Command planproof-eval runs the planning agent on the task set and
// compares models (PRD area E). Results are generated, never typed (E-23).
//
//	planproof-eval run --model local|claude|scripted [--tries 4] [--tasks a,b] [--repeat 1] [--append] [--out results/]
//	planproof-eval summary results/agent-*.jsonl > results/agent-summary.md
//
// run writes one JSON line per run (transcript and grade) to
// results/agent-<model>.jsonl and prints a summary.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/billykong/planproof/agent"
	"github.com/billykong/planproof/evals"
	"github.com/billykong/planproof/kernel"
)

// Record is one run: the transcript and its grade.
type Record struct {
	agent.Transcript
	Grade      evals.Grade `json:"grade"`
	Seconds    float64     `json:"seconds"`
	infeasible bool        // the task has no safe plan (from tasks.yaml, not stored)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "run":
		run(os.Args[2:])
	case "seeded":
		seeded(os.Stdout)
	case "responses":
		var recs []Record
		asJSON := false
		for _, p := range os.Args[2:] {
			if p == "--json" {
				asJSON = true
				continue
			}
			recs = append(recs, read(p)...)
		}
		if asJSON {
			responsesJSON(os.Stdout, recs)
		} else {
			responses(os.Stdout, recs)
		}
	case "summary":
		var recs []Record
		for _, p := range os.Args[2:] {
			recs = append(recs, read(p)...)
		}
		summary(os.Stdout, markInfeasible(recs))
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: planproof-eval run --model local|claude|scripted [...] | planproof-eval summary FILE...")
	os.Exit(2)
}

func run(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	model := fs.String("model", "", "local, claude or scripted")
	tries := fs.Int("tries", 4, "submissions per task")
	only := fs.String("tasks", "", "comma-separated task ids (default: all)")
	repeat := fs.Int("repeat", 1, "runs per task")
	out := fs.String("out", "results", "directory for the JSON lines")
	appendRuns := fs.Bool("append", false, "add to the model's results file instead of replacing it")
	claudeModel := fs.String("claude-model", "claude-opus-5-5", "Claude model")
	effort := fs.String("effort", "high", "Claude effort")
	tag := fs.String("tag", "", "label added to the model's name and results file (e.g. prompt-v2)")
	localURL := fs.String("local-url", "http://172.30.99.10:8000/v1", "the local model server")
	localKeyFile := fs.String("local-key-file", filepath.Join(os.Getenv("HOME"), ".config/safe-ops/local-model-key"), "its bearer key")
	localModel := fs.String("local-model", "qwen2.5-1.5b-instruct", "its served model name")
	fs.Parse(args)

	var m agent.Model
	switch *model {
	case "claude":
		c := agent.NewClaude(*claudeModel, anthropic.BetaOutputConfigEffort(*effort))
		c.Label = *tag
		m = c
	case "local":
		key, err := os.ReadFile(*localKeyFile)
		if err != nil {
			fail(err)
		}
		m = &agent.Local{BaseURL: *localURL, Key: strings.TrimSpace(string(key)), Model: *localModel}
	case "scripted":
		m = nil // per task: replays the reference or naive plan
	default:
		usage()
	}

	root := evals.Root()
	specs, err := evals.Load(root)
	if err != nil {
		fail(err)
	}
	name := *model
	if m != nil {
		name = strings.NewReplacer("/", "-", ":", "-").Replace(m.Name())
	}
	os.MkdirAll(filepath.Join(root, *out), 0o755)
	path := filepath.Join(root, *out, "agent-"+name+".jsonl")
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if *appendRuns {
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	var recs []Record
	for _, s := range specs {
		if *only != "" && !slices.Contains(strings.Split(*only, ","), s.ID) {
			continue
		}
		task, err := s.Task(root)
		if err != nil {
			fail(err)
		}
		for i := 0; i < *repeat; i++ {
			mm := m
			if mm == nil {
				plan := s.Reference
				final := ""
				if plan == nil {
					plan, final = s.Naive, agent.NoSafePlan+": "+s.Expect.NoSafePlan
				}
				mm = agent.Scripted{Plans: []json.RawMessage{plan}, Final: final}
			}
			start := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			tr := agent.Run(ctx, mm, task, *tries)
			cancel()
			r := Record{Transcript: tr, Grade: evals.GradeRun(s, tr), Seconds: time.Since(start).Seconds()}
			b, _ := json.Marshal(r)
			f.Write(append(b, '\n'))
			recs = append(recs, r)
			fmt.Fprintf(os.Stderr, "%-18s %-22s tries %d  %6.1fs  %s\n", s.ID, r.Grade.Outcome, len(tr.Attempts), r.Seconds, short(tr))
		}
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", path)
	if *appendRuns {
		recs = read(path) // summarise every run in the file
	}
	summary(os.Stdout, markInfeasible(recs))
}

func short(tr agent.Transcript) string {
	if tr.Error != "" {
		return "error: " + tr.Error
	}
	var parts []string
	for _, a := range tr.Attempts {
		p := string(a.Result)
		if len(a.Rules) > 0 {
			p += "(" + strings.Join(slices.Compact(slices.Sorted(slices.Values(a.Rules))), ",") + ")"
		}
		if a.Trigger != "" {
			p += "(" + a.Trigger + ")"
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, " > ")
}

// markInfeasible notes which records are for tasks with no safe plan.
func markInfeasible(recs []Record) []Record {
	specs, err := evals.Load(evals.Root())
	if err != nil {
		fail(err)
	}
	no := map[string]bool{}
	for _, s := range specs {
		no[s.ID] = s.Expect.NoSafePlan != ""
	}
	for i := range recs {
		recs[i].infeasible = no[recs[i].Task]
	}
	return recs
}

func read(path string) []Record {
	f, err := os.Open(path)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	var out []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			fail(err)
		}
		out = append(out, r)
	}
	return out
}

// price is $ per million tokens: input, output, cache read, cache write.
var price = map[string][4]float64{
	"claude-opus-5-5":   {4, 20, 0.20, 5},
	"claude-sonnet-5-5": {2, 10, 0.20, 2.50},
	"claude-haiku-5-5":  {0.10, 0.50, 0.01, 0.125}, // up to 100K-token prompts; cache prices assumed at the usual 0.1x / 1.25x
}

// summary is the E-12 table, one row per model.
func summary(w io.Writer, recs []Record) {
	byModel := map[string][]Record{}
	var models []string
	for _, r := range recs {
		if _, ok := byModel[r.Model]; !ok {
			models = append(models, r.Model)
		}
		byModel[r.Model] = append(byModel[r.Model], r)
	}
	fmt.Fprintln(w, "| Model | Runs | Solvable tasks solved | Infeasible tasks declined | First plan Proved | Proved after repair | Submissions per run | Unknown seen | Proved, wrong goal | \"No safe plan\" on a solvable task | Errors | Tokens in / out (cached) | Cost |")
	fmt.Fprintln(w, "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |")
	for _, m := range models {
		rs := byModel[m]
		var solved, solvable, declined, infeasible, first, repaired, subs, unknown, wrong, falseDecl, errs int
		var u agent.Usage
		for _, r := range rs {
			u.Add(r.Usage)
			subs += len(r.Attempts)
			if r.infeasible {
				infeasible++
			} else {
				solvable++
			}
			if r.Grade.Outcome == evals.ProvedCorrect {
				solved++
			}
			if r.Grade.Outcome == evals.Declared {
				declined++
			}
			if len(r.Attempts) > 0 && r.Attempts[0].Result == kernel.Proved {
				first++
			} else if r.Stopped == "proved" {
				repaired++
			}
			if slices.ContainsFunc(r.Attempts, func(a agent.Attempt) bool { return a.Result == kernel.Unknown }) {
				unknown++
			}
			switch r.Grade.Outcome {
			case evals.ProvedWrongGoal:
				wrong++
			case evals.DeclaredWrongly:
				falseDecl++
			case evals.RunError:
				errs++
			}
		}
		cost := "-"
		if p, ok := price[strings.SplitN(m, "/", 2)[0]]; ok {
			c := (float64(u.Input)*p[0] + float64(u.Output)*p[1] + float64(u.CacheRead)*p[2] + float64(u.CacheWrite)*p[3]) / 1e6
			cost = fmt.Sprintf("$%.2f", c)
		}
		n := len(rs)
		frac := func(k, of int) string {
			if of == 0 {
				return "-"
			}
			return fmt.Sprintf("%d/%d (%.0f%%)", k, of, 100*float64(k)/float64(of))
		}
		fmt.Fprintf(w, "| %s | %d | %s | %s | %s | %s | %.1f | %d | %d | %d | %d | %d / %d (%d) | %s |\n",
			m, n, frac(solved, solvable), frac(declined, infeasible), frac(first, n), frac(repaired, n), float64(subs)/float64(n),
			unknown, wrong, falseDecl, errs, u.Input+u.CacheRead+u.CacheWrite, u.Output, u.CacheRead, cost)
	}
	// Per task, for every model: what each run ended as.
	fmt.Fprintln(w)
	fmt.Fprintf(w, "| Task | %s |\n|%s\n", strings.Join(models, " | "), strings.Repeat(" --- |", len(models)+1))
	var tasks []string
	cell := map[string]map[string][]string{}
	for _, r := range recs {
		if cell[r.Task] == nil {
			cell[r.Task] = map[string][]string{}
			tasks = append(tasks, r.Task)
		}
		mark := "✗ " + r.Grade.Outcome
		if r.Grade.Pass {
			mark = "✓ " + r.Grade.Outcome
		}
		cell[r.Task][r.Model] = append(cell[r.Task][r.Model], fmt.Sprintf("%s (%d)", mark, len(r.Attempts)))
	}
	for _, t := range tasks {
		row := []string{t}
		for _, m := range models {
			row = append(row, strings.Join(cell[t][m], "; "))
		}
		fmt.Fprintf(w, "| %s |\n", strings.Join(row, " | "))
	}
}

// responses rebuilds what the kernel sent back to the model in each run:
// the kernel is deterministic, so checking each stored plan again gives
// exactly the tool result the loop sent (agent.Run: after every submission
// except a Proved one or the last allowed one).
func responses(w io.Writer, recs []Record) {
	root := evals.Root()
	specs, err := evals.Load(root)
	if err != nil {
		fail(err)
	}
	byID := map[string]evals.Spec{}
	for _, s := range specs {
		byID[s.ID] = s
	}
	const maxTries = 4
	for _, r := range recs {
		if len(r.Attempts) == 0 && !r.Nudged {
			continue
		}
		task, err := byID[r.Task].Task(root)
		if err != nil {
			fail(err)
		}
		fmt.Fprintf(w, "### %s, %s: %s\n\n", r.Model, r.Task, r.Grade.Outcome)
		for i, a := range r.Attempts {
			fmt.Fprintf(w, "Submission %d: %s\n\n", i+1, a.Result)
			if a.Result == kernel.Proved || i+1 >= maxTries {
				fmt.Fprintf(w, "(nothing sent back: the loop stops here)\n\n")
				continue
			}
			v := kernel.Check(task.Snapshot, agent.Plan(task, a.Plan), task.Policy)
			var pretty bytes.Buffer
			json.Indent(&pretty, []byte(agent.Feedback(v, maxTries-(i+1))), "", "  ")
			fmt.Fprintf(w, "```json\n%s\n```\n\n", pretty.String())
		}
		if r.Nudged {
			fmt.Fprintf(w, "Reminder sent once: %q\n\n", agent.Reminder)
		}
	}
}

// Exchange is one submission and what the kernel sent back, if anything.
type Exchange struct {
	N        int             `json:"n"`
	Plan     json.RawMessage `json:"plan"`
	Result   kernel.Result   `json:"result"`
	Response json.RawMessage `json:"response,omitempty"` // null: nothing sent (Proved, or the last allowed submission)
}

// responsesJSON is responses as data, one object per run.
func responsesJSON(w io.Writer, recs []Record) {
	root := evals.Root()
	specs, err := evals.Load(root)
	if err != nil {
		fail(err)
	}
	byID := map[string]evals.Spec{}
	for _, s := range specs {
		byID[s.ID] = s
	}
	const maxTries = 4
	type run struct {
		Model, Task, Ticket, Outcome, Final, Reminder string
		Pass                                          bool
		Exchanges                                     []Exchange
	}
	var out []run
	for _, r := range recs {
		task, err := byID[r.Task].Task(root)
		if err != nil {
			fail(err)
		}
		x := run{Model: r.Model, Task: r.Task, Ticket: byID[r.Task].Ticket, Outcome: r.Grade.Outcome, Pass: r.Grade.Pass, Final: r.Final, Exchanges: []Exchange{}}
		if r.Nudged {
			x.Reminder = agent.Reminder
		}
		for i, a := range r.Attempts {
			e := Exchange{N: i + 1, Plan: a.Plan, Result: a.Result}
			if a.Result != kernel.Proved && i+1 < maxTries {
				v := kernel.Check(task.Snapshot, agent.Plan(task, a.Plan), task.Policy)
				e.Response = json.RawMessage(agent.Feedback(v, maxTries-(i+1)))
			}
			x.Exchanges = append(x.Exchanges, e)
		}
		out = append(out, x)
	}
	b, _ := json.MarshalIndent(out, "", " ")
	w.Write(append(b, '\n'))
}

// seeded writes the seeded-plan table (E-11, E-21).
func seeded(w io.Writer) {
	root := evals.Root()
	specs, err := evals.Load(root)
	if err != nil {
		fail(err)
	}
	byID := map[string]evals.Spec{}
	for _, s := range specs {
		byID[s.ID] = s
	}
	list, err := evals.LoadSeeded(root)
	if err != nil {
		fail(err)
	}
	counts := map[string]int{}
	fmt.Fprintln(w, "| Seeded plan | Should be caught by | Kernel | Caught by |")
	fmt.Fprintln(w, "| --- | --- | --- | --- |")
	for _, s := range list {
		r, err := evals.CheckSeeded(root, byID, s)
		if err != nil {
			fail(err)
		}
		k := r.Kernel
		if len(r.Rules) > 0 {
			k += " (" + strings.Join(slices.Compact(slices.Sorted(slices.Values(r.Rules))), ", ") + ")"
		}
		if r.Trigger != "" {
			k += " (" + r.Trigger + ")"
		}
		caught := r.CaughtBy
		if caught == "" {
			caught = "**MISSED**"
		}
		counts[caught]++
		fmt.Fprintf(w, "| %s | %s | %s | %s |\n", r.ID, r.Want, k, caught)
	}
	fmt.Fprintf(w, "\n%d seeded plans: %d caught by the kernel, %d only by checking the goal against the ticket, %d missed.\n",
		len(list), counts["kernel"], counts["grader (goal vs ticket)"], counts["**MISSED**"])
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "planproof-eval:", err)
	os.Exit(1)
}
