//! Packs/unpacks the message-ID-binding payload embedded in the Lombard bridge's
//! `optional_message: Option<[u8; 36]>` field.
//!
//! Layout mirrors EVM's `optionalMessage = versionTag (4 bytes) || messageId (32 bytes)`.
//! The binding fills the field exactly: bridge::OPTIONAL_MESSAGE_SIZE is 36.

use crate::bridge::constants::OPTIONAL_MESSAGE_SIZE;

use crate::constants::MESSAGE_BINDING_SIZE;

pub fn pack_message_binding(
    version_tag: [u8; 4],
    message_id: [u8; 32],
) -> [u8; OPTIONAL_MESSAGE_SIZE] {
    let mut packed = [0u8; OPTIONAL_MESSAGE_SIZE];
    packed[..4].copy_from_slice(&version_tag);
    packed[4..MESSAGE_BINDING_SIZE].copy_from_slice(&message_id);
    packed
}

/// Returns `(version_tag, message_id)` if `packed` starts with the expected binding layout.
pub fn unpack_message_binding(packed: &[u8; OPTIONAL_MESSAGE_SIZE]) -> ([u8; 4], [u8; 32]) {
    let mut version_tag = [0u8; 4];
    version_tag.copy_from_slice(&packed[..4]);
    let mut message_id = [0u8; 32];
    message_id.copy_from_slice(&packed[4..MESSAGE_BINDING_SIZE]);
    (version_tag, message_id)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn round_trips() {
        let version_tag = [0xDE, 0xAD, 0xBE, 0xEF];
        let message_id = [7u8; 32];
        let packed = pack_message_binding(version_tag, message_id);
        assert_eq!(packed.len(), OPTIONAL_MESSAGE_SIZE);
        let (unpacked_tag, unpacked_id) = unpack_message_binding(&packed);
        assert_eq!(unpacked_tag, version_tag);
        assert_eq!(unpacked_id, message_id);
    }
}
