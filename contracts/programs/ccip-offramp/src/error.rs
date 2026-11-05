use anchor_lang::error_code;

#[error_code]
pub enum CcipOfframpError {
    // offset error code so that they don't clash with other programs
    // (Anchor's base custom error code 6000 + offset 3000 = start at 9000)
    #[msg("Invalid RMN Remote Address")]
    InvalidRMNRemoteAddress = 3000,
    #[msg("The signer is unauthorized")]
    Unauthorized,
    #[msg("Account should be writable")]
    InvalidInputsMissingWritable,
    #[msg("Onramp was not configured")]
    OnrampNotConfigured,
    #[msg("Invalid version of the onchain state")]
    InvalidVersion,
    #[msg("Proposed owner is the current owner")]
    RedundantOwnerProposal,
    #[msg("Source chain selector not supported")]
    UnsupportedSourceChainSelector,
    #[msg("Invalid message dest chain selector")]
    InvalidMessageDestChainSelector,
    #[msg("Invalid pool accounts")]
    InvalidInputsPoolAccounts,
    #[msg("Invalid sysvar instructions account")]
    InvalidInputsSysvarAccount,
    #[msg("Invalid offramp authorization account")]
    InvalidInputsAllowedOfframpAccount,
    #[msg("Release or mint balance mismatch")]
    OfframpReleaseMintBalanceMismatch,
    #[msg("Invalid data length")]
    OfframpInvalidDataLength,
    #[msg("Invalid code version")]
    InvalidCodeVersion,
    #[msg("Invalid onramp address")]
    InvalidOnrampAddress,
    #[msg("Invalid offramp address")]
    InvalidOfframpAddress,
    #[msg("The buffer already contains that chunk")]
    BufferAlreadyContainsChunk,
    #[msg("The buffer is already initialized")]
    BufferAlreadyInitialized,
    #[msg("Invalid length for buffer")]
    BufferInvalidLength,
    #[msg("Chunk lies outside the buffer")]
    BufferInvalidChunkIndex,
    #[msg("Total number of chunks is not consistent")]
    BufferInvalidChunkNumber,
    #[msg("Invalid chunk size")]
    BufferInvalidChunkSize,
    #[msg("Buffer is not complete: chunks are missing")]
    BufferIncomplete,
    #[msg("Buffer is not available")]
    BufferUnavailable,
    #[msg("Token pool returned an unexpected derivation response")]
    InvalidTokenPoolAccountDerivationResponse,
    #[msg("Proposed owner is the default pubkey")]
    DefaultOwnerProposal,
    #[msg("Invalid account list lengths")]
    InvalidAccountListLengths,
    #[msg("Invalid instruction data")]
    InvalidInstructionData,
    #[msg("Invalid dest blob")]
    InvalidDestBlob,
    #[msg("Invalid receiver address")]
    InvalidReceiverAddress,
    #[msg("Invalid metadata")]
    InvalidMetadata,
    #[msg("CPI target did not return data")]
    MissingCpiReturnData,
    #[msg("CPI return data came from an unexpected program")]
    InvalidCpiReturnProgram,
    #[msg("Invalid number of CCVs in input")]
    InvalidInputsNumberOfCCVs,
    #[msg("Invalid CCV account derivation response")]
    InvalidCCVAccountDerivationResponse,
    #[msg("Invalid address bytes in input")]
    InvalidInputPubkeyBytes,
    #[msg("Invalid CCV account resolution response")]
    InvalidCCVAccountResolutionResponse,
    #[msg("Unexpected signer account in account list")]
    InvalidInputsUnexpectedSigner,
    #[msg("Too many messaging accounts")]
    TooManyMessagingAccounts,
    #[msg("Failed to deserialize buffer")]
    FailedToDeserializeBuffer,
    #[msg("Must specify at least one default or lane-mandated CCV")]
    MustSpecifyDefaultOrMandatedCCVs,
    #[msg("Zero pubkey is not allowed in CCV config")]
    ZeroPubkeyNotAllowedInCCVConfig,
    #[msg("Duplicate CCV is not allowed")]
    DuplicateCCVNotAllowed,
    #[msg("Required CCV missing from execute input")]
    RequiredCCVMissing,
    #[msg("Optional CCV quorum not reached")]
    OptionalCCVQuorumNotReached,
    #[msg("Invalid optional CCV threshold")]
    InvalidOptionalThreshold,
    #[msg("Message requested finality not allowed by the receiver")]
    RequestedFinalityNotAllowed,
    #[msg("V1 pools with account resolution (CCTP/Lombard) are not supported on the v2 lane")]
    UnsupportedPoolV1ResolutionOnLaneV2,
    #[msg("Invalid associated token account program")]
    InvalidInputAssociatedTokenAccountProgram,
    #[msg("Unsupported use of 'UseAsIs' token receiver usage for pool v1")]
    UnsupportedTokenReceiverUseAsIsForPoolV1,
    #[msg("Invalid external execution signer account for the receiver CPI")]
    InvalidExternalExecutionSigner,
    #[msg("Failed to serialize the receiver message")]
    InvalidMessage,
    #[msg("Receiver CPI accounts do not match the signed message dest blob")]
    InvalidReceiverAccounts,
    #[msg("Duplicate onramp is not allowed")]
    DuplicateOnRampNotAllowed,
    #[msg("Too many onramps configured for the source chain")]
    TooManyOnRamps,
    #[msg("The buffer authority is not the caller")]
    WrongBufferAuthority,
    #[msg("There is a redundant buffer account as the message is in the ix data already")]
    RedundantBuffer,
    #[msg("The source chain account does not match the message source chain selector")]
    InvalidMessageSourceChainSelector,
    #[msg("Receiver registry account missing for arbitrary message")]
    ReceiverRegistryAccountMissing,
}
