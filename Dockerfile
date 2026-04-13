# Build stage: Rust FFI + Go binary
FROM golang:1.25-bookworm AS builder

# Install Rust 1.90.0 (pinned for leansig/leanMultisig compatibility)
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain 1.90.0
ENV PATH="/root/.cargo/bin:${PATH}"

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    pkg-config \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy Rust FFI dependencies first for better caching
COPY xmss/rust/ xmss/rust/

# Build Rust FFI libraries
RUN cd xmss/rust && cargo build --release --locked

# Copy Go module files for dependency caching
COPY go.mod go.sum ./
RUN go mod download

# Copy all source code
COPY . .

# Build Go binaries
ARG GIT_COMMIT=unknown
ARG GIT_BRANCH=unknown
RUN mkdir -p bin && \
    go build -o bin/gean ./cmd/gean && \
    go build -o bin/keygen ./cmd/keygen

# Runtime stage
FROM ubuntu:24.04 AS runtime
WORKDIR /app

LABEL org.opencontainers.image.source=https://github.com/geanlabs/gean
LABEL org.opencontainers.image.description="Go Ethereum Lean Consensus Client"
LABEL org.opencontainers.image.licenses="MIT"

ARG GIT_COMMIT=unknown
ARG GIT_BRANCH=unknown
LABEL org.opencontainers.image.revision=$GIT_COMMIT
LABEL org.opencontainers.image.ref.name=$GIT_BRANCH

# Copy binaries
COPY --from=builder /app/bin/gean /usr/local/bin/
COPY --from=builder /app/bin/keygen /usr/local/bin/

# Copy only the .py source files needed at runtime by rec_aggregation's
# source fingerprint verification (reads .py files from CARGO_MANIFEST_DIR).
# Full checkout would add hundreds of MB; these 8 files are a few KB.
COPY --from=builder /root/.cargo/git/checkouts/leanmultisig-f4c4eb5eca99429a/fd88140/crates/rec_aggregation/*.py \
     /root/.cargo/git/checkouts/leanmultisig-f4c4eb5eca99429a/fd88140/crates/rec_aggregation/

# 9000/udp - P2P QUIC
# 5052 - API
# 5054 - Prometheus metrics
EXPOSE 9000/udp 5052 5054

ENTRYPOINT ["/usr/local/bin/gean"]
