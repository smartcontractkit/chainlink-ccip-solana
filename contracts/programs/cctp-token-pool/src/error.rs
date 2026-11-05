use anchor_lang::error_code;

#[error_code]
pub enum CctpPoolVerifierError {
    // ------------------------------
    // Pool errors
    // ------------------------------
    #[msg("Invalid token data")]
    InvalidTokenData = 6000, // offset for CctpTokenPoolErrors, so they don't overlap with errors of other CCIP programs
    #[msg("Invalid receiver")]
    InvalidReceiver,
    #[msg("Invalid source domain")]
    InvalidSourceDomain,
    #[msg("Invalid destination domain")]
    InvalidDestDomain,
    #[msg("Invalid nonce")]
    InvalidNonce,
    #[msg("CCTP message is malformed or too short")]
    MalformedCctpV1Message,
    #[msg("Invalid CCTP message version")]
    InvalidCctpV1MessageVersion,
    #[msg("Invalid Token Messenger Minter")]
    InvalidTokenMessengerMinter,
    #[msg("Invalid Message Transmitter")]
    InvalidMessageTransmitter,
    #[msg("Invalid Message Sent Event Account")]
    InvalidMessageSentEventAccount,
    #[msg("Invalid Token Pool Extra Data")]
    InvalidTokenPoolExtraData,
    #[msg("Failed CCTP CPI")]
    FailedCctpCpi,
    #[msg("Fund Manager is invalid or misconfigured")]
    InvalidFundManager,
    #[msg("Invalid destination for funds reclaim")]
    InvalidReclaimDestination,
    #[msg("Insufficient funds")]
    InsufficientFunds,
    #[msg("Invalid SOL amount")]
    InvalidSolAmount,
    #[msg("CPI target did not return data")]
    MissingCpiReturnData,
    #[msg("CPI return data has unexpected length")]
    InvalidCpiReturnDataLength,
    #[msg("CPI return data came from an unexpected program")]
    InvalidCpiReturnProgram,
    #[msg("Missing configuration of required CCTP CCV")]
    MissingConfigurationRequiredCctpCcv,
    #[msg("Unsupported BPS fee")]
    UnsupportedBpsFee,

    // ------------------------------
    // Verifier errors
    // ------------------------------
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
    #[msg("Proposed storage location admin cannot be the default pubkey")]
    DefaultStorageLocationAdminProposal,
    #[msg("Proposed storage location admin is the current storage location admin")]
    RedundantStorageLocationAdminProposal,
    #[msg("Invalid allowed sender PDA")]
    InvalidAllowedSenderPDA,
    #[msg("Source chain is not configured")]
    SourceNotConfigured,
    #[msg("Invalid verifier results version tag")]
    InvalidVersionTag,
    #[msg("Malformed verifier results")]
    MalformedVerifierResults,
    #[msg("Sender not allowed by the destination chain allowlist")]
    SenderNotAllowed,
    #[msg("Router cannot be the default (zero-value) pubkey")]
    ZeroRouter,
    #[msg("Invalid Finality")]
    InvalidFinality,
    #[msg("Account to migrate contains invalid data")]
    InvalidAccountForMigration,
    #[msg("CCTP is not enabled for this chain")]
    UnknownDomain,
    #[msg("CCTP is not enabled for this token for this chain")]
    UnsupportedTokenForDomain,
    #[msg("Missing token transfer in the message")]
    MissingTokenTransfer,
    #[msg("Nonce PDA is owned by an unexpected program")]
    InvalidNoncePDAOwner,
    #[msg("Nonce account data is invalid")]
    InvalidNonceAccountData,
    #[msg("Wrong Event PDA address")]
    WrongEventAccount,
    #[msg("Invalid transfer amount")]
    InvalidTransferAmount,
    #[msg("Invalid dest token address")]
    InvalidDestTokenAddress,
    #[msg("Invalid source token address")]
    InvalidSourceTokenAddress,
    #[msg("Invalid verifier args")]
    InvalidVerifierArgs,
    #[msg("Could not decode nested resolution inputs, or the passed-in account is neither this program's pool State nor its VerifierConfig")]
    InvalidResolutionInputs,
    #[msg("Account owned by the wrong program")]
    InvalidResolutionAccountOwner,
    #[msg("Invalid address bytes")]
    InvalidPubkeyBytes,
    #[msg("Invalid CCTP message version")]
    InvalidCctpMessageVersion,
    #[msg("Invalid CCTP message sender")]
    InvalidCctpSender,
    #[msg("Invalid CCTP message recipient")]
    InvalidCctpRecipient,
    #[msg("Invalid CCTP message hook data, it should match the version tag and message ID")]
    InvalidCctpHookData,
    #[msg("Invalid source domain in the CCTP message")]
    InvalidCctpSourceDomain,
    #[msg("Invalid destination domain in the CCTP message")]
    InvalidCctpDestinationDomain,
    #[msg("Invalid CCTP message, could not parse some field")]
    InvalidCctpMessage,
    #[msg("Invalid CCTP mint amount")]
    InvalidCctpMintAmount,
    #[msg("Executed fee is greater than the amount in the CCTP message")]
    FeeGreaterThanAmount,
    #[msg("Unexpected source pool address")]
    UnexpectedSourcePool,
    #[msg("The CCTP allowed callers must not be the zero address")]
    CctpAllowedCallersInvalidZero,
    #[msg("Fee aggregator not set")]
    FeeAggregatorNotSet,
}
