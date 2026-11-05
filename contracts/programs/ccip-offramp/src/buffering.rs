use anchor_lang::prelude::*;

use crate::error::CcipOfframpError;

#[account]
#[derive(InitSpace, Debug)]
pub struct Buffer {
    pub version: u8,
    pub bump: u8,
    pub buffer_id: [u8; 32],
    pub authority: Pubkey,
    pub chunk_bitmap: u64,
    pub num_chunks: u8,
    pub chunk_length: u32,
    #[max_len(0)]
    pub data: Vec<u8>,
}

pub trait Buffering {
    fn is_initialized(&self) -> bool;
    fn filled_chunks(&self) -> u8;
    fn is_complete(&self) -> bool;
    fn bytes(&self) -> Result<&[u8]>;
    fn add_chunk(
        &mut self,
        total_length: u32,
        chunk: &[u8],
        chunk_index: u8,
        num_chunks: u8,
    ) -> Result<()>;
}

impl Buffering for Buffer {
    fn is_initialized(&self) -> bool {
        !self.data.is_empty()
    }

    fn filled_chunks(&self) -> u8 {
        // SAFETY: count_ones() on u64 returns 0..=64, fits in u8
        self.chunk_bitmap.count_ones() as u8
    }

    fn is_complete(&self) -> bool {
        self.is_initialized() && self.filled_chunks() == self.num_chunks
    }

    fn bytes(&self) -> Result<&[u8]> {
        require!(self.is_complete(), CcipOfframpError::BufferIncomplete);

        Ok(&self.data)
    }

    fn add_chunk(
        &mut self,
        total_length: u32,
        chunk: &[u8],
        chunk_index: u8,
        num_chunks: u8,
    ) -> Result<()> {
        require_gt!(num_chunks, 0, CcipOfframpError::BufferInvalidChunkIndex);
        require_gt!(
            num_chunks,
            chunk_index,
            CcipOfframpError::BufferInvalidChunkIndex
        );
        require!(!chunk.is_empty(), CcipOfframpError::BufferInvalidChunkSize);
        require_gte!(
            total_length as usize,
            chunk.len(),
            CcipOfframpError::BufferInvalidChunkSize
        );

        if !self.is_initialized() {
            // SAFETY: chunk.len() bounded by Solana tx size (~1232 bytes)
            self.initialize(total_length, chunk.len() as u32, num_chunks, chunk_index)?;
        }

        require_eq!(
            self.data.len(),
            total_length as usize,
            CcipOfframpError::BufferInvalidLength
        );
        require_eq!(
            self.num_chunks,
            num_chunks,
            CcipOfframpError::BufferInvalidChunkNumber
        );

        let chunk_mask = 1u64 << chunk_index;
        require_eq!(
            chunk_mask & self.chunk_bitmap,
            0,
            CcipOfframpError::BufferAlreadyContainsChunk
        );

        require_gt!(
            self.num_chunks,
            chunk_index,
            CcipOfframpError::BufferInvalidChunkIndex
        );

        let stride = self.chunk_length as usize;
        let start = stride * chunk_index as usize;

        // Lengths are pinned exactly so the chunks tile the buffer with no gaps, the terminator
        // filling whatever space the others leave. A short terminator would otherwise leave a
        // zeroed hole in a buffer that `is_complete()` still reports as complete.
        let expected_length = if chunk_index as usize == self.num_chunks as usize - 1 {
            self.data
                .len()
                .saturating_sub(stride * (self.num_chunks as usize - 1))
        } else {
            stride
        };
        require_eq!(
            chunk.len(),
            expected_length,
            CcipOfframpError::BufferInvalidChunkSize
        );

        let end = start + chunk.len();
        require_gte!(
            self.data.len(),
            end,
            CcipOfframpError::BufferInvalidChunkSize
        );
        self.data[start..end].copy_from_slice(chunk);
        self.chunk_bitmap |= chunk_mask;

        Ok(())
    }
}

impl Buffer {
    fn initialize(
        &mut self,
        total_length: u32,
        chunk_length: u32,
        total_chunks: u8,
        chunk_index: u8,
    ) -> Result<()> {
        require!(
            !self.is_initialized(),
            CcipOfframpError::BufferAlreadyInitialized
        );
        require_gt!(total_length, 0, CcipOfframpError::BufferInvalidLength);
        require_gt!(chunk_length, 0, CcipOfframpError::BufferInvalidLength);
        require_gte!(
            total_length,
            chunk_length,
            CcipOfframpError::BufferInvalidLength
        );
        require_gt!(total_chunks, 0, CcipOfframpError::BufferInvalidChunkSize);
        require_gte!(64u8, total_chunks, CcipOfframpError::BufferInvalidChunkSize);

        // If we're initializing with the last chunk, it could be smaller
        // than the rest, so we calculate the chunk size based on the expected
        // size of all the others.
        let is_last = chunk_index == total_chunks - 1;
        let global_chunk_length = if is_last && total_chunks > 1 {
            // SAFETY: total_chunks is u8, widening to u32
            (total_length - chunk_length) / (total_chunks as u32 - 1)
        } else {
            chunk_length
        };

        require_gt!(
            global_chunk_length,
            0,
            CcipOfframpError::BufferInvalidChunkSize
        );

        require_eq!(
            // SAFETY: total_chunks is u8, widening to u32
            total_chunks as u32,
            div_ceil(total_length, global_chunk_length),
            CcipOfframpError::BufferInvalidLength,
        );
        self.chunk_length = global_chunk_length;
        self.num_chunks = total_chunks;
        self.data.resize(total_length as usize, 0);

        Ok(())
    }
}

// Using a.div_ceil(b) is unstable on Rust 1.68, so we implement our own version even though clippy may complain.
// See https://github.com/rust-lang/rust/issues/88581 for more info.
// Whenever we upgrade Anchor & Rust, we can remove this.
fn div_ceil<T: Into<u32>>(a: T, b: T) -> u32 {
    let (a, b) = (a.into(), b.into());
    assert!(a + b - 1 > 0);

    a.div_ceil(b)
}

pub trait AccountInfoExt {
    fn deserialize_as_buffer<T: AnchorDeserialize>(
        &self,
        authority: Pubkey,
    ) -> Result<(T, usize, [u8; 32])>;
}

impl<'info> AccountInfoExt for AccountInfo<'info> {
    fn deserialize_as_buffer<T: AnchorDeserialize>(
        &self,
        authority: Pubkey,
    ) -> Result<(T, usize, [u8; 32])> {
        require_keys_eq!(*self.owner, crate::ID, CcipOfframpError::BufferUnavailable);

        let buffer = Buffer::try_deserialize(&mut self.data.borrow().as_ref())?;

        require_keys_eq!(
            buffer.authority,
            authority,
            CcipOfframpError::WrongBufferAuthority
        );

        let value = T::deserialize(&mut buffer.bytes()?)
            .map_err(|_| CcipOfframpError::FailedToDeserializeBuffer)?;

        Ok((value, buffer.data.len(), buffer.buffer_id))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn empty_buffer() -> Buffer {
        Buffer {
            data: vec![],
            chunk_bitmap: 0,
            num_chunks: 0,
            chunk_length: 0,
            version: 0,
            bump: 0,
            buffer_id: [0; 32],
            authority: Pubkey::default(),
        }
    }

    #[test]
    fn rejects_invalid_initializations() {
        let mut buffer = empty_buffer();

        assert_eq!(
            buffer.initialize(10, 100, 1, 0).unwrap_err(),
            CcipOfframpError::BufferInvalidLength.into()
        );

        let mut buffer = empty_buffer();
        assert_eq!(
            buffer.initialize(0, 1, 1, 0).unwrap_err(),
            CcipOfframpError::BufferInvalidLength.into()
        );

        let mut buffer = empty_buffer();
        // Too small buffer sizes aren't OK as they'd break the bitmap
        assert_eq!(
            buffer.initialize(100000, 1, 1, 0).unwrap_err(),
            CcipOfframpError::BufferInvalidLength.into()
        );

        let mut buffer = empty_buffer();
        assert_eq!(
            buffer.initialize(100, 10, 255, 0).unwrap_err(),
            CcipOfframpError::BufferInvalidChunkSize.into()
        );
    }

    #[test]
    fn buffering_happy_path() {
        let mut buffer = empty_buffer();

        const DIVISIBLE_BY_THREE_SIZE: usize = 33;

        let message: &[u8; DIVISIBLE_BY_THREE_SIZE] = b"Very large message, wow so large.";
        let chunk_size = DIVISIBLE_BY_THREE_SIZE / 3;

        for i in 0..3 {
            assert!(!buffer.is_complete());
            buffer
                .add_chunk(
                    message.len() as u32,
                    &message[i * chunk_size..(i + 1) * chunk_size],
                    i as u8,
                    3,
                )
                .unwrap();
            assert_eq!(buffer.filled_chunks() as usize, i + 1);
        }

        dbg!(&buffer);
        assert!(buffer.is_complete());
        assert_eq!(buffer.bytes().unwrap(), message);
    }

    #[test]
    fn buffering_with_a_smaller_terminator() {
        let mut buffer = empty_buffer();

        const NOT_DIVISIBLE_BY_THREE_SIZE: usize = 35;

        let message: &[u8; NOT_DIVISIBLE_BY_THREE_SIZE] = b"Very large message, wow so large!!!";
        let chunk_size = NOT_DIVISIBLE_BY_THREE_SIZE / 3;

        for i in 0..4 {
            assert!(!buffer.is_complete());
            buffer
                .add_chunk(
                    message.len() as u32,
                    &message[i * chunk_size..message.len().min((i + 1) * chunk_size)],
                    i as u8,
                    4,
                )
                .unwrap();
            assert_eq!(buffer.filled_chunks() as usize, i + 1);
        }

        dbg!(std::str::from_utf8(&buffer.data).unwrap());
        assert!(buffer.is_complete());
        assert_eq!(buffer.bytes().unwrap(), message);
    }

    #[test]
    fn buffering_with_a_smaller_terminator_out_of_order() {
        let mut buffer = empty_buffer();

        const NOT_DIVISIBLE_BY_THREE_SIZE: usize = 35;

        let message: &[u8; NOT_DIVISIBLE_BY_THREE_SIZE] = b"Very large message, wow so large!!!";
        let chunk_size = NOT_DIVISIBLE_BY_THREE_SIZE / 3;

        // Note the rev(). The smaller terminator will be received first!
        for i in (0..4).rev() {
            assert!(!buffer.is_complete());
            buffer
                .add_chunk(
                    message.len() as u32,
                    &message[i * chunk_size..message.len().min((i + 1) * chunk_size)],
                    i as u8,
                    4,
                )
                .unwrap();
            assert_eq!(buffer.filled_chunks() as usize, 4 - i);
        }

        dbg!(&buffer);
        dbg!("{}", std::str::from_utf8(&buffer.data).unwrap());
        assert!(buffer.is_complete());
        assert_eq!(buffer.bytes().unwrap(), message);
    }

    #[test]
    fn rejects_invalid_chunks() {
        let mut buffer = empty_buffer();
        buffer.add_chunk(22, b"AAAAA", 0, 5).unwrap();
        // This is OK as it is the terminator.
        buffer.add_chunk(22, b"BB", 4, 5).unwrap();
        // This is not OK.
        buffer.add_chunk(22, b"CCCCCCCCCCC", 2, 5).unwrap_err();

        let mut buffer = empty_buffer();
        buffer.add_chunk(100, b"small", 0, 20).unwrap();
        // Invalid due to inconsistent total size.
        buffer.add_chunk(50, b"small", 1, 20).unwrap_err();

        let mut buffer = empty_buffer();
        buffer.add_chunk(100, b"small", 0, 20).unwrap();
        // Invalid due to repeated index.
        buffer.add_chunk(100, b"small", 0, 20).unwrap_err();

        let mut buffer = empty_buffer();
        buffer
            .add_chunk(10, b"Much bigger than ten characters", 0, 1)
            .unwrap_err();

        let mut buffer = empty_buffer();
        buffer.add_chunk(29, b"Medium sized", 1, 3).unwrap();
        // Cannot be smaller: only the terminator can (must come at the end.)
        buffer.add_chunk(29, b"small", 0, 3).unwrap_err();
        buffer.add_chunk(29, b"small", 2, 3).unwrap();
    }

    #[test]
    fn rejects_terminator_that_leaves_a_gap() {
        let mut buffer = empty_buffer();
        // A 10-byte stride over 25 bytes leaves 5 bytes for the terminator.
        buffer.add_chunk(25, &[1; 10], 0, 3).unwrap();
        buffer.add_chunk(25, &[2; 10], 1, 3).unwrap();
        assert_eq!(
            buffer.add_chunk(25, &[3; 1], 2, 3).unwrap_err(),
            CcipOfframpError::BufferInvalidChunkSize.into()
        );
        assert!(!buffer.is_complete());

        buffer.add_chunk(25, &[3; 5], 2, 3).unwrap();
        assert_eq!(buffer.bytes().unwrap()[20..], [3; 5]);
    }

    #[test]
    fn rejects_terminator_initialization_that_loses_a_remainder() {
        let mut buffer = empty_buffer();
        // A 5-byte terminator over 28 bytes would derive a 11-byte stride, leaving 6 bytes
        // unaccounted for at the tail.
        assert_eq!(
            buffer.add_chunk(28, &[3; 5], 2, 3).unwrap_err(),
            CcipOfframpError::BufferInvalidChunkSize.into()
        );

        let mut buffer = empty_buffer();
        buffer.add_chunk(28, &[3; 6], 2, 3).unwrap();
        buffer.add_chunk(28, &[1; 11], 0, 3).unwrap();
        buffer.add_chunk(28, &[2; 11], 1, 3).unwrap();
        assert_eq!(buffer.bytes().unwrap()[22..], [3; 6]);
    }

    #[test]
    fn rejects_oversized_terminator_without_panicking() {
        let mut buffer = empty_buffer();
        buffer.add_chunk(25, &[1; 10], 0, 3).unwrap();
        // Fits the stride, but not the 5 bytes left at the tail.
        assert_eq!(
            buffer.add_chunk(25, &[3; 10], 2, 3).unwrap_err(),
            CcipOfframpError::BufferInvalidChunkSize.into()
        );
    }

    #[test]
    fn completeness_implies_every_byte_was_written() {
        for total_length in 1u32..=64 {
            for num_chunks in 1u8..=8 {
                let mut buffer = empty_buffer();
                let stride = div_ceil(total_length, num_chunks as u32) as usize;
                let chunks: Vec<Vec<u8>> = (0..num_chunks as usize)
                    .map(|i| {
                        let start = (stride * i).min(total_length as usize);
                        let end = (start + stride).min(total_length as usize);
                        vec![1; end - start]
                    })
                    .collect();
                if chunks.iter().any(|c| c.is_empty()) {
                    continue;
                }

                for (i, chunk) in chunks.iter().enumerate() {
                    buffer
                        .add_chunk(total_length, chunk, i as u8, num_chunks)
                        .unwrap();
                }
                assert!(buffer.is_complete());
                assert!(buffer.bytes().unwrap().iter().all(|b| *b == 1));
            }
        }
    }
}
