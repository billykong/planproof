package main

// The reviewer study (E-22) and its AI proxy (E-24):
//
//	planproof-eval study-items > results/study/items.json
//	planproof-eval review-proxy --claude-model claude-haiku-5-5 --repeat 3
//	planproof-eval review-summary results/review-proxy-*.jsonl > results/review-proxy.md

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/billykong/planproof/evals"
	"github.com/billykong/planproof/study"
)

func studyItems(w io.Writer) {
	items, err := study.Build(evals.Root())
	if err != nil {
		fail(err)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(map[string]any{"instructions": study.Instructions, "items": items}); err != nil {
		fail(err)
	}
}

func reviewProxy(args []string) {
	fs := flag.NewFlagSet("review-proxy", flag.ExitOnError)
	model := fs.String("claude-model", "claude-haiku-5-5", "the Claude model that reviews")
	effort := fs.String("effort", "high", "effort level")
	repeat := fs.Int("repeat", 1, "reviews of each item in each format")
	out := fs.String("out", "", "append reviews here (default results/review-proxy-<model>.jsonl)")
	fs.Parse(args)
	if *out == "" {
		*out = fmt.Sprintf("results/review-proxy-%s.jsonl", *model)
	}
	items, err := study.Build(evals.Root())
	if err != nil {
		fail(err)
	}
	f, err := os.OpenFile(*out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	c := anthropic.NewClient()
	for rep := 0; rep < *repeat; rep++ {
		for _, it := range items {
			for _, format := range []string{"raw", "packet"} {
				r := study.ProxyReview(context.Background(), c, *model, anthropic.BetaOutputConfigEffort(*effort), it, format)
				b, _ := json.Marshal(r)
				fmt.Fprintln(f, string(b))
				fmt.Fprintf(os.Stderr, "%s %-6s %-7s correct=%v %s\n", it.ID, format, r.Decision, r.Correct, r.Error)
			}
		}
	}
}

// reviewSummary is the E-24 table: per reviewer and format, the bad plans
// rejected (split by what kind of bad), the good plans rejected, and cost.
func reviewSummary(w io.Writer, paths []string) {
	items, err := study.Build(evals.Root())
	if err != nil {
		fail(err)
	}
	kind := map[string]string{}
	for _, it := range items {
		switch {
		case it.Answer == "approve":
			kind[it.ID] = "good"
		case it.Kernel == "Proved":
			kind[it.ID] = "goal"
		default:
			kind[it.ID] = "rule"
		}
	}
	type key struct{ reviewer, format string }
	type tally struct {
		n, errs                  int
		rule, ruleN, goal, goalN int
		falseAlarm, good         int
		in, outT, cacheR, cacheW int64
	}
	t := map[key]*tally{}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			fail(err)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			var r study.Review
			if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
				fail(err)
			}
			k := key{r.Reviewer, r.Format}
			if t[k] == nil {
				t[k] = &tally{}
			}
			x := t[k]
			x.n++
			x.in, x.outT, x.cacheR, x.cacheW = x.in+r.Usage.Input, x.outT+r.Usage.Output, x.cacheR+r.Usage.CacheRead, x.cacheW+r.Usage.CacheWrite
			if r.Error != "" {
				x.errs++
			}
			rejected := r.Decision == "reject"
			switch kind[r.Item] {
			case "good":
				x.good++
				if rejected {
					x.falseAlarm++
				}
			case "rule":
				x.ruleN++
				if rejected {
					x.rule++
				}
			case "goal":
				x.goalN++
				if rejected {
					x.goal++
				}
			}
		}
		f.Close()
	}
	var keys []key
	for k := range t {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].reviewer != keys[j].reviewer {
			return keys[i].reviewer < keys[j].reviewer
		}
		return keys[i].format > keys[j].format // raw before packet
	})
	frac := func(k, of int) string {
		if of == 0 {
			return "-"
		}
		return fmt.Sprintf("%d/%d (%d%%)", k, of, 100*k/of)
	}
	fmt.Fprintln(w, "| AI reviewer (proxy, not a person) | Format | Reviews | Bad plans rejected | Rule-breaking plans rejected | Wrong-goal plans rejected | Good plans rejected (false alarms) | No decision | Cost |")
	fmt.Fprintln(w, "| --- | --- | --- | --- | --- | --- | --- | --- | --- |")
	for _, k := range keys {
		x := t[k]
		cost := "-"
		if p, ok := price[strings.SplitN(k.reviewer, "/", 2)[0]]; ok {
			cost = fmt.Sprintf("$%.2f", (float64(x.in)*p[0]+float64(x.outT)*p[1]+float64(x.cacheR)*p[2]+float64(x.cacheW)*p[3])/1e6)
		}
		fmt.Fprintf(w, "| %s | %s | %d | %s | %s | %s | %s | %d | %s |\n", k.reviewer, k.format, x.n,
			frac(x.rule+x.goal, x.ruleN+x.goalN), frac(x.rule, x.ruleN), frac(x.goal, x.goalN), frac(x.falseAlarm, x.good), x.errs, cost)
	}
	fmt.Fprintln(w, "\nRule-breaking plans: the kernel refutes them (the packet says so). Wrong-goal plans: safe, but not what the ticket asked; the kernel proves them, and only the reviewer can catch them.")
}
