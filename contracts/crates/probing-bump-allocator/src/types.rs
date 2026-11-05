/// A location in the arena, carrying the provenance of the region it belongs to.
///
/// Holds a real pointer rather than a bare `usize` so every derived address inherits the
/// provenance of the arena it came from. That is what lets Miri check this code under
/// `-Zmiri-strict-provenance`.
#[repr(transparent)]
#[derive(Clone, Copy)]
pub struct Address(*mut u8);

impl PartialEq for Address {
    #[inline]
    fn eq(&self, other: &Self) -> bool {
        self.addr() == other.addr()
    }
}

impl Eq for Address {}

impl PartialOrd for Address {
    #[inline]
    fn partial_cmp(&self, other: &Self) -> Option<core::cmp::Ordering> {
        Some(self.cmp(other))
    }
}

impl Ord for Address {
    #[inline]
    fn cmp(&self, other: &Self) -> core::cmp::Ordering {
        self.addr().cmp(&other.addr())
    }
}

impl Address {
    /// An address in a region the runtime maps at a fixed location rather than one obtained
    /// from a Rust allocation. Solana's program heap is the only such region.
    ///
    /// Uses exposed provenance, which is what Rust sanctions for accessing memory at a fixed
    /// address that no Rust allocation owns. It must run at runtime: an integer-to-pointer
    /// conversion during const evaluation yields a pointer with no provenance, which may not
    /// be dereferenced. Accesses through it are outside what Miri's strict provenance mode
    /// can validate.
    #[inline]
    pub fn from_exposed_provenance(value: usize) -> Self {
        Self(core::ptr::with_exposed_provenance_mut(value))
    }

    /// An address carrying `ptr`'s provenance.
    ///
    /// Host-only: on Solana the arena is a fixed range and the allocator never has to turn
    /// a pointer it handed out back into an `Address`, because it keeps no per-allocation
    /// metadata to look up.
    #[cfg(any(test, feature = "host-testing"))]
    #[inline]
    pub const fn from_ptr(ptr: *mut u8) -> Self {
        Self(ptr)
    }

    #[inline]
    fn addr(self) -> usize {
        self.0.addr()
    }

    #[inline]
    fn with_addr(self, addr: usize) -> Self {
        Self(self.0.with_addr(addr))
    }

    #[inline]
    pub fn checked_add(self, offset: usize) -> Option<Self> {
        self.addr().checked_add(offset).map(|a| self.with_addr(a))
    }

    #[inline]
    pub fn checked_sub(self, offset: usize) -> Option<Self> {
        self.addr().checked_sub(offset).map(|a| self.with_addr(a))
    }

    #[inline]
    pub unsafe fn unchecked_distance_from(self, earlier: Self) -> usize {
        // SAFETY: The caller guarantees that `self` is not below `earlier`.
        unsafe { self.addr().unchecked_sub(earlier.addr()) }
    }

    #[inline]
    pub fn align_up(self, alignment: usize) -> Option<Self> {
        debug_assert!(
            alignment.is_power_of_two(),
            "alignment must be a power of two"
        );
        let aligned = self.addr().checked_add(alignment - 1)? & !(alignment - 1);
        Some(self.with_addr(aligned))
    }

    #[inline]
    pub const fn as_mut_ptr<T>(self) -> *mut T {
        self.0.cast()
    }

    #[inline]
    pub unsafe fn unchecked_add(self, offset: usize) -> Self {
        // SAFETY: The caller guarantees that the resulting address does not overflow.
        self.with_addr(unsafe { self.addr().unchecked_add(offset) })
    }
}

/// One reservation carved out of the memory after the cursor.
///
/// The allocator hands back `payload` and publishes `probe` as the new cursor, so the whole
/// range from `payload` through the end of `probe`'s word belongs to this allocation for as
/// long as the program runs — this allocator never reclaims. `probe` is the last whole word
/// of that range and is therefore always inside it, which is what makes the probing write
/// safe to perform even for requests smaller than a word.
#[derive(Clone, Copy)]
pub struct Block {
    pub payload: Address,
    pub probe: Address,
}
