package executor

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"
	"time"
)

// The translation check (C-17): after a run, the API audit log's record of
// the executor's writes must match what Compile produced, exactly. It
// detects what RBAC can't prevent (GAP-01): a write the plan didn't call
// for, or a patch that touched other fields.

// AuditEvent is the part of a Kubernetes audit event (audit.k8s.io/v1) the
// check reads.
type AuditEvent struct {
	Stage            string                     `json:"stage"`
	Verb             string                     `json:"verb"`
	User             struct{ Username string }  `json:"user"`
	ImpersonatedUser *struct{ Username string } `json:"impersonatedUser"`
	ObjectRef        *struct {
		Resource, Subresource, Namespace, Name string
	} `json:"objectRef"`
	ResponseStatus *struct{ Code int } `json:"responseStatus"`
	RequestObject  json.RawMessage     `json:"requestObject"`
	Received       time.Time           `json:"requestReceivedTimestamp"`
}

// ReadAudit reads JSON-lines audit events.
func ReadAudit(r io.Reader) ([]AuditEvent, error) {
	var out []AuditEvent
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var e AuditEvent
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

// identity is who made the request, after impersonation.
func (e AuditEvent) identity() string {
	if e.ImpersonatedUser != nil {
		return e.ImpersonatedUser.Username
	}
	return e.User.Username
}

// CheckTranslation compares the successful writes user made from since on
// with the compiled calls and the run's report. It returns every mismatch;
// none means the run did exactly what the plan's translation says.
func CheckTranslation(calls []Call, rep Report, events []AuditEvent, user string, since time.Time) []string {
	type write struct{ verb, resource, ns, name string }
	key := func(w write) string { return fmt.Sprintf("%s %s %s/%s", w.verb, w.resource, w.ns, w.name) }

	// What the run should have written: every patch Compile made, for the
	// steps that ran (several may touch one object, so each keeps its own
	// body), and every eviction the report says the drain made.
	expected := map[string][]string{} // write -> compiled bodies not yet seen ("" for evictions)
	ran := map[int]bool{}
	for _, d := range rep.Done {
		ran[d.Step] = true
		if d.Resource == "pods/eviction" {
			k := key(write{"create", "pods/eviction", d.Namespace, d.Name})
			expected[k] = append(expected[k], "")
		}
	}
	for _, c := range calls {
		if c.Verb == "patch" && ran[c.Step] {
			k := key(write{"patch", c.Resource, c.Namespace, c.Name})
			expected[k] = append(expected[k], c.Patch)
		}
	}

	var problems []string
	for _, e := range events {
		if e.Stage != "ResponseComplete" || e.identity() != user || e.Received.Before(since) || e.ObjectRef == nil ||
			e.ResponseStatus == nil || e.ResponseStatus.Code/100 != 2 {
			continue // only the executor's completed, successful requests in this run
		}
		switch e.Verb {
		case "get", "list", "watch":
			continue
		}
		resource := e.ObjectRef.Resource
		if e.ObjectRef.Subresource != "" {
			resource += "/" + e.ObjectRef.Subresource
		}
		k := key(write{e.Verb, resource, e.ObjectRef.Namespace, e.ObjectRef.Name})
		i := slices.IndexFunc(expected[k], func(body string) bool { return body == "" || samePatch(e.RequestObject, body) })
		if i < 0 {
			if len(expected[k]) == 0 {
				problems = append(problems, "a write the plan didn't call for: "+k)
			} else {
				problems = append(problems, fmt.Sprintf("%s sent %s, which no compiled step sends", k, e.RequestObject))
			}
			continue
		}
		expected[k] = slices.Delete(expected[k], i, i+1)
	}
	for _, k := range slices.Sorted(maps.Keys(expected)) {
		for range expected[k] {
			problems = append(problems, "a compiled write that isn't in the audit log: "+k)
		}
	}
	return problems
}

// samePatch compares a sent patch with the compiled one, ignoring the
// resourceVersion precondition the runner adds.
func samePatch(sent json.RawMessage, compiled string) bool {
	var a, b map[string]any
	if json.Unmarshal(sent, &a) != nil || json.Unmarshal([]byte(compiled), &b) != nil {
		return false
	}
	if md, ok := a["metadata"].(map[string]any); ok {
		delete(md, "resourceVersion")
		if len(md) == 0 {
			delete(a, "metadata")
		}
	}
	return reflect.DeepEqual(a, b)
}
