mod interfaces;
mod v1;

pub mod router;

// Reused for v2 legacy price logic
pub(crate) use v1::get_validated_token_price;
pub(crate) use v1::public::PackedPrice;
pub(crate) use v1::public::CCIP_LOCK_OR_BURN_V1_RET_BYTES;
pub(crate) use v1::safe_deserialize;
