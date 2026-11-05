pub mod dest_chain;
pub mod fallback_get_token_transfer_fee;
pub mod observe;
pub mod quote_gas_for_exec_v2;
pub mod token_price;

// Maximum account versions the V2 instructions accept, shared across the module so the
// per-instruction contexts can't drift apart.
pub(crate) const V2_MAX_CONFIG_V: u8 = 1;
pub(crate) const V2_MAX_CHAINSTATE_V: u8 = 1;
pub(crate) const V2_MAX_BILLING_TOKEN_CONFIG_V: u8 = 1;
