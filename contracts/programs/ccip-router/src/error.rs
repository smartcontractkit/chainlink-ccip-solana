use anchor_lang::error_code;

#[error_code]
pub enum CcipRouterError {
    #[msg("The signer is unauthorized")]
    // offset error code so that they don't clash with other programs
    // (Anchor's base custom error code 6000 + offset 1000 = start at 7000)
    Unauthorized = 1000,
    #[msg("Invalid RMN Remote Address")]
    InvalidRMNRemoteAddress,
    #[msg("Mint account input is invalid")]
    InvalidInputsMint,
    #[msg("Invalid version of the onchain state")]
    InvalidVersion,
    #[msg("Fee token doesn't match transfer token")]
    FeeTokenMismatch,
    #[msg("Proposed owner is the current owner")]
    RedundantOwnerProposal,
    #[msg("Reached max message number")]
    ReachedMaxMessageNumber,
    #[msg("Invalid pool account account indices")]
    InvalidInputsTokenIndices,
    #[msg("Invalid pool accounts")]
    InvalidInputsPoolAccounts,
    #[msg("Invalid token accounts")]
    InvalidInputsTokenAccounts,
    #[msg("Invalid Token Admin Registry account")]
    InvalidInputsTokenAdminRegistryAccounts,
    #[msg("Invalid LookupTable account")]
    InvalidInputsLookupTableAccounts,
    #[msg("Invalid LookupTable account writable access")]
    InvalidInputsLookupTableAccountWritable,
    #[msg("Cannot send zero tokens")]
    InvalidInputsTokenAmount,
    #[msg("Must specify zero amount to send alongside transfer_all")]
    InvalidInputsTransferAllAmount,
    #[msg("Invalid Associated Token Account address")]
    InvalidInputsAtaAddress,
    #[msg("Invalid Associated Token Account writable flag")]
    InvalidInputsAtaWritable,
    #[msg("Chain selector is invalid")]
    InvalidInputsChainSelector,
    #[msg("Insufficient lamports")]
    InsufficientLamports,
    #[msg("Insufficient funds")]
    InsufficientFunds,
    #[msg("Source token data is too large")]
    SourceTokenDataTooLarge,
    #[msg("New Admin can not be zero address")]
    InvalidTokenAdminRegistryInputsZeroAddress,
    #[msg("An already owned registry can not be proposed")]
    InvalidTokenAdminRegistryProposedAdmin,
    #[msg("Sender not allowed for that destination chain")]
    SenderNotAllowed,
    #[msg("Invalid code version")]
    InvalidCodeVersion,
    #[msg("Invalid rollback attempt on the CCIP version of the onramp to the destination chain")]
    InvalidCcipVersionRollback,
    #[msg("Invalid account list for PDA derivation")]
    InvalidAccountListForPdaDerivation,
    #[msg("Unexpected account derivation stage")]
    InvalidDerivationStage,
    #[msg("Invalid version of the Nonce account")]
    InvalidNonceVersion,
    #[msg("Token pool returned an unexpected derivation response")]
    InvalidTokenPoolAccountDerivationResponse,
    #[msg("Can't fit account derivation response.")]
    AccountDerivationResponseTooLarge,
    #[msg("Proposed owner is the default pubkey")]
    DefaultOwnerProposal,
    #[msg("Invalid account list lengths")]
    InvalidAccountListLengths,
    #[msg("Invalid instruction data")]
    InvalidInstructionData,
    #[msg("Invalid FeeQuoter address")]
    InvalidFeeQuoterAddress,
    #[msg("Invalid CCV return data")]
    InvalidCCVReturnData,
    #[msg("Invalid CCV account resolution response")]
    InvalidCCVAccountResolutionResponse,
    #[msg("CPI target did not return data")]
    MissingCpiReturnData,
    #[msg("CPI return data came from an unexpected program")]
    InvalidCpiReturnProgram,
    #[msg("Invalid amount of token transfers")]
    InvalidInputsMultipleTokenTransfers,
    #[msg("Invalid address bytes in input")]
    InvalidInputPubkeyBytes,
    #[msg("V1 pools with account resolution (CCTP/Lombard) are not supported on the v2 lane")]
    UnsupportedPoolV1ResolutionOnLaneV2,
    #[msg("Account resolution requires pool interface version >= 2")]
    AccountResolutionRequiresV2,
    #[msg("Duplicate CCV in user-provided extra args")]
    DuplicateCCVInExtraArgs,
    #[msg("Invalid CCV accounts")]
    InvalidInputsCcvAccounts,
    #[msg("Invalid receipt data")]
    InvalidReceiptData,
    #[msg("Invalid finality value")]
    InvalidFinality,
    #[msg("Account to migrate contains invalid data")]
    InvalidAccountForMigration,
    #[msg("Must specify at least one default or lane-mandated CCV")]
    MustSpecifyDefaultOrMandatedCcvs,
    #[msg("CCV address cannot be the zero/default pubkey")]
    CcvZeroAddressNotAllowed,
    #[msg("Duplicate CCV not allowed in default/lane-mandated lists")]
    DuplicateCcvNotAllowed,
    #[msg("Invalid token price")]
    InvalidTokenPrice,
    #[msg("Destination chain config contains an empty offramp address")]
    MissingOfframpAddress,
    #[msg("Invalid destination address length")]
    InvalidInputsAddressBytesLength,
    #[msg("Invalid destination receiver address")]
    InvalidReceiverAddress,
    #[msg("Invalid destination token address")]
    InvalidDestTokenAddress,
    #[msg("Invalid destination token receiver address")]
    InvalidDestTokenReceiverAddress,
    #[msg("Message is configured to opt out of automated execution")]
    NoExecMode,
    #[msg("Missing associated token account for receipt issuer")]
    MissingAtaForReceiptIssuer,
    #[msg("Fee token is unsupported")]
    UnsupportedFeeToken,
    #[msg("Token arithmetic resulted in an invalid value")]
    InvalidTokenArithmetic,
    #[msg("Fee for this message exceeds the maximum allowed")]
    FeeExceedsMaxAllowed,
    #[msg("The same issuer appears in more than one receipt")]
    DuplicateReceiptIssuer,
    #[msg("Faster-than-finality is not supported on a v1 token pool")]
    FinalityNotSupportedOnPoolV1,
    #[msg("Token args are not supported on a v1 token pool")]
    TokenArgsNotSupportedOnPoolV1,
    #[msg("Default executor is invalid")]
    InvalidExecutor,
    #[msg("Token Receiver not allowed for current destination chain")]
    TokenReceiverNotAllowed,
    #[msg("Token uses a multisig as mint authority - use the multisig-specific method instead")]
    TokenOwnedByMultisig,
}
