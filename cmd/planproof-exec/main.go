// Command planproof-exec runs a plan the kernel proves (PRD area C). It
// re-runs the kernel itself, so a verdict file is only checked, never
// trusted; stops if the cluster has drifted from the snapshot; and never
// rolls back on its own. Run it as the executor service account
// (deploy/rbac/executor.yaml): the only identity that writes.
//
//	planproof-exec --kubeconfig FILE --context NAME [--as USER]
//	  --snapshot FILE --plan FILE --policy FILE [--verdict FILE] [--approved-plan sha256:...]
//
// It prints a report as JSON. Exit 0: done; 1: stopped part way (the
// report says where, and offers the checked rollback); 3: refused or a
// usage error, with nothing changed.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/billykong/planproof/executor"
	"github.com/billykong/planproof/internal/yamljson"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "", "kubeconfig file (required: never the default one)")
	kubeContext := flag.String("context", "", "context in the kubeconfig (required)")
	as := flag.String("as", "", "impersonate this user, e.g. system:serviceaccount:planproof:executor")
	snap := flag.String("snapshot", "", "snapshot JSON")
	plan := flag.String("plan", "", "plan, JSON or YAML")
	policy := flag.String("policy", "", "policy, JSON or YAML")
	verdict := flag.String("verdict", "", "the verdict a person read (optional; must match)")
	approved := flag.String("approved-plan", "", "the plan hash a person approved, when the policy requires approval")
	stepTimeout := flag.Duration("step-timeout", 5*time.Minute, "how long a drain may take")
	flag.Parse()
	if *kubeconfig == "" || *kubeContext == "" || *snap == "" || *plan == "" || *policy == "" || flag.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "usage: planproof-exec --kubeconfig FILE --context NAME [--as USER] --snapshot FILE --plan FILE --policy FILE [--verdict FILE] [--approved-plan sha256:...]")
		os.Exit(3)
	}
	in, err := inputs(*snap, *plan, *policy, *verdict)
	if err != nil {
		fail(err)
	}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: *kubeconfig}, &clientcmd.ConfigOverrides{CurrentContext: *kubeContext}).ClientConfig()
	if err != nil {
		fail(err)
	}
	cfg.Impersonate.UserName = *as
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		fail(err)
	}
	r := &executor.Runner{Client: client, StepTimeout: *stepTimeout, Poll: 2 * time.Second}
	rep := r.Run(context.Background(), in, *approved)
	b, _ := json.MarshalIndent(rep, "", "  ")
	fmt.Println(string(b))
	os.Exit(map[string]int{"done": 0, "stopped": 1, "refused": 3}[rep.Result])
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "planproof-exec: %v\n", err)
	os.Exit(3)
}

// inputs reads the files exactly as planproof check does, so the hashes
// match: YAML plans and policies through the same strict conversion.
func inputs(snap, plan, policy, verdict string) (executor.Inputs, error) {
	read := func(path string, yamlOK bool) ([]byte, error) {
		b, err := os.ReadFile(path)
		if err != nil || !yamlOK {
			return b, err
		}
		if ext := strings.ToLower(filepath.Ext(path)); ext == ".yaml" || ext == ".yml" {
			return yamljson.Convert(b)
		}
		return b, nil
	}
	var in executor.Inputs
	var err error
	if in.Snapshot, err = read(snap, false); err != nil {
		return in, err
	}
	if in.Plan, err = read(plan, true); err != nil {
		return in, err
	}
	if in.Policy, err = read(policy, true); err != nil {
		return in, err
	}
	if verdict != "" {
		in.Verdict, err = os.ReadFile(verdict)
	}
	return in, err
}
