use std::fmt::Display;

use anchor_lang::prelude::*;

use crate::CommonCcipError;

pub trait AccountSchemaVersion {
    /// Current maximum version of the account schema. This is increased when there is a change
    /// to the account schema.
    const MAX_V: u8;

    fn version(&self) -> u8;

    fn is_latest_version(&self) -> bool {
        self.version() == Self::MAX_V
    }

    fn is_uninitialized(&self) -> bool {
        self.version() == 0
    }
}

#[derive(Debug, PartialEq, Eq, Clone, Copy, InitSpace, AnchorDeserialize, AnchorSerialize)]
#[borsh(use_discriminant = true)]
#[repr(u8)]
pub enum CodeVersion {
    Default = 0,
    V1,
}

impl Display for CodeVersion {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            CodeVersion::Default => write!(f, "Default"),
            CodeVersion::V1 => write!(f, "V1"),
        }
    }
}

impl From<CodeVersion> for u8 {
    fn from(v: CodeVersion) -> Self {
        // SAFETY: CodeVersion is #[repr(u8)] with variants 0 and 1
        v as u8
    }
}

impl TryFrom<u8> for CodeVersion {
    type Error = CommonCcipError;

    fn try_from(value: u8) -> std::result::Result<Self, Self::Error> {
        match value {
            0 => Ok(CodeVersion::Default),
            1 => Ok(CodeVersion::V1),
            _ => Err(CommonCcipError::InvalidCodeVersion),
        }
    }
}

#[derive(InitSpace, AnchorSerialize, AnchorDeserialize, Clone, Copy, PartialEq, Debug)]
pub enum PoolChainConfigOverrideType {
    FTF,
}

impl PoolChainConfigOverrideType {
    pub const fn as_bytes(&self) -> &'static [u8] {
        match self {
            PoolChainConfigOverrideType::FTF => b"ftf",
        }
    }
}
