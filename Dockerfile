# Multi-stage build for optimal image size
FROM golang:1.25-bookworm AS builder

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates tzdata build-essential \
    && rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /build

# Copy the whole tree: go.mod has local replace directives for the grammar
# modules under internal/parser/grammars, so go.mod alone cannot resolve.
COPY . .

# Build arguments for version info
ARG VERSION=dev
ARG COMMIT=unknown

# Build the binary (cgo is mandatory: the tree-sitter parsers are C libraries)
RUN CGO_ENABLED=1 CGO_CFLAGS="-O2 -w" go build \
    -ldflags="-s -w -X github.com/janreges/ai-distiller/internal/version.Version=${VERSION} -X github.com/janreges/ai-distiller/internal/version.Commit=${COMMIT}" \
    -o aid ./cmd/aid/

# Production image
FROM debian:bookworm-slim

# CA certificates for HTTPS support
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy the binary
COPY --from=builder /build/aid /usr/local/bin/aid

# Set the binary as entrypoint
ENTRYPOINT ["/usr/local/bin/aid"]

# Default command
CMD ["--help"]

# Labels for better maintainability
LABEL org.opencontainers.image.title="AI Distiller"
LABEL org.opencontainers.image.description="High-performance CLI tool for extracting essential code structure from large codebases"
LABEL org.opencontainers.image.url="https://github.com/janreges/ai-distiller"
LABEL org.opencontainers.image.source="https://github.com/janreges/ai-distiller"
LABEL org.opencontainers.image.vendor="Jan Reges"
LABEL org.opencontainers.image.licenses="MIT"