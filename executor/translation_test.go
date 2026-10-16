package executor

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func clone(s []string) []string { return append([]string{}, s...) }

func TestCheckTranslation(t *testing.T) {
	const exe = "system:serviceaccount:planproof:executor"
	t0 := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	calls := []Call{
		{Step: 1, Verb: "patch", Resource: "nodes", Name: "n1", Patch: `{"spec":{"unschedulable":true}}`},
		{Step: 2, Verb: "evict-pods-on-node", Resource: "pods", Name: "n1"},
		{Step: 3, Verb: "patch", Resource: "deployments", Namespace: "shop", Name: "web", Patch: `{"spec":{"replicas":4}}`},
	}
	rep := Report{Result: "done", Done: []Done{
		{1, "patch", "nodes", "", "n1"}, {2, "create", "pods/eviction", "shop", "web-1"}, {3, "patch", "deployments", "shop", "web"},
	}}
	ev := func(user, verb, resource, sub, ns, name string, code int, body string, at time.Time) string {
		return `{"stage":"ResponseComplete","verb":"` + verb + `","user":{"username":"` + user + `"},"objectRef":{"resource":"` + resource +
			`","subresource":"` + sub + `","namespace":"` + ns + `","name":"` + name + `"},"responseStatus":{"code":` + strconv.Itoa(code) +
			`},"requestObject":` + body + `,"requestReceivedTimestamp":"` + at.Format(time.RFC3339Nano) + `"}`
	}
	good := []string{
		ev(exe, "patch", "nodes", "", "", "n1", 200, `{"metadata":{"resourceVersion":"5"},"spec":{"unschedulable":true}}`, t0),
		ev(exe, "create", "pods", "eviction", "shop", "web-1", 429, `{}`, t0), // refused by a budget, retried: ignored
		ev(exe, "create", "pods", "eviction", "shop", "web-1", 201, `{}`, t0),
		ev(exe, "patch", "deployments", "", "shop", "web", 200, `{"metadata":{"resourceVersion":"9"},"spec":{"replicas":4}}`, t0),
		ev("someone-else", "delete", "pods", "", "shop", "x", 200, `{}`, t0),                   // another identity
		ev(exe, "patch", "nodes", "", "", "n9", 200, `{}`, t0.Add(-time.Hour)),                 // before the run
		ev(exe, "patch", "deployments", "", "shop", "web", 409, `{"spec":{"replicas":4}}`, t0), // failed
	}
	check := func(lines []string) []string {
		events, err := ReadAudit(strings.NewReader(strings.Join(lines, "\n")))
		if err != nil {
			t.Fatal(err)
		}
		return CheckTranslation(calls, rep, events, exe, t0.Add(-time.Second))
	}
	if p := check(good); len(p) != 0 {
		t.Fatalf("a matching run: %v", p)
	}
	for name, c := range map[string]struct {
		lines []string
		want  string
	}{
		"extra write":    {append(clone(good), ev(exe, "patch", "nodes", "", "", "n2", 200, `{"spec":{"unschedulable":true}}`, t0)), "didn't call for"},
		"other fields":   {append(clone(good[:3]), ev(exe, "patch", "deployments", "", "shop", "web", 200, `{"spec":{"replicas":4,"paused":true}}`, t0)), "which no compiled step sends"},
		"missing write":  {clone(good[:3]), "isn't in the audit log"},
		"extra eviction": {append(clone(good), ev(exe, "create", "pods", "eviction", "shop", "db-0", 201, `{}`, t0)), "didn't call for"},
	} {
		t.Run(name, func(t *testing.T) {
			p := check(c.lines)
			if len(p) == 0 || !strings.Contains(strings.Join(p, "; "), c.want) {
				t.Fatalf("got %v, want %q", p, c.want)
			}
		})
	}
}
