# aide project Makefile
#
# Usage:
#   make release                     Auto-bump patch version, commit, and tag
#   make release VERSION=1.2.0       Bump to specific version, commit, and tag
#   make release-push                Auto-bump, commit, tag, and push
#   make release-push VERSION=1.2.0  Bump to specific, commit, tag, and push

.PHONY: release release-push build test lint

VERSION_FILES = package.json .claude-plugin/plugin.json .claude-plugin/marketplace.json

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

# Update version in all JSON manifests, commit, and tag
release: check-version
	@echo "Releasing v$(VERSION)..."
	@for f in $(VERSION_FILES); do \
		sed -i 's/"version": *"[^"]*"/"version": "$(VERSION)"/' $$f; \
	done
	@echo "Updated: $(VERSION_FILES)"
	@echo ""
	@git diff --stat
	@echo ""
	@git add $(VERSION_FILES)
	@git commit -m "release: v$(VERSION)"
	@git tag -a "v$(VERSION)" -m "v$(VERSION)"
	@echo ""
	@echo "Tagged v$(VERSION). Push with:"
	@echo "  git push origin main v$(VERSION)"

# Release and push in one step
release-push: release
	git push origin main "v$(VERSION)"

# Delegate to aide/ Makefile for Go targets
build:
	$(MAKE) -C aide build

test:
	$(MAKE) -C aide test

lint:
	$(MAKE) -C aide lint
