//! A tiny arrayvec <https://docs.rs/arrayvec/>, simplified by assuming Pod/Zeroable elements.
//!
//! Use this instead of hand-rolling `(buffer, len)` accessors: the recurring bug is reading the
//! whole backing array rather than just the initialized `..len` prefix.
//!
//! ```ignore
//! pub struct OnRampAddress {
//!     bytes: [u8; 64],
//!     len: u32,
//! }
//! arrayvec!(OnRampAddress, u8, 64, u32, bytes, len);
//! ```
//!
//! [`arrayvec_ro`] generates the read-only surface; [`arrayvec`] adds the mutators.
//!
//! Keep both fields private, as above. They are the invariant — a `pub len` lets a caller set a
//! length the backing array doesn't back, and a `pub` array lets one write past the length without
//! moving it. Privacy in Rust is per-module, so invoking the macro alongside the struct definition
//! still reaches the fields; everyone outside goes through the generated API.
//!
//! These types are usually deserialized straight from account data, so the stored length is
//! untrusted. Two consequences:
//!
//! - `len()` is clamped to the capacity, so it always agrees with `as_slice()` and no accessor can
//!   panic. Call `is_len_valid()` at the deserialization boundary to reject bad data outright.
//! - Mutators come in `try_*` and panicking flavours. Prefer `try_*`: a panic in a BPF program is
//!   an opaque abort with no error code.

/// Why a mutating operation could not be performed.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CapacityError {
    /// The operation would have grown the vec past its backing array.
    Overflow,
    /// The index was outside the initialized `..len` prefix.
    OutOfBounds,
}

impl std::fmt::Display for CapacityError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            CapacityError::Overflow => f.write_str("operation would exceed capacity"),
            CapacityError::OutOfBounds => f.write_str("index is outside the initialized prefix"),
        }
    }
}

impl std::error::Error for CapacityError {}

/// Read-only vec-like surface for a struct holding a fixed array plus a length.
/// See [`arrayvec`] for the variant that also generates mutators.
///
/// Parameters mirror the struct's own fields: `xs: [$ty; $capacity], len: $capacity_ty`. The field
/// names default to `xs` / `len`; pass them explicitly when they differ.
///
/// ```ignore
/// arrayvec_ro!(MultisigAccount, Pubkey, MAX_SIGNERS, u8, signers, n);
/// ```
#[macro_export]
macro_rules! arrayvec_ro {
    ($name:ident, $ty:ty, $capacity:expr, $capacity_ty:ty) => {
        $crate::arrayvec_ro!($name, $ty, $capacity, $capacity_ty, xs, len);
    };
    ($name:ident, $ty:ty, $capacity:expr, $capacity_ty:ty, $xs:ident, $len:ident) => {
        // Guards the `as $capacity_ty` casts in the mutators: a [T; 300] with a u8 len would wrap.
        const _: () = assert!(
            ($capacity as u128) <= (<$capacity_ty>::MAX as u128),
            "arrayvec: capacity does not fit in the length type"
        );

        // The declared capacity must be the array's real length; a mismatch is a type error here.
        const _: () = {
            #[allow(dead_code)]
            fn assert_backing_array_shape(v: $name) -> [$ty; $capacity] {
                v.$xs
            }
        };

        impl $name {
            /// Number of initialized elements, clamped to `capacity()` so a corrupt stored length
            /// can never disagree with `as_slice()`. See `is_len_valid()`.
            #[inline(always)]
            pub fn len(&self) -> usize {
                (self.$len as usize).min(self.capacity())
            }

            /// Whether the stored length actually fits the backing array. False only for corrupt
            /// or hostile data; check it where the value is deserialized.
            #[inline]
            pub fn is_len_valid(&self) -> bool {
                (self.$len as usize) <= self.capacity()
            }

            #[inline]
            pub fn is_empty(&self) -> bool {
                self.len() == 0
            }

            #[inline(always)]
            pub fn capacity(&self) -> usize {
                $capacity
            }

            #[inline]
            pub fn remaining_capacity(&self) -> usize {
                self.capacity() - self.len()
            }

            #[inline]
            pub fn as_slice(&self) -> &[$ty] {
                &self.$xs[..self.len()]
            }
        }

        impl std::ops::Deref for $name {
            type Target = [$ty];

            #[inline]
            fn deref(&self) -> &Self::Target {
                self.as_slice()
            }
        }
    };
}

/// Full vec-like surface: the [`arrayvec_ro`] accessors plus mutators.
///
/// Parameters mirror the struct's own fields: `xs: [$ty; $capacity], len: $capacity_ty`. The field
/// names default to `xs` / `len`; pass them explicitly when they differ.
///
/// ```ignore
/// arrayvec!(OnRampAddress, u8, 64, u32, bytes, len);
/// ```
///
/// Elements must be `Copy + Default`; `Default` is what vacated slots are zeroed to. There is
/// deliberately no `DerefMut`, so `vec[0] = x` does not silently mutate.
#[macro_export]
macro_rules! arrayvec {
    ($name:ident, $ty:ty, $capacity:expr, $capacity_ty:ty) => {
        $crate::arrayvec!($name, $ty, $capacity, $capacity_ty, xs, len);
    };
    ($name:ident, $ty:ty, $capacity:expr, $capacity_ty:ty, $xs:ident, $len:ident) => {
        $crate::arrayvec_ro!($name, $ty, $capacity, $capacity_ty, $xs, $len);

        impl $name {
            /// Mutable access to the initialized prefix. Explicit by design: there is no
            /// `DerefMut`. Reordering or overwriting elements cannot break the length invariant.
            #[inline]
            pub fn as_mut_slice(&mut self) -> &mut [$ty] {
                let len = self.len();
                &mut self.$xs[..len]
            }

            /// Appends an element, or errors if full.
            pub fn try_push(
                &mut self,
                element: $ty,
            ) -> std::result::Result<(), $crate::CapacityError> {
                let len = self.len();
                if len >= self.capacity() {
                    return Err($crate::CapacityError::Overflow);
                }
                self.$xs[len] = element;
                // Assigning rather than incrementing also repairs an out-of-range stored length.
                self.$len = (len + 1) as $capacity_ty;
                Ok(())
            }

            /// Appends an element. Panics if full; prefer `try_push`.
            pub fn push(&mut self, element: $ty) {
                self.try_push(element)
                    .expect("arrayvec: push past capacity")
            }

            /// Appends all of `data`, or errors and writes nothing if it does not fit.
            pub fn try_extend(
                &mut self,
                data: &[$ty],
            ) -> std::result::Result<(), $crate::CapacityError> {
                if data.len() > self.remaining_capacity() {
                    return Err($crate::CapacityError::Overflow);
                }
                let offset = self.len();
                self.$xs[offset..offset + data.len()].copy_from_slice(data);
                self.$len = (offset + data.len()) as $capacity_ty;
                Ok(())
            }

            /// Appends all of `data`. Panics if it does not fit; prefer `try_extend`.
            pub fn extend(&mut self, data: &[$ty]) {
                self.try_extend(data)
                    .expect("arrayvec: extend past capacity")
            }

            /// Truncates to empty, zeroing the live elements: the backing array lives in account
            /// data, where anything left past the length stays publicly readable.
            pub fn clear(&mut self) {
                for slot in self.as_mut_slice() {
                    *slot = <$ty>::default();
                }
                self.$len = 0;
            }

            /// Removes and returns the element at `index`, shifting the rest down. Errors if
            /// `index` is outside the initialized prefix.
            pub fn try_remove(
                &mut self,
                index: usize,
            ) -> std::result::Result<$ty, $crate::CapacityError> {
                let len = self.len();
                if index >= len {
                    return Err($crate::CapacityError::OutOfBounds);
                }
                let element = self.$xs[index];
                self.$xs.copy_within(index + 1..len, index); // move index+1..len back by one
                self.$xs[len - 1] = <$ty>::default(); // don't leave the vacated slot behind
                self.$len = (len - 1) as $capacity_ty;
                Ok(element)
            }

            /// Removes and returns the element at `index`. Panics if out of bounds; prefer
            /// `try_remove`.
            pub fn remove(&mut self, index: usize) -> $ty {
                self.try_remove(index)
                    .expect("arrayvec: remove out of bounds")
            }

            /// Inserts `element` at `index`, shifting the rest up. Errors if full, or if `index`
            /// is past the end of the initialized prefix.
            pub fn try_insert(
                &mut self,
                index: usize,
                element: $ty,
            ) -> std::result::Result<(), $crate::CapacityError> {
                let len = self.len();
                if len >= self.capacity() {
                    return Err($crate::CapacityError::Overflow);
                }
                if index > len {
                    return Err($crate::CapacityError::OutOfBounds);
                }
                self.$xs.copy_within(index..len, index + 1); // move index..len forward by one
                self.$xs[index] = element;
                self.$len = (len + 1) as $capacity_ty;
                Ok(())
            }

            /// Inserts `element` at `index`. Panics if full or out of bounds; prefer `try_insert`.
            pub fn insert(&mut self, index: usize, element: $ty) {
                self.try_insert(index, element)
                    .expect("arrayvec: insert past capacity or out of bounds")
            }
        }
    };
}

#[cfg(test)]
mod tests {
    // The macros are deliberately not blanket `#[allow(unused)]`, so a program opting into the
    // mutating variant it doesn't need gets told. These fixtures exercise everything.
    #![allow(dead_code)]

    use crate::CapacityError;

    pub struct ArrayVec {
        xs: [u8; 3],
        len: u32,
    }
    impl ArrayVec {
        pub fn new() -> Self {
            Self {
                len: 0,
                xs: [u8::default(); 3],
            }
        }
    }
    arrayvec!(ArrayVec, u8, 3, u32);

    #[test]
    fn remove() {
        let mut vec = ArrayVec::new();
        vec.push(1);
        vec.push(2);
        vec.push(3);

        let el = vec.remove(2);
        assert_eq!(el, 3);
        assert_eq!(vec.len(), 2);
        assert_eq!(vec.as_slice(), &[1, 2]);

        let el = vec.remove(0);
        assert_eq!(el, 1);
        assert_eq!(vec.len(), 1);
        assert_eq!(vec.as_slice(), &[2]);
    }

    #[test]
    fn insert() {
        let mut vec = ArrayVec::new();
        vec.insert(0, 3);
        vec.insert(0, 1);
        vec.insert(1, 2);
        assert_eq!(vec.as_slice(), &[1, 2, 3]);
    }

    #[test]
    #[should_panic]
    fn insert_overflow() {
        let mut vec = ArrayVec::new();
        vec.push(1);
        vec.push(2);
        vec.push(3);
        vec.insert(3, 4);
    }

    /// Bounds must be real checks, not `debug_assert!`s that vanish in the release builds we ship.
    mod fallible_mutators {
        use super::*;

        #[test]
        fn push_reports_overflow() {
            let mut vec = ArrayVec::new();
            assert_eq!(vec.try_push(1), Ok(()));
            assert_eq!(vec.try_push(2), Ok(()));
            assert_eq!(vec.try_push(3), Ok(()));
            assert_eq!(vec.try_push(4), Err(CapacityError::Overflow));
            assert_eq!(vec.as_slice(), &[1, 2, 3]);
        }

        #[test]
        fn extend_reports_overflow_and_writes_nothing() {
            let mut vec = ArrayVec::new();
            vec.extend(&[1, 2]);

            assert_eq!(vec.try_extend(&[3, 4]), Err(CapacityError::Overflow));
            assert_eq!(vec.as_slice(), &[1, 2], "must not partially write");

            assert_eq!(vec.try_extend(&[3]), Ok(()));
            assert_eq!(vec.as_slice(), &[1, 2, 3]);
        }

        #[test]
        fn remove_reports_out_of_bounds() {
            let mut vec = ArrayVec::new();
            vec.extend(&[1, 2]);

            // Index 2 is inside the backing array but past the prefix: previously a debug_assert
            // that compiled out, letting a stale element be read.
            assert_eq!(vec.try_remove(2), Err(CapacityError::OutOfBounds));
            assert_eq!(vec.try_remove(99), Err(CapacityError::OutOfBounds));
            assert_eq!(vec.try_remove(0), Ok(1));
            assert_eq!(vec.as_slice(), &[2]);
        }

        #[test]
        fn insert_reports_out_of_bounds_and_overflow() {
            let mut vec = ArrayVec::new();
            vec.extend(&[1, 2]);

            assert_eq!(vec.try_insert(3, 9), Err(CapacityError::OutOfBounds));
            assert_eq!(vec.try_insert(2, 3), Ok(()));
            assert_eq!(vec.as_slice(), &[1, 2, 3]);
            assert_eq!(vec.try_insert(0, 9), Err(CapacityError::Overflow));
        }
    }

    /// A length read out of account data is untrusted and can exceed the backing array.
    mod untrusted_length {
        use super::*;

        fn corrupt() -> ArrayVec {
            ArrayVec {
                xs: [1, 2, 3],
                len: 250, // claims far more elements than the array holds
            }
        }

        #[test]
        fn accessors_clamp_instead_of_panicking() {
            let vec = corrupt();

            assert!(!vec.is_len_valid());
            assert_eq!(vec.len(), 3);
            assert_eq!(vec.as_slice(), &[1, 2, 3]);
            assert_eq!(vec.len(), vec.as_slice().len());
            assert_eq!(vec.remaining_capacity(), 0);
            assert!(!vec.is_empty());
        }

        #[test]
        fn mutators_do_not_grow_past_capacity() {
            let mut vec = corrupt();

            assert_eq!(vec.try_push(4), Err(CapacityError::Overflow));
            assert_eq!(vec.try_extend(&[4]), Err(CapacityError::Overflow));
        }

        #[test]
        fn a_valid_length_is_reported_valid() {
            let vec = ArrayVec {
                xs: [1, 2, 0],
                len: 2,
            };
            assert!(vec.is_len_valid());
            assert_eq!(vec.as_slice(), &[1, 2]);
        }

        #[test]
        fn removing_repairs_the_stored_length() {
            let mut vec = corrupt();
            assert_eq!(vec.try_remove(0), Ok(1));
            assert!(vec.is_len_valid());
            assert_eq!(vec.as_slice(), &[2, 3]);
        }
    }

    /// Vacated slots are zeroed: in account data, anything past the length stays readable.
    mod zeroing {
        use super::*;

        #[test]
        fn clear_zeroes_the_live_elements() {
            let mut vec = ArrayVec::new();
            vec.extend(&[1, 2, 3]);
            vec.clear();

            assert!(vec.is_empty());
            assert_eq!(vec.as_slice(), &[]);
            assert_eq!(vec.xs, [0, 0, 0], "stale bytes left in the backing array");
        }

        #[test]
        fn remove_zeroes_the_vacated_slot() {
            let mut vec = ArrayVec::new();
            vec.extend(&[1, 2, 3]);

            assert_eq!(vec.remove(0), 1);
            assert_eq!(vec.as_slice(), &[2, 3]);
            assert_eq!(vec.xs, [2, 3, 0], "the shifted-from slot must be zeroed");
        }
    }

    /// The backing array and length fields don't have to be named `xs` / `len`.
    mod custom_field_names {
        pub struct Address {
            bytes: [u8; 4],
            size: u8,
        }
        arrayvec!(Address, u8, 4, u8, bytes, size);

        #[test]
        fn only_exposes_the_initialized_prefix() {
            // A trailing garbage byte past `size`, as a partial write would leave.
            let address = Address {
                bytes: [1, 2, 0, 0xFF],
                size: 2,
            };

            assert_eq!(address.as_slice(), &[1, 2]);
            assert_eq!(address.len(), 2);
            assert_eq!(address.capacity(), 4);
            assert!(!address.is_empty());
            // Deref means comparisons go through the prefix, not the backing array.
            assert_eq!(&*address, &[1, 2]);
        }

        #[test]
        fn extend_appends_after_the_prefix() {
            let mut address = Address {
                bytes: [0; 4],
                size: 0,
            };
            address.extend(&[7, 8]);
            address.extend(&[9]);

            assert_eq!(address.as_slice(), &[7, 8, 9]);
            assert_eq!(address.remaining_capacity(), 1);
        }
    }

    /// A read-only type exposes no mutation API at all.
    mod read_only {
        pub struct Signers {
            keys: [u32; 4],
            n: u8,
        }
        arrayvec_ro!(Signers, u32, 4, u8, keys, n);

        #[test]
        fn exposes_the_read_surface() {
            let signers = Signers {
                keys: [10, 20, 0, 0],
                n: 2,
            };

            assert_eq!(signers.as_slice(), &[10, 20]);
            assert_eq!(signers.len(), 2);
            assert_eq!(signers.capacity(), 4);
            assert!(signers.is_len_valid());
            // `contains` comes from the generated Deref, and so sees only the prefix.
            assert!(signers.contains(&20));
            assert!(!signers.contains(&30));
        }

        #[test]
        fn clamps_an_untrusted_length() {
            let signers = Signers {
                keys: [10, 20, 30, 40],
                n: 200,
            };

            assert!(!signers.is_len_valid());
            assert_eq!(signers.len(), 4);
        }
    }
}
