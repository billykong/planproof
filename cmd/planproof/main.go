// Command planproof checks AI-proposed operations plans with the planproof
// kernel and renders the result for a person (docs/prd/areas/t-cli.md).
//
//	planproof check --snapshot FILE --plan FILE --policy FILE [--output json|packet]
//	planproof render [--debug] [VERDICT-FILE]
//	planproof hash FILE
//
// check exits 0 for Proved, 1 for Refuted, 2 for Unknown; any command exits
// 3 for a usage or file error. The command reads only the files it is
// given (and stdin for render), writes no files and uses no network.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/billykong/planproof/internal/yamljson"
	"github.com/billykong/planproof/kernel"
	"github.com/billykong/planproof/render"
)

const usage = `usage:
  planproof check --snapshot FILE --plan FILE --policy FILE [--output json|packet]
  planproof render [--debug] [VERDICT-FILE]
  planproof hash FILE

check exits 0 for Proved, 1 for Refuted, 2 for Unknown; 3 for a usage or file error.
`

const exitError = 3

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is the whole command, testable without a process.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitError
	}
	var out bytes.Buffer // nothing reaches stdout unless the command succeeds
	code, err := 0, error(nil)
	switch args[0] {
	case "check":
		code, err = check(args[1:], &out)
	case "render":
		err = renderCmd(args[1:], stdin, &out)
	case "hash":
		err = hash(args[1:], &out)
	default:
		err = fmt.Errorf("unknown command %q", args[0])
	}
	if err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintf(stderr, "planproof: %v\n", err)
		}
		fmt.Fprint(stderr, usage)
		return exitError
	}
	stdout.Write(out.Bytes())
	return code
}

func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// input reads a file as the kernel receives it: YAML plans and policies
// are converted strictly; everything else is passed on as is (T-02).
func input(path string, yamlOK bool) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		if !yamlOK {
			return nil, fmt.Errorf("%s: snapshots must be JSON, as the fetcher writes them", path)
		}
		if b, err = yamljson.Convert(b); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	return b, nil
}

func check(args []string, out io.Writer) (int, error) {
	fs := newFlags("check")
	snapPath := fs.String("snapshot", "", "snapshot JSON from the fetcher")
	planPath := fs.String("plan", "", "plan, JSON or YAML")
	polPath := fs.String("policy", "", "policy, JSON or YAML")
	output := fs.String("output", "json", "json or packet")
	if err := fs.Parse(args); err != nil {
		return 0, err
	}
	if *snapPath == "" || *planPath == "" || *polPath == "" || fs.NArg() > 0 {
		return 0, errors.New("check needs --snapshot, --plan and --policy, and nothing else")
	}
	if *output != "json" && *output != "packet" {
		return 0, fmt.Errorf("--output must be json or packet, not %q", *output)
	}
	snap, err := input(*snapPath, false)
	if err != nil {
		return 0, err
	}
	plan, err := input(*planPath, true)
	if err != nil {
		return 0, err
	}
	pol, err := input(*polPath, true)
	if err != nil {
		return 0, err
	}
	v := kernel.Check(snap, plan, pol)
	if *output == "packet" {
		io.WriteString(out, render.Packet(v))
	} else {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return 0, err
		}
		out.Write(append(b, '\n'))
	}
	return map[kernel.Result]int{kernel.Proved: 0, kernel.Refuted: 1, kernel.Unknown: 2}[v.Result], nil
}

func renderCmd(args []string, stdin io.Reader, out io.Writer) error {
	fs := newFlags("render")
	debug := fs.Bool("debug", false, "tag each line with the fact that produced it")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var b []byte
	var err error
	switch fs.NArg() {
	case 0:
		b, err = io.ReadAll(io.LimitReader(stdin, 16<<20))
	case 1:
		b, err = os.ReadFile(fs.Arg(0))
	default:
		return errors.New("render takes at most one verdict file")
	}
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields() // T-05
	var v kernel.Verdict
	if err := dec.Decode(&v); err != nil {
		return fmt.Errorf("reading the verdict: %w", err)
	}
	if v.Kind != "Verdict" || len(v.Facts) == 0 {
		return errors.New("not a verdict")
	}
	if *debug {
		io.WriteString(out, render.Debug(v))
	} else {
		io.WriteString(out, render.Packet(v))
	}
	return nil
}

// hash prints the hash the kernel computes for a file, after conversion (T-04).
func hash(args []string, out io.Writer) error {
	if len(args) != 1 {
		return errors.New("hash takes one file")
	}
	b, err := input(args[0], true)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, kernel.Hash(b))
	return nil
}
