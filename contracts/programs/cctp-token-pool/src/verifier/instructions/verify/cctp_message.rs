use anchor_lang::prelude::*;

use crate::error::CctpPoolVerifierError;

#[derive(Debug)]
pub struct CctpMessage<'a> {
    pub header: Header,
    pub body: Body<'a>,
}

impl<'a> CctpMessage<'a> {
    pub const MIN_SIZE: usize = Header::SIZE + Body::MIN_SIZE;

    pub fn from_bytes(data: &'a [u8]) -> Result<Self> {
        require_gte!(
            data.len(),
            Self::MIN_SIZE,
            CctpPoolVerifierError::InvalidInstructionData
        );

        let (header_data, body_data) = data.split_at(Header::SIZE);

        let header = Header {
            data: header_data.try_into().expect(
                "slice is hardcoded, always 148 bytes, and protected by the require_gte! above",
            ),
        };

        require_eq!(
            header.version(),
            Header::SUPPORTED_VERSION,
            CctpPoolVerifierError::InvalidCctpV1MessageVersion
        );

        let body = Body { data: body_data };

        require_eq!(
            body.version(),
            Body::SUPPORTED_VERSION,
            CctpPoolVerifierError::InvalidCctpV1MessageVersion
        );

        Ok(Self { header, body })
    }

    pub fn as_bytes(&self) -> Vec<u8> {
        let mut data = Vec::with_capacity(Self::MIN_SIZE + self.body.hook_data().len());
        data.extend_from_slice(&self.header.data);
        data.extend_from_slice(self.body.data);
        data
    }
}

#[derive(Debug)]
pub struct Header {
    data: [u8; Self::SIZE],
}

#[derive(Debug)]
pub struct Body<'a> {
    data: &'a [u8],
}

impl Header {
    pub const SUPPORTED_VERSION: u32 = 1;

    // See https://developers.circle.com/cctp/references/technical-guide#message-header
    const SIZE: usize = 148;

    const VERSION: SliceRange = slice_range(0, 4);
    const SOURCE_DOMAIN: SliceRange = slice_range(4, 4);
    const DESTINATION_DOMAIN: SliceRange = slice_range(8, 4);
    const NONCE: SliceRange = slice_range(12, 32);
    const _SENDER: SliceRange = slice_range(44, 32);
    const RECIPIENT: SliceRange = slice_range(76, 32);
    const _DESTINATION_CALLER: SliceRange = slice_range(108, 32);
    const _MIN_FINALITY_THRESHOLD: SliceRange = slice_range(140, 4);
    const _FINALITY_THRESHOLD_EXECUTED: SliceRange = slice_range(144, 4);

    pub fn version(&self) -> u32 {
        self.u32(Self::VERSION)
    }

    pub fn source_domain(&self) -> u32 {
        self.u32(Self::SOURCE_DOMAIN)
    }

    pub fn destination_domain(&self) -> u32 {
        self.u32(Self::DESTINATION_DOMAIN)
    }

    pub fn nonce(&self) -> [u8; 32] {
        Self::NONCE
            .of(&self.data)
            .try_into()
            .expect("slice is hardcoded, always 32 bytes")
    }

    pub fn _sender(&self) -> Pubkey {
        self.pubkey(Self::_SENDER)
    }

    pub fn recipient(&self) -> Pubkey {
        self.pubkey(Self::RECIPIENT)
    }

    pub fn _destination_caller(&self) -> Pubkey {
        self.pubkey(Self::_DESTINATION_CALLER)
    }

    pub fn _min_finality_threshold(&self) -> u32 {
        self.u32(Self::_MIN_FINALITY_THRESHOLD)
    }

    pub fn _finality_threshold_executed(&self) -> u32 {
        self.u32(Self::_FINALITY_THRESHOLD_EXECUTED)
    }
}

impl<'a> Body<'a> {
    pub const SUPPORTED_VERSION: u32 = 1;

    // See https://developers.circle.com/cctp/references/technical-guide#message-body
    const MIN_SIZE: usize = 228;

    const VERSION: SliceRange = slice_range(0, 4);
    const BURN_TOKEN: SliceRange = slice_range(4, 32);
    const MINT_RECIPIENT: SliceRange = slice_range(36, 32);
    const AMOUNT: SliceRange = slice_range(68, 32);
    const MESSAGE_SENDER: SliceRange = slice_range(100, 32);
    const _MAX_FEE: SliceRange = slice_range(132, 32);
    const FEE_EXECUTED: SliceRange = slice_range(164, 32);
    const _EXPIRATION_BLOCK: SliceRange = slice_range(196, 32);

    pub fn version(&self) -> u32 {
        self.u32(Self::VERSION)
    }

    pub fn burn_token(&self) -> Pubkey {
        self.pubkey(Self::BURN_TOKEN)
    }

    pub fn mint_recipient(&self) -> Pubkey {
        self.pubkey(Self::MINT_RECIPIENT)
    }

    pub fn amount(&self) -> Result<u64> {
        self.padded_u64(Self::AMOUNT)
    }

    pub fn message_sender(&self) -> Pubkey {
        self.pubkey(Self::MESSAGE_SENDER)
    }

    pub fn _max_fee(&self) -> [u8; 32] {
        Self::_MAX_FEE
            .of(self.data)
            .try_into()
            .expect("slice is hardcoded, always 32 bytes")
    }

    pub fn fee_executed(&self) -> Result<u64> {
        self.padded_u64(Self::FEE_EXECUTED)
    }

    pub fn _expiration_block(&self) -> [u8; 32] {
        Self::_EXPIRATION_BLOCK
            .of(self.data)
            .try_into()
            .expect("slice is hardcoded, always 32 bytes")
    }

    pub fn hook_data(&self) -> &[u8] {
        // SAFETY: guaranteed by the require_gte! in CctpMessage::from_bytes that self.data.len() >= Body::MIN_SIZE
        &self.data[Self::MIN_SIZE..]
    }
}

pub struct SliceRange {
    pub start: usize,
    pub end: usize,
    pub len: usize,
}

impl SliceRange {
    #[inline]
    pub fn of<'a>(&self, data: &'a [u8]) -> &'a [u8] {
        &data[self.start..self.end]
    }
}

pub const fn slice_range(start: usize, len: usize) -> SliceRange {
    SliceRange {
        start,
        end: start + len,
        len,
    }
}

trait Parse {
    fn data(&self) -> &[u8];

    #[inline]
    fn u32(&self, range: SliceRange) -> u32 {
        u32::from_be_bytes(
            range
                .of(self.data())
                .try_into()
                .expect("slice is hardcoded, always 4 bytes"),
        )
    }

    #[inline]
    fn pubkey(&self, range: SliceRange) -> Pubkey {
        Pubkey::new_from_array(
            range
                .of(self.data())
                .try_into()
                .expect("slice is hardcoded, always 32 bytes"),
        )
    }

    #[inline]
    fn padded_u64(&self, range: SliceRange) -> Result<u64> {
        let slice = range.of(self.data());
        debug_assert!(range.len > std::mem::size_of::<u64>()); // slices are hardcoded, this is just to catch dev errors early

        let (padding, bytes) = slice.split_at(range.len - std::mem::size_of::<u64>());
        require!(
            padding.iter().all(|&b| b == 0),
            CctpPoolVerifierError::InvalidCctpMessage
        );

        Ok(u64::from_be_bytes(
            bytes
                .try_into()
                .expect("slice is hardcoded, always 8 bytes"),
        ))
    }
}

impl Parse for Header {
    #[inline]
    fn data(&self) -> &[u8] {
        &self.data
    }
}

impl Parse for Body<'_> {
    #[inline]
    fn data(&self) -> &[u8] {
        self.data
    }
}
