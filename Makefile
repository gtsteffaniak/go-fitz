# go-fitz developer targets. Run `make help` for the list.
#
# The module is pure Go and builds without cgo. The comparison reference is
# gen2brain/go-fitz, a cgo library pinned in test/reference/go.mod. It is a
# separate module, so cgo never enters this module.

GO ?= go
# Pinned to match the workflows (.github/workflows/pr.yml and test.yml).
GOLANGCI_LINT_VERSION ?= v2.15.0
BIN_DIR := $(shell $(GO) env GOPATH)/bin
GOLANGCI_LINT ?= $(BIN_DIR)/golangci-lint
PACKAGES := ./internal/... ./pkg/... ./test/...
TOOLS := ./cmd/...
REPORT_SITE ?= test/compare/report_site
REPORT_PORT ?= 8765
GOLDEN := test/compare/golden/test.pdf.json
REFERENCE_PDF := pkg/fitz/testdata/test.pdf
REFERENCE_BIN := test/reference/bin/fitz-compare

.DEFAULT_GOAL := help
.PHONY: help build fmt fmt-check lint test race \
	compare serve reference-bin golden setup ci clean

help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  %-14s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the packages and tools with cgo disabled
	CGO_ENABLED=0 $(GO) build $(PACKAGES) $(TOOLS)

fmt: ## Format all Go files in place
	gofmt -w .

fmt-check: ## Fail if any Go file is not gofmt-clean
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "Files need gofmt:" >&2; \
		echo "$$unformatted" >&2; \
		exit 1; \
	fi

lint: ## Run golangci-lint with .golangci.yml (includes go vet; needs v2.15.0 or newer)
	$(GOLANGCI_LINT) run --config .golangci.yml $(PACKAGES) $(TOOLS)

test: ## Unit tests and the compare gate tests (no cgo)
	CGO_ENABLED=0 $(GO) test -count=1 $(PACKAGES)

race: ## Tests under the race detector (requires cgo for -race)
	CGO_ENABLED=1 $(GO) test -race -count=1 $(PACKAGES)

compare: ## Write the comparison report and fail on any non-green case
	CGO_ENABLED=0 $(GO) run ./cmd/fitz-report -gate

serve: ## Serve the comparison report on localhost (port 8765 by default)
	-$(MAKE) compare
	python3 -m http.server $(REPORT_PORT) -d $(REPORT_SITE)

reference-bin: ## Build the cgo reference command in test/reference (needs a C compiler)
	cd test/reference && $(GO) build -o bin/fitz-compare .

golden: reference-bin ## Check goldens against the cgo reference (make golden UPDATE=1 to rewrite them)
ifeq ($(UPDATE),1)
	$(REFERENCE_BIN) $(REFERENCE_PDF) > $(GOLDEN)
else
	@tmp="$$(mktemp)"; \
	$(REFERENCE_BIN) $(REFERENCE_PDF) > "$$tmp" && \
	diff -u $(GOLDEN) "$$tmp"; \
	status=$$?; rm -f "$$tmp"; exit $$status
endif

setup: ## Prepare this machine: Go version, pinned golangci-lint, modules, optional tools
	@required="$$(awk '/^go /{print $$2}' go.mod)"; \
	installed="$$($(GO) env GOVERSION | sed 's/^go//')"; \
	if [ "$$(printf '%s\n%s\n' "$$required" "$$installed" | sort -V | head -1)" != "$$required" ]; then \
		echo "Go $$required or newer is required; found $$installed" >&2; \
		exit 1; \
	fi
	GOBIN=$(BIN_DIR) $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	$(GO) mod download
	@command -v cc >/dev/null 2>&1 || echo "warning: no C compiler found; needed for the cgo reference (make golden, race)" >&2
	@command -v python3 >/dev/null 2>&1 || echo "warning: python3 not found; needed for make serve" >&2
	@echo "Setup complete: golangci-lint $(GOLANGCI_LINT_VERSION) in $(BIN_DIR)"

ci: fmt-check lint test race compare golden ## Same checks as the CI workflows

clean: ## Remove generated report output and the reference binary
	rm -rf $(REPORT_SITE)/data $(REPORT_SITE)/build.json test/reference/bin
