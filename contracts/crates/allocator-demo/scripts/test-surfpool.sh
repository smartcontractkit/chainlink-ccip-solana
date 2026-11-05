#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")/.."

if ! command -v cargo-build-sbf >/dev/null 2>&1; then
    echo "cargo-build-sbf is required; install the Solana/Agave CLI toolchain first" >&2
    exit 1
fi

cargo build-sbf --manifest-path Cargo.toml --sbf-out-dir target/deploy
cargo test \
    --locked \
    -p allocator-demo \
    --features surfpool-tests \
    --test surfpool \
    -- \
    --nocapture
