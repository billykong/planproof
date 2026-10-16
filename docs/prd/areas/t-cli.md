# T - The CLI

**Goal:** one small command that agents, CI and people use to check a plan, read the packet, and get the hashes that bind plans, snapshots and approvals together. It is outside the kernel, but on the trusted path: it decides which bytes the kernel checks and the approver's hash covers. Completes phase 1.

Status: **Done** (phase 1 milestone 5). Code: `cmd/planproof/`, `internal/yamljson/`.

## User stories

- As an **agent**, I run `planproof check` on my plan and get the verdict JSON, with an exit code I can branch on, and the facts to repair a refuted plan.
- As an **approver**, I run `planproof render` on a verdict and read the packet.
- As an **agent or executor**, I run `planproof hash` to get exactly the hash the kernel will compute, to put the snapshot's hash in a plan or check an approval.
- As a **security reviewer**, I know that the YAML a person reviewed means one thing only: no anchors, aliases, merge keys or type tricks change what the kernel sees.

## Commands

```text
planproof check --snapshot FILE --plan FILE --policy FILE [--output json|packet]
planproof render [--debug] [VERDICT-FILE]       (stdin if no file)
planproof hash FILE
```

Exit codes: `check` returns 0 for Proved, 1 for Refuted, 2 for Unknown; any command returns 3 for a usage or file error, with a message on stderr and nothing on stdout.

## Requirements

| ID | Requirement | Priority | Status |
| --- | --- | --- | --- |
| T-01 | The three commands above, with the exit codes above. | Must | **Done**: `TestExamplesEndToEnd` (exit 0 and 1), `TestUnknownExit` (exit 2), `TestErrors` (exit 3, empty stdout) |
| T-02 | Plans and policies may be YAML (`.yaml`, `.yml`); everything else is passed to the kernel as is. Snapshots must be JSON: the fetcher writes them and the plan binds their exact bytes. | Must | **Done**: `TestExamplesEndToEnd`, `TestErrors` ("YAML snapshot") |
| T-03 | YAML is converted strictly and deterministically to compact JSON with sorted keys: exactly one document; no anchors, aliases or merge keys; only the core tags (string, integer, boolean, null); no floats; no duplicate keys; at most 32 levels deep; input at most 64 KiB. Anything else is an error, not a guess. | Must | **Done**: `internal/yamljson` (`TestRejected`: anchors, aliases, merge keys, duplicate keys, floats, timestamps, custom and binary tags, non-string keys, two documents, depth, size, huge integers; `TestYAML11Words`: `no`, `on`, `yes` stay strings). Its output is byte-identical to the previous converter on every example and test case (the golden hashes didn't change) |
| T-04 | `hash` prints exactly the hash the kernel computes for the file (after conversion), tested against the kernel's own hashes in the golden verdicts. | Must | **Done**: `TestHash` |
| T-05 | `render` reads a verdict strictly (unknown fields rejected). | Must | **Done**: `TestErrors` ("unknown verdict key", "not a verdict") |
| T-06 | End to end: the CLI on the worked examples reproduces the golden verdicts and packets byte for byte (`make examples`). | Must | **Done**: `make examples` (the built binary, in CI) and `TestExamplesEndToEnd` |
| T-07 | The CLI never writes files, makes no network calls, and reads only the files named on the command line (and stdin for `render`). | Must | **Partial**: by construction (`cmd/planproof/main.go` imports no network package and opens files only with `os.ReadFile` on its arguments); no automated check yet |
