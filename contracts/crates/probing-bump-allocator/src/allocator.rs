use core::mem::{align_of, size_of};

use core::{
    alloc::{GlobalAlloc, Layout},
    cmp,
    ptr::{copy_nonoverlapping, null_mut, write_volatile},
};

use crate::types::{Address, Block};

/// Size and alignment of the stored cursor offset, which is also the size and alignment
/// of the probing write. Every reservation is padded to a multiple of it.
const WORD_SIZE: usize = size_of::<usize>();
const WORD_ALIGN: usize = align_of::<usize>();

/// Largest heap frame currently accepted by Solana's compute budget.
///
/// Used only as an upper bound, the actual limit in a given instruction could be lower.
pub const SOLANA_MAX_HEAP_LENGTH: usize = 256 * 1024;

/// The value stored by the probing write.
///
/// The value is arbitrary: the allocator never reads it back and the bytes are
/// overwritten by whatever the caller stores in the allocation. All that
/// matters is that the store happens, at the highest word of the reservation. A
/// recognisable pattern is used only so that a human reading a heap dump can tell it apart from zeroes.
pub const PROBE_PATTERN: usize = 0xA110_C8ED;

/// An upward-growing bump allocator that never reclaims, and that probes the top of every
/// allocation before handing it out.
///
/// The first word of the arena stores the byte offset of the cursor: the word immediately
/// before the next region available for allocation.
///
/// # Why it probes
///
/// A Solana program cannot ask the runtime how much heap it actually got: the transaction
/// picks the heap frame with `ComputeBudgetInstruction::request_heap_frame`, and the
/// program only sees the fixed start address. This allocator is therefore configured with
/// the largest frame Solana can grant ([`SOLANA_MAX_HEAP_LENGTH`]) and finds the real end
/// by touching it: every allocation performs a volatile store at the last word of its
/// reservation *before* the frontier moves and the pointer is returned. If that word lies
/// past the mapped heap, the runtime aborts the instruction with a heap access violation
/// instead of the program continuing with a pointer into unmapped memory.
///
/// The store is `write_volatile` so the compiler may not drop it.
///
/// # Concurrency
///
/// Allocator metadata is non-atomic. This type may be used as a global allocator only in
/// an environment, such as Solana program execution, that guarantees allocator calls
/// cannot overlap. Accordingly, its `GlobalAlloc` implementation is available only on
/// Solana and in single-threaded unit tests; normal host builds cannot install it as a
/// global allocator.
///
/// **`Allocator` is `Sync` only so that `#[global_allocator]` accepts it as a `static`.**
/// Sharing one across threads is a data race regardless of what that impl says; the
/// `target_os = "solana"` gate in
/// [`install_global_allocator!`](crate::install_global_allocator) is what keeps that from
/// happening. See the comment on the `unsafe impl Sync` below.
///
/// # Arena layout
///
/// The word at `start` stores the current cursor offset. Allocations begin in the word
/// after the cursor, each padded up to a whole number of words so that its final word — the
/// one the probing write targets — is always inside the reservation. That final word
/// becomes the next cursor:
///
/// ```text
/// [cursor offset: usize] [pad] [payload .. probe] [pad] [payload .. probe]
///  ^ cursor when empty                    ^ then here                ^ then here
/// ```
pub struct Allocator {
    base: Base,
    len: usize,
}

/// Where the arena starts.
#[derive(Clone, Copy)]
enum Base {
    /// A runtime-mapped region at a fixed address, kept as an integer and turned into a
    /// pointer on every use. The conversion must not happen in the constructor: the global
    /// allocator is a `static`, so the constructor runs during const evaluation, where an
    /// integer-to-pointer conversion yields a pointer with no provenance.
    Fixed(usize),
    /// A caller-owned region whose provenance every derived address inherits.
    #[cfg(any(test, feature = "host-testing"))]
    Owned(*mut u8),
}

// This `Sync` impl is a promise the type cannot keep on its own. Read before relying on it.
//
// `Allocator`'s cursor word is not atomic memory. Two threads allocating through one
// shared `Allocator` is a data race and therefore undefined behaviour. Nothing in this
// type prevents that; the guarantee comes entirely from Solana's program execution, which
// serialises every call into the global allocator.
//
// What keeps this sound is the `target_os = "solana"` gate in
// `install_global_allocator!`: it is what stops the allocator ever being installed as a
// global on a multi-threaded host. Do not remove that gate, and do not share an
// `Allocator` across threads anywhere else. The host test harness sidesteps the issue by
// keeping each `Allocator` as a local, never a shared static.
//
// SAFETY: on Solana, allocator calls cannot overlap, so no two accesses to `start`'s
// pointee are ever concurrent.
unsafe impl Sync for Allocator {}

impl Allocator {
    /// Creates an allocator tied to a fixed virtual-address range.
    ///
    /// This is the constructor used for Solana's runtime-provided program heap. It does
    /// not dereference any memory until the first allocation.
    ///
    /// # Safety
    ///
    /// `start` must be writable, live for the allocator's entire use, and aligned to
    /// `align_of::<usize>()`. `start + len` must not overflow and `len` must be larger
    /// than one `usize`. Every address after that must either be writable or
    /// synchronously abort execution when written. Before the allocator's first use, the
    /// pointer-sized word at `start` must be zero. Solana's runtime-provided program heap
    /// satisfies these requirements.
    ///
    /// Note that it is not possible to know the exact length of the available heap space
    /// (anywhere between 32KB and 256KB), so this allocator deliberately probes past it:
    /// an allocation that does not fit in the heap the transaction requested aborts on the
    /// probing write rather than returning a null pointer. That is not UB, because the
    /// runtime aborts (Allowed by GlobalAlloc).
    pub const unsafe fn with_fixed_address_range(start: usize, len: usize) -> Self {
        Self {
            base: Base::Fixed(start),
            len,
        }
    }

    /// Creates an allocator over a caller-owned region, preserving `start`'s provenance.
    ///
    /// This is the constructor used by the test harness: every address the allocator
    /// derives traces back to this allocation so Miri can check it under
    /// `-Zmiri-strict-provenance`.
    ///
    /// # Safety
    ///
    /// As [`Allocator::with_fixed_address_range`], but `start` must additionally be a
    /// pointer valid for reads and writes over the full `len` bytes. The resulting
    /// `Allocator` must stay thread-local: it is nominally `Sync`, but the host is
    /// multi-threaded and its cursor word is not atomic.
    #[cfg(any(test, feature = "host-testing"))]
    pub const unsafe fn with_owned_region(start: *mut u8, len: usize) -> Self {
        Self {
            base: Base::Owned(start),
            len,
        }
    }

    // Memory location where the cursor offset is stored.
    #[inline]
    fn cursor_offset_ptr(&self) -> *mut usize {
        self.start().as_mut_ptr()
    }

    #[inline]
    fn start(&self) -> Address {
        match self.base {
            Base::Fixed(address) => Address::from_exposed_provenance(address),
            #[cfg(any(test, feature = "host-testing"))]
            Base::Owned(ptr) => Address::from_ptr(ptr),
        }
    }

    /// Returns the current cursor: the word immediately *before* the next region available
    /// for allocation. Available memory begins in the word after it.
    ///
    /// # Safety
    ///
    /// The allocator's backing range must satisfy its constructor contract.
    #[inline]
    unsafe fn current_cursor(&self) -> Address {
        // SAFETY: Guaranteed by the allocator constructor's contract.
        let offset = unsafe { *self.cursor_offset_ptr() };
        debug_assert!(offset < self.len, "stored cursor offset escaped the arena");
        // SAFETY: Zero, or an offset previously saved within the arena, cannot overflow
        // when added to `start`.
        unsafe { self.start().unchecked_add(offset) }
    }

    /// Save `cursor` as a byte offset from the arena start.
    ///
    /// # Safety
    ///
    /// `cursor` must be the arena start or the last word of a reservation within the
    /// allocator's backing range.
    #[inline]
    unsafe fn save_cursor(&self, cursor: Address) {
        debug_assert!(
            cursor >= self.start() && cursor < self.arena_end(),
            "cursor saved outside the arena"
        );
        // SAFETY: The caller guarantees that `cursor` is not below `start`.
        let offset = unsafe { cursor.unchecked_distance_from(self.start()) };
        // SAFETY: Guaranteed by the allocator constructor's contract.
        unsafe { *self.cursor_offset_ptr() = offset };
    }

    #[inline]
    fn arena_end(&self) -> Address {
        // SAFETY: Guaranteed by the allocator constructor's contract.
        unsafe { self.start().unchecked_add(self.len) }
    }

    /// Host-testing only: the cursor's byte offset from the arena start, as stored in the
    /// arena's first word. Zero means nothing has been allocated yet.
    ///
    /// # Safety
    ///
    /// The allocator's backing range must satisfy its constructor contract.
    #[cfg(any(test, feature = "host-testing"))]
    #[inline]
    pub unsafe fn cursor_offset(&self) -> usize {
        // SAFETY: Guaranteed by the allocator constructor's contract.
        unsafe { *self.cursor_offset_ptr() }
    }

    /// Carves a reservation for `size` bytes at `alignment` out of the memory after
    /// `cursor`.
    ///
    /// The reservation is padded up to a whole number of words at both ends, so its final
    /// word is always fully inside it and correctly aligned for the probing write. A
    /// zero-sized request still reserves one word for the same reason.
    #[inline]
    fn block_for(&self, cursor: Address, size: usize, alignment: usize) -> Option<Block> {
        let alignment = cmp::max(alignment, WORD_ALIGN);
        let available = cursor.checked_add(WORD_SIZE)?;
        let payload = available.align_up(alignment)?;
        let payload_end = payload.checked_add(size)?;
        let end = payload_end.align_up(WORD_ALIGN)?;
        let end = if end == payload {
            end.checked_add(WORD_SIZE)?
        } else {
            end
        };
        // The only bound checked here is the *configured* one. On Solana the configured
        // length is the largest frame the compute budget can grant, which is usually more
        // than the runtime mapped; finding that real end is the probing write's job.
        if end > self.arena_end() {
            return None;
        }
        // The last word of the reservation, which is both the probe target and the cursor
        // the next allocation starts from.
        let probe = end.checked_sub(WORD_SIZE)?;
        debug_assert!(probe >= payload, "the probe word escaped its reservation");
        Some(Block { payload, probe })
    }
}

// SAFETY: Solana program execution is single-threaded, so allocator calls for
// the installed program heap cannot overlap.
unsafe impl GlobalAlloc for Allocator {
    #[inline]
    unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
        // SAFETY: Guaranteed by the allocator constructor's contract.
        let cursor = unsafe { self.current_cursor() };
        let Some(block) = self.block_for(cursor, layout.size(), layout.align()) else {
            return null_mut();
        };

        // The probing write happens before the
        // frontier moves and before the pointer is returned, so on Solana a reservation
        // that runs past the heap the transaction actually requested aborts here rather
        // than becoming a live pointer into unmapped memory.
        //
        // SAFETY: `block_for` placed an aligned probe word inside the reservation and
        // below the configured ceiling. The constructor requires the address to be
        // writable or to synchronously abort when written.
        unsafe { write_volatile(block.probe.as_mut_ptr::<usize>(), PROBE_PATTERN) };
        // The probe word is also the last word of the reservation, so publishing it as the
        // cursor is what makes the next allocation start right after this one.
        // SAFETY: `block.probe` is the last word of a reservation inside the arena.
        unsafe { self.save_cursor(block.probe) };
        block.payload.as_mut_ptr()
    }

    /// Does nothing: this allocator never reclaims.
    #[inline]
    unsafe fn dealloc(&self, _ptr: *mut u8, _layout: Layout) {}

    /// Always allocates afresh and copies. This is what `GlobalAlloc`'s default `realloc`
    /// does too.
    #[inline]
    unsafe fn realloc(&self, ptr: *mut u8, layout: Layout, new_size: usize) -> *mut u8 {
        let Ok(new_layout) = Layout::from_size_align(new_size, layout.align()) else {
            return null_mut();
        };
        // SAFETY: `new_layout` has the alignment of a layout this allocator served.
        let new_ptr = unsafe { self.alloc(new_layout) };
        if new_ptr.is_null() {
            return null_mut();
        }
        // The copy happens after the probing write, so it restores any payload byte the
        // probe overwrote inside the copied prefix.
        //
        // SAFETY: The new reservation is disjoint from every earlier one, and both blocks
        // cover at least the copied minimum of their requested sizes.
        unsafe { copy_nonoverlapping(ptr, new_ptr, cmp::min(layout.size(), new_size)) };
        new_ptr
    }
}

/// Host-testing only: where [`Allocator::alloc`] must have performed its probing write for
/// a returned `payload` and requested `size`.
///
/// Restates the rule independently of `block_for` so the harness can check the store
/// really happened, and happened at the top of the reservation.
#[cfg(any(test, feature = "host-testing"))]
pub fn probe_word_of(payload: *mut u8, size: usize) -> *const usize {
    let payload = Address::from_ptr(payload);
    let end = payload
        .checked_add(size)
        .and_then(|end| end.align_up(WORD_ALIGN))
        .expect("a served allocation cannot overflow");
    let end = if end == payload {
        end.checked_add(WORD_SIZE).expect("no overflow")
    } else {
        end
    };
    end.checked_sub(WORD_SIZE)
        .expect("a reservation is at least one word")
        .as_mut_ptr::<usize>()
        .cast_const()
}

#[cfg(test)]
mod tests {
    use super::*;

    fn arena_bytes<const N: usize>(backing: &mut [usize; N]) -> &mut [u8] {
        // SAFETY: A `[usize]` is aligned for `usize`, and the byte slice covers exactly
        // the initialized backing storage.
        unsafe {
            core::slice::from_raw_parts_mut(
                backing.as_mut_ptr().cast::<u8>(),
                N * size_of::<usize>(),
            )
        }
    }

    fn layout(size: usize, alignment: usize) -> Layout {
        Layout::from_size_align(size, alignment).unwrap()
    }

    unsafe fn allocator_for(arena: &mut [u8]) -> Allocator {
        // SAFETY: Callers provide an aligned, zero-initialized arena that remains live and
        // exclusively accessed for the allocator's use.
        unsafe { Allocator::with_owned_region(arena.as_mut_ptr(), arena.len()) }
    }

    unsafe fn allocate(allocator: &Allocator, size: usize, alignment: usize) -> *mut u8 {
        // SAFETY: Every test keeps its backing arena live and uses valid layouts.
        unsafe { allocator.alloc(layout(size, alignment)) }
    }

    unsafe fn offset(allocator: &Allocator) -> usize {
        // SAFETY: Tests construct live, valid arenas for the allocator.
        unsafe { allocator.cursor_offset() }
    }

    /// Reads back the word the probing write must have targeted.
    unsafe fn probe_value(payload: *mut u8, size: usize) -> usize {
        // SAFETY: The probe word is inside the reservation the allocator just served.
        unsafe { *probe_word_of(payload, size) }
    }

    #[test]
    fn an_untouched_arena_stores_a_zero_offset_and_allocates_after_the_cursor_word() {
        let mut backing = [0usize; 16];
        let arena = arena_bytes(&mut backing);
        let start = arena.as_ptr() as usize;
        // SAFETY: `arena` is aligned, zeroed, large enough, and exclusively borrowed.
        let allocator = unsafe { allocator_for(arena) };

        // SAFETY: The arena remains live and valid.
        assert_eq!(unsafe { offset(&allocator) }, 0);
        // SAFETY: As above.
        let ptr = unsafe { allocate(&allocator, 8, 8) };
        assert_eq!(ptr as usize, start + WORD_SIZE);
        // The cursor is the new block's probe word, one word on from the arena start.
        // SAFETY: As above.
        assert_eq!(unsafe { offset(&allocator) }, WORD_SIZE);
    }

    #[test]
    fn every_allocation_probes_its_last_word() {
        let mut backing = [0usize; 64];
        let arena = arena_bytes(&mut backing);
        // SAFETY: `arena` is aligned, zeroed, large enough, and exclusively borrowed.
        let allocator = unsafe { allocator_for(arena) };

        for size in [1, 7, 8, 9, 16, 31, 64] {
            // SAFETY: The arena remains live and valid.
            let ptr = unsafe { allocate(&allocator, size, 1) };
            assert!(!ptr.is_null(), "size {size} should fit");
            // SAFETY: As above.
            assert_eq!(
                unsafe { probe_value(ptr, size) },
                PROBE_PATTERN,
                "size {size} did not get a probing write"
            );
        }
    }

    #[test]
    fn the_probe_word_stays_inside_the_reservation() {
        let mut backing = [0usize; 64];
        let arena = arena_bytes(&mut backing);
        // SAFETY: `arena` is aligned, zeroed, large enough, and exclusively borrowed.
        let allocator = unsafe { allocator_for(arena) };

        // A single byte still reserves a whole word, so its probe lands on the payload
        // itself rather than below it.
        // SAFETY: The arena remains live and valid.
        let ptr = unsafe { allocate(&allocator, 1, 1) };
        assert_eq!(probe_word_of(ptr, 1) as usize, ptr as usize);

        for size in [1usize, 2, 7, 8, 9, 17, 33] {
            // SAFETY: As above.
            let ptr = unsafe { allocate(&allocator, size, 1) };
            let probe = probe_word_of(ptr, size) as usize;
            assert!(probe >= ptr as usize, "probe below the payload for {size}");
            assert!(
                probe + WORD_SIZE
                    <= (ptr as usize) + size.max(WORD_SIZE).next_multiple_of(WORD_SIZE),
                "probe above the reservation for {size}"
            );
            assert_eq!(probe % WORD_ALIGN, 0, "probe misaligned for {size}");
        }
    }

    #[test]
    fn successive_allocations_do_not_overlap_and_honour_alignment() {
        let mut backing = [0usize; 128];
        let arena = arena_bytes(&mut backing);
        // SAFETY: `arena` is aligned, zeroed, large enough, and exclusively borrowed.
        let allocator = unsafe { allocator_for(arena) };

        let mut previous_end = 0usize;
        for (size, alignment) in [(1usize, 1usize), (8, 8), (3, 4), (24, 16), (5, 64), (7, 2)] {
            // SAFETY: The arena remains live and valid.
            let ptr = unsafe { allocate(&allocator, size, alignment) };
            assert!(!ptr.is_null(), "({size}, {alignment}) should fit");
            let start = ptr as usize;
            assert_eq!(start % alignment, 0, "({size}, {alignment}) misaligned");
            assert!(
                start >= previous_end,
                "({size}, {alignment}) overlapped the previous block"
            );
            previous_end = start + size;
        }
    }

    #[test]
    fn dealloc_reclaims_nothing() {
        let mut backing = [0usize; 32];
        let arena = arena_bytes(&mut backing);
        // SAFETY: `arena` is aligned, zeroed, large enough, and exclusively borrowed.
        let allocator = unsafe { allocator_for(arena) };

        // SAFETY: The arena remains live and valid.
        let first = unsafe { allocate(&allocator, 16, 8) };
        // SAFETY: As above.
        let after_alloc = unsafe { offset(&allocator) };
        // SAFETY: `first` is a live allocation from this allocator with this layout.
        unsafe { allocator.dealloc(first, layout(16, 8)) };
        // SAFETY: As above.
        assert_eq!(unsafe { offset(&allocator) }, after_alloc);

        // SAFETY: As above.
        let second = unsafe { allocate(&allocator, 16, 8) };
        assert!(
            second as usize >= first as usize + 16,
            "a freed block was handed out again"
        );
    }

    #[test]
    fn realloc_moves_the_payload_and_keeps_the_prefix() {
        let mut backing = [0usize; 64];
        let arena = arena_bytes(&mut backing);
        // SAFETY: `arena` is aligned, zeroed, large enough, and exclusively borrowed.
        let allocator = unsafe { allocator_for(arena) };

        // SAFETY: The arena remains live and valid.
        let ptr = unsafe { allocate(&allocator, 8, 8) };
        // SAFETY: The allocator handed us 8 writable bytes.
        unsafe { ptr.write_bytes(0xAB, 8) };

        // SAFETY: `ptr` is live with this layout and 24 is a valid size for it.
        let grown = unsafe { allocator.realloc(ptr, layout(8, 8), 24) };
        assert!(!grown.is_null());
        assert_ne!(grown, ptr, "this allocator cannot grow in place");
        // SAFETY: `grown` is live for 24 bytes, of which the first 8 were copied.
        let prefix = unsafe { core::slice::from_raw_parts(grown, 8) };
        assert!(prefix.iter().all(|byte| *byte == 0xAB));

        // Shrinking also moves, and also keeps the prefix.
        // SAFETY: `grown` is live with the layout it was reallocated to.
        let shrunk = unsafe { allocator.realloc(grown, layout(24, 8), 4) };
        assert!(!shrunk.is_null());
        assert_ne!(shrunk, grown);
        // SAFETY: `shrunk` is live for 4 copied bytes.
        let prefix = unsafe { core::slice::from_raw_parts(shrunk, 4) };
        assert!(prefix.iter().all(|byte| *byte == 0xAB));
    }

    #[test]
    fn the_cursor_never_moves_backwards() {
        let mut backing = [0usize; 64];
        let arena = arena_bytes(&mut backing);
        // SAFETY: `arena` is aligned, zeroed, large enough, and exclusively borrowed.
        let allocator = unsafe { allocator_for(arena) };

        let mut previous = 0usize;
        for size in [8usize, 1, 32, 4, 16] {
            // SAFETY: The arena remains live and valid.
            let ptr = unsafe { allocate(&allocator, size, 8) };
            // SAFETY: As above.
            let after_alloc = unsafe { offset(&allocator) };
            assert!(after_alloc > previous, "alloc did not advance the cursor");
            previous = after_alloc;

            // SAFETY: `ptr` is live with this layout.
            unsafe { allocator.dealloc(ptr, layout(size, 8)) };
            // SAFETY: As above.
            assert_eq!(unsafe { offset(&allocator) }, previous);
        }
    }

    #[test]
    fn exhaustion_returns_null_without_moving_the_cursor() {
        let mut backing = [0usize; 8];
        let arena = arena_bytes(&mut backing);
        let len = arena.len();
        // SAFETY: `arena` is aligned, zeroed, large enough, and exclusively borrowed.
        let allocator = unsafe { allocator_for(arena) };

        // Fills everything after the cursor word.
        // SAFETY: The arena remains live and valid.
        let ptr = unsafe { allocate(&allocator, len - WORD_SIZE, 8) };
        assert!(!ptr.is_null(), "the arena should be exactly filled");
        // The cursor lands on the arena's last word, which is that block's probe word.
        // SAFETY: As above.
        assert_eq!(unsafe { offset(&allocator) }, len - WORD_SIZE);

        // SAFETY: As above.
        assert!(unsafe { allocate(&allocator, 1, 1) }.is_null());
        // SAFETY: As above.
        assert_eq!(unsafe { offset(&allocator) }, len - WORD_SIZE);
    }

    /// The largest requests `Layout` can even express are refused rather than wrapping. The
    /// checked arithmetic in `block_for` also guards genuine address-space overflow, which
    /// a real arena's addresses are far too low to reach from a test.
    #[test]
    fn an_absurdly_large_request_returns_null() {
        let mut backing = [0usize; 8];
        let arena = arena_bytes(&mut backing);
        // SAFETY: `arena` is aligned, zeroed, large enough, and exclusively borrowed.
        let allocator = unsafe { allocator_for(arena) };

        // SAFETY: The arena remains live and valid.
        assert!(unsafe { allocate(&allocator, isize::MAX as usize, 1) }.is_null());
        // SAFETY: As above.
        assert!(unsafe { allocate(&allocator, isize::MAX as usize - 63, 64) }.is_null());
        // SAFETY: As above.
        assert_eq!(unsafe { offset(&allocator) }, 0);
    }

    #[test]
    fn a_zero_sized_request_still_reserves_a_probeable_word() {
        let mut backing = [0usize; 8];
        let arena = arena_bytes(&mut backing);
        // SAFETY: `arena` is aligned, zeroed, large enough, and exclusively borrowed.
        let allocator = unsafe { allocator_for(arena) };

        // `GlobalAlloc` forbids this, so it is only a defensive check that nothing is
        // written below the payload.
        // SAFETY: The arena remains live and valid.
        let ptr = unsafe { allocate(&allocator, 0, 1) };
        assert!(!ptr.is_null());
        assert_eq!(probe_word_of(ptr, 0) as usize, ptr as usize);
        // SAFETY: As above.
        assert_eq!(unsafe { offset(&allocator) }, WORD_SIZE);
    }

    /// The only test that goes through [`Allocator::with_fixed_address_range`], whose
    /// address is a bare integer turned into a pointer with exposed provenance — which
    /// Miri's strict provenance mode rejects, and why the on-chain instantiation is outside
    /// Miri's reach.
    #[test]
    #[cfg_attr(miri, ignore)]
    fn a_fixed_address_allocator_starts_from_a_zero_offset() {
        let mut backing = [0usize; 16];
        let arena = arena_bytes(&mut backing);
        let start = arena.as_mut_ptr() as usize;
        let len = arena.len();

        // SAFETY: `start` addresses an aligned, zeroed, exclusively borrowed region of
        // `len` bytes that outlives the allocator.
        let allocator = unsafe { Allocator::with_fixed_address_range(start, len) };

        // SAFETY: The backing region remains live and valid.
        let ptr = unsafe { allocate(&allocator, 8, 8) };
        assert_eq!(ptr as usize, start + WORD_SIZE);
        // SAFETY: As above.
        assert_eq!(unsafe { probe_value(ptr, 8) }, PROBE_PATTERN);
    }
}
