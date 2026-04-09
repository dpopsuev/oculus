# Oculus LSP Test Container
# Multi-language LSP servers for integration testing.
#
# Build:  docker build -t oculus-lsp-test lsp/testcontainer/
# Usage:  docker run -i --rm -v /path:/path -w /path oculus-lsp-test <lsp-cmd>
#
# Covered: Go (gopls), Python (pyright), TypeScript/JavaScript
#          (typescript-language-server), C/C++ (clangd), Rust (rust-analyzer)

FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

# System packages: build tools, Python 3, clangd
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl git xz-utils \
    gcc g++ make cmake \
    python3 python3-venv \
    clangd-18 \
    && ln -sf /usr/bin/clangd-18 /usr/bin/clangd \
    && rm -rf /var/lib/apt/lists/*

# Go + gopls
ARG GO_VERSION=1.22.5
RUN ARCH=$(dpkg --print-architecture) && \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${ARCH}.tar.gz" | \
    tar -C /usr/local -xz
ENV PATH="/usr/local/go/bin:/root/go/bin:${PATH}"
RUN go install golang.org/x/tools/gopls@latest

# Node.js (for TypeScript LSP + Pyright)
ARG NODE_VERSION=20.18.1
RUN ARCH=$(dpkg --print-architecture) && \
    case "$ARCH" in amd64) NODE_ARCH="x64" ;; arm64) NODE_ARCH="arm64" ;; *) exit 1 ;; esac && \
    curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODE_ARCH}.tar.xz" | \
    tar -C /usr/local --strip-components=1 -xJ

# TypeScript LSP + Pyright
RUN npm install -g typescript typescript-language-server pyright

# Rust + rust-analyzer
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y \
    --default-toolchain stable --profile minimal --component rust-analyzer
ENV PATH="/root/.cargo/bin:${PATH}"

WORKDIR /workspace
