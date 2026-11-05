//! Errors in the Lombard Token Pool.
use anchor_lang::prelude::*;

#[error_code]
pub enum LombardTokenPoolError {
    #[msg("Invalid token data")]
    InvalidTokenData = 6000, // offset for LombardTokenPoolErrors, so they don't overlap with errors of other CCIP programs
    #[msg("Invalid receiver")]
    InvalidReceiver,
    #[msg("Invalid source domain")]
    InvalidSourceDomain,
    #[msg("Invalid destination domain")]
    InvalidDestDomain,
    #[msg("Invalid nonce")]
    InvalidNonce,
    #[msg("Invalid Token Messenger Minter")]
    InvalidTokenMessengerMinter,
    #[msg("Invalid Bridge")]
    InvalidBridge,
    #[msg("Invalid Message Sent Event Account")]
    InvalidMessageSentEventAccount,
    #[msg("Invalid Token Pool Extra Data")]
    InvalidTokenPoolExtraData,
    #[msg("Failed to handle GMP message")]
    FailedGmpMessageHandle,
    #[msg("Fund Manager is invalid or misconfigured")]
    InvalidFundManager,
    #[msg("Invalid destination for funds reclaim")]
    InvalidReclaimDestination,
    #[msg("Insufficient funds")]
    InsufficientFunds,
    #[msg("Invalid SOL amount")]
    InvalidSolAmount,
    #[msg("Malformed Attestation data")]
    MalformedAttestationData,
    #[msg("Remote chain id mismatch")]
    RemoteChainMismatch,
    #[msg("Amount mismatch")]
    AmountMismatch,
    #[msg("Invalid payload size")]
    InvalidPayloadLength,
    #[msg("Invalid payload")]
    InvalidPayload,
    #[msg("ABI word value overflows the target integer type")]
    AbiWordOverflow,
    #[msg("Unexpected size of receiver byte vector")]
    MalformedReceiver,

    // --- CCIP 2.0 / verifier additions ---
    #[msg("Cannot propose the default (zero) pubkey as a new admin")]
    DefaultAdminProposal,
    #[msg("Proposed admin is already the current admin")]
    RedundantAdminProposal,
    #[msg("Router signer must not be the default (zero) pubkey")]
    ZeroRouterSigner,
    #[msg("Path not configured for this remote chain")]
    PathNotConfigured,
    #[msg("Path lChainId/allowedCaller/remoteBridgeSender must not be zero")]
    InvalidPath,
    #[msg("Token is not in the verifier's supported token list")]
    UnsupportedToken,
    #[msg("Sender is not in the allowlist for this destination chain")]
    SenderNotAllowed,
    #[msg("Duplicate or zero-address CCV entry")]
    DuplicateOrZeroCcv,
    #[msg("Could not decode nested resolution inputs, or the passed-in account is neither this program's pool State nor its VerifierConfig")]
    InvalidResolutionInputs,
    #[msg("The bridge's configured destination token for this mint doesn't match the message's declared destination token, and no matching remote adapter override is configured")]
    RemoteTokenOrAdapterMismatch,
    #[msg("local_adapter is not supported")]
    LocalAdapterNotSupported,
    #[msg("Unsupported BPS fee")]
    UnsupportedBpsFee,
    #[msg("Unexpected source pool address")]
    UnexpectedSourcePool,
    #[msg("Fee aggregator not set")]
    FeeAggregatorNotSet,
    #[msg("Account owned by the wrong program")]
    InvalidResolutionAccountOwner,
}
