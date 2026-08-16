## tgju-api-go — development tasks.
##
## Run `make help` for the list of targets.

GO            ?= go
GOFLAGS       ?=
PKGS          := ./...
BINARY        := tgju
BUILD_DIR     := bin
COVER_PROFILE := coverage.out
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS       := -s -w -X main.buildVersion=$(VERSION)
IMAGE         ?= ghcr.io/amiranmanesh/tgju-api-go
PORT          ?= 8080

# Build with exactly the toolchain go.mod asks for, so a stray newer toolchain
# on a laptop cannot make a build that CI will not reproduce.
export GOTOOLCHAIN = local

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

# --------------------------------------------------------------------------
# Build
# --------------------------------------------------------------------------

.PHONY: build
build: ## Compile the binary into ./bin
	$(GO) build $(GOFLAGS) -trimpath -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) ./cmd/tgju

.PHONY: install
install: ## Install the binary into GOBIN
	$(GO) install $(GOFLAGS) -trimpath -ldflags="$(LDFLAGS)" ./cmd/tgju

.PHONY: compile
compile: ## Type check every package
	$(GO) build $(GOFLAGS) $(PKGS)

# --------------------------------------------------------------------------
# Test
# --------------------------------------------------------------------------

.PHONY: test
test: ## Run the test suite with the race detector
	$(GO) test $(GOFLAGS) -race $(PKGS)

.PHONY: test-short
test-short: ## Run the test suite without the race detector
	$(GO) test $(GOFLAGS) $(PKGS)

.PHONY: cover
cover: ## Run the tests and report total coverage
	$(GO) test $(GOFLAGS) -coverprofile=$(COVER_PROFILE) -covermode=atomic $(PKGS)
	$(GO) tool cover -func=$(COVER_PROFILE) | tail -n 1

.PHONY: cover-html
cover-html: cover ## Open the coverage report in a browser
	$(GO) tool cover -html=$(COVER_PROFILE)

.PHONY: fuzz
fuzz: ## Fuzz the number parser for thirty seconds
	$(GO) test -run=xxx -fuzz=FuzzValue -fuzztime=30s ./internal/numfmt
	$(GO) test -run=xxx -fuzz=FuzzChange -fuzztime=30s ./internal/numfmt

.PHONY: bench
bench: ## Run the benchmarks
	$(GO) test -run=xxx -bench=. -benchmem $(PKGS)

.PHONY: live
live: build ## Smoke test against the real tgju.org
	./$(BUILD_DIR)/$(BINARY) get currency --keys price_dollar_rl,price_eur
	./$(BUILD_DIR)/$(BINARY) get gold --keys geram18
	./$(BUILD_DIR)/$(BINARY) get coin --keys sekee

# --------------------------------------------------------------------------
# Lint
# --------------------------------------------------------------------------

.PHONY: fmt
fmt: ## Format the code
	$(GO) fmt $(PKGS)

.PHONY: fmt-check
fmt-check: ## Fail when the code is not formatted
	@unformatted=$$(gofmt -l . || true); \
	if [ -n "$$unformatted" ]; then \
		echo "these files are not formatted:"; echo "$$unformatted"; exit 1; \
	fi

.PHONY: vet
vet: ## Run go vet
	$(GO) vet $(PKGS)

.PHONY: lint
lint: fmt-check vet ## Format check plus go vet

.PHONY: golangci
golangci: ## Run golangci-lint, if it is installed
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint is not installed: https://golangci-lint.run/welcome/install/"; exit 1; }
	golangci-lint run

.PHONY: vuln
vuln: ## Check the dependencies for known vulnerabilities
	$(GO) run golang.org/x/vuln/cmd/govulncheck@latest $(PKGS)

.PHONY: tidy
tidy: ## Tidy the module file
	$(GO) mod tidy

.PHONY: deps-check
deps-check: ## Fail when a dependency other than golang.org/x sneaks in
	@bad=$$(grep -E '^\s+[a-z0-9./-]+ v' go.mod | grep -v 'golang.org/x/' || true); \
	if [ -n "$$bad" ]; then \
		echo "tgju-api-go depends on the standard library plus golang.org/x only, but found:"; \
		echo "$$bad"; exit 1; \
	fi
	@echo "dependencies are within policy"

# --------------------------------------------------------------------------
# Run
# --------------------------------------------------------------------------

.PHONY: run
run: ## Serve the API on http://localhost:$(PORT)
	$(GO) run ./cmd/tgju serve --addr :$(PORT)

.PHONY: examples
examples: ## Run the example programs that need no arguments
	$(GO) run ./examples/basic

.PHONY: doc
doc: ## Serve the package documentation on http://localhost:6060
	$(GO) run golang.org/x/tools/cmd/godoc@latest -http=:6060

.PHONY: pages
pages: ## Assemble the GitHub Pages site into ./_site
	rm -rf _site && mkdir -p _site
	cp -R docs/. _site/
	cp server/openapi.yaml _site/openapi.yaml
	touch _site/.nojekyll
	$(GO) run ./internal/cmd/checkspec _site/openapi.yaml
	@echo "site assembled in ./_site — serve it with: python3 -m http.server -d _site 8000"

.PHONY: spec
spec: ## Check the OpenAPI document is internally consistent
	$(GO) run ./internal/cmd/checkspec server/openapi.yaml

# --------------------------------------------------------------------------
# Docker
# --------------------------------------------------------------------------

.PHONY: docker
docker: ## Build the container image
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) -t $(IMAGE):latest .

.PHONY: docker-run
docker-run: docker ## Run the container image on http://localhost:$(PORT)
	docker run --rm -p $(PORT):8080 $(IMAGE):$(VERSION)

.PHONY: compose
compose: ## Bring the stack up with docker compose
	docker compose up --build

# --------------------------------------------------------------------------
# Fixtures and release
# --------------------------------------------------------------------------

.PHONY: fixtures
fixtures: ## Refresh the saved tgju.org pages used by the tests
	$(GO) run ./internal/cmd/fixtures

.PHONY: ci
ci: lint deps-check spec test ## Everything the pipeline runs

.PHONY: release-check
release-check: ci cover ## Everything that must pass before a tag is pushed
	@echo
	@echo "ready to tag $(VERSION)"

.PHONY: clean
clean: ## Remove build and coverage artefacts
	rm -rf $(BUILD_DIR) $(COVER_PROFILE) coverage.html
	$(GO) clean -testcache
