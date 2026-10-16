package kernel

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Validation of each input's shape: required fields, enums, names, ranges
// and uniqueness (K-10). What a plan means for the cluster is the rules'
// job, not this file's: a goal whose from differs from the snapshot parses
// fine and fails rule frame.

const apiVersion = "planproof/v0"

// validator records the first failure only, so results are deterministic.
type validator struct {
	input string
	err   *inputError
}

func (v *validator) fail(reason, path string) {
	if v.err == nil {
		v.err = &inputError{v.input, reason, path}
	}
}

func (v *validator) need(ok bool, path string) {
	if !ok {
		v.fail(reasonMissing, path)
	}
}

func (v *validator) check(ok bool, path string) {
	if !ok {
		v.fail(reasonBadValue, path)
	}
}

func (v *validator) header(gotVersion, gotKind, kind string) {
	if gotVersion != apiVersion || gotKind != kind {
		v.fail(reasonWrongVersion, "apiVersion")
	}
}

func (v *validator) nonNeg(n *int64, path string) {
	v.need(n != nil, path)
	if n != nil {
		v.check(*n >= 0, path)
	}
}

func (v *validator) resources(r Resources, path string) {
	v.nonNeg(r.CPUMilli, path+".cpuMilli")
	v.nonNeg(r.MemoryMiB, path+".memoryMiB")
	v.nonNeg(r.Pods, path+".pods")
}

// unique fails on the first repeated key.
func (v *validator) unique(keys []string, path string) {
	seen := map[string]bool{}
	for i, k := range keys {
		if seen[k] {
			v.fail(reasonDuplicate, fmt.Sprintf("%s[%d]", path, i))
		}
		seen[k] = true
	}
}

// Kubernetes names: a DNS-1123 label (namespaces, containers, plan names)
// or subdomain (nodes, deployments).
func isLabel(s string) bool {
	if len(s) == 0 || len(s) > 63 || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	return !strings.ContainsFunc(s, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') })
}

func isSubdomain(s string) bool {
	if len(s) > 253 {
		return false
	}
	for _, part := range strings.Split(s, ".") {
		if !isLabel(part) {
			return false
		}
	}
	return true
}

// isDeploymentRef checks "namespace/name".
func isDeploymentRef(s string) bool {
	ns, name, ok := strings.Cut(s, "/")
	return ok && isLabel(ns) && isSubdomain(name)
}

func isImage(s string) bool {
	return s != "" && len(s) <= 512 && !strings.ContainsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
}

func isHash(s string) bool {
	h, ok := strings.CutPrefix(s, "sha256:")
	return ok && len(h) == 64 && !strings.ContainsFunc(h, func(r rune) bool { return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') })
}

func (v *validator) text(s *string, max int, path string) {
	if s != nil {
		v.check(utf8.RuneCountInString(*s) <= max, path)
	}
}

// Goal fields: the object they belong to and the type of from and to (spec 01).
var goalFields = map[string]struct {
	deployment bool
	kind       string
}{
	"replicas":      {true, "int"},
	"image":         {true, "string"},
	"restarted":     {true, "bool"},
	"unschedulable": {false, "bool"},
	"drained":       {false, "bool"},
}

// Step parameters each operation needs; every other parameter must be absent.
var opParams = map[string][]string{
	"scale":           {"deployment", "replicas"},
	"set-image":       {"deployment", "container", "image"},
	"rollout-restart": {"deployment"},
	"cordon":          {"node"},
	"uncordon":        {"node"},
	"drain":           {"node"},
	"wait":            {"deployments", "timeoutSeconds"},
}

const maxSteps = 50

func validatePlan(p *Plan) *inputError {
	v := &validator{input: "plan"}
	v.header(p.APIVersion, p.Kind, "Plan")
	v.check(isLabel(p.Metadata.Name), "metadata.name")
	v.text(p.Metadata.Ticket, 200, "metadata.ticket")
	v.text(p.Rationale, 1000, "rationale")
	v.check(isHash(p.Snapshot), "snapshot")
	v.need(len(p.Goal) > 0, "goal")
	for i, c := range p.Goal {
		path := fmt.Sprintf("goal[%d]", i)
		f, known := goalFields[c.Field]
		v.check(known, path+".field")
		if f.deployment {
			v.check(isDeploymentRef(c.Deployment) && c.Node == "", path+".deployment")
		} else {
			v.check(isSubdomain(c.Node) && c.Deployment == "", path+".node")
		}
		if c.Field == "image" {
			v.check(isLabel(c.Container), path+".container")
		} else {
			v.check(c.Container == "", path+".container")
		}
		v.check(c.From.Kind == f.kind, path+".from")
		v.check(c.To.Kind == f.kind, path+".to")
	}
	v.need(len(p.Steps) > 0, "steps")
	v.check(len(p.Steps) <= maxSteps && len(p.Rollback) <= maxSteps, "steps")
	for i := range p.Steps {
		v.step(&p.Steps[i], fmt.Sprintf("steps[%d]", i))
	}
	for i := range p.Rollback {
		v.step(&p.Rollback[i], fmt.Sprintf("rollback[%d]", i))
	}
	return v.err
}

func (v *validator) step(s *Step, path string) {
	want, known := opParams[s.Op]
	if !known {
		v.fail(reasonBadValue, path+".op")
		return
	}
	present := map[string]bool{
		"deployment":     s.Deployment != "",
		"node":           s.Node != "",
		"replicas":       s.Replicas != nil,
		"container":      s.Container != "",
		"image":          s.Image != "",
		"deployments":    s.Deployments != nil,
		"timeoutSeconds": s.TimeoutSeconds != nil,
	}
	for _, name := range []string{"deployment", "node", "replicas", "container", "image", "deployments", "timeoutSeconds"} {
		if slices.Contains(want, name) {
			v.need(present[name], path+"."+name)
		} else {
			v.check(!present[name], path+"."+name)
		}
	}
	if s.Deployment != "" {
		v.check(isDeploymentRef(s.Deployment), path+".deployment")
	}
	if s.Node != "" {
		v.check(isSubdomain(s.Node), path+".node")
	}
	if s.Replicas != nil {
		v.check(*s.Replicas >= 0, path+".replicas")
	}
	if s.Container != "" {
		v.check(isLabel(s.Container), path+".container")
	}
	if s.Image != "" {
		v.check(isImage(s.Image), path+".image")
	}
	if s.Deployments != nil {
		v.need(len(s.Deployments) > 0, path+".deployments")
		for i, d := range s.Deployments {
			v.check(isDeploymentRef(d), fmt.Sprintf("%s.deployments[%d]", path, i))
		}
		v.unique(s.Deployments, path+".deployments")
	}
	if s.TimeoutSeconds != nil {
		v.check(*s.TimeoutSeconds >= 1, path+".timeoutSeconds")
	}
	v.assertions(s.Pre, s, path+".pre")
	v.assertions(s.Post, s, path+".post")
}

// assertions may only name keys that apply to the step's object (spec 01).
func (v *validator) assertions(a *Assertions, s *Step, path string) {
	if a == nil {
		return
	}
	onDeployment, onNode := s.Deployment != "", s.Node != ""
	v.check(onDeployment || onNode, path)
	v.check(onDeployment || a.Replicas == nil && a.Image == nil && a.Container == "", path)
	v.check(onNode || a.Unschedulable == nil && a.PodsOnNode == nil, path)
	if a.Image != nil {
		v.check(isImage(*a.Image) && (isLabel(a.Container) || a.Container == "" && s.Container != ""), path+".image")
	} else {
		v.check(a.Container == "", path+".container")
	}
	for _, n := range []*int64{a.Replicas, a.PodsOnNode} {
		v.check(n == nil || *n >= 0, path)
	}
}

func validateSnapshot(s *Snapshot) *inputError {
	v := &validator{input: "snapshot"}
	v.header(s.APIVersion, s.Kind, "Snapshot")
	v.need(s.Cluster != "", "cluster")
	v.check(len(s.Cluster) <= 253 && len(s.FetchedAt) <= 64, "cluster")
	var names []string
	nodes := map[string]bool{}
	for i, n := range s.Nodes {
		path := fmt.Sprintf("nodes[%d]", i)
		v.check(isSubdomain(n.Name), path+".name")
		v.need(n.Ready != nil && n.Unschedulable != nil && n.Taints != nil, path)
		v.resources(n.Allocatable, path+".allocatable")
		names = append(names, n.Name)
		nodes[n.Name] = true
	}
	v.unique(names, "nodes")
	names = nil
	for i, d := range s.Deployments {
		path := fmt.Sprintf("deployments[%d]", i)
		v.check(isLabel(d.Namespace), path+".namespace")
		v.check(isSubdomain(d.Name), path+".name")
		v.nonNeg(d.Replicas, path+".replicas")
		v.nonNeg(d.ReadyReplicas, path+".readyReplicas")
		v.nonNeg(d.UpdatedReplicas, path+".updatedReplicas")
		v.check(d.Strategy.Type == "RollingUpdate" || d.Strategy.Type == "Recreate", path+".strategy.type")
		v.nonNeg(d.Strategy.MaxUnavailable, path+".strategy.maxUnavailable")
		v.nonNeg(d.Strategy.MaxSurge, path+".strategy.maxSurge")
		v.need(len(d.Containers) > 0, path+".containers")
		var containers []string
		for j, c := range d.Containers {
			v.check(isLabel(c.Name) && isImage(c.Image), fmt.Sprintf("%s.containers[%d]", path, j))
			containers = append(containers, c.Name)
		}
		v.unique(containers, path+".containers")
		v.resources(d.PodRequests, path+".podRequests")
		v.need(d.PlacementRules != nil && d.HPA != nil, path)
		names = append(names, d.Namespace+"/"+d.Name)
	}
	v.unique(names, "deployments")
	names = nil
	for i, p := range s.Pods {
		path := fmt.Sprintf("pods[%d]", i)
		v.check(isLabel(p.Namespace), path+".namespace")
		v.check(isSubdomain(p.Name), path+".name")
		v.check(nodes[p.Node], path+".node")
		v.need(p.Ready != nil && p.LocalStorage != nil && p.PlacementRules != nil, path)
		v.check(slices.Contains([]string{"Deployment", "DaemonSet", "StatefulSet", "Job", "Other", "None"}, p.Owner.Kind), path+".owner.kind")
		v.check((p.Owner.Kind == "None") == (p.Owner.Name == "") && (p.Owner.Name == "" || isSubdomain(p.Owner.Name)), path+".owner.name")
		v.resources(p.Requests, path+".requests")
		names = append(names, p.Namespace+"/"+p.Name)
	}
	v.unique(names, "pods")
	names = nil
	for i, b := range s.PDBs {
		path := fmt.Sprintf("pdbs[%d]", i)
		v.check(isLabel(b.Namespace), path+".namespace")
		v.check(isSubdomain(b.Name), path+".name")
		if b.Deployment == nil {
			v.need(b.Unresolved != "", path+".unresolved")
			v.check(b.MinAvailable == nil && b.MaxUnavailable == nil && len(b.Unresolved) <= 64, path)
		} else {
			v.check(isSubdomain(*b.Deployment) && b.Unresolved == "", path+".deployment")
			v.check((b.MinAvailable == nil) != (b.MaxUnavailable == nil), path)
			for _, n := range []*int64{b.MinAvailable, b.MaxUnavailable} {
				v.check(n == nil || *n >= 0, path)
			}
		}
		names = append(names, b.Namespace+"/"+b.Name)
	}
	v.unique(names, "pdbs")
	return v.err
}

var approvalReasons = []string{"always", "drain", "critical", "scale-down", "image"}

func validatePolicy(p *Policy) *inputError {
	v := &validator{input: "policy"}
	v.header(p.APIVersion, p.Kind, "Policy")
	for i, ns := range p.ManagedNamespaces {
		v.check(isLabel(ns), fmt.Sprintf("managedNamespaces[%d]", i))
	}
	for i, n := range p.ManagedNodes {
		v.check(isSubdomain(n), fmt.Sprintf("managedNodes[%d]", i))
	}
	for i, d := range p.ProtectedDeployments {
		v.check(isDeploymentRef(d), fmt.Sprintf("protectedDeployments[%d]", i))
	}
	for i, d := range p.CriticalDeployments {
		v.check(isDeploymentRef(d), fmt.Sprintf("criticalDeployments[%d]", i))
	}
	for i, r := range p.ApprovalRequiredFor {
		v.check(slices.Contains(approvalReasons, r), fmt.Sprintf("approvalRequiredFor[%d]", i))
	}
	v.unique(p.ManagedNamespaces, "managedNamespaces")
	v.unique(p.ManagedNodes, "managedNodes")
	v.unique(p.ProtectedDeployments, "protectedDeployments")
	v.unique(p.CriticalDeployments, "criticalDeployments")
	v.unique(p.ApprovalRequiredFor, "approvalRequiredFor")
	l := p.Limits
	v.nonNeg(l.MaxReplicas, "limits.maxReplicas")
	v.nonNeg(l.MaxUnschedulableNodes, "limits.maxUnschedulableNodes")
	v.nonNeg(l.MinSchedulableNodes, "limits.minSchedulableNodes")
	v.nonNeg(l.MaxWaitSeconds, "limits.maxWaitSeconds")
	v.need(l.RequireImageDigest != nil, "limits.requireImageDigest")
	v.need(l.AllowScaleToZero != nil, "limits.allowScaleToZero")
	return v.err
}
