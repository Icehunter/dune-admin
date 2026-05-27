FROM golang:1.26.3 AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dune-admin ./cmd/dune-admin

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /build/dune-admin .
COPY --from=builder /build/tags-data.json /build/item-data.json ./

EXPOSE 8080

ENTRYPOINT ["./dune-admin"]
