package study

import (
	"strings"
	"testing"

	"github.com/billykong/planproof/evals"
)

// TestItems checks the answer key: a good item is proved and its goal is the
// ticket's; a bad one is refuted, or proved with a goal that isn't the
// ticket's. Nothing a reviewer sees names the item's source.
func TestItems(t *testing.T) {
	items, err := Build(evals.Root())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 12 {
		t.Fatalf("%d items, want 12", len(items))
	}
	good, wrongGoal := 0, 0
	for _, it := range items {
		switch it.Answer {
		case "approve":
			good++
			if it.Kernel != "Proved" || !it.Goal {
				t.Errorf("%s (%s): answer approve, but kernel %s, goal matches ticket %v", it.ID, it.Source, it.Kernel, it.Goal)
			}
		case "reject":
			if it.Kernel == "Proved" && it.Goal {
				t.Errorf("%s (%s): answer reject, but the kernel proves it and the goal is the ticket's", it.ID, it.Source)
			}
			if it.Kernel == "Proved" {
				wrongGoal++
			}
		default:
			t.Errorf("%s: answer %q", it.ID, it.Answer)
		}
		_, name, _ := strings.Cut(it.Source, ":")
		for _, shown := range []string{it.Raw, it.Packet, it.Ticket} {
			if strings.Contains(shown, name) && !strings.Contains(it.Ticket, name) {
				t.Errorf("%s: what the reviewer sees names the source %q", it.ID, name)
			}
		}
	}
	if good != 6 || wrongGoal != 3 {
		t.Errorf("%d good items and %d proved with the wrong goal, want 6 and 3", good, wrongGoal)
	}
}

func TestParseDecision(t *testing.T) {
	for _, c := range []struct{ text, want string }{
		{"Looks fine.\n{\"decision\": \"approve\", \"reason\": \"ok\"}", "approve"},
		{"```\n{\"decision\": \"reject\", \"reason\": \"no wait\"}\n```", "reject"},
		{"{\"decision\": \"maybe\"}", ""},
		{"no json at all", ""},
	} {
		got, _, err := ParseDecision(c.text)
		if got != c.want || (c.want == "") != (err != nil) {
			t.Errorf("ParseDecision(%q) = %q, %v; want %q", c.text, got, err, c.want)
		}
	}
}
