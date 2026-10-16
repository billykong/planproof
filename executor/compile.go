// Package executor runs a Proved plan against the cluster (PRD area C):
// it admits only a plan the kernel proves, stops if the cluster has drifted
// from the snapshot, checks each step's assertions live, writes with
// resourceVersion preconditions, drains only through the Eviction API, and
// never rolls back on its own.
//
// The executor is trusted, and it holds the only write identity. Its
// translation from plan to API calls is a fixed, pure function (Compile),
// so what ran can be checked against the plan afterwards (C-17).
package executor

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/billykong/planproof/kernel"
)

// Call is one API call a step makes. Patches are strategic merge patches;
// the runner adds the object's current resourceVersion as a precondition
// just before it sends them. A drain is one call that the runner expands
// into one eviction per pod found on the node at run time.
type Call struct {
	Step           int      `json:"step"`
	Verb           string   `json:"verb"` // "patch", "evict-pods-on-node", "wait"
	Resource       string   `json:"resource,omitempty"`
	Namespace      string   `json:"namespace,omitempty"`
	Name           string   `json:"name,omitempty"`
	Patch          string   `json:"patch,omitempty"`
	Deployments    []string `json:"deployments,omitempty"`
	TimeoutSeconds int64    `json:"timeoutSeconds,omitempty"`
}

// RestartAnnotation is the template annotation a rollout-restart sets. Its
// value names the plan and step instead of a time, so Compile stays pure.
const RestartAnnotation = "planproof.io/restarted-by"

// Compile turns a plan's steps into API calls (C-11). planHash names the
// plan in restart annotations.
func Compile(steps []kernel.Step, planHash string) ([]Call, error) {
	var calls []Call
	for i, s := range steps {
		n := i + 1
		ns, name, _ := strings.Cut(s.Deployment, "/")
		patch := func(v any) string { b, _ := json.Marshal(v); return string(b) }
		switch s.Op {
		case "cordon", "uncordon":
			calls = append(calls, Call{Step: n, Verb: "patch", Resource: "nodes", Name: s.Node,
				Patch: patch(map[string]any{"spec": map[string]any{"unschedulable": s.Op == "cordon"}})})
		case "drain":
			calls = append(calls, Call{Step: n, Verb: "evict-pods-on-node", Resource: "pods", Name: s.Node})
		case "scale":
			calls = append(calls, Call{Step: n, Verb: "patch", Resource: "deployments", Namespace: ns, Name: name,
				Patch: patch(map[string]any{"spec": map[string]any{"replicas": *s.Replicas}})})
		case "set-image":
			calls = append(calls, Call{Step: n, Verb: "patch", Resource: "deployments", Namespace: ns, Name: name,
				Patch: patch(map[string]any{"spec": map[string]any{"template": map[string]any{"spec": map[string]any{
					"containers": []any{map[string]any{"name": s.Container, "image": s.Image}}}}}})})
		case "rollout-restart":
			calls = append(calls, Call{Step: n, Verb: "patch", Resource: "deployments", Namespace: ns, Name: name,
				Patch: patch(map[string]any{"spec": map[string]any{"template": map[string]any{"metadata": map[string]any{
					"annotations": map[string]any{RestartAnnotation: restartValue(planHash, n)}}}}})})
		case "wait":
			calls = append(calls, Call{Step: n, Verb: "wait", Resource: "deployments", Deployments: s.Deployments, TimeoutSeconds: *s.TimeoutSeconds})
		default:
			return nil, fmt.Errorf("step %d: no translation for op %q", n, s.Op)
		}
	}
	return calls, nil
}

func restartValue(planHash string, step int) string {
	h := strings.TrimPrefix(planHash, "sha256:")
	return fmt.Sprintf("%s-step-%d", h[:min(12, len(h))], step)
}
