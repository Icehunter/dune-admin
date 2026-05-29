.PHONY: build web go linux dev dev-server dev-backend dev-web setup deploy-web \
        render-k8s render-k8s-stdout k8s-dry-run \
        vulncheck gosec pnpm-audit \
        test test-race vet fmt fmt-check \
        tools verify \
        version version-patch version-minor version-major

# ── Build ─────────────────────────────────────────────────────────────────────
CMD    := ./cmd/dune-admin
PKG    := ./...
GO     := go
PREFIX ?= /usr/local
COGNIT_TARGET := $(if $(wildcard cmd/dune-admin),./cmd/dune-admin,.)

# Windows produces .exe binaries. On Windows, bare `make` runs recipes under
# cmd.exe, which lacks POSIX `mkdir -p` and `install` — so the `go` target
# branches on OS below.
ifeq ($(OS),Windows_NT)
BIN       := bin/dune-admin.exe
LOCAL_BIN := dune-admin.exe
else
BIN       := bin/dune-admin
LOCAL_BIN := dune-admin
endif

VERSION    ?= $(shell cat VERSION 2>/dev/null || git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS    := -ldflags "-s -w -X main.AppVersion=$(VERSION) -X main.GitCommit=$(GIT_COMMIT) -X main.BuildTime=$(BUILD_TIME)"

# Build frontend + backend binary.
build: web go

# Build backend binary only.
go:
ifeq ($(OS),Windows_NT)
	@if not exist bin mkdir bin
	$(GO) build -trimpath $(LDFLAGS) -o $(BIN) $(CMD)
	@copy /Y "bin\dune-admin.exe" "$(LOCAL_BIN)" >NUL
else
	@mkdir -p bin
	$(GO) build -trimpath $(LDFLAGS) -o $(BIN) $(CMD)
	install -m 0755 $(BIN) ./$(LOCAL_BIN)
endif

# Install the binary system-wide.
install: go
	install -d $(DESTDIR)$(PREFIX)/bin
	install -m 0755 $(BIN) $(DESTDIR)$(PREFIX)/bin/dune-admin

linux:
	GOOS=linux GOARCH=amd64 $(GO) build -trimpath $(LDFLAGS) -o dune-admin-linux $(CMD)

dev-server:
	go run $(CMD)

dev-backend:
	go tool github.com/air-verse/air

dev-web:
	cd web && pnpm dev

dev:
	@set -e; \
	AIR_PID=; VITE_PID=; \
	cleanup() { \
		trap - EXIT INT TERM; \
		[ -n "$$AIR_PID" ] && kill $$AIR_PID 2>/dev/null || true; \
		[ -n "$$VITE_PID" ] && kill $$VITE_PID 2>/dev/null || true; \
		[ -n "$$AIR_PID" ] && wait $$AIR_PID 2>/dev/null || true; \
		[ -n "$$VITE_PID" ] && wait $$VITE_PID 2>/dev/null || true; \
	}; \
	trap 'cleanup' EXIT INT TERM; \
	$(MAKE) dev-backend & AIR_PID=$$!; \
	$(MAKE) dev-web & VITE_PID=$$!; \
	set +e; \
	while kill -0 $$AIR_PID 2>/dev/null && kill -0 $$VITE_PID 2>/dev/null; do \
		sleep 1; \
	done; \
	if ! kill -0 $$AIR_PID 2>/dev/null; then \
		wait $$AIR_PID; status=$$?; \
		kill $$VITE_PID 2>/dev/null || true; \
		wait $$VITE_PID 2>/dev/null || true; \
	else \
		wait $$VITE_PID; status=$$?; \
		kill $$AIR_PID 2>/dev/null || true; \
		wait $$AIR_PID 2>/dev/null || true; \
	fi; \
	exit $$status

setup:
	go run $(CMD) -setup

# ── Web ───────────────────────────────────────────────────────────────────────

web:
	cd web && pnpm install --frozen-lockfile && pnpm build

deploy-web:
	cd web && pnpm install --frozen-lockfile && pnpm build && wrangler pages deploy dist --project-name dune-admin

render-k8s:
	go run $(CMD) -render-k8s deploy/k8s/dune-admin.rendered.yaml

render-k8s-stdout:
	go run $(CMD) -render-k8s -

k8s-dry-run:
	@$(MAKE) render-k8s-stdout | kubectl apply --dry-run=client -f -

# ── Test ──────────────────────────────────────────────────────────────────────

test:
	go test $(PKG)

test-race:
	go test -race $(PKG)

# ── Quality ───────────────────────────────────────────────────────────────────

vet:
	go vet $(PKG)

fmt:
	go fmt $(PKG)
	gofmt -s -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Code is not formatted. Run 'make fmt'" && exit 1)

vulncheck:
	go tool golang.org/x/vuln/cmd/govulncheck $(PKG)

gocognit:
	@echo "Running code complexity analysis with gocognit..."
	@$(GO) tool github.com/uudashr/gocognit/cmd/gocognit -over 15 -ignore "_test|node_modules" $(COGNIT_TARGET) \
		> /tmp/gocognit-out.txt 2>&1 || true; \
	grep -v '^#' .gocognit-ignore | awk '{print $$1}' > /tmp/gocognit-ignore.txt; \
	grep -v -F -f /tmp/gocognit-ignore.txt /tmp/gocognit-out.txt > /tmp/gocognit-new.txt || true; \
	if [ -s /tmp/gocognit-new.txt ]; then cat /tmp/gocognit-new.txt; exit 1; fi

gosec:
	go tool github.com/securego/gosec/v2/cmd/gosec -severity high -confidence high $(PKG)

pnpm-audit:
	cd web && pnpm audit --audit-level=high

lint:
	@$(MAKE) lint-go
	@$(MAKE) lint-md

lint-go:
	@$(GO) tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint run

lint-md:
	@npx -y markdownlint-cli2 --fix "**/*.md"

verify:
	@$(MAKE) fmt-check && \
	$(MAKE) vet && \
	$(MAKE) test-race && \
	$(MAKE) vulncheck && \
	$(MAKE) lint && \
	$(MAKE) gocognit && \
	echo "All verification checks passed!"

# ── Tools ─────────────────────────────────────────────────────────────────────

tools:
	@echo "Caching dev tools (versions pinned in go.mod)..."
	@$(GO) tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint --version || true
	@$(GO) tool github.com/air-verse/air -v || true
	@$(GO) tool golang.org/x/vuln/cmd/govulncheck -version || true
	@$(GO) tool github.com/uudashr/gocognit/cmd/gocognit -version || true
	@$(GO) tool github.com/securego/gosec/v2/cmd/gosec --version || true
	@echo "Done!"

# Print current version.
version:
	@echo $(VERSION)

# Setup git hooks
hooks:
	@git config core.hooksPath .githooks
	@echo "Git hooks configured!"

# Bump patch version (1.0.0 → 1.0.1), commit, tag, and push — triggers release workflow.
version-patch:
	@V=$$(cat VERSION); \
	MAJOR=$$(echo $$V | cut -d. -f1); \
	MINOR=$$(echo $$V | cut -d. -f2); \
	PATCH=$$(echo $$V | cut -d. -f3); \
	NEW="$$MAJOR.$$MINOR.$$((PATCH + 1))"; \
	printf "Push tag v$$NEW to origin? [y/N] "; read ans; [ "$$ans" = "y" ] || { echo "Aborted."; exit 1; }; \
	echo $$NEW > VERSION; \
	git add VERSION && git commit -m "chore: bump version to $$NEW" && git tag "v$$NEW"; \
	git push && git push origin "v$$NEW"; \
	echo "Bumped $$V -> $$NEW (tagged and pushed v$$NEW)"

# Bump minor version (1.0.0 → 1.1.0), commit, tag, and push — triggers release workflow.
version-minor:
	@V=$$(cat VERSION); \
	MAJOR=$$(echo $$V | cut -d. -f1); \
	MINOR=$$(echo $$V | cut -d. -f2); \
	NEW="$$MAJOR.$$((MINOR + 1)).0"; \
	printf "Push tag v$$NEW to origin? [y/N] "; read ans; [ "$$ans" = "y" ] || { echo "Aborted."; exit 1; }; \
	echo $$NEW > VERSION; \
	git add VERSION && git commit -m "chore: bump version to $$NEW" && git tag "v$$NEW"; \
	git push && git push origin "v$$NEW"; \
	echo "Bumped $$V -> $$NEW (tagged and pushed v$$NEW)"

# Bump major version (1.0.0 → 2.0.0), commit, tag, and push — triggers release workflow.
version-major:
	@V=$$(cat VERSION); \
	MAJOR=$$(echo $$V | cut -d. -f1); \
	NEW="$$((MAJOR + 1)).0.0"; \
	printf "Push tag v$$NEW to origin? [y/N] "; read ans; [ "$$ans" = "y" ] || { echo "Aborted."; exit 1; }; \
	echo $$NEW > VERSION; \
	git add VERSION && git commit -m "chore: bump version to $$NEW" && git tag "v$$NEW"; \
	git push && git push origin "v$$NEW"; \
	echo "Bumped $$V -> $$NEW (tagged and pushed v$$NEW)"
