#!/usr/bin/env sh
# Fuzzes the allocator with cargo-fuzz. Local only: a useful run is measured in hours, so
# this is deliberately not wired into CI.
#
#   ./fuzz.sh                 fuzz for $FUZZ_TIME seconds
#   ./fuzz.sh repro <file>    replay a single artifact or corpus input
set -eu

cd "$(dirname "$0")/../fuzz"

FUZZ_TARGET="${FUZZ_TARGET:-ops}"
FUZZ_TIME="${FUZZ_TIME:-60}"
FUZZ_JOBS="${FUZZ_JOBS:-1}"

if ! rustup toolchain list | grep -q '^nightly'; then
    echo "the nightly toolchain is required; run: rustup toolchain install nightly" >&2
    exit 1
fi

if ! cargo +nightly fuzz --version >/dev/null 2>&1; then
    echo "cargo-fuzz is required; run: cargo install cargo-fuzz" >&2
    exit 1
fi

if [ "${1:-}" = "repro" ]; then
    if [ -z "${2:-}" ]; then
        echo "usage: $0 repro <artifact-path>" >&2
        exit 1
    fi
    exec cargo +nightly fuzz run "$FUZZ_TARGET" "$2"
fi

# AddressSanitizer is on by default, and the harness backs each arena with an exactly
# sized heap allocation so a write past the arena end lands in a redzone.
cargo +nightly fuzz run "$FUZZ_TARGET" --jobs "$FUZZ_JOBS" -- \
    -max_total_time="$FUZZ_TIME" \
    -rss_limit_mb=4096
