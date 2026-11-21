.PHONY: build clean docker-build fmt help leanSpec/fixtures lean-devnet lean-e2e lean-ffi lean-node lean-spectest lean-sszgen lean-test-ffi lean-testnet lint test test-all

GIT_COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
GIT_BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)

# Build tags for `make build`, e.g. GO_TAGS=fakecrypto.
GO_TAGS ?=
LDFLAGS := -X github.com/geanlabs/gean/internal/lean/node.gitCommit=$(GIT_COMMIT)

XMSS_RUST := internal/core/crypto/xmss/rust

# leanSpec publishes its generated fixtures as one release file, rebuilt from its main branch.
# The hash pins the exact file gean is tested against, so a new release fails the download
# until this hash is updated on purpose. This file was built from leanSpec commit
# 0b7d33ecbc9ee2435759c92de4da4d08d7faf1c8 (its release notes name it): read the spec there. Defined
# before the lean-spectest/test-all rules: Make expands a rule's prerequisites when it reads the rule.
LEAN_SPEC_FIXTURES_URL := https://github.com/leanEthereum/leanSpec/releases/download/latest/fixtures-prod-scheme.tar.gz
LEAN_SPEC_FIXTURES_SHA256 := 21d9de7056b4e658031dc09e50c0e9dc1b0206089253258ce69ecff01154d4bd

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Build

DOCKER_IMAGE ?= ghcr.io/geanlabs/gean
DOCKER_TAG ?= local

build: lean-ffi ## Build bin/gean
	@mkdir -p bin
	go build -tags "$(GO_TAGS)" -ldflags "$(LDFLAGS)" -o bin/gean ./cmd/gean

docker-build: ## Build the Docker image DOCKER_IMAGE:DOCKER_TAG
	docker build \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg GIT_BRANCH=$(GIT_BRANCH) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) .

##@ Test

test: lean-ffi ## Run unit tests (no FFI tests, spec fixtures or simulations)
	go test $$(go list ./... | grep -v '/crypto/xmss$$') -v -count=1

test-all: lean-ffi leanSpec/fixtures ## Run every test but lean-e2e: unit, FFI, spec fixtures, simulations (slow)
	go test ./... -v -count=1 -tags=spectests,sim

##@ Code

lint: ## Run linters (import rules fail; other golangci-lint findings are reported)
	@command -v golangci-lint >/dev/null || { echo "golangci-lint not found: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"; exit 1; }
	go vet ./...
	golangci-lint run --enable-only depguard ./...
	golangci-lint run --issues-exit-code=0 ./...
	cd $(XMSS_RUST) && cargo fmt --check
	cd $(XMSS_RUST) && cargo clippy -- -D warnings -A clippy::missing_safety_doc

fmt: ## Format Go and Rust code
	gofmt -w .
	cd $(XMSS_RUST) && cargo fmt

clean: ## Remove build artifacts and node data
	rm -rf bin data $(XMSS_LIB)
	cd $(XMSS_RUST) && cargo clean

##@ Lean

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

lean-ffi: ## Build the XMSS Rust library (lean's signatures) the node links against
	cd $(XMSS_RUST) && $(if $(XMSS_RUSTFLAGS),CARGO_ENCODED_RUSTFLAGS="$(XMSS_RUSTFLAGS)") cargo build --profile multisig-release --locked
	@mkdir -p $(XMSS_LIB)
	$(XMSS_STRIP) $(XMSS_RUST)/target/multisig-release/libcgo_glue.a $(XMSS_LIB)/libcgo_glue.a

lean-test-ffi: lean-ffi ## Run the XMSS FFI tests (slow)
	go test ./internal/core/crypto/xmss/ -v -count=1

lean-spectest: lean-ffi leanSpec/fixtures ## Run the leanSpec fixture tests
	cd testing/spectest/lean && go test -count=1 -tags=spectests

lean-e2e: ## Run the end-to-end multi-node test with fake XMSS (no Rust needed)
	go test ./testing/sim/ -v -count=1 -tags=fakecrypto

lean-sszgen: ## Regenerate the lean SSZ encoding files from struct tags
	@rm -f internal/lean/types/*_encoding.go
	sszgen --path internal/lean/types --objs ChainConfig --output internal/lean/types/config_encoding.go
	sszgen --path internal/lean/types --objs Checkpoint --output internal/lean/types/checkpoint_encoding.go
	sszgen --path internal/lean/types --objs Validator --output internal/lean/types/validator_encoding.go
	sszgen --path internal/lean/types --objs AttestationData,Attestation,SignedAttestation,AggregatedAttestation,SingleMessageAggregate,SignedAggregatedAttestation --exclude-objs Checkpoint --output internal/lean/types/attestation_encoding.go
	sszgen --path internal/lean/types --objs BlockHeader,BlockBody,Block,MultiMessageAggregate,SignedBlock --exclude-objs Checkpoint,AttestationData,AggregatedAttestation --output internal/lean/types/block_encoding.go
	sszgen --path internal/lean/types --objs State --exclude-objs ChainConfig,Checkpoint,Validator,BlockHeader --output internal/lean/types/state_encoding.go
	sszgen --path internal/lean/types --objs BlocksByRangeRequest --output internal/lean/types/blocks_by_range_encoding.go
	sszgen --path internal/lean/types --objs BlocksByRootRequest --output internal/lean/types/blocks_by_root_encoding.go

TESTNET_DIR ?= testnet
NUM_VALIDATORS ?= 5
NUM_NODES ?= 3
# Node to run with lean-node. Node N listens on gossipsub port 9000+N (as keygen's nodes.yaml
# lists it), API port 5052+N and metrics port 8080+N; node 0 aggregates.
N ?= 0

lean-testnet: build ## Generate a local network's config and keys in TESTNET_DIR (keys are reused, genesis is reset)
	bin/gean lean keygen --validators $(NUM_VALIDATORS) --nodes $(NUM_NODES) --output $(TESTNET_DIR)

lean-node: build ## Run local node N from a fresh data directory, e.g. make lean-node N=1
	@rm -rf data/node$(N)
	bin/gean lean \
		--custom-network-config-dir $(TESTNET_DIR) \
		--node-key $(TESTNET_DIR)/node$(N).key \
		--node-id node$(N) \
		--data-dir data/node$(N) \
		--gossipsub-port $$((9000 + $(N))) \
		--api-port $$((5052 + $(N))) \
		--metrics-port $$((8080 + $(N))) \
		$(if $(filter 0,$(N)),--is-aggregator)

LEAN_QUICKSTART_REPO ?= https://github.com/blockblaz/lean-quickstart.git
LEAN_QUICKSTART_BRANCH ?= main

lean-quickstart:
	git clone $(LEAN_QUICKSTART_REPO) --branch $(LEAN_QUICKSTART_BRANCH) --depth 1 --single-branch

# lean-quickstart's gean command names a published image and predates the `lean`
# subcommand, so point it at the image just built and run it as `gean lean`. The edit
# keeps an existing ` lean`, so running this twice changes nothing more; a temp file
# rather than `sed -i` works on both GNU and BSD sed.
lean-devnet: docker-build lean-quickstart ## Run a local multi-client lean devnet on lean-quickstart with this build
	@sed 's|ghcr.io/geanlabs/gean:[^ }]*\( lean\)\{0,1\}|$(DOCKER_IMAGE):$(DOCKER_TAG) lean|' lean-quickstart/client-cmds/gean-cmd.sh > lean-quickstart/client-cmds/gean-cmd.sh.tmp \
		&& mv lean-quickstart/client-cmds/gean-cmd.sh.tmp lean-quickstart/client-cmds/gean-cmd.sh
	@echo "Starting a local devnet with $(DOCKER_IMAGE):$(DOCKER_TAG); logs go to devnet.log. Ctrl+C stops every node."
	@cd lean-quickstart \
		&& NETWORK_DIR=local-devnet ./spin-node.sh --node all --generateGenesis --metrics > ../devnet.log 2>&1

# --- leanSpec fixtures, fetched by lean-spectest and test-all ---

leanSpec/fixtures: leanSpec/fixtures/.sha256-$(LEAN_SPEC_FIXTURES_SHA256) ## Download the pinned leanSpec fixtures

leanSpec/fixtures/.sha256-$(LEAN_SPEC_FIXTURES_SHA256):
	@tmp=$$(mktemp -d) && trap 'rm -rf "$$tmp"' EXIT && \
	curl -fL --retry 5 --retry-all-errors --connect-timeout 30 --speed-limit 10000 --speed-time 60 -o "$$tmp/fixtures.tar.gz" $(LEAN_SPEC_FIXTURES_URL) && \
	echo "$(LEAN_SPEC_FIXTURES_SHA256)  $$tmp/fixtures.tar.gz" | shasum -a 256 -c - && \
	rm -rf leanSpec/fixtures && mkdir -p leanSpec/fixtures && \
	tar -xzf "$$tmp/fixtures.tar.gz" -C leanSpec/fixtures --strip-components=1 && \
	touch $@
