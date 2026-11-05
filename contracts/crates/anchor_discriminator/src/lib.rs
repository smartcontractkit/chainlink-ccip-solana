use proc_macro::TokenStream;
use quote::quote;
use sha2::{Digest, Sha256};
use syn::{parse_macro_input, LitStr};

fn discriminator_bytes(namespace: &str, name: &str) -> [u8; 8] {
    let preimage = format!("{namespace}:{name}");
    let hash = Sha256::digest(preimage.as_bytes());

    let mut bytes = [0u8; 8];
    bytes.copy_from_slice(&hash[..8]);
    bytes
}

#[proc_macro]
pub fn anchor_discriminator(input: TokenStream) -> TokenStream {
    let instruction_name = parse_macro_input!(input as LitStr);
    let bytes = discriminator_bytes("global", &instruction_name.value());

    quote! {
        [#(#bytes),*]
    }
    .into()
}

#[proc_macro]
pub fn anchor_account_discriminator(input: TokenStream) -> TokenStream {
    let instruction_name = parse_macro_input!(input as LitStr);
    let bytes = discriminator_bytes("account", &instruction_name.value());

    quote! {
        [#(#bytes),*]
    }
    .into()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn computes_get_required_ccvs_discriminator() {
        assert_eq!(
            discriminator_bytes("global", "get_required_ccvs"),
            [0xe9, 0x4e, 0xcb, 0x8e, 0x0c, 0x71, 0x60, 0x63]
        );
    }

    #[test]
    fn computes_release_or_mint_tokens_discriminator() {
        assert_eq!(
            discriminator_bytes("global", "release_or_mint_tokens"),
            [0x5c, 0x64, 0x96, 0xc6, 0xfc, 0x3f, 0xa4, 0xe4]
        );
    }

    #[test]
    fn computes_lock_or_burn_tokens_discriminator() {
        assert_eq!(
            discriminator_bytes("global", "lock_or_burn_tokens"),
            [0x72, 0xa1, 0x5e, 0x1d, 0x93, 0x19, 0xe8, 0xbf]
        );
    }

    #[test]
    fn computes_lock_or_burn_tokens_v2_discriminator() {
        assert_eq!(
            discriminator_bytes("global", "lock_or_burn_tokens_v2"),
            [0x59, 0xc8, 0x67, 0x35, 0x0e, 0xf9, 0xc4, 0xc5]
        );
    }

    #[test]
    fn computes_release_or_mint_tokens_v2_discriminator() {
        assert_eq!(
            discriminator_bytes("global", "release_or_mint_tokens_v2"),
            [0x95, 0xd8, 0xb2, 0xe3, 0x59, 0xd5, 0x69, 0xba]
        );
    }

    #[test]
    fn computes_ccip_receive_discriminator() {
        assert_eq!(
            discriminator_bytes("global", "ccip_receive"),
            [0x0b, 0xf4, 0x09, 0xf9, 0x2c, 0x53, 0x2f, 0xf5]
        );
    }

    #[test]
    fn computes_tokenpool_release_or_mint_discriminator() {
        assert_eq!(
            discriminator_bytes("global", "release_or_mint_tokens"),
            [0x5c, 0x64, 0x96, 0xc6, 0xfc, 0x3f, 0xa4, 0xe4]
        );
    }

    #[test]
    fn computes_tokenpool_derive_release_or_mint_discriminator() {
        assert_eq!(
            discriminator_bytes("global", "derive_accounts_release_or_mint_tokens"),
            [0x28, 0x5b, 0xf4, 0xe4, 0x00, 0x02, 0x33, 0xee]
        );
    }

    #[test]
    fn computes_tokenpool_derive_lock_or_burn_discriminator() {
        assert_eq!(
            discriminator_bytes("global", "derive_accounts_lock_or_burn_tokens"),
            [20, 237, 184, 4, 166, 153, 108, 174]
        );
    }

    #[test]
    fn computes_receive_message_discriminator() {
        assert_eq!(
            discriminator_bytes("global", "receive_message"),
            [38, 144, 127, 225, 31, 225, 238, 25]
        );
    }

    #[test]
    fn computes_deposit_for_burn_with_caller_discriminator() {
        assert_eq!(
            discriminator_bytes("global", "deposit_for_burn_with_caller"),
            [167, 222, 19, 114, 85, 21, 14, 118]
        );
    }

    #[test]
    fn computes_reclaim_event_account_discriminator() {
        assert_eq!(
            discriminator_bytes("global", "reclaim_event_account"),
            [94, 198, 180, 159, 131, 236, 15, 174]
        );
    }

    #[test]
    fn computes_ccip_send_discriminator() {
        assert_eq!(
            discriminator_bytes("global", "ccip_send"),
            [108, 216, 134, 191, 249, 234, 33, 84]
        );
    }

    #[test]
    fn computes_get_fee_discriminator() {
        assert_eq!(
            discriminator_bytes("global", "get_fee"),
            [115, 195, 235, 161, 25, 219, 60, 29]
        );
    }
}
