package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/billykong/planproof/internal/testcases"
)

const ex = "../../examples/drain-two-nodes/"

func cli(t *testing.T, stdin string, args ...string) (code int, out, errOut string) {
	t.Helper()
	var o, e bytes.Buffer
	code = run(args, strings.NewReader(stdin), &o, &e)
	return code, o.String(), e.String()
}

func read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(ex, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestExamplesEndToEnd: the CLI on the worked examples gives the golden
// verdicts and packets byte for byte, with the right exit codes (T-01, T-06).
func TestExamplesEndToEnd(t *testing.T) {
	for name, wantCode := range map[string]int{"proved": 0, "no-wait": 1, "extra-change": 1} {
		t.Run(name, func(t *testing.T) {
			args := []string{"check", "--snapshot", ex + "snapshot.json", "--plan", ex + "plan-" + name + ".yaml", "--policy", ex + "policy.yaml"}
			code, out, errOut := cli(t, "", args...)
			if code != wantCode || out != read(t, "verdict-"+name+".json") {
				t.Fatalf("check: exit %d (want %d), stderr %q, verdict differs from golden:\n%s", code, wantCode, errOut, out)
			}
			if _, packet, _ := cli(t, "", append(args, "--output", "packet")...); packet != read(t, "packet-"+name+".txt") {
				t.Fatalf("check --output packet differs from golden:\n%s", packet)
			}
			if _, packet, _ := cli(t, "", "render", ex+"verdict-"+name+".json"); packet != read(t, "packet-"+name+".txt") {
				t.Fatalf("render FILE differs from golden:\n%s", packet)
			}
			if _, packet, _ := cli(t, read(t, "verdict-"+name+".json"), "render"); packet != read(t, "packet-"+name+".txt") {
				t.Fatal("render from stdin differs from golden")
			}
		})
	}
}

// TestHash: hash prints exactly what the kernel computes (T-04).
func TestHash(t *testing.T) {
	var v struct{ PlanSha256, SnapshotSha256, PolicySha256 string }
	if err := json.Unmarshal([]byte(read(t, "verdict-proved.json")), &v); err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]string{"plan-proved.yaml": v.PlanSha256, "snapshot.json": v.SnapshotSha256, "policy.yaml": v.PolicySha256} {
		if code, out, _ := cli(t, "", "hash", ex+file); code != 0 || out != want+"\n" {
			t.Fatalf("hash %s: %q, want %s", file, out, want)
		}
	}
}

// TestErrors: usage and file errors exit 3 and print nothing on stdout.
func TestErrors(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	yamlSnap := write("snapshot.yaml", "apiVersion: planproof/v0\n")
	aliasPlan := write("plan.yaml", "a: &x 1\nb: *x\n")
	badVerdict := write("verdict.json", `{"kind":"Verdict","facts":[{"type":"check"}],"approvedBy":"me"}`)
	for name, args := range map[string][]string{
		"no command":          {},
		"unknown command":     {"apply"},
		"missing flag":        {"check", "--snapshot", ex + "snapshot.json", "--plan", ex + "plan-proved.yaml"},
		"bad output":          {"check", "--snapshot", ex + "snapshot.json", "--plan", ex + "plan-proved.yaml", "--policy", ex + "policy.yaml", "--output", "html"},
		"YAML snapshot":       {"check", "--snapshot", yamlSnap, "--plan", ex + "plan-proved.yaml", "--policy", ex + "policy.yaml"},
		"alias in plan":       {"check", "--snapshot", ex + "snapshot.json", "--plan", aliasPlan, "--policy", ex + "policy.yaml"},
		"missing file":        {"hash", filepath.Join(dir, "nope.json")},
		"unknown verdict key": {"render", badVerdict},
		"not a verdict":       {"render", ex + "snapshot.json"},
	} {
		t.Run(name, func(t *testing.T) {
			if code, out, errOut := cli(t, "", args...); code != exitError || out != "" || errOut == "" {
				t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
			}
		})
	}
}

// TestUnknownExit: an Unknown verdict exits 2 (T-01).
func TestUnknownExit(t *testing.T) {
	for _, c := range testcases.Cases {
		if c.Want != "Unknown" {
			continue
		}
		snap, plan, pol := testcases.Inputs(c)
		dir := t.TempDir()
		for name, b := range map[string][]byte{"snapshot.json": snap, "plan.json": plan, "policy.json": pol} {
			if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		code, _, errOut := cli(t, "", "check", "--snapshot", filepath.Join(dir, "snapshot.json"), "--plan", filepath.Join(dir, "plan.json"), "--policy", filepath.Join(dir, "policy.json"))
		if code != 2 {
			t.Fatalf("%s: exit %d, want 2 (%s)", c.Name, code, errOut)
		}
		return
	}
	t.Fatal("no Unknown case in testcases")
}
