# aide project Makefile
#
# Usage:
#   make release                     Auto-bump patch version, commit, and tag
#   make release VERSION=1.2.0       Bump to specific version, commit, and tag
#   make release-push                Auto-bump, commit, tag, and push
#   make release-push VERSION=1.2.0  Bump to specific, commit, tag, and push
#   make hooks                       Install git hooks (lefthook)

.PHONY: release release-push build build-pprof build-web test test-ts test-go lint check-version check-release-needed hooks hooks-check

VERSION_FILES = package.json .claude-plugin/plugin.json .claude-plugin/marketplace.json .codex-plugin/plugin.json packages/opencode-plugin/package.json $(wildcard packages/aide-binary-*/package.json)

# Auto-detect next version from latest git tag (same logic as release.yml)
# If VERSION is passed, use that; otherwise bump patch from latest tag.
ifndef VERSION
  LATEST_TAG := $(shell git describe --tags --abbrev=0 --match 'v[0-9]*.[0-9]*.[0-9]*' 2>/dev/null)
  ifdef LATEST_TAG
    _MAJOR := $(shell echo $(LATEST_TAG) | sed 's/^v//' | cut -d. -f1)
    _MINOR := $(shell echo $(LATEST_TAG) | sed 's/^v//' | cut -d. -f2)
    _PATCH := $(shell echo $(LATEST_TAG) | sed 's/^v//' | cut -d. -f3)
    VERSION := $(_MAJOR).$(_MINOR).$(shell echo $$(($(_PATCH) + 1)))
  else
    VERSION := 0.0.1
  endif
endif

# Validate VERSION looks like semver
check-version:
	@echo "$(VERSION)" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$$' || \
		(echo "ERROR: VERSION must be semver (e.g., 1.2.3), got: $(VERSION)" && exit 1)

# Refuse to release if HEAD is already at the latest tag or the tree is dirty
check-release-needed:
	@LAST_TAG=$$(git describe --tags --abbrev=0 --match 'v[0-9]*.[0-9]*.[0-9]*' 2>/dev/null); \
	if [ -n "$$LAST_TAG" ]; then \
		TAG_SHA=$$(git rev-list -n 1 "$$LAST_TAG"); \
		HEAD_SHA=$$(git rev-parse HEAD); \
		if [ "$$TAG_SHA" = "$$HEAD_SHA" ]; then \
			echo "ERROR: HEAD is already at $$LAST_TAG — nothing to release."; \
			echo "Commit changes before running release."; \
			exit 1; \
		fi; \
	fi
	@if ! git diff --quiet || ! git diff --cached --quiet; then \
		echo "ERROR: working tree has uncommitted changes. Commit or stash before releasing."; \
		git status --short; \
		exit 1; \
	fi

# Update version in all JSON manifests, bump changed blueprints, commit, and tag
release: check-version check-release-needed test-ts
	@echo "Releasing v$(VERSION)..."
	@for f in $(VERSION_FILES); do \
		sed -i 's/"version": *"[^"]*"/"version": "$(VERSION)"/' $$f; \
	done
	@echo "Updated: $(VERSION_FILES)"
	@command -v jq >/dev/null || { echo "jq is required: brew install jq / apt install jq"; exit 1; }
	@# The per-arch binary packages are pinned exactly, so their pins must move
	@# with the release — a stale pin silently ships the previous release's
	@# binaries. sed above only rewrites "version" keys, not dependency values.
	@PKG=packages/opencode-plugin/package.json; \
	jq --arg v "$(VERSION)" \
		'if .optionalDependencies then .optionalDependencies |= with_entries(.value = $$v) else . end' \
		"$$PKG" > "$$PKG.tmp" && mv "$$PKG.tmp" "$$PKG"
	@echo "Pinned optionalDependencies to $(VERSION)"
	@LAST_TAG=$$(git describe --tags --abbrev=0 --match 'v[0-9]*.[0-9]*.[0-9]*' 2>/dev/null || echo ""); \
	BP_DIR=aide/pkg/blueprint/blueprints; \
	if [ -n "$$LAST_TAG" ]; then \
		echo "Checking blueprints changed since $$LAST_TAG..."; \
		CHANGED=$$(git diff --name-only "$$LAST_TAG" HEAD -- "$$BP_DIR" | grep '\.json$$'); \
	else \
		echo "No previous tag found, updating all blueprints..."; \
		CHANGED=$$(ls "$$BP_DIR/"*.json 2>/dev/null); \
	fi; \
	UPDATED=0; \
	for f in $$CHANGED; do \
		if [ -f "$$f" ]; then \
			echo "  $$f → $(VERSION)"; \
			jq --arg v "$(VERSION)" '.version = $$v' "$$f" > "$$f.tmp" && mv "$$f.tmp" "$$f"; \
			UPDATED=$$((UPDATED + 1)); \
		fi; \
	done; \
	if [ "$$UPDATED" -gt 0 ]; then \
		echo "$$UPDATED blueprint(s) updated to $(VERSION)"; \
	else \
		echo "No blueprint files changed since $${LAST_TAG:-the beginning}"; \
	fi
	@echo "Snapshotting docs for v$(VERSION)..."
	@cd docs && bun install --frozen-lockfile && bun run docusaurus docs:version $(VERSION)
	@echo "Syncing bun.lock..."
	@bun install
	@echo ""
	@git diff --stat
	@echo ""
	@git add $(VERSION_FILES) bun.lock aide/pkg/blueprint/blueprints/ \
		docs/versions.json docs/versioned_docs/ docs/versioned_sidebars/
	@git commit -m "release: v$(VERSION)"
	@git tag -a "v$(VERSION)" -m "v$(VERSION)"
	@echo ""
	@echo "Tagged v$(VERSION). Push with:"
	@echo "  git push origin main v$(VERSION)"

# Release and push in one step
release-push: release
	git push origin main "v$(VERSION)"

# Idempotent. Also runs via package.json `prepare`; this is for Go-only devs.
hooks:
	@bunx --bun lefthook install >/dev/null 2>&1 && echo "hooks: lefthook installed" || \
		echo "hooks: could not install lefthook - run 'bun install', or see docs/getting-started/from-source"

# Warns only - a build must not need network or rewrite .git/hooks silently.
hooks-check:
	@[ -f .git/hooks/pre-commit ] || \
		echo "hooks: git hooks not installed - run 'make hooks' (decisions will not auto-export on commit)"

# Delegate to aide/ Makefile for Go targets
build: hooks-check
	$(MAKE) -C aide build

build-pprof:
	$(MAKE) -C aide build-pprof

test: hooks-check test-go test-ts

test-go:
	$(MAKE) -C aide test

test-ts:
	bunx vitest run --exclude='tests/memory-capture.test.ts' --exclude='dist/**'

build-web:
	$(MAKE) -C aide-web build

lint:
	$(MAKE) -C aide lint
