.SUFFIXES:

SRCS := $(shell find . -name '*.go' -not -path './vendor/*')

help:  ## Show this help
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ./bin/goruby ./bin/girb ./bin/grgr  ## Build all binaries (compressed with upx if available)

./bin/goruby: $(SRCS) Makefile go.mod go.sum
	mkdir -p ./bin
	go build -o ./bin/goruby ./cmd/goruby
	@if command -v upx >/dev/null 2>&1; then \
		upx ./bin/goruby || echo "upx failed, skipping compression"; \
	fi

./bin/girb: $(SRCS) Makefile go.mod go.sum
	mkdir -p ./bin
	go build -o ./bin/girb ./cmd/girb
	@if command -v upx >/dev/null 2>&1; then \
		upx ./bin/girb || echo "upx failed, skipping compression"; \
	fi

./bin/grgr: $(SRCS) Makefile go.mod go.sum
	mkdir -p ./bin
	go build -o ./bin/grgr ./cmd/grgr
	@if command -v upx >/dev/null 2>&1; then \
		upx ./bin/grgr || echo "upx failed, skipping compression"; \
	fi

.PHONY: du
du: build  ## Show binary sizes
	du -h ./bin/goruby ./bin/girb ./bin/grgr

.PHONY: install
install: build  ## Symlink binaries into ~/.local/bin
	mkdir -p $(HOME)/.local/bin
	ln -sf "$(PWD)/bin/goruby" "$(HOME)/.local/bin/goruby"
	ln -sf "$(PWD)/bin/girb" "$(HOME)/.local/bin/girb"
	ln -sf "$(PWD)/bin/grgr" "$(HOME)/.local/bin/grgr"

.PHONY: test
test:  ## Run the test suite with race detector
	@if command -v gotest >/dev/null 2>&1; then \
		gotest -race ./...; \
	else \
		go test -race ./...; \
	fi

.PHONY: lint
lint:  ## go vet + gofmt check (no writes)
	go vet ./...
	@out=$$(gofmt -s -l $$(find . -name '*.go' -not -path './vendor/*')); \
	if [ -n "$$out" ]; then \
		echo "Unformatted files:"; echo "$$out"; exit 1; \
	fi

.PHONY: format
format:  ## gofmt the tree in place
	gofmt -s -w $$(find . -name '*.go' -not -path './vendor/*')

.PHONY: bench
bench:  ## Run benchmarks (override scope/duration: PKG=... BENCH=... BENCHTIME=...)
	go test -run '^$$' -bench '$(or $(BENCH),.)' -benchmem -benchtime '$(or $(BENCHTIME),1s)' $(or $(PKG),./...)

.PHONY: cover
cover:  ## Coverage profile + HTML file (cover.out, cover.html)
	go test -coverpkg=./... -coverprofile=cover.out -race ./...
	go tool cover -func=cover.out
	go tool cover -html=cover.out -o cover.html

.PHONY: cover-open
cover-open: cover  ## Run coverage and open the HTML report in a browser
	go tool cover -html=cover.out

.PHONY: verify
verify: lint test  ## Pre-commit gate: lint, test
	@echo "All checks passed."

.PHONY: clean
clean:  ## Remove build artifacts
	rm -rf ./bin
	rm -f cover.out cover.html
