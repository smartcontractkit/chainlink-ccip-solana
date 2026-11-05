use anchor_lang::prelude::*;

/// Returns the program type (name) and version.
/// Used by offchain code to easily determine which program & version is being interacted with.
pub fn type_version(_ctx: Context<Empty>) -> Result<String> {
    let response = env!("CCIP_BUILD_TYPE_VERSION").to_string();
    msg!("{}", response);
    Ok(response)
}

// Empty is a no-op Accounts context used by instructions that don't
// need any on-chain accounts (e.g. type_version, derive_accounts_*).
#[derive(Accounts)]
pub struct Empty {}
