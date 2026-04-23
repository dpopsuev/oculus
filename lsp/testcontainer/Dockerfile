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

# System packages: build tools, Python 3
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl git xz-utils gnupg \
    gcc g++ make cmake \
    python3 python3-venv \
    && rm -rf /var/lib/apt/lists/*

# clangd 20 from LLVM apt repo (18 lacks callHierarchy/outgoingCalls)
ARG LLVM_VERSION=20
RUN curl -fsSL https://apt.llvm.org/llvm-snapshot.gpg.key | gpg --dearmor -o /usr/share/keyrings/llvm.gpg && \
    echo "deb [signed-by=/usr/share/keyrings/llvm.gpg] https://apt.llvm.org/noble/ llvm-toolchain-noble-${LLVM_VERSION} main" \
      > /etc/apt/sources.list.d/llvm.list && \
    apt-get update && apt-get install -y --no-install-recommends clangd-${LLVM_VERSION} && \
    ln -sf /usr/bin/clangd-${LLVM_VERSION} /usr/bin/clangd && \
    rm -rf /var/lib/apt/lists/*

# Go + gopls
ARG GO_VERSION=1.25.8
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
RUN npm install -g typescript@5 typescript-language-server pyright

# Rust + rust-analyzer
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y \
    --default-toolchain stable --profile minimal --component rust-analyzer
ENV PATH="/root/.cargo/bin:${PATH}"

WORKDIR /workspace
