ARG BUILDPLATFORM
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH

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

EXPOSE 8080

ENTRYPOINT ["./dune-admin"]
