BINARY  := parrot
VERSION := 2.4
GOOS    ?= $(shell go env GOOS)
GOARCH  ?= $(shell go env GOARCH)

.DEFAULT_GOAL := build

# ── Build ────────────────────────────────────────────────────────────────────

.PHONY: build
build:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BINARY) .

.PHONY: run
run:
	go run .

.PHONY: dev
dev:
	go run . -count 3 -rate-limit 10 -delay 50ms

# ── Quality ──────────────────────────────────────────────────────────────────

.PHONY: vet
vet:
	go vet ./...

.PHONY: lint
lint:
	@which staticcheck > /dev/null 2>&1 || go install honnef.co/go/tools/cmd/staticcheck@latest
	staticcheck ./...

.PHONY: check
check: vet lint

# ── Cross-compilation ────────────────────────────────────────────────────────

.PHONY: build-all
build-all:
	GOOS=linux   GOARCH=amd64  go build -ldflags="-s -w" -o dist/$(BINARY)-linux-amd64   .
	GOOS=linux   GOARCH=arm64  go build -ldflags="-s -w" -o dist/$(BINARY)-linux-arm64   .
	GOOS=darwin  GOARCH=amd64  go build -ldflags="-s -w" -o dist/$(BINARY)-darwin-amd64  .
	GOOS=darwin  GOARCH=arm64  go build -ldflags="-s -w" -o dist/$(BINARY)-darwin-arm64  .
	GOOS=windows GOARCH=amd64  go build -ldflags="-s -w" -o dist/$(BINARY)-windows-amd64.exe .

# ── Clean ────────────────────────────────────────────────────────────────────

.PHONY: clean
clean:
	rm -f $(BINARY)
	rm -rf dist/

.PHONY: help
help:
	@echo ""
	@echo "  🦜 parrot $(VERSION)"
	@echo ""
	@echo "  build       Build for current platform (default)"
	@echo "  run         Run with default flags"
	@echo "  dev         Run with 3 instances, rate limiting, and a small delay"
	@echo "  vet         Run go vet"
	@echo "  lint        Run staticcheck (installs if missing)"
	@echo "  check       Run vet + lint"
	@echo "  build-all   Cross-compile for linux/darwin/windows (amd64 + arm64)"
	@echo "  clean       Remove binary and dist/"
	@echo ""
