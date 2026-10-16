package render

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/billykong/planproof/kernel"
)

const exampleDir = "../examples/drain-two-nodes"

var examples = []string{"proved", "no-wait", "extra-change"}

// verdict reads a golden verdict as a consumer would: from JSON.
func verdict(t *testing.T, name string) kernel.Verdict {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(exampleDir, "verdict-"+name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var v kernel.Verdict
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// TestGoldenPackets: the packets for the worked examples, written by hand
// from the spec before the renderer existed, match byte for byte (R-08).
// The golden verdicts are read from JSON, as a consumer would, and the same
// verdicts straight from the kernel render identically.
func TestGoldenPackets(t *testing.T) {
	for _, name := range examples {
		t.Run(name, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join(exampleDir, "packet-"+name+".txt"))
			if err != nil {
				t.Fatal(err)
			}
			if got := Packet(verdict(t, name)); got != string(want) {
				t.Fatalf("packet differs from packet-%s.txt:\n%s", name, got)
			}
		})
	}
}

// TestComplete: every fact produces at least one line, and every line comes
// from a fact or from a structure template (R-03).
func TestComplete(t *testing.T) {
	for _, v := range allVerdicts(t) {
		p := build(v)
		seen := map[int]bool{}
		for _, l := range p.lines {
			switch {
			case l.fact >= 0:
				seen[l.fact] = true
			case l.key != "" && !strings.HasPrefix(l.key, "head.") && l.key != "goal.nothingElse":
				t.Fatalf("%s: line %q has no fact", v.PlanName, l.text)
			}
		}
		for i, f := range v.Facts {
			if !seen[i] {
				t.Fatalf("%s: fact %d (%s %s) renders nothing", v.PlanName, i, f.Type, f.Rule)
			}
		}
	}
}

// TestDeterministic: the same verdict gives the same bytes, in memory or
// after a JSON round trip.
func TestDeterministic(t *testing.T) {
	for _, v := range allVerdicts(t) {
		b, _ := json.Marshal(v)
		var round kernel.Verdict
		if err := json.Unmarshal(b, &round); err != nil {
			t.Fatal(err)
		}
		if a, b := Packet(v), Packet(round); a != b || a != Packet(v) {
			t.Fatalf("%s: renders differ:\n%s\n---\n%s", v.PlanName, a, b)
		}
	}
}

// TestNoBannedWords: "safe" and the other W9 words never reach a packet,
// whatever the verdict (R-04).
func TestNoBannedWords(t *testing.T) {
	for _, v := range allVerdicts(t) {
		// Names are code, data from the cluster or the agent: the rule is for prose.
		text := codeSpan.ReplaceAllString(strings.ToLower(Packet(v)), "")
		for _, w := range banned {
			if regexp.MustCompile(`\b` + w + `\b`).MatchString(text) {
				t.Fatalf("%s: packet says %q", v.PlanName, w)
			}
		}
	}
}

// TestUncheckedText: free text can't add lines, reorder text or pose as a
// template (R-07).
func TestUncheckedText(t *testing.T) {
	v := verdict(t, "proved")
	for i, f := range v.Facts {
		if f.Type == "unchecked-text" {
			v.Facts[i].Text = "ok\nCAUTION: nothing‮ to see {d} {text}"
		}
	}
	out := Packet(v)
	if strings.Contains(out, "\nCAUTION: nothing") || strings.ContainsRune(out, '‮') || !strings.Contains(out, "{d} {text}") {
		t.Fatalf("unchecked text not neutralised:\n%s", out)
	}
}
