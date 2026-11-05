use anchor_lang::error_code;

#[error_code]
pub enum CommitteeVerifierError {
    // offset error code so that they don't clash with other programs
    // (Anchor's base custom error code 6000 + offset 7000 = start at 13000)
    #[msg("The signer is unauthorized")]
    Unauthorized = 7000,
    #[msg("Invalid version of the onchain state")]
    InvalidVersion,
    #[msg("Invalid instruction data")]
    InvalidInstructionData,
    #[msg("Invalid message")]
    InvalidMessage,
    #[msg("Proposed owner cannot be the default pubkey")]
    DefaultOwnerProposal,
    #[msg("Proposed owner is the current owner")]
    RedundantOwnerProposal,
    #[msg("Invalid code version")]
    InvalidCodeVersion,
    #[msg("Proposed storage location admin cannot be the default pubkey")]
    DefaultStorageLocationAdminProposal,
    #[msg("Proposed storage location admin is the current storage location admin")]
    RedundantStorageLocationAdminProposal,
    #[msg("Invalid allowed sender PDA")]
    InvalidAllowedSenderPDA,
    #[msg("Threshold must be > 0 and <= number of signers")]
    InvalidSignerThreshold,
    #[msg("Signer cannot be the zero address")]
    ZeroAddressSigner,
    #[msg("Duplicate signer address")]
    DuplicateSigner,
    #[msg("Source chain is not configured")]
    SourceNotConfigured,
    #[msg("Invalid verifier results version tag")]
    InvalidVersionTag,
    #[msg("Malformed verifier results")]
    MalformedVerifierResults,
    #[msg("Not enough signatures to meet threshold")]
    InsufficientSignatures,
    #[msg("Invalid signature")]
    InvalidSignature,
    #[msg("Unauthorized signer")]
    UnauthorizedSigner,
    #[msg("Signatures not in ascending signer order")]
    SignaturesNotSorted,
    #[msg("Sender not allowed by the destination chain allowlist")]
    SenderNotAllowed,
    #[msg("Router signer cannot be the default (zero-value) pubkey")]
    ZeroRouterSigner,
    #[msg("Invalid Finality")]
    InvalidFinality,
    #[msg("Account to migrate contains invalid data")]
    InvalidAccountForMigration,
    #[msg("Fee aggregator not set")]
    FeeAggregatorNotSet,
    #[msg("Insufficient funds (vault empty)")]
    InsufficientFunds,
}
