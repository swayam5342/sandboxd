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
FROM golang:1.25-bookworm AS go-builder

WORKDIR /app

# Pre-copy go.mod and go.sum for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application
COPY . .

# Compile a statically linked Go binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/sandboxd ./cmd/sandbox/main.go

# ==============================================================================
# STAGE 3: Final Production Runtime Image
# ==============================================================================
FROM debian:bookworm-slim

LABEL maintainer="Swayam <swayam@example.com>"
LABEL description="Secure Sandbox Runner Service powered by NsJail and Go"

# Install NsJail runtime libraries AND language toolchains
# Languages included: Python 3, Node.js, C/C++, Java (OpenJDK 17), Verilog (Icarus), Rust
RUN apt-get update && apt-get install -y --no-install-recommends \
    # NsJail dynamic link dependencies
    libprotobuf32 \
    libnl-route-3-200 \
    \
    # Language 1: Python 3
    python3 \
    python3-minimal \
    \
    # Language 2: Node.js
    nodejs \
    \
    # Language 3 & 4: C / C++ Compilers and Standard C library headers
    gcc \
    g++ \
    libc6-dev \
    \
    # Language 5: Java (OpenJDK 17 compiler & runtime)
    openjdk-17-jdk-headless \
    \
    # Language 6: Verilog (Icarus Verilog compiler & VVP runtime)
    iverilog \
    \
    # Language 7: Rust
    rustc \
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

# Copy default config and environment files
COPY config/ /app/config/

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
