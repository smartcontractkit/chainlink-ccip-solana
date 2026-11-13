/**
 * Program IDL in camelCase format in order to be used in JS/TS.
 *
 * Note that this is only a type helper and is not the actual IDL. The original
 * IDL can be found at `target/idl/lockrelease_token_pool.json`.
 */
export type LockreleaseTokenPool = {
  "address": "8eqh8wppT9c5rw4ERqNCffvU6cNFJWff9WmkcYtmGiqC",
  "metadata": {
    "name": "lockreleaseTokenPool",
    "version": "2.1.0",
    "spec": "0.1.0",
    "description": "Created with Anchor"
  },
  "instructions": [
    {
      "name": "acceptOwnership",
      "docs": [
        "Accepts a pending ownership transfer, making the caller the new pool owner.",
        "Shared function signature used by all CCIP token pool programs."
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
          "name": "state",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "mint"
        },
        {
          "name": "authority",
          "signer": true
        }
      ],
      "args": []
    },
    {
      "name": "appendRemotePoolAddresses",
      "docs": [
        "Appends additional remote pool addresses to an existing ChainConfig.",
        "Reallocates the PDA to accommodate the extra entries."
      ],
      "discriminator": [
        172,
        57,
        83,
        55,
        70,
        112,
        26,
        197
      ],
      "accounts": [
        {
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "chainConfig",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "remoteChainSelector"
              },
              {
                "kind": "arg",
                "path": "mint"
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
        }
      ],
      "args": [
        {
          "name": "remoteChainSelector",
          "type": "u64"
        },
        {
          "name": "mint",
          "type": "pubkey"
        },
        {
          "name": "addresses",
          "type": {
            "vec": {
              "defined": {
                "name": "remoteAddress"
              }
            }
          }
        }
      ]
    },
    {
      "name": "configureAllowList",
      "docs": [
        "Adds addresses to the allow-list and/or enables the allow-list check.",
        "When enabled, only listed addresses may send tokens through the pool."
      ],
      "discriminator": [
        18,
        180,
        102,
        187,
        209,
        0,
        130,
        191
      ],
      "accounts": [
        {
          "name": "state",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "mint"
        },
        {
          "name": "authority",
          "writable": true,
          "signer": true
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "add",
          "type": {
            "vec": "pubkey"
          }
        },
        {
          "name": "enabled",
          "type": "bool"
        }
      ]
    },
    {
      "name": "deleteChainConfig",
      "docs": [
        "Closes and reclaims rent from a ChainConfig PDA.",
        "After this call no cross-chain messages can be sent to or from that chain."
      ],
      "discriminator": [
        241,
        159,
        142,
        210,
        64,
        173,
        77,
        179
      ],
      "accounts": [
        {
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "chainConfig",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "remoteChainSelector"
              },
              {
                "kind": "arg",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "authority",
          "writable": true,
          "signer": true
        }
      ],
      "args": [
        {
          "name": "remoteChainSelector",
          "type": "u64"
        },
        {
          "name": "mint",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "deleteChainConfigOverride",
      "docs": [
        "Closes and reclaims rent from a ChainConfigOverride PDA."
      ],
      "discriminator": [
        17,
        37,
        85,
        51,
        2,
        5,
        122,
        82
      ],
      "accounts": [
        {
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "params.mint"
              }
            ]
          }
        },
        {
          "name": "chainConfigOverride",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103,
                  95,
                  111,
                  118,
                  101,
                  114,
                  114,
                  105,
                  100,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "params.remoteChainSelector"
              },
              {
                "kind": "arg",
                "path": "params.mint"
              },
              {
                "kind": "arg",
                "path": "params.overrideType"
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
              "name": "deleteChainConfigOverrideParams"
            }
          }
        }
      ]
    },
    {
      "name": "deriveAccountsLockOrBurnTokens",
      "docs": [
        "Called by the router via CPI during `DeriveAccountsCcipSend` to discover",
        "any extra accounts needed for the onramp token transfer.",
        "Lockrelease pools have no additional account requirements beyond the",
        "standard token lookup table, so this returns an empty set."
      ],
      "discriminator": [
        20,
        237,
        184,
        4,
        166,
        153,
        108,
        174
      ],
      "accounts": [],
      "args": [
        {
          "name": "stage",
          "type": "string"
        },
        {
          "name": "lockOrBurn",
          "type": {
            "defined": {
              "name": "lockOrBurnInV1"
            }
          }
        }
      ],
      "returns": {
        "defined": {
          "name": "deriveAccountsResponse"
        }
      }
    },
    {
      "name": "deriveAccountsReleaseOrMintTokens",
      "docs": [
        "Called by the offramp via CPI during `DeriveAccountsExecute` to discover",
        "any extra accounts needed for the offramp token transfer.",
        "Lockrelease pools have no additional account requirements beyond the",
        "standard token lookup table, so this returns an empty set."
      ],
      "discriminator": [
        40,
        91,
        244,
        228,
        0,
        2,
        51,
        238
      ],
      "accounts": [],
      "args": [
        {
          "name": "stage",
          "type": "string"
        },
        {
          "name": "releaseOrMint",
          "type": {
            "defined": {
              "name": "releaseOrMintInV1"
            }
          }
        }
      ],
      "returns": {
        "defined": {
          "name": "deriveAccountsResponse"
        }
      }
    },
    {
      "name": "editChainConfigV2",
      "discriminator": [
        118,
        24,
        17,
        167,
        67,
        206,
        107,
        213
      ],
      "accounts": [
        {
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "params.mint"
              }
            ]
          }
        },
        {
          "name": "chainConfigV2",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103,
                  95,
                  50,
                  95,
                  48
                ]
              },
              {
                "kind": "arg",
                "path": "params.remoteChainSelector"
              },
              {
                "kind": "account",
                "path": "state.config.mint",
                "account": "state"
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
          "name": "program"
        }
      ],
      "args": [
        {
          "name": "params",
          "type": {
            "defined": {
              "name": "setChainConfigV2Params"
            }
          }
        }
      ]
    },
    {
      "name": "editChainRemoteConfig",
      "docs": [
        "Overwrites the remote config (token address and pool addresses) for an",
        "existing ChainConfig. Reallocates the PDA if the new config is larger."
      ],
      "discriminator": [
        149,
        112,
        186,
        72,
        116,
        217,
        159,
        175
      ],
      "accounts": [
        {
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "chainConfig",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "remoteChainSelector"
              },
              {
                "kind": "arg",
                "path": "mint"
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
        }
      ],
      "args": [
        {
          "name": "remoteChainSelector",
          "type": "u64"
        },
        {
          "name": "mint",
          "type": "pubkey"
        },
        {
          "name": "cfg",
          "type": {
            "defined": {
              "name": "remoteConfig"
            }
          }
        }
      ]
    },
    {
      "name": "getPoolFee",
      "docs": [
        "Returns the fee receipt for a token transfer."
      ],
      "discriminator": [
        168,
        225,
        198,
        232,
        103,
        224,
        41,
        65
      ],
      "accounts": [
        {
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "getFeeIn.localToken"
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
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "getFeeIn.remoteChainSelector"
              },
              {
                "kind": "account",
                "path": "state.config.mint",
                "account": "state"
              }
            ]
          }
        },
        {
          "name": "chainConfigV2",
          "docs": [
            "May or may not be initialized"
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103,
                  95,
                  50,
                  95,
                  48
                ]
              },
              {
                "kind": "arg",
                "path": "getFeeIn.remoteChainSelector"
              },
              {
                "kind": "account",
                "path": "state.config.mint",
                "account": "state"
              }
            ]
          }
        }
      ],
      "args": [
        {
          "name": "getFeeIn",
          "type": {
            "defined": {
              "name": "getFeeParams"
            }
          }
        }
      ],
      "returns": {
        "defined": {
          "name": "getFeeResult"
        }
      }
    },
    {
      "name": "getRequiredCcvs",
      "discriminator": [
        233,
        78,
        203,
        142,
        12,
        113,
        96,
        99
      ],
      "accounts": [
        {
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "getRequiredCcvsIn.localToken"
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
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "getRequiredCcvsIn.remoteChainSelector"
              },
              {
                "kind": "account",
                "path": "state.config.mint",
                "account": "state"
              }
            ]
          }
        },
        {
          "name": "chainConfigV2",
          "docs": [
            "When initialized (owner == this program)"
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103,
                  95,
                  50,
                  95,
                  48
                ]
              },
              {
                "kind": "arg",
                "path": "getRequiredCcvsIn.remoteChainSelector"
              },
              {
                "kind": "account",
                "path": "state.config.mint",
                "account": "state"
              }
            ]
          }
        }
      ],
      "args": [
        {
          "name": "getRequiredCcvsIn",
          "type": {
            "defined": {
              "name": "getRequiredCcVsIn"
            }
          }
        }
      ],
      "returns": {
        "defined": {
          "name": "getRequiredCcVsOut"
        }
      }
    },
    {
      "name": "initChainConfigOverride",
      "docs": [
        "Initializes a ChainConfigOverride PDA for the given remote chain and override type.",
        "Used to set separate rate limit buckets (e.g. for faster-than-finality transfers)."
      ],
      "discriminator": [
        196,
        118,
        243,
        229,
        244,
        18,
        234,
        9
      ],
      "accounts": [
        {
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "params.mint"
              }
            ]
          }
        },
        {
          "name": "chainConfigOverride",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103,
                  95,
                  111,
                  118,
                  101,
                  114,
                  114,
                  105,
                  100,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "params.remoteChainSelector"
              },
              {
                "kind": "arg",
                "path": "params.mint"
              },
              {
                "kind": "arg",
                "path": "params.overrideType"
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
          "name": "program"
        }
      ],
      "args": [
        {
          "name": "params",
          "type": {
            "defined": {
              "name": "initChainConfigOverrideParams"
            }
          }
        }
      ]
    },
    {
      "name": "initChainConfigV2",
      "discriminator": [
        31,
        189,
        231,
        179,
        25,
        226,
        94,
        90
      ],
      "accounts": [
        {
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "params.mint"
              }
            ]
          }
        },
        {
          "name": "chainConfigV2",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103,
                  95,
                  50,
                  95,
                  48
                ]
              },
              {
                "kind": "arg",
                "path": "params.remoteChainSelector"
              },
              {
                "kind": "account",
                "path": "state.config.mint",
                "account": "state"
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
          "name": "mint",
          "docs": [
            "The token this pool wraps. Needed to create the fee vault ATA the lane's in-token BPS fee",
            "accrues into."
          ]
        },
        {
          "name": "feeVault",
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
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "feeVaultTokenAccount",
          "docs": [
            "The fee vault's ATA for the pool's own mint, where `lock_or_burn_tokens_v2` accrues the",
            "in-token BPS fee. Created here so that configuring a",
            "lane and being able to collect (and withdraw) its BPS fee are the same operation."
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
                "path": "tokenProgram"
              },
              {
                "kind": "account",
                "path": "mint"
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
          "name": "tokenProgram"
        },
        {
          "name": "associatedTokenProgram",
          "address": "ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL"
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
              "name": "setChainConfigV2Params"
            }
          }
        }
      ]
    },
    {
      "name": "initChainRemoteConfig",
      "docs": [
        "Initializes a ChainConfig PDA for the given remote chain.",
        "Must be called before any cross-chain messages can be sent to or from",
        "that chain. Remote pool addresses are added separately via",
        "append_remote_pool_addresses."
      ],
      "discriminator": [
        21,
        150,
        133,
        36,
        2,
        116,
        199,
        129
      ],
      "accounts": [
        {
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "chainConfig",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "remoteChainSelector"
              },
              {
                "kind": "arg",
                "path": "mint"
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
        }
      ],
      "args": [
        {
          "name": "remoteChainSelector",
          "type": "u64"
        },
        {
          "name": "mint",
          "type": "pubkey"
        },
        {
          "name": "cfg",
          "type": {
            "defined": {
              "name": "remoteConfig"
            }
          }
        }
      ]
    },
    {
      "name": "initGlobalConfig",
      "docs": [
        "Initializes the program-level PoolConfig PDA.",
        "Must be called once by the program's upgrade authority before any",
        "token pool can be created."
      ],
      "discriminator": [
        140,
        136,
        214,
        48,
        87,
        0,
        120,
        255
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
          "name": "program",
          "address": "8eqh8wppT9c5rw4ERqNCffvU6cNFJWff9WmkcYtmGiqC"
        },
        {
          "name": "programData"
        }
      ],
      "args": [
        {
          "name": "routerAddress",
          "type": "pubkey"
        },
        {
          "name": "rmnAddress",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "initialize",
      "docs": [
        "Creates a new per-mint State PDA for the given mint.",
        "The upgrade authority (or a mint authority when self-serve is enabled)",
        "becomes the pool owner."
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
          "name": "state",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "mint"
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
          "name": "program",
          "address": "8eqh8wppT9c5rw4ERqNCffvU6cNFJWff9WmkcYtmGiqC"
        },
        {
          "name": "programData"
        },
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
      "args": []
    },
    {
      "name": "initializeByMultisig",
      "docs": [
        "Creates a new per-mint State PDA for the given mint via a mint authority multisig signer.",
        "Self-serve must be enabled. Only 1-of-N multisigs are supported."
      ],
      "discriminator": [
        170,
        42,
        240,
        9,
        144,
        13,
        204,
        211
      ],
      "accounts": [
        {
          "name": "state",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "mint"
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
          "name": "program",
          "address": "8eqh8wppT9c5rw4ERqNCffvU6cNFJWff9WmkcYtmGiqC"
        },
        {
          "name": "programData"
        },
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
          "name": "mintAuthorityMultisig",
          "docs": [
            "Only 1-of-N SPL Token multisigs are supported here."
          ]
        }
      ],
      "args": []
    },
    {
      "name": "lockOrBurnTokens",
      "docs": [
        "Locks tokens from the sender's account as part of an outbound",
        "cross-chain transfer. Called by the router via CPI after the onramp",
        "has validated the message."
      ],
      "discriminator": [
        114,
        161,
        94,
        29,
        147,
        25,
        232,
        191
      ],
      "accounts": [
        {
          "name": "authority",
          "signer": true
        },
        {
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "tokenProgram"
        },
        {
          "name": "mint",
          "writable": true
        },
        {
          "name": "poolSigner",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  115,
                  105,
                  103,
                  110,
                  101,
                  114
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "poolTokenAccount",
          "writable": true
        },
        {
          "name": "rmnRemote"
        },
        {
          "name": "rmnRemoteCurses"
        },
        {
          "name": "rmnRemoteConfig"
        },
        {
          "name": "chainConfig",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "lockOrBurn.remoteChainSelector"
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        }
      ],
      "args": [
        {
          "name": "lockOrBurn",
          "type": {
            "defined": {
              "name": "lockOrBurnInV1"
            }
          }
        }
      ],
      "returns": {
        "defined": {
          "name": "lockOrBurnOutV1"
        }
      }
    },
    {
      "name": "lockOrBurnTokensV2",
      "docs": [
        "Locks tokens in the pool as part of an outbound cross-chain transfer (V2)."
      ],
      "discriminator": [
        89,
        200,
        103,
        53,
        14,
        249,
        196,
        197
      ],
      "accounts": [
        {
          "name": "authority",
          "signer": true
        },
        {
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "tokenProgram"
        },
        {
          "name": "mint",
          "writable": true
        },
        {
          "name": "poolSigner",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  115,
                  105,
                  103,
                  110,
                  101,
                  114
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "poolTokenAccount",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "account",
                "path": "poolSigner"
              },
              {
                "kind": "account",
                "path": "tokenProgram"
              },
              {
                "kind": "account",
                "path": "mint"
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
          "name": "chainConfig",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "lockOrBurn.remoteChainSelector"
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "chainConfigV2",
          "docs": [
            "May or may not be initialized; read via `ChainConfigV2::try_load`, defaulting to",
            "`WAIT_FOR_FINALITY` (full-finality only) when absent. The router supplies this account for",
            "every v2 pool transfer, and its position (between `chain_config` and `chain_config_override`)",
            "must mirror the router's `transfer_token_v2` CPI account order and the resolver's",
            "`accounts_to_save` order."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103,
                  95,
                  50,
                  95,
                  48
                ]
              },
              {
                "kind": "arg",
                "path": "lockOrBurn.remoteChainSelector"
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "chainConfigOverride",
          "docs": [
            "When initialized (owner == this program), its rate limit buckets",
            "override the default ones for faster-than-finality transfers."
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103,
                  95,
                  111,
                  118,
                  101,
                  114,
                  114,
                  105,
                  100,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "lockOrBurn.remoteChainSelector"
              },
              {
                "kind": "account",
                "path": "mint"
              },
              {
                "kind": "const",
                "value": [
                  102,
                  116,
                  102
                ]
              }
            ]
          }
        },
        {
          "name": "feeVaultTokenAccount",
          "docs": [
            "token-pool receipt fees into. In-token BPS fees accrue here until the owner withdraws them",
            "with `withdraw_fee_tokens`; keeping them here rather than in `pool_token_account` is what",
            "makes fees distinguishable from the locked liquidity. The vault PDA is derived inline rather",
            "than passed in: it holds no data and never signs here, and every account this instruction",
            "takes costs 32 bytes of a `ccip_send` transaction that is already close to the size limit."
          ],
          "writable": true
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
          "name": "lockOrBurn",
          "type": {
            "defined": {
              "name": "lockOrBurnInV2"
            }
          }
        }
      ],
      "returns": {
        "defined": {
          "name": "lockOrBurnOutV2"
        }
      }
    },
    {
      "name": "nestedResolveAccountsStart",
      "docs": [
        "Resolves the accounts needed for V2 lock_or_burn or release_or_mint",
        "instructions based on the instruction discriminator."
      ],
      "discriminator": [
        123,
        100,
        128,
        5,
        46,
        162,
        23,
        210
      ],
      "accounts": [
        {
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "account",
                "path": "state.config.mint",
                "account": "state"
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
              "name": "nestedResolveAccountsParams"
            }
          }
        }
      ],
      "returns": {
        "defined": {
          "name": "nestedResolveAccountsResponse"
        }
      }
    },
    {
      "name": "observeChainConfig",
      "docs": [
        "Returns a pool's configuration for a single remote chain, for observability consumers.",
        "Rate limiter buckets are reported as of now, i.e. including the refill accrued since the",
        "last transfer.",
        "",
        "Permissionless and read-only: meant to be executed through `simulateTransaction`. Fails if",
        "the lane has no chain config, which callers can read as \"not configured\"."
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
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "remoteChainSelector"
              },
              {
                "kind": "arg",
                "path": "mint"
              }
            ]
          }
        }
      ],
      "args": [
        {
          "name": "remoteChainSelector",
          "type": "u64"
        },
        {
          "name": "mint",
          "type": "pubkey"
        }
      ],
      "returns": {
        "defined": {
          "name": "poolChainConfigObservation"
        }
      }
    },
    {
      "name": "observeChainConfigOverride",
      "docs": [
        "Returns a pool's rate limit override for a single remote chain and override type, for",
        "observability consumers. Rate limiter buckets are reported as of now, i.e. including the",
        "refill accrued since the last transfer.",
        "",
        "Permissionless and read-only: meant to be executed through `simulateTransaction`. Fails if",
        "the lane has no override of that type, which callers can read as \"not overridden\"."
      ],
      "discriminator": [
        108,
        241,
        121,
        187,
        172,
        68,
        220,
        214
      ],
      "accounts": [
        {
          "name": "chainConfigOverride",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103,
                  95,
                  111,
                  118,
                  101,
                  114,
                  114,
                  105,
                  100,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "remoteChainSelector"
              },
              {
                "kind": "arg",
                "path": "mint"
              },
              {
                "kind": "arg",
                "path": "overrideType"
              }
            ]
          }
        }
      ],
      "args": [
        {
          "name": "remoteChainSelector",
          "type": "u64"
        },
        {
          "name": "mint",
          "type": "pubkey"
        },
        {
          "name": "overrideType",
          "type": {
            "defined": {
              "name": "poolChainConfigOverrideType"
            }
          }
        }
      ],
      "returns": {
        "defined": {
          "name": "poolChainConfigOverrideObservation"
        }
      }
    },
    {
      "name": "observeChainConfigV2",
      "docs": [
        "Returns a pool's CCIP 2.0 CCV configuration for a single remote chain, for observability",
        "consumers.",
        "",
        "Permissionless and read-only: meant to be executed through `simulateTransaction`. Fails if",
        "the lane has no CCV config, which callers can read as \"not configured\"."
      ],
      "discriminator": [
        88,
        88,
        179,
        204,
        234,
        156,
        14,
        55
      ],
      "accounts": [
        {
          "name": "chainConfigV2",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103,
                  95,
                  50,
                  95,
                  48
                ]
              },
              {
                "kind": "arg",
                "path": "remoteChainSelector"
              },
              {
                "kind": "arg",
                "path": "mint"
              }
            ]
          }
        }
      ],
      "args": [
        {
          "name": "remoteChainSelector",
          "type": "u64"
        },
        {
          "name": "mint",
          "type": "pubkey"
        }
      ],
      "returns": {
        "defined": {
          "name": "poolChainConfigV2Observation"
        }
      }
    },
    {
      "name": "observeState",
      "docs": [
        "Returns a pool's per-mint configuration, for observability consumers.",
        "",
        "Permissionless and read-only: meant to be executed through `simulateTransaction`. The",
        "allow list is reported as a count only."
      ],
      "discriminator": [
        168,
        76,
        245,
        226,
        87,
        123,
        209,
        216
      ],
      "accounts": [
        {
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "mint"
              }
            ]
          }
        }
      ],
      "args": [
        {
          "name": "mint",
          "type": "pubkey"
        }
      ],
      "returns": {
        "defined": {
          "name": "poolStateObservation"
        }
      }
    },
    {
      "name": "provideLiquidity",
      "docs": [
        "Rebalancer adds liquidity to the pool's token account."
      ],
      "discriminator": [
        40,
        110,
        107,
        116,
        174,
        127,
        97,
        204
      ],
      "accounts": [
        {
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "tokenProgram"
        },
        {
          "name": "mint",
          "writable": true
        },
        {
          "name": "poolSigner",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  115,
                  105,
                  103,
                  110,
                  101,
                  114
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "poolTokenAccount",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "account",
                "path": "poolSigner"
              },
              {
                "kind": "account",
                "path": "tokenProgram"
              },
              {
                "kind": "account",
                "path": "mint"
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
          "name": "remoteTokenAccount",
          "writable": true
        },
        {
          "name": "authority",
          "signer": true
        }
      ],
      "args": [
        {
          "name": "amount",
          "type": "u64"
        }
      ]
    },
    {
      "name": "releaseOrMintTokens",
      "docs": [
        "Releases tokens to the receiver's account as part of an inbound",
        "cross-chain transfer. Called by the offramp via CPI after the",
        "message has been verified and committed."
      ],
      "discriminator": [
        92,
        100,
        150,
        198,
        252,
        63,
        164,
        228
      ],
      "accounts": [
        {
          "name": "authority",
          "signer": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  101,
                  120,
                  116,
                  101,
                  114,
                  110,
                  97,
                  108,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  95,
                  112,
                  111,
                  111,
                  108,
                  115,
                  95,
                  115,
                  105,
                  103,
                  110,
                  101,
                  114
                ]
              },
              {
                "kind": "const",
                "value": [
                  113,
                  179,
                  90,
                  202,
                  245,
                  125,
                  141,
                  246,
                  8,
                  132,
                  0,
                  73,
                  169,
                  26,
                  164,
                  68,
                  173,
                  167,
                  244,
                  230,
                  168,
                  217,
                  242,
                  187,
                  176,
                  247,
                  109,
                  115,
                  172,
                  116,
                  227,
                  231
                ]
              }
            ],
            "program": {
              "kind": "account",
              "path": "offrampProgram"
            }
          }
        },
        {
          "name": "offrampProgram",
          "docs": [
            "CHECK offramp program: exists only to derive the allowed offramp PDA",
            "and the authority PDA."
          ]
        },
        {
          "name": "allowedOfframp",
          "docs": [
            "CHECK PDA of the router program verifying the signer is an allowed offramp.",
            "If PDA does not exist, the router doesn't allow this offramp"
          ]
        },
        {
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "tokenProgram"
        },
        {
          "name": "mint",
          "writable": true
        },
        {
          "name": "poolSigner",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  115,
                  105,
                  103,
                  110,
                  101,
                  114
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "poolTokenAccount",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "account",
                "path": "poolSigner"
              },
              {
                "kind": "account",
                "path": "tokenProgram"
              },
              {
                "kind": "account",
                "path": "mint"
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
          "name": "chainConfig",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "releaseOrMint.remoteChainSelector"
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "rmnRemote"
        },
        {
          "name": "rmnRemoteCurses"
        },
        {
          "name": "rmnRemoteConfig"
        },
        {
          "name": "receiverTokenAccount",
          "writable": true
        }
      ],
      "args": [
        {
          "name": "releaseOrMint",
          "type": {
            "defined": {
              "name": "releaseOrMintInV1"
            }
          }
        }
      ],
      "returns": {
        "defined": {
          "name": "releaseOrMintOutV1"
        }
      }
    },
    {
      "name": "releaseOrMintTokensV2",
      "docs": [
        "Releases tokens to the receiver as part of an inbound cross-chain transfer (V2)."
      ],
      "discriminator": [
        149,
        216,
        178,
        227,
        89,
        213,
        105,
        186
      ],
      "accounts": [
        {
          "name": "authority",
          "signer": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  101,
                  120,
                  116,
                  101,
                  114,
                  110,
                  97,
                  108,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  95,
                  112,
                  111,
                  111,
                  108,
                  115,
                  95,
                  115,
                  105,
                  103,
                  110,
                  101,
                  114
                ]
              },
              {
                "kind": "const",
                "value": [
                  113,
                  179,
                  90,
                  202,
                  245,
                  125,
                  141,
                  246,
                  8,
                  132,
                  0,
                  73,
                  169,
                  26,
                  164,
                  68,
                  173,
                  167,
                  244,
                  230,
                  168,
                  217,
                  242,
                  187,
                  176,
                  247,
                  109,
                  115,
                  172,
                  116,
                  227,
                  231
                ]
              }
            ],
            "program": {
              "kind": "account",
              "path": "offrampProgram"
            }
          }
        },
        {
          "name": "offrampProgram",
          "docs": [
            "CHECK offramp program: exists only to derive the allowed offramp PDA",
            "and the authority PDA."
          ]
        },
        {
          "name": "allowedOfframp",
          "docs": [
            "CHECK PDA of the router program verifying the signer is an allowed offramp.",
            "If PDA does not exist, the router doesn't allow this offramp"
          ]
        },
        {
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "tokenProgram"
        },
        {
          "name": "mint",
          "writable": true
        },
        {
          "name": "poolSigner",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  115,
                  105,
                  103,
                  110,
                  101,
                  114
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "poolTokenAccount",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "account",
                "path": "poolSigner"
              },
              {
                "kind": "account",
                "path": "tokenProgram"
              },
              {
                "kind": "account",
                "path": "mint"
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
          "name": "chainConfig",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "releaseOrMint.remoteChainSelector"
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "chainConfigOverride",
          "docs": [
            "When initialized (owner == this program), its rate limit buckets",
            "override the default ones for faster-than-finality transfers."
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103,
                  95,
                  111,
                  118,
                  101,
                  114,
                  114,
                  105,
                  100,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "releaseOrMint.remoteChainSelector"
              },
              {
                "kind": "account",
                "path": "mint"
              },
              {
                "kind": "const",
                "value": [
                  102,
                  116,
                  102
                ]
              }
            ]
          }
        },
        {
          "name": "receiverTokenAccount",
          "writable": true
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
          "name": "releaseOrMint",
          "type": {
            "defined": {
              "name": "releaseOrMintInV2"
            }
          }
        }
      ],
      "returns": {
        "defined": {
          "name": "releaseOrMintOutV2"
        }
      }
    },
    {
      "name": "removeFromAllowList",
      "docs": [
        "Removes addresses from the allow-list."
      ],
      "discriminator": [
        44,
        46,
        123,
        213,
        40,
        11,
        107,
        18
      ],
      "accounts": [
        {
          "name": "state",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "mint"
        },
        {
          "name": "authority",
          "writable": true,
          "signer": true
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "remove",
          "type": {
            "vec": "pubkey"
          }
        }
      ]
    },
    {
      "name": "setChainConfigOverrideRateLimit",
      "docs": [
        "Updates the rate limits on a ChainConfigOverride PDA.",
        "Callable by the pool owner or the rate-limit admin."
      ],
      "discriminator": [
        179,
        114,
        216,
        195,
        164,
        146,
        235,
        58
      ],
      "accounts": [
        {
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "params.mint"
              }
            ]
          }
        },
        {
          "name": "chainConfigOverride",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103,
                  95,
                  111,
                  118,
                  101,
                  114,
                  114,
                  105,
                  100,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "params.remoteChainSelector"
              },
              {
                "kind": "arg",
                "path": "params.mint"
              },
              {
                "kind": "arg",
                "path": "params.overrideType"
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
              "name": "setChainConfigOverrideRateLimitParams"
            }
          }
        }
      ]
    },
    {
      "name": "setChainRateLimit",
      "docs": [
        "Updates the inbound and outbound rate limits for a specific remote chain.",
        "Callable by either the pool owner or the rate-limit admin."
      ],
      "discriminator": [
        188,
        188,
        161,
        37,
        100,
        249,
        123,
        170
      ],
      "accounts": [
        {
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "chainConfig",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "remoteChainSelector"
              },
              {
                "kind": "arg",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "authority",
          "writable": true,
          "signer": true
        }
      ],
      "args": [
        {
          "name": "remoteChainSelector",
          "type": "u64"
        },
        {
          "name": "mint",
          "type": "pubkey"
        },
        {
          "name": "inbound",
          "type": {
            "defined": {
              "name": "rateLimitConfig"
            }
          }
        },
        {
          "name": "outbound",
          "type": {
            "defined": {
              "name": "rateLimitConfig"
            }
          }
        }
      ]
    },
    {
      "name": "setRateLimitAdmin",
      "docs": [
        "Sets the rate-limit admin for a token pool.",
        "The rate-limit admin may call set_chain_rate_limit without being the pool owner."
      ],
      "discriminator": [
        21,
        182,
        126,
        128,
        31,
        241,
        37,
        34
      ],
      "accounts": [
        {
          "name": "state",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "arg",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "authority",
          "writable": true,
          "signer": true
        }
      ],
      "args": [
        {
          "name": "mint",
          "type": "pubkey"
        },
        {
          "name": "newRateLimitAdmin",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "setRebalancer",
      "docs": [
        "Sets the rebalancer address for the pool.",
        "If the rebalancer is the default address then no rebalance is allowed."
      ],
      "discriminator": [
        28,
        161,
        168,
        250,
        215,
        4,
        144,
        154
      ],
      "accounts": [
        {
          "name": "state",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "mint"
        },
        {
          "name": "authority",
          "signer": true
        }
      ],
      "args": [
        {
          "name": "rebalancer",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "setRmn",
      "docs": [
        "Updates the RMN Remote address on an individual token pool's State.",
        "Only callable by the upgrade authority if they also own the pool."
      ],
      "discriminator": [
        252,
        89,
        60,
        179,
        198,
        54,
        169,
        120
      ],
      "accounts": [
        {
          "name": "state",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "mint"
        },
        {
          "name": "authority",
          "writable": true,
          "signer": true
        },
        {
          "name": "program",
          "address": "8eqh8wppT9c5rw4ERqNCffvU6cNFJWff9WmkcYtmGiqC"
        },
        {
          "name": "programData"
        }
      ],
      "args": [
        {
          "name": "rmnAddress",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "setRouter",
      "docs": [
        "Updates the router address on an individual token pool's State.",
        "Changes the expected signer for lock/burn and release/mint calls.",
        "Only callable by the upgrade authority if they also own the pool."
      ],
      "discriminator": [
        236,
        248,
        107,
        200,
        151,
        160,
        44,
        250
      ],
      "accounts": [
        {
          "name": "state",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "mint"
        },
        {
          "name": "authority",
          "writable": true,
          "signer": true
        },
        {
          "name": "program",
          "address": "8eqh8wppT9c5rw4ERqNCffvU6cNFJWff9WmkcYtmGiqC"
        },
        {
          "name": "programData"
        }
      ],
      "args": [
        {
          "name": "newRouter",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "transferOwnership",
      "docs": [
        "Initiates an ownership transfer for this token pool to a new proposed owner.",
        "The proposed owner must call accept_ownership to complete the handover."
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
          "name": "state",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "mint"
        },
        {
          "name": "authority",
          "signer": true
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
        "Returns the program type (name) and version string encoded at build time.",
        "Offchain code calls this to confirm which program and version is deployed",
        "before sending any other instruction."
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
      "accounts": [],
      "args": [],
      "returns": "string"
    },
    {
      "name": "updateDefaultRmn",
      "docs": [
        "Updates the default RMN Remote address stored in PoolConfig.",
        "New token pools created after this call will use the new RMN Remote."
      ],
      "discriminator": [
        204,
        186,
        36,
        125,
        180,
        133,
        227,
        162
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
          "name": "program",
          "address": "8eqh8wppT9c5rw4ERqNCffvU6cNFJWff9WmkcYtmGiqC"
        },
        {
          "name": "programData"
        }
      ],
      "args": [
        {
          "name": "rmnAddress",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "updateDefaultRouter",
      "docs": [
        "Updates the default router address stored in PoolConfig.",
        "New token pools created after this call will use the new router."
      ],
      "discriminator": [
        29,
        86,
        6,
        222,
        73,
        220,
        6,
        186
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
          "name": "program",
          "address": "8eqh8wppT9c5rw4ERqNCffvU6cNFJWff9WmkcYtmGiqC"
        },
        {
          "name": "programData"
        }
      ],
      "args": [
        {
          "name": "routerAddress",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "updateSelfServedAllowed",
      "docs": [
        "Enables or disables self-served pool initialization.",
        "When enabled, any mint authority may create a pool for their mint",
        "without requiring the program upgrade authority."
      ],
      "discriminator": [
        210,
        165,
        57,
        132,
        64,
        203,
        100,
        73
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
          "name": "program",
          "address": "8eqh8wppT9c5rw4ERqNCffvU6cNFJWff9WmkcYtmGiqC"
        },
        {
          "name": "programData"
        }
      ],
      "args": [
        {
          "name": "selfServedAllowed",
          "type": "bool"
        }
      ]
    },
    {
      "name": "withdrawFeeTokens",
      "docs": [
        "Sweeps the fees this pool accrued for `mint` in a single fee token into a recipient token",
        "account of the pool owner's choosing. Restricted to the pool owner.",
        "",
        "If `amount` is provided, only the requested amount is transferred. Otherwise (if None) the",
        "entire balance is transferred."
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
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "mint",
          "docs": [
            "The token this pool wraps. Unrelated to `fee_token_mint`, but part of the fee vault seeds."
          ]
        },
        {
          "name": "feeVault",
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
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "feeVaultTokenAccount",
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
          "name": "recipient",
          "docs": [
            "Where the fees go. Freely chosen by the pool owner, unlike the executor and the CCVs, which",
            "pin the destination to a `fee_aggregator` held in their config."
          ],
          "writable": true
        },
        {
          "name": "feeTokenMint",
          "docs": [
            "The token the fees were billed in, which is whatever the sender paid with, so a pool",
            "accrues one vault ATA per fee token it has ever been paid in."
          ]
        },
        {
          "name": "feeTokenProgram"
        },
        {
          "name": "authority",
          "signer": true
        }
      ],
      "args": [
        {
          "name": "amount",
          "type": {
            "option": "u64"
          }
        }
      ]
    },
    {
      "name": "withdrawLiquidity",
      "docs": [
        "Rebalancer withdraws liquidity from the pool's token account.",
        "Can be used to transfer liquidity from one pool to another by setting",
        "the `rebalancer` to the calling pool."
      ],
      "discriminator": [
        149,
        158,
        33,
        185,
        47,
        243,
        253,
        31
      ],
      "accounts": [
        {
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  99,
                  111,
                  110,
                  102,
                  105,
                  103
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "tokenProgram"
        },
        {
          "name": "mint",
          "writable": true
        },
        {
          "name": "poolSigner",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  105,
                  112,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110,
                  112,
                  111,
                  111,
                  108,
                  95,
                  115,
                  105,
                  103,
                  110,
                  101,
                  114
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ]
          }
        },
        {
          "name": "poolTokenAccount",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "account",
                "path": "poolSigner"
              },
              {
                "kind": "account",
                "path": "tokenProgram"
              },
              {
                "kind": "account",
                "path": "mint"
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
          "name": "remoteTokenAccount",
          "writable": true
        },
        {
          "name": "authority",
          "signer": true
        }
      ],
      "args": [
        {
          "name": "amount",
          "type": "u64"
        }
      ]
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
      "name": "chainConfigOverride",
      "discriminator": [
        129,
        84,
        107,
        120,
        82,
        243,
        19,
        22
      ]
    },
    {
      "name": "chainConfigV2",
      "discriminator": [
        63,
        128,
        8,
        247,
        39,
        249,
        249,
        204
      ]
    },
    {
      "name": "poolConfig",
      "discriminator": [
        26,
        108,
        14,
        123,
        116,
        230,
        129,
        43
      ]
    },
    {
      "name": "state",
      "discriminator": [
        216,
        146,
        107,
        94,
        104,
        75,
        182,
        177
      ]
    }
  ],
  "types": [
    {
      "name": "baseChain",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "remote",
            "type": {
              "defined": {
                "name": "remoteConfig"
              }
            }
          },
          {
            "name": "inboundRateLimit",
            "type": {
              "defined": {
                "name": "rateLimitTokenBucket"
              }
            }
          },
          {
            "name": "outboundRateLimit",
            "type": {
              "defined": {
                "name": "rateLimitTokenBucket"
              }
            }
          }
        ]
      }
    },
    {
      "name": "baseConfig",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "tokenProgram",
            "type": "pubkey"
          },
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "decimals",
            "type": "u8"
          },
          {
            "name": "poolSigner",
            "type": "pubkey"
          },
          {
            "name": "poolTokenAccount",
            "type": "pubkey"
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
            "name": "rateLimitAdmin",
            "type": "pubkey"
          },
          {
            "name": "routerOnrampAuthority",
            "type": "pubkey"
          },
          {
            "name": "router",
            "type": "pubkey"
          },
          {
            "name": "rebalancer",
            "docs": [
              "Set the rebalancer address for the pool; if set to the default \"no rebalance\" address",
              "(Pubkey::default()), no rebalance is allowed."
            ],
            "type": "pubkey"
          },
          {
            "name": "canAcceptLiquidity",
            "docs": [
              "DEPRECATED: this field is no longer enforced at runtime. It is kept for backward",
              "compatibility (no PDA migration) and is always initialized to `true`.",
              "To disable inflows operationally, set the rebalancer to `Pubkey::default()`."
            ],
            "type": "bool"
          },
          {
            "name": "listEnabled",
            "type": "bool"
          },
          {
            "name": "allowList",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "rmnRemote",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "baseOverrideConfig",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "inboundRateLimit",
            "type": {
              "defined": {
                "name": "rateLimitTokenBucket"
              }
            }
          },
          {
            "name": "outboundRateLimit",
            "type": {
              "defined": {
                "name": "rateLimitTokenBucket"
              }
            }
          },
          {
            "name": "overrideType",
            "type": {
              "defined": {
                "name": "poolChainConfigOverrideType"
              }
            }
          }
        ]
      }
    },
    {
      "name": "ccvConfig",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "thresholdAmount",
            "type": "u64"
          },
          {
            "name": "outboundCcvs",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "thresholdOutboundCcvs",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "inboundCcvs",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "thresholdInboundCcvs",
            "type": {
              "vec": "pubkey"
            }
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
            "name": "base",
            "type": {
              "defined": {
                "name": "baseChain"
              }
            }
          }
        ]
      }
    },
    {
      "name": "chainConfigOverride",
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
            "name": "base",
            "type": {
              "defined": {
                "name": "baseOverrideConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "chainConfigV2",
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
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "allowedFinalityConfig",
            "docs": [
              "Allowed finality policy for *outbound* (lock_or_burn) transfers on this lane.",
              "Mirrors EVM's pool-wide `s_allowedFinalityConfig`. Zero-initialized",
              "(`WAIT_FOR_FINALITY`) means only full-finality transfers are permitted."
            ],
            "type": {
              "defined": {
                "name": "finalityConfig"
              }
            }
          },
          {
            "name": "tokenTransferFeeConfig",
            "type": {
              "defined": {
                "name": "tokenTransferFeeConfig"
              }
            }
          },
          {
            "name": "ccvConfig",
            "type": {
              "defined": {
                "name": "ccvConfig"
              }
            }
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
      "name": "deleteChainConfigOverrideParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "overrideType",
            "type": {
              "defined": {
                "name": "poolChainConfigOverrideType"
              }
            }
          }
        ]
      }
    },
    {
      "name": "deriveAccountsResponse",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "askAgainWith",
            "type": {
              "vec": {
                "defined": {
                  "name": "baseTokenPoolV1CcipAccountMeta"
                }
              }
            }
          },
          {
            "name": "accountsToSave",
            "type": {
              "vec": {
                "defined": {
                  "name": "baseTokenPoolV1CcipAccountMeta"
                }
              }
            }
          },
          {
            "name": "lookUpTablesToSave",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "currentStage",
            "type": "string"
          },
          {
            "name": "nextStage",
            "type": "string"
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
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "amount",
            "type": "u64"
          },
          {
            "name": "localToken",
            "type": "pubkey"
          },
          {
            "name": "feeToken",
            "type": "pubkey"
          },
          {
            "name": "requestedFinalityConfig",
            "type": {
              "defined": {
                "name": "finalityConfig"
              }
            }
          },
          {
            "name": "tokenArgs",
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "getFeeResult",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destGasLimit",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "destBytesOverhead",
            "type": "u32"
          },
          {
            "name": "fee",
            "type": {
              "defined": {
                "name": "usdCents"
              }
            }
          },
          {
            "name": "tokenFeeBps",
            "type": "u16"
          },
          {
            "name": "isEnabled",
            "type": "bool"
          }
        ]
      }
    },
    {
      "name": "getRequiredCcVsIn",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "amount",
            "docs": [
              "Token amount used to determine the required CCVs.",
              "",
              "For inbound transfers, the amount is denominated in the remote token's decimals",
              "and must be converted to local token decimals before applying amount-based CCV",
              "policies. For outbound transfers, the amount is already denominated in local",
              "token decimals, but is the gross amount before the pool's in-token BPS fee: the",
              "pool deducts that fee before applying amount-based CCV policies, as it only sends",
              "the post-fee remainder cross-chain. Inbound amounts are already post-fee."
            ],
            "type": {
              "defined": {
                "name": "protocolAmount"
              }
            }
          },
          {
            "name": "localToken",
            "type": "pubkey"
          },
          {
            "name": "direction",
            "type": {
              "defined": {
                "name": "messageDirection"
              }
            }
          },
          {
            "name": "requestedFinalityConfig",
            "type": {
              "defined": {
                "name": "finalityConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "getRequiredCcVsOut",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "requiredCcvs",
            "type": {
              "vec": "pubkey"
            }
          }
        ]
      }
    },
    {
      "name": "initChainConfigOverrideParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "overrideType",
            "type": {
              "defined": {
                "name": "poolChainConfigOverrideType"
              }
            }
          },
          {
            "name": "inbound",
            "type": {
              "defined": {
                "name": "rateLimitConfig"
              }
            }
          },
          {
            "name": "outbound",
            "type": {
              "defined": {
                "name": "rateLimitConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "lockOrBurnInV1",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "receiver",
            "type": "bytes"
          },
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "originalSender",
            "type": "pubkey"
          },
          {
            "name": "amount",
            "type": "u64"
          },
          {
            "name": "localToken",
            "type": "pubkey"
          },
          {
            "name": "msgTotalNonce",
            "docs": [
              "DEPRECATED. The onramp's full nonce for this message, keyed by `original_sender` and",
              "`remote_chain_selector`. Still populated with a real value by the 1.6 onramp",
              "(`ccip-router`'s `instructions_v1/v1/onramp.rs`), but hard-coded to `0` by `ccip_send_v2`,",
              "which no longer tracks per-sender nonces.",
              "",
              "The only pool that reads it is `cctp-token-pool`, which uses it as a PDA seed for the",
              "`cctp_message_sent_event` account (see that program's `lib.rs` and `derive.rs`). A constant",
              "`0` would therefore collide across messages — so the `0` is only sound because a CCTP v1 pool",
              "can never be reached from `ccip_send_v2`:",
              "",
              "- CCTP and Lombard are the only v1 pools that set `implements_account_resolution`.",
              "- `TokenTransferContext::new` (the router's `instructions/common.rs`) verifies the",
              "`TokenAdminRegistry` PDA and then rejects `interface_version == 1` &&",
              "`implements_account_resolution` with `UnsupportedPoolV1ResolutionOnLaneV2`. That runs in",
              "`ccip_send_v2` before any pool CPI, so it is the authoritative gate; the account-resolution",
              "stage (`ccip_send/resolve_accounts/token_transfer_static.rs`) rejects the same combination",
              "earlier, but is only advisory since callers can build the transaction themselves.",
              "- `set_pool_interface_implementation` additionally refuses to register",
              "`implements_account_resolution` below `interface_version` 2. The runtime check still",
              "matters because registries migrated from 1.6 can carry the v1/resolution combination.",
              "",
              "Removing the field outright is a breaking change to the v1 pool interface, so it stays until",
              "the 1.6 lane is retired. Anything that starts consuming it must not assume it is meaningful",
              "on the v2 lane."
            ],
            "type": "u64"
          }
        ]
      }
    },
    {
      "name": "lockOrBurnInV2",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "receiver",
            "type": "bytes"
          },
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "originalSender",
            "type": "pubkey"
          },
          {
            "name": "amount",
            "type": "u64"
          },
          {
            "name": "localToken",
            "type": "pubkey"
          },
          {
            "name": "requestedFinalityConfig",
            "type": {
              "defined": {
                "name": "finalityConfig"
              }
            }
          },
          {
            "name": "tokenArgs",
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "lockOrBurnOutV1",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destTokenAddress",
            "type": {
              "defined": {
                "name": "remoteAddress"
              }
            }
          },
          {
            "name": "destPoolData",
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "lockOrBurnOutV2",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destTokenAddress",
            "type": {
              "defined": {
                "name": "remoteAddress"
              }
            }
          },
          {
            "name": "destPoolData",
            "type": "bytes"
          },
          {
            "name": "destTokenAmount",
            "type": "u64"
          }
        ]
      }
    },
    {
      "name": "messageDirection",
      "docs": [
        "Direction of a cross-chain message, used for CCV requirements."
      ],
      "type": {
        "kind": "enum",
        "variants": [
          {
            "name": "outbound"
          },
          {
            "name": "inbound"
          }
        ]
      }
    },
    {
      "name": "nestedResolveAccountsParams",
      "docs": [
        "Resolution parameters used by nested resolvers. Rather than the full ix_data and a metadata",
        "field, they have an opaque `cpi_resolution_inputs` field that doubles as both. The caller",
        "of the nested resolution is responsible for correctly encoding it by consulting",
        "the documentation."
      ],
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
            "name": "callerProgram",
            "docs": [
              "The program ID that derived `caller` and is driving this resolution (the router on the",
              "send side, the offramp on the execute side)."
            ],
            "type": "pubkey"
          },
          {
            "name": "cpiResolutionInputs",
            "docs": [
              "Will be deserialized into a type the nested resolver understands. For what to",
              "send in the first stage of nested resolution, consult the nested resolver's documentation."
            ],
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "nestedResolveAccountsResponse",
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
                  "name": "ccipCommonResolutionCcipAccountMeta"
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
            "name": "nextCpiResolutionInputs",
            "docs": [
              "If nonempty, perform another resolution stage with these inputs."
            ],
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "poolChainConfigObservation",
      "docs": [
        "Per-remote-chain configuration of a token pool, as observed by off-chain consumers."
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
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "remoteTokenAddress",
            "docs": [
              "Token address on the remote chain, as raw bytes."
            ],
            "type": "bytes"
          },
          {
            "name": "remoteDecimals",
            "type": "u8"
          },
          {
            "name": "remotePoolAddresses",
            "docs": [
              "Pool addresses on the remote chain, as raw bytes. More than one entry means earlier pool",
              "versions are still accepted as sources."
            ],
            "type": {
              "vec": "bytes"
            }
          },
          {
            "name": "inboundRateLimit",
            "type": {
              "defined": {
                "name": "rateLimiterObservation"
              }
            }
          },
          {
            "name": "outboundRateLimit",
            "type": {
              "defined": {
                "name": "rateLimiterObservation"
              }
            }
          }
        ]
      }
    },
    {
      "name": "poolChainConfigOverrideObservation",
      "docs": [
        "Per-remote-chain, per-override-type rate limit override of a token pool, as observed by",
        "off-chain consumers.",
        "",
        "Override PDAs are optional: when a lane has no override of the requested type, the observing",
        "instruction fails, which callers can read as \"not overridden\"."
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
            "type": "u64"
          },
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "overrideType",
            "docs": [
              "Which flavour of transfer this override applies to (e.g. fast transfers)."
            ],
            "type": {
              "defined": {
                "name": "poolChainConfigOverrideType"
              }
            }
          },
          {
            "name": "inboundRateLimit",
            "type": {
              "defined": {
                "name": "rateLimiterObservation"
              }
            }
          },
          {
            "name": "outboundRateLimit",
            "type": {
              "defined": {
                "name": "rateLimiterObservation"
              }
            }
          }
        ]
      }
    },
    {
      "name": "poolChainConfigOverrideType",
      "type": {
        "kind": "enum",
        "variants": [
          {
            "name": "ftf"
          }
        ]
      }
    },
    {
      "name": "poolChainConfigV2Observation",
      "docs": [
        "Per-remote-chain CCIP 2.0 configuration of a token pool, as observed by off-chain consumers."
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
            "type": "u64"
          },
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "thresholdAmount",
            "docs": [
              "Transfer amount above which the threshold CCVs are additionally required. Zero disables the",
              "threshold lists."
            ],
            "type": "u64"
          },
          {
            "name": "outboundCcvs",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "thresholdOutboundCcvs",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "inboundCcvs",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "thresholdInboundCcvs",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "allowedFinalityConfig",
            "docs": [
              "Allowed finality policy for outbound (lock_or_burn) transfers on this lane."
            ],
            "type": {
              "defined": {
                "name": "finalityConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "poolConfig",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "version",
            "type": "u8"
          },
          {
            "name": "selfServedAllowed",
            "type": "bool"
          },
          {
            "name": "router",
            "type": "pubkey"
          },
          {
            "name": "rmnRemote",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "poolStateObservation",
      "docs": [
        "Per-mint configuration of a token pool, as observed by off-chain consumers.",
        "",
        "The allow list is reported as a count only, as its size is unbounded by configuration."
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
            "name": "tokenProgram",
            "type": "pubkey"
          },
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "decimals",
            "type": "u8"
          },
          {
            "name": "poolSigner",
            "type": "pubkey"
          },
          {
            "name": "poolTokenAccount",
            "type": "pubkey"
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
            "name": "rateLimitAdmin",
            "type": "pubkey"
          },
          {
            "name": "router",
            "type": "pubkey"
          },
          {
            "name": "routerOnrampAuthority",
            "type": "pubkey"
          },
          {
            "name": "rmnRemote",
            "type": "pubkey"
          },
          {
            "name": "rebalancer",
            "docs": [
              "Rebalancer, only meaningful for lock/release pools. The default pubkey means rebalancing is",
              "not allowed."
            ],
            "type": "pubkey"
          },
          {
            "name": "listEnabled",
            "type": "bool"
          },
          {
            "name": "allowListCount",
            "type": "u32"
          }
        ]
      }
    },
    {
      "name": "protocolAmount",
      "docs": [
        "CCIP 2.0 compatible cross-chain amount representation (u256 in big-endian bytes, based",
        "on abi.encodePacked)"
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "beBytes",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          }
        ]
      }
    },
    {
      "name": "rateLimitConfig",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "enabled",
            "type": "bool"
          },
          {
            "name": "capacity",
            "type": "u64"
          },
          {
            "name": "rate",
            "type": "u64"
          }
        ]
      }
    },
    {
      "name": "rateLimitTokenBucket",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "tokens",
            "type": "u64"
          },
          {
            "name": "lastUpdated",
            "type": "u64"
          },
          {
            "name": "cfg",
            "type": {
              "defined": {
                "name": "rateLimitConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "rateLimiterObservation",
      "docs": [
        "State of one rate limiter bucket, as of the moment of observation."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "isEnabled",
            "type": "bool"
          },
          {
            "name": "capacity",
            "type": "u64"
          },
          {
            "name": "rate",
            "type": "u64"
          },
          {
            "name": "tokens",
            "docs": [
              "Tokens available now: the stored balance plus the refill accrued since the bucket was last",
              "touched, capped at `capacity`. This is the value EVM's `getCurrent*RateLimiterState`",
              "returns, not the raw stored balance."
            ],
            "type": "u64"
          },
          {
            "name": "lastUpdated",
            "docs": [
              "Timestamp the `tokens` value corresponds to (the observation time, for an enabled bucket)."
            ],
            "type": "u64"
          }
        ]
      }
    },
    {
      "name": "releaseOrMintInV1",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "originalSender",
            "type": {
              "defined": {
                "name": "remoteAddress"
              }
            }
          },
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "receiver",
            "type": "pubkey"
          },
          {
            "name": "amount",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          },
          {
            "name": "localToken",
            "type": "pubkey"
          },
          {
            "name": "sourcePoolAddress",
            "docs": [
              "@dev WARNING: sourcePoolAddress should be checked prior to any processing of funds. Make sure it matches the",
              "expected pool address for the given remoteChainSelector."
            ],
            "type": {
              "defined": {
                "name": "remoteAddress"
              }
            }
          },
          {
            "name": "sourcePoolData",
            "type": "bytes"
          },
          {
            "name": "offchainTokenData",
            "docs": [
              "@dev WARNING: offchainTokenData is untrusted data."
            ],
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "releaseOrMintInV2",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "originalSender",
            "type": {
              "defined": {
                "name": "remoteAddress"
              }
            }
          },
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "receiver",
            "type": "pubkey"
          },
          {
            "name": "amount",
            "type": {
              "defined": {
                "name": "protocolAmount"
              }
            }
          },
          {
            "name": "localToken",
            "type": "pubkey"
          },
          {
            "name": "sourcePoolAddress",
            "type": {
              "defined": {
                "name": "remoteAddress"
              }
            }
          },
          {
            "name": "sourcePoolData",
            "type": "bytes"
          },
          {
            "name": "requestedFinalityConfig",
            "type": {
              "defined": {
                "name": "finalityConfig"
              }
            }
          },
          {
            "name": "receiverUsage",
            "type": {
              "defined": {
                "name": "svmTokenReceiverUsage"
              }
            }
          }
        ]
      }
    },
    {
      "name": "releaseOrMintOutV1",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destinationAmount",
            "type": "u64"
          }
        ]
      }
    },
    {
      "name": "releaseOrMintOutV2",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destinationAmount",
            "type": "u64"
          }
        ]
      }
    },
    {
      "name": "remoteAddress",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "address",
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "remoteConfig",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "poolAddresses",
            "type": {
              "vec": {
                "defined": {
                  "name": "remoteAddress"
                }
              }
            }
          },
          {
            "name": "tokenAddress",
            "type": {
              "defined": {
                "name": "remoteAddress"
              }
            }
          },
          {
            "name": "decimals",
            "type": "u8"
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
      "name": "setChainConfigOverrideRateLimitParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "overrideType",
            "type": {
              "defined": {
                "name": "poolChainConfigOverrideType"
              }
            }
          },
          {
            "name": "inbound",
            "type": {
              "defined": {
                "name": "rateLimitConfig"
              }
            }
          },
          {
            "name": "outbound",
            "type": {
              "defined": {
                "name": "rateLimitConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "setChainConfigV2Params",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "allowedFinalityConfig",
            "docs": [
              "Allowed finality policy for outbound (lock_or_burn) transfers on this lane.",
              "`WAIT_FOR_FINALITY` (zero) permits only full-finality transfers."
            ],
            "type": {
              "defined": {
                "name": "finalityConfig"
              }
            }
          },
          {
            "name": "tokenTransferFeeConfig",
            "type": {
              "defined": {
                "name": "tokenTransferFeeConfig"
              }
            }
          },
          {
            "name": "ccvConfig",
            "type": {
              "defined": {
                "name": "ccvConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "state",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "version",
            "type": "u8"
          },
          {
            "name": "config",
            "type": {
              "defined": {
                "name": "baseConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "tokenTransferFeeConfig",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destGasOverhead",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "destBytesOverhead",
            "type": "u32"
          },
          {
            "name": "finalityFee",
            "type": {
              "defined": {
                "name": "usdCents"
              }
            }
          },
          {
            "name": "fastFinalityFee",
            "type": {
              "defined": {
                "name": "usdCents"
              }
            }
          },
          {
            "name": "finalityBpsFee",
            "type": "u16"
          },
          {
            "name": "fastFinalityBpsFee",
            "type": "u16"
          },
          {
            "name": "isEnabled",
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
    },
    {
      "name": "baseTokenPoolV1CcipAccountMeta",
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
      "name": "ccipCommonResolutionCcipAccountMeta",
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
    }
  ]
};
