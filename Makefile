.PHONY: help build ffi test-ffi test test-spec test-all spec-fixtures lint check-architecture check-go fmt sszgen clean tidy docker-build run-devnet run-setup run run-node1 run-node2

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")

TESTNET_DIR ?= testnet
NUM_VALIDATORS ?= 5
NUM_NODES ?= 3

# leanSpec's latest released fixtures, checked against the release's SHA-256. Must be defined
# before the test-spec/test-all rules that reference it: Make expands a rule's prerequisites when
# it reads the rule. Run `make spec-fixtures` again after deleting leanSpec/fixtures to refresh.
LEAN_SPEC_FIXTURES_URL ?= https://github.com/leanEthereum/leanSpec/releases/latest/download/fixtures-prod-scheme.tar.gz
LEAN_SPEC_FIXTURES := leanSpec/fixtures/.release
STATICCHECK_VERSION := v0.8.1

help: ## Show help for each Makefile recipe
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

# leanVM's prover does binary-field arithmetic with carryless multiplication. On x86 the
# Haswell baseline provides PCLMULQDQ; on aarch64 PMULL is gated behind the `aes` target
# feature, which Linux aarch64 targets do not enable by default. Without it the prover
# falls back to scalar code.
#
# Go 1.26.11 and later refuse cgo archives that carry LLVM bitcode sections. The only members
# that do are compiler_builtins objects, which ship precompiled with Rust's standard library;
# the linker drops these sections from the binary anyway, so stripping them changes no code.
# Done on Linux only; macOS archives would need their __LLVM segment removed instead.
ffi: ## Build XMSS FFI glue libraries (hashsig-glue + multisig-glue)
	@cd lean/crypto/xmss/rust && \
		case "$$(uname -m)" in \
			x86_64) CARGO_ENCODED_RUSTFLAGS="-Ctarget-cpu=haswell" cargo build --profile multisig-release --locked ;; \
			aarch64|arm64) CARGO_ENCODED_RUSTFLAGS="-Ctarget-feature=+aes" cargo build --profile multisig-release --locked ;; \
			*) cargo build --profile multisig-release --locked ;; \
		esac
	@if [ "$$(uname -s)" = Linux ]; then \
		objcopy --remove-section=.llvmbc --remove-section=.llvmcmd lean/crypto/xmss/rust/target/multisig-release/libcgo_glue.a; \
	fi

build: ffi ## Build gean and keygen binaries
	@mkdir -p bin
	@go build -ldflags "-X github.com/geanlabs/gean/lean/internal/chain.gitCommit=$(GIT_COMMIT)" -o bin/gean ./cmd/gean
	@go build -o bin/keygen ./cmd/keygen

test: ## Run unit tests (excludes crypto FFI and spec tests)
	go test $(shell go list ./... | grep -v '/xmss$$' | grep -v '/spectests$$' | grep -v '/cmd/') -v -count=1

test-ffi: ffi ## Run XMSS crypto FFI tests (builds FFI first)
	go test ./lean/crypto/xmss/ -v -count=1

test-spec: ffi $(LEAN_SPEC_FIXTURES) ## Run spec fixture tests; lists cases skipped for a key-scheme mismatch
	go test ./lean/spectests/ -v -count=1 -tags=spectests

test-all: ffi $(LEAN_SPEC_FIXTURES) ## Run all tests including spec fixtures and xmss FFI (slow)
	go test ./... -v -count=1 -tags=spectests

check-architecture: ## Check package boundaries and the boundary checker's regression tests (no FFI)
	go test ./tools/archcheck -count=1
	go run ./tools/archcheck

check-go: ## Run vet and pinned bug/unused-code checks (run make ffi first)
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) -checks='SA*,U1000' ./...
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) -checks='SA*,U1000' -tags=spectests,hive_testdriver,aggregation_diagnostic ./...

lint: check-architecture check-go ## Run linters for go & rust
	cd lean/crypto/xmss/rust && cargo fmt --check
	cd lean/crypto/xmss/rust && cargo clippy -- -D warnings -A clippy::missing_safety_doc

fmt: ## Format all Go code
	gofmt -w .
	cd lean/crypto/xmss/rust && cargo fmt

sszgen: ## Regenerate SSZ encoding files from struct tags
	@rm -f lean/consensus/types/*_encoding.go
	sszgen --path lean/consensus/types --objs ChainConfig --output lean/consensus/types/config_encoding.go
	sszgen --path lean/consensus/types --objs Checkpoint --output lean/consensus/types/checkpoint_encoding.go
	sszgen --path lean/consensus/types --objs Validator --output lean/consensus/types/validator_encoding.go
	sszgen --path lean/consensus/types --objs AttestationData,Attestation,SignedAttestation,AggregatedAttestation,SingleMessageAggregate,SignedAggregatedAttestation --exclude-objs Checkpoint --output lean/consensus/types/attestation_encoding.go
	sszgen --path lean/consensus/types --objs BlockHeader,BlockBody,Block,MultiMessageAggregate,SignedBlock --exclude-objs Checkpoint,AttestationData,AggregatedAttestation --output lean/consensus/types/block_encoding.go
	sszgen --path lean/consensus/types --objs State --exclude-objs ChainConfig,Checkpoint,Validator,BlockHeader --output lean/consensus/types/state_encoding.go
	sszgen --path lean/consensus/types --objs BlocksByRangeRequest --output lean/consensus/types/blocks_by_range_encoding.go

clean: ## Remove build artifacts and generated files
	rm -rf bin data
	cd lean/crypto/xmss/rust && cargo clean

tidy: ## Tidy Go module dependencies
	go mod tidy

# --- Local testnet ---

run-setup: build ## Generate testnet config + XMSS keys (first run only, refreshes genesis time)
	@bin/keygen --validators $(NUM_VALIDATORS) --nodes $(NUM_NODES) --output $(TESTNET_DIR)

run: build ## Run node0 (aggregator) — requires make run-setup first
	@rm -rf data/node0
	@bin/gean \
		--custom-network-config-dir $(TESTNET_DIR) \
		--node-key $(TESTNET_DIR)/node0.key \
		--node-id node0 \
		--data-dir data/node0 \
		--is-aggregator \
		--gossipsub-port 9000 \
		--api-port 5052 \
		--metrics-port 8080

run-node1: build ## Run node1 on port 9001
	@rm -rf data/node1
	@bin/gean \
		--custom-network-config-dir $(TESTNET_DIR) \
		--node-key $(TESTNET_DIR)/node1.key \
		--node-id node1 \
		--data-dir data/node1 \
		--gossipsub-port 9001 \
		--api-port 5053 \
		--metrics-port 8081

run-node2: build ## Run node2 on port 9002
	@rm -rf data/node2
	@bin/gean \
		--custom-network-config-dir $(TESTNET_DIR) \
		--node-key $(TESTNET_DIR)/node2.key \
		--node-id node2 \
		--data-dir data/node2 \
		--gossipsub-port 9002 \
		--api-port 5054 \
		--metrics-port 8082

# --- leanSpec fixtures ---

spec-fixtures: $(LEAN_SPEC_FIXTURES) ## Download and verify leanSpec's latest released fixtures

# Downloads into leanSpec/download and replaces leanSpec/fixtures only after the checksum matches.
$(LEAN_SPEC_FIXTURES):
	rm -rf leanSpec/download
	mkdir -p leanSpec/download
	curl -fsSL -o leanSpec/download/fixtures.tar.gz "$(LEAN_SPEC_FIXTURES_URL)"
	curl -fsSL -o leanSpec/download/fixtures.tar.gz.sha256 "$(LEAN_SPEC_FIXTURES_URL).sha256"
	cd leanSpec/download && echo "$$(cut -d' ' -f1 fixtures.tar.gz.sha256)  fixtures.tar.gz" | sha256sum --check --quiet
	rm -rf leanSpec/fixtures
	mkdir -p leanSpec/fixtures
	tar -xzf leanSpec/download/fixtures.tar.gz -C leanSpec/fixtures --strip-components=1
	sha256sum leanSpec/download/fixtures.tar.gz > $@
	rm -rf leanSpec/download
	@echo "leanSpec fixtures: $$(cut -d' ' -f1 $@)"

# --- Docker ---

DOCKER_TAG ?= local

docker-build: ## Build Docker image (also tagged :shadow for the lean-shadow-fuzzer)
	docker build \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg GIT_BRANCH=$(GIT_BRANCH) \
		-t gean:$(VERSION) \
		-t ghcr.io/geanlabs/gean:devnet5 .

# --- Multi-client devnet ---

lean-quickstart: ## Clone lean-quickstart for local devnet
	git clone https://github.com/blockblaz/lean-quickstart.git --depth 1 --single-branch

run-devnet: docker-build lean-quickstart ## Run local multi-client devnet
	@echo "Starting local devnet with gean client (\"$(DOCKER_TAG)\" tag)."
	@cd lean-quickstart \
		&& NETWORK_DIR=local-devnet ./spin-node.sh --node all --generateGenesis --metrics > ../devnet.log 2>&1
