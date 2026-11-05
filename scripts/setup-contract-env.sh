#!/usr/bin/env bash
set -euo pipefail

ANCHOR_VERSION="1.1.2"
SOLANA_VERSION="4.3.0"
# Validator used by the deployment tests only, never to build contracts.
# Publishing program metadata (IDLs, security.txt) makes the metadata program
# CPI into system instruction 13, CreateAccountAllowPrefund, which Agave only
# gained in 4.0. SOLANA_VERSION is pinned for reproducible builds and this
# validator is pinned separately at a version that already has the
# instruction, so the two can be bumped independently.
#
# Pinned rather than tracking the latest release so that a local run and CI
# agree on the validator, the same reason SOLANA_VERSION is pinned. Must stay
# at or above 4.0; bump it in lockstep with the workflow.
TEST_VALIDATOR_VERSION="4.2.2"
ANCHOR_GO_VERSION="v1.0.0"
# Must match the rustc bundled in the anchor Docker image used by CI
# (quay.io/ottersec/anchor:v${ANCHOR_VERSION}). Clippy lints are version-specific,
# so a mismatch here means CI catches lint errors that local runs miss. Bump this
# in lockstep with ANCHOR_VERSION.
RUST_VERSION="1.96.0"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*"; }

check_command() {
    if ! command -v "$1" &>/dev/null; then
        error "$1 is not installed."
        echo "  Install it from: $2"
        return 1
    fi
}

# ─── Prerequisites ──────────────────────────────────────────────────────────────

info "Checking prerequisites..."

check_command rustc    "https://www.rust-lang.org/tools/install" || exit 1
check_command cargo    "https://www.rust-lang.org/tools/install" || exit 1
check_command go       "https://go.dev/doc/install"              || exit 1
check_command python3  "https://www.python.org/downloads/"       || exit 1

# ─── Rust toolchain ─────────────────────────────────────────────────────────────
# Pin the exact Rust version used by CI (solanafoundation/anchor Docker image)
# to ensure cargo fmt output matches.

CURRENT_RUST=$(rustc --version | grep -oE '[0-9]+\.[0-9]+\.[0-9]+')

if [[ "$CURRENT_RUST" != "$RUST_VERSION" ]]; then
    if command -v rustup &>/dev/null; then
        info "Installing Rust ${RUST_VERSION} to match CI (current: ${CURRENT_RUST})..."
        rustup install "$RUST_VERSION"
        rustup default "$RUST_VERSION"
    else
        error "rustup not found — please install Rust ${RUST_VERSION} manually."
        exit 1
    fi
else
    info "Rust ${CURRENT_RUST} matches CI version."
fi

# Ensure rustfmt is available for the pinned toolchain
if ! rustup component list --toolchain "$RUST_VERSION" 2>/dev/null | grep -q 'rustfmt.*installed'; then
    info "Installing rustfmt for Rust ${RUST_VERSION}..."
    rustup component add rustfmt --toolchain "$RUST_VERSION"
fi

# Ensure clippy is available for the pinned toolchain
if ! rustup component list --toolchain "$RUST_VERSION" 2>/dev/null | grep -q 'clippy.*installed'; then
    info "Installing clippy for Rust ${RUST_VERSION}..."
    rustup component add clippy --toolchain "$RUST_VERSION"
fi

# ─── AVM / Anchor ───────────────────────────────────────────────────────────────

if ! command -v avm &>/dev/null; then
    info "Installing AVM (Anchor Version Manager)..."
    cargo install --git https://github.com/coral-xyz/anchor --tag v${ANCHOR_VERSION} avm --force
fi

CURRENT_ANCHOR=$(anchor --version 2>/dev/null | awk '{print $2}' || echo "none")
if [[ "$CURRENT_ANCHOR" != "$ANCHOR_VERSION" ]]; then
    info "Switching Anchor to ${ANCHOR_VERSION} (current: ${CURRENT_ANCHOR})..."
    avm install "$ANCHOR_VERSION" 2>/dev/null || true
    avm use "$ANCHOR_VERSION"
else
    info "Anchor ${ANCHOR_VERSION} already active."
fi

# ─── Solana CLI ──────────────────────────────────────────────────────────────────

CURRENT_SOLANA=$(solana --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' || echo "none")
if [[ "$CURRENT_SOLANA" != "$SOLANA_VERSION" ]]; then
    info "Installing Solana CLI ${SOLANA_VERSION} (current: ${CURRENT_SOLANA})..."
    sh -c "$(curl -sSfL https://release.anza.xyz/v${SOLANA_VERSION}/install)"

    # Ensure solana is on PATH for the rest of this script
    export PATH="$HOME/.local/share/solana/install/active_release/bin:$PATH"
else
    info "Solana CLI ${SOLANA_VERSION} already installed."
fi

# ─── Test validator ──────────────────────────────────────────────────────────────
# Installed into the releases directory without touching active_release, so the
# CLI and contract builds stay on SOLANA_VERSION. The deployment metadata tests
# find it by scanning installed releases for one new enough; without it they
# skip rather than fail.

VALIDATOR_DIR="$HOME/.local/share/solana/install/releases/${TEST_VALIDATOR_VERSION}/solana-release"

if [[ -x "$VALIDATOR_DIR/bin/solana-test-validator" ]]; then
    info "Test validator ${TEST_VALIDATOR_VERSION} already installed."
else
    case "$(uname -s)/$(uname -m)" in
        Darwin/arm64)  VALIDATOR_TARGET="aarch64-apple-darwin"     ;;
        Darwin/x86_64) VALIDATOR_TARGET="x86_64-apple-darwin"      ;;
        Linux/x86_64)  VALIDATOR_TARGET="x86_64-unknown-linux-gnu" ;;
        Linux/aarch64) VALIDATOR_TARGET="aarch64-unknown-linux-gnu";;
        *)             VALIDATOR_TARGET=""                         ;;
    esac

    if [[ -z "$VALIDATOR_TARGET" ]]; then
        warn "No prebuilt test validator for $(uname -s)/$(uname -m); metadata tests will skip."
    else
        info "Installing test validator ${TEST_VALIDATOR_VERSION} (${VALIDATOR_TARGET})..."
        VALIDATOR_TMP="$(mktemp -d)"
        trap 'rm -rf "$VALIDATOR_TMP"' EXIT

        VALIDATOR_URL="https://github.com/anza-xyz/agave/releases/download/v${TEST_VALIDATOR_VERSION}/solana-release-${VALIDATOR_TARGET}.tar.bz2"
        if curl -sSfL "$VALIDATOR_URL" -o "$VALIDATOR_TMP/solana-release.tar.bz2"; then
            mkdir -p "$(dirname "$VALIDATOR_DIR")"
            tar xjf "$VALIDATOR_TMP/solana-release.tar.bz2" -C "$(dirname "$VALIDATOR_DIR")"
            info "Test validator installed; active release left on ${SOLANA_VERSION}."
        else
            warn "Could not download ${VALIDATOR_URL}; metadata tests will skip."
        fi

        rm -rf "$VALIDATOR_TMP"
        trap - EXIT
    fi
fi

# ─── anchor-go ───────────────────────────────────────────────────────────────────

info "Installing anchor-go ${ANCHOR_GO_VERSION}..."
go install "github.com/smartcontractkit/anchor-go@${ANCHOR_GO_VERSION}"

GO_BIN_DIR="$(go env GOBIN)"
if [[ -z "$GO_BIN_DIR" ]]; then
    GO_BIN_DIR="$(go env GOPATH)/bin"
fi

case ":$PATH:" in
    *":$GO_BIN_DIR:"*) ;;
    *)
        export PATH="$GO_BIN_DIR:$PATH"
        warn "Added ${GO_BIN_DIR} to PATH for this session."
        warn "To make this permanent, add the following to your shell profile:"
        echo "  export PATH=\"$GO_BIN_DIR:\$PATH\""
        ;;
esac
info "anchor-go ${ANCHOR_GO_VERSION} installed."

# ─── chainlink-ccv checkout ─────────────────────────────────────────────────────

REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
info "Syncing ../chainlink-ccv to pinned ref from ccv-ref.yaml..."
(cd "$REPO_DIR" && make sync-ccv-checkout) || warn "Could not sync ../chainlink-ccv. You may need to do it manually."

# ─── Verify ─────────────────────────────────────────────────────────────────────

echo ""
info "Environment setup complete. Versions:"
echo "  anchor    : $(anchor --version 2>/dev/null)"
echo "  solana    : $(solana --version 2>/dev/null)"
echo "  rustc     : $(rustc --version 2>/dev/null)"
echo "  rustfmt   : $(rustfmt --version 2>/dev/null)"
echo "  clippy    : $(cargo clippy --version 2>/dev/null)"
echo "  go        : $(go version 2>/dev/null)"
echo "  anchor-go : ${ANCHOR_GO_VERSION}"
echo ""
info "You can now build contracts with:"
echo "  make build-contracts"
