package kind

import (
	"bytes"
	"context"
	"os/exec"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/billykong/planproof/executor"
	"github.com/billykong/planproof/kernel"
)

// auditLog reads the API server's audit log off the control-plane node.
func auditLog(t *testing.T) []executor.AuditEvent {
	t.Helper()
	out, err := exec.Command("docker", "exec", "planproof-control-plane", "cat", "/var/log/kubernetes/audit/audit.log").Output()
	if err != nil {
		t.Fatalf("reading the audit log (make kind-up turns it on): %v", err)
	}
	events, err := executor.ReadAudit(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	return events
}

// TestTranslationCheck: after a real run, the executor's writes in the API
// audit log match the compiled plan exactly; and a write outside the plan,
// made with the executor's identity, is caught (C-17, GAP-01).
func TestTranslationCheck(t *testing.T) {
	l := newLab(t)
	l.fetch()
	node := l.nodeWithWeb()
	web := l.ns + "/web"
	in := l.inputs(
		[]map[string]any{
			m("node", node, "field", "unschedulable", "from", false, "to", true), m("node", node, "field", "drained", "from", false, "to", true),
			m("deployment", web, "field", "replicas", "from", 3, "to", 4), m("deployment", web, "field", "restarted", "from", false, "to", true),
		},
		[]map[string]any{
			m("op", "cordon", "node", node), m("op", "drain", "node", node), m("op", "wait", "deployments", []string{web}, "timeoutSeconds", 120),
			m("op", "scale", "deployment", web, "replicas", 4), m("op", "rollout-restart", "deployment", web),
			m("op", "wait", "deployments", []string{web}, "timeoutSeconds", 120),
		}, nil)
	since := time.Now().Add(-time.Second)
	rep := l.runner().Run(ctx(t), in, kernel.Hash(in.Plan))
	if rep.Result != "done" {
		t.Fatalf("run: %+v", rep)
	}
	_, plan, err := executor.Admit(in, kernel.Hash(in.Plan))
	if err != nil {
		t.Fatal(err)
	}
	calls, _ := executor.Compile(plan.Steps, kernel.Hash(in.Plan))

	// The audit backend writes asynchronously: give it a moment.
	var problems []string
	for i := 0; i < 10; i++ {
		problems = executor.CheckTranslation(calls, rep, auditLog(t), executorSA, since)
		if len(problems) == 0 {
			break
		}
		time.Sleep(time.Second)
	}
	if len(problems) > 0 {
		t.Fatalf("the audit log doesn't match the compiled plan:\n%v", problems)
	}
	t.Logf("translation check: %d writes in the audit log, all compiled, none extra", len(rep.Done))

	// Now a write outside the plan, with the executor's identity: RBAC
	// allows it (GAP-01); the check must catch it.
	exe := as(t, l.cfg, executorSA)
	if _, err := exe.CoreV1().Nodes().Patch(context.Background(), node, types.StrategicMergePatchType,
		[]byte(`{"metadata":{"labels":{"planproof-test":"not-in-the-plan"}}}`), metav1.PatchOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		l.admin.CoreV1().Nodes().Patch(context.Background(), node, types.StrategicMergePatchType, []byte(`{"metadata":{"labels":{"planproof-test":null}}}`), metav1.PatchOptions{})
	})
	var caught []string
	for i := 0; i < 10 && len(caught) == 0; i++ {
		caught = executor.CheckTranslation(calls, rep, auditLog(t), executorSA, since)
		time.Sleep(time.Second)
	}
	t.Logf("the extra write: %v", caught)
	if len(caught) == 0 {
		t.Fatal("a write outside the plan, with the executor's identity, wasn't caught")
	}
}
