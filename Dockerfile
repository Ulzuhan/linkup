# ==============================================================================
# Build Stage
# ==============================================================================
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Install security certificates & build essentials
RUN apk add --no-cache ca-certificates tzdata

# Cache Go dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Compile optimized static Go binary (pure Go, CGO disabled)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -extldflags '-static'" \
    -trimpath \
    -o /build/bin/linkup ./cmd/linkup

# ==============================================================================
# Production Runtime Stage (Hardened, Non-Root, Minimal)
# ==============================================================================
FROM alpine:3.21

# Create non-root system user and group (UID 10001)
RUN addgroup -g 10001 -S linkup && \
    adduser -u 10001 -S linkup -G linkup && \
    mkdir -p /data && \
    chown -R linkup:linkup /data

# Import CA certs & timezone data from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /build/bin/linkup /usr/local/bin/linkup

# Switch to non-root user
USER linkup:linkup

# Storage volume for SQLite database
VOLUME ["/data"]

EXPOSE 3464

ENV LINKUP_PORT=3464 \
    LINKUP_HOST=0.0.0.0 \
    LINKUP_DB_PATH=/data/linkup.db

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["wget", "--no-verbose", "--tries=1", "--spider", "http://127.0.0.1:3464/healthz"]

ENTRYPOINT ["/usr/local/bin/linkup"]
