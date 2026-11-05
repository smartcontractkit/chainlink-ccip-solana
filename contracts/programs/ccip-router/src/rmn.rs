use anchor_lang::prelude::*;
use rmn_remote::state::CurseSubject;

pub fn verify_uncursed_cpi<'info>(
    rmn_remote: &UncheckedAccount<'info>,
    rmn_remote_curses: &UncheckedAccount<'info>,
    rmn_remote_config: &UncheckedAccount<'info>,
    dest_chain_selector: u64,
) -> Result<()> {
    let cpi_program = rmn_remote.key();
    let cpi_accounts = rmn_remote::cpi::accounts::InspectCurses {
        config: rmn_remote_config.to_account_info(),
        curses: rmn_remote_curses.to_account_info(),
    };
    let cpi_context = CpiContext::new(cpi_program, cpi_accounts);
    rmn_remote::cpi::verify_not_cursed(
        cpi_context,
        CurseSubject::from_chain_selector(dest_chain_selector),
    )
}
