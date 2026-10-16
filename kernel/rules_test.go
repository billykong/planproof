package kernel

import (
	"slices"
	"testing"

	"github.com/billykong/planproof/internal/testcases"
)

// Tests of the rules (K-21), Unknown triggers (K-22) and operations (K-20),
// as small variations on the drain-two-nodes example cluster:
//
//	n1: checkout-a, cart-a     n3: checkout-c, payments-a
//	n2: checkout-b, cart-b     n4: payments-b
//
// Budgets: checkout 2, cart 1, payments 1. payments is protected.

func TestRules(t *testing.T) {
	for _, c := range testcases.Cases {
		t.Run(c.Name, func(t *testing.T) {
			v := Check(testcases.Inputs(c))
			var rules []string
			trigger := ""
			for _, f := range v.Facts {
				switch f.Type {
				case "failure":
					rules = append(rules, f.Rule)
					if f.Rule == "input" {
						t.Fatalf("input rejected: %v", f.Evidence)
					}
				case "unknown":
					trigger = f.Trigger
				}
			}
			if string(v.Result) != c.Want || c.Rule != "" && !slices.Contains(rules, c.Rule) || trigger != c.Trigger {
				t.Fatalf("got %s, failures %v, trigger %q; want %s %s%s", v.Result, rules, trigger, c.Want, c.Rule, c.Trigger)
			}
		})
	}
}
