ARG BUILDPLATFORM
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH

# ── Frontend build ────────────────────────────────────────────────────────────
FROM --platform=$BUILDPLATFORM node:22-slim AS web-builder

WORKDIR /build/web

# Install pnpm via corepack (matches packageManager field in package.json)
RUN corepack enable && corepack prepare pnpm@10.28.1 --activate

COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY web/ ./
# No VITE_CDN_BASE_URL → isCdnDeploy=false → SPA uses same-origin API base.
# VITE_MARKET_BOT_ENABLED=true shows the Bot Control button in the UI.
RUN VITE_MARKET_BOT_ENABLED=true pnpm build

# ── Go build ──────────────────────────────────────────────────────────────────
FROM --platform=$BUILDPLATFORM golang:1.26.3 AS go-builder

WORKDIR /build

ARG TARGETOS
ARG TARGETARCH
ARG APP_VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build \
    -ldflags="-s -w -X main.AppVersion=${APP_VERSION} -X main.GitCommit=${GIT_COMMIT} -X main.BuildTime=${BUILD_TIME}" \
    -o dune-admin ./cmd/dune-admin

# ── Runtime ───────────────────────────────────────────────────────────────────
FROM --platform=$TARGETPLATFORM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates kubernetes-client postgresql-client && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=go-builder /build/dune-admin .
COPY --from=go-builder /build/tags-data.json /build/item-data.json ./
COPY --from=web-builder /build/web/dist ./dist

EXPOSE 8080

ENTRYPOINT ["./dune-admin"]
