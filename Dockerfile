# ==============================================================================
# Build Stage
# ==============================================================================
FROM golang:1.27.1-alpine@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS builder

# The selected image is the compiler: never silently download another toolchain.
ENV GOTOOLCHAIN=local

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
FROM alpine:3.21@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d

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
