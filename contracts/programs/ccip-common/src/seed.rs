use anchor_lang::{
    prelude::{AccountInfo, Pubkey},
    Key,
};
use anchor_spl::associated_token::get_associated_token_address_with_program_id;

// Fixed seeds - different contexts must use different PDA seeds
pub const CONFIG: &[u8] = b"config";
pub const ALLOWED_PRICE_UPDATER: &[u8] = b"allowed_price_updater";
pub const DEST_CHAIN: &[u8] = b"dest_chain";
pub const DEST_CHAIN_V2: &[u8] = b"dest_chain_v2";
pub const FEE_BILLING_TOKEN_CONFIG: &[u8] = b"fee_billing_token_config";
pub const PER_CHAIN_PER_TOKEN_CONFIG: &[u8] = b"per_chain_per_token_config";
pub const SOURCE_CHAIN: &[u8] = b"source_chain_state";
pub const COMMIT_REPORT: &[u8] = b"commit_report";
pub const REFERENCE_ADDRESSES: &[u8] = b"reference_addresses";
pub const STATE: &[u8] = b"state";
pub const CURSES: &[u8] = b"curses";
pub const DEST_CHAIN_STATE: &[u8] = b"dest_chain_state";
pub const NONCE: &[u8] = b"nonce";
pub const ALLOWED_OFFRAMP: &[u8] = b"allowed_offramp";
pub const EXECUTION_REPORT_BUFFER: &[u8] = b"execution_report_buffer";

// Router-owned per-receiver interface-version registry. Seeds:
//   [RECEIVER_REGISTRY, receiver_program_id]
// Absent ⇒ the receiver uses the v1 interface; present ⇒ read `receiver_version`.
pub const RECEIVER_REGISTRY: &[u8] = b"receiver_registry";
// PDA under a *receiver* program that must sign the `register_receiver` CPI into the router. Only
// the receiver program can `invoke_signed` for it, so its signature proves the receiver authorized
// its own registration.
pub const RECEIVER_REGISTRATION_SIGNER: &[u8] = b"ccip_receiver_registration";

// arbitrary messaging signer
pub const EXTERNAL_EXECUTION_CONFIG: &[u8] = b"external_execution_config";
// token pool interaction signer
pub const EXTERNAL_TOKEN_POOLS_SIGNER: &[u8] = b"external_token_pools_signer";
// signer for billing fee token transfer
pub const FEE_BILLING_SIGNER: &[u8] = b"fee_billing_signer";
// signer for source-side CCV hooks
pub const CCV_SIGNER: &[u8] = b"ccv_signer";

// token specific
pub const TOKEN_ADMIN_REGISTRY: &[u8] = b"token_admin_registry";
pub const POOL_STATE_SEED: &[u8] = b"ccip_tokenpool_config";
pub const POOL_SIGNER_SEED: &[u8] = b"ccip_tokenpool_signer";
pub const POOL_CHAINCONFIG_SEED: &[u8] = b"ccip_tokenpool_chainconfig";

pub fn token_admin_registry(mint: &Pubkey, router_program_id: Pubkey) -> Pubkey {
    Pubkey::find_program_address(&[TOKEN_ADMIN_REGISTRY, mint.as_ref()], &router_program_id).0
}

// v2 Token specific
pub const POOL_OVERRIDE_CHAINCONFIG_SEED: &[u8] = b"chainconfig_override"; // custom override configs for ccip_tokenpool_chainconfig
pub const POOL_CONFIG_OVERRIDE_TYPE_FTF: &[u8] = b"ftf";
pub const POOL_CHAINCONFIG_SEED_2_0: &[u8] = b"ccip_tokenpool_chainconfig_2_0";

// v2
pub const MESSAGE_EXEC_STATE: &[u8] = b"message_exec_state";
pub const EVENT_AUTHORITY: &[u8] = b"__event_authority"; // this is an Anchor-defined value for cpi-events
pub const EXECUTION_INPUTS_BUFFER: &[u8] = b"execution_inputs_buffer";
// Fee vault PDA for all fee receivers (e.g. receipt producers). Fee tokens will be sent to the ATA of this PDA.
pub const FEE_VAULT: &[u8] = b"fee_vault";
// v2.0 dest chain state. Chain selectors identify the destination chain regardless of CCIP
// version, so the v2 dest chain state uses a distinct seed to coexist with a v1 lane on the same
// selector (parallel v1/v2 lanes).
pub const DEST_CHAIN_STATE_V2: &[u8] = b"dest_chain_state_v2";

// CCV (cross-chain verifier) seeds
pub const REMOTE_CHAIN: &[u8] = b"remote_chain";
pub const STORAGE_LOCATIONS: &[u8] = b"storage_locations";
pub const CHAINS_WITH_CONFIGURED_SIGNERS_LIST: &[u8] = b"chains_with_signers";

// Resolves the vault ata of a receipt issuer.
//
// The vault authority is a PDA of the issuer program itself, so the issuer can withdraw its
// accrued fees by signing with these same seeds. The optional `qualifier` is an extra seed
// that lets a single issuer program keep separate, individually billable fee vaults for each
// resource it manages: token pools qualify the vault with the mint of the token they manage,
// so a pool program serving multiple tokens accrues the fees of each one separately.
pub fn vault_ata<'info>(
    receipt_issuer: Pubkey,
    qualifier: Option<&[u8]>,
    token_mint: &AccountInfo<'info>,
) -> Pubkey {
    let (issuer_vault, _) = match qualifier {
        Some(qualifier) => Pubkey::find_program_address(&[FEE_VAULT, qualifier], &receipt_issuer),
        None => Pubkey::find_program_address(&[FEE_VAULT], &receipt_issuer),
    };
    get_associated_token_address_with_program_id(
        &issuer_vault,
        &token_mint.key(),
        &token_mint.owner.key(),
    )
}
