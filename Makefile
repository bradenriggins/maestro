SHELL := /bin/bash

.PHONY: fmt fmt-check build test test-race lint smoke e2e ui-audit ui-snapshots ci

fmt:
	gofmt -w $(shell git ls-files '*.go')

fmt-check:
	@test -z "$(shell gofmt -l $(shell git ls-files '*.go'))" || \
		(echo "Go files need formatting:" && gofmt -l $(shell git ls-files '*.go') && exit 1)

build:
	go build ./...

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --timeout=3m --out-format=line-number --max-issues-per-linter=0 --max-same-issues=0; \
	else \
		echo "golangci-lint not found in PATH"; \
		exit 1; \
	fi

smoke:
	./scripts/smoke.sh

e2e:
	./scripts/e2e.sh

ui-audit:
	./scripts/ui_audit.sh

ui-snapshots:
	./scripts/ui_snapshots.sh

ci: fmt-check build test test-race smoke e2e ui-audit ui-snapshots
