.PHONY: help build ffi test-ffi test test-spec test-all lint fmt sszgen clean tidy docker-build run-devnet run-setup run run-node1 run-node2 devnet-test devnet-test-sync devnet-status devnet-cleanup devnet-analyze devnet-run devnet-clean-logs

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")

TESTNET_DIR ?= testnet
NUM_VALIDATORS ?= 5
NUM_NODES ?= 3

help: ## Show help for each Makefile recipe
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

ffi: ## Build XMSS FFI glue libraries (hashsig-glue + multisig-glue)
	@cd xmss/rust && cargo build --release --locked

build: ffi ## Build gean and keygen binaries
	@mkdir -p bin
	@go build -o bin/gean ./cmd/gean
	@go build -o bin/keygen ./cmd/keygen

test: ## Run unit tests (excludes crypto FFI and spec tests)
	go test $(shell go list ./... | grep -v '/xmss$$' | grep -v '/spectests$$' | grep -v '/cmd/') -v -count=1

test-ffi: ffi ## Run XMSS crypto FFI tests (builds FFI first)
	go test ./xmss/ -v -count=1

test-spec: leanSpec/fixtures ## Run spec fixture tests only (fast, excludes xmss FFI)
	go test ./spectests/  -count=1 -tags=spectests

test-all: leanSpec/fixtures ## Run all tests including spec fixtures and xmss FFI (slow)
	go test ./... -v -count=1 -tags=spectests

lint: ## Run linters for go & rust
	go vet ./...
	cd xmss/rust && cargo fmt --check
	cd xmss/rust && cargo clippy -- -D warnings -A clippy::missing_safety_doc

fmt: ## Format all Go code
	gofmt -w .
	cd xmss/rust && cargo fmt

sszgen: ## Regenerate SSZ encoding files from struct tags
	@rm -f types/*_encoding.go
	sszgen --path pkg/types --objs ChainConfig --output types/config_encoding.go
	sszgen --path pkg/types --objs Checkpoint --output types/checkpoint_encoding.go
	sszgen --path pkg/types --objs Validator --output types/validator_encoding.go
	sszgen --path pkg/types --objs AttestationData,Attestation,SignedAttestation,AggregatedAttestation,SignedAggregatedAttestation --exclude-objs Checkpoint --output types/attestation_encoding.go
	sszgen --path pkg/types --objs BlockHeader,BlockBody,Block,BlockWithAttestation,AggregatedSignatureProof,BlockSignatures,SignedBlockWithAttestation --exclude-objs Checkpoint,AttestationData,Attestation,AggregatedAttestation,AggregatedSignatureProof --output types/block_encoding.go
	sszgen --path pkg/types --objs State --exclude-objs ChainConfig,Checkpoint,Validator,BlockHeader --output types/state_encoding.go

clean: ## Remove build artifacts and generated files
	rm -rf bin data
	rm -f types/*_encoding.go
	cd xmss/rust && cargo clean

tidy: ## Tidy Go module dependencies
	go mod tidy

# --- Local testnet ---

run-setup: build ## Generate testnet config + XMSS keys (first run only, refreshes genesis time)
	@bin/keygen --validators $(NUM_VALIDATORS) --nodes $(NUM_NODES) --output $(TESTNET_DIR)

run: build ## Run node0 (aggregator) — requires make run-setup first
	@rm -rf data/node0
	@bin/keygen --validators $(NUM_VALIDATORS) --nodes $(NUM_NODES) --output $(TESTNET_DIR)
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

LEAN_SPEC_COMMIT_HASH := be853180d21aa36d6401b8c1541aa6fcaad5008d

leanSpec: ## Clone leanSpec at devnet-3 commit
	git clone https://github.com/leanEthereum/leanSpec.git --single-branch
	cd leanSpec && git checkout $(LEAN_SPEC_COMMIT_HASH)

leanSpec/fixtures: leanSpec ## Generate consensus test fixtures from leanSpec
	cd leanSpec && uv run fill --fork devnet --scheme=prod -o fixtures

# --- Docker ---

DOCKER_TAG ?= local

docker-build: ## Build Docker image
	docker build \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg GIT_BRANCH=$(GIT_BRANCH) \
		-t gean:$(VERSION) \
		-t ghcr.io/geanlabs/gean:devnet3 .

# --- Multi-client devnet ---

lean-quickstart: ## Clone lean-quickstart for local devnet
	git clone https://github.com/blockblaz/lean-quickstart.git --depth 1 --single-branch

run-devnet: docker-build lean-quickstart ## Run local multi-client devnet
	@echo "Starting local devnet with gean client (\"$(DOCKER_TAG)\" tag)."
	@cd lean-quickstart \
		&& NETWORK_DIR=local-devnet ./spin-node.sh --node all --generateGenesis --metrics > ../devnet.log 2>&1

# --- Claude Code skills (multi-client testing helpers) ---
# See .claude/skills/README.md for details.
#
# Targets that take an optional duration support BOTH styles:
#   make devnet-run                  # default 120s
#   make devnet-run 600              # 600s (positional, picked from MAKECMDGOALS)
#   make devnet-run DURATION=600     # 600s (named variable)
#
# The DEVNET_ARGS filter pulls any extra non-target words off the command line
# so you can pass "make devnet-test 300" without errors. The numeric catch-all
# rule at the bottom of this section absorbs the trailing word.
DEVNET_ARGS := $(filter-out $(MAKECMDGOALS),$(MAKECMDGOALS))
DEVNET_EXTRA := $(filter-out devnet-run devnet-test devnet-test-sync devnet-status devnet-cleanup devnet-clean-logs devnet-analyze,$(MAKECMDGOALS))

DURATION ?= 120
TEST_DURATION ?= 60

devnet-run: ## Run multi-client devnet for DURATION seconds (default 120), dump logs, then stop
	@.claude/skills/devnet-runner/scripts/run-devnet-with-timeout.sh $(or $(DEVNET_EXTRA),$(DURATION))

devnet-test: ## Build current branch + test in 5-client devnet (default 60s, override: make devnet-test 300)
	@RUN_DURATION=$(or $(DEVNET_EXTRA),$(TEST_DURATION)) .claude/skills/test-pr-devnet/scripts/test-branch.sh

devnet-test-sync: ## Same as devnet-test but also tests sync recovery (pause/resume)
	@RUN_DURATION=$(or $(DEVNET_EXTRA),$(TEST_DURATION)) .claude/skills/test-pr-devnet/scripts/test-branch.sh --with-sync-test

devnet-status: ## Show status of running devnet (heads, errors, gean metrics)
	@.claude/skills/test-pr-devnet/scripts/check-status.sh

devnet-cleanup: devnet-clean-logs ## Stop devnet, restore gean-cmd.sh, and remove dumped logs
	@.claude/skills/test-pr-devnet/scripts/cleanup.sh

devnet-clean-logs: ## Remove dumped client logs from the repo root
	@rm -f gean_0.log zeam_0.log ream_0.log lantern_0.log ethlambda_0.log qlean_0.log devnet.log
	@echo "✓ Dumped logs removed"

devnet-analyze: ## Analyze .log files in current directory (errors, blocks, consensus progress)
	@.claude/skills/devnet-log-review/scripts/analyze-logs.sh .

# Catch-all for numeric positional args after devnet-* targets.
# Matches any all-digit "target name" so `make devnet-test 300` doesn't error
# trying to build a `300` target.
%:
	@:
