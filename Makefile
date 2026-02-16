.PHONY: build test test-race lint fmt clean docker-build run run-devnet help

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

build:
	@mkdir -p bin
	@echo "Building gean..."
	@go build -ldflags "-X main.version=$(VERSION)" -o bin/gean ./cmd/gean
	@echo "Done: bin/gean"
	@echo "Building keygen..."
	@go build -o bin/keygen ./cmd/keygen
	@echo "Done: bin/keygen"
	@echo "Building leansig..."
	@cd leansig && cargo build --release
	@echo "Done: leansig/target/release/leansig"

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

run: build
	./bin/gean --genesis config.yaml --bootnodes nodes.yaml --validator-registry-path validators.yaml --validator-keys keys --node-id node0

run-devnet:
	@if [ ! -d "../lean-quickstart" ]; then \
		echo "Cloning lean-quickstart..."; \
		git clone https://github.com/blockblaz/lean-quickstart.git ../lean-quickstart; \
	fi
	$(MAKE) docker-build
	cd ../lean-quickstart && NETWORK_DIR=local-devnet ./spin-node.sh --node gean_0 --generateGenesis --metrics
