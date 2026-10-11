.PHONY: help build ffi test-ffi test test-spec test-all lint fmt sszgen clean tidy docker-build run-devnet run-setup run run-node1 run-node2 zk-vectors zk-stf-test zk-guest zk-host zk-exec zk-prove

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")

TESTNET_DIR ?= testnet
NUM_VALIDATORS ?= 5
NUM_NODES ?= 3

# Pinned leanSpec revision for spec fixtures. Must be defined before the test-spec/test-all
# rules that reference it: Make expands a rule's prerequisites when it reads the rule, so a
# definition placed after those rules would expand to empty in their prerequisites.
LEAN_SPEC_COMMIT_HASH := eca701efeb5931010fe63925cd203c9ee55b2dbc

help: ## Show help for each Makefile recipe
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

ffi: ## Build XMSS FFI glue libraries (hashsig-glue + multisig-glue)
	@cd xmss/rust && \
		if [ "$$(uname -m)" = "x86_64" ]; then \
			CARGO_ENCODED_RUSTFLAGS="-Ctarget-cpu=haswell" cargo build --profile multisig-release --locked; \
		else \
			cargo build --profile multisig-release --locked; \
		fi

build: ffi ## Build gean and keygen binaries
	@mkdir -p bin
	@go build -ldflags "-X github.com/geanlabs/gean/internal/node.gitCommit=$(GIT_COMMIT)" -o bin/gean ./cmd/gean
	@go build -o bin/keygen ./cmd/keygen

test: ## Run unit tests (excludes crypto FFI and spec tests)
	go test $(shell go list ./... | grep -v '/xmss$$' | grep -v '/spectests$$' | grep -v '/cmd/') -v -count=1

test-ffi: ffi ## Run XMSS crypto FFI tests (builds FFI first)
	go test ./xmss/ -v -count=1

test-spec: ffi leanSpec/fixtures/.generated-$(LEAN_SPEC_COMMIT_HASH) ## Run spec fixture tests only (fast, excludes xmss FFI)
	go test ./internal/spectests/  -count=1 -tags=spectests

test-all: ffi leanSpec/fixtures/.generated-$(LEAN_SPEC_COMMIT_HASH) ## Run all tests including spec fixtures and xmss FFI (slow)
	go test ./... -v -count=1 -tags=spectests

lint: ## Run linters for go & rust
	go vet ./...
	cd xmss/rust && cargo fmt --check
	cd xmss/rust && cargo clippy -- -D warnings -A clippy::missing_safety_doc
	cd zk/stf && cargo fmt --check
	cd zk/stf && cargo clippy --all-targets --locked -- -D warnings

fmt: ## Format all Go code
	gofmt -w .
	cd xmss/rust && cargo fmt
	cd zk/stf && cargo fmt

sszgen: ## Regenerate SSZ encoding files from struct tags
	@rm -f internal/types/*_encoding.go
	sszgen --path internal/types --objs ChainConfig --output internal/types/config_encoding.go
	sszgen --path internal/types --objs Checkpoint --output internal/types/checkpoint_encoding.go
	sszgen --path internal/types --objs Validator --output internal/types/validator_encoding.go
	sszgen --path internal/types --objs AttestationData,Attestation,SignedAttestation,AggregatedAttestation,SingleMessageAggregate,SignedAggregatedAttestation --exclude-objs Checkpoint --output internal/types/attestation_encoding.go
	sszgen --path internal/types --objs BlockHeader,BlockBody,Block,MultiMessageAggregate,SignedBlock --exclude-objs Checkpoint,AttestationData,AggregatedAttestation --output internal/types/block_encoding.go
	sszgen --path internal/types --objs State --exclude-objs ChainConfig,Checkpoint,Validator,BlockHeader --output internal/types/state_encoding.go
	sszgen --path internal/types --objs BlocksByRangeRequest --output internal/types/blocks_by_range_encoding.go

clean: ## Remove build artifacts and generated files
	rm -rf bin data
	cd xmss/rust && cargo clean

tidy: ## Tidy Go module dependencies
	go mod tidy

# --- zkVM proofs of the state transition (opt-in; see zk/README.md) ---

ZKVM ?= sp1
ZK_OUT := $(abspath zk/out)
ZK_ELF = $(ZK_OUT)/$(ZKVM)/stf.elf
ZK_HOST = zk/$(ZKVM)/host/target/release/$(ZKVM)host
INPUT ?= $(ZK_OUT)/vectors/0000.gstf

# How each zkVM's toolchain builds its guest into $(ZK_ELF).
ZK_GUEST_sp1 = cd zk/sp1/guest && cargo prove build --locked --output-directory $(ZK_OUT)/sp1 --elf-name stf.elf
ZK_GUEST_risc0 = cd zk/risc0/methods && cargo run --release --locked -- $(ZK_ELF)
ZK_GUEST_zisk = cd zk/zisk/guest && cargo-zisk build --release && cp target/elf/riscv64ima-zisk-zkvm-elf/release/stf-zisk $(ZK_ELF)
ZK_GUEST_openvm = cd zk/openvm/guest && cargo openvm build && cp target/riscv64im-unknown-openvm-elf/release/stf-openvm $(ZK_ELF)

zk-vectors: ## Write the state-transition vectors, with the Go outcome of each, to zk/out/vectors
	go run ./cmd/stfprove vectors -o $(ZK_OUT)/vectors

zk-stf-test: zk-vectors ## Check the Rust port (zk/stf) against the Go transition on every vector
	cd zk/stf && cargo test --release --locked

zk-guest: ## Build the guest for ZKVM (sp1, risc0, zisk or openvm) into zk/out/<zkvm>/stf.elf
	@test -n "$(ZK_GUEST_$(ZKVM))" || (echo "unknown ZKVM=$(ZKVM)"; exit 1)
	@mkdir -p $(ZK_OUT)/$(ZKVM)
	$(ZK_GUEST_$(ZKVM))

# MAKEFLAGS is cleared so cargo build scripts that run make do not inherit
# this make's command-line variables.
zk-host: ## Build zk/<ZKVM>/host, which executes, proves and verifies on ZKVM
	cd zk/$(ZKVM)/host && env -u MAKEFLAGS cargo build --release --locked

zk-exec: zk-vectors ## Run every vector on ZKVM and compare with the Go transition
	go run ./cmd/stfprove exec -host $(ZK_HOST) -elf $(ZK_ELF) -vectors $(ZK_OUT)/vectors

zk-prove: ## Prove and verify INPUT (default the first vector) on ZKVM
	go run ./cmd/stfprove prove -host $(ZK_HOST) -elf $(ZK_ELF) -i $(INPUT) -o $(ZK_OUT)/$(ZKVM)/proof.bin

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

# --- leanSpec fixtures --- (LEAN_SPEC_COMMIT_HASH is defined near the top, before test-spec)

leanSpec/.git: ## Clone leanSpec
	git clone https://github.com/leanEthereum/leanSpec.git --single-branch

leanSpec/.pinned-$(LEAN_SPEC_COMMIT_HASH): leanSpec/.git
	cd leanSpec && git fetch origin $(LEAN_SPEC_COMMIT_HASH) && git checkout --detach $(LEAN_SPEC_COMMIT_HASH)
	touch $@

leanSpec/fixtures/.generated-$(LEAN_SPEC_COMMIT_HASH): leanSpec/.pinned-$(LEAN_SPEC_COMMIT_HASH)
	@cd leanSpec && for attempt in 1 2 3; do \
		uv run keys --download --scheme=prod && break; \
		test $$attempt -eq 3 && exit 1; \
		sleep $$((attempt * 5)); \
	done
	cd leanSpec && uv run fill --clean --fork=lstar --scheme=prod --output=fixtures
	touch $@

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
