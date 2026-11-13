# qw: QuenchWorks images tooling. Every target runs from here; qw finds the images
# root (catalog.yaml) by walking up, so paths in ARGS are relative to images/.
#   make check ARGS="-q"        make bump ARGS="opa 1.23.0"        make test

SHELL := /bin/bash
.SHELLFLAGS := -euo pipefail -c
.DEFAULT_GOAL := help
MAKEFLAGS += --no-print-directory

BIN  ?= bin/qw
ARGS ?=
COMMANDS := check bump gen-catalog drift-scan vex blocked unbuilt wolfi-latest

.PHONY: help build test vet fmt lint clean $(COMMANDS)

help: ## list targets
	@grep -hE '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  make %-8s %s\n", $$1, $$2}'
	@echo "  make <command> ARGS=...   one per qw command:"
	@go run . help 2>&1 | tail -n +2

build: ## compile to $(BIN)
	go build -o $(BIN) .

test: ## run the unit tests
	go test ./...

vet: ## go vet
	go vet ./...

fmt: ## gofmt in place
	gofmt -w .

lint: ## fail on unformatted files, then vet
	@out="$$(gofmt -l .)"; [ -z "$$out" ] || { echo "gofmt needed:"; echo "$$out"; exit 1; }
	go vet ./...

clean: ## remove the built binary
	rm -f $(BIN)

$(COMMANDS):
	go run . $@ $(ARGS)
