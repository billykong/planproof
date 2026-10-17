// Package study builds the reviewer study's items (PRD E-22): each plan
// shown two ways, as the raw plan with the cluster facts a reviewer would
// otherwise look up, or as planproof's review packet. Both are generated
// from the same task, snapshot, policy and plan, so they differ only in
// format.
package study

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/billykong/planproof/agent"
	"github.com/billykong/planproof/evals"
	"github.com/billykong/planproof/internal/yamljson"
	"github.com/billykong/planproof/kernel"
	"github.com/billykong/planproof/render"
)

// Entry is one line of study/items.yaml.
type Entry struct {
	ID        string `json:"id"`
	Reference string `json:"reference"` // a task whose reference plan is the item
	Seeded    string `json:"seeded"`    // or a seeded bad plan
	Answer    string `json:"answer"`    // approve or reject
	Why       string `json:"why"`       // shown after the reviewer decides
}

// Item is what a reviewer sees, in either format, and the answer.
type Item struct {
	ID     string `json:"id"`
	Ticket string `json:"ticket"`
	Raw    string `json:"raw"`
	Packet string `json:"packet"`
	Answer string `json:"answer"`
	Why    string `json:"why"`
	// Not shown to reviewers: where the item came from and the kernel's verdict.
	Source string `json:"source"`
	Kernel string `json:"kernel"`
	Goal   bool   `json:"goalMatchesTicket"`
}

// Build reads study/items.yaml and renders every item.
func Build(root string) ([]Item, error) {
	b, err := os.ReadFile(filepath.Join(root, "study", "items.yaml"))
	if err != nil {
		return nil, err
	}
	j, err := yamljson.Convert(b)
	if err != nil {
		return nil, fmt.Errorf("items.yaml: %w", err)
	}
	var entries []Entry
	if err := json.Unmarshal(j, &entries); err != nil {
		return nil, err
	}
	specs, err := evals.Load(root)
	if err != nil {
		return nil, err
	}
	seeded, err := evals.LoadSeeded(root)
	if err != nil {
		return nil, err
	}
	var items []Item
	for _, e := range entries {
		var spec evals.Spec
		var body json.RawMessage
		source := ""
		switch {
		case e.Reference != "":
			i := slices.IndexFunc(specs, func(s evals.Spec) bool { return s.ID == e.Reference })
			if i < 0 || specs[i].Reference == nil {
				return nil, fmt.Errorf("%s: no task %q with a reference plan", e.ID, e.Reference)
			}
			spec, body, source = specs[i], specs[i].Reference, "reference:"+e.Reference
		case e.Seeded != "":
			i := slices.IndexFunc(seeded, func(s evals.Seeded) bool { return s.ID == e.Seeded })
			if i < 0 {
				return nil, fmt.Errorf("%s: no seeded plan %q", e.ID, e.Seeded)
			}
			j := slices.IndexFunc(specs, func(s evals.Spec) bool { return s.ID == seeded[i].Task })
			if j < 0 {
				return nil, fmt.Errorf("%s: no task %q", e.ID, seeded[i].Task)
			}
			spec, body, source = specs[j], seeded[i].Plan, "seeded:"+e.Seeded
		default:
			return nil, fmt.Errorf("%s: names no plan", e.ID)
		}
		task, err := spec.Task(root)
		if err != nil {
			return nil, err
		}
		// The plan's name is the item's neutral ID, never the source's.
		task.ID = e.ID
		v := kernel.Check(task.Snapshot, agent.Plan(task, body), task.Policy)
		policyYAML, err := os.ReadFile(filepath.Join(root, spec.Policy))
		if err != nil {
			return nil, err
		}
		raw, err := Raw(task, policyYAML, body)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.ID, err)
		}
		items = append(items, Item{
			ID: e.ID, Ticket: spec.Ticket, Raw: raw, Packet: render.Packet(v), Answer: e.Answer, Why: e.Why,
			Source: source, Kernel: string(v.Result), Goal: evals.SameGoal(body, spec.Expect.Goal),
		})
	}
	return items, nil
}

// Raw is the plan as a reviewer without planproof would see it: the plan as
// the agent wrote it, the policy file, and the snapshot's facts as tables
// (generous to this format: a reviewer would otherwise query the cluster).
func Raw(t agent.Task, policyYAML []byte, plan json.RawMessage) (string, error) {
	var snap kernel.Snapshot
	if err := json.Unmarshal(t.Snapshot, &snap); err != nil {
		return "", err
	}
	var b strings.Builder
	p := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	p("CLUSTER (snapshot taken %s)\n\nNodes:\n", snap.FetchedAt)
	for _, n := range snap.Nodes {
		state := "schedulable"
		if n.Unschedulable != nil && *n.Unschedulable {
			state = "cordoned"
		}
		var pods []string
		for _, pod := range snap.Pods {
			if pod.Node == n.Name && pod.Namespace != "kube-system" {
				pods = append(pods, pod.Namespace+"/"+pod.Name)
			}
		}
		p("  %s: %dm CPU, %d MiB, %s; pods: %s\n", n.Name, val(n.Allocatable.CPUMilli), val(n.Allocatable.MemoryMiB), state, strings.Join(pods, ", "))
	}
	p("\nDeployments:\n")
	for _, d := range snap.Deployments {
		images := []string{}
		for _, c := range d.Containers {
			images = append(images, c.Name+"="+c.Image)
		}
		p("  %s/%s: %d replicas, %d ready; each pod requests %dm CPU, %d MiB; rollout maxUnavailable %d, maxSurge %d", d.Namespace, d.Name,
			val(d.Replicas), val(d.ReadyReplicas), val(d.PodRequests.CPUMilli), val(d.PodRequests.MemoryMiB), val(d.Strategy.MaxUnavailable), val(d.Strategy.MaxSurge))
		if d.HPA != nil && *d.HPA {
			p("; autoscaled (HPA)")
		}
		p("\n    image: %s\n", strings.Join(images, ", "))
	}
	if len(snap.PDBs) > 0 {
		p("\nDisruption budgets:\n")
		for _, d := range snap.PDBs {
			if d.MinAvailable != nil {
				p("  %s/%s: minAvailable %d\n", d.Namespace, d.Name, *d.MinAvailable)
			} else if d.MaxUnavailable != nil {
				p("  %s/%s: maxUnavailable %d\n", d.Namespace, d.Name, *d.MaxUnavailable)
			}
		}
	}
	p("\nPOLICY (policy file)\n\n%s\n", strings.TrimSpace(string(policyYAML)))
	var m yaml.Node
	if err := yaml.Unmarshal(plan, &m); err != nil {
		return "", err
	}
	block(&m)
	var out strings.Builder
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&m); err != nil {
		return "", err
	}
	p("\nPLAN (as the agent wrote it)\n\n%s", out.String())
	return b.String(), nil
}

// block makes the plan read as YAML a person writes, not as JSON: no
// quotes where none are needed, block style for the lists of steps and goal
// items, flow style for each small item.
func block(n *yaml.Node) {
	n.Style = 0
	if n.Kind == yaml.MappingNode {
		// Keys in the order people write them (the JSON round trip sorted them).
		order := []string{"op", "node", "deployment", "deployments", "container", "field", "replicas", "image", "from", "to", "timeoutSeconds"}
		rank := func(k string) int {
			if i := slices.Index(order, k); i >= 0 {
				return i
			}
			return len(order)
		}
		pairs := make([][2]*yaml.Node, 0, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			pairs = append(pairs, [2]*yaml.Node{n.Content[i], n.Content[i+1]})
		}
		slices.SortStableFunc(pairs, func(a, b [2]*yaml.Node) int { return rank(a[0].Value) - rank(b[0].Value) })
		n.Content = n.Content[:0]
		for _, kv := range pairs {
			n.Content = append(n.Content, kv[0], kv[1])
		}
	}
	for _, c := range n.Content {
		block(c)
		if c.Kind == yaml.MappingNode && n.Kind == yaml.SequenceNode {
			c.Style = yaml.FlowStyle
		}
	}
}

func val(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
