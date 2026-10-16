// Package kind holds the tests that need the lab cluster (make kind-up).
// They are skipped unless PLANPROOF_KUBECONFIG names its kubeconfig; run
// them with make kind-test.
package kind

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	kubeContext = "kind-planproof"
	fetcherSA   = "system:serviceaccount:planproof:fetcher"
	pause       = "registry.k8s.io/pause:3.10" // on every kind node: nothing is pulled
)

// clients returns an admin client and the REST config, or skips.
func clients(t *testing.T) (*kubernetes.Clientset, *rest.Config) {
	t.Helper()
	path := os.Getenv("PLANPROOF_KUBECONFIG")
	if path == "" {
		t.Skip("PLANPROOF_KUBECONFIG not set: run make kind-test")
	}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: path},
		&clientcmd.ConfigOverrides{CurrentContext: kubeContext},
	).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	return kubernetes.NewForConfigOrDie(cfg), cfg
}

// as returns a client that impersonates user: the test then has exactly
// that identity's permissions.
func as(t *testing.T, cfg *rest.Config, user string) *kubernetes.Clientset {
	t.Helper()
	c := rest.CopyConfig(cfg)
	c.Impersonate.UserName = user
	return kubernetes.NewForConfigOrDie(c)
}

// unique names a fixture namespace, so a re-run doesn't wait for the last
// run's namespace to finish terminating.
func unique(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()%1_000_000)
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	return c
}
