/**
 * This program is an example of an Invalid CCIP Receiver Program.
 * Used to test CCIP Router execute and check that it fails.
 *
 * The CPI proxy functionality (pool_proxy_release_or_mint, pool_proxy_lock_or_burn,
 * receiver_proxy_execute) has been moved to the generic arbitrary-cpi-proxy program.
 */
use anchor_lang::prelude::*;
use anchor_lang::pubkey;
use example_ccip_receiver::Any2SVMMessage;

declare_id!("MhPhxwRHtZt9rkHEEFJ9eFjEna8fwbMHr8KYByvJkUm");

#[program]
pub mod test_ccip_invalid_receiver {
    use super::*;

    // Actual invalid receiver method
    pub fn ccip_receive(ctx: Context<Initialize>, _message: Any2SVMMessage) -> Result<()> {
        msg!("Not reachable due to uninitialized accounts");

        let counter = &mut ctx.accounts.counter;
        counter.value = 1;

        Ok(())
    }

    ///////////////////////////////////////
    // Mocks of router, onramp & offramp //
    ///////////////////////////////////////
    // Mock of the router's own handler, mirroring its signature on purpose.
    pub fn add_offramp(
        _ctx: Context<AddOfframp>,
        _source_chain_selector: u64,
        _offramp: Pubkey,
    ) -> Result<()> {
        msg!("Registering offramp as allowed for source chain. This is a mock of the router functionality.");
        Ok(())
    }
}

const ANCHOR_DISCRIMINATOR: usize = 8;

const TEST_ROUTER: Pubkey = pubkey!("Ccip842gzYHhvdDkSyi2YVCoAWPbYJoApMFzSxQroE9C");

#[derive(Accounts, Debug)]
#[instruction(message: Any2SVMMessage)]
pub struct Initialize<'info> {
    // router CPI signer must be first
    // PERMISSIONLESS: test program
    #[account(mut)]
    pub authority: Signer<'info>,

    /// CHECK offramp program: exists only to derive the allowed offramp PDA
    /// and the authority PDA. Must be second.
    pub offramp_program: UncheckedAccount<'info>,

    // PDA to verify that calling offramp is valid. Must be third. It is left up to the implementer to decide
    // how they want to persist the router address to verify that this is the correct account (e.g. in the top level of
    // a global config/state account for the receiver, which is what this example does, or hard-coded,
    // or stored in any other way in any other account).
    /// CHECK PDA of the router program verifying the signer is an allowed offramp.
    /// If PDA does not exist, the router doesn't allow this offramp
    #[account(
        owner = TEST_ROUTER, // this guarantees that it was initialized
        seeds = [
            ALLOWED_OFFRAMP,
            message.source_chain_selector.to_le_bytes().as_ref(),
            offramp_program.key().as_ref()
        ],
        bump,
        seeds::program = TEST_ROUTER,
    )]
    pub allowed_offramp: UncheckedAccount<'info>,

    #[account(
        init,
        seeds = [b"counter"],
        bump,
        payer = authority,
        space = ANCHOR_DISCRIMINATOR + Counter::INIT_SPACE,
    )]
    pub counter: Account<'info, Counter>,

    pub system_program: Program<'info, System>,
}

pub const ALLOWED_OFFRAMP: &[u8] = b"allowed_offramp";

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

#[account]
#[derive(InitSpace, Debug)]
pub struct Counter {
    pub value: u8,
}
