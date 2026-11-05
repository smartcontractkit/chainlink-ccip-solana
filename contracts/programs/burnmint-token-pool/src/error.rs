use anchor_lang::prelude::*;

#[error_code]
pub enum CcipBnMTokenPoolError {
    #[msg("Invalid Multisig Mint")]
    InvalidMultisig,
    #[msg("Mint Authority already set")]
    MintAuthorityAlreadySet,
    #[msg("Token with no Mint Authority")]
    FixedMintToken,
    #[msg("Unsupported Token Program")]
    UnsupportedTokenProgram,
    #[msg("Invalid Multisig Account Data for Token 2022")]
    InvalidToken2022Multisig,
    #[msg("Invalid Multisig Account Data for SPL Token")]
    InvalidSPLTokenMultisig,
    #[msg("Token Pool Signer PDA must be m times a signer of the Multisig")]
    PoolSignerNotInMultisig,
    #[msg("Multisig must have at least 2 valid signers")]
    MultisigMustHaveAtLeastTwoSigners,
    #[msg("Multisig must have more than one required signer")]
    MultisigMustHaveMoreThanOneSigner,
    #[msg("Multisig Owner must match Token Program ID")]
    InvalidMultisigOwner,
    #[msg("Invalid multisig threshold: required signatures cannot exceed total signers")]
    InvalidMultisigThreshold,
    #[msg(
        "Invalid multisig m: required signatures cannot exceed the available for outside signers"
    )]
    InvalidMultisigThresholdTooHigh,
}
