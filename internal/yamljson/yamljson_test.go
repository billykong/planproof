package yamljson

import (
	"strings"
	"testing"
)

func TestConvert(t *testing.T) {
	got, err := Convert([]byte(`
b: [1, -2, 0x10]
a: {s: text, t: true, n: null, q: "3"}
`))
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"a":{"n":null,"q":"3","s":"text","t":true},"b":[1,-2,16]}`; string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

// TestRejected: every YAML feature that could make a file mean something
// other than what its reader saw is an error (T-03).
func TestRejected(t *testing.T) {
	cases := map[string]string{
		"anchor and alias": "a: &x 1\nb: *x\n",
		"merge key":        "base: &b {replicas: 1}\nstep:\n  <<: *b\n",
		"duplicate key":    "replicas: 1\nreplicas: 50\n",
		"float":            "replicas: 1.5\n",
		"timestamp":        "ticket: 2026-10-14\n",
		"custom tag":       "image: !secret x\n",
		"binary tag":       "image: !!binary aGk=\n",
		"non-string key":   "1: one\n",
		"two documents":    "a: 1\n---\nb: 2\n",
		"empty":            "",
		"too deep":         strings.Repeat("[", 40) + strings.Repeat("]", 40),
		"too large":        "a: " + strings.Repeat("x", MaxBytes),
		"huge integer":     "replicas: 99999999999999999999\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if out, err := Convert([]byte(in)); err == nil {
				t.Fatalf("accepted, gave %s", out)
			}
		})
	}
}

// TestYAML11Words: words YAML 1.1 read as booleans stay strings, so a node
// named "no" stays "no" (the "Norway problem").
func TestYAML11Words(t *testing.T) {
	got, err := Convert([]byte("node: no\nflag: on\nok: yes\n"))
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"flag":"on","node":"no","ok":"yes"}`; string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
