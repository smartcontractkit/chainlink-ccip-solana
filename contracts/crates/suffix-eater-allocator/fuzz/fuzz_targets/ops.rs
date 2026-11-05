#![no_main]

use libfuzzer_sys::fuzz_target;
use suffix_eater_allocator::harness;

// Drives an arbitrary allocate/free/realloc sequence against a fresh arena, asserting the
// allocator's invariants after every call. See `harness` for what is checked, and for the
// one production configuration this deliberately cannot reach.
fuzz_target!(|data: &[u8]| {
    harness::run_ops(harness::DEFAULT_ARENA_WORDS, &harness::decode_ops(data));
});
