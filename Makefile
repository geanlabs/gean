.PHONY: build test clean run run-validator help generate lint

BIN_DIR := bin
BINARY := $(BIN_DIR)/gean

# Default values
GENESIS_TIME ?= $(shell date +%s)
VALIDATOR_COUNT ?= 4
VALIDATOR_INDEX ?= 0
LISTEN_ADDR ?= /ip4/0.0.0.0/tcp/9000
BOOTNODES ?=
LOG_LEVEL ?= info

help: ## Show help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

generate: ## Generate SSZ encoding code
	go run github.com/ferranbt/fastssz/sszgen --path=./consensus --objs=Checkpoint,Config,Vote,SignedVote,BlockHeader,BlockBody,Block,SignedBlock,State

build: ## Build the gean binary
	@mkdir -p $(BIN_DIR)
	go build -o $(BINARY) ./cmd/gean

test: ## Run tests
	go test ./... -v

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)
	go clean

run: build ## Run as non-validator node
	./$(BINARY) \
		--genesis-time $(GENESIS_TIME) \
		--validator-count $(VALIDATOR_COUNT) \
		--listen "$(LISTEN_ADDR)" \
		--log-level $(LOG_LEVEL)

run-validator: build ## Run as validator node
	./$(BINARY) \
		--genesis-time $(GENESIS_TIME) \
		--validator-count $(VALIDATOR_COUNT) \
		--validator-index $(VALIDATOR_INDEX) \
		--listen "$(LISTEN_ADDR)" \
		--bootnodes "$(BOOTNODES)" \
		--log-level $(LOG_LEVEL)

lint: ## Run go vet
	go vet ./...
