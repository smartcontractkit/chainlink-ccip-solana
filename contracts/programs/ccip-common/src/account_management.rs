use anchor_lang::prelude::*;
use anchor_lang::system_program;

// This module cannot be called `accounts` as it clashes with names that Anchor generates...

// Anchor has this same `close` method but doesn't make it public for some reason, so we reimplement it here,
// see https://github.com/otter-sec/anchor/blob/v1.1.2/lang/src/common.rs#L7-L17, just adding a basic debug assert
pub fn close<'info>(info: &AccountInfo<'info>, sol_destination: &AccountInfo<'info>) -> Result<()> {
    debug_assert_ne!(info.key(), sol_destination.key()); // just to catch dev errors, this is not a security check

    // Transfer lamports from the account to the sol_destination.
    sol_destination.add_lamports(info.lamports())?;
    **info.lamports.borrow_mut() = 0;

    info.assign(&system_program::ID);
    info.resize(0).map_err(Into::into)
}

pub fn initialize_from_authority<'info>(
    account_info: &AccountInfo<'info>,
    system_program: &AccountInfo<'info>,
    authority: &AccountInfo<'info>,
    required_space: usize,
    signer_seeds: &[&[&[u8]]],
    owner: &Pubkey,
) -> Result<()> {
    // paid by the authority directly, as it was already a signing account of the overall
    // ix that called this function
    let transfer_ctx = CpiContext::new(
        system_program.key(),
        system_program::Transfer {
            from: authority.to_account_info(),
            to: account_info.clone(),
        },
    );

    initialize(
        account_info,
        system_program,
        authority,
        required_space,
        signer_seeds,
        owner,
        transfer_ctx,
    )
}

pub fn initialize_from_pda<'info>(
    account_info: &AccountInfo<'info>,
    system_program: &AccountInfo<'info>,
    authority: &AccountInfo<'info>,
    required_space: usize,
    signer_seeds: &[&[&[u8]]],
    owner: &Pubkey,
) -> Result<()> {
    // using an authority that wasn't received as a signer account_info - assumed to be the
    // PDA whose seeds were passed in, and it must hold a balance to pay for this.
    let transfer_ctx = CpiContext::new_with_signer(
        system_program.key(),
        system_program::Transfer {
            from: authority.to_account_info(),
            to: account_info.clone(),
        },
        signer_seeds,
    );

    initialize(
        account_info,
        system_program,
        authority,
        required_space,
        signer_seeds,
        owner,
        transfer_ctx,
    )
}

// Based on Anchor's `init` function, but with some customizations to allow a PDA owned by CCIP
// to pay for the new account being initialized, which requires signing seeds in some additional cases.
fn initialize<'info>(
    account_info: &AccountInfo<'info>,
    system_program: &AccountInfo<'info>,
    authority: &AccountInfo<'info>,
    required_space: usize,
    signer_seeds: &[&[&[u8]]],
    owner: &Pubkey,
    transfer_ctx: CpiContext<'_, '_, '_, 'info, system_program::Transfer<'info>>,
) -> Result<()> {
    let minimum_balance = Rent::get()?.minimum_balance(required_space);
    let current_lamports = account_info.lamports();

    if current_lamports == 0 {
        // Common path: account has no lamports, use create_account (single CPI).
        let cpi_ctx = CpiContext::new_with_signer(
            system_program.key(),
            system_program::CreateAccount {
                from: authority.to_account_info(),
                to: account_info.clone(),
            },
            signer_seeds,
        );
        system_program::create_account(cpi_ctx, minimum_balance, required_space as u64, owner)?;
    } else {
        // Pre-funded path: account already has lamports (e.g. from a transfer
        // before initialization). Use allocate + assign, and only transfer the
        // delta needed for rent exemption — repurposing existing funds.
        let required_lamports = minimum_balance.saturating_sub(current_lamports);
        if required_lamports > 0 {
            system_program::transfer(transfer_ctx, required_lamports)?;
        }
        system_program::allocate(
            CpiContext::new_with_signer(
                system_program.key(),
                system_program::Allocate {
                    account_to_allocate: account_info.clone(),
                },
                signer_seeds,
            ),
            required_space as u64,
        )?;
        system_program::assign(
            CpiContext::new_with_signer(
                system_program.key(),
                system_program::Assign {
                    account_to_assign: account_info.clone(),
                },
                signer_seeds,
            ),
            owner,
        )?;
    };

    Ok(())
}

pub fn resize<'info>(
    account_info: &AccountInfo<'info>,
    target_space: usize,
    authority: &AccountInfo<'info>,
) -> Result<()> {
    let required_lamports = Rent::get()?
        .minimum_balance(target_space)
        .saturating_sub(account_info.lamports());
    if required_lamports > 0 {
        system_program::transfer(
            CpiContext::new(
                system_program::ID,
                system_program::Transfer {
                    from: authority.to_account_info(),
                    to: account_info.clone(),
                },
            ),
            required_lamports,
        )?;
    }
    account_info.resize(target_space)?;
    Ok(())
}
