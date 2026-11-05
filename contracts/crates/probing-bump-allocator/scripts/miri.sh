#!/usr/bin/env sh
# Runs the allocator's unit tests and the fuzz corpus replay under Miri, which is the only
# tool here that checks the raw pointer arithmetic in src/types.rs for undefined behaviour.
set -eu

cd "$(dirname "$0")/.."

if ! rustup toolchain list | grep -q '^nightly'; then
    echo "the nightly toolchain is required; run: rustup toolchain install nightly" >&2
    exit 1
fi

if ! cargo +nightly miri --version >/dev/null 2>&1; then
    echo "miri is required; run: rustup +nightly component add miri" >&2
    exit 1
fi

# `Address` carries a real pointer, so every address the allocator derives stays traceable
# to the arena it came from and Miri can check aliasing properly. Permissive provenance
# (Miri's default) would accept integer-to-pointer casts and silently downgrade those
# pointers to wildcard provenance, which is precisely the checking we do not want to lose.
BASE_FLAGS="-Zmiri-strict-provenance -Zmiri-symbolic-alignment-check"

for model in stacked-borrows tree-borrows; do
    echo "=== miri: $model ==="
    case "$model" in
        tree-borrows) flags="$BASE_FLAGS -Zmiri-tree-borrows" ;;
        *)            flags="$BASE_FLAGS" ;;
    esac
    MIRIFLAGS="$flags" cargo +nightly miri test -p probing-bump-allocator "$@"
done
