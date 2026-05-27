ARG BUILDPLATFORM
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH

FROM --platform=$BUILDPLATFORM node:22-bookworm-slim AS web-builder

WORKDIR /build/web

COPY web/package.json web/pnpm-lock.yaml ./
RUN corepack enable && corepack prepare pnpm@10.28.1 --activate && pnpm install --frozen-lockfile

COPY web/ ./
RUN pnpm build

FROM --platform=$BUILDPLATFORM golang:1.26.3 AS go-builder

WORKDIR /build

ARG TARGETOS
ARG TARGETARCH

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -ldflags="-s -w" -o dune-admin ./cmd/dune-admin

FROM --platform=$TARGETPLATFORM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates kubernetes-client postgresql-client && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=go-builder /build/dune-admin .
COPY --from=go-builder /build/tags-data.json /build/item-data.json ./
COPY --from=web-builder /build/web/dist ./dist

EXPOSE 8080

ENTRYPOINT ["./dune-admin"]
