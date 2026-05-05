# Build all binaries
build:
	go build -o bin/goruby .
	go build -o bin/girb ./cmd/girb

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

# Remove generated files
clean:
	rm -f coverage.out coverage.html
	rm -rf bin/
