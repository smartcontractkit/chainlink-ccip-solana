//! Op-sequence driver for [`Allocator`], shared by the `cargo-fuzz` target in `fuzz/` and
//! by the corpus replay test that runs under `cargo test` and Miri.
//!
//! The allocator is exercised as an ordinary object over a host arena, never installed as
//! a `#[global_allocator]`: its metadata is non-atomic and the host is multi-threaded.
//!
//! # What is deliberately not covered
//!
//! [`Allocator::with_fixed_address_range`] is always given a `len` equal to the arena's
//! real length. On Solana the program instead configures the 256KB maximum while the
//! runtime may map only 32KB, and relies on the footer write past the mapped end aborting
//! the transaction. That write is genuine out-of-bounds UB on a host, so it cannot be
//! fuzzed here; `make test-allocator-surfpool` covers it on-chain instead.

use core::alloc::{GlobalAlloc, Layout};
use core::mem::size_of;
use std::alloc::{alloc_zeroed, dealloc};

use crate::Allocator;

/// Arena size used by the fuzz target, in `usize` words.
///
/// Deliberately small (4KB) so that exhaustion, and therefore the allocation-failure
/// paths, are reached often rather than only by a lucky input.
pub const DEFAULT_ARENA_WORDS: usize = 512;

/// Upper bound on simultaneously live blocks, so a run cannot spend all its ops growing
/// the bookkeeping vector.
const MAX_LIVE_BLOCKS: usize = 64;

/// Largest alignment requested, as a power-of-two exponent. Covers every alignment an
/// on-chain Rust type can ask for, plus over-aligned outliers.
const MAX_ALIGN_LOG2: u8 = 6;

/// Cap on decoded ops. Keeps a Miri replay (roughly 100x slower than native) bounded.
const MAX_OPS: usize = 1024;

const WORD: usize = size_of::<usize>();

/// One allocator call. Slot indices are taken modulo the live-block count, so every
/// decoded op is meaningful for almost any input rather than mostly being skipped.
#[derive(Clone, Copy, Debug)]
pub enum Op {
    Alloc { size: u16, align_log2: u8 },
    Dealloc { slot: u8 },
    Realloc { slot: u8, new_size: u16 },
}

/// Decodes a fuzz input into ops, four bytes each: `[selector, a, b, c]`.
///
/// The mapping is fixed and total, so a corpus file promoted from a crash artifact keeps
/// replaying the same sequence for as long as it stays committed.
pub fn decode_ops(bytes: &[u8]) -> Vec<Op> {
    bytes
        .chunks(4)
        .take(MAX_OPS)
        .map(|chunk| {
            let byte = |index: usize| chunk.get(index).copied().unwrap_or(0);
            let (a, b, c) = (byte(1), byte(2), byte(3));
            match byte(0) % 3 {
                0 => Op::Alloc {
                    size: u16::from_le_bytes([a, b]),
                    align_log2: c,
                },
                1 => Op::Dealloc { slot: a },
                _ => Op::Realloc {
                    slot: a,
                    new_size: u16::from_le_bytes([b, c]),
                },
            }
        })
        .collect()
}

/// A zeroed, exactly-sized heap region backing one allocator.
///
/// Raw rather than a `Box<[usize]>` for two reasons: an exact-size allocation puts an
/// AddressSanitizer redzone immediately past the arena end during fuzzing, and there is
/// no `Box` retag to invalidate the derived raw pointer under Miri's aliasing models.
struct Arena {
    ptr: *mut u8,
    layout: Layout,
}

impl Arena {
    fn new(words: usize) -> Self {
        assert!(
            words > 1,
            "an arena needs room for the cursor word and a footer"
        );
        let layout = Layout::from_size_align(words * WORD, WORD).expect("valid arena layout");
        // SAFETY: `layout` has a non-zero size.
        let ptr = unsafe { alloc_zeroed(layout) };
        assert!(!ptr.is_null(), "arena allocation failed");
        Self { ptr, layout }
    }

    fn start(&self) -> usize {
        self.ptr as usize
    }

    fn end(&self) -> usize {
        self.start() + self.layout.size()
    }
}

impl Drop for Arena {
    fn drop(&mut self) {
        // SAFETY: `ptr` came from `alloc_zeroed` with this exact layout and is still live.
        unsafe { dealloc(self.ptr, self.layout) };
    }
}

/// A block the harness believes is live, and the byte pattern it should still hold.
struct Live {
    ptr: *mut u8,
    layout: Layout,
    tag: u8,
}

/// Runs `ops` against a fresh arena of `arena_words` words, asserting the allocator's
/// invariants after every call. Panics on the first violation, which is what both
/// libFuzzer and Miri treat as a finding.
pub fn run_ops(arena_words: usize, ops: &[Op]) {
    let arena = Arena::new(arena_words);
    // SAFETY: The arena is writable for its whole length, zeroed (so the cursor word
    // starts at zero), `usize`-aligned, larger than one word, outlives the allocator, and
    // cannot overflow because it came from a successful allocation.
    let allocator = unsafe { Allocator::with_owned_region(arena.ptr, arena.layout.size()) };
    let mut live: Vec<Live> = Vec::new();
    let mut tag_counter: u8 = 0;

    for op in ops {
        match *op {
            Op::Alloc { size, align_log2 } => {
                if live.len() >= MAX_LIVE_BLOCKS {
                    continue;
                }
                let layout = request_layout(size, align_log2, &arena);
                // SAFETY: The allocator's backing arena is live for the whole run.
                let before = unsafe { allocator.cursor_offset() };
                // SAFETY: `layout` has a non-zero size and a power-of-two alignment.
                let ptr = unsafe { allocator.alloc(layout) };
                if ptr.is_null() {
                    // SAFETY: As above.
                    let after = unsafe { allocator.cursor_offset() };
                    assert_eq!(after, before, "a failed alloc moved the cursor");
                    continue;
                }
                assert_placement(&arena, ptr, layout, &live);
                let tag = fresh_tag(&mut tag_counter, &live);
                // SAFETY: The allocator just handed us `layout.size()` writable bytes.
                unsafe { ptr.write_bytes(tag, layout.size()) };
                live.push(Live { ptr, layout, tag });
            }

            Op::Dealloc { slot } => {
                let Some(index) = pick(slot, live.len()) else {
                    continue;
                };
                let block = live.swap_remove(index);
                assert_intact(&block);
                // SAFETY: `block` is a live allocation from this allocator, freed once
                // with the layout it was allocated with.
                unsafe { allocator.dealloc(block.ptr, block.layout) };
            }

            Op::Realloc { slot, new_size } => {
                let Some(index) = pick(slot, live.len()) else {
                    continue;
                };
                assert_intact(&live[index]);
                let block = live.swap_remove(index);
                let new_size = usable_size(new_size, &arena);

                // SAFETY: The allocator's backing arena is live for the whole run.
                let before = unsafe { allocator.cursor_offset() };
                // SAFETY: `block` is live, `block.layout` is the layout it was allocated
                // with, and `new_size` is non-zero and a valid size for that alignment.
                let ptr = unsafe { allocator.realloc(block.ptr, block.layout, new_size) };
                if ptr.is_null() {
                    // SAFETY: As above.
                    let after = unsafe { allocator.cursor_offset() };
                    assert_eq!(after, before, "a failed realloc moved the cursor");
                    assert_intact(&block);
                    live.push(block);
                    continue;
                }

                let layout = Layout::from_size_align(new_size, block.layout.align())
                    .expect("realloc layout stays valid");
                assert_placement(&arena, ptr, layout, &live);

                // Only the copied prefix is guaranteed; the rest of a grown block is
                // uninitialised and must not be read.
                let kept = block.layout.size().min(new_size);
                // SAFETY: `ptr` is live for `new_size >= kept` bytes.
                let prefix = unsafe { core::slice::from_raw_parts(ptr, kept) };
                assert!(
                    prefix.iter().all(|byte| *byte == block.tag),
                    "realloc lost the preserved prefix of a live block"
                );

                // SAFETY: As above, for the whole new size.
                unsafe { ptr.write_bytes(block.tag, new_size) };
                live.push(Live {
                    ptr,
                    layout,
                    tag: block.tag,
                });
            }
        }
    }

    // Suffix reclamation's main property: once nothing is live, the whole arena is
    // back, whatever order the frees arrived in.
    while let Some(block) = live.pop() {
        assert_intact(&block);
        // SAFETY: `block` is live and freed once with its allocation layout.
        unsafe { allocator.dealloc(block.ptr, block.layout) };
    }
    // SAFETY: The allocator's backing arena is still live.
    let drained = unsafe { allocator.cursor_offset() };
    assert_eq!(
        drained, 0,
        "the arena was not fully reclaimed after freeing every block"
    );
}

/// Turns a raw size byte pair into a size the arena could plausibly satisfy, spanning
/// from a single byte up to the whole arena so that both success and exhaustion are hit.
fn usable_size(size: u16, arena: &Arena) -> usize {
    1 + (size as usize) % arena.layout.size()
}

fn request_layout(size: u16, align_log2: u8, arena: &Arena) -> Layout {
    let align = 1usize << (align_log2 % (MAX_ALIGN_LOG2 + 1));
    Layout::from_size_align(usable_size(size, arena), align).expect("valid request layout")
}

/// Maps a raw slot byte onto a live block, or `None` when nothing is live.
fn pick(slot: u8, live_count: usize) -> Option<usize> {
    (live_count > 0).then(|| slot as usize % live_count)
}

/// A tag not currently in use, so corruption of one live block is never masked by another
/// block happening to share its pattern. Zero is reserved for untouched arena bytes.
fn fresh_tag(counter: &mut u8, live: &[Live]) -> u8 {
    loop {
        *counter = counter.wrapping_add(1);
        if *counter == 0 {
            continue;
        }
        if !live.iter().any(|block| block.tag == *counter) {
            return *counter;
        }
    }
}

/// Checks a freshly returned pointer against every structural promise `GlobalAlloc` makes.
fn assert_placement(arena: &Arena, ptr: *mut u8, layout: Layout, live: &[Live]) {
    let start = ptr as usize;
    let end = start + layout.size();

    assert_eq!(start % layout.align(), 0, "returned pointer is misaligned");
    assert!(
        start >= arena.start() + WORD,
        "payload overlaps the arena's cursor word"
    );
    assert!(end <= arena.end(), "payload runs past the end of the arena");

    for block in live {
        let other_start = block.ptr as usize;
        let other_end = other_start + block.layout.size();
        assert!(
            end <= other_start || start >= other_end,
            "a new payload overlaps a live block"
        );
    }
}

/// Re-reads a live block and checks it still holds only its own tag.
fn assert_intact(block: &Live) {
    // SAFETY: `block` is a live allocation of `block.layout.size()` initialised bytes.
    let bytes = unsafe { core::slice::from_raw_parts(block.ptr, block.layout.size()) };
    assert!(
        bytes.iter().all(|byte| *byte == block.tag),
        "a live block was corrupted while it was still allocated"
    );
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Inputs replayed by `cargo test` and, more importantly, by `make miri-allocator`.
    ///
    /// To promote a fuzz finding into a permanent regression, copy the artifact from
    /// `fuzz/artifacts/ops/` into `testdata/corpus/` and add one line here.
    const CORPUS: &[(&str, &[u8])] = &[
        (
            "baseline",
            include_bytes!("../testdata/corpus/baseline.bin"),
        ),
        (
            "frontier_grow_shrink",
            include_bytes!("../testdata/corpus/frontier_grow_shrink.bin"),
        ),
        ("cascade", include_bytes!("../testdata/corpus/cascade.bin")),
        (
            "blocked_cascade",
            include_bytes!("../testdata/corpus/blocked_cascade.bin"),
        ),
        (
            "exhaustion",
            include_bytes!("../testdata/corpus/exhaustion.bin"),
        ),
    ];

    #[test]
    fn replays_seed_corpus() {
        for (name, input) in CORPUS {
            let ops = decode_ops(input);
            assert!(!ops.is_empty(), "corpus entry `{name}` decoded to no ops");
            run_ops(DEFAULT_ARENA_WORDS, &ops);
        }
    }

    #[test]
    fn a_truncated_input_still_decodes() {
        assert_eq!(decode_ops(&[]).len(), 0);
        assert_eq!(decode_ops(&[0]).len(), 1);
        assert_eq!(decode_ops(&[0, 1, 2, 3, 1]).len(), 2);
    }

    #[test]
    fn tags_stay_unique_among_live_blocks() {
        let layout = Layout::from_size_align(8, 8).unwrap();
        let live: Vec<Live> = (1..=3)
            .map(|tag| Live {
                ptr: core::ptr::dangling_mut(),
                layout,
                tag,
            })
            .collect();
        let mut counter = 0;
        assert_eq!(fresh_tag(&mut counter, &live), 4);
    }
}
