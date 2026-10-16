# Go from scripts/setup or ~/.local/go, as in safe-ops-agent-k8s.
export PATH := $(HOME)/.local/go/bin:$(PATH)

FUZZTIME ?= 30s

.PHONY: ci fmt-check vet test budget fuzz cover

## ci: everything CI runs on a pull request
ci: fmt-check vet test budget fuzz

fmt-check:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "run gofmt -w ."; exit 1; }

vet:
	go vet ./...

test:
	go test -count=1 ./...

## budget: kernel size, import allowlist, no goroutines or I/O
budget:
	./scripts/kernel-budget.sh

## fuzz: the kernel never panics, is deterministic, and never wrongly proves.
## Short minimization: the default (60 s) stalls on the multi-KiB seeds.
fuzz:
	go test ./kernel -run='^$$' -fuzz=FuzzCheck -fuzztime=$(FUZZTIME) -fuzzminimizetime=2s

cover:
	go test -count=1 -coverprofile=coverage.out ./kernel && go tool cover -func=coverage.out | tail -1
