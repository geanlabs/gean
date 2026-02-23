# Rust Builder for leansig-ffi
FROM rust:alpine AS rust-builder
RUN apk add --no-cache musl-dev

WORKDIR /build
COPY xmss/leansig-ffi xmss/leansig-ffi/

WORKDIR /build/xmss/leansig-ffi
RUN cargo build --release

# Go Builder for gean
FROM golang:1.24-alpine AS go-builder
RUN apk add --no-cache git build-base musl-dev gcc pkgconf

WORKDIR /build

# Copy Go modules manifests
COPY go.mod go.sum ./
RUN go mod download

# Copy Go source code
COPY . .

# Copy Rust compiled static library and headers
# leansig.go expects the header in ../leansig-ffi/include and the lib in ../leansig-ffi/target/release/deps/
COPY --from=rust-builder /build/xmss/leansig-ffi/target/release/deps/libleansig_ffi.a xmss/leansig-ffi/target/release/deps/
COPY --from=rust-builder /build/xmss/leansig-ffi/include xmss/leansig-ffi/include/

# Build Go binary including CGO binding
RUN CGO_ENABLED=1 go build -o /build/gean ./cmd/gean

# Runtime minimal image
FROM alpine:3.21

# libgcc is needed for cgo execution
RUN apk add --no-cache ca-certificates libgcc
COPY --from=go-builder /build/gean /usr/local/bin/gean

ENTRYPOINT ["gean"]