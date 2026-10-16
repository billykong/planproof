package executor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/billykong/planproof/internal/testcases"
	"github.com/billykong/planproof/kernel"
)

func i64(v int64) *int64 { return &v }

// TestCompile: one fixed translation per operation (C-11).
func TestCompile(t *testing.T) {
	steps := []kernel.Step{
		{Op: "cordon", Node: "n1"},
		{Op: "uncordon", Node: "n1"},
		{Op: "drain", Node: "n1"},
		{Op: "scale", Deployment: "shop/cart", Replicas: i64(3)},
		{Op: "set-image", Deployment: "shop/cart", Container: "web", Image: "img@sha256:1"},
		{Op: "rollout-restart", Deployment: "shop/cart"},
		{Op: "wait", Deployments: []string{"shop/cart"}, TimeoutSeconds: i64(60)},
	}
	calls, err := Compile(steps, "sha256:0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`{"step":1,"verb":"patch","resource":"nodes","name":"n1","patch":"{\"spec\":{\"unschedulable\":true}}"}`,
		`{"step":2,"verb":"patch","resource":"nodes","name":"n1","patch":"{\"spec\":{\"unschedulable\":false}}"}`,
		`{"step":3,"verb":"evict-pods-on-node","resource":"pods","name":"n1"}`,
		`{"step":4,"verb":"patch","resource":"deployments","namespace":"shop","name":"cart","patch":"{\"spec\":{\"replicas\":3}}"}`,
		`{"step":5,"verb":"patch","resource":"deployments","namespace":"shop","name":"cart","patch":"{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"image\":\"img@sha256:1\",\"name\":\"web\"}]}}}}"}`,
		`{"step":6,"verb":"patch","resource":"deployments","namespace":"shop","name":"cart","patch":"{\"spec\":{\"template\":{\"metadata\":{\"annotations\":{\"planproof.io/restarted-by\":\"0123456789ab-step-6\"}}}}}"}`,
		`{"step":7,"verb":"wait","resource":"deployments","deployments":["shop/cart"],"timeoutSeconds":60}`,
	}
	for i, c := range calls {
		got, _ := json.Marshal(c)
		if string(got) != want[i] {
			t.Errorf("step %d:\n got %s\nwant %s", i+1, got, want[i])
		}
	}
	again, _ := Compile(steps, "sha256:0123456789abcdef")
	a, _ := json.Marshal(calls)
	b, _ := json.Marshal(again)
	if string(a) != string(b) {
		t.Fatal("Compile is not deterministic")
	}
	if _, err := Compile([]kernel.Step{{Op: "delete"}}, ""); err == nil {
		t.Fatal("an unknown op compiled")
	}
}

// TestAdmit: only a plan the kernel proves runs, with a matching verdict
// and, when required, an approval of exactly this plan (C-10).
func TestAdmit(t *testing.T) {
	snap, pol := testcases.Load("snapshot.json"), testcases.Load("policy.yaml")
	proved, refuted := testcases.Load("plan-proved.yaml"), testcases.Load("plan-no-wait.yaml")
	planHash := kernel.Hash(proved)
	verdict, _ := json.Marshal(kernel.Check(snap, proved, pol))
	otherVerdict, _ := json.Marshal(kernel.Check(snap, refuted, pol))
	cases := []struct {
		name     string
		in       Inputs
		approval string
		refused  string // "" admits
	}{
		{"proved and approved", Inputs{snap, proved, pol, verdict}, planHash, ""},
		{"no verdict file is fine", Inputs{snap, proved, pol, nil}, planHash, ""},
		{"refuted", Inputs{snap, refuted, pol, nil}, kernel.Hash(refuted), "not Proved"},
		{"approval needed", Inputs{snap, proved, pol, nil}, "", "requires approval"},
		{"approval of another plan", Inputs{snap, proved, pol, nil}, kernel.Hash(refuted), "not this plan"},
		{"verdict for other inputs", Inputs{snap, proved, pol, otherVerdict}, planHash, "not the kernel's verdict"},
		{"edited verdict", Inputs{snap, proved, pol, []byte(strings.Replace(string(otherVerdict), `"Refuted"`, `"Proved"`, 1))}, planHash, "not the kernel's verdict"},
		{"snapshot changed after the plan", Inputs{append(snap, ' '), proved, pol, nil}, planHash, "not Proved"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := Admit(c.in, c.approval)
			switch {
			case c.refused == "" && err != nil:
				t.Fatalf("refused: %v", err)
			case c.refused != "" && (err == nil || !strings.Contains(err.Error(), c.refused)):
				t.Fatalf("got %v, want refusal %q", err, c.refused)
			}
		})
	}
}
