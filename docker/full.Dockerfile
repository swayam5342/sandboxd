# ==============================================================================
# sandboxd with all eight languages, built on the published core image.
#
#   docker compose up --build          (uses this file, see docker-compose.yml)
#   docker build -f docker/full.Dockerfile -t sandboxd-full .
#
# The core image (swayam5342/sandboxd:core) already has sandboxd, nsjail and bash.
# This file only adds toolchains and swaps in the full registry from config/lang.yaml.
# To make your own: keep the FROM line, install the toolchains you want, and COPY a
# lang.yaml that lists only those languages.
# ==============================================================================
ARG CORE_IMAGE=swayam5342/sandboxd:core
FROM ${CORE_IMAGE}

RUN apt-get update && apt-get install -y --no-install-recommends \
    # Python 3
    python3 \
    python3-minimal \
    \
    # JavaScript (Node.js)
    nodejs \
    \
    # C / C++ compilers and the standard C library headers
    gcc \
    g++ \
    libc6-dev \
    \
    # Java (OpenJDK 17 compiler and runtime)
    openjdk-17-jdk-headless \
    \
    # Verilog (Icarus Verilog compiler and vvp runtime)
    iverilog \
    \
    # Rust
    rustc \
    && rm -rf /var/lib/apt/lists/*

# The full registry: every language installed above, plus bash from core.
COPY config/lang.yaml /app/config/lang.yaml
