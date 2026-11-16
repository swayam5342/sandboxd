# ==============================================================================
# STAGE 1: Build NsJail from source
# ==============================================================================
FROM debian:bookworm-slim AS nsjail-builder

# Install NsJail build-time dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    autoconf \
    bison \
    flex \
    gcc \
    g++ \
    git \
    libprotobuf-dev \
    libnl-route-3-dev \
    libtool \
    make \
    pkg-config \
    protobuf-compiler \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Clone and compile NsJail (using --recursive for kafel submodule)
RUN git clone --recursive https://github.com/google/nsjail.git /nsjail && \
    cd /nsjail && \
    make && \
    cp nsjail /usr/local/bin/nsjail

# ==============================================================================
# STAGE 2: Build the Go Sandboxd Application
# ==============================================================================
FROM golang:1.27-bookworm AS go-builder

WORKDIR /app

# Pre-copy go.mod and go.sum for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application
COPY . .

# Compile a statically linked Go binary, stamping the version and commit reported by GET /info
ARG VERSION=dev
ARG REVISION=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${REVISION}" \
    -o /app/sandboxd ./cmd/sandbox/main.go

# ==============================================================================
# STAGE 3: Final Production Runtime Image
# ==============================================================================
FROM debian:bookworm-slim

ARG VERSION=dev
ARG REVISION=unknown
ARG CREATED=unknown
LABEL org.opencontainers.image.title="sandboxd" \
      org.opencontainers.image.description="Run code you don't trust: compiles and runs untrusted code inside NsJail, as user nobody, with no network, behind a JSON API." \
      org.opencontainers.image.authors="swayam5342" \
      org.opencontainers.image.url="https://sandboxd.swayam.li/" \
      org.opencontainers.image.source="https://github.com/swayam5342/sandboxd" \
      org.opencontainers.image.documentation="https://github.com/swayam5342/sandboxd#readme" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.created="${CREATED}"

# This is the core image: sandboxd + nsjail, with bash as the only language.
# Language toolchains are NOT installed here. Build on top of this image and install the
# ones you need (see docker/full.Dockerfile for all eight). The jail bind-mounts this
# container's own /usr, /lib and /bin read-only, so a toolchain must live in the image.
RUN apt-get update && apt-get install -y --no-install-recommends \
    # NsJail dynamic link dependencies
    libprotobuf32 \
    libnl-route-3-200 \
    \
    # Security/runtime utilities
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Create working directories for the app
WORKDIR /app

# Copy NsJail binary from Stage 1 to /usr/sbin/nsjail as configured in application env
COPY --from=nsjail-builder /usr/local/bin/nsjail /usr/sbin/nsjail

# Copy sandboxd binary from Stage 2
COPY --from=go-builder /app/sandboxd /app/sandboxd

# Copy the nsjail config and the core language registry (bash only).
# Images built on top replace /app/config/lang.yaml with their own registry.
COPY config/nsjail.yaml /app/config/nsjail.yaml
COPY config/lang.core.yaml /app/config/lang.yaml

# Create base directory for jail sandboxes
RUN mkdir -p /tmp/sandboxd-jails && chmod 777 /tmp/sandboxd-jails

# Expose default application port
EXPOSE 8089

# Expose environment variables with sane defaults
ENV PORT=:8089
ENV LOG_LEVEL=info
ENV LOG_JSON=true
ENV NSJAIL_PATH=/usr/sbin/nsjail
ENV LANG_CONFIG=config/lang.yaml
ENV MAX_CONCURRENT=100
ENV NSJAIL_BASE_DIR=/tmp/sandboxd-jails

# Run the Go sandboxd application
ENTRYPOINT ["/app/sandboxd"]
