.PHONY: build clean docker-build ffi fmt help lint loc spec-pq spectest test test-ffi vet

include pins.env

GIT_COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
GIT_BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)

XMSS_RUST := internal/core/crypto/xmss/rust

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Build

DOCKER_IMAGE ?= ghcr.io/geanlabs/gean
DOCKER_TAG ?= local

build: ## Build bin/gean
	@mkdir -p bin
	go build -o bin/gean ./cmd/gean

docker-build: ## Build the Docker image DOCKER_IMAGE:DOCKER_TAG
	docker build \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg GIT_BRANCH=$(GIT_BRANCH) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) .

##@ Test

test: ffi ## Run unit tests and the XMSS scheme vector (no other FFI tests)
	go test $$(go list ./... | grep -v '/crypto/xmss$$') -v -count=1
	go test ./internal/core/crypto/xmss/ -run '^TestSchemeKnownAnswer$$' -v -count=1

spectest: ## Run the reference tests in spectests/ (make spec-pq) or GEAN_SPEC_TESTS_DIR
	go test -tags spectests ./testing/spectest/ -count=1 -v

# consensus-specs is checked out here at the pinned commit. Point it at an existing clone to reuse
# its Python environment; the clone is left detached at the pin.
CONSENSUS_SPECS_DIR ?= .consensus-specs
SPEC_TESTS_STAMP := spectests/.commit-$(CONSENSUS_SPECS_COMMIT)
# A fix to consensus-specs' test helpers at the pin, applied before generating: their VRF tree
# cache mixes the ValidatorIndex types of different spec builds, which then refuse to compare.
SPEC_TESTS_PATCH := testing/spectest/consensus-specs.patch
SPEC_TESTS_REPORT := $(CONSENSUS_SPECS_DIR)/tests/core/pyspec/test-reports/test_results.xml

spec-pq: $(SPEC_TESTS_STAMP) ## Generate the PQ reference tests into spectests/ (needs uv)

# The stamp names the commit, so a new pin regenerates, and so does a newer patch. The first run
# downloads about 120 MB of Python dependencies, more than uv's 30 s default timeout allows on a
# slow link. The patch is taken back out before checking out, so a new pin's checkout isn't
# blocked by it.
$(SPEC_TESTS_STAMP): export UV_HTTP_TIMEOUT = 600
$(SPEC_TESTS_STAMP): $(SPEC_TESTS_PATCH)
	@# git looks for a repository in parent directories too, so only a clone rooted here counts.
	@top=$$(git -C $(CONSENSUS_SPECS_DIR) rev-parse --show-toplevel 2>/dev/null); \
		[ -n "$$top" ] && [ "$$top" = "$$(cd $(CONSENSUS_SPECS_DIR) && pwd -P)" ] || \
		git init -q $(CONSENSUS_SPECS_DIR)
	@git -C $(CONSENSUS_SPECS_DIR) cat-file -e $(CONSENSUS_SPECS_COMMIT)^{commit} 2>/dev/null || \
		git -C $(CONSENSUS_SPECS_DIR) fetch -q --depth=1 https://github.com/ethereum/consensus-specs $(CONSENSUS_SPECS_COMMIT)
	@! git -C $(CONSENSUS_SPECS_DIR) apply -R --check $(CURDIR)/$(SPEC_TESTS_PATCH) 2>/dev/null || \
		git -C $(CONSENSUS_SPECS_DIR) apply -R $(CURDIR)/$(SPEC_TESTS_PATCH)
	git -C $(CONSENSUS_SPECS_DIR) checkout -q --detach $(CONSENSUS_SPECS_COMMIT)
	@git -C $(CONSENSUS_SPECS_DIR) apply -R --check $(CURDIR)/$(SPEC_TESTS_PATCH) 2>/dev/null || \
		git -C $(CONSENSUS_SPECS_DIR) apply $(CURDIR)/$(SPEC_TESTS_PATCH)
	rm -rf $(CONSENSUS_SPECS_DIR)/reftests $(SPEC_TESTS_REPORT)
	$(MAKE) -C $(CONSENSUS_SPECS_DIR) sync
	@# A failing upstream test writes no case and leaves the others valid, so test failures don't
	@# stop the target. pytest writes its report only when the run completes, so a run that was
	@# killed or crashed has none and does stop it.
	@#
	@# The fast confirmation rule's tests are left out: they take minutes each, and gean has no
	@# confirmation rule. The rule also gives get_safe_execution_block_hash, the safe block sent to
	@# the execution client; the Engine API client either brings these cases back to implement it,
	@# or uses the justified block and lists that in docs/spec_deviations.md.
	-$(MAKE) -C $(CONSENSUS_SPECS_DIR) test reftests=true fork=pq k="not fast_confirmation"
	@[ -f $(SPEC_TESTS_REPORT) ] || { echo "consensus-specs did not finish generating" >&2; exit 1; }
	rm -rf spectests
	mv $(CONSENSUS_SPECS_DIR)/reftests spectests
	touch $@

##@ Code

lint: ## Run linters; any finding fails
	@command -v golangci-lint >/dev/null || { echo "golangci-lint not found: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"; exit 1; }
	go vet ./...
	golangci-lint run ./...
	cd $(XMSS_RUST) && cargo fmt --check
	cd $(XMSS_RUST) && cargo clippy -- -D warnings -A clippy::missing_safety_doc

vet: ## Vet the build-tagged variants make lint leaves out
	go vet -tags fakecrypto ./...
	go vet -tags spectests ./testing/...

fmt: ## Format Go and Rust code
	gofmt -w .
	cd $(XMSS_RUST) && cargo fmt

clean: ## Remove build artifacts
	rm -rf bin $(XMSS_LIB)
	cd $(XMSS_RUST) && cargo clean

loc: ## Count lines of code per package; BASE=<ref> compares with that ref
	@scripts/loc.sh $(BASE)

##@ XMSS

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

ffi: ## Build the XMSS Rust library
	cd $(XMSS_RUST) && $(if $(XMSS_RUSTFLAGS),CARGO_ENCODED_RUSTFLAGS="$(XMSS_RUSTFLAGS)") cargo build --profile multisig-release --locked
	@mkdir -p $(XMSS_LIB)
	$(XMSS_STRIP) $(XMSS_RUST)/target/multisig-release/libcgo_glue.a $(XMSS_LIB)/libcgo_glue.a

test-ffi: ffi ## Run the XMSS FFI tests (slow)
	go test ./internal/core/crypto/xmss/ -v -count=1
