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

refresh-genesis-time:
	@NEW_TIME=$$(( $$(date +%s) + 30 )); \
	sed -i "s/^GENESIS_TIME:.*/GENESIS_TIME: $$NEW_TIME/" config.yaml; \
	echo "Updated GENESIS_TIME to $$NEW_TIME"

run: build refresh-genesis-time
	@./bin/gean --genesis config.yaml --bootnodes nodes.yaml --validator-registry-path validators.yaml --validator-keys keys --node-id node0

run-devnet:
	@if [ ! -d "../lean-quickstart" ]; then \
		echo "Cloning lean-quickstart..."; \
		git clone https://github.com/blockblaz/lean-quickstart.git ../lean-quickstart; \
	fi
	$(MAKE) docker-build
	cd ../lean-quickstart && NETWORK_DIR=local-devnet ./spin-node.sh --node gean_0 --generateGenesis --metrics
