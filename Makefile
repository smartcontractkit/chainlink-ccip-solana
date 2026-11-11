BIN_DIR = bin
export GOPATH ?= $(shell go env GOPATH)
export GO111MODULE ?= on
export ANCHOR_VERSION ?=v1.1.2
export ANCHOR_IMAGE ?= quay.io/ottersec/anchor:$(ANCHOR_VERSION)
export ANCHOR_BUILD_ARGS ?=
# Allow overriding the output directory for docker builds
TARGET_DIR ?= $(PWD)/contracts/target

MOCKERY_VERSION := 2.53.5
MOCKERY_XTOOLS_VERSION := v0.51.0

.PHONY: projectserum_version
anchor_version:
	@echo "${ANCHOR_VERSION}"

.PHONY: clippy
clippy:
	cd ./contracts && cargo check && cargo clippy -- -D warnings

.PHONY: gomodtidy
gomodtidy:
	@# Cargo's target/ dir lives inside the root Go module and holds transient rustc
	@# temp dirs that make `go mod tidy` fail. A nested go.mod makes Go skip it.
	@mkdir -p ./contracts/target && printf 'module ignore\n' > ./contracts/target/go.mod
	@for dir in ./contracts/gobindings/v2 ./deployment . ./integration-tests; do \
		echo "=== go mod tidy $$dir ==="; \
		(cd $$dir && go mod tidy) || exit 1; \
	done

.PHONY: format-contracts
format-contracts:
	cd ./contracts && cargo fmt && cd gobindings/v2 && go fmt ./...

# Run on a contracts release PR branch: release-please bumps the program version
# in contracts/Cargo.toml, this updates Cargo.lock, IDLs and TS types to match.
.PHONY: contracts-release-sync
contracts-release-sync:
	./contracts/scripts/release-sync.sh

.PHONY: rust-tests
rust-tests:
	cd ./contracts && cargo test

# Local only, deliberately not part of solana-checks or CI: needs cargo-build-sbf and
# compiles the full Agave/Surfpool graph. See contracts/crates/allocator-demo/README.md.
.PHONY: test-allocator-surfpool
test-allocator-surfpool:
	./contracts/crates/allocator-demo/scripts/test-surfpool.sh

# Local only, deliberately not part of solana-checks or CI: nightly Miri runs the tests
# roughly 100x slower than a native build. See contracts/crates/suffix-eater-allocator/README.md.
.PHONY: miri-allocator
miri-allocator: ## Check the allocator for undefined behaviour under Miri (Stacked + Tree Borrows)
	./contracts/crates/suffix-eater-allocator/scripts/miri.sh

# Local only, deliberately not part of solana-checks or CI: needs cargo-fuzz and nightly,
# and it's veeeeery slow.
.PHONY: fuzz-allocator
fuzz-allocator: ## Fuzz the allocator (FUZZ_TIME=60 FUZZ_JOBS=1 FUZZ_TARGET=ops)
	./contracts/crates/suffix-eater-allocator/scripts/fuzz.sh

.PHONY: fuzz-allocator-repro
fuzz-allocator-repro: ## Replay one fuzz artifact: make fuzz-allocator-repro ARTIFACT=<path>
	./contracts/crates/suffix-eater-allocator/scripts/fuzz.sh repro "$(ARTIFACT)"

# The same two tools for the second allocator, probing-bump-allocator. Also local only, for
# the same reasons. See contracts/crates/probing-bump-allocator/README.md.
.PHONY: miri-probing-bump
miri-probing-bump: ## Check probing-bump-allocator under Miri (Stacked + Tree Borrows)
	./contracts/crates/probing-bump-allocator/scripts/miri.sh

.PHONY: fuzz-probing-bump
fuzz-probing-bump: ## Fuzz probing-bump-allocator (FUZZ_TIME=60 FUZZ_JOBS=1 FUZZ_TARGET=ops)
	./contracts/crates/probing-bump-allocator/scripts/fuzz.sh

.PHONY: fuzz-probing-bump-repro
fuzz-probing-bump-repro: ## Replay one fuzz artifact: make fuzz-probing-bump-repro ARTIFACT=<path>
	./contracts/crates/probing-bump-allocator/scripts/fuzz.sh repro "$(ARTIFACT)"

.PHONY: install-mockery
install-mockery:
	@# mockery v$(MOCKERY_VERSION) ships an old golang.org/x/tools that cannot read the standard library
	@# of newer Go releases ("package \"context\" without types was imported"), so build it
	@# in a scratch module with x/tools pinned to $(MOCKERY_XTOOLS_VERSION).
	@if ! command -v mockery >/dev/null 2>&1 \
		|| ! mockery version 2>&1 | grep -qF "$(MOCKERY_VERSION)" \
		|| ! go version -m "$$(command -v mockery)" | grep -qE "golang.org/x/tools[[:space:]]+$(MOCKERY_XTOOLS_VERSION)([[:space:]]|$$)"; then \
		echo "Installing mockery v$(MOCKERY_VERSION) (x/tools $(MOCKERY_XTOOLS_VERSION))..."; \
		gover=$$(go env GOVERSION | sed 's/^go//'); tmp=$$(mktemp -d); \
		(cd "$$tmp" && go mod init mockerytool >/dev/null 2>&1 && go mod edit -go=$$gover \
			&& go get github.com/vektra/mockery/v2@v$(MOCKERY_VERSION) golang.org/x/tools@$(MOCKERY_XTOOLS_VERSION) \
			&& go install github.com/vektra/mockery/v2); rc=$$?; \
		rm -rf "$$tmp"; exit $$rc; \
	fi

.PHONY: go-mocks
go-mocks: install-mockery
	@mkdir -p internal/mocks
	rm -f internal/mocks/mock_*.go
	mockery

.PHONY: go-mocks-check
go-mocks-check: go-mocks
	git diff --exit-code -- internal/mocks/

.PHONY: anchor-go-gen
anchor-go-gen: build-contracts
	./scripts/anchor-go-gen.sh

.PHONY: format
format:
	cd ./contracts/gobindings/v2 && go fmt ./... 2>/dev/null; cd $(CURDIR)/contracts && cargo fmt

.PHONY: go-tests-contracts
go-tests-contracts:
	cd contracts && go test -p 1 -parallel 1 -v ./... -json -covermode=atomic -coverpkg=./... -coverprofile=integration_coverage.txt 2>&1 | tee /tmp/gotest.log | gotestloghelper -ci=true -singlepackage=true -hidepassingtests=false -hidepassinglogs=false

.PHONY: go-unit-tests
go-unit-tests:
	go test ./pkg/... -count=1
	# -p 1: the deployment integration tests each stand up a solana-test-validator,
	# and two of those cloning from devnet at once is enough to time out genesis.
	cd deployment && go test ./... -count=1 -p 1

.PHONY: ccip-router-contracts-go-tests
ccip-router-contracts-go-tests:
	go test -run TestCCIPRouter -v -failfast ./... -json -covermode=atomic -coverpkg=./... -coverprofile=integration_coverage.txt 2>&1 | tee /tmp/gotest.log | gotestloghelper -ci=true -singlepackage=true -hidepassingtests=false -hidepassinglogs=false

.PHONY: test-token-pool-contracts-go-tests
test-token-pool-contracts-go-tests:
	go test -run TestBaseTokenPoolHappyPath -run TestTokenPool -v -failfast ./... -json -covermode=atomic -coverpkg=./... -coverprofile=integration_coverage.txt 2>&1 | tee /tmp/gotest.log | gotestloghelper -ci=true -singlepackage=true -hidepassingtests=false -hidepassinglogs=false

.PHONY: build-contracts
build-contracts:
	cd ./contracts && anchor build --ignore-keys
	$(MAKE) copy-cctp-v2-programs

# Note on CARGO_REGISTRIES_CRATES_IO_PROTOCOL=sparse
# Sparse index significantly speeds up dependency fetching, supposedly default in 1.70 but anchor build was still using a slow git download
# --ignore-keys: program IDs are fixed in Anchor.toml for devenv/CTF; stale target/deploy keypairs must not fail the build.
.PHONY: docker-build-contracts
docker-build-contracts:
	docker run --rm --platform linux/amd64 \
		-v $(shell pwd)/contracts:/workdir \
		-v ${TARGET_DIR}:/workdir/target \
		-e CARGO_TARGET_DIR=/workdir/target \
		-e CARGO_REGISTRIES_CRATES_IO_PROTOCOL=sparse \
		${ANCHOR_IMAGE} anchor build --ignore-keys -- ${ANCHOR_BUILD_ARGS}
	$(MAKE) copy-cctp-v2-programs

# The devenv deploys Circle's vendored CCTP V2 programs next to the built ones.
.PHONY: copy-cctp-v2-programs
copy-cctp-v2-programs:
	cp contracts/target/vendor/cctp_message_transmitter_v2.so contracts/target/vendor/cctp_token_messenger_minter_v2.so contracts/target/deploy/

.PHONY: docker-update-contracts
docker-update-contracts:
	docker run --rm \
		-v $(shell pwd)/contracts:/workdir \
		-v ${TARGET_DIR}:/workdir/target \
		-e CARGO_TARGET_DIR=/workdir/target \
		-e CARGO_REGISTRIES_CRATES_IO_PROTOCOL=sparse \
		${ANCHOR_IMAGE} anchor keys sync

.PHONY: anchor-lint
anchor-lint: ## Run Anchor program static analysis checks (INV-LINT-*)
	python3 scripts/anchor-lint/src/anchor-lint.py

.PHONY: anchor-lint-test
anchor-lint-test: ## Run anchor-lint self-tests against the test fixture
	bash scripts/anchor-lint/tests/test_anchor_lint.sh

.PHONY: anchor-lint-strict
anchor-lint-strict: ## Same as anchor-lint but exits non-zero on any finding
	python3 scripts/anchor-lint/src/anchor-lint.py --strict

# Base ref for diff-scoped linting. CI overrides this with the PR's base branch;
# locally it defaults to main so `make anchor-lint-diff` checks your branch's work.
ANCHOR_LINT_BASE ?= origin/main

.PHONY: anchor-lint-diff
anchor-lint-diff: ## Run anchor-lint over only lines changed vs $(ANCHOR_LINT_BASE); fails on any finding there
	python3 scripts/anchor-lint/src/anchor-lint.py --strict --diff $(ANCHOR_LINT_BASE)

.PHONY: anchor-lint-baseline
anchor-lint-baseline: ## Rewrite scripts/anchor-lint/suppressions.json from the current findings
	python3 scripts/anchor-lint/src/anchor-lint.py --write-baseline

.PHONY: anchor-lint-all
anchor-lint-all: ## Report every finding, ignoring the suppression baseline
	python3 scripts/anchor-lint/src/anchor-lint.py --no-baseline

.PHONY: solana-checks
solana-checks: clippy anchor-lint-diff anchor-go-gen format gomodtidy lint-go rust-tests go-tests-contracts

.PHONY: solana-build-tidy
solana-build-tidy: clippy anchor-lint-strict anchor-go-gen format gomodtidy lint-go

# CCV dependency management — see scripts/ci/ccv-ref.yaml for config.
# Submodules listed in ccv-ref.yaml (e.g. build/devenv) are pinned to the
# same SHA as the root module. `pin-ccv` does both in one shot; the separate
# targets exist for backward compatibility and for partial workflows.
.PHONY: pin-ccv
pin-ccv: ## Pin chainlink-ccv root + every yaml submodule. Usage: make pin-ccv REF=<commit>
	./scripts/ci/ccv-ref.sh pin $(REF)

.PHONY: pin-ccv-submodules
pin-ccv-submodules: ## Pin yaml submodules to the root pseudo-version already in go.mod
	./scripts/ci/ccv-ref.sh pin-submodules

.PHONY: pin-ccv-all
pin-ccv-all: pin-ccv ## Pin chainlink-ccv root + submodules. Usage: make pin-ccv-all REF=<commit>

.PHONY: validate-ccv
validate-ccv: ## Check yaml + every go.mod (root and submodules) match the pinned ref
	./scripts/ci/ccv-ref.sh validate

# Ensure ../chainlink-ccv is checked out at the pinned SHA from ccv-ref.yaml.
# Automatically fetches + checks out the correct commit so devenv images match.
.PHONY: sync-ccv-checkout
sync-ccv-checkout:
	@pinned=$$(yq -r '.pinned' scripts/ci/ccv-ref.yaml); \
	if [ ! -d ../chainlink-ccv ]; then \
		echo "ERROR: ../chainlink-ccv not found. Clone it first:"; \
		echo "  git clone git@github.com:smartcontractkit/chainlink-ccv.git ../chainlink-ccv"; \
		exit 1; \
	fi; \
	current=$$(cd ../chainlink-ccv && git rev-parse HEAD); \
	if [ "$$current" = "$$pinned" ]; then \
		echo "../chainlink-ccv already at pinned ref $${pinned:0:12}"; \
	else \
		echo "Syncing ../chainlink-ccv to pinned ref $${pinned:0:12} (was $${current:0:12})..."; \
		(cd ../chainlink-ccv && git fetch origin && git checkout $$pinned) || \
		{ echo "ERROR: could not checkout $$pinned in ../chainlink-ccv"; exit 1; }; \
	fi

# Devenv targets - assumes chainlink-ccv is checked out at ../chainlink-ccv
# CTF bind-mounts contracts/target/deploy (see devenv/env.toml).
# Docker requires the path to exist.
.PHONY: ensure-contract-deploy-dir
ensure-contract-deploy-dir:
	@mkdir -p contracts/target/deploy
	@if ! ls contracts/target/deploy/*.so >/dev/null 2>&1; then \
		echo "Missing Solana program binaries under contracts/target/deploy."; \
		echo "Build them once (see devenv/README.md), then retry:"; \
		echo "  make docker-build-contracts"; \
		exit 1; \
	fi

# Build CCV Docker images (verifier, aggregator, indexer, etc) with :latest tags
.PHONY: build-ccv-images
build-ccv-images:
	cd ../chainlink-ccv/build/devenv && just build-docker

# Build the Solana-specific verifier Docker image
.PHONY: build-solana-verifier-image
build-solana-verifier-image:
	docker build -f cmd/committeeverifier/Dockerfile -t solana-verifier:latest .

# Build the Solana-specific verifier Docker image
.PHONY: build-solana-token-verifier-image
build-solana-token-verifier-image:
	docker build -f cmd/tokenverifier/Dockerfile -t solana-token-verifier:latest .

# Build the Solana-specific executor Docker image (CCV bootstrap + service factory binary).
.PHONY: build-solana-executor-image
build-solana-executor-image:
	docker build -f cmd/executor/Dockerfile -t solana-executor:latest .

# Start unified devenv (1 EVM + 2 Solana chains) in standalone mode.
# Supports S2S, EVM→Solana, and Solana→EVM directions.
.PHONY: start-devenv
start-devenv: ensure-contract-deploy-dir sync-ccv-checkout build-ccv-images build-solana-verifier-image build-solana-token-verifier-image build-solana-executor-image restart-devenv

# Tear-down & spin back up the devenv, without rebuilding images or doing other checks
.PHONY: restart-devenv
restart-devenv:
	go run devenv/cmd/ccv/main.go down && \
	go run devenv/cmd/ccv/main.go up devenv/env-phased.toml --env-mode phased

# Tear down
.PHONY: stop-devenv
stop-devenv:
	go run devenv/cmd/ccv/main.go down

# sync CLD addresses to devenv .deployed.toml (for use in e2e tests)
# Default path & environment options (can be overridden from CLI or env vars)
CLD_PATH ?= ../chainlink-deployments
ENV ?= staging_testnet_solana
DEVENV_PATH ?= devenv
TARGET_FILE ?=

.PHONY: sync-addresses
sync-addresses: ## Sync address_refs.json from chainlink-deployments into devenv .deployed.toml
	@if [ ! -d "$(CLD_PATH)" ]; then \
		echo "Error: CLD_PATH '$(CLD_PATH)' directory does not exist."; \
		echo "Usage: make sync-addresses CLD_PATH=/path/to/cld ENV=staging_testnet_solana [TARGET_FILE=custom.toml]"; \
		exit 1; \
	fi
	go run devenv/scripts/sync_addresses.go \
		-cld-path="$(CLD_PATH)" \
		-env="$(ENV)" \
		-devenv-path="$(DEVENV_PATH)" \
		-target-file="$(TARGET_FILE)"

# Run the devenv smoke cases. Env-agnostic: attaches to whatever SMOKE_TEST_CONFIG
# points at (unset = local env-phased-out.toml; set it to a persistent attach file
# such as ../../env-private-staging.deployed.toml). CASES is a
# caller-supplied `go test -run` regex (empty = all); a `$` anchor must be doubled to
# `$$` on the make command line. Signer keys (PRIVATE_KEY, SOLANA_PRIVATE_KEY) and
# SMOKE_TEST_CONFIG are read from the ambient env, inject them from the caller (CI
# secrets, or `set -a && source devenv/.env && set +a`). Example:
#   SMOKE_TEST_CONFIG=../../env-private-staging.deployed.toml \
#   make test-smoke CASES='TestEVM2Solana/ArbMessaging$$'
.PHONY: test-smoke
test-smoke:
	SMOKE_TEST_CONFIG="$${SMOKE_TEST_CONFIG:-../../env-phased-out.toml}" \
		go test -v -count=1 -timeout 15m -run '$(CASES)' ./devenv/tests/smoke


# Run the devenv chaos suite. Local devenv ONLY: the tests docker stop/start
# RPC/aggregator/executor containers, so they cannot attach to a deployed env.
# SMOKE_TEST_CONFIG selects the local devenv attach file (unset = env-phased-out.toml).
# CASES is a caller-supplied `go test -run` regex (empty = all chaos tests);
# a `$` anchor must be doubled to `$$` on the make command line. Example:
#   make test-chaos CASES='TestChaos_RPCLatency$$'
.PHONY: test-chaos
test-chaos:
	SMOKE_TEST_CONFIG="$${SMOKE_TEST_CONFIG:-../../env-phased-out.toml}" \
		go test -v -count=1 -timeout 15m -run '$(CASES)' ./devenv/tests/chaos


# Run the devenv load tests. Env-agnostic: attaches to whatever LOAD_TEST_OUT_FILE
# points at (unset = local env-phased-out.toml; set it to a persistent attach file
# such as ../../env-private-staging.deployed.toml). CASES is a
# caller-supplied `go test -run` regex (empty = all); a `$` anchor must be doubled to
# `$$` on the make command line. Signer keys (PRIVATE_KEY, SOLANA_PRIVATE_KEY) are
# read from the ambient env, inject them from the caller (CI secrets, or
# `set -a && source devenv/.env && set +a`). Example:
#   LOAD_TEST_OUT_FILE=../../env-private-staging.deployed.toml \
#   make test-load CASES='TestEVM2SolanaLoad'
.PHONY: test-load
test-load:
	LOAD_TEST_OUT_FILE="$${LOAD_TEST_OUT_FILE:-../../env-phased-out.toml}" \
		go test -v -count=1 -timeout 25m -run '$(CASES)' ./devenv/tests/load


# Go - linting and formatting
GOLANGCI_LINT_VERSION := v2.14.0

.PHONY: print-golangci-version
print-golangci-version:
	@echo $(GOLANGCI_LINT_VERSION)

# Build .golangci.yml from the upstream chainlink config + this repo's deltas.
# Overrides (gci sections, linter exclusions) live in .golangci.local.yml;
# the merge logic lives in scripts/ci/fetch-golangci-config.sh.
.PHONY: fetch-golangci-config
fetch-golangci-config:
	./scripts/ci/fetch-golangci-config.sh

.PHONY: install-golangci-lint
install-golangci-lint:
	@# Built from source with the local Go toolchain: the prebuilt release binaries
	@# are compiled with an older Go and refuse to lint modules whose go.mod targets a newer one.
	@if ! command -v golangci-lint >/dev/null 2>&1 \
		|| [ "$$(golangci-lint version 2>&1 | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1)" != "$(GOLANGCI_LINT_VERSION)" ] \
		|| [ "$$(golangci-lint version 2>&1 | grep -oE 'built with go[0-9.]+' | head -1 | sed 's/built with //')" != "$$(go env GOVERSION)" ]; then \
		echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION) with $$(go env GOVERSION)..."; \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	fi

.PHONY: lint-go
# lint-go mirrors CI intent for PR workflows: scan whole files and report
# only issues introduced since merge-base with origin's default branch.
lint-go: install-golangci-lint fetch-golangci-config
	@for dir in $$(find . -path ./contracts/target -prune -o -name go.mod -exec dirname {} \;); do \
		echo "=== Linting $$dir ==="; \
		base_ref=$$(git merge-base HEAD $$(git symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null || echo origin/main)); \
		(cd $$dir && golangci-lint run --max-issues-per-linter 0 --max-same-issues 0 --color=always --timeout 15m --whole-files --new-from-rev $$base_ref) || exit 1; \
	done

# scan all Go modules in the repo, ignoring merge-base and reporting all issues
.PHONY: lint-go-all
lint-go-all: install-golangci-lint fetch-golangci-config
	@for dir in $$(find . -path ./contracts/target -prune -o -name go.mod -exec dirname {} \;); do \
		echo "=== Linting $$dir ==="; \
		(cd $$dir && golangci-lint run --max-issues-per-linter 0 --max-same-issues 0 --color=always --timeout 15m --whole-files) || exit 1; \
	done

# scan all Go modules in the repo, ignoring merge-base and fixing all fixable issues
.PHONY: lint-go-fix
lint-go-fix: install-golangci-lint fetch-golangci-config
	@for dir in $$(find . -path ./contracts/target -prune -o -name go.mod -exec dirname {} \;); do \
		echo "=== Linting $$dir ==="; \
		(cd $$dir && golangci-lint run --max-issues-per-linter 0 --max-same-issues 0 --color=always --timeout 15m --whole-files --verbose --fix) || exit 1; \
	done