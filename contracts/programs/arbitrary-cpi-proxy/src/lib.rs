/// Arbitrary CPI Proxy for Testing
///
/// A generic test program that executes arbitrary CPIs signed with a PDA.
/// This allows tests to invoke any program that authenticates callers via PDA
/// (token pools, executor, verifier, etc.) without needing the full
/// onramp/offramp setup.
///
/// The PDA signer is derived as:
///   `PDA([pda_seed_prefix, target_program_account.key()], proxy_program_id)`
///
/// Common patterns:
///   - Pool CPI:     prefix = b"external_token_pools_signer"
///   - Receiver CPI: prefix = b"external_execution_config"
///
/// The CPI signer PDA is NOT a named account — callers must include it in
/// `remaining_accounts` at whatever position the target program expects.
///
/// This program is for testing only. It should never be deployed to mainnet.
use anchor_lang::prelude::*;
use anchor_lang::solana_program::instruction::Instruction;
use anchor_lang::solana_program::program::{get_return_data, invoke_signed};

declare_id!("4tiRp7mqGJwihhW7W32U9jx2h24grdgj67WwutyF2Yg6");

const ANCHOR_DISCRIMINATOR: usize = 8;
pub const ALLOWED_OFFRAMP: &[u8] = b"allowed_offramp";

#[program]
pub mod arbitrary_cpi_proxy {
    use super::*;

    /// Execute an arbitrary CPI, signing with a PDA derived from
    /// `[pda_seed_prefix, target_program_account.key()]`.
    ///
    /// # Arguments
    /// * `ix_data` - Raw instruction data (discriminator + serialized args) to send.
    /// * `pda_seed_prefix` - Seed prefix for PDA derivation. The target program
    ///   account key is automatically appended, and the bump is found internally.
    ///
    /// # Accounts
    /// * `target_program_account` - The program to invoke via CPI. Its key is
    ///   also used as the second PDA seed component.
    /// * `remaining_accounts` - All accounts to pass to the target program's
    ///   instruction. The caller must include the PDA signer at the position
    ///   the target program expects.
    // Test-only proxy program, not part of the deployed CCIP surface.
    pub fn execute_cpi<'info>(
        ctx: Context<'info, ExecuteCpi<'info>>,
        ix_data: Vec<u8>,
        pda_seed_prefix: Vec<u8>,
    ) -> Result<Vec<u8>> {
        let target_program = ctx.accounts.target_program_account.key();

        // Derive PDA from [prefix, target_program_key]
        let seeds: &[&[u8]] = &[pda_seed_prefix.as_slice(), target_program.as_ref()];
        let (expected_pda, bump) = Pubkey::find_program_address(seeds, ctx.program_id);

        // Build account metas from remaining_accounts, marking the PDA as signer.
        // The PDA must be present in remaining_accounts at the position the
        // target program expects it.
        let acc_metas: Vec<AccountMeta> = ctx
            .remaining_accounts
            .iter()
            .flat_map(|acc_info| {
                let is_signer = acc_info.key() == expected_pda;
                acc_info.to_account_metas(Some(is_signer))
            })
            .collect();

        let ix = Instruction {
            program_id: target_program,
            accounts: acc_metas,
            data: ix_data,
        };

        // Build account infos: remaining_accounts + target program (needed by invoke_signed)
        let mut acc_infos: Vec<AccountInfo> = ctx.remaining_accounts.to_vec();
        acc_infos.push(ctx.accounts.target_program_account.to_account_info());

        // Build signing seeds: [prefix, target_key, bump]
        let bump_bytes = [bump];
        let sign_seeds: &[&[u8]] = &[
            pda_seed_prefix.as_slice(),
            target_program.as_ref(),
            &bump_bytes,
        ];

        invoke_signed(&ix, &acc_infos, &[sign_seeds])?;

        // Only surface return data we can attribute to the program we invoked, not to a nested
        // CPI target it happened to call. Solana's get_return_data() returns data from whichever
        // program last called set_return_data() in the call tree, which for a target instruction
        // that returns `()` (and so never sets its own return data) is stale data left behind by
        // one of ITS OWN nested CPIs (e.g. lombard_token_pool's `verify` -> mailbox::handle_message)
        // - not a bug, just a target with nothing of its own to return. Treat that case the same
        // as "no return data" instead of erroring.
        match get_return_data() {
            Some((returning_program, data)) if returning_program == target_program => Ok(data),
            _ => Ok(vec![]),
        }
    }

    /// Register an offramp as allowed for a given source chain. This creates the
    /// `allowed_offramp` PDA that token pools check during `release_or_mint`.
    pub fn add_offramp(
        _ctx: Context<AddOfframp>,
        _source_chain_selector: u64,
        _offramp: Pubkey,
    ) -> Result<()> {
        msg!("Registering offramp as allowed for source chain. This is a mock of the router functionality.");
        Ok(())
    }
}

#[error_code]
pub enum CpiProxyError {
    #[msg("Return data came from a different program than the one invoked")]
    InvalidCpiReturnProgram,
}

#[derive(Accounts)]
pub struct ExecuteCpi<'info> {
    /// The program to invoke via CPI. Must be present in the transaction's
    /// account list so invoke_signed can resolve it.
    pub target_program_account: UncheckedAccount<'info>,
}

#[account]
#[derive(Copy, Debug, InitSpace)]
pub struct AllowedOfframp {}

#[derive(Accounts)]
#[instruction(source_chain_selector: u64, offramp: Pubkey)]
pub struct AddOfframp<'info> {
    #[account(
        init,
        seeds = [ALLOWED_OFFRAMP, source_chain_selector.to_le_bytes().as_ref(), offramp.as_ref()],
        bump,
        payer = authority,
        space = ANCHOR_DISCRIMINATOR + AllowedOfframp::INIT_SPACE,
    )]
    pub allowed_offramp: Account<'info, AllowedOfframp>,
    // PERMISSIONLESS: test-only mock
    #[account(mut)]
    pub authority: Signer<'info>,

    pub system_program: Program<'info, System>,
}
