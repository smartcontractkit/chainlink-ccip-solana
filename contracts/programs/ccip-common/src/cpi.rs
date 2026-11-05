use anchor_lang::prelude::*;
use anchor_lang::solana_program::program::get_return_data;

/// Reads CPI return data, verifies it originated from `expected_program`, and returns the raw
/// bytes, reporting `err` on any verification failure.
///
/// Solana's `get_return_data()` returns the data set by whichever program last called
/// `set_return_data()` anywhere in the current CPI call tree. So if the program we invoked
/// internally CPIs another program that also sets return data, a naive read would silently return
/// that other program's output. Verifying the returning program matches the one we directly invoked
/// closes that gap. Mirrors Anchor's own CPI return handling:
/// https://github.com/coral-xyz/anchor/blob/0109f4a3cf4117570f627e3ae465b6247d504f69/lang/syn/src/codegen/program/cpi.rs#L83
///
/// The error value is caller-supplied so each program keeps its own error codes (see
/// INV-SOL-CPI-RETURN). Every CCIP CPI that consumes return data MUST go through this helper or
/// one of its wrappers.
pub fn get_verified_cpi_return_data<E>(expected_program: Pubkey, err: E) -> Result<Vec<u8>>
where
    E: Into<anchor_lang::error::Error> + Clone,
{
    let (returning_program, data) = get_return_data().ok_or_else(|| err.clone().into())?;
    // `require_keys_eq!` needs a concrete error type, so we compare manually; `with_pubkeys`
    // keeps the same compared-values log output the macro produces.
    if returning_program != expected_program {
        return Err(err
            .into()
            .with_pubkeys((returning_program, expected_program)));
    }
    Ok(data)
}

/// [`get_verified_cpi_return_data`] plus borsh deserialization into `T`, with separate errors
/// for verification failure and malformed return data.
pub fn parse_verified_cpi_return_as<T, E>(
    expected_program: Pubkey,
    verify_err: E,
    decode_err: E,
) -> Result<T>
where
    T: AnchorDeserialize,
    E: Into<anchor_lang::error::Error> + Clone,
{
    let data = get_verified_cpi_return_data(expected_program, verify_err)?;
    T::try_from_slice(&data).map_err(|_| decode_err.into())
}

/// [`parse_verified_cpi_return_as`] reporting all failures as the same error.
pub fn parse_verified_cpi_return_or<T, E>(expected_program: Pubkey, err: E) -> Result<T>
where
    T: AnchorDeserialize,
    E: Into<anchor_lang::error::Error> + Clone,
{
    parse_verified_cpi_return_as(expected_program, err.clone(), err)
}
