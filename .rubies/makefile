.SUFFIXES:

.PHONY: rubies
rubies:  ## Fetch and compile MRI ruby binaries for syntax verification
	@bash fetch_rubies.sh

.PHONY: clean
clean:  ## Remove compiled ruby binaries and build artifacts
	rm -rf versions/ build/ cache/

.PHONY: verify
verify:  ## Verify all test fixtures against downloaded ruby binaries (assumes rubies + gems built)
	@bash verify.sh

.PHONY: golden
golden:  ## Generate MRI golden TSV for integration tests (assumes rubies + gems built)
	@bash generate-golden.sh

.PHONY: help
help:  ## Show this help
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
