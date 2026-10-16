package kernel

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
)

// The abstract state (spec 02). The kernel builds it from the snapshot and
// changes it in place, step by step.

type res struct{ cpu, mem, pods int64 }

func toRes(r Resources) res { return res{*r.CPUMilli, *r.MemoryMiB, *r.Pods} }

// mpod is a modelled pod. nodes is where it may be: one node when known,
// else the candidate nodes at the moment the model created it. at is the
// node first fit charged its requests to.
type mpod struct {
	name      string
	dep       string // owning deployment as ns/name; "" if not a deployment pod
	owner     string // owner kind
	nodes     []string
	at        string
	req       res
	ready     bool
	created   bool // made by a plan step
	gone      bool // removed by a scale-down; still charged, never available
	local     bool
	placement bool
}

type mdep struct {
	replicas   int64
	images     map[string]string
	restarted  bool
	rolling    bool  // mid rolling update, until a wait names it
	floor      int64 // available while rolling
	maxUnavail int64
	maxSurge   int64
	recreate   bool
	req        res
	placement  bool
	hpa        bool
	unsettled  bool
	pdbs       int
	unresolved bool
	made       int // counter for names of created pods
}

type mnode struct {
	ready, unsched, taints, drained bool
	alloc                           res
}

type mpdb struct {
	name, dep string
	min, max  *int64
}

type state struct {
	deps  map[string]*mdep
	nodes map[string]*mnode
	pods  []*mpod
	pdbs  []mpdb
}

func build(s *Snapshot) *state {
	st := &state{deps: map[string]*mdep{}, nodes: map[string]*mnode{}}
	for _, n := range s.Nodes {
		st.nodes[n.Name] = &mnode{ready: *n.Ready, unsched: *n.Unschedulable, taints: *n.Taints, alloc: toRes(n.Allocatable)}
	}
	for _, d := range s.Deployments {
		m := &mdep{
			replicas: *d.Replicas, images: map[string]string{}, req: toRes(d.PodRequests),
			maxUnavail: *d.Strategy.MaxUnavailable, maxSurge: *d.Strategy.MaxSurge, recreate: d.Strategy.Type == "Recreate",
			placement: *d.PlacementRules, hpa: *d.HPA,
			unsettled: *d.ReadyReplicas < *d.Replicas || *d.UpdatedReplicas < *d.Replicas,
		}
		for _, c := range d.Containers {
			m.images[c.Name] = c.Image
		}
		st.deps[d.Namespace+"/"+d.Name] = m
	}
	for _, p := range s.Pods {
		mp := &mpod{name: p.Namespace + "/" + p.Name, owner: p.Owner.Kind, nodes: []string{p.Node}, at: p.Node,
			req: toRes(p.Requests), ready: *p.Ready, local: *p.LocalStorage, placement: *p.PlacementRules}
		if p.Owner.Kind == "Deployment" {
			mp.dep = p.Namespace + "/" + p.Owner.Name
		}
		st.pods = append(st.pods, mp)
	}
	for _, b := range s.PDBs {
		if b.Deployment == nil {
			// The fetcher couldn't tie it to one deployment: any deployment in
			// its namespace may be covered.
			for key, d := range st.deps {
				if ns, _, _ := strings.Cut(key, "/"); ns == b.Namespace {
					d.unresolved = true
				}
			}
			continue
		}
		key := b.Namespace + "/" + *b.Deployment
		if d, ok := st.deps[key]; ok {
			d.pdbs++
			d.unresolved = d.unresolved || d.pdbs > 1
			st.pdbs = append(st.pdbs, mpdb{name: b.Namespace + "/" + b.Name, dep: key, min: b.MinAvailable, max: b.MaxUnavailable})
		}
	}
	slices.SortFunc(st.pdbs, func(a, b mpdb) int { return cmp.Or(cmp.Compare(a.dep, b.dep), cmp.Compare(a.name, b.name)) })
	return st
}

// avail is the number of ready pods of a deployment, or the rollout floor
// while it is mid rolling update.
func (st *state) avail(dep string) int64 {
	if d := st.deps[dep]; d != nil && d.rolling {
		return d.floor
	}
	var n int64
	for _, p := range st.pods {
		if p.dep == dep && p.ready && !p.gone {
			n++
		}
	}
	return n
}

func (st *state) podsOf(dep string) []*mpod {
	var out []*mpod
	for _, p := range st.pods {
		if p.dep == dep && !p.gone {
			out = append(out, p)
		}
	}
	return out
}

// mightBeOn: the pod is on the node, or the node is one of its candidates.
func (p *mpod) mightBeOn(node string) bool { return !p.gone && slices.Contains(p.nodes, node) }

// schedulable nodes, by name: ready, not cordoned, not tainted.
func (st *state) schedulable() []string {
	var out []string
	for name, n := range st.nodes {
		if n.ready && !n.unsched && !n.taints {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// newPods makes count pending pods of a deployment with the current
// schedulable nodes as candidates.
func (st *state) newPods(dep string, count int64) []*mpod {
	d, nodes := st.deps[dep], st.schedulable()
	var out []*mpod
	for ; count > 0; count-- {
		d.made++
		p := &mpod{name: dep + "+" + strconv.Itoa(d.made), dep: dep, owner: "Deployment", nodes: nodes, req: d.req, created: true, placement: d.placement}
		st.pods = append(st.pods, p)
		out = append(out, p)
	}
	return out
}

func (st *state) usage() map[string]res {
	u := map[string]res{}
	for _, p := range st.pods {
		if p.at != "" {
			r := u[p.at]
			u[p.at] = res{r.cpu + p.req.cpu, r.mem + p.req.mem, r.pods + p.req.pods}
		}
	}
	return u
}

// place charges pods to nodes by first-fit-decreasing (spec 02): pods by CPU
// request, largest first, then by name; each on the first candidate node,
// by name, with room. It returns the first pod that doesn't fit, or nil.
func (st *state) place(pods []*mpod) *mpod {
	for _, p := range pods {
		p.at = ""
	}
	u := st.usage()
	pods = slices.Clone(pods)
	slices.SortFunc(pods, func(a, b *mpod) int { return cmp.Or(cmp.Compare(b.req.cpu, a.req.cpu), cmp.Compare(a.name, b.name)) })
	for _, p := range pods {
		for _, name := range p.nodes {
			n, used := st.nodes[name], u[name]
			if used.cpu+p.req.cpu <= n.alloc.cpu && used.mem+p.req.mem <= n.alloc.mem && used.pods+p.req.pods <= n.alloc.pods {
				p.at, u[name] = name, res{used.cpu + p.req.cpu, used.mem + p.req.mem, used.pods + p.req.pods}
				break
			}
		}
		if p.at == "" {
			return p
		}
	}
	return nil
}

func (st *state) remove(drop []*mpod) {
	st.pods = slices.DeleteFunc(st.pods, func(p *mpod) bool { return slices.Contains(drop, p) })
}
