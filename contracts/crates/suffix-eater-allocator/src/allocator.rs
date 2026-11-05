use core::mem::{align_of, size_of};

use core::{
    alloc::{GlobalAlloc, Layout},
    cmp,
    ptr::{copy_nonoverlapping, null_mut},
};

use crate::types::{Address, Block, Footer};

/// Size and alignment is the same for the stored cursor and
/// for the allocation footers.
const METADATA_SIZE: usize = size_of::<Footer>();
const METADATA_ALIGN: usize = align_of::<Footer>();

/// Largest heap frame currently accepted by Solana's compute budget.
///
/// Used only as an upper bound, the actual limit in a given instruction could be lower.
pub const SOLANA_MAX_HEAP_LENGTH: usize = 256 * 1024;

/// An upward-growing bump allocator that reclaims freed suffix blocks at once.
///
/// The first word of the arena stores the byte offset of the current cursor
/// from the arena start. An offset of zero represents an empty arena, whose
/// cursor is the stored-offset word itself. Otherwise, the cursor points to
/// the footer of the last allocation.
///
/// Each returned allocation has an aligned footer after its
/// payload. Deallocation marks that footer free, then retreats the bump
/// cursor across all consecutive free footers exposed at the
/// frontier.
///
/// This allocator does not reuse free blocks behind a newer live allocation.
/// It is intended for workloads whose temporary allocations tend to die
/// together between sequential phases such as CPI calls.
///
/// # Concurrency
///
/// Allocator metadata is non-atomic. This type may be used as a
/// global allocator only in an environment, such as Solana program execution,
/// that guarantees allocator calls cannot overlap. Accordingly, its
/// `GlobalAlloc` implementation is available only on Solana and in
/// single-threaded unit tests; normal host builds cannot install it as a global
/// allocator.
///
/// **`Allocator` is `Sync` only so that `#[global_allocator]` accepts it as a
/// `static`.** Sharing one across threads is a data race regardless of what that
/// impl says; the `target_os = "solana"` gate in
/// [`install_global_allocator!`](crate::install_global_allocator) is what keeps
/// that from happening. See the comment on the `unsafe impl Sync` below.
///
/// # Arena layout
///
/// The word at `start` stores the current cursor offset. Allocations begin
/// after that word and each end with another metadata word used as its footer.
/// The cursor addresses the word at `start` when empty and the newest footer
/// otherwise:
///
/// ```text
/// [cursor offset: usize] [alignment] [payload] [footer] ...
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
// `Allocator`'s metadata is not atomic memory. Two threads allocating through one shared
// `Allocator` is a data race and therefore undefined behaviour. Nothing in this type prevents
// that; the guarantee comes entirely from Solana's program execution, which serialises every
// call into the global allocator.
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
    /// This is the constructor used for Solana's runtime-provided program heap.
    /// It does not dereference any memory until the first allocation.
    ///
    /// # Safety
    ///
    /// `start` must be writable, live for the allocator's entire use, and
    /// aligned to `align_of::<usize>()`. `start + len` must not overflow and
    /// `len` must be larger than one `usize`. Every address after that
    /// must either be writable or synchronously abort execution when written.
    /// Before the allocator's first use, the pointer-sized word at `start` must
    /// be zero. Solana's runtime-provided program heap satisfies these
    /// requirements.
    ///
    /// Note that it's not possible to know the exact length of the available heap
    /// space (could be anywhere between 32kb and 256kb), which can lead this allocator
    /// to attempt to write out of bounds if allocated with a len beyond that
    /// requested by the compute budget instruction, rather than return a zero pointer.
    /// This won't lead to UB however as the runtime will abort, which is an acceptable
    /// outcome.
    pub const unsafe fn with_fixed_address_range(start: usize, len: usize) -> Self {
        Self {
            base: Base::Fixed(start),
            len,
        }
    }

    /// Creates an allocator over a caller-owned region, preserving `start`'s provenance.
    ///
    /// This is the constructor used by the test harness: every address the allocator
    /// derives traces back to this allocation so Miri can check it
    /// under `-Zmiri-strict-provenance`.
    ///
    /// # Safety
    ///
    /// As [`Allocator::with_fixed_address_range`], but `start` must additionally be a
    /// pointer valid for reads and writes over the full `len` bytes. The resulting
    /// `Allocator` must stay thread-local: it is nominally `Sync`, but the host is
    /// multi-threaded and its metadata is not atomic.
    #[cfg(any(test, feature = "host-testing"))]
    pub const unsafe fn with_owned_region(start: *mut u8, len: usize) -> Self {
        Self {
            base: Base::Owned(start),
            len,
        }
    }

    // Memory location where the cursor is stored.
    #[inline]
    fn cursor_offset_ptr(&self) -> *mut usize {
        self.start().as_mut_ptr()
    }

    /// The arena's first word, carrying the provenance every other address is derived from.
    #[inline]
    fn start(&self) -> Address {
        match self.base {
            Base::Fixed(address) => Address::from_exposed_provenance(address),
            #[cfg(any(test, feature = "host-testing"))]
            Base::Owned(ptr) => Address::from_ptr(ptr),
        }
    }

    /// Returns the current cursor. The cursor points to the
    /// footer of the last allocation, or in case of an empty arena,
    /// to address 0. In both cases, available memory begins in the next word.
    ///
    /// # Safety
    ///
    /// The allocator's backing range must satisfy its constructor contract.
    #[inline]
    unsafe fn current_cursor(&self) -> Address {
        // SAFETY: Guaranteed by the allocator constructor's contract.
        let offset = unsafe { *self.cursor_offset_ptr() };
        // SAFETY: Zero or an offset previously published within the arena
        // cannot overflow when added to `start`.
        unsafe { self.start().unchecked_add(offset) }
    }

    /// Save `cursor` as a byte offset from the arena start.
    ///
    /// # Safety
    ///
    /// `cursor` must be the arena start or an allocation footer within the
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

    /// Host-testing only: the cursor's byte offset from the arena start, as stored in
    /// the arena's first word. Zero means every allocation has been reclaimed.
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

    /// Calculates a block after `previous_cursor`.
    #[inline]
    fn block_for(&self, previous_cursor: Address, size: usize, alignment: usize) -> Option<Block> {
        let allocation_start = previous_cursor.checked_add(METADATA_SIZE)?;
        let payload = allocation_start.align_up(alignment)?;
        let payload_end = payload.checked_add(size)?;
        let footer_address = payload_end.align_up(METADATA_ALIGN)?;
        let end = footer_address.checked_add(METADATA_SIZE)?;
        if end > self.arena_end() {
            return None;
        }

        let span = footer_address.checked_distance_from(previous_cursor)?;
        Some(Block {
            payload,
            footer_address,
            span,
        })
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

        // Write the footer before moving the cursor. On Solana, this store is
        // also the effective-heap bounds check: a block beyond the mapped heap
        // aborts here, before an invalid payload pointer can be returned.
        // SAFETY: `block_for` kept the aligned footer below the configured
        // ceiling. The constructor requires the address to be writable or to
        // synchronously abort when written.
        unsafe {
            *block.footer_address.as_mut_ptr::<Footer>() = Footer::from_live_span(block.span);
            self.save_cursor(block.footer_address);
        }
        block.payload.as_mut_ptr()
    }

    #[inline]
    unsafe fn dealloc(&self, ptr: *mut u8, layout: Layout) {
        let Some(payload_end) = Address::from_ptr(ptr).checked_add(layout.size()) else {
            return;
        };
        let Some(footer_address) = payload_end.align_up(METADATA_ALIGN) else {
            return;
        };

        // SAFETY: GlobalAlloc requires `ptr` to identify a live allocation from
        // this allocator with `layout`. Its aligned footer follows the payload.
        unsafe {
            let footer_ptr = footer_address.as_mut_ptr::<Footer>();
            *footer_ptr = (*footer_ptr).freed();
        }

        // SAFETY: Guaranteed by the allocator constructor's contract.
        let mut cursor = unsafe { self.current_cursor() };

        while cursor != self.start() {
            // SAFETY: A non-empty cursor addresses the newest aligned footer.
            let footer = unsafe { *cursor.as_ptr::<Footer>() };
            if !footer.is_free() {
                break;
            }

            // Metadata integrity is a program-wide precondition: a heap overflow/underflow
            // that clobbers metadata breaks this allocator as it would break the default
            // Solana one, and is not defended against here. These assertions exist to
            // catch potential regressions in the test suites.
            let span = footer.span();
            debug_assert!(
                span != 0 && span & (METADATA_ALIGN - 1) == 0,
                "footer span is not a non-zero multiple of the metadata alignment"
            );
            let Some(previous_cursor) = cursor.checked_sub(span) else {
                break;
            };
            debug_assert!(
                previous_cursor >= self.start(),
                "footer span reaches below the arena start"
            );
            cursor = previous_cursor;
        }

        // SAFETY: `cursor` is either the old valid cursor, the arena start, or
        // an earlier footer decoded from a block span this allocator wrote.
        unsafe { self.save_cursor(cursor) };
    }

    #[inline]
    unsafe fn realloc(&self, ptr: *mut u8, layout: Layout, new_size: usize) -> *mut u8 {
        let Some(payload_end) = Address::from_ptr(ptr).checked_add(layout.size()) else {
            return null_mut();
        };
        let Some(footer_address) = payload_end.align_up(METADATA_ALIGN) else {
            return null_mut();
        };
        // SAFETY: Guaranteed by the allocator constructor's contract.
        let cursor = unsafe { self.current_cursor() };

        if footer_address == cursor {
            // SAFETY: The frontier cursor addresses this live allocation's
            // footer. Guaranteed by correct usage of the allocator API.
            let footer = unsafe { *footer_address.as_ptr::<Footer>() };
            debug_assert!(!footer.is_free(), "reallocating an already freed block");
            let span = footer.span();
            let Some(previous_cursor) = cursor.checked_sub(span) else {
                return null_mut();
            };
            let Some(block) = self.block_for(previous_cursor, new_size, layout.align()) else {
                return null_mut();
            };
            // `block_for` recomputes the payload from the same `previous_cursor`
            // and alignment the original allocation used, so it lands back on `ptr`.
            debug_assert!(
                block.payload == Address::from_ptr(ptr),
                "a frontier realloc moved the payload"
            );

            // Frontier reallocations keep their payload address. Moving the
            // footer upward grows the block; moving it downward physically
            // reclaims a shrink. As in `alloc`, a growth beyond Solana's
            // effective heap aborts on the footer write before returning.
            // SAFETY: `block_for` kept the aligned footer below the configured
            // ceiling, and the constructor supplies the write behavior.
            unsafe {
                *block.footer_address.as_mut_ptr::<Footer>() = Footer::from_live_span(block.span);
                self.save_cursor(block.footer_address);
            }
            return ptr;
        }

        // The general case uses allocate-copy-free.
        let Ok(new_layout) = Layout::from_size_align(new_size, layout.align()) else {
            return null_mut();
        };
        // SAFETY: `new_layout` is a valid, non-zero allocation request.
        let new_ptr = unsafe { self.alloc(new_layout) };
        if new_ptr.is_null() {
            return null_mut();
        }

        // SAFETY: Distinct live allocator blocks do not overlap, and both cover
        // at least the copied minimum of their requested sizes.
        unsafe { copy_nonoverlapping(ptr, new_ptr, cmp::min(layout.size(), new_size)) };
        // SAFETY: `ptr` still identifies the original live allocation.
        unsafe { self.dealloc(ptr, layout) };
        new_ptr
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn arena_bytes<const N: usize>(backing: &mut [usize; N]) -> &mut [u8] {
        // SAFETY: A `[usize]` is aligned for `usize`, and the byte slice covers
        // exactly the initialized backing storage.
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

    fn align_up(address: usize, alignment: usize) -> Option<usize> {
        Address::without_provenance(address)
            .align_up(alignment)
            .map(Address::get)
    }

    unsafe fn allocator_for(arena: &mut [u8]) -> Allocator {
        // SAFETY: Callers provide an aligned, zero-initialized arena that
        // remains live and exclusively accessed for the allocator's use.
        unsafe { Allocator::with_owned_region(arena.as_mut_ptr(), arena.len()) }
    }

    unsafe fn test_position(allocator: &Allocator) -> usize {
        // SAFETY: Tests construct live, valid arenas for the allocator.
        unsafe { allocator.current_cursor() }.get()
    }

    unsafe fn allocate(allocator: &Allocator, layout: Layout) -> *mut u8 {
        // SAFETY: Every test keeps its backing arena live and uses valid layouts.
        unsafe { allocator.alloc(layout) }
    }

    #[test]
    fn zero_cursor_offset_addresses_the_first_metadata_word() {
        let mut backing = [0usize; 16];
        let arena = arena_bytes(&mut backing);
        let expected = arena.as_ptr() as usize;

        // SAFETY: `arena` is aligned, zeroed, large enough, and exclusively borrowed.
        let allocator = unsafe { allocator_for(arena) };

        // SAFETY: The arena remains live and valid.
        assert_eq!(unsafe { test_position(&allocator) }, expected);
        // SAFETY: The stored cursor offset belongs to the live arena.
        assert_eq!(unsafe { *allocator.cursor_offset_ptr() }, 0);
    }

    #[test]
    fn a_footer_must_fit_below_an_odd_arena_end() {
        let mut backing = [0usize; 16];
        let bytes = arena_bytes(&mut backing);
        let len = METADATA_SIZE * 3 - 1;
        // SAFETY: The shortened arena still has an aligned start and valid size.
        let allocator = unsafe { allocator_for(&mut bytes[..len]) };

        // The cursor word consumes the first word. Even a one-byte allocation
        // needs one payload byte, alignment padding, and a complete footer.
        // SAFETY: The layout is valid and the arena remains live.
        assert!(unsafe { allocate(&allocator, layout(1, 1)) }.is_null());
    }

    #[test]
    fn allocations_grow_upward_without_overlap_and_honor_varied_alignment() {
        let mut backing = [0usize; 128];
        let arena = arena_bytes(&mut backing);
        let start = arena.as_ptr() as usize;
        // SAFETY: The local arena satisfies `allocator_for`'s contract.
        let allocator = unsafe { allocator_for(arena) };
        let alignments = [1, 2, 4, 8, 16, 32, 64, 128];
        let mut previous_end = start + METADATA_SIZE;

        for (index, alignment) in alignments.into_iter().enumerate() {
            let requested = layout(index * 2 + 3, alignment);
            // SAFETY: The layout is valid and the arena remains live.
            let ptr = unsafe { allocate(&allocator, requested) };
            assert!(!ptr.is_null());
            assert_eq!(ptr as usize % alignment, 0);
            assert!(ptr as usize >= previous_end);
            let footer_address = align_up(ptr as usize + requested.size(), METADATA_ALIGN).unwrap();
            assert_eq!(footer_address % METADATA_ALIGN, 0);
            let end = footer_address + METADATA_SIZE;
            // SAFETY: The arena remains live and valid.
            assert_eq!(unsafe { test_position(&allocator) }, footer_address);
            previous_end = end;
        }
    }

    #[test]
    fn allocation_failure_does_not_move_the_cursor() {
        let mut backing = [0usize; 10];
        let arena = arena_bytes(&mut backing);
        // SAFETY: The local arena satisfies `allocator_for`'s contract.
        let allocator = unsafe { allocator_for(arena) };

        // SAFETY: The layout is valid and the arena remains live.
        assert!(!unsafe { allocate(&allocator, layout(48, 1)) }.is_null());
        // SAFETY: The arena remains live and valid.
        let position_before_failure = unsafe { test_position(&allocator) };
        // SAFETY: The layout is valid and the arena remains live.
        assert!(unsafe { allocate(&allocator, layout(24, 1)) }.is_null());
        // SAFETY: The arena remains live and valid.
        assert_eq!(
            unsafe { test_position(&allocator) },
            position_before_failure
        );
    }

    #[test]
    fn allocation_records_its_span_in_a_trailing_footer() {
        let mut backing = [0usize; 32];
        let arena = arena_bytes(&mut backing);
        let start = arena.as_ptr() as usize;
        // SAFETY: The local arena satisfies `allocator_for`'s contract.
        let allocator = unsafe { allocator_for(arena) };
        // SAFETY: The arena remains live and valid.
        let previous_position = unsafe { test_position(&allocator) };
        let requested = layout(13, 16);

        // SAFETY: The layout is valid and the arena remains live.
        let ptr = unsafe { allocate(&allocator, requested) };
        let footer_address = align_up(ptr.addr() + requested.size(), METADATA_ALIGN).unwrap();
        // SAFETY: The footer belongs to the live allocation, and the pointer keeps the
        // arena's provenance rather than being rebuilt from an integer.
        let footer = unsafe { *ptr.with_addr(footer_address).cast::<Footer>() };
        // SAFETY: The arena remains live and valid.
        let position = unsafe { test_position(&allocator) };

        assert!(!footer.is_free());
        assert_eq!(footer.span(), position - previous_position);
        assert_eq!(position, footer_address);
        // SAFETY: The stored cursor offset belongs to the live arena.
        assert_eq!(unsafe { *allocator.cursor_offset_ptr() }, position - start);
    }

    #[test]
    fn deallocating_the_frontier_reclaims_payload_footer_and_padding() {
        let mut backing = [0usize; 32];
        let arena = arena_bytes(&mut backing);
        // SAFETY: The local arena satisfies `allocator_for`'s contract.
        let allocator = unsafe { allocator_for(arena) };
        // SAFETY: The arena remains live and valid.
        let empty = unsafe { test_position(&allocator) };
        let requested = layout(13, 16);
        // SAFETY: The layout is valid and the arena remains live.
        let ptr = unsafe { allocate(&allocator, requested) };
        assert!(!ptr.is_null());
        // SAFETY: `ptr` came from this allocator with `requested`.
        unsafe { allocator.dealloc(ptr, requested) };

        // SAFETY: The arena remains live and valid.
        assert_eq!(unsafe { test_position(&allocator) }, empty);
    }

    #[test]
    fn out_of_order_free_waits_for_the_live_frontier_then_cascades() {
        let mut backing = [0usize; 48];
        let arena = arena_bytes(&mut backing);
        // SAFETY: The local arena satisfies `allocator_for`'s contract.
        let allocator = unsafe { allocator_for(arena) };
        // SAFETY: The arena remains live and valid.
        let empty = unsafe { test_position(&allocator) };
        let requested = layout(9, 8);
        // SAFETY: Layouts are valid and the arena remains live.
        let older = unsafe { allocate(&allocator, requested) };
        // SAFETY: Layouts are valid and the arena remains live.
        let newer = unsafe { allocate(&allocator, requested) };
        // SAFETY: The arena remains live and valid.
        let frontier = unsafe { test_position(&allocator) };

        // SAFETY: `older` came from this allocator and remains live.
        unsafe { allocator.dealloc(older, requested) };
        // SAFETY: The arena remains live and valid.
        assert_eq!(unsafe { test_position(&allocator) }, frontier);

        // SAFETY: `newer` came from this allocator and remains live.
        unsafe { allocator.dealloc(newer, requested) };
        // SAFETY: Both adjacent freed blocks should be popped.
        assert_eq!(unsafe { test_position(&allocator) }, empty);
    }

    #[test]
    fn a_live_older_block_stops_a_multi_block_cascade() {
        let mut backing = [0usize; 64];
        let arena = arena_bytes(&mut backing);
        // SAFETY: The local arena satisfies `allocator_for`'s contract.
        let allocator = unsafe { allocator_for(arena) };
        let requested = layout(8, 8);
        // SAFETY: Layouts are valid and the arena remains live.
        let oldest = unsafe { allocate(&allocator, requested) };
        // SAFETY: The arena remains live and valid.
        let oldest_end = unsafe { test_position(&allocator) };
        // SAFETY: Layouts are valid and the arena remains live.
        let middle = unsafe { allocate(&allocator, requested) };
        // SAFETY: Layouts are valid and the arena remains live.
        let newest = unsafe { allocate(&allocator, requested) };

        // SAFETY: Both pointers identify live allocator blocks.
        unsafe {
            allocator.dealloc(middle, requested);
            allocator.dealloc(newest, requested);
        }
        // SAFETY: The arena remains live and valid.
        assert_eq!(unsafe { test_position(&allocator) }, oldest_end);

        // SAFETY: `oldest` remains live.
        unsafe { allocator.dealloc(oldest, requested) };
        // SAFETY: An empty arena's cursor addresses its stored-offset word.
        assert_eq!(
            unsafe { test_position(&allocator) },
            allocator.start().get()
        );
    }

    #[test]
    fn reclaimed_space_is_reused_at_the_same_address() {
        let mut backing = [0usize; 32];
        let arena = arena_bytes(&mut backing);
        // SAFETY: The local arena satisfies `allocator_for`'s contract.
        let allocator = unsafe { allocator_for(arena) };
        let requested = layout(24, 16);
        // SAFETY: The layout is valid and the arena remains live.
        let first = unsafe { allocate(&allocator, requested) };
        // SAFETY: `first` is live.
        unsafe { allocator.dealloc(first, requested) };
        // SAFETY: The layout is valid and the arena remains live.
        let second = unsafe { allocate(&allocator, requested) };

        assert_eq!(second, first);
    }

    // The only test that goes through `with_fixed_address_range`, whose address is a bare
    // integer turned into a pointer with exposed provenance. Miri's strict provenance mode
    // rejects that by construction, which is exactly why the on-chain instantiation is out
    // of its reach.
    #[test]
    #[cfg_attr(miri, ignore = "integer address uses exposed provenance")]
    fn fixed_address_allocator_uses_zero_offset_for_an_empty_arena() {
        let mut backing = [0usize; 16];
        let start = backing.as_mut_ptr() as usize;
        let len = size_of_val(&backing);
        // SAFETY: The local backing storage is aligned, writable, and live.
        let allocator = unsafe { Allocator::with_fixed_address_range(start, len) };
        let requested = layout(8, 8);

        // SAFETY: The fixed address and layout satisfy the contracts.
        let ptr = unsafe { allocate(&allocator, requested) };
        let expected_payload = start + METADATA_SIZE;
        let expected_position = expected_payload + requested.size();

        assert_eq!(ptr as usize, expected_payload);
        // SAFETY: The arena remains live and valid.
        assert_eq!(unsafe { test_position(&allocator) }, expected_position);
    }

    #[test]
    fn frontier_shrink_reclaims_space_and_regrowth_keeps_the_pointer() {
        let mut backing = [0usize; 48];
        let arena = arena_bytes(&mut backing);
        // SAFETY: The local arena satisfies `allocator_for`'s contract.
        let allocator = unsafe { allocator_for(arena) };
        let original = layout(32, 8);
        // SAFETY: The layout is valid and the arena remains live.
        let ptr = unsafe { allocate(&allocator, original) };
        // SAFETY: The arena remains live and valid.
        let original_position = unsafe { test_position(&allocator) };

        // SAFETY: `ptr` is the live frontier block described by `original`.
        let shrunk = unsafe { allocator.realloc(ptr, original, 16) };
        assert_eq!(shrunk, ptr);
        // SAFETY: Shrinking moves the footer and cursor toward the payload.
        let shrunk_position = unsafe { test_position(&allocator) };
        assert!(shrunk_position < original_position);

        let shrunk_layout = layout(16, 8);
        // SAFETY: `shrunk` is now described by `shrunk_layout`.
        let regrown = unsafe { allocator.realloc(shrunk, shrunk_layout, 32) };
        assert_eq!(regrown, ptr);
        // SAFETY: Regrowth returns the footer to its original position.
        assert_eq!(unsafe { test_position(&allocator) }, original_position);
    }

    #[test]
    fn frontier_growth_keeps_the_pointer_and_moves_the_footer_upward() {
        let mut backing = [0usize; 64];
        let arena = arena_bytes(&mut backing);
        // SAFETY: The local arena satisfies `allocator_for`'s contract.
        let allocator = unsafe { allocator_for(arena) };
        let original = layout(13, 16);
        // SAFETY: The layout is valid and the arena remains live.
        let ptr = unsafe { allocate(&allocator, original) };
        for index in 0..original.size() {
            // SAFETY: `ptr` covers `original.size()` writable bytes.
            unsafe { ptr.add(index).write(index as u8) };
        }
        // SAFETY: The arena remains live and valid.
        let old_position = unsafe { test_position(&allocator) };
        // SAFETY: The live frontier can be reallocated.
        let grown = unsafe { allocator.realloc(ptr, original, 61) };

        assert!(!grown.is_null());
        assert_eq!(grown, ptr);
        assert_eq!(grown as usize % original.align(), 0);
        // SAFETY: The arena remains live and valid.
        let new_position = unsafe { test_position(&allocator) };
        assert!(new_position > old_position);
        assert_eq!(
            new_position,
            align_up(grown as usize + 61, METADATA_ALIGN).unwrap()
        );
        for index in 0..original.size() {
            // SAFETY: `grown` covers the requested 61 bytes.
            assert_eq!(unsafe { grown.add(index).read() }, index as u8);
        }
    }

    #[test]
    fn frontier_growth_failure_leaves_the_original_allocation_untouched() {
        let mut backing = [0usize; 16];
        let arena = arena_bytes(&mut backing);
        // SAFETY: The local arena satisfies `allocator_for`'s contract.
        let allocator = unsafe { allocator_for(arena) };
        let original = layout(40, 8);
        // SAFETY: The layout is valid and the arena remains live.
        let ptr = unsafe { allocate(&allocator, original) };
        // SAFETY: `ptr` names at least one writable byte.
        unsafe { ptr.write(0xa5) };
        // SAFETY: The arena remains live and valid.
        let original_position = unsafe { test_position(&allocator) };

        // SAFETY: `ptr` is the live frontier described by `original`.
        let failed = unsafe { allocator.realloc(ptr, original, 256) };
        assert!(failed.is_null());
        // SAFETY: Failure preserves the original allocation and metadata.
        assert_eq!(unsafe { test_position(&allocator) }, original_position);
        // SAFETY: The original allocation remains live after failed realloc.
        assert_eq!(unsafe { ptr.read() }, 0xa5);
    }

    #[test]
    fn non_frontier_realloc_uses_allocate_copy_free_and_later_cascades() {
        let mut backing = [0usize; 96];
        let arena = arena_bytes(&mut backing);
        // SAFETY: The local arena satisfies `allocator_for`'s contract.
        let allocator = unsafe { allocator_for(arena) };
        // SAFETY: The arena remains live and valid.
        let empty = unsafe { test_position(&allocator) };
        let old_layout = layout(16, 8);
        let barrier_layout = layout(8, 8);
        // SAFETY: Layouts are valid and the arena remains live.
        let old = unsafe { allocate(&allocator, old_layout) };
        for index in 0..old_layout.size() {
            // SAFETY: `old` covers `old_layout.size()` bytes.
            unsafe { old.add(index).write((index + 10) as u8) };
        }
        // SAFETY: Layouts are valid and the arena remains live.
        let barrier = unsafe { allocate(&allocator, barrier_layout) };
        // SAFETY: The arena remains live and valid.
        let barrier_end = unsafe { test_position(&allocator) };

        // SAFETY: `old` is live but not the frontier.
        let grown = unsafe { allocator.realloc(old, old_layout, 40) };
        assert!(!grown.is_null());
        assert_ne!(grown, old);
        for index in 0..old_layout.size() {
            // SAFETY: `grown` covers the requested 40 bytes.
            assert_eq!(unsafe { grown.add(index).read() }, (index + 10) as u8);
        }

        let grown_layout = layout(40, 8);
        // SAFETY: Freeing the new frontier exposes the still-live barrier.
        unsafe { allocator.dealloc(grown, grown_layout) };
        assert_eq!(
            // SAFETY: The arena remains live and valid.
            unsafe { test_position(&allocator) },
            barrier_end
        );
        // SAFETY: Freeing the barrier exposes and cascades over the marked old
        // allocation.
        unsafe { allocator.dealloc(barrier, barrier_layout) };
        // SAFETY: The arena remains live and valid.
        assert_eq!(unsafe { test_position(&allocator) }, empty);
    }

    #[test]
    fn repeated_frontier_growth_does_not_accumulate_old_buffer_spans() {
        let mut backing = [0usize; 128];
        let arena = arena_bytes(&mut backing);
        // SAFETY: The local arena satisfies `allocator_for`'s contract.
        let allocator = unsafe { allocator_for(arena) };
        // SAFETY: The arena remains live and valid.
        let empty = unsafe { test_position(&allocator) };
        let mut current_layout = layout(8, 8);
        // SAFETY: The layout is valid and the arena remains live.
        let ptr = unsafe { allocate(&allocator, current_layout) };

        for new_size in [16, 32, 64, 128, 256] {
            // SAFETY: `ptr` is always the live frontier described by
            // `current_layout`.
            let grown = unsafe { allocator.realloc(ptr, current_layout, new_size) };
            assert_eq!(grown, ptr);
            current_layout = layout(new_size, 8);
        }

        // SAFETY: The arena remains live and valid.
        let used = unsafe { test_position(&allocator) } - empty;
        assert_eq!(used, current_layout.size() + METADATA_SIZE);
        // SAFETY: `ptr` remains the live frontier.
        unsafe { allocator.dealloc(ptr, current_layout) };
        // SAFETY: The arena remains live and valid.
        assert_eq!(unsafe { test_position(&allocator) }, empty);
    }
}
