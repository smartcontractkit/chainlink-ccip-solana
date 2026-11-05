use anchor_lang::error_code;

#[error_code]
pub enum ExecutorError {
    // offset error code so that they don't clash with other programs
    // (Anchor's base custom error code 6000 + offset 8000 = start at 14000)
    #[msg("The signer is unauthorized")]
    Unauthorized = 8000,
    #[msg("Invalid version of the onchain state")]
    InvalidVersion,
    #[msg("Invalid instruction data")]
    InvalidInstructionData,
    #[msg("Proposed owner cannot be the default pubkey")]
    DefaultOwnerProposal,
    #[msg("Proposed owner is the current owner")]
    RedundantOwnerProposal,
    #[msg("Invalid code version")]
    InvalidCodeVersion,
    #[msg("Invalid destination chain")]
    InvalidDestChain,
    #[msg("Duplicate CCV")]
    DuplicateCCV,
    #[msg("Exceeds maximum CCVs per message")]
    ExceedsMaxCCVs,
    #[msg("Invalid maximum CCVs per message")]
    InvalidMaxCCVs,
    #[msg("Invalid Finality")]
    InvalidFinality,
    #[msg("Invalid CCV")]
    InvalidCCV,
    #[msg("Fee aggregator not set")]
    FeeAggregatorNotSet,
    #[msg("Insufficient funds (vault empty)")]
    InsufficientFunds,
    #[msg("Too many CCV allowlist changes in one instruction")]
    TooManyCCVUpdates,
    #[msg("Allowlist exceeds the config account's CCV capacity")]
    TooManyCCVs,
}
