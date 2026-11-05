#!/usr/bin/env bash
# Release build for the Solana programs: one `solana-verify build
# --library-name <lib>` per program, the documented standard for multi-program
# repos.
#
# solana-verify prints the executable hash of the built program as its last output
# line; those hashes are collected into target/executable-hashes.txt and shipped in
# the release tarball so deployments can be checked against them.
set -euo pipefail

# The programs require the v3 ISA. The build image comes from [workspace.metadata.cli]
# solana in Cargo.toml. Keep the build args in sync with buildArgs in
# .github/workflows/contract-release.yml, which the on-chain build record reproduces.
# NOTE: the current test-validator cannot load v3 artifacts yet, so release artifacts
# are validated structurally, not by deploying.
ARCH="${ARCH:-v3}"

CONTRACTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$CONTRACTS_DIR"

HASHES_FILE="$CONTRACTS_DIR/target/executable-hashes.txt"
: > "$HASHES_FILE"

programs="$(bash "$CONTRACTS_DIR/scripts/list_programs.sh")"
count="$(printf '%s\n' "$programs" | wc -l | tr -d ' ')"
echo "Building $count programs with solana-verify (arch $ARCH)..."

for lib in $programs; do
  echo "=== $lib"
  log="$(mktemp)"
  if ! solana-verify build --library-name "$lib" --cargo-build-sbf-args="--arch $ARCH" 2>&1 | tee "$log"; then
    rm -f "$log"
    echo "error: solana-verify build failed for $lib" >&2
    exit 1
  fi
  hash="$(grep -oE '^[0-9a-f]{64}$' "$log" | tail -1 || true)"
  rm -f "$log"
  if [ -z "$hash" ]; then
    echo "error: solana-verify did not print an executable hash for $lib" >&2
    exit 1
  fi
  artifact="$CONTRACTS_DIR/target/deploy/$lib.so"
  idl="$CONTRACTS_DIR/target/idl/$lib.json"
  if [ ! -f "$artifact" ]; then
    echo "error: solana-verify did not produce expected artifact $artifact" >&2
    exit 1
  fi
  if [ ! -f "$idl" ]; then
    echo "error: expected IDL is missing: $idl" >&2
    exit 1
  fi
  echo "$hash  $lib.so" >> "$HASHES_FILE"
done

echo "Wrote $HASHES_FILE"
