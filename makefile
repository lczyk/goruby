# Run tests with race detection
test:
	go test -race ./...

# Run static analysis
lint:
	go vet ./...
	gofmt -s -d . | { ! grep .; }

# Format Go source files
fmt:
	gofmt -s -w .

# Spellcheck
spellcheck:
	npx cspell --no-progress "**" 2>/dev/null || echo "  (cspell not found, skipping)"

# Benchmark; override PKG BENCH BENCHTIME to customise
bench:
	go test -bench=$(or $(BENCH),.) -benchtime=$(or $(BENCHTIME),1s) -run='^$$' ./$(or $(PKG),...)

# Coverage profile and HTML report
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	go tool cover -html=coverage.out -o coverage.html

# Pre-commit gate: lint + test + spellcheck
verify: lint test spellcheck

# Fetch ruby gems listed in internal/integrationtest/testdata/gems.lock.
# Used by integration tests; never invoked by `go test` (see `integration`).
gems:
	@bash internal/integrationtest/testdata/fetch_gems.sh

# Remove fetched gem fixtures
gems-clean:
	rm -rf internal/integrationtest/testdata/gems/

# Run integration tests (requires fetched fixtures)
integration: gems
	go test -tags=integration -race -timeout 10m ./internal/integrationtest/...

# Remove generated files
clean:
	rm -f coverage.out coverage.html
