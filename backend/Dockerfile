# syntax=docker/dockerfile:1
FROM golang:alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

# Cache module downloads
COPY go.mod go.sum ./
RUN go mod download

# Copy backend source code
COPY . .

# Build statically linked standalone binary
ARG VERSION=docker
ARG BUILD_ID=docker
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ) -X main.buildID=${BUILD_ID}" \
    -o /app/santral ./cmd/santral

# Minimal runtime image
FROM alpine:3.21

# ca-certificates for external HTTPS (Meta, Google Drive, Verimor API)
# tzdata for Turkey timezone
# postgresql16-client for automated pg_dump backups
# curl for healthchecks
RUN apk add --no-cache ca-certificates tzdata postgresql16-client curl

WORKDIR /app

RUN addgroup -S santral && adduser -S santral -G santral

COPY --from=builder /app/santral /app/santral
RUN chown -R santral:santral /app

USER santral

EXPOSE 8090

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
  CMD curl -fsS http://127.0.0.1:8090/healthz || exit 1

ENTRYPOINT ["/app/santral"]
CMD ["-config", "/app/config.yml"]
