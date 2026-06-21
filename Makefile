# ============================================================================
# QuenchWorks images — local build pipeline
# ----------------------------------------------------------------------------
# Builds hardened, 0-CVE, multi-arch images locally and publishes them to GHCR,
# signed + attested with a local cosign key. This replaces GitHub Actions for
# the (private) images repo — the 2,000 free private-repo minutes can't cover a
# daily multi-arch rebuild of the whole catalog. Charts/common stay on Actions.
# ============================================================================

SHELL := /bin/bash
.SHELLFLAGS := -euo pipefail -c

.DEFAULT_GOAL := help
MAKEFLAGS += --warn-undefined-variables --no-print-directory

# ------------------------------------------
# Settings
# ------------------------------------------
OWNER       ?= quenchworks
GHCR        ?= ghcr.io/$(OWNER)/images
# arm64 (aarch64) is paused: building it via qemu emulation is slow and flaky
# (e.g. Erlang/rebar get_cwd failures). Until a native arm64 builder exists,
# default to amd64 only. Re-enable per-invocation: ARCHES=x86_64,aarch64 make …
ARCHES      ?= x86_64,aarch64
PUSH        ?= 1
APP         ?=
VERSION     ?=
COSIGN_KEY  ?= $(CURDIR)/.secrets/cosign.key
COSIGN_PUB  ?= $(CURDIR)/cosign.pub
BUILD       := ARCHES=$(ARCHES) GHCR_OWNER=$(OWNER) COSIGN_KEY=$(COSIGN_KEY) scripts/build-image.sh

# ------------------------------------------
# Logging
# ------------------------------------------
TIMESTAMP := $(shell date +"%Y-%m-%d_%H-%M-%S")
DATE      := $(shell date +"%Y-%m-%d")
LOG_DIR    = logs/$(DATE)
LOG_FILE   = $(LOG_DIR)/$(CMD)_$(TIMESTAMP).log

define run_with_log
	@mkdir -p $(LOG_DIR)
	@echo "===== $(CMD) START $(TIMESTAMP) =====" | tee -a $(LOG_FILE)
	@{ $(1) ; } 2>&1 | sed 's/\x1b\[[0-9;]*m//g' | tee -a $(LOG_FILE); EXIT=$${PIPESTATUS[0]}; \
	echo "===== $(CMD) END (EXIT: $$EXIT) =====" | tee -a $(LOG_FILE); exit $$EXIT
endef

# ------------------------------------------
# Pre-flight
# ------------------------------------------
.PHONY: check-tools
check-tools:
	@for t in apko melange trivy syft cosign crane docker; do \
		command -v $$t >/dev/null 2>&1 || { echo "❌ $$t not installed"; exit 1; }; done
	@echo "✅ build tools present (apko, melange, trivy, syft, cosign, crane, docker)"

.PHONY: check-key
check-key:
	@[ -f "$(COSIGN_KEY)" ] || { echo "❌ no signing key at $(COSIGN_KEY) — run 'make keygen'"; exit 1; }
	@echo "✅ signing key present"

.PHONY: require-app
require-app:
	@[ -n "$(APP)" ] || { echo "❌ APP is required, e.g. make build APP=redis"; exit 1; }
	@[ -d "apps/$(APP)" ] || { echo "❌ no such app: apps/$(APP)"; exit 1; }

# ------------------------------------------
# One-time setup
# ------------------------------------------
.PHONY: keygen
keygen:
	@if [ -f "$(COSIGN_KEY)" ]; then echo "🔑 key already exists at $(COSIGN_KEY) (not overwriting)"; exit 0; fi
	@mkdir -p .secrets && printf '%s\n' '*' > .secrets/.gitignore
	@PW="$$(openssl rand -hex 24)"; printf '%s' "$$PW" > .secrets/cosign.password; chmod 600 .secrets/cosign.password; \
	COSIGN_PASSWORD="$$PW" cosign generate-key-pair --output-key-prefix .secrets/cosign
	@chmod 600 .secrets/cosign.key && cp .secrets/cosign.pub cosign.pub
	@echo "✅ key pair created. Private: .secrets/cosign.key (gitignored). Public: cosign.pub (commit + publish on the site)."

.PHONY: login
login:
	@[ -n "$${GHCR_TOKEN:-}" ] || { echo "❌ set GHCR_TOKEN (a PAT with write:packages) first"; exit 1; }
	@echo "$$GHCR_TOKEN" | docker login ghcr.io -u "$(OWNER)" --password-stdin >/dev/null && echo "✅ logged in to ghcr.io"

# ------------------------------------------
# Build
# ------------------------------------------
.PHONY: build
build: check-tools check-key require-app
	$(eval CMD=build-$(APP))
	@[ -n "$(VERSION)" ] && echo "🏗  building $(APP):$(VERSION) → $(GHCR)/$(APP)" || echo "🏗  building ALL versions of $(APP) → $(GHCR)/$(APP)"
	$(call run_with_log,if [ -n "$(VERSION)" ]; then PUSH=$(PUSH) $(BUILD) "$(APP)" "$(VERSION)"; else PUSH=$(PUSH) ARCHES=$(ARCHES) GHCR_OWNER=$(OWNER) COSIGN_KEY=$(COSIGN_KEY) scripts/build-all.sh "$(APP)"; fi)
	@echo "📊 log: $(LOG_FILE)"

.PHONY: scan
scan: check-tools require-app
	$(eval CMD=scan-$(APP))
	@echo "🛡  build + 0-CVE scan only (no publish) for $(APP)"
	$(call run_with_log,PUSH=0 $(BUILD) "$(APP)" "$(VERSION)")

.PHONY: build-all
build-all: check-tools check-key
	$(eval CMD=build-all)
	@echo "🏗  building every app, every version, one at a time (arches=$(ARCHES), push=$(PUSH))…"
	$(call run_with_log,PUSH=$(PUSH) ARCHES=$(ARCHES) GHCR_OWNER=$(OWNER) COSIGN_KEY=$(COSIGN_KEY) scripts/build-all.sh)

# Whole catalog, JOBS apps in parallel (each app builds all its versions in
# order). The native build strategy: 2 at a time, next starts as one finishes.
# Optional APPS="redis nginx" limits to a subset.
JOBS ?= 2
.PHONY: catalog
catalog: check-tools check-key
	$(eval CMD=catalog)
	@echo "🏗  catalog — $(JOBS) apps in parallel, every version, multi-arch (arches=$(ARCHES), push=$(PUSH))…"
	$(call run_with_log,PUSH=$(PUSH) ARCHES=$(ARCHES) GHCR_OWNER=$(OWNER) COSIGN_KEY=$(COSIGN_KEY) JOBS=$(JOBS) scripts/build-catalog.sh $(APPS))

# ------------------------------------------
# Verify (what your users run, key-based)
# ------------------------------------------
.PHONY: verify
verify: require-app
	$(eval V=$(if $(VERSION),$(VERSION),latest))
	@echo "🔎 verifying $(GHCR)/$(APP):$(V) against $(COSIGN_PUB)"
	@cosign verify --key $(COSIGN_PUB) $(GHCR)/$(APP):$(V) >/dev/null && echo "✅ signature OK"
	@cosign verify-attestation --key $(COSIGN_PUB) --type spdxjson $(GHCR)/$(APP):$(V) >/dev/null && echo "✅ SBOM attestation OK"
	@cosign verify-attestation --key $(COSIGN_PUB) --type slsaprovenance $(GHCR)/$(APP):$(V) >/dev/null && echo "✅ provenance attestation OK"

.PHONY: pubkey
pubkey:
	@cat $(COSIGN_PUB)

# ------------------------------------------
# Housekeeping
# ------------------------------------------
.PHONY: clean
clean:
	@find apps -maxdepth 2 -type d -name packages -exec rm -rf {} + 2>/dev/null || true
	@find apps -maxdepth 2 \( -name '*.tar' -o -name 'melange.rsa*' -o -name 'apko.rendered.yaml' -o -name 'sbom.spdx.json' -o -name 'provenance.json' \) -delete 2>/dev/null || true
	@echo "✅ build artifacts cleaned"

.PHONY: logs
logs:
	@ls -lah logs/$(DATE)/ 2>/dev/null || echo "no logs today"

.PHONY: version
version:
	@for t in apko melange trivy syft cosign crane; do printf '%-9s ' "$$t"; $$t version 2>&1 | head -1; done

# ------------------------------------------
# Help
# ------------------------------------------
.PHONY: help
help:
	@echo ""
	@echo "============================================================"
	@echo "  QuenchWorks images — local build pipeline"
	@echo "============================================================"
	@echo ""
	@echo "ONE-TIME SETUP"
	@echo "  make keygen                 - create the cosign signing key (.secrets/, gitignored)"
	@echo "  GHCR_TOKEN=… make login      - docker login ghcr.io (PAT with write:packages)"
	@echo ""
	@echo "BUILD"
	@echo "  make build APP=redis              - build+scan+publish+sign+attest one image (multi-arch)"
	@echo "  make build APP=node VERSION=24.16.0 - build a specific version (multi-version apps)"
	@echo "  make scan  APP=redis              - build + 0-CVE gate only, no publish (PUSH=0)"
	@echo "  make build-all                    - build every app, one at a time"
	@echo "  ARCHES=x86_64 make build APP=redis - amd64-only (skip arm64/qemu)"
	@echo ""
	@echo "VERIFY (what users run — key-based)"
	@echo "  make verify APP=redis [VERSION=…] - cosign verify signature + SBOM + provenance"
	@echo "  make pubkey                       - print the public key (cosign.pub)"
	@echo ""
	@echo "HOUSEKEEPING"
	@echo "  make check-tools   make version   make logs   make clean"
	@echo ""
	@echo "NOTES"
	@echo "  • Signing is KEY-BASED (no GitHub OIDC locally). Verifiers use --key cosign.pub."
	@echo "  • From-source apps (melange.yaml) build the package first; arm64 uses qemu/binfmt."
	@echo "  • Placeholder/runtime apps (node/python/… with __VER__) need a render step (WIP)."
	@echo ""
