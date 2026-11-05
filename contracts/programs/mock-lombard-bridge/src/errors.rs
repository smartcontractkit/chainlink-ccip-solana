use anchor_lang::prelude::*;

#[error_code]
pub enum MockLombardBridgeError {
    #[msg("recipient_token_account owner does not match the recipient argument")]
    RecipientMismatch,
}
