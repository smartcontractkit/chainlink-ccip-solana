//! Op-sequence driver for [`Allocator`], shared by the `cargo-fuzz` target in `fuzz/` and
//! by the corpus replay test that runs under `cargo test` and Miri.
//!
//! The allocator is exercised as an ordinary object over a host arena, never installed as a
//! `#[global_allocator]`: its cursor word is non-atomic and the host is multi-threaded.
//!
//! Because this allocator never reclaims, the harness keeps *every* block it was ever
//! handed, freed or not, and keeps checking them: a freed block's bytes must stay exactly
//! as they were, forever. That, plus the strictly monotonic cursor, is the whole
//! behavioural contract on the bump side.
//!
//! # What is deliberately not covered
//!
//! The allocator is built with [`Allocator::with_owned_region`], which keeps the host
//! arena's provenance, not the production [`Allocator::with_fixed_address_range`]. It is
//! always given a `len` equal to the arena's real length, so the probing write always lands
//! in mapped memory. On Solana the program instead configures the 256KB maximum while the
//! runtime may map only 32KB, and relies on the probe past the mapped end aborting the
//! transaction. That write is genuine out-of-bounds UB on a host, so it cannot be fuzzed
//! here, and no test in this crate currently covers it.

use core::alloc::{GlobalAlloc, Layout};
use core::mem::size_of;
use std::alloc::{alloc_zeroed, dealloc};

use crate::{Allocator, PROBE_PATTERN, probe_word_of};

/// Arena size used by the fuzz target, in `usize` words (16KB).
///
/// Sized together with [`MAX_SMALL_REQUEST`] so that one run performs tens of successful
/// allocations *and* still reaches exhaustion, which a non-reclaiming allocator does
/// permanently after a single run of bad luck.
pub const DEFAULT_ARENA_WORDS: usize = 2048;

/// Cap on an ordinary request, in bytes. See [`usable_size`] for why most requests are
/// held down to this.
const MAX_SMALL_REQUEST: usize = 512;

/// Upper bound on simultaneously live blocks, so a run cannot spend all its ops growing the
/// bookkeeping vector.
const MAX_LIVE_BLOCKS: usize = 64;

/// Largest alignment requested, as a power-of-two exponent. Covers every alignment an
/// on-chain Rust type can ask for, plus over-aligned outliers.
const MAX_ALIGN_LOG2: u8 = 6;

/// Cap on decoded ops. Keeps a Miri replay (roughly 100x slower than native) bounded.
const MAX_OPS: usize = 1024;

const WORD: usize = size_of::<usize>();

/// One allocator call. Slot indices are taken modulo the live-block count, so every decoded
/// op is meaningful for almost any input rather than mostly being skipped.
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
/// AddressSanitizer redzone immediately past the arena end during fuzzing, and there is no
/// `Box` retag to invalidate the derived raw pointer under Miri's aliasing models.
struct Arena {
    ptr: *mut u8,
    layout: Layout,
}

impl Arena {
    fn new(words: usize) -> Self {
        assert!(
            words > 1,
            "an arena needs room for the cursor word and one allocation"
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

/// A block the allocator handed out, and the byte pattern it should still hold.
///
/// Kept after the block is freed as well: this allocator never reclaims, so a freed block's
/// contents must stay untouched for the rest of the run.
struct Block {
    ptr: *mut u8,
    layout: Layout,
    tag: u8,
}

/// Sliding-window bound on the retired ledger.
///
/// A run can retire far more blocks than there are distinct tags, so the ledger keeps the
/// most recent ones and forgets the oldest. `MAX_LIVE_BLOCKS + MAX_RETIRED_BLOCKS` must stay
/// below 255 so [`fresh_tag`] can always find an unused tag.
const MAX_RETIRED_BLOCKS: usize = 128;

// A run must never exhaust the tag space, or `fresh_tag` would spin forever.
const _: () = assert!(MAX_LIVE_BLOCKS + MAX_RETIRED_BLOCKS < 255);

/// Moves a dead block into the ledger, dropping the oldest entry once the window is full.
fn retire(retired: &mut Vec<Block>, block: Block) {
    if retired.len() >= MAX_RETIRED_BLOCKS {
        retired.remove(0);
    }
    retired.push(block);
}

/// Runs `ops` against a fresh arena of `arena_words` words, asserting the allocator's
/// invariants after every call. Panics on the first violation, which is what both libFuzzer
/// and Miri treat as a finding.
pub fn run_ops(arena_words: usize, ops: &[Op]) {
    let arena = Arena::new(arena_words);
    // SAFETY: The arena is writable for its whole length, zeroed (so the cursor word
    // starts at zero), `usize`-aligned, larger than one word, outlives the allocator, and
    // cannot overflow because it came from a successful allocation.
    let allocator = unsafe { Allocator::with_owned_region(arena.ptr, arena.layout.size()) };
    let mut live: Vec<Block> = Vec::new();
    // Every block ever freed or moved away from, still expected to hold its old bytes.
    let mut retired: Vec<Block> = Vec::new();
    let mut tag_counter: u8 = 0;
    // Highest cursor offset seen so far; the cursor must never come back below it.
    let mut cursor_watermark = 0;

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

                assert_placement(&arena, ptr, layout, &live, cursor_watermark);
                assert_probed(ptr, layout);
                cursor_watermark =
                    assert_cursor_advanced(&allocator, &arena, ptr, layout, cursor_watermark);

                let tag = fresh_tag(&mut tag_counter, &live, &retired);
                // The tag write also restores the payload bytes the probe overwrote.
                // SAFETY: The allocator just handed us `layout.size()` writable bytes.
                unsafe { ptr.write_bytes(tag, layout.size()) };
                live.push(Block { ptr, layout, tag });
            }

            Op::Dealloc { slot } => {
                let Some(index) = pick(slot, live.len()) else {
                    continue;
                };
                let block = live.swap_remove(index);
                assert_intact(&block, "before being freed");
                // SAFETY: The allocator's backing arena is live for the whole run.
                let before = unsafe { allocator.cursor_offset() };
                // SAFETY: `block` is a live allocation from this allocator, freed once with
                // the layout it was allocated with.
                unsafe { allocator.dealloc(block.ptr, block.layout) };
                // SAFETY: As above.
                let after = unsafe { allocator.cursor_offset() };
                assert_eq!(after, before, "dealloc reclaimed memory");
                // The block stays in the ledger: nothing may ever touch it again.
                retire(&mut retired, block);
            }

            Op::Realloc { slot, new_size } => {
                let Some(index) = pick(slot, live.len()) else {
                    continue;
                };
                assert_intact(&live[index], "before being reallocated");
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
                    assert_intact(&block, "after a failed realloc");
                    live.push(block);
                    continue;
                }

                assert_ne!(
                    ptr, block.ptr,
                    "realloc grew in place, which this allocator cannot do"
                );
                let layout = Layout::from_size_align(new_size, block.layout.align())
                    .expect("realloc layout stays valid");
                assert_placement(&arena, ptr, layout, &live, cursor_watermark);
                cursor_watermark =
                    assert_cursor_advanced(&allocator, &arena, ptr, layout, cursor_watermark);

                // Only the copied prefix is guaranteed; the rest of a grown block is
                // uninitialised and must not be read. The probe word is not checked here:
                // the copy legitimately overwrites it when the preserved prefix reaches
                // the top of the new reservation.
                let kept = block.layout.size().min(new_size);
                // SAFETY: `ptr` is live for `new_size >= kept` bytes.
                let prefix = unsafe { core::slice::from_raw_parts(ptr, kept) };
                assert!(
                    prefix.iter().all(|byte| *byte == block.tag),
                    "realloc lost the preserved prefix of a live block"
                );

                // The old block is now dead, and must never be handed out or written again.
                assert_intact(&block, "after being reallocated away from");
                retire(&mut retired, block);

                // A fresh tag rather than the old one, so that a write from the new block
                // reaching back into the retired one cannot hide behind a shared pattern.
                let tag = fresh_tag(&mut tag_counter, &live, &retired);
                // SAFETY: As above, for the whole new size.
                unsafe { ptr.write_bytes(tag, new_size) };
                live.push(Block { ptr, layout, tag });
            }
        }
    }

    // The headline property of a non-reclaiming allocator, and the exact opposite of
    // `suffix-eater-allocator`: freeing everything gives nothing back.
    // SAFETY: The allocator's backing arena is still live.
    let final_offset = unsafe { allocator.cursor_offset() };
    while let Some(block) = live.pop() {
        assert_intact(&block, "at the end of the run");
        // SAFETY: `block` is live and freed once with its allocation layout.
        unsafe { allocator.dealloc(block.ptr, block.layout) };
        retire(&mut retired, block);
    }
    // SAFETY: As above.
    assert_eq!(
        unsafe { allocator.cursor_offset() },
        final_offset,
        "freeing every block moved the cursor"
    );

    // Every block the run ever saw, in whatever state, still holds its own bytes.
    for block in &retired {
        assert_intact(block, "after the run finished");
    }
}

/// Turns a raw size byte pair into a size the arena could plausibly satisfy.
///
/// Most requests are held down to [`MAX_SMALL_REQUEST`] so that a run gets through many
/// successful allocations; roughly one input in eight asks for up to the whole arena, so
/// that exhaustion and the null-return paths are still reached early. Total and fixed, so
/// committed corpus files keep replaying the same sequence.
fn usable_size(size: u16, arena: &Arena) -> usize {
    if size & 0xE000 == 0xE000 {
        1 + (size as usize) % arena.layout.size()
    } else {
        1 + (size as usize) % MAX_SMALL_REQUEST
    }
}

fn request_layout(size: u16, align_log2: u8, arena: &Arena) -> Layout {
    let align = 1usize << (align_log2 % (MAX_ALIGN_LOG2 + 1));
    Layout::from_size_align(usable_size(size, arena), align).expect("valid request layout")
}

/// Maps a raw slot byte onto a live block, or `None` when nothing is live.
fn pick(slot: u8, live_count: usize) -> Option<usize> {
    (live_count > 0).then(|| slot as usize % live_count)
}

/// A tag not currently in use by any block the run still tracks — live or retired — so
/// corruption of one block is never masked by another block happening to share its pattern.
/// Zero is reserved for untouched arena bytes.
fn fresh_tag(counter: &mut u8, live: &[Block], retired: &[Block]) -> u8 {
    loop {
        *counter = counter.wrapping_add(1);
        if *counter == 0 {
            continue;
        }
        let taken = live
            .iter()
            .chain(retired)
            .any(|block| block.tag == *counter);
        if !taken {
            return *counter;
        }
    }
}

/// Checks a freshly returned pointer against every structural promise `GlobalAlloc` makes,
/// plus this allocator's own promise that it hands out strictly rising memory.
fn assert_placement(
    arena: &Arena,
    ptr: *mut u8,
    layout: Layout,
    live: &[Block],
    cursor_watermark: usize,
) {
    let start = ptr as usize;
    let end = start + layout.size();

    assert_eq!(start % layout.align(), 0, "returned pointer is misaligned");
    assert!(
        start >= arena.start() + WORD,
        "payload overlaps the arena's cursor word"
    );
    assert!(end <= arena.end(), "payload runs past the end of the arena");
    assert!(
        start > arena.start() + cursor_watermark,
        "a payload was handed out at or below the previous cursor"
    );

    for block in live {
        let other_start = block.ptr as usize;
        let other_end = other_start + block.layout.size();
        assert!(
            end <= other_start || start >= other_end,
            "a new payload overlaps a live block"
        );
    }
}

/// Checks that the probing write really happened, at the top word of the reservation.
///
/// Only valid immediately after `alloc`, before the payload is written: the pattern is
/// stored into bytes the caller owns and is expected to overwrite.
fn assert_probed(ptr: *mut u8, layout: Layout) {
    let probe = probe_word_of(ptr, layout.size());
    let start = ptr as usize;
    assert!(
        probe as usize >= start,
        "the probe word is below the payload"
    );
    assert_eq!(probe as usize % WORD, 0, "the probe word is misaligned");
    // SAFETY: The probe word lies inside the reservation the allocator just served.
    let stored = unsafe { *probe };
    assert_eq!(
        stored, PROBE_PATTERN,
        "alloc did not perform its probing write"
    );
}

/// Checks that the cursor moved to exactly the last word of the new reservation — the same
/// word the probe targeted — and returns the new watermark.
fn assert_cursor_advanced(
    allocator: &Allocator,
    arena: &Arena,
    ptr: *mut u8,
    layout: Layout,
    cursor_watermark: usize,
) -> usize {
    let probe = probe_word_of(ptr, layout.size()) as usize;
    let expected = probe - arena.start();
    // SAFETY: The allocator's backing arena is live for the whole run.
    let actual = unsafe { allocator.cursor_offset() };
    assert_eq!(
        actual, expected,
        "the cursor is not on the new reservation's last word"
    );
    assert!(
        expected > cursor_watermark,
        "a successful allocation did not advance the cursor"
    );
    assert!(
        probe + WORD <= arena.end(),
        "the reservation overruns the arena"
    );
    expected
}

/// Re-reads a block and checks it still holds only its own tag. Applied to freed blocks too:
/// this allocator never reuses memory, so their bytes must survive the whole run.
fn assert_intact(block: &Block, when: &str) {
    // SAFETY: `block` covers `block.layout.size()` initialised bytes inside the arena, which
    // stays mapped for the whole run whether or not the block has been freed.
    let bytes = unsafe { core::slice::from_raw_parts(block.ptr, block.layout.size()) };
    assert!(
        bytes.iter().all(|byte| *byte == block.tag),
        "a block was corrupted {when}"
    );
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Inputs replayed by `cargo test` and, more importantly, by `make miri-probing-bump`.
    ///
    /// To promote a fuzz finding into a permanent regression, copy the artifact from
    /// `fuzz/artifacts/ops/` into `testdata/corpus/` and add one line here.
    const CORPUS: &[(&str, &[u8])] = &[
        (
            "baseline",
            include_bytes!("../testdata/corpus/baseline.bin"),
        ),
        (
            "realloc_chain",
            include_bytes!("../testdata/corpus/realloc_chain.bin"),
        ),
        (
            "free_everything",
            include_bytes!("../testdata/corpus/free_everything.bin"),
        ),
        (
            "alignments",
            include_bytes!("../testdata/corpus/alignments.bin"),
        ),
        (
            "exhaustion",
            include_bytes!("../testdata/corpus/exhaustion.bin"),
        ),
        (
            "tiny_allocations",
            include_bytes!("../testdata/corpus/tiny_allocations.bin"),
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
    fn tags_stay_unique_across_live_and_retired_blocks() {
        let layout = Layout::from_size_align(8, 8).unwrap();
        let block = |tag| Block {
            ptr: core::ptr::dangling_mut(),
            layout,
            tag,
        };
        let live: Vec<Block> = (1..=3).map(block).collect();
        let retired: Vec<Block> = (4..=5).map(block).collect();
        let mut counter = 0;
        assert_eq!(fresh_tag(&mut counter, &live, &retired), 6);
    }

    #[test]
    fn most_requests_are_small_so_a_run_reaches_many_allocations() {
        let arena = Arena::new(DEFAULT_ARENA_WORDS);
        assert_eq!(usable_size(0, &arena), 1);
        assert_eq!(usable_size(511, &arena), 512);
        assert_eq!(usable_size(512, &arena), 1);
        // The escape hatch that keeps exhaustion reachable.
        assert!(usable_size(0xFFFF, &arena) > MAX_SMALL_REQUEST);
    }
}
