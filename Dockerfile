# Build stage
FROM golang:1.26-bookworm AS builder

WORKDIR /app

# Copy Go module files for dependency caching
COPY go.mod go.sum ./
RUN go mod download

# Copy all source code
COPY . .

# Build Go binaries
RUN make build

# Runtime stage
FROM ubuntu:24.04 AS runtime
WORKDIR /app

LABEL org.opencontainers.image.source=https://github.com/geanlabs/gean
LABEL org.opencontainers.image.description="Go post-quantum Ethereum consensus client"
LABEL org.opencontainers.image.licenses="MIT"

ARG GIT_COMMIT=unknown
ARG GIT_BRANCH=unknown
LABEL org.opencontainers.image.revision=$GIT_COMMIT
LABEL org.opencontainers.image.ref.name=$GIT_BRANCH

# Copy the binary
COPY --from=builder /app/bin/gean /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/gean"]
