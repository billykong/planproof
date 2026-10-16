// Command planproof-fetch reads a cluster and prints the snapshot the
// kernel checks against (spec 02, PRD area C). It is a separate command from
// planproof because it has its own identity: run it as the read-only
// fetcher service account (deploy/rbac/fetcher.yaml), never as an agent.
//
//	planproof-fetch --kubeconfig FILE --context NAME [--cluster NAME] [--as USER]
//
// It prints the snapshot JSON on stdout; put its hash (planproof hash) in
// the plan. Exit 1 on any error, with nothing on stdout.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/billykong/planproof/fetch"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "", "kubeconfig file (required: never the default one)")
	kubeContext := flag.String("context", "", "context in the kubeconfig (required)")
	cluster := flag.String("cluster", "", "cluster name recorded in the snapshot (default: the context)")
	as := flag.String("as", "", "impersonate this user, e.g. system:serviceaccount:planproof:fetcher")
	flag.Parse()
	if *kubeconfig == "" || *kubeContext == "" || flag.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "usage: planproof-fetch --kubeconfig FILE --context NAME [--cluster NAME] [--as USER]")
		os.Exit(1)
	}
	if *cluster == "" {
		*cluster = *kubeContext
	}
	if err := run(*kubeconfig, *kubeContext, *cluster, *as); err != nil {
		fmt.Fprintf(os.Stderr, "planproof-fetch: %v\n", err)
		os.Exit(1)
	}
}

func run(kubeconfig, kubeContext, cluster, as string) error {
	// The kubeconfig and context are always explicit: the current context
	// is whatever another tool last switched to.
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig},
		&clientcmd.ConfigOverrides{CurrentContext: kubeContext},
	).ClientConfig()
	if err != nil {
		return err
	}
	cfg.Impersonate.UserName = as
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	s, err := fetch.Snapshot(ctx, client, cluster, time.Now())
	if err != nil {
		return err
	}
	b, err := fetch.Marshal(s)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(b)
	return err
}
