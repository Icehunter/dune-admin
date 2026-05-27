.PHONY: build web go linux dev dev-server setup deploy-web \
        vulncheck gosec pnpm-audit \
        test test-race vet fmt fmt-check \
        tools verify \
        version version-patch version-minor version-major

# ── Build ─────────────────────────────────────────────────────────────────────
BIN    := bin/dune-admin
PKG    := ./...
GO     := go
PREFIX ?= /usr/local

VERSION    ?= $(shell cat VERSION 2>/dev/null || git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS    := -ldflags "-s -w -X main.AppVersion=$(VERSION) -X main.GitCommit=$(GIT_COMMIT) -X main.BuildTime=$(BUILD_TIME)"

# Build the binary.
build:
	@mkdir -p bin
	$(GO) build -trimpath $(LDFLAGS) -o $(BIN) ./
	install -m 0755 $(BIN) ./dune-admin

# Install the binary system-wide.
install: build
	install -d $(DESTDIR)$(PREFIX)/bin
	install -m 0755 $(BIN) $(DESTDIR)$(PREFIX)/bin/dune-admin

linux:
	GOOS=linux GOARCH=amd64 go build -o dune-admin-linux .

dev-server:
	go run .

dev:
	go tool github.com/air-verse/air

setup:
	go run . -setup

# ── Web ───────────────────────────────────────────────────────────────────────

web:
	cd web && npm ci && npm run build

deploy-web:
	cd web && npm ci && npm run build && wrangler pages deploy dist --project-name dune-admin

# ── Test ──────────────────────────────────────────────────────────────────────

test:
	go test ./...

test-race:
	go test -race ./...

# ── Quality ───────────────────────────────────────────────────────────────────

vet:
	go vet ./...

fmt:
	go fmt ./...
	gofmt -s -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Code is not formatted. Run 'make fmt'" && exit 1)

vulncheck:
	go tool golang.org/x/vuln/cmd/govulncheck ./...

gosec:
	go tool github.com/securego/gosec/v2/cmd/gosec -severity high -confidence high ./...

pnpm-audit:
	cd web && pnpm audit --audit-level=high

verify:
	@$(MAKE) fmt-check && \
	$(MAKE) vet && \
	$(MAKE) test-race && \
	$(MAKE) vulncheck && \
	$(MAKE) gosec && \
	echo "All checks passed!"

# ── Tools ─────────────────────────────────────────────────────────────────────

tools:
	@echo "Caching dev tools (versions pinned in go.mod)..."
	@go tool golang.org/x/vuln/cmd/govulncheck -version || true
	@go tool github.com/securego/gosec/v2/cmd/gosec --version || true
	@go tool github.com/air-verse/air -v || true
	@echo "Done!"

# Print current version.
version:
	@echo $(VERSION)

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
