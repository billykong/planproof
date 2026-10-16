package render

import (
	"bufio"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/billykong/planproof/internal/testcases"
	"github.com/billykong/planproof/kernel"
)

// The writing rules of spec 06, checked on every template (R-05).

// banned are the W9 words: they claim more than the kernel shows.
var banned = []string{"safe", "safely", "guaranteed", "verified", "ensure", "probably", "should", "simply", "just", "obviously", "fine"}

// notVerbs end in "-ing" but aren't verb forms (W5).
var notVerbs = []string{"bring", "during", "nothing"}

// samples fill the placeholders for the lint. Object names are code, so
// one sample word stands for any of them.
var samples = map[string]string{
	"n": "`x`", "d": "`x`", "c": "`x`", "p": "`x`", "object": "`x`", "image": "`x`", "field": "`x`", "key": "`x`",
	"reason": "`x`", "path": "`x`", "list": "`x` and `y`", "from": "2", "to": "3", "actual": "2", "expected": "3",
	"i": "1", "k": "2", "x": "2", "lim": "2", "need": "2", "cpu": "1400", "mem": "5568", "time": "5 minutes",
	"step": "step 4", "Step": "Step 4", "When": "During step 4", "phase": "during", "where": "at step 4",
	"steps": "steps 3 and 5", "title": "Limits", "claim": "node `x` accepts new pods", "text": `"…"`, "input": "plan",
	"name": "x", "plan": "x", "snapshot": "x", "policy": "x", "version": "x",
}

func sampleFill(t *testing.T, key string) string {
	kv := []string{}
	for _, m := range placeholder.FindAllStringSubmatch(templates[key], -1) {
		s, ok := samples[m[1]]
		if !ok {
			t.Fatalf("%s: no sample for {%s}", key, m[1])
		}
		kv = append(kv, m[1], s)
	}
	return fill(key, kv...)
}

var (
	codeSpan = regexp.MustCompile("`[^`]*`")
	wordRe   = regexp.MustCompile(`[A-Za-z]+`)
	passive  = regexp.MustCompile(`(?i)\b(is|are|was|were|be|been|being)\s+[a-z]+ed\b`)
)

// words returns the words of a sentence, lower case, without code spans
// and numbers.
func words(s string) []string {
	var out []string
	for _, w := range wordRe.FindAllString(codeSpan.ReplaceAllString(s, " "), -1) {
		out = append(out, strings.ToLower(w))
	}
	return out
}

func wordList(t *testing.T) map[string]bool {
	f, err := os.Open("words.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	list := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if w := strings.TrimSpace(sc.Text()); w != "" && !strings.HasPrefix(w, "#") {
			list[w] = true
		}
	}
	return list
}

func TestLintTemplates(t *testing.T) {
	list, used := wordList(t), map[string]bool{}
	keys := make([]string, 0, len(templates))
	for k := range templates {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if key == "head.title" || key == "head.hashes" {
			continue // identifiers, not prose
		}
		text := sampleFill(t, key)
		// Sentences end at ". " or the end; a line break also ends one.
		for _, s := range regexp.MustCompile(`[.:]\s+|\n`).Split(text, -1) {
			ws := words(s)
			max := 25
			if strings.HasPrefix(key, "step.") || strings.HasPrefix(key, "goal.") {
				max = 20 // instructions (W2)
			}
			if n := len(strings.Fields(s)); n > max {
				t.Errorf("%s: %d words, more than %d (W2): %q", key, n, max, s)
			}
			if passive.MatchString(s) {
				t.Errorf("%s: passive voice (W3): %q", key, s)
			}
			for _, w := range ws {
				used[w] = true
				switch {
				case slices.Contains(banned, w):
					t.Errorf("%s: banned word %q (W9)", key, w)
				case strings.HasSuffix(w, "ing") && !slices.Contains(notVerbs, w):
					t.Errorf("%s: %q looks like an -ing verb form (W5)", key, w)
				case !list[w]:
					t.Errorf("%s: %q is not in words.txt (W4)", key, w)
				}
			}
		}
	}
	for w := range list {
		if !used[w] {
			t.Errorf("words.txt: %q is in no template; remove it", w)
		}
	}
}

// allVerdicts is every verdict the tests know: the worked examples, every
// case in internal/testcases, and an input failure.
func allVerdicts(t *testing.T) []kernel.Verdict {
	vs := []kernel.Verdict{}
	for _, n := range examples {
		vs = append(vs, verdict(t, n))
	}
	for _, c := range testcases.Cases {
		vs = append(vs, kernel.Check(testcases.Inputs(c)))
	}
	snap, _, pol := testcases.Inputs(testcases.Cases[0])
	for _, bad := range []string{`{"apiVersion":"planproof/v0","force":true}`, `{`} {
		vs = append(vs, kernel.Check(snap, []byte(bad), pol))
	}
	return vs
}

// TestEveryTemplateUsed: each template is reached by some verdict, so the
// lint and the golden tests cover what a person can actually read.
func TestEveryTemplateUsed(t *testing.T) {
	used := map[string]bool{}
	trace = func(k string) { used[k] = true }
	defer func() { trace = nil }()
	for _, v := range allVerdicts(t) {
		Packet(v)
	}
	var missing []string
	for k := range templates {
		if !used[k] && !strings.HasPrefix(k, "title.") {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("templates no test reaches: %v", missing)
	}
}
