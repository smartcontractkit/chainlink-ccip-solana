use anchor_lang::prelude::*;

#[error_code]
pub enum CcipTokenPoolError {
    #[msg("Pool authority does not match token mint owner")]
    InvalidInitPoolPermissions,
    #[msg("Invalid RMN Remote Address")]
    InvalidRMNRemoteAddress,
    #[msg("Unauthorized")]
    Unauthorized,
    #[msg("Invalid inputs")]
    InvalidInputs,
    #[msg("Invalid state version")]
    InvalidVersion,
    #[msg("Caller is not ramp on router")]
    InvalidPoolCaller,
    #[msg("Sender not allowed")]
    InvalidSender,
    #[msg("Invalid source pool address")]
    InvalidSourcePoolAddress,
    #[msg("Invalid token")]
    InvalidToken,
    #[msg("Invalid token amount conversion")]
    InvalidTokenAmountConversion,
    #[msg("Key already existed in the allowlist")]
    AllowlistKeyAlreadyExisted,
    #[msg("Key did not exist in the allowlist")]
    AllowlistKeyDidNotExist,
    #[msg("Remote pool address already exists")]
    RemotePoolAddressAlreadyExisted,
    #[msg("Expected empty pool addresses during initialization")]
    NonemptyPoolAddressesInit,
    #[msg("Unexpected account derivation stage")]
    InvalidDerivationStage,

    // Rate limit errors
    #[msg("RateLimit: bucket overfilled")]
    RLBucketOverfilled,
    #[msg("RateLimit: max capacity exceeded")]
    RLMaxCapacityExceeded,
    #[msg("RateLimit: rate limit reached")]
    RLRateLimitReached,
    #[msg("RateLimit: invalid rate limit rate")]
    RLInvalidRateLimitRate,
    #[msg("RateLimit: disabled non-zero rate limit")]
    RLDisabledNonZeroRateLimit,

    // Lock/Release errors
    /// DEPRECATED: no longer enforced at runtime. Kept for backward compatibility.
    #[msg("Liquidity not accepted")]
    LiquidityNotAccepted,
    #[msg("Transferring zero tokens is not allowed")]
    TransferZeroTokensNotAllowed,
    #[msg("Insufficient pool balance")]
    InsufficientPoolBalance,

    // V2 Base Token Pool Errors
    #[msg("Invalid duplicated ccvs")]
    DuplicateCCVNotAllowed,
    #[msg("Invalid Threshold CCVs config")]
    MustSpecifyUnderThresholdCCVsForThresholdCCVs,
    #[msg("Invalid Direction for Get Required CCVs")]
    InboundOutboundDirectionOnly,
    #[msg("CCV address cannot be the zero/default pubkey")]
    CcvZeroAddressNotAllowed,
    #[msg("Threshold amount must be non-zero when threshold CCVs are configured")]
    ThresholdAmountRequiredForThresholdCcvs,
    #[msg("Requested finality is not allowed by the pool's configured allowed finality")]
    InvalidRequestedFinality,
    #[msg("Remote token address cannot be zero/empty")]
    ZeroTokenAddress,
    #[msg("Token transfer fee bps must not exceed 10000 (100%)")]
    InvalidTokenTransferFeeBps,
    #[msg("Token transfer fee destination bytes overhead is below the required minimum")]
    InvalidTokenTransferFeeDestBytesOverhead,
    #[msg("RateLimit: arithmetic error")]
    RLArithmeticError,
    #[msg("No fees available to withdraw (vault empty)")]
    NoFeesToWithdraw,
    #[msg("Invalid requested amount of fees to withdraw")]
    InvalidFeeWithdrawAmount,
    #[msg("Not enough stored fees to withdraw the requested amount")]
    InsufficientFeesToWithdraw,
    #[msg("Encoded source pool data is invalid")]
    InvalidSourcePoolData,
    #[msg("Remote chain config and remote pool show different decimals for token")]
    ConfigAndRemotePoolDisagreeOnDecimals,
    #[msg("The fee vault token account for the pool's own mint does not exist")]
    MissingFeeVaultTokenAccount,
    #[msg("A key appears more than once")]
    DuplicatedKeys,
}
