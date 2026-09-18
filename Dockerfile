# Build stage: Rust FFI + Go binary
FROM golang:1.25-bookworm AS builder

# Install Rust 1.92.0 (pinned for leansig/leanMultisig compatibility)
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain 1.92.0
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

# Detect the build-stage architecture: legacy builders do not populate TARGETARCH.
# Same flags as make ffi: Haswell on x86_64, +aes (PMULL) on aarch64.
RUN cd xmss/rust && \
    case "$(uname -m)" in \
      x86_64) CARGO_ENCODED_RUSTFLAGS="-Ctarget-cpu=haswell" cargo build --profile multisig-release --locked ;; \
      aarch64) CARGO_ENCODED_RUSTFLAGS="-Ctarget-feature=+aes" cargo build --profile multisig-release --locked ;; \
      *) cargo build --profile multisig-release --locked ;; \
    esac

# Copy Go module files for dependency caching
COPY go.mod go.sum ./
RUN go mod download

# Copy all source code
COPY . .

# Build Go binaries
ARG GIT_COMMIT=unknown
ARG GIT_BRANCH=unknown
RUN mkdir -p bin && \
    go build -tags hive_testdriver -ldflags "-X github.com/geanlabs/gean/internal/node.gitCommit=$GIT_COMMIT" -o bin/gean ./cmd/gean && \
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


# Prove on jemalloc, not glibc malloc. The prover frees its scratch after every
# proof, but glibc keeps it: most lands in the main heap, which only shrinks from
# the top, so one live allocation pins everything below it. jemalloc uses mmap and
# returns pages on a decay timer. Devnet, same slot: 448 MB against 1,126-1,150 MB
# on glibc nodes. See #424.
#
# Unqualified soname so ld.so resolves it per architecture (this image builds
# arm64 too). The ldconfig check fails the build rather than letting a missing
# library become a silent runtime warning.
RUN apt-get update && apt-get install -y --no-install-recommends libjemalloc2 \
    && rm -rf /var/lib/apt/lists/* \
    && ldconfig -p | grep -q 'libjemalloc\.so\.2'
ENV LD_PRELOAD=libjemalloc.so.2

# Keep the Go heap tight so the XMSS prover's transient multi-GB proving
# peaks (allocated by the Rust arena, invisible to the Go GC) land on free
# memory instead of an uncollected heap. Operators can override.
ENV GOMEMLIMIT=2GiB

# 9000/udp - P2P QUIC
# 5052 - API
# 5054 - Prometheus metrics
EXPOSE 9000/udp 5052 5054

ENTRYPOINT ["/usr/local/bin/gean"]
