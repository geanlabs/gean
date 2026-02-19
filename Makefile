.PHONY: build test test-race lint fmt clean docker-build run run-devnet refresh-genesis-time help

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

build:
	@cd xmss/leansig-ffi && cargo build --release > /dev/null 2>&1
	@mkdir -p bin
	@go build -ldflags "-X main.version=$(VERSION)" -o bin/gean ./cmd/gean
	@go build -o bin/keygen ./cmd/keygen

test:
	go test ./...

test-race:
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
	sed -i'' "s/^GENESIS_TIME:.*/GENESIS_TIME: $$NEW_TIME/" $(CONFIG); \
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