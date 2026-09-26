BINARY := bin/gtme
PKG    := ./...
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-dev)

.PHONY: check fmt vet test build install clean tidy live live-deliver docs-adapters docs-reference

check: fmt vet test

fmt:
	@out=$$(gofmt -l $$(git ls-files '*.go' 2>/dev/null || find . -name '*.go' -not -path './bin/*')); \
	if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

vet:
	go vet $(PKG)

test:
	go test $(PKG)

build:
	go build -ldflags "-X github.com/gtme-run/gtme/internal/cli.Version=$(VERSION)" -o $(BINARY) ./cmd/gtme

# install puts `gtme` on your PATH (~/.local/bin by default; see install.sh
# for PREFIX). After this, every `./bin/gtme` in the docs is just `gtme`.
install:
	./install.sh

# live runs the manual provider smoke tests (SPEC §12: a human gate). Each test
# skips unless its credential is set; nothing is delivered to a real campaign
# unless GTME_LIVE_DELIVER=yes.
live:
	go test -tags live -count=1 -v ./test/live/

live-deliver:
	GTME_LIVE_DELIVER=yes go test -tags live -count=1 -v -run Deliver ./test/live/

tidy:
	go mod tidy

clean:
	rm -rf bin

# docs-adapters regenerates docs/_adapters.json: every built-in adapter's
# manifest as `gtme help --agent` reports it from a clean home with nothing
# installed. gtme.run renders its connector pages from this file, and the e2e
# suite fails when it drifts from the binary (test/e2e/docs_adapters_test.go).
docs-adapters:
	@tmp=$$(mktemp -d); go build -o $$tmp/gtme ./cmd/gtme; \
	HOME=$$tmp/home GTME_ADAPTER_PATH=$$tmp/empty $$tmp/gtme help --agent > $$tmp/agent.json; \
	python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); out={"generated_by":"make docs-adapters: gtme help --agent from a clean home, built-ins only","adapters":d["adapters"]}; f=open("docs/_adapters.json","w"); json.dump(out,f,indent=2); f.write("\n")' $$tmp/agent.json; \
	rm -rf $$tmp; echo "wrote docs/_adapters.json"

# docs-reference regenerates the generated half of docs/: reference/cli,
# reference/adapters, reference/fields, reference/ledger-schema (an index
# page plus one node per item) and glossary.md, from `gtme help --agent`
# captured from a clean home, docs/_adapters.json, spec/fields/*.json,
# spec/ledger.sql, and each concept page's `defines:`. It also rewrites the
# reference children and the terms map in docs/_outline.yaml. The e2e suite
# fails when the committed pages drift (test/e2e/docs_reference_test.go).
docs-reference: docs-adapters
	@tmp=$$(mktemp -d); go build -o $$tmp/gtme ./cmd/gtme; \
	HOME=$$tmp/home GTME_ADAPTER_PATH=$$tmp/empty $$tmp/gtme help --agent > $$tmp/agent.json; \
	go run ./cmd/docsgen -agent $$tmp/agent.json; rm -rf $$tmp
