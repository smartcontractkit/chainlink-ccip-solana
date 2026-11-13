/**
 * Program IDL in camelCase format in order to be used in JS/TS.
 *
 * Note that this is only a type helper and is not the actual IDL. The original
 * IDL can be found at `target/idl/ccip_common.json`.
 */
export type CcipCommon = {
  "address": "Ccip842gzYHhvdDkSyi2YVCoAWPbYJoApMFzSxQroE9C",
  "metadata": {
    "name": "ccipCommon",
    "version": "2.1.0",
    "spec": "0.1.0"
  },
  "instructions": [
    {
      "name": "resolveAccountsStart",
      "discriminator": [
        195,
        88,
        233,
        63,
        16,
        50,
        55,
        52
      ],
      "accounts": [
        {
          "name": "config",
          "docs": [
            "This here is only used to generate common bindings, though each contract will define its own context."
          ]
        }
      ],
      "args": [
        {
          "name": "params",
          "type": {
            "defined": {
              "name": "resolveAccountsParams"
            }
          }
        }
      ],
      "returns": {
        "defined": {
          "name": "resolveAccountsResponse"
        }
      }
    }
  ],
  "events": [
    {
      "name": "idlBuildTypeExport",
      "discriminator": [
        116,
        138,
        204,
        211,
        57,
        158,
        245,
        116
      ]
    }
  ],
  "errors": [
    {
      "code": 10000,
      "name": "invalidSequenceInterval",
      "msg": "The given sequence interval is invalid"
    },
    {
      "code": 10001,
      "name": "invalidInputsPoolAccounts",
      "msg": "Invalid pool accounts"
    },
    {
      "code": 10002,
      "name": "invalidInputsTokenAccounts",
      "msg": "Invalid token accounts"
    },
    {
      "code": 10003,
      "name": "invalidInputsTokenAdminRegistryAccounts",
      "msg": "Invalid Token Admin Registry account"
    },
    {
      "code": 10004,
      "name": "invalidInputsLookupTableAccounts",
      "msg": "Invalid LookupTable account"
    },
    {
      "code": 10005,
      "name": "invalidInputsLookupTableAccountWritable",
      "msg": "Invalid LookupTable account writable access"
    },
    {
      "code": 10006,
      "name": "invalidInputsPoolSignerAccounts",
      "msg": "Invalid pool signer account"
    },
    {
      "code": 10007,
      "name": "invalidChainFamilySelector",
      "msg": "Invalid chain family selector"
    },
    {
      "code": 10008,
      "name": "invalidEncoding",
      "msg": "Invalid encoding"
    },
    {
      "code": 10009,
      "name": "invalidEvmAddress",
      "msg": "Invalid EVM address"
    },
    {
      "code": 10010,
      "name": "invalidSvmAddress",
      "msg": "Invalid SVM address"
    },
    {
      "code": 10011,
      "name": "invalidTvmAddress",
      "msg": "Invalid TVM address"
    },
    {
      "code": 10012,
      "name": "invalidAptosAddress",
      "msg": "Invalid Aptos address"
    },
    {
      "code": 10013,
      "name": "invalidSuiAddress",
      "msg": "Invalid Sui address"
    },
    {
      "code": 10014,
      "name": "messageDecodingError",
      "msg": "Message decoding error"
    },
    {
      "code": 10015,
      "name": "invalidResolutionInstructionData",
      "msg": "Invalid instruction data for account resolution"
    },
    {
      "code": 10016,
      "name": "invalidResolutionMetadata",
      "msg": "Invalid metadata for account resolution"
    },
    {
      "code": 10017,
      "name": "invalidNestedResolutionSigner",
      "msg": "Unexpected signer in nested account resolution response"
    },
    {
      "code": 10018,
      "name": "accountListLengthOverflow",
      "msg": "Account list length overflow"
    },
    {
      "code": 10019,
      "name": "invalidCodeVersion",
      "msg": "Invalid code version"
    },
    {
      "code": 10020,
      "name": "invalidPoolInterfaceVersion",
      "msg": "Invalid pool interface version"
    },
    {
      "code": 10021,
      "name": "invalidProtocolAmount",
      "msg": "Invalid ProtocolAmount"
    },
    {
      "code": 10022,
      "name": "missingCpiReturnData",
      "msg": "MissingCpiReturnData"
    },
    {
      "code": 10023,
      "name": "invalidCpiReturnProgram",
      "msg": "InvalidCpiReturnProgram"
    },
    {
      "code": 10024,
      "name": "invalidCpiTargetProgram",
      "msg": "CPI target program does not match the expected program"
    },
    {
      "code": 10025,
      "name": "invalidAccountListLengths",
      "msg": "InvalidAccountListLengths"
    },
    {
      "code": 10026,
      "name": "invalidAccountVersion",
      "msg": "Invalid Account version"
    },
    {
      "code": 10027,
      "name": "invalidAccountData",
      "msg": "Invalid Account data"
    },
    {
      "code": 10028,
      "name": "invalidReceiverVersion",
      "msg": "Invalid receiver version"
    },
    {
      "code": 10029,
      "name": "invalidReceiverRegistry",
      "msg": "Invalid receiver registry account"
    },
    {
      "code": 10030,
      "name": "invalidUsdConversion",
      "msg": "Invalid USD value conversion"
    },
    {
      "code": 10031,
      "name": "resolutionPageCapacityExhausted",
      "msg": "Account resolution response is too full to paginate"
    },
    {
      "code": 10032,
      "name": "tooManyCcvs",
      "msg": "Message requires more CCVs than account resolution can describe"
    },
    {
      "code": 10033,
      "name": "invalidCantonAddress",
      "msg": "Invalid Canton address"
    }
  ],
  "types": [
    {
      "name": "ccipAccountMeta",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "pubkey",
            "type": "pubkey"
          },
          {
            "name": "isSigner",
            "type": "bool"
          },
          {
            "name": "isWritable",
            "type": "bool"
          }
        ]
      }
    },
    {
      "name": "crossChainGas",
      "type": {
        "kind": "struct",
        "fields": [
          "u32"
        ]
      }
    },
    {
      "name": "finalityConfig",
      "docs": [
        "Per-message finality configuration. Wire-format: 4 bytes big-endian (flags || block_depth).",
        "Mirrors the `bytes4` Finality type in chainlink-ccv's `protocol/finality.go`.",
        "",
        "Layout (MSB first):",
        "flags[15:1]  — reserved",
        "flags[0]     — S: wait for `safe` head instead of full finality",
        "block_depth  — number of block confirmations required (0 = full finality / safe, per flags)"
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "flags",
            "type": "u16"
          },
          {
            "name": "blockDepth",
            "type": "u16"
          }
        ]
      }
    },
    {
      "name": "genericExtraArgsV2",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "gasLimit",
            "type": "u128"
          },
          {
            "name": "allowOutOfOrderExecution",
            "type": "bool"
          }
        ]
      }
    },
    {
      "name": "genericExtraArgsV3",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "gasLimit",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "finality",
            "type": {
              "defined": {
                "name": "finalityConfig"
              }
            }
          },
          {
            "name": "ccvs",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "ccvArgs",
            "type": {
              "vec": "bytes"
            }
          },
          {
            "name": "executor",
            "type": "pubkey"
          },
          {
            "name": "executorArgs",
            "type": "bytes"
          },
          {
            "name": "tokenReceiver",
            "type": "bytes"
          },
          {
            "name": "tokenArgs",
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "idlBuildTypeExport",
      "docs": [
        "Anchor 0.32 only includes types in the IDL when they are referenced by an instruction",
        "argument, an #[account] context, or an #[event] struct. Types that are deserialized from",
        "raw bytes at runtime (not part of any instruction signature) would be missing from the IDL",
        "and Go bindings. This cfg-guarded event forces Anchor to include those types. It is never",
        "emitted at runtime — it only exists during `anchor idl build`."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "tokenAdminRegistry",
            "type": {
              "defined": {
                "name": "tokenAdminRegistry"
              }
            }
          },
          {
            "name": "receiverRegistry",
            "type": {
              "defined": {
                "name": "receiverRegistry"
              }
            }
          },
          {
            "name": "ccipAccountMeta",
            "type": {
              "defined": {
                "name": "ccipAccountMeta"
              }
            }
          },
          {
            "name": "resolveAccountsParams",
            "type": {
              "defined": {
                "name": "resolveAccountsParams"
              }
            }
          },
          {
            "name": "resolveAccountsResponse",
            "type": {
              "defined": {
                "name": "resolveAccountsResponse"
              }
            }
          },
          {
            "name": "svmDestBlob",
            "type": {
              "defined": {
                "name": "svmDestBlob"
              }
            }
          },
          {
            "name": "genericExtraArgsV3",
            "type": {
              "defined": {
                "name": "genericExtraArgsV3"
              }
            }
          },
          {
            "name": "genericExtraArgsV2",
            "type": {
              "defined": {
                "name": "genericExtraArgsV2"
              }
            }
          },
          {
            "name": "svmExtraArgsV1",
            "type": {
              "defined": {
                "name": "svmExtraArgsV1"
              }
            }
          },
          {
            "name": "suiExtraArgsV1",
            "type": {
              "defined": {
                "name": "suiExtraArgsV1"
              }
            }
          },
          {
            "name": "usd18Decimals",
            "type": {
              "defined": {
                "name": "usd18Decimals"
              }
            }
          }
        ]
      }
    },
    {
      "name": "receiverRegistry",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "bump",
            "type": "u8"
          },
          {
            "name": "version",
            "type": "u8"
          },
          {
            "name": "receiverProgram",
            "type": "pubkey"
          },
          {
            "name": "receiverVersion",
            "type": "u8"
          }
        ]
      }
    },
    {
      "name": "resolveAccountsParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "caller",
            "docs": [
              "The account expected to sign the final instruction being resolved."
            ],
            "type": "pubkey"
          },
          {
            "name": "ixData",
            "docs": [
              "Borsh-encoded final instruction data, including its 8-byte",
              "Anchor discriminator prefix.",
              "",
              "IMPORTANT: For CPI instructions, it's possible the entire `ix_data` cannot",
              "be known at the point of account resolution. This is common in cases where",
              "part of the input to the CPI will be determined dynamically when executing the",
              "parent instruction. In those cases, `ix_data` must contain a best-effort",
              "approximation of the final instruction data, with those fields that are impossible",
              "to know ahead of time swapped with defaults, dummies or other context-appropriate",
              "values. In these cases, the resolver must take care not to rely on those dummy",
              "values to generate a valid list of accounts. An example of this pattern is",
              "CCVs receiving a `message_id` as part of their `ix_data`. The `message_id` cannot",
              "be known until execution time, so the value must be ignored by the `CCV` resolver."
            ],
            "type": "bytes"
          },
          {
            "name": "metadata",
            "docs": [
              "Opaque Borsh-encoded metadata forwarded from the previous stage.",
              "Must be empty on the first call."
            ],
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "resolveAccountsResponse",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "askAgainWith",
            "docs": [
              "In the next resolution round, call the resolution method",
              "including these accounts (all read-only)."
            ],
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "accountsToSave",
            "docs": [
              "Append these accounts to a separate accumulator list. Once",
              "`next_ix_discriminator` is empty, this accumulated list",
              "contains all accounts for the instruction of interest."
            ],
            "type": {
              "vec": {
                "defined": {
                  "name": "ccipAccountMeta"
                }
              }
            }
          },
          {
            "name": "lookupTablesToSave",
            "docs": [
              "Append these lookup tables to a separate accumulator list.",
              "Once resolution is complete, the accumulated list contains all",
              "LUTs that the instruction of interest can use."
            ],
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "nextIxDiscriminator",
            "docs": [
              "Identifies the next resolution stage. If None, resolution is",
              "complete."
            ],
            "type": {
              "option": {
                "array": [
                  "u8",
                  8
                ]
              }
            }
          },
          {
            "name": "metadata",
            "docs": [
              "Metadata required for the next resolution stage. In some cases, it",
              "may be an input to the final instruction as well."
            ],
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "svmDestBlob",
      "docs": [
        "CCIP 2.0 cross-chain DestBlob when SVM is the destination. Can be borsh-encoded or",
        "`abi.encodePacked`-encoded. The destBlob is equal to the executorArgs."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "useAta",
            "type": {
              "defined": {
                "name": "svmTokenReceiverUsage"
              }
            }
          },
          {
            "name": "accountIsWritableBitmap",
            "type": "u64"
          },
          {
            "name": "accounts",
            "type": {
              "vec": "pubkey"
            }
          }
        ]
      }
    },
    {
      "name": "svmExtraArgsV1",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "computeUnits",
            "type": "u32"
          },
          {
            "name": "accountIsWritableBitmap",
            "type": "u64"
          },
          {
            "name": "allowOutOfOrderExecution",
            "type": "bool"
          },
          {
            "name": "tokenReceiver",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          },
          {
            "name": "accounts",
            "type": {
              "vec": {
                "array": [
                  "u8",
                  32
                ]
              }
            }
          }
        ]
      }
    },
    {
      "name": "svmTokenReceiverUsage",
      "type": {
        "kind": "enum",
        "variants": [
          {
            "name": "deriveAtaAndCreate"
          },
          {
            "name": "deriveAtaDontCreate"
          },
          {
            "name": "useAsIs"
          }
        ]
      }
    },
    {
      "name": "suiExtraArgsV1",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "gasLimit",
            "type": "u128"
          },
          {
            "name": "allowOutOfOrderExecution",
            "type": "bool"
          },
          {
            "name": "tokenReceiver",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          },
          {
            "name": "receiverObjectIds",
            "type": {
              "vec": {
                "array": [
                  "u8",
                  32
                ]
              }
            }
          }
        ]
      }
    },
    {
      "name": "tokenAdminRegistry",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "version",
            "type": "u8"
          },
          {
            "name": "administrator",
            "type": "pubkey"
          },
          {
            "name": "pendingAdministrator",
            "type": "pubkey"
          },
          {
            "name": "lookupTable",
            "type": "pubkey"
          },
          {
            "name": "writableIndexes",
            "type": {
              "array": [
                "u128",
                2
              ]
            }
          },
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "accountResolutionEnabled",
            "type": "bool"
          },
          {
            "name": "bump",
            "type": "u8"
          },
          {
            "name": "interfaceVersion",
            "type": "u8"
          },
          {
            "name": "skipLookupTableAdditionalEntries",
            "type": "bool"
          }
        ]
      }
    },
    {
      "name": "usd18Decimals",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "array": [
              "u128",
              2
            ]
          }
        ]
      }
    }
  ]
};
