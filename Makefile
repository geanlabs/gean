.PHONY: build ffi spec-test unit-test test-race lint fmt clean docker-build run run-quic run-devnet refresh-genesis-time help leanSpec leanSpec/fixtures

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

ffi:
	@cd xmss/leansig-ffi && cargo build --release

build: ffi
	@mkdir -p bin
	@go build -ldflags "-X github.com/geanlabs/gean/node.Version=$(VERSION)" -o bin/gean ./cmd/gean
	@go build -o bin/keygen ./cmd/keygen

# Run the spectests with the leanSpec fixtures, skipping signature verification for faster test execution
spec-test: ffi leanSpec/fixtures
	go test -tags skip_sig_verify -count=1 ./spectests/...

# Run the unit tests, which include signature verification and thus take longer to execute
unit-test: ffi
	go test ./... -count=1

test-race: ffi
	go test -race ./...

lint:
	go vet ./...
	@which staticcheck > /dev/null 2>&1 && staticcheck ./... || echo "staticcheck not installed, skipping"

fmt:
	go fmt ./...

clean:
	rm -rf bin
	go clean

docker-build:
	docker build -t gean:$(VERSION) .

# Resolve the directory this Makefile lives in
MAKEFILE_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
CONFIG := $(MAKEFILE_DIR)config.yaml

refresh-genesis-time:
	@NEW_TIME=$$(($$(date +%s) + 30)); \
	if [ "$$(uname -s)" = "Darwin" ]; then \
		sed -i '' "s/^GENESIS_TIME:.*/GENESIS_TIME: $$NEW_TIME/" $(CONFIG); \
	else \
		sed -i "s/^GENESIS_TIME:.*/GENESIS_TIME: $$NEW_TIME/" $(CONFIG); \
	fi; \
	echo "Updated GENESIS_TIME to $$NEW_TIME in $(CONFIG)"

run: build refresh-genesis-time
	@./bin/gean --genesis config.yaml --bootnodes nodes.yaml --validator-registry-path validators.yaml --validator-keys keys --node-id node0 --listen-addr /ip4/0.0.0.0/tcp/9000 --node-key node0.key --data-dir data/node0

run-devnet:
	@if [ ! -d "../lean-quickstart" ]; then \
		echo "Cloning lean-quickstart..."; \
		git clone https://github.com/blockblaz/lean-quickstart.git ../lean-quickstart; \
	fi
	$(MAKE) docker-build
	cd ../lean-quickstart && NETWORK_DIR=local-devnet ./spin-node.sh --node gean_0 --generateGenesis --metrics

run-node-1:
	@./bin/gean --genesis config.yaml --bootnodes nodes.yaml --validator-registry-path validators.yaml --validator-keys keys --node-id node1 --listen-addr /ip4/0.0.0.0/tcp/9001 --node-key node1.key --data-dir data/node1 --discovery-port 9001

run-node-2:
	@./bin/gean --genesis config.yaml --bootnodes nodes.yaml --validator-registry-path validators.yaml --validator-keys keys --node-id node2 --listen-addr /ip4/0.0.0.0/tcp/9002 --node-key node2.key --data-dir data/node2 --discovery-port 9002

# The commit hash of the leanSpec repository to use for testing and fixtures
LEAN_SPEC_COMMIT_HASH := 050fa4a18881d54d7dc07601fe59e34eb20b9630

# A file to track which commit of the leanSpec fixtures have been generated, to avoid unnecessary regeneration
LEAN_SPEC_FIXTURE_STAMP := leanSpec/.fixtures-commit

# Clone the leanSpec repository if it doesn't exist, and checkout the specified commit
leanSpec:
	@if [ ! -d "leanSpec/.git" ]; then \
		git clone https://github.com/leanEthereum/leanSpec.git --single-branch leanSpec; \
	fi
	@cd leanSpec && CURRENT_COMMIT=$$(git rev-parse HEAD) && \
	if [ "$$CURRENT_COMMIT" != "$(LEAN_SPEC_COMMIT_HASH)" ]; then \
		git fetch --all --tags --prune && git checkout $(LEAN_SPEC_COMMIT_HASH); \
	fi

# Generate the leanSpec fixtures if they are not already generated for the specified commit
leanSpec/fixtures: leanSpec
	@CURRENT_FIXTURE_COMMIT=$$(cat $(LEAN_SPEC_FIXTURE_STAMP) 2>/dev/null || true); \
	if [ "$$CURRENT_FIXTURE_COMMIT" != "$(LEAN_SPEC_COMMIT_HASH)" ] || [ ! -d "leanSpec/fixtures/consensus" ]; then \
		cd leanSpec && uv run fill --fork=Devnet --layer=consensus --clean -o fixtures && \
		echo "$(LEAN_SPEC_COMMIT_HASH)" > .fixtures-commit; \
	fi
