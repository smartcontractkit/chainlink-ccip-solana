use anchor_lang::prelude::*;

#[error_code]
pub enum MockLombardMailboxError {
    #[msg("message has already been handled")]
    AlreadyHandled,
    #[msg("not enough remaining_accounts to forward to mock-lombard-bridge's mock_gmp_receive")]
    MissingRemainingAccounts,
    #[msg("handler does not match the message's destination caller")]
    InvalidDestinationCaller,
}
