.PHONY: build clean docker-build ffi fmt help lint loc test test-ffi

GIT_COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
GIT_BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)

XMSS_RUST := internal/core/crypto/xmss/rust

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Build

DOCKER_IMAGE ?= ghcr.io/geanlabs/gean
DOCKER_TAG ?= local

build: ## Build bin/gean
	@mkdir -p bin
	go build -o bin/gean ./cmd/gean

docker-build: ## Build the Docker image DOCKER_IMAGE:DOCKER_TAG
	docker build \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg GIT_BRANCH=$(GIT_BRANCH) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) .

##@ Test

test: ffi ## Run unit tests and the XMSS scheme vector (no other FFI tests)
	go test $$(go list ./... | grep -v '/crypto/xmss$$') -v -count=1
	go test ./internal/core/crypto/xmss/ -run '^TestSchemeKnownAnswer$$' -v -count=1

##@ Code

lint: ## Run linters; any finding fails
	@command -v golangci-lint >/dev/null || { echo "golangci-lint not found: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"; exit 1; }
	go vet ./...
	golangci-lint run ./...
	cd $(XMSS_RUST) && cargo fmt --check
	cd $(XMSS_RUST) && cargo clippy -- -D warnings -A clippy::missing_safety_doc

fmt: ## Format Go and Rust code
	gofmt -w .
	cd $(XMSS_RUST) && cargo fmt

clean: ## Remove build artifacts
	rm -rf bin $(XMSS_LIB)
	cd $(XMSS_RUST) && cargo clean

loc: ## Count lines of code per package; BASE=<ref> compares with that ref
	@scripts/loc.sh $(BASE)

##@ XMSS

# leanVM's prover does binary-field arithmetic with carryless multiplication. On x86 the
# Haswell baseline provides PCLMULQDQ; on aarch64 PMULL is gated behind the `aes` target
# feature, which Linux aarch64 targets do not enable by default. Without it the prover
# falls back to scalar code.
XMSS_RUSTFLAGS := $(strip \
	$(if $(filter x86_64,$(shell uname -m)),-Ctarget-cpu=haswell) \
	$(if $(filter aarch64 arm64,$(shell uname -m)),-Ctarget-feature=+aes))

# Go 1.26.11+ rejects any archive under a #cgo -L directory, walked recursively,
# that carries LLVM bitcode, and Rust's prebuilt compiler_builtins embeds it. The
# node links a stripped copy from a directory that holds only that copy, so
# rebuild the library through this target. macOS has no objcopy, so there the
# copy keeps its bitcode, which Go is likely to reject as well.
XMSS_LIB := internal/core/crypto/xmss/lib
XMSS_STRIP := $(if $(filter Darwin,$(shell uname -s)),cp,objcopy --remove-section=.llvmbc --remove-section=.llvmcmd)

ffi: ## Build the XMSS Rust library
	cd $(XMSS_RUST) && $(if $(XMSS_RUSTFLAGS),CARGO_ENCODED_RUSTFLAGS="$(XMSS_RUSTFLAGS)") cargo build --profile multisig-release --locked
	@mkdir -p $(XMSS_LIB)
	$(XMSS_STRIP) $(XMSS_RUST)/target/multisig-release/libcgo_glue.a $(XMSS_LIB)/libcgo_glue.a

test-ffi: ffi ## Run the XMSS FFI tests (slow)
	go test ./internal/core/crypto/xmss/ -v -count=1
