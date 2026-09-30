.PHONY: help build ffi test-ffi test test-spec test-all lint fmt sszgen clean tidy docker-build run-devnet run-setup run run-node1 run-node2 zk-toolchain zk-guest zk-vectors zk-exec zk-haltcheck

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

build: ffi ## Build gean, keygen and stfprove binaries
	@mkdir -p bin
	@go build -ldflags "-X github.com/geanlabs/gean/internal/node.gitCommit=$(GIT_COMMIT)" -o bin/gean ./cmd/gean
	@go build -o bin/keygen ./cmd/keygen
	@go build -o bin/stfprove ./cmd/stfprove

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

fmt: ## Format all Go code
	gofmt -w .
	cd xmss/rust && cargo fmt

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

# --- zkVM state-transition guest (opt-in; see zk/README.md) ---

ZKVM ?= zisk
ZK_OUT := zk/out
ZK_TOOLS := zk/.tools
TAMAGO_VERSION := tamago-go1.26.6
TAMAGO_SHA256 := d9a59d85886ef9a755ce7d8ae5a8a4cf60a6b50292cbfafaa058a2ccaf90f00f
TAMAGO_GO := $(ZK_TOOLS)/$(TAMAGO_VERSION)/usr/local/tamago-go/bin/go
ZISKEMU ?= ziskemu

# Per-zkVM link addresses: code in the zkVM's ROM, data at the start of its RAM.
ZK_LDFLAGS_zisk := -T 0x80001000 -D 0xa0430000 -R 0x1000

# Reproducible, soft-float, uncompressed RV64 bare-metal build.
ZK_GO_ENV = GOTOOLCHAIN=local GOOS=tamago GOARCH=riscv64 GORISCV64=rva20u64 CGO_ENABLED=0 GOOSPKG=github.com/geanlabs/gean/zk
ZK_GO_FLAGS = -trimpath -buildvcs=false -tags zkvm_$(ZKVM) \
	-gcflags=all=-d=softfloat,compressinstructions=0 -asmflags=all=-d=compressinstructions=0 \
	-ldflags "-buildid= $(ZK_LDFLAGS_$(ZKVM))"

$(TAMAGO_GO):
	@mkdir -p $(ZK_TOOLS)
	curl -sSfL -o $(ZK_TOOLS)/$(TAMAGO_VERSION).tar.gz \
		https://github.com/usbarmory/tamago-go/releases/download/$(TAMAGO_VERSION)/$(TAMAGO_VERSION).linux-amd64.tar.gz
	echo "$(TAMAGO_SHA256)  $(ZK_TOOLS)/$(TAMAGO_VERSION).tar.gz" | sha256sum -c -
	mkdir -p $(ZK_TOOLS)/$(TAMAGO_VERSION)
	tar -xzf $(ZK_TOOLS)/$(TAMAGO_VERSION).tar.gz -C $(ZK_TOOLS)/$(TAMAGO_VERSION)

zk-toolchain: $(TAMAGO_GO) ## Download the pinned TamaGo toolchain for zkVM guests

zk-guest: $(TAMAGO_GO) ## Build the state-transition guest for ZKVM (default zisk)
	@test -n "$(ZK_LDFLAGS_$(ZKVM))" || (echo "no guest board for ZKVM=$(ZKVM)"; exit 1)
	@mkdir -p $(ZK_OUT)
	cd zk && $(ZK_GO_ENV) ../$(TAMAGO_GO) build $(ZK_GO_FLAGS) -o out/stf-$(ZKVM).raw.elf ./guest/stf
	cd zk && go run ./cmd/elffix -zkvm $(ZKVM) out/stf-$(ZKVM).raw.elf out/stf-$(ZKVM).elf
	@sha256sum $(ZK_OUT)/stf-$(ZKVM).elf

zk-haltcheck: $(TAMAGO_GO) ## Check that only a committed run halts successfully (ZKVM=zisk)
	@mkdir -p $(ZK_OUT)
	cd zk && $(ZK_GO_ENV) ../$(TAMAGO_GO) build $(ZK_GO_FLAGS) -o out/haltcheck-$(ZKVM).raw.elf ./guest/haltcheck
	cd zk && go run ./cmd/elffix -zkvm $(ZKVM) out/haltcheck-$(ZKVM).raw.elf out/haltcheck-$(ZKVM).elf
	cd zk && ZISKEMU=$(ZISKEMU) go run ./cmd/haltcheck -elf out/haltcheck-$(ZKVM).elf

zk-vectors: ## Write the generated state-transition vectors to zk/out/vectors
	go run ./cmd/stfprove vectors -o $(ZK_OUT)/vectors

zk-exec: zk-vectors ## Execute every vector on the ZisK emulator and compare with native
	go run ./cmd/stfprove execute --zkvm $(ZKVM) --backend emu --ziskemu $(ZISKEMU) --manifest $(ZK_OUT)/vectors/manifest.json

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
