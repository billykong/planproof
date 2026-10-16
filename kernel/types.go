package kernel

// The wire types for the three inputs (spec 01, 02, 04). Pointers mark
// required numbers and booleans whose zero value is meaningful, so a missing
// field can be told apart from 0 or false.

// Plan is what the agent submits. Untrusted.
type Plan struct {
	APIVersion string       `json:"apiVersion"`
	Kind       string       `json:"kind"`
	Metadata   PlanMetadata `json:"metadata"`
	Snapshot   string       `json:"snapshot"`
	Rationale  *string      `json:"rationale"`
	Goal       []Change     `json:"goal"`
	Steps      []Step       `json:"steps"`
	Rollback   []Step       `json:"rollback"`
}

type PlanMetadata struct {
	Name   string  `json:"name"`
	Ticket *string `json:"ticket"`
}

// Change is one goal entry: one field of one object, from and to.
type Change struct {
	Deployment string `json:"deployment"`
	Node       string `json:"node"`
	Field      string `json:"field"`
	Container  string `json:"container"`
	From       Value  `json:"from"`
	To         Value  `json:"to"`
}

// Step is one operation. Which parameters it may and must have depends on Op.
type Step struct {
	Op             string      `json:"op,omitempty"`
	Deployment     string      `json:"deployment,omitempty"`
	Node           string      `json:"node,omitempty"`
	Replicas       *int64      `json:"replicas,omitempty"`
	Container      string      `json:"container,omitempty"`
	Image          string      `json:"image,omitempty"`
	Deployments    []string    `json:"deployments,omitempty"`
	TimeoutSeconds *int64      `json:"timeoutSeconds,omitempty"`
	Pre            *Assertions `json:"pre,omitempty"`
	Post           *Assertions `json:"post,omitempty"`
}

// Assertions are claims about the step's object, checked by the kernel and,
// for pre, again by the executor against the live cluster.
type Assertions struct {
	Replicas      *int64  `json:"replicas,omitempty"`
	Image         *string `json:"image,omitempty"`
	Container     string  `json:"container,omitempty"`
	Unschedulable *bool   `json:"unschedulable,omitempty"`
	PodsOnNode    *int64  `json:"podsOnNode,omitempty"`
}

// Snapshot is the cluster state the fetcher recorded. Trusted, but still
// parsed strictly.
type Snapshot struct {
	APIVersion  string       `json:"apiVersion"`
	Kind        string       `json:"kind"`
	Cluster     string       `json:"cluster"`
	FetchedAt   string       `json:"fetchedAt"`
	Nodes       []Node       `json:"nodes"`
	Deployments []Deployment `json:"deployments"`
	Pods        []Pod        `json:"pods"`
	PDBs        []PDB        `json:"pdbs"`
}

type Resources struct {
	CPUMilli  *int64 `json:"cpuMilli"`
	MemoryMiB *int64 `json:"memoryMiB"`
	Pods      *int64 `json:"pods"`
}

type Node struct {
	Name            string    `json:"name"`
	Ready           *bool     `json:"ready"`
	Unschedulable   *bool     `json:"unschedulable"`
	Taints          *bool     `json:"taints"`
	Allocatable     Resources `json:"allocatable"`
	ResourceVersion string    `json:"resourceVersion"`
}

type Deployment struct {
	Namespace       string      `json:"namespace"`
	Name            string      `json:"name"`
	Replicas        *int64      `json:"replicas"`
	ReadyReplicas   *int64      `json:"readyReplicas"`
	UpdatedReplicas *int64      `json:"updatedReplicas"`
	Strategy        Strategy    `json:"strategy"`
	Containers      []Container `json:"containers"`
	PodRequests     Resources   `json:"podRequests"`
	PlacementRules  *bool       `json:"placementRules"`
	HPA             *bool       `json:"hpa"`
	ResourceVersion string      `json:"resourceVersion"`
}

type Strategy struct {
	Type           string `json:"type"`
	MaxUnavailable *int64 `json:"maxUnavailable"`
	MaxSurge       *int64 `json:"maxSurge"`
}

type Container struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

type Pod struct {
	Namespace      string    `json:"namespace"`
	Name           string    `json:"name"`
	Node           string    `json:"node"`
	Ready          *bool     `json:"ready"`
	Owner          Owner     `json:"owner"`
	Requests       Resources `json:"requests"`
	LocalStorage   *bool     `json:"localStorage"`
	PlacementRules *bool     `json:"placementRules"`
}

type Owner struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// PDB is a PodDisruptionBudget resolved by the fetcher to one deployment,
// or marked unresolved.
type PDB struct {
	Namespace      string  `json:"namespace"`
	Name           string  `json:"name"`
	Deployment     *string `json:"deployment"`
	MinAvailable   *int64  `json:"minAvailable"`
	MaxUnavailable *int64  `json:"maxUnavailable"`
	Unresolved     string  `json:"unresolved"`
}

// Policy is the human-reviewed file of scope and limits.
type Policy struct {
	APIVersion           string   `json:"apiVersion"`
	Kind                 string   `json:"kind"`
	ManagedNamespaces    []string `json:"managedNamespaces"`
	ManagedNodes         []string `json:"managedNodes"`
	ProtectedDeployments []string `json:"protectedDeployments"`
	CriticalDeployments  []string `json:"criticalDeployments"`
	ApprovalRequiredFor  []string `json:"approvalRequiredFor"`
	Limits               Limits   `json:"limits"`
}

type Limits struct {
	MaxReplicas           *int64 `json:"maxReplicas"`
	MaxUnschedulableNodes *int64 `json:"maxUnschedulableNodes"`
	MinSchedulableNodes   *int64 `json:"minSchedulableNodes"`
	MaxWaitSeconds        *int64 `json:"maxWaitSeconds"`
	RequireImageDigest    *bool  `json:"requireImageDigest"`
	AllowScaleToZero      *bool  `json:"allowScaleToZero"`
}
