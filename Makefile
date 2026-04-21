# Copyright 2025 Deutsche Telekom IT GmbH
#
# SPDX-License-Identifier: Apache-2.0

# Root Makefile — delegates to per-module Makefiles.
# Usage:
#   make verify              Run all pre-push checks
#   make verify MODULES="gateway identity"  Check specific modules
#   make build-all           Build all modules
#   make test-all            Test all modules
#   make lint-all            Lint all modules
#   make tidy-all            Run go mod tidy on all modules

SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# Discover Go modules that have a Makefile (excluding docs and tools)
MODULES ?= $(shell find . -name 'go.mod' -not -path './docs/*' -not -path '*/node_modules/*' -not -path './tools/*' -exec dirname {} \; | while read d; do [ -f "$$d/Makefile" ] && echo "$$d"; done | sed 's|^\./||' | sort)

##@ Verification

.PHONY: verify
verify: ## Run all pre-push checks (pre-commit, tidy, build, lint).
	hack/verify.sh $(MODULES)

##@ Bulk operations

.PHONY: build-all
build-all: ## Build all modules.
	@for mod in $(MODULES); do \
		echo "=== $$mod: make build ===" && \
		$(MAKE) -C $$mod build || exit 1; \
	done

.PHONY: test-all
test-all: ## Test all modules.
	@for mod in $(MODULES); do \
		echo "=== $$mod: make test ===" && \
		$(MAKE) -C $$mod test || exit 1; \
	done

.PHONY: tidy-all
tidy-all: ## Run go mod tidy on all modules.
	@for mod in $(MODULES); do \
		echo "=== $$mod: go mod tidy ===" && \
		cd $$mod && go mod tidy && cd $(CURDIR) || exit 1; \
	done

.PHONY: lint-all
lint-all: ## Lint all modules.
	@for mod in $(MODULES); do \
		if grep -q '^lint:' $$mod/Makefile 2>/dev/null; then \
			echo "=== $$mod: make lint ===" && \
			$(MAKE) -C $$mod lint || exit 1; \
		else \
			echo "=== $$mod: SKIP (no lint target) ==="; \
		fi; \
	done

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
