.SUFFIXES:

GO_DIRS := ./ast ./lexer ./parser ./token ./internal

help:  ## Show this help
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

GOTEST := $(shell command -v gotest 2>/dev/null || echo go test)

.PHONY: unit
unit:  ## Run unit tests with race detection
	$(GOTEST) -race -timeout 2m ./...

.PHONY: test
test: unit integration  ## Run all tests (unit + integration)

.PHONY: lint
lint:  ## go vet + gofmt check (no writes)
	go vet ./...
	@out=$$(gofmt -s -l $(GO_DIRS)); \
	if [ -n "$$out" ]; then echo "Unformatted files:"; echo "$$out"; exit 1; fi

.PHONY: fmt
fmt:  ## gofmt the tree in place
	gofmt -s -w $(GO_DIRS)

.PHONY: spellcheck
spellcheck:  ## Spellcheck sources and docs with cspell (via npx)
	npx --yes cspell --no-progress --gitignore "**/*.go" "**/*.md" "makefile"

.PHONY: bench
bench:  ## Run benchmarks (override scope/duration: PKG=... BENCH=... BENCHTIME=...)
	go test -run '^$$' -bench '$(or $(BENCH),.)' -benchmem -benchtime '$(or $(BENCHTIME),1s)' $(or $(PKG),./...)

.PHONY: cover
cover:  ## Coverage profile + HTML report (cover.out, cover.html)
	go test -coverpkg=./... -coverprofile=cover.out -race ./...
	go tool cover -func=cover.out
	go tool cover -html=cover.out -o cover.html

.PHONY: cover-open
cover-open: cover  ## Run coverage and open the HTML report in a browser
	go tool cover -html=cover.out

.PHONY: verify
verify: lint test spellcheck  ## Pre-commit gate: lint + test + spellcheck
	@echo "All checks passed."

.PHONY: gems
gems:  ## Fetch ruby gem fixtures for the integration suite
	@bash internal/integrationtest/testdata/fetch_gems.sh

.PHONY: gems-clean
gems-clean:  ## Remove fetched gem fixtures
	rm -rf internal/integrationtest/testdata/gems/

.PHONY: integration
integration: gems  ## Run integration smoke suite (requires fetched fixtures)
	$(GOTEST) -v -tags=integration -race -timeout 10m ./internal/integrationtest/...

.PHONY: rubies
rubies:  ## Fetch and compile MRI ruby binaries for syntax verification
	@$(MAKE) -C .rubies rubies

.PHONY: rubies-clean
rubies-clean:  ## Remove compiled ruby binaries and build artifacts
	@$(MAKE) -C .rubies clean

.PHONY: rubies-verify
rubies-verify: rubies gems  ## Verify all test fixtures against downloaded ruby binaries
	@$(MAKE) -C .rubies verify

.PHONY: rubies-golden
rubies-golden: rubies gems  ## Regenerate MRI golden TSV for version-aware integration tests
	@$(MAKE) -C .rubies golden

.PHONY: oracle
oracle: rubies gems  ## Diff goruby roundtrip against MRI parsetree dump (slow; requires built rubies)
	$(GOTEST) -v -tags=oracle -timeout 20m -run TestMRIParseTreeDiff ./internal/integrationtest/...

.PHONY: clean
clean:  ## Remove generated files
	rm -f cover.out cover.html
