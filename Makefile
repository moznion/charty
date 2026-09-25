BIN := $(CURDIR)/bin
CMD := charty
PKG := ./cmd/charty

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

# Pinned so a local run and a CI run lint against the same rules. Override
# GOLANGCI_LINT with an absolute path to use an already-installed binary.
GOLANGCI_LINT_VERSION ?= v2.14.0
GOLANGCI_LINT ?= $(BIN)/golangci-lint

SNAPSHOT := $(CURDIR)/.vendor-snapshot

.PHONY: all
all: fmt-check lint vendor-check test build

.PHONY: build
build:
	go build -ldflags '$(LDFLAGS)' -o $(BIN)/$(CMD) $(PKG)

.PHONY: test
test:
	go test ./...

.PHONY: lint
lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run

# fmt rewrites; fmt-check only reports, and fails, which is what CI wants.
.PHONY: fmt
fmt: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) fmt

.PHONY: fmt-check
fmt-check: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) fmt --diff

# Dependencies are vendored, so building the CLI needs no network. Run this
# after changing a dependency and commit the result.
.PHONY: vendor
vendor:
	go mod tidy
	go mod vendor

# Go rejects a vendor tree that disagrees with go.mod, so the gap this closes
# is narrower here than in a module with a local replace directive: a hand-edit
# to vendor/, or a `go get` whose re-vendoring was forgotten.
#
# The comparison is against a snapshot rather than `git diff`, which sees only
# tracked files and so reports nothing at all in a fresh repository. On a
# mismatch the regenerated tree is left in place: it is what you would commit.
.PHONY: vendor-check
vendor-check:
	@rm -rf $(SNAPSHOT)
	@mkdir -p $(SNAPSHOT)
	@cp -a vendor $(SNAPSHOT)/vendor 2>/dev/null || true
	@cp go.mod go.sum $(SNAPSHOT)/
	@go mod tidy
	@go mod vendor
	@if diff -rq $(SNAPSHOT)/vendor vendor >/dev/null 2>&1 \
		&& cmp -s $(SNAPSHOT)/go.mod go.mod && cmp -s $(SNAPSHOT)/go.sum go.sum; then \
		rm -rf $(SNAPSHOT); \
	else \
		rm -rf $(SNAPSHOT); \
		echo 'vendor/ was out of date and has been regenerated; commit the result'; \
		exit 1; \
	fi

.PHONY: tools
tools: $(GOLANGCI_LINT)

$(BIN)/golangci-lint:
	@mkdir -p $(BIN)
	GOBIN=$(BIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

.PHONY: clean
clean:
	rm -rf $(BIN) $(SNAPSHOT)

.PHONY: help
help:
	@echo 'build       build $(CMD) into bin/'
	@echo 'test        run the tests'
	@echo 'lint        run golangci-lint'
	@echo 'fmt         apply gofmt and goimports'
	@echo 'fmt-check   report formatting differences without rewriting'
	@echo 'vendor      refresh vendor/ after a dependency change'
	@echo 'vendor-check  fail if vendor/ is out of date'
	@echo 'all         fmt-check, lint, vendor-check, test, build'
