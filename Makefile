SHELL := /bin/bash

# Go from scripts/setup or ~/.local/go, as in safe-ops-agent-k8s.
export PATH := $(HOME)/.local/go/bin:$(PATH)

FUZZTIME ?= 30s

# The lab cluster. Its kubeconfig lives in the repo (.kube/, ignored by git),
# so nothing here reads or changes ~/.kube/config or another cluster's
# context; every kubectl call names both (lesson from safe-ops-agent-k8s).
CLUSTER    := planproof
KCFG       := $(CURDIR)/.kube/$(CLUSTER)
KUBECTL    := kubectl --kubeconfig $(KCFG) --context kind-$(CLUSTER)

.PHONY: ci fmt-check vet test budget fuzz cover build examples kind-up kind-down kind-test

## ci: everything CI runs on a pull request
ci: fmt-check vet test budget cover examples fuzz

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

## cover: kernel statement coverage, failing below COVER_MIN percent
COVER_MIN ?= 95
cover:
	@mkdir -p bin && go test -count=1 -coverprofile=bin/coverage.out ./kernel >/dev/null
	@pct=$$(go tool cover -func=bin/coverage.out | awk '/^total:/ {sub("%","",$$3); print $$3}'); \
	echo "kernel coverage: $$pct% (minimum $(COVER_MIN)%)"; \
	awk -v p=$$pct -v m=$(COVER_MIN) 'BEGIN { exit !(p >= m) }' || { echo "FAIL: kernel coverage below $(COVER_MIN)%"; exit 1; }

build:
	go build -o bin/planproof ./cmd/planproof

## examples: the built CLI on the worked examples reproduces the golden
## verdicts and packets byte for byte (T-06)
EX := examples/drain-two-nodes
examples: build
	@set -e; for p in proved no-wait extra-change; do \
	  bin/planproof check --snapshot $(EX)/snapshot.json --plan $(EX)/plan-$$p.yaml --policy $(EX)/policy.yaml > bin/verdict-$$p.json || [ $$? -le 2 ]; \
	  diff -u $(EX)/verdict-$$p.json bin/verdict-$$p.json; \
	  bin/planproof render bin/verdict-$$p.json | diff -u $(EX)/packet-$$p.txt -; \
	  echo "examples: $$p matches"; \
	done

## kind-up: create the lab cluster (idempotent) and the planproof namespace and RBAC
kind-up:
	@mkdir -p .kube
	@kind get clusters 2>/dev/null | grep -qx $(CLUSTER) || kind create cluster --config cluster/kind.yaml --kubeconfig $(KCFG)
	@kind export kubeconfig --name $(CLUSTER) --kubeconfig $(KCFG) >/dev/null
	$(KUBECTL) wait --for=condition=Ready nodes --all --timeout=180s
	$(KUBECTL) apply -f deploy/rbac/

kind-down:
	kind delete cluster --name $(CLUSTER) --kubeconfig $(KCFG)

## kind-test: the tests that need the lab cluster (make kind-up first)
## The output, with the model-fidelity numbers (predicted vs real lowest
## ready pods), goes to results/kind-test.txt: evidence is generated, not typed.
kind-test:
	@mkdir -p results
	set -o pipefail; PLANPROOF_KUBECONFIG=$(KCFG) go test ./test/kind -count=1 -v -timeout 20m 2>&1 | tee results/kind-test.txt
