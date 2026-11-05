/**
 * Program IDL in camelCase format in order to be used in JS/TS.
 *
 * Note that this is only a type helper and is not the actual IDL. The original
 * IDL can be found at `target/idl/executor.json`.
 */
export type Executor = {
  "address": "Exex3nkLcxMFASH75gudot5ZSQXUMRc8BrYaYHTDJrt4",
  "metadata": {
    "name": "executor",
    "version": "2.0.0",
    "spec": "0.1.0",
    "description": "CCIP Executor"
  },
  "instructions": [
    {
      "name": "acceptOwnership",
      "docs": [
        "Accepts a pending ownership transfer. Must be called by the proposed owner."
      ],
      "discriminator": [
        172,
        23,
        43,
        13,
        238,
        213,
        85,
        150
      ],
      "accounts": [
        {
          "name": "config",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              }
            ]
          }
        },
        {
          "name": "authority",
          "signer": true
        },
        {
          "name": "eventAuthority"
        },
        {
          "name": "program"
        }
      ],
      "args": []
    },
    {
      "name": "applyAllowedCcvUpdates",
      "docs": [
        "Delta-shaped counterpart to `set_allowed_ccvs`, matching EVM's",
        "`Executor.applyAllowedCCVUpdates(ccvsToRemove, ccvsToAdd, ccvAllowlistEnabled)` argument for",
        "argument: removals are applied first, then additions, then the allowlist flag. Both lists",
        "are idempotent."
      ],
      "discriminator": [
        254,
        32,
        208,
        188,
        119,
        1,
        254,
        156
      ],
      "accounts": [
        {
          "name": "config",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              }
            ]
          }
        },
        {
          "name": "authority",
          "signer": true
        },
        {
          "name": "eventAuthority"
        },
        {
          "name": "program"
        }
      ],
      "args": [
        {
          "name": "params",
          "type": {
            "defined": {
              "name": "applyAllowedCcvUpdatesParams"
            }
          }
        }
      ]
    },
    {
      "name": "getFee",
      "discriminator": [
        115,
        195,
        235,
        161,
        25,
        219,
        60,
        29
      ],
      "accounts": [
        {
          "name": "config",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              }
            ]
          }
        },
        {
          "name": "chainConfig",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  114,
                  101,
                  109,
                  111,
                  116,
                  101,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110
                ]
              },
              {
                "kind": "arg",
                "path": "params.destChainSelector"
              }
            ]
          }
        },
        {
          "name": "eventAuthority"
        },
        {
          "name": "program"
        }
      ],
      "args": [
        {
          "name": "params",
          "type": {
            "defined": {
              "name": "getFeeParams"
            }
          }
        }
      ],
      "returns": {
        "defined": {
          "name": "usdCents"
        }
      }
    },
    {
      "name": "initialize",
      "docs": [
        "Initializes the Executor Config account.",
        "",
        "Restricted to the program upgrade authority."
      ],
      "discriminator": [
        175,
        175,
        109,
        31,
        13,
        152,
        155,
        237
      ],
      "accounts": [
        {
          "name": "config",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              }
            ]
          }
        },
        {
          "name": "authority",
          "writable": true,
          "signer": true
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        },
        {
          "name": "eventAuthority"
        },
        {
          "name": "program",
          "address": "Exex3nkLcxMFASH75gudot5ZSQXUMRc8BrYaYHTDJrt4"
        },
        {
          "name": "programData"
        }
      ],
      "args": [
        {
          "name": "params",
          "type": {
            "defined": {
              "name": "initializeParams"
            }
          }
        }
      ]
    },
    {
      "name": "observeAllowedCcvs",
      "docs": [
        "Returns a page of this executor's CCV allow list, for observability consumers.",
        "",
        "Permissionless and read-only: meant to be executed through `simulateTransaction`."
      ],
      "discriminator": [
        255,
        194,
        105,
        142,
        168,
        71,
        105,
        99
      ],
      "accounts": [
        {
          "name": "config",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              }
            ]
          }
        }
      ],
      "args": [
        {
          "name": "page",
          "type": {
            "defined": {
              "name": "observePageParams"
            }
          }
        }
      ],
      "returns": {
        "defined": {
          "name": "executorAllowedCcvsObservation"
        }
      }
    },
    {
      "name": "observeChainConfig",
      "docs": [
        "Returns this executor's configuration for a single destination chain, for observability",
        "consumers.",
        "",
        "Permissionless and read-only: meant to be executed through `simulateTransaction`. Fails if",
        "the chain has no configuration, which callers can read as \"not configured\"."
      ],
      "discriminator": [
        120,
        31,
        158,
        11,
        8,
        110,
        2,
        54
      ],
      "accounts": [
        {
          "name": "chainConfig",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  114,
                  101,
                  109,
                  111,
                  116,
                  101,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110
                ]
              },
              {
                "kind": "arg",
                "path": "remoteChainSelector"
              }
            ]
          }
        }
      ],
      "args": [
        {
          "name": "remoteChainSelector",
          "type": "u64"
        }
      ],
      "returns": {
        "defined": {
          "name": "executorChainConfigObservation"
        }
      }
    },
    {
      "name": "observeConfig",
      "docs": [
        "Returns this executor's global configuration, for observability consumers.",
        "",
        "Permissionless and read-only: meant to be executed through `simulateTransaction`. The CCV",
        "allow list is reported as a count only; page through it with `observe_allowed_ccvs`."
      ],
      "discriminator": [
        139,
        122,
        253,
        169,
        59,
        227,
        193,
        228
      ],
      "accounts": [
        {
          "name": "config",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              }
            ]
          }
        }
      ],
      "args": [],
      "returns": {
        "defined": {
          "name": "executorConfigObservation"
        }
      }
    },
    {
      "name": "resolveAccountsStart",
      "docs": [
        "Entrypoint to resolve accounts for any supported instruction."
      ],
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
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              }
            ]
          }
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
    },
    {
      "name": "setAllowedCcvs",
      "docs": [
        "Replaces the CCV allowlist with `ccvs` and sets the allowlist flag, emitting one event per",
        "CCV that actually entered or left the list."
      ],
      "discriminator": [
        189,
        80,
        177,
        150,
        153,
        19,
        42,
        245
      ],
      "accounts": [
        {
          "name": "config",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              }
            ]
          }
        },
        {
          "name": "authority",
          "signer": true
        },
        {
          "name": "eventAuthority"
        },
        {
          "name": "program"
        }
      ],
      "args": [
        {
          "name": "params",
          "type": {
            "defined": {
              "name": "setAllowedCcvsParams"
            }
          }
        }
      ]
    },
    {
      "name": "setChainConfig",
      "docs": [
        "Sets the per-chain configuration for a remote destination chain."
      ],
      "discriminator": [
        252,
        248,
        193,
        186,
        119,
        43,
        227,
        108
      ],
      "accounts": [
        {
          "name": "config",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              }
            ]
          }
        },
        {
          "name": "authority",
          "writable": true,
          "signer": true
        },
        {
          "name": "chainConfig",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  114,
                  101,
                  109,
                  111,
                  116,
                  101,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110
                ]
              },
              {
                "kind": "arg",
                "path": "params.remoteChainSelector"
              }
            ]
          }
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        },
        {
          "name": "eventAuthority"
        },
        {
          "name": "program"
        }
      ],
      "args": [
        {
          "name": "params",
          "type": {
            "defined": {
              "name": "setChainConfigParams"
            }
          }
        }
      ]
    },
    {
      "name": "setConfig",
      "discriminator": [
        108,
        158,
        154,
        175,
        212,
        98,
        52,
        66
      ],
      "accounts": [
        {
          "name": "config",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              }
            ]
          }
        },
        {
          "name": "authority",
          "signer": true
        },
        {
          "name": "eventAuthority"
        },
        {
          "name": "program"
        }
      ],
      "args": [
        {
          "name": "params",
          "type": {
            "defined": {
              "name": "setConfigParams"
            }
          }
        }
      ]
    },
    {
      "name": "transferOwnership",
      "docs": [
        "Transfers ownership to a new proposed owner. The proposed owner must",
        "call `accept_ownership` to complete the transfer."
      ],
      "discriminator": [
        65,
        177,
        215,
        73,
        53,
        45,
        99,
        47
      ],
      "accounts": [
        {
          "name": "config",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              }
            ]
          }
        },
        {
          "name": "authority",
          "signer": true
        },
        {
          "name": "eventAuthority"
        },
        {
          "name": "program"
        }
      ],
      "args": [
        {
          "name": "proposedOwner",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "typeVersion",
      "docs": [
        "Returns the program type (name) and version."
      ],
      "discriminator": [
        129,
        251,
        8,
        243,
        122,
        229,
        252,
        164
      ],
      "accounts": [
        {
          "name": "clock",
          "address": "SysvarC1ock11111111111111111111111111111111"
        }
      ],
      "args": [],
      "returns": "string"
    },
    {
      "name": "withdrawFeeTokens",
      "docs": [
        "Sweeps every fee this executor accrued in `fee_token_mint` into the configured fee",
        "aggregator's ATA.",
        "",
        "Permissionless: the destination is pinned to `config.fee_aggregator`, so a caller can only",
        "trigger the sweep, never redirect it."
      ],
      "discriminator": [
        130,
        83,
        245,
        243,
        236,
        40,
        197,
        229
      ],
      "accounts": [
        {
          "name": "config",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              }
            ]
          }
        },
        {
          "name": "feeVaultTokenAccount",
          "docs": [
            "The account the router pays this executor's fees into: the ATA of `fee_vault` for the fee",
            "token. There is one per fee token, since the sender picks which token to bill in."
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "account",
                "path": "feeVault"
              },
              {
                "kind": "account",
                "path": "feeTokenProgram"
              },
              {
                "kind": "account",
                "path": "feeTokenMint"
              }
            ],
            "program": {
              "kind": "const",
              "value": [
                140,
                151,
                37,
                143,
                78,
                36,
                137,
                241,
                187,
                61,
                16,
                41,
                20,
                142,
                13,
                131,
                11,
                90,
                19,
                153,
                218,
                255,
                16,
                132,
                4,
                142,
                123,
                216,
                219,
                233,
                248,
                89
              ]
            }
          }
        },
        {
          "name": "feeVault",
          "docs": [
            "The router derives the destination of every executor receipt fee from these exact seeds",
            "(see `ccip_common::seed::vault_ata`), so this is the only authority that can move the",
            "accrued fees out."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  102,
                  101,
                  101,
                  95,
                  118,
                  97,
                  117,
                  108,
                  116
                ]
              }
            ]
          }
        },
        {
          "name": "feeAggregatorTokenAccount",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "account",
                "path": "config.feeAggregator",
                "account": "config"
              },
              {
                "kind": "account",
                "path": "feeTokenProgram"
              },
              {
                "kind": "account",
                "path": "feeTokenMint"
              }
            ],
            "program": {
              "kind": "const",
              "value": [
                140,
                151,
                37,
                143,
                78,
                36,
                137,
                241,
                187,
                61,
                16,
                41,
                20,
                142,
                13,
                131,
                11,
                90,
                19,
                153,
                218,
                255,
                16,
                132,
                4,
                142,
                123,
                216,
                219,
                233,
                248,
                89
              ]
            }
          }
        },
        {
          "name": "feeTokenMint"
        },
        {
          "name": "feeTokenProgram"
        }
      ],
      "args": []
    }
  ],
  "accounts": [
    {
      "name": "chainConfig",
      "discriminator": [
        13,
        177,
        233,
        141,
        212,
        29,
        148,
        56
      ]
    },
    {
      "name": "config",
      "discriminator": [
        155,
        12,
        170,
        224,
        30,
        250,
        204,
        130
      ]
    }
  ],
  "events": [
    {
      "name": "ccvAdded",
      "discriminator": [
        201,
        219,
        172,
        138,
        169,
        127,
        22,
        116
      ]
    },
    {
      "name": "ccvAllowlistUpdated",
      "discriminator": [
        214,
        197,
        38,
        143,
        224,
        94,
        135,
        92
      ]
    },
    {
      "name": "ccvRemoved",
      "discriminator": [
        0,
        94,
        15,
        206,
        26,
        134,
        216,
        31
      ]
    },
    {
      "name": "chainConfigSet",
      "discriminator": [
        109,
        91,
        11,
        236,
        150,
        51,
        67,
        6
      ]
    },
    {
      "name": "configSet",
      "discriminator": [
        15,
        104,
        59,
        16,
        236,
        241,
        8,
        6
      ]
    },
    {
      "name": "ownershipTransferRequested",
      "discriminator": [
        79,
        54,
        99,
        123,
        57,
        244,
        134,
        35
      ]
    },
    {
      "name": "ownershipTransferred",
      "discriminator": [
        172,
        61,
        205,
        183,
        250,
        50,
        38,
        98
      ]
    }
  ],
  "errors": [
    {
      "code": 14000,
      "name": "unauthorized",
      "msg": "The signer is unauthorized"
    },
    {
      "code": 14001,
      "name": "invalidVersion",
      "msg": "Invalid version of the onchain state"
    },
    {
      "code": 14002,
      "name": "invalidInstructionData",
      "msg": "Invalid instruction data"
    },
    {
      "code": 14003,
      "name": "defaultOwnerProposal",
      "msg": "Proposed owner cannot be the default pubkey"
    },
    {
      "code": 14004,
      "name": "redundantOwnerProposal",
      "msg": "Proposed owner is the current owner"
    },
    {
      "code": 14005,
      "name": "invalidCodeVersion",
      "msg": "Invalid code version"
    },
    {
      "code": 14006,
      "name": "invalidDestChain",
      "msg": "Invalid destination chain"
    },
    {
      "code": 14007,
      "name": "duplicateCcv",
      "msg": "Duplicate CCV"
    },
    {
      "code": 14008,
      "name": "exceedsMaxCcVs",
      "msg": "Exceeds maximum CCVs per message"
    },
    {
      "code": 14009,
      "name": "invalidMaxCcVs",
      "msg": "Invalid maximum CCVs per message"
    },
    {
      "code": 14010,
      "name": "invalidFinality",
      "msg": "Invalid Finality"
    },
    {
      "code": 14011,
      "name": "invalidCcv",
      "msg": "Invalid CCV"
    },
    {
      "code": 14012,
      "name": "feeAggregatorNotSet",
      "msg": "Fee aggregator not set"
    },
    {
      "code": 14013,
      "name": "insufficientFunds",
      "msg": "Insufficient funds (vault empty)"
    },
    {
      "code": 14014,
      "name": "tooManyCcvUpdates",
      "msg": "Too many CCV allowlist changes in one instruction"
    },
    {
      "code": 14015,
      "name": "tooManyCcVs",
      "msg": "Allowlist exceeds the config account's CCV capacity"
    }
  ],
  "types": [
    {
      "name": "applyAllowedCcvUpdatesParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "ccvsToRemove",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "ccvsToAdd",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "allowlistEnabled",
            "type": "bool"
          }
        ]
      }
    },
    {
      "name": "ccvAdded",
      "docs": [
        "Mirrors EVM's `Executor.CCVAdded(address indexed ccv)`."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "ccv",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "ccvAllowlistUpdated",
      "docs": [
        "Mirrors EVM's `Executor.CCVAllowlistUpdated(bool enabled)`. Like EVM, only emitted when the",
        "flag actually changes."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "enabled",
            "type": "bool"
          }
        ]
      }
    },
    {
      "name": "ccvRemoved",
      "docs": [
        "Mirrors EVM's `Executor.CCVRemoved(address indexed ccv)`."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "ccv",
            "type": "pubkey"
          }
        ]
      }
    },
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
      "name": "chainConfig",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "version",
            "type": "u8"
          },
          {
            "name": "bump",
            "type": "u8"
          },
          {
            "name": "chainSelector",
            "type": "u64"
          },
          {
            "name": "usdCentsFee",
            "type": {
              "defined": {
                "name": "usdCents"
              }
            }
          },
          {
            "name": "enabled",
            "type": "bool"
          }
        ]
      }
    },
    {
      "name": "chainConfigSet",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "usdCentsFee",
            "type": {
              "defined": {
                "name": "usdCents"
              }
            }
          },
          {
            "name": "enabled",
            "type": "bool"
          }
        ]
      }
    },
    {
      "name": "config",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "version",
            "type": "u8"
          },
          {
            "name": "bump",
            "type": "u8"
          },
          {
            "name": "maxCcvsPerMsg",
            "type": "u8"
          },
          {
            "name": "owner",
            "type": "pubkey"
          },
          {
            "name": "proposedOwner",
            "type": "pubkey"
          },
          {
            "name": "feeAggregator",
            "type": "pubkey"
          },
          {
            "name": "allowedFinalityConfig",
            "type": {
              "defined": {
                "name": "finalityConfig"
              }
            }
          },
          {
            "name": "ccvAllowlistEnabled",
            "type": "bool"
          },
          {
            "name": "allowedCcvs",
            "type": {
              "vec": "pubkey"
            }
          }
        ]
      }
    },
    {
      "name": "configSet",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "feeAggregator",
            "type": "pubkey"
          },
          {
            "name": "allowedFinalityConfig",
            "type": {
              "defined": {
                "name": "finalityConfig"
              }
            }
          },
          {
            "name": "ccvAllowlistEnabled",
            "type": "bool"
          }
        ]
      }
    },
    {
      "name": "executorAllowedCcvsObservation",
      "docs": [
        "A page of this executor's CCV allow list."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "observationVersion",
            "type": "u16"
          },
          {
            "name": "allowedCcvsCount",
            "docs": [
              "Total number of entries in the allow list, so consumers know when to stop paging."
            ],
            "type": "u32"
          },
          {
            "name": "offset",
            "docs": [
              "Offset the returned page starts at."
            ],
            "type": "u32"
          },
          {
            "name": "allowedCcvs",
            "type": {
              "vec": "pubkey"
            }
          }
        ]
      }
    },
    {
      "name": "executorChainConfigObservation",
      "docs": [
        "Per-destination-chain configuration of this executor, as observed by off-chain consumers."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "observationVersion",
            "type": "u16"
          },
          {
            "name": "typeVersion",
            "type": "string"
          },
          {
            "name": "version",
            "type": "u8"
          },
          {
            "name": "remoteChainSelector",
            "docs": [
              "Taken from the instruction argument: this account predates storing its own selector."
            ],
            "type": "u64"
          },
          {
            "name": "usdCentsFee",
            "type": {
              "defined": {
                "name": "usdCents"
              }
            }
          },
          {
            "name": "enabled",
            "type": "bool"
          }
        ]
      }
    },
    {
      "name": "executorConfigObservation",
      "docs": [
        "Global configuration of this executor, as observed by off-chain consumers.",
        "",
        "The CCV allow list holds up to 64 pubkeys, which does not fit the 1024-byte return data limit,",
        "so only its length is reported here. Use `observe_allowed_ccvs` to page through the entries."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "observationVersion",
            "type": "u16"
          },
          {
            "name": "typeVersion",
            "type": "string"
          },
          {
            "name": "version",
            "type": "u8"
          },
          {
            "name": "maxCcvsPerMsg",
            "type": "u8"
          },
          {
            "name": "owner",
            "type": "pubkey"
          },
          {
            "name": "proposedOwner",
            "type": "pubkey"
          },
          {
            "name": "feeAggregator",
            "type": "pubkey"
          },
          {
            "name": "allowedFinalityConfig",
            "type": {
              "defined": {
                "name": "finalityConfig"
              }
            }
          },
          {
            "name": "ccvAllowlistEnabled",
            "type": "bool"
          },
          {
            "name": "allowedCcvsCount",
            "type": "u32"
          }
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
      "name": "getFeeParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "requestedFinality",
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
            "name": "extraArgs",
            "type": "bytes"
          },
          {
            "name": "feeToken",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "initializeParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "maxCcvsPerMsg",
            "type": "u8"
          },
          {
            "name": "feeAggregator",
            "type": "pubkey"
          },
          {
            "name": "allowedFinalityConfig",
            "type": {
              "defined": {
                "name": "finalityConfig"
              }
            }
          },
          {
            "name": "ccvAllowlistEnabled",
            "type": "bool"
          }
        ]
      }
    },
    {
      "name": "observePageParams",
      "docs": [
        "Parameters for the paged `observe_*` instructions."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "offset",
            "docs": [
              "Index of the first item to return."
            ],
            "type": "u32"
          },
          {
            "name": "limit",
            "docs": [
              "Maximum number of items to return, capped at [`MAX_OBSERVE_PAGE_LEN`]."
            ],
            "type": "u32"
          }
        ]
      }
    },
    {
      "name": "ownershipTransferRequested",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "from",
            "type": "pubkey"
          },
          {
            "name": "to",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "ownershipTransferred",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "from",
            "type": "pubkey"
          },
          {
            "name": "to",
            "type": "pubkey"
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
      "name": "setAllowedCcvsParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "ccvs",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "allowlistEnabled",
            "type": "bool"
          }
        ]
      }
    },
    {
      "name": "setChainConfigParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "usdCentsFee",
            "type": {
              "defined": {
                "name": "usdCents"
              }
            }
          },
          {
            "name": "enabled",
            "type": "bool"
          }
        ]
      }
    },
    {
      "name": "setConfigParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "feeAggregator",
            "type": "pubkey"
          },
          {
            "name": "allowedFinalityConfig",
            "type": {
              "defined": {
                "name": "finalityConfig"
              }
            }
          },
          {
            "name": "ccvAllowlistEnabled",
            "type": "bool"
          }
        ]
      }
    },
    {
      "name": "usdCents",
      "type": {
        "kind": "struct",
        "fields": [
          "u32"
        ]
      }
    }
  ]
};
