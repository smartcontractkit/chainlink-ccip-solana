use anchor_lang::prelude::*;
use solana_keccak_hasher::{hash, hashv};
use solana_secp256k1_recover::secp256k1_recover;

use ccip_common::messages::MessageV1;
use ccip_common::seed;

use crate::instructions::VERSION_TAG;
use crate::state::{valid_version, MAX_CONFIG_V};
use crate::state::{Config, RemoteChainConfig};
use crate::CommitteeVerifierError;

const VERSION_TAG_LEN: usize = 4;
const SIG_LENGTH_LEN: usize = 2;
const HEADER_LEN: usize = VERSION_TAG_LEN + SIG_LENGTH_LEN;
const SIGNATURE_LEN: usize = 64;

#[derive(AnchorSerialize, AnchorDeserialize, Debug)]
pub struct VerifyParams {
    pub message_id: [u8; 32],
    pub encoded_message: Vec<u8>,
    pub verifier_results: Vec<u8>,
}

pub fn verify(ctx: Context<Verify>, params: VerifyParams) -> Result<()> {
    // 1. Parse encoded_message to get source_chain_selector and validate the PDA
    let message = MessageV1::from_bytes(&params.encoded_message)
        .map_err(|_| CommitteeVerifierError::MalformedVerifierResults)?;

    let expected_pda = Pubkey::create_program_address(
        &[
            seed::REMOTE_CHAIN,
            &message.source_chain_selector.to_le_bytes(),
            &[ctx.accounts.remote_chain_config.bump],
        ],
        &crate::ID,
    )
    .map_err(|_| CommitteeVerifierError::InvalidInstructionData)?;

    require_keys_eq!(
        ctx.accounts.remote_chain_config.key(),
        expected_pda,
        CommitteeVerifierError::InvalidInstructionData
    );

    let signer_config = &ctx.accounts.remote_chain_config.signer_config;

    let Some(threshold) = signer_config.threshold() else {
        return Err(CommitteeVerifierError::SourceNotConfigured.into());
    };

    // 3. Parse verifier_results wire format
    let verifier_results = DecodedVerifierResults::new(&params.verifier_results)?;

    require_gte!(
        verifier_results.signatures.len(),
        threshold as usize,
        CommitteeVerifierError::InsufficientSignatures
    );

    // 4. Compute signed hash and verify signatures.
    let signed_hash = compute_signed_hash(&verifier_results.version_tag, &params.message_id);

    verify_signatures(
        &signed_hash,
        &verifier_results.signatures[..threshold as usize],
        signer_config.signers(),
    )
}

fn verify_signatures(
    signed_hash: &[u8; 32],
    signatures: &[&[u8]],
    authorized_signers: &[[u8; 20]],
) -> Result<()> {
    let mut previous_signer: [u8; 20] = [0u8; 20];

    for sig_bytes in signatures {
        // recovery_id = 0: EVM normalizes all signatures to V=27 via ECDSA malleability
        let recovered = secp256k1_recover(signed_hash, 0, sig_bytes)
            .map_err(|_| CommitteeVerifierError::InvalidSignature)?;

        // Pubkey -> 20 byte ETH address
        let ethereum_address: [u8; 20] = hash(&recovered.0).to_bytes()[12..32]
            .try_into()
            .map_err(|_| CommitteeVerifierError::InvalidSignature)?;

        // Strictly ascending order: enforces uniqueness + rejects zero address
        require!(
            ethereum_address > previous_signer,
            CommitteeVerifierError::SignaturesNotSorted
        );
        previous_signer = ethereum_address;

        // Check signer is in the authorized set (sorted for binary search)
        authorized_signers
            .binary_search(&ethereum_address)
            .map_err(|_| CommitteeVerifierError::UnauthorizedSigner)?;
    }

    Ok(())
}

// TODO: the version tag should be unique per chain, this can be achieved if we append the program ID to the version_tag. This is a protocol change, impacts more than just Solana.
fn compute_signed_hash(version_tag: &[u8; 4], message_id: &[u8; 32]) -> [u8; 32] {
    hashv(&[version_tag, message_id]).to_bytes()
}

#[derive(Accounts)]
pub struct Verify<'info> {
    #[account(
        seeds = [seed::CONFIG],
        bump = config.bump,
        constraint = valid_version(config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub config: Account<'info, Config>,

    // NOTE: The remote_chain_config PDA is not validated via a seeds constraint
    // here because deriving the seeds requires the source_chain_selector, which
    // is only available after decoding the message. The processor validates the
    // PDA address manually to avoid decoding the message twice.
    #[account(
        constraint = valid_version(remote_chain_config.version, MAX_CONFIG_V) @ CommitteeVerifierError::InvalidVersion,
    )]
    pub remote_chain_config: Account<'info, RemoteChainConfig>,
}

#[derive(Debug)]
struct DecodedVerifierResults<'a> {
    pub version_tag: [u8; 4],
    pub signatures: Vec<&'a [u8]>,
}

impl<'a> DecodedVerifierResults<'a> {
    pub fn new(bytes: &'a [u8]) -> Result<Self> {
        require_gte!(
            bytes.len(),
            HEADER_LEN,
            CommitteeVerifierError::MalformedVerifierResults
        );

        let version_tag: [u8; 4] = bytes[..VERSION_TAG_LEN]
            .try_into()
            .expect("bytes.len() >= HEADER_LEN verified above; VERSION_TAG_LEN (4) <= HEADER_LEN");
        require!(
            version_tag == VERSION_TAG,
            CommitteeVerifierError::InvalidVersionTag
        );

        let sig_byte_len = u16::from_be_bytes(
            bytes[VERSION_TAG_LEN..HEADER_LEN]
                .try_into()
                .expect("HEADER_LEN - VERSION_TAG_LEN = 2 bytes, matching [u8; 2] for u16"),
        ) as usize;

        require!(
            sig_byte_len.is_multiple_of(SIGNATURE_LEN),
            CommitteeVerifierError::MalformedVerifierResults
        );
        require_gte!(
            bytes.len(),
            HEADER_LEN + sig_byte_len,
            CommitteeVerifierError::MalformedVerifierResults
        );

        let signatures = bytes[HEADER_LEN..HEADER_LEN + sig_byte_len]
            .chunks_exact(SIGNATURE_LEN)
            .collect();

        Ok(Self {
            version_tag,
            signatures,
        })
    }
}

#[cfg(test)]
mod tests {
    use solana_keccak_hasher::hash;

    use super::*;

    fn make_verifier_results(version_tag: [u8; 4], signatures: &[&[u8; 64]]) -> Vec<u8> {
        let sig_byte_len = (signatures.len() * SIGNATURE_LEN) as u16;
        let mut buf = Vec::with_capacity(HEADER_LEN + sig_byte_len as usize);
        buf.extend_from_slice(&version_tag);
        buf.extend_from_slice(&sig_byte_len.to_be_bytes());
        for sig in signatures {
            buf.extend_from_slice(sig.as_slice());
        }
        buf
    }

    fn eth_address(verifying_key: &k256::ecdsa::VerifyingKey) -> [u8; 20] {
        let point = verifying_key.to_encoded_point(false);
        // Uncompressed point is 65 bytes; skip the 0x04 prefix for keccak
        hash(&point.as_bytes()[1..65]).to_bytes()[12..32]
            .try_into()
            .unwrap()
    }

    // Deterministic low-s signature (k256 normalizes s into the lower half). The on-chain verifier
    // recovers with recovery_id = 0 and the host `secp256k1_recover` rejects high-s, so a signature
    // is only usable when it *naturally* recovers with recovery_id 0 — `sorted_signers` filters to
    // those keys. (Forcing a V=28 key to V=27 needs s -> n-s, i.e. high-s, which gets rejected.)
    fn sign_normalized(signing_key: &k256::ecdsa::SigningKey, hash: &[u8; 32]) -> [u8; 64] {
        use k256::ecdsa::signature::hazmat::PrehashSigner;
        let sig: k256::ecdsa::Signature = signing_key.sign_prehash(hash).unwrap();
        sig.to_bytes().into()
    }

    // The eth address `verify_signatures` would recover for `sig` over `signed_hash` (recovery_id 0),
    // or None if recovery fails (e.g. high-s).
    fn recovered_id0_address(signed_hash: &[u8; 32], sig: &[u8; 64]) -> Option<[u8; 20]> {
        let recovered = secp256k1_recover(signed_hash, 0, sig).ok()?;
        hash(&recovered.0).to_bytes()[12..32].try_into().ok()
    }

    struct TestSigner {
        secret: k256::ecdsa::SigningKey,
        address: [u8; 20],
    }

    // Deterministically returns `n` distinct signers whose signature over `signed_hash` recovers with
    // recovery_id 0, sorted ascending by eth address. Keys that would recover with odd parity (V=28)
    // are skipped — they cannot be represented as the low-s / V=27 signatures the on-chain verifier
    // requires. The candidate pool is bounded; we assert enough id=0 signers were found.
    fn sorted_signers(signed_hash: &[u8; 32], n: usize) -> Vec<TestSigner> {
        let mut seed = [0u8; 32];
        let mut signers: Vec<TestSigner> = (1u32..=256)
            .filter_map(|i| {
                seed[..4].copy_from_slice(&i.to_be_bytes());
                let secret = k256::ecdsa::SigningKey::from_bytes((&seed).into()).unwrap();
                let address = eth_address(secret.verifying_key());
                let sig = sign_normalized(&secret, signed_hash);
                (recovered_id0_address(signed_hash, &sig) == Some(address))
                    .then_some(TestSigner { secret, address })
            })
            .collect();

        signers.sort_by(|a, b| a.address.cmp(&b.address));
        assert!(
            signers.len() >= n,
            "not enough recovery_id=0 signers in pool (have {}, need {})",
            signers.len(),
            n
        );
        signers.truncate(n);
        signers
    }

    // ── DecodedVerifierResults tests ─────────────────────────────────

    #[test]
    fn parse_valid() {
        let sig_a = [0xAAu8; 64];
        let sig_b = [0xBBu8; 64];
        let buf = make_verifier_results(VERSION_TAG, &[&sig_a, &sig_b]);

        let vr = DecodedVerifierResults::new(&buf).unwrap();
        assert_eq!(vr.version_tag, VERSION_TAG);
        assert_eq!(vr.signatures.len(), 2);
        assert_eq!(vr.signatures[0], &sig_a[..]);
        assert_eq!(vr.signatures[1], &sig_b[..]);
    }

    #[test]
    fn parse_zero_signatures() {
        let buf = make_verifier_results(VERSION_TAG, &[]);
        let vr = DecodedVerifierResults::new(&buf).unwrap();
        assert_eq!(vr.signatures.len(), 0);
    }

    #[test]
    fn parse_wrong_version_tag() {
        let buf = make_verifier_results([0x00, 0x00, 0x00, 0x00], &[]);
        let err = DecodedVerifierResults::new(&buf).unwrap_err();
        assert_eq!(err, CommitteeVerifierError::InvalidVersionTag.into());
    }

    #[test]
    fn parse_too_short() {
        let buf = vec![0xe9, 0xa0, 0x5a]; // 3 bytes, less than HEADER_LEN
        let err = DecodedVerifierResults::new(&buf).unwrap_err();
        assert_eq!(err, CommitteeVerifierError::MalformedVerifierResults.into());
    }

    #[test]
    fn parse_sig_len_not_multiple_of_64() {
        let mut buf = VERSION_TAG.to_vec();
        buf.extend_from_slice(&65u16.to_be_bytes());
        buf.extend_from_slice(&[0u8; 65]);
        let err = DecodedVerifierResults::new(&buf).unwrap_err();
        assert_eq!(err, CommitteeVerifierError::MalformedVerifierResults.into());
    }

    #[test]
    fn parse_truncated_signature_data() {
        let mut buf = VERSION_TAG.to_vec();
        buf.extend_from_slice(&64u16.to_be_bytes());
        buf.extend_from_slice(&[0u8; 32]); // only 32 bytes provided
        let err = DecodedVerifierResults::new(&buf).unwrap_err();
        assert_eq!(err, CommitteeVerifierError::MalformedVerifierResults.into());
    }

    // ── compute_signed_hash tests ───────────────────────────────────

    #[test]
    fn signed_hash_is_keccak_of_tag_and_message_id() {
        let message_id = [0x42u8; 32];
        let signed_hash = compute_signed_hash(&VERSION_TAG, &message_id);

        // version_tag (4) || message_id (32) — matches the off-chain committee signer.
        let mut expected_input = [0u8; 36];
        expected_input[..4].copy_from_slice(&VERSION_TAG);
        expected_input[4..].copy_from_slice(&message_id);
        let expected = hash(&expected_input).to_bytes();

        assert_eq!(signed_hash, expected);
    }

    #[test]
    fn signed_hash_differs_for_different_message_ids() {
        let hash_a = compute_signed_hash(&VERSION_TAG, &[0x01; 32]);
        let hash_b = compute_signed_hash(&VERSION_TAG, &[0x02; 32]);
        assert_ne!(hash_a, hash_b);
    }

    #[test]
    fn signed_hash_differs_for_different_version_tags() {
        let message_id = [0x01; 32];
        let hash_a = compute_signed_hash(&[0x00, 0x00, 0x00, 0x01], &message_id);
        let hash_b = compute_signed_hash(&[0x00, 0x00, 0x00, 0x02], &message_id);
        assert_ne!(hash_a, hash_b);
    }

    // ── verify_signatures tests ─────────────────────────────────────

    #[test]
    fn valid_signatures_in_ascending_order() {
        let message_id = [0xAB; 32];
        let signed_hash = compute_signed_hash(&VERSION_TAG, &message_id);
        let signers = sorted_signers(&signed_hash, 3);
        let authorized: Vec<[u8; 20]> = signers.iter().map(|s| s.address).collect();

        let sig_bytes: Vec<[u8; 64]> = signers
            .iter()
            .map(|s| sign_normalized(&s.secret, &signed_hash))
            .collect();
        let sig_refs: Vec<&[u8]> = sig_bytes.iter().map(|s| s.as_slice()).collect();

        verify_signatures(&signed_hash, &sig_refs, &authorized).unwrap();
    }

    #[test]
    fn subset_of_authorized_signers_accepted() {
        let message_id = [0xCD; 32];
        let signed_hash = compute_signed_hash(&VERSION_TAG, &message_id);
        let signers = sorted_signers(&signed_hash, 5);
        let authorized: Vec<[u8; 20]> = signers.iter().map(|s| s.address).collect();

        // Only use first 2 of 5 authorized signers
        let sig_bytes: Vec<[u8; 64]> = signers[..2]
            .iter()
            .map(|s| sign_normalized(&s.secret, &signed_hash))
            .collect();
        let sig_refs: Vec<&[u8]> = sig_bytes.iter().map(|s| s.as_slice()).collect();

        verify_signatures(&signed_hash, &sig_refs, &authorized).unwrap();
    }

    #[test]
    fn signatures_out_of_order_rejected() {
        let message_id = [0xAB; 32];
        let signed_hash = compute_signed_hash(&VERSION_TAG, &message_id);
        let signers = sorted_signers(&signed_hash, 2);
        let authorized: Vec<[u8; 20]> = signers.iter().map(|s| s.address).collect();

        // Reverse: descending order
        let sig_bytes: Vec<[u8; 64]> = signers
            .iter()
            .rev()
            .map(|s| sign_normalized(&s.secret, &signed_hash))
            .collect();
        let sig_refs: Vec<&[u8]> = sig_bytes.iter().map(|s| s.as_slice()).collect();

        let err = verify_signatures(&signed_hash, &sig_refs, &authorized).unwrap_err();
        assert_eq!(err, CommitteeVerifierError::SignaturesNotSorted.into());
    }

    #[test]
    fn duplicate_signature_rejected() {
        let message_id = [0xAB; 32];
        let signed_hash = compute_signed_hash(&VERSION_TAG, &message_id);
        let signers = sorted_signers(&signed_hash, 1);
        let authorized: Vec<[u8; 20]> = signers.iter().map(|s| s.address).collect();

        let sig = sign_normalized(&signers[0].secret, &signed_hash);
        let sig_refs: Vec<&[u8]> = vec![&sig, &sig];

        let err = verify_signatures(&signed_hash, &sig_refs, &authorized).unwrap_err();
        assert_eq!(err, CommitteeVerifierError::SignaturesNotSorted.into());
    }

    #[test]
    fn unauthorized_signer_rejected() {
        let message_id = [0xAB; 32];
        let signed_hash = compute_signed_hash(&VERSION_TAG, &message_id);
        let signers = sorted_signers(&signed_hash, 2);

        // Only authorize the first signer
        let authorized: Vec<[u8; 20]> = vec![signers[0].address];

        // But present signatures from both
        let sig_bytes: Vec<[u8; 64]> = signers
            .iter()
            .map(|s| sign_normalized(&s.secret, &signed_hash))
            .collect();
        let sig_refs: Vec<&[u8]> = sig_bytes.iter().map(|s| s.as_slice()).collect();

        let err = verify_signatures(&signed_hash, &sig_refs, &authorized).unwrap_err();
        assert_eq!(err, CommitteeVerifierError::UnauthorizedSigner.into());
    }

    #[test]
    fn corrupted_signature_rejected() {
        let message_id = [0xAB; 32];
        let signed_hash = compute_signed_hash(&VERSION_TAG, &message_id);
        let signers = sorted_signers(&signed_hash, 1);
        let authorized: Vec<[u8; 20]> = signers.iter().map(|s| s.address).collect();

        let mut sig = sign_normalized(&signers[0].secret, &signed_hash);
        sig[0] ^= 0xFF; // corrupt one byte
        let sig_refs: Vec<&[u8]> = vec![&sig];

        let err = verify_signatures(&signed_hash, &sig_refs, &authorized).unwrap_err();
        // Corrupted sig either fails recovery or recovers to a wrong address
        assert!(
            err == CommitteeVerifierError::InvalidSignature.into()
                || err == CommitteeVerifierError::UnauthorizedSigner.into()
        );
    }

    #[test]
    fn wrong_message_hash_rejected() {
        let signed_hash = compute_signed_hash(&VERSION_TAG, &[0xAA; 32]);
        let signers = sorted_signers(&signed_hash, 1);
        let wrong_hash = compute_signed_hash(&VERSION_TAG, &[0xBB; 32]);
        let authorized: Vec<[u8; 20]> = signers.iter().map(|s| s.address).collect();

        // Sign the correct hash but verify against the wrong one
        let sig = sign_normalized(&signers[0].secret, &signed_hash);
        let sig_refs: Vec<&[u8]> = vec![&sig];

        let err = verify_signatures(&wrong_hash, &sig_refs, &authorized).unwrap_err();
        assert_eq!(err, CommitteeVerifierError::UnauthorizedSigner.into());
    }

    #[test]
    fn empty_signatures_accepted() {
        let authorized: Vec<[u8; 20]> = vec![[0x11; 20]];
        verify_signatures(&[0u8; 32], &[], &authorized).unwrap();
    }
}
