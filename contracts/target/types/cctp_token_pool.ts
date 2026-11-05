/**
 * Program IDL in camelCase format in order to be used in JS/TS.
 *
 * Note that this is only a type helper and is not the actual IDL. The original
 * IDL can be found at `target/idl/cctp_token_pool.json`.
 */
export type CctpTokenPool = {
  "address": "CCiTPESGEevd7TBU8EGBKrcxuRq7jx3YtW6tPidnscaZ",
  "metadata": {
    "name": "cctpTokenPool",
    "version": "2.0.0",
    "spec": "0.1.0",
    "description": "USDC Token Pool for CCIP utilizing CCTP"
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
      "name": "acceptVerifierOwnership",
      "discriminator": [
        238,
        214,
        210,
        64,
        157,
        109,
        238,
        253
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
      "name": "acceptVerifierStorageLocationAdmin",
      "discriminator": [
        233,
        170,
        224,
        96,
        16,
        226,
        99,
        31
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
        "any extra accounts needed for the onramp token transfer, of which there",
        "are many over more than one round derivation."
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
        "any extra accounts needed for the offramp token transfer, of which there",
        "are many over more than one round derivation."
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
      "name": "editChainRemoteConfigCctp",
      "docs": [
        "Edits remote config values that are specific to CCTP."
      ],
      "discriminator": [
        5,
        215,
        79,
        193,
        234,
        35,
        15,
        87
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
          "name": "cfg",
          "type": {
            "defined": {
              "name": "cctpChain"
            }
          }
        }
      ]
    },
    {
      "name": "editVerifierChainConfig",
      "docs": [
        "Overwrites the admin configuration of an already configured remote chain, leaving its",
        "allowlist and CCTP configuration untouched."
      ],
      "discriminator": [
        145,
        58,
        54,
        9,
        222,
        53,
        100,
        195
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
          "signer": true
        },
        {
          "name": "remoteChainConfig",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  118,
                  101,
                  114,
                  105,
                  102,
                  105,
                  101,
                  114,
                  95,
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
              "name": "setVerifierChainConfigParams"
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
                "kind": "account",
                "path": "state.config.mint",
                "account": "state"
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
      "docs": [
        "This implementation returns the CctpVerifier only, for any message"
      ],
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
      "name": "getVerifierFee",
      "discriminator": [
        221,
        143,
        116,
        69,
        217,
        1,
        200,
        250
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
          "signer": true
        },
        {
          "name": "remoteChainConfig",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  118,
                  101,
                  114,
                  105,
                  102,
                  105,
                  101,
                  114,
                  95,
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
        }
      ],
      "args": [
        {
          "name": "params",
          "type": {
            "defined": {
              "name": "getVerifierFeeParams"
            }
          }
        }
      ],
      "returns": {
        "defined": {
          "name": "getVerifierFeeResult"
        }
      }
    },
    {
      "name": "getVerifierStorageLocations",
      "discriminator": [
        49,
        192,
        201,
        197,
        90,
        110,
        37,
        0
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
          "name": "storageLocations",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  115,
                  116,
                  111,
                  114,
                  97,
                  103,
                  101,
                  95,
                  108,
                  111,
                  99,
                  97,
                  116,
                  105,
                  111,
                  110,
                  115
                ]
              }
            ]
          }
        }
      ],
      "args": [],
      "returns": {
        "vec": {
          "defined": {
            "name": "byteString"
          }
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
      "name": "initVerifierChainConfig",
      "docs": [
        "Creates the verifier configuration for a remote chain and sets its admin configuration.",
        "Fails if the remote chain is already configured; use `edit_verifier_chain_config` instead."
      ],
      "discriminator": [
        123,
        31,
        71,
        236,
        46,
        210,
        39,
        76
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
          "name": "remoteChainConfig",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  118,
                  101,
                  114,
                  105,
                  102,
                  105,
                  101,
                  114,
                  95,
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
              "name": "setVerifierChainConfigParams"
            }
          }
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
          "address": "CCiTPESGEevd7TBU8EGBKrcxuRq7jx3YtW6tPidnscaZ"
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
      "args": [
        {
          "name": "router",
          "type": "pubkey"
        },
        {
          "name": "rmnRemote",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "initializeVerifier",
      "discriminator": [
        94,
        212,
        44,
        130,
        71,
        242,
        204,
        155
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
          "name": "storageLocations",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  115,
                  116,
                  111,
                  114,
                  97,
                  103,
                  101,
                  95,
                  108,
                  111,
                  99,
                  97,
                  116,
                  105,
                  111,
                  110,
                  115
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
          "address": "CCiTPESGEevd7TBU8EGBKrcxuRq7jx3YtW6tPidnscaZ"
        },
        {
          "name": "programData"
        }
      ],
      "args": []
    },
    {
      "name": "lockOrBurnTokens",
      "docs": [
        "Burns tokens from the sender's account as part of an outbound",
        "cross-chain transfer. Called by the router via CPI after the onramp",
        "has validated the message. Relies on CCTPv1 to do the actual burning."
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
          "docs": [
            "hold a balance to pay for the rent of initializing the CCTP MessageSentEvent account"
          ],
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
        },
        {
          "name": "cctpMessageTransmitterAccount",
          "docs": [
            "CHECK this is not read by the pool, just forwarded to CCTP"
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  109,
                  101,
                  115,
                  115,
                  97,
                  103,
                  101,
                  95,
                  116,
                  114,
                  97,
                  110,
                  115,
                  109,
                  105,
                  116,
                  116,
                  101,
                  114
                ]
              }
            ],
            "program": {
              "kind": "account",
              "path": "cctpMessageTransmitter"
            }
          }
        },
        {
          "name": "cctpTokenMessengerMinter",
          "docs": [
            "CHECK this is CCTP's TokenMessengerMinter program, which",
            "is invoked by this program."
          ],
          "address": "CCTPiPYPc6AsJuwueEnWgSgucamXDZwBd53dQ11YiKX3"
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        },
        {
          "name": "cctpMessageTransmitter",
          "docs": [
            "CHECK this is CCTP's MessageTransmitter program, which",
            "is invoked transitively by CCTP's TokenMessengerMinter,",
            "which in turn is invoked explicitly by this program."
          ],
          "address": "CCTPmbSD7gX1bxKPAmg77w8oFzNFpaQiQUWD43TKaecd"
        },
        {
          "name": "cctpTokenMessengerAccount",
          "docs": [
            "CHECK this is not read by the pool, just forwarded to CCTP"
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  116,
                  111,
                  107,
                  101,
                  110,
                  95,
                  109,
                  101,
                  115,
                  115,
                  101,
                  110,
                  103,
                  101,
                  114
                ]
              }
            ],
            "program": {
              "kind": "account",
              "path": "cctpTokenMessengerMinter"
            }
          }
        },
        {
          "name": "cctpTokenMinterAccount",
          "docs": [
            "CHECK this is not read by the pool, just forwarded to CCTP"
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  116,
                  111,
                  107,
                  101,
                  110,
                  95,
                  109,
                  105,
                  110,
                  116,
                  101,
                  114
                ]
              }
            ],
            "program": {
              "kind": "account",
              "path": "cctpTokenMessengerMinter"
            }
          }
        },
        {
          "name": "cctpLocalToken",
          "docs": [
            "CHECK this is not read by the pool, just forwarded to CCTP"
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  108,
                  111,
                  99,
                  97,
                  108,
                  95,
                  116,
                  111,
                  107,
                  101,
                  110
                ]
              },
              {
                "kind": "account",
                "path": "mint"
              }
            ],
            "program": {
              "kind": "account",
              "path": "cctpTokenMessengerMinter"
            }
          }
        },
        {
          "name": "cctpEventAuthority",
          "docs": [
            "CHECK this is not read by the pool, just forwarded to CCTP"
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  95,
                  95,
                  101,
                  118,
                  101,
                  110,
                  116,
                  95,
                  97,
                  117,
                  116,
                  104,
                  111,
                  114,
                  105,
                  116,
                  121
                ]
              }
            ],
            "program": {
              "kind": "account",
              "path": "cctpTokenMessengerMinter"
            }
          }
        },
        {
          "name": "cctpAuthorityPda",
          "docs": [
            "CHECK this is not read by the pool, just forwarded to CCTP"
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  115,
                  101,
                  110,
                  100,
                  101,
                  114,
                  95,
                  97,
                  117,
                  116,
                  104,
                  111,
                  114,
                  105,
                  116,
                  121
                ]
              }
            ],
            "program": {
              "kind": "account",
              "path": "cctpTokenMessengerMinter"
            }
          }
        },
        {
          "name": "cctpRemoteTokenMessengerKey",
          "docs": [
            "CHECK this is not read by the pool, just forwarded to CCTP"
          ],
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
                  116,
                  111,
                  107,
                  101,
                  110,
                  95,
                  109,
                  101,
                  115,
                  115,
                  101,
                  110,
                  103,
                  101,
                  114
                ]
              },
              {
                "kind": "account",
                "path": "chainConfig.cctp.domainId",
                "account": "chainConfig"
              }
            ],
            "program": {
              "kind": "account",
              "path": "cctpTokenMessengerMinter"
            }
          }
        },
        {
          "name": "cctpMessageSentEvent",
          "docs": [
            "CHECK this is the account in which CCTP will store the event. It is not a PDA of CCTP,",
            "but CCTP will initialize it and become the owner for it."
          ],
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
                  99,
                  99,
                  116,
                  112,
                  95,
                  109,
                  101,
                  115,
                  115,
                  97,
                  103,
                  101,
                  95,
                  115,
                  101,
                  110,
                  116,
                  95,
                  101,
                  118,
                  101,
                  110,
                  116
                ]
              },
              {
                "kind": "arg",
                "path": "lockOrBurn.originalSender"
              },
              {
                "kind": "arg",
                "path": "lockOrBurn.remoteChainSelector"
              },
              {
                "kind": "arg",
                "path": "lockOrBurn.msgTotalNonce"
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
        "Burns tokens from the pool as part of an outbound cross-chain transfer (V2)."
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
        "Resolves the accounts needed for nested CPI instructions based on the instruction discriminator.",
        "Includes verifier & poolV2 methods for onramp, offramp, and getting fees.",
        "To be called by the router & offramp programs as part of their account resolution methods."
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
          "name": "poolStateOrVerifierConfig",
          "docs": [
            "`VerifierConfig`, per the module doc above. Whichever account resolution is being asked",
            "about (pool or verifier role) determines which one the caller passes in."
          ]
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
      "name": "reclaimEventAccount",
      "docs": [
        "Closes an old CCTPv1 event PDA and reclaims its rent."
      ],
      "discriminator": [
        94,
        198,
        180,
        159,
        131,
        236,
        15,
        174
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
          "name": "poolSigner",
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
                  115,
                  105,
                  103,
                  110,
                  101,
                  114
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
          "name": "messageSentEventAccount",
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
                  99,
                  99,
                  116,
                  112,
                  95,
                  109,
                  101,
                  115,
                  115,
                  97,
                  103,
                  101,
                  95,
                  115,
                  101,
                  110,
                  116,
                  95,
                  101,
                  118,
                  101,
                  110,
                  116
                ]
              },
              {
                "kind": "arg",
                "path": "originalSender"
              },
              {
                "kind": "arg",
                "path": "remoteChainSelector"
              },
              {
                "kind": "arg",
                "path": "msgNonce"
              }
            ]
          }
        },
        {
          "name": "messageTransmitter",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  109,
                  101,
                  115,
                  115,
                  97,
                  103,
                  101,
                  95,
                  116,
                  114,
                  97,
                  110,
                  115,
                  109,
                  105,
                  116,
                  116,
                  101,
                  114
                ]
              }
            ],
            "program": {
              "kind": "account",
              "path": "cctpMessageTransmitter"
            }
          }
        },
        {
          "name": "cctpMessageTransmitter",
          "address": "CCTPmbSD7gX1bxKPAmg77w8oFzNFpaQiQUWD43TKaecd"
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
          "name": "mint",
          "type": "pubkey"
        },
        {
          "name": "originalSender",
          "type": "pubkey"
        },
        {
          "name": "remoteChainSelector",
          "type": "u64"
        },
        {
          "name": "msgNonce",
          "type": "u64"
        },
        {
          "name": "attestation",
          "type": "bytes"
        }
      ]
    },
    {
      "name": "reclaimFunds",
      "docs": [
        "Returns an amount of SOL from the pool signer account to the designated",
        "fund reclaimer. There are three entities involved:",
        "",
        "* `owner`: can configure the reclaimer and fund manager.",
        "* `fund_manager`: can execute this instruction.",
        "* `fund_reclaim_destination`: receives the funds.",
        "",
        "The resulting funds on the PDA cannot drop below `minimum_signer_funds`."
      ],
      "discriminator": [
        38,
        246,
        147,
        248,
        43,
        41,
        43,
        198
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
          "name": "mint"
        },
        {
          "name": "poolSigner",
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
          "name": "fundReclaimDestination",
          "docs": [
            "to be a particular fund reclaimer"
          ],
          "writable": true
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
          "name": "amount",
          "type": "u64"
        }
      ]
    },
    {
      "name": "reclaimVerifierEventAccountV2",
      "docs": [
        "Closes an old CCTPv2 event PDA and reclaims its rent. Circle imposes a 5-day cooldown",
        "(since the message was originally sent) before it can be reclaimed."
      ],
      "discriminator": [
        38,
        19,
        166,
        15,
        113,
        126,
        254,
        209
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
          "name": "cpiSigner",
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
                  115,
                  105,
                  103,
                  110,
                  101,
                  114
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
          "name": "messageSentEventAccount",
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
                  99,
                  99,
                  116,
                  112,
                  118,
                  50,
                  95,
                  101,
                  118,
                  101,
                  110,
                  116
                ]
              },
              {
                "kind": "arg",
                "path": "params.originalSender"
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
                "path": "params.ccipCctpNonce"
              }
            ]
          }
        },
        {
          "name": "messageTransmitterPda",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  109,
                  101,
                  115,
                  115,
                  97,
                  103,
                  101,
                  95,
                  116,
                  114,
                  97,
                  110,
                  115,
                  109,
                  105,
                  116,
                  116,
                  101,
                  114
                ]
              }
            ],
            "program": {
              "kind": "account",
              "path": "messageTransmitterProgram"
            }
          }
        },
        {
          "name": "messageTransmitterProgram",
          "address": "CCTPV2Sm4AdWt5296sk4P66VBZ7bEhcARwFaaS9YPbeC"
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
              "name": "reclaimVerifierEventAccountParams"
            }
          }
        }
      ]
    },
    {
      "name": "releaseOrMintTokens",
      "docs": [
        "Mints tokens to the receiver's account as part of an inbound",
        "cross-chain transfer. Called by the offramp via CPI after the",
        "message has been verified and committed. Relies in CCTPv1 to do the actual minting."
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
                  166,
                  112,
                  209,
                  122,
                  155,
                  183,
                  57,
                  204,
                  13,
                  239,
                  112,
                  70,
                  98,
                  90,
                  15,
                  97,
                  207,
                  14,
                  15,
                  184,
                  57,
                  226,
                  59,
                  137,
                  107,
                  66,
                  45,
                  89,
                  64,
                  63,
                  93,
                  230
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
        "Mints tokens to the receiver as part of an inbound cross-chain transfer (V2)."
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
                  166,
                  112,
                  209,
                  122,
                  155,
                  183,
                  57,
                  204,
                  13,
                  239,
                  112,
                  70,
                  98,
                  90,
                  15,
                  97,
                  207,
                  14,
                  15,
                  184,
                  57,
                  226,
                  59,
                  137,
                  107,
                  66,
                  45,
                  89,
                  64,
                  63,
                  93,
                  230
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
      "name": "setFundManager",
      "docs": [
        "Sets a new authority who can reclaim funds and close old event PDAs."
      ],
      "discriminator": [
        120,
        5,
        18,
        143,
        165,
        103,
        181,
        139
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
          "name": "fundManager",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "setFundReclaimDestination",
      "docs": [
        "Sets a new destination to which SOL can be transferred (withdrawn) from the pool's signer account."
      ],
      "discriminator": [
        81,
        6,
        71,
        218,
        85,
        141,
        95,
        162
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
          "name": "fundReclaimDestination",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "setMinimumSignerFunds",
      "docs": [
        "Sets a threshold for the minimum amount of SOL that must remain in the pool's signer account.",
        "When withdrawing SOL the pool signer account cannot drop below this threshold."
      ],
      "discriminator": [
        142,
        36,
        82,
        253,
        191,
        111,
        114,
        53
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
          "name": "amount",
          "type": "u64"
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
          "address": "CCiTPESGEevd7TBU8EGBKrcxuRq7jx3YtW6tPidnscaZ"
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
          "address": "CCiTPESGEevd7TBU8EGBKrcxuRq7jx3YtW6tPidnscaZ"
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
      "name": "setVerifierAllowedFinalityConfig",
      "discriminator": [
        199,
        3,
        49,
        252,
        213,
        16,
        178,
        199
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
          "name": "finality",
          "type": {
            "defined": {
              "name": "finalityConfig"
            }
          }
        }
      ]
    },
    {
      "name": "setVerifierAllowlist",
      "discriminator": [
        126,
        154,
        186,
        208,
        1,
        238,
        131,
        193
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
          "docs": [
            "The owner or the allowlist admin, matching EVM `applyAllowlistUpdates`."
          ],
          "writable": true,
          "signer": true
        },
        {
          "name": "remoteChainConfig",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  118,
                  101,
                  114,
                  105,
                  102,
                  105,
                  101,
                  114,
                  95,
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
              "name": "setVerifierAllowlistParams"
            }
          }
        }
      ]
    },
    {
      "name": "setVerifierAllowlistAdmin",
      "discriminator": [
        7,
        233,
        111,
        174,
        67,
        164,
        203,
        19
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
          "name": "allowlistAdmin",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "setVerifierChainConfigCctp",
      "discriminator": [
        148,
        175,
        243,
        102,
        23,
        169,
        221,
        182
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
          "name": "remoteChainConfig",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  118,
                  101,
                  114,
                  105,
                  102,
                  105,
                  101,
                  114,
                  95,
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
              "name": "setVerifierChainConfigCctpParams"
            }
          }
        }
      ]
    },
    {
      "name": "setVerifierFastFinalityBps",
      "discriminator": [
        206,
        108,
        248,
        138,
        44,
        120,
        212,
        160
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
          "name": "fastFinalityBps",
          "type": "u16"
        }
      ]
    },
    {
      "name": "setVerifierFeeAggregator",
      "discriminator": [
        20,
        53,
        192,
        190,
        11,
        215,
        87,
        50
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
          "name": "feeAggregator",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "setVerifierStorageLocations",
      "discriminator": [
        43,
        175,
        255,
        108,
        243,
        16,
        245,
        137
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
          "name": "storageLocations",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  115,
                  116,
                  111,
                  114,
                  97,
                  103,
                  101,
                  95,
                  108,
                  111,
                  99,
                  97,
                  116,
                  105,
                  111,
                  110,
                  115
                ]
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
              "name": "setVerifierStorageLocationsParams"
            }
          }
        }
      ]
    },
    {
      "name": "startVerificationAtSource",
      "discriminator": [
        214,
        46,
        173,
        20,
        219,
        225,
        143,
        56
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
          "signer": true
        },
        {
          "name": "remoteChainConfig",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  118,
                  101,
                  114,
                  105,
                  102,
                  105,
                  101,
                  114,
                  95,
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
                "path": "params.message.destChainSelector"
              }
            ]
          }
        },
        {
          "name": "mint",
          "writable": true
        },
        {
          "name": "tokenProgram"
        },
        {
          "name": "cpiSigner",
          "docs": [
            "hold a balance to pay for the rent of initializing the CCTP MessageSentEvent account.",
            "This is intentionally reusing the signer from the pool module of this same contract, so as",
            "to keep SOL on a single PDA and a single owner across CCTPv1 & CCTPv2 event accounts."
          ],
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
          "name": "nonce",
          "writable": true
        },
        {
          "name": "cctpMessageSentEvent",
          "docs": [
            "CHECK this is the account in which CCTP will store the event. It is not a PDA of CCTP,",
            "but CCTP will initialize it and become the owner for it. It's address is checked in the instruction body."
          ],
          "writable": true
        },
        {
          "name": "burnTokenAccount",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "account",
                "path": "cpiSigner"
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
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        },
        {
          "name": "senderAuthorityPda"
        },
        {
          "name": "denylistAccount"
        },
        {
          "name": "messageTransmitter"
        },
        {
          "name": "tokenMessenger"
        },
        {
          "name": "remoteTokenMessenger"
        },
        {
          "name": "tokenMinter"
        },
        {
          "name": "localToken"
        },
        {
          "name": "tokenMessengerMinterEventAuthority"
        },
        {
          "name": "messageTransmitterEventAuthority"
        },
        {
          "name": "messageTransmitterProgram",
          "address": "CCTPV2Sm4AdWt5296sk4P66VBZ7bEhcARwFaaS9YPbeC"
        },
        {
          "name": "tokenMessengerMinterProgram",
          "address": "CCTPV2vPZJS2u2BBsUoscuikbYjnpFmbFsvVuJdgUMQe"
        }
      ],
      "args": [
        {
          "name": "params",
          "type": {
            "defined": {
              "name": "startVerificationAtSourceParams"
            }
          }
        }
      ],
      "returns": "bytes"
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
      "name": "transferVerifierOwnership",
      "discriminator": [
        204,
        8,
        95,
        140,
        81,
        79,
        174,
        254
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
      "name": "transferVerifierStorageLocationAdmin",
      "discriminator": [
        193,
        224,
        66,
        74,
        123,
        139,
        28,
        31
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
          "name": "proposedStorageLocationAdmin",
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
      "name": "verify",
      "discriminator": [
        133,
        161,
        141,
        48,
        120,
        198,
        88,
        150
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
          "name": "remoteChainConfig"
        },
        {
          "name": "mint",
          "relations": [
            "endReceiverTokenAccount"
          ]
        },
        {
          "name": "cpiSigner",
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
                "path": "cpiSigner"
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
          "name": "endReceiverTokenAccount",
          "writable": true
        },
        {
          "name": "tokenProgram"
        },
        {
          "name": "authorityPda"
        },
        {
          "name": "messageTransmitter"
        },
        {
          "name": "usedNonce",
          "writable": true
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        },
        {
          "name": "tokenMessenger"
        },
        {
          "name": "remoteTokenMessenger"
        },
        {
          "name": "tokenMinter"
        },
        {
          "name": "localToken",
          "writable": true
        },
        {
          "name": "tokenPair"
        },
        {
          "name": "feeRecipientAta",
          "writable": true
        },
        {
          "name": "custodyTokenAccount",
          "writable": true
        },
        {
          "name": "messageTransmitterEventAuthority"
        },
        {
          "name": "messageTransmitterProgram",
          "address": "CCTPV2Sm4AdWt5296sk4P66VBZ7bEhcARwFaaS9YPbeC"
        },
        {
          "name": "tokenMessengerMinterEventAuthority"
        },
        {
          "name": "tokenMessengerMinterProgram",
          "address": "CCTPV2vPZJS2u2BBsUoscuikbYjnpFmbFsvVuJdgUMQe"
        },
        {
          "name": "authority",
          "signer": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  118,
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
                  166,
                  112,
                  209,
                  122,
                  155,
                  183,
                  57,
                  204,
                  13,
                  239,
                  112,
                  70,
                  98,
                  90,
                  15,
                  97,
                  207,
                  14,
                  15,
                  184,
                  57,
                  226,
                  59,
                  137,
                  107,
                  66,
                  45,
                  89,
                  64,
                  63,
                  93,
                  230
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
        }
      ],
      "args": [
        {
          "name": "params",
          "type": {
            "defined": {
              "name": "verifyParams"
            }
          }
        }
      ]
    },
    {
      "name": "withdrawPoolFeeTokens",
      "docs": [
        "Sweeps the fees this pool accrued for `mint` in a single fee token into a recipient token",
        "account of the pool owner's choosing. Restricted to the pool owner.",
        "",
        "If `amount` is provided, only the requested amount is transferred. Otherwise (if None) the",
        "entire balance is transferred."
      ],
      "discriminator": [
        210,
        30,
        78,
        149,
        25,
        222,
        158,
        106
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
      "name": "withdrawVerifierFeeTokens",
      "docs": [
        "Sweeps this verifier's accrued fees for one fee token into the fee aggregator's ATA.",
        "",
        "Permissionless: the destination is pinned to `config.fee_aggregator`."
      ],
      "discriminator": [
        109,
        117,
        135,
        195,
        179,
        57,
        25,
        14
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
            "The account the router pays this verifier's fees into: the ATA of `fee_vault` for the fee",
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
                "path": "tokenProgram"
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
            "The router derives the destination of every CCV receipt fee from these exact seeds (see",
            "`ccip_common::seed::vault_ata`), so this is the only authority that can move the accrued",
            "fees out."
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
                "path": "tokenProgram"
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
          "name": "tokenProgram"
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
    },
    {
      "name": "nonce",
      "discriminator": [
        143,
        197,
        147,
        95,
        106,
        165,
        50,
        43
      ]
    },
    {
      "name": "remoteChainConfig",
      "discriminator": [
        248,
        170,
        246,
        200,
        84,
        101,
        138,
        67
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
    },
    {
      "name": "storageLocations",
      "discriminator": [
        181,
        104,
        172,
        221,
        163,
        103,
        113,
        150
      ]
    }
  ],
  "events": [
    {
      "name": "allowedFinalityConfigSet",
      "discriminator": [
        121,
        58,
        252,
        160,
        110,
        154,
        134,
        31
      ]
    },
    {
      "name": "allowlistAdminSet",
      "discriminator": [
        215,
        162,
        35,
        80,
        78,
        251,
        253,
        207
      ]
    },
    {
      "name": "allowlistSet",
      "discriminator": [
        5,
        175,
        41,
        145,
        80,
        182,
        74,
        12
      ]
    },
    {
      "name": "ccipCctpFundingConfigChanged",
      "discriminator": [
        7,
        191,
        168,
        67,
        102,
        168,
        45,
        16
      ]
    },
    {
      "name": "ccipCctpMessageEventAccountClosed",
      "discriminator": [
        136,
        216,
        177,
        229,
        207,
        28,
        50,
        100
      ]
    },
    {
      "name": "ccipCctpMessageSentEvent",
      "discriminator": [
        209,
        108,
        176,
        71,
        21,
        246,
        67,
        82
      ]
    },
    {
      "name": "ccipCctpV2MessageEventAccountClosed",
      "discriminator": [
        179,
        212,
        231,
        10,
        232,
        117,
        162,
        17
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
      "name": "chainConfigSetCctp",
      "discriminator": [
        52,
        53,
        109,
        166,
        126,
        50,
        179,
        238
      ]
    },
    {
      "name": "fastFinalityBpsSet",
      "discriminator": [
        103,
        246,
        85,
        87,
        132,
        53,
        138,
        97
      ]
    },
    {
      "name": "feeAggregatorSet",
      "discriminator": [
        161,
        76,
        121,
        176,
        125,
        65,
        122,
        250
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
    },
    {
      "name": "remoteChainCctpConfigChanged",
      "discriminator": [
        165,
        210,
        169,
        250,
        84,
        92,
        232,
        104
      ]
    },
    {
      "name": "storageLocationAdminTransferRequested",
      "discriminator": [
        166,
        158,
        3,
        232,
        221,
        198,
        114,
        248
      ]
    },
    {
      "name": "storageLocationAdminTransferred",
      "discriminator": [
        87,
        6,
        20,
        94,
        109,
        104,
        54,
        167
      ]
    },
    {
      "name": "storageLocationsSet",
      "discriminator": [
        9,
        156,
        80,
        246,
        97,
        165,
        91,
        205
      ]
    }
  ],
  "errors": [
    {
      "code": 12000,
      "name": "invalidTokenData",
      "msg": "Invalid token data"
    },
    {
      "code": 12001,
      "name": "invalidReceiver",
      "msg": "Invalid receiver"
    },
    {
      "code": 12002,
      "name": "invalidSourceDomain",
      "msg": "Invalid source domain"
    },
    {
      "code": 12003,
      "name": "invalidDestDomain",
      "msg": "Invalid destination domain"
    },
    {
      "code": 12004,
      "name": "invalidNonce",
      "msg": "Invalid nonce"
    },
    {
      "code": 12005,
      "name": "malformedCctpV1Message",
      "msg": "CCTP message is malformed or too short"
    },
    {
      "code": 12006,
      "name": "invalidCctpV1MessageVersion",
      "msg": "Invalid CCTP message version"
    },
    {
      "code": 12007,
      "name": "invalidTokenMessengerMinter",
      "msg": "Invalid Token Messenger Minter"
    },
    {
      "code": 12008,
      "name": "invalidMessageTransmitter",
      "msg": "Invalid Message Transmitter"
    },
    {
      "code": 12009,
      "name": "invalidMessageSentEventAccount",
      "msg": "Invalid Message Sent Event Account"
    },
    {
      "code": 12010,
      "name": "invalidTokenPoolExtraData",
      "msg": "Invalid Token Pool Extra Data"
    },
    {
      "code": 12011,
      "name": "failedCctpCpi",
      "msg": "Failed CCTP CPI"
    },
    {
      "code": 12012,
      "name": "invalidFundManager",
      "msg": "Fund Manager is invalid or misconfigured"
    },
    {
      "code": 12013,
      "name": "invalidReclaimDestination",
      "msg": "Invalid destination for funds reclaim"
    },
    {
      "code": 12014,
      "name": "insufficientFunds",
      "msg": "Insufficient funds"
    },
    {
      "code": 12015,
      "name": "invalidSolAmount",
      "msg": "Invalid SOL amount"
    },
    {
      "code": 12016,
      "name": "missingCpiReturnData",
      "msg": "CPI target did not return data"
    },
    {
      "code": 12017,
      "name": "invalidCpiReturnDataLength",
      "msg": "CPI return data has unexpected length"
    },
    {
      "code": 12018,
      "name": "invalidCpiReturnProgram",
      "msg": "CPI return data came from an unexpected program"
    },
    {
      "code": 12019,
      "name": "missingConfigurationRequiredCctpCcv",
      "msg": "Missing configuration of required CCTP CCV"
    },
    {
      "code": 12020,
      "name": "unsupportedBpsFee",
      "msg": "Unsupported BPS fee"
    },
    {
      "code": 13000,
      "name": "unauthorized",
      "msg": "The signer is unauthorized"
    },
    {
      "code": 13001,
      "name": "invalidVersion",
      "msg": "Invalid version of the onchain state"
    },
    {
      "code": 13002,
      "name": "invalidInstructionData",
      "msg": "Invalid instruction data"
    },
    {
      "code": 13003,
      "name": "invalidMessage",
      "msg": "Invalid message"
    },
    {
      "code": 13004,
      "name": "defaultOwnerProposal",
      "msg": "Proposed owner cannot be the default pubkey"
    },
    {
      "code": 13005,
      "name": "redundantOwnerProposal",
      "msg": "Proposed owner is the current owner"
    },
    {
      "code": 13006,
      "name": "defaultStorageLocationAdminProposal",
      "msg": "Proposed storage location admin cannot be the default pubkey"
    },
    {
      "code": 13007,
      "name": "redundantStorageLocationAdminProposal",
      "msg": "Proposed storage location admin is the current storage location admin"
    },
    {
      "code": 13008,
      "name": "invalidAllowedSenderPda",
      "msg": "Invalid allowed sender PDA"
    },
    {
      "code": 13009,
      "name": "sourceNotConfigured",
      "msg": "Source chain is not configured"
    },
    {
      "code": 13010,
      "name": "invalidVersionTag",
      "msg": "Invalid verifier results version tag"
    },
    {
      "code": 13011,
      "name": "malformedVerifierResults",
      "msg": "Malformed verifier results"
    },
    {
      "code": 13012,
      "name": "senderNotAllowed",
      "msg": "Sender not allowed by the destination chain allowlist"
    },
    {
      "code": 13013,
      "name": "zeroRouter",
      "msg": "Router cannot be the default (zero-value) pubkey"
    },
    {
      "code": 13014,
      "name": "invalidFinality",
      "msg": "Invalid Finality"
    },
    {
      "code": 13015,
      "name": "invalidAccountForMigration",
      "msg": "Account to migrate contains invalid data"
    },
    {
      "code": 13016,
      "name": "unknownDomain",
      "msg": "CCTP is not enabled for this chain"
    },
    {
      "code": 13017,
      "name": "unsupportedTokenForDomain",
      "msg": "CCTP is not enabled for this token for this chain"
    },
    {
      "code": 13018,
      "name": "missingTokenTransfer",
      "msg": "Missing token transfer in the message"
    },
    {
      "code": 13019,
      "name": "invalidNoncePdaOwner",
      "msg": "Nonce PDA is owned by an unexpected program"
    },
    {
      "code": 13020,
      "name": "invalidNonceAccountData",
      "msg": "Nonce account data is invalid"
    },
    {
      "code": 13021,
      "name": "wrongEventAccount",
      "msg": "Wrong Event PDA address"
    },
    {
      "code": 13022,
      "name": "invalidTransferAmount",
      "msg": "Invalid transfer amount"
    },
    {
      "code": 13023,
      "name": "invalidDestTokenAddress",
      "msg": "Invalid dest token address"
    },
    {
      "code": 13024,
      "name": "invalidSourceTokenAddress",
      "msg": "Invalid source token address"
    },
    {
      "code": 13025,
      "name": "invalidVerifierArgs",
      "msg": "Invalid verifier args"
    },
    {
      "code": 13026,
      "name": "invalidResolutionInputs",
      "msg": "Could not decode nested resolution inputs, or the passed-in account is neither this program's pool State nor its VerifierConfig"
    },
    {
      "code": 13027,
      "name": "invalidResolutionAccountOwner",
      "msg": "Account owned by the wrong program"
    },
    {
      "code": 13028,
      "name": "invalidPubkeyBytes",
      "msg": "Invalid address bytes"
    },
    {
      "code": 13029,
      "name": "invalidCctpMessageVersion",
      "msg": "Invalid CCTP message version"
    },
    {
      "code": 13030,
      "name": "invalidCctpSender",
      "msg": "Invalid CCTP message sender"
    },
    {
      "code": 13031,
      "name": "invalidCctpRecipient",
      "msg": "Invalid CCTP message recipient"
    },
    {
      "code": 13032,
      "name": "invalidCctpHookData",
      "msg": "Invalid CCTP message hook data, it should match the version tag and message ID"
    },
    {
      "code": 13033,
      "name": "invalidCctpSourceDomain",
      "msg": "Invalid source domain in the CCTP message"
    },
    {
      "code": 13034,
      "name": "invalidCctpDestinationDomain",
      "msg": "Invalid destination domain in the CCTP message"
    },
    {
      "code": 13035,
      "name": "invalidCctpMessage",
      "msg": "Invalid CCTP message, could not parse some field"
    },
    {
      "code": 13036,
      "name": "invalidCctpMintAmount",
      "msg": "Invalid CCTP mint amount"
    },
    {
      "code": 13037,
      "name": "feeGreaterThanAmount",
      "msg": "Executed fee is greater than the amount in the CCTP message"
    },
    {
      "code": 13038,
      "name": "unexpectedSourcePool",
      "msg": "Unexpected source pool address"
    },
    {
      "code": 13039,
      "name": "cctpAllowedCallersInvalidZero",
      "msg": "The CCTP allowed callers must not be the zero address"
    },
    {
      "code": 13040,
      "name": "feeAggregatorNotSet",
      "msg": "Fee aggregator not set"
    }
  ],
  "types": [
    {
      "name": "allowedFinalityConfigSet",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "old",
            "type": {
              "defined": {
                "name": "finalityConfig"
              }
            }
          },
          {
            "name": "new",
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
      "name": "allowlistAdminSet",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "old",
            "type": "pubkey"
          },
          {
            "name": "new",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "allowlistSet",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "enabled",
            "type": "bool"
          },
          {
            "name": "allowedSenders",
            "type": {
              "vec": "pubkey"
            }
          }
        ]
      }
    },
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
      "name": "byteString",
      "docs": [
        "Conventionally-text data stored on-chain as opaque bytes.",
        "",
        "Borsh-compatible with `String`: both encode as a u32 length prefix followed by the raw bytes, so",
        "a field can move from `String` to `ByteString` without migrating existing account data. What",
        "changes is what the program pays for and can fail on — deserializing a `String` validates UTF-8",
        "on every account load, a cost and a failure mode that a program which only stores and echoes the",
        "value has no use for. `ByteString` keeps the bytes opaque on-chain while callers keep string",
        "semantics off-chain, recovering the text with [`ByteString::to_utf8`].",
        "",
        "Use it only where the content is text by convention but the program never interprets it. If",
        "on-chain logic needs to *read* the text, `String` and its validation are the right choice.",
        "",
        "`INIT_SPACE` covers the length prefix alone, since the payload is variable: an account holding",
        "these must add the payload lengths itself (see committee-verifier's",
        "`StorageLocations::dynamic_space`)."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          "bytes"
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
      "name": "ccipCctpFundingConfigChanged",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "old",
            "type": {
              "defined": {
                "name": "fundingConfig"
              }
            }
          },
          {
            "name": "new",
            "type": {
              "defined": {
                "name": "fundingConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "ccipCctpMessageEventAccountClosed",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "originalSender",
            "type": "pubkey"
          },
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "msgTotalNonce",
            "type": "u64"
          },
          {
            "name": "address",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "ccipCctpMessageSentEvent",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "originalSender",
            "type": "pubkey"
          },
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "msgTotalNonce",
            "type": "u64"
          },
          {
            "name": "eventAddress",
            "type": "pubkey"
          },
          {
            "name": "sourceDomain",
            "type": "u32"
          },
          {
            "name": "cctpNonce",
            "type": "u64"
          },
          {
            "name": "messageSentBytes",
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "ccipCctpV2MessageEventAccountClosed",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "originalSender",
            "type": "pubkey"
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
            "name": "ccipCctpNonce",
            "type": "u64"
          },
          {
            "name": "address",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "cctpChain",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "domainId",
            "type": "u32"
          },
          {
            "name": "destinationCaller",
            "type": "pubkey"
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
            "name": "base",
            "type": {
              "defined": {
                "name": "baseChain"
              }
            }
          },
          {
            "name": "cctp",
            "type": {
              "defined": {
                "name": "cctpChain"
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
      "name": "chainConfigSet",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "router",
            "type": "pubkey"
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
            "name": "gasForVerification",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "payloadSizeBytes",
            "type": "u32"
          }
        ]
      }
    },
    {
      "name": "chainConfigSetCctp",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "enabled",
            "type": "bool"
          },
          {
            "name": "domainIdentifier",
            "type": "u32"
          },
          {
            "name": "lookupTable",
            "type": {
              "option": "pubkey"
            }
          },
          {
            "name": "allowedCallerReceive",
            "type": "pubkey"
          },
          {
            "name": "allowedCallerDeposit",
            "type": "pubkey"
          },
          {
            "name": "mintRecipientOnDest",
            "type": {
              "option": "pubkey"
            }
          },
          {
            "name": "supportedTokens",
            "type": {
              "vec": {
                "defined": {
                  "name": "remoteLocalPair"
                }
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
            "name": "owner",
            "type": "pubkey"
          },
          {
            "name": "storageLocationAdmin",
            "type": "pubkey"
          },
          {
            "name": "allowlistAdmin",
            "type": "pubkey"
          },
          {
            "name": "feeAggregator",
            "type": "pubkey"
          },
          {
            "name": "proposedOwner",
            "type": "pubkey"
          },
          {
            "name": "proposedStorageLocationAdmin",
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
            "name": "fastFinalityBps",
            "type": "u16"
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
      "name": "fastFinalityBpsSet",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "old",
            "type": "u16"
          },
          {
            "name": "new",
            "type": "u16"
          }
        ]
      }
    },
    {
      "name": "feeAggregatorSet",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "old",
            "type": "pubkey"
          },
          {
            "name": "new",
            "type": "pubkey"
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
      "name": "fundingConfig",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "manager",
            "type": "pubkey"
          },
          {
            "name": "reclaimDestination",
            "type": "pubkey"
          },
          {
            "name": "minimumSignerFunds",
            "type": "u64"
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
      "name": "getVerifierFeeParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "message",
            "type": {
              "defined": {
                "name": "svm2AnyMessage"
              }
            }
          },
          {
            "name": "extraArgs",
            "type": "bytes"
          },
          {
            "name": "finality",
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
      "name": "getVerifierFeeResult",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "fee",
            "type": {
              "defined": {
                "name": "usdCents"
              }
            }
          },
          {
            "name": "gasForVerification",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "payloadSizeBytes",
            "type": "u32"
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
      "name": "messageV1",
      "docs": [
        "CCIP 2.0 cross-chain MessageV1. Chain-agnostic wire format."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "sourceChainSelector",
            "type": "u64"
          },
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "messageNumber",
            "docs": [
              "Named after EVM's `MessageV1Codec.MessageV1.messageNumber`. CCIP 1.6 called the equivalent",
              "header field `sequenceNumber`; 1.7 renamed it, and the 1.6 spelling is still used by the",
              "legacy `RampMessageHeader` in the router."
            ],
            "type": "u64"
          },
          {
            "name": "executionGasLimit",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "ccipReceiveGasLimit",
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
            "name": "ccvAndExecutorHash",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          },
          {
            "name": "onRampAddress",
            "type": "bytes"
          },
          {
            "name": "offRampAddress",
            "type": "bytes"
          },
          {
            "name": "sender",
            "type": "bytes"
          },
          {
            "name": "receiver",
            "type": "bytes"
          },
          {
            "name": "destBlob",
            "type": "bytes"
          },
          {
            "name": "tokenTransfer",
            "type": {
              "option": {
                "defined": {
                  "name": "tokenTransferV1"
                }
              }
            }
          },
          {
            "name": "data",
            "type": "bytes"
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
      "name": "nonce",
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
            "name": "sender",
            "type": "pubkey"
          },
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "nonce",
            "type": "u64"
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
      "name": "reclaimVerifierEventAccountParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "originalSender",
            "type": "pubkey"
          },
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "ccipCctpNonce",
            "docs": [
              "The nonce assigned by this verifier for that particular message",
              "(per-sender, per-token, per-remote-chain)"
            ],
            "type": "u64"
          },
          {
            "name": "attestation",
            "docs": [
              "Attestation bytes as returned by Circle's iris endpoint"
            ],
            "type": "bytes"
          },
          {
            "name": "destinationMessage",
            "docs": [
              "Message bytes as returned by Circle's iris endpoint"
            ],
            "type": "bytes"
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
      "name": "remoteCctpConfig",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "enabled",
            "type": "bool"
          },
          {
            "name": "domainIdentifier",
            "type": "u32"
          },
          {
            "name": "allowedCallerReceive",
            "type": "pubkey"
          },
          {
            "name": "allowedCallerDeposit",
            "type": "pubkey"
          },
          {
            "name": "mintRecipientOnDest",
            "type": {
              "option": "pubkey"
            }
          },
          {
            "name": "lookupTable",
            "type": {
              "option": "pubkey"
            }
          },
          {
            "name": "supportedTokens",
            "type": {
              "vec": {
                "defined": {
                  "name": "remoteLocalPair"
                }
              }
            }
          }
        ]
      }
    },
    {
      "name": "remoteChainAdminConfig",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "router",
            "docs": [
              "The address of the Solana router program"
            ],
            "type": "pubkey"
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
            "name": "gasForVerification",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "payloadSizeBytes",
            "type": "u32"
          }
        ]
      }
    },
    {
      "name": "remoteChainAllowlistConfig",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "allowlistEnabled",
            "type": "bool"
          },
          {
            "name": "allowedSenders",
            "type": {
              "vec": "pubkey"
            }
          }
        ]
      }
    },
    {
      "name": "remoteChainCctpConfigChanged",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "config",
            "type": {
              "defined": {
                "name": "cctpChain"
              }
            }
          }
        ]
      }
    },
    {
      "name": "remoteChainConfig",
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
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "adminConfig",
            "type": {
              "defined": {
                "name": "remoteChainAdminConfig"
              }
            }
          },
          {
            "name": "allowlistConfig",
            "type": {
              "defined": {
                "name": "remoteChainAllowlistConfig"
              }
            }
          },
          {
            "name": "cctp",
            "type": {
              "defined": {
                "name": "remoteCctpConfig"
              }
            }
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
      "name": "remoteLocalPair",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "local",
            "type": "pubkey"
          },
          {
            "name": "remote",
            "docs": [
              "The remote address has to always be trimmed to the remote chain's length already, matching the",
              "router's dest_chain_state.config.address_bytes_length"
            ],
            "type": {
              "defined": {
                "name": "remoteAddress"
              }
            }
          }
        ]
      }
    },
    {
      "name": "svm2AnyMessage",
      "docs": [
        "User-facing message struct for sending cross-chain messages from SVM."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "receiver",
            "type": "bytes"
          },
          {
            "name": "data",
            "type": "bytes"
          },
          {
            "name": "tokenAmounts",
            "type": {
              "vec": {
                "defined": {
                  "name": "svmTokenAmount"
                }
              }
            }
          },
          {
            "name": "feeToken",
            "type": "pubkey"
          },
          {
            "name": "extraArgs",
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "svmTokenAmount",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "token",
            "type": "pubkey"
          },
          {
            "name": "amount",
            "type": "u64"
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
      "name": "setVerifierAllowlistParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "enabled",
            "type": "bool"
          },
          {
            "name": "allowedSenders",
            "type": {
              "vec": "pubkey"
            }
          }
        ]
      }
    },
    {
      "name": "setVerifierChainConfigCctpParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "cctp",
            "type": {
              "defined": {
                "name": "remoteCctpConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "setVerifierChainConfigParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "chainConfig",
            "type": {
              "defined": {
                "name": "remoteChainAdminConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "setVerifierStorageLocationsParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "storageLocations",
            "type": {
              "vec": {
                "defined": {
                  "name": "byteString"
                }
              }
            }
          }
        ]
      }
    },
    {
      "name": "startVerificationAtSourceParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "message",
            "type": {
              "defined": {
                "name": "messageV1"
              }
            }
          },
          {
            "name": "messageId",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          },
          {
            "name": "feeToken",
            "type": {
              "defined": {
                "name": "svmTokenAmount"
              }
            }
          },
          {
            "name": "verifierArgs",
            "type": "bytes"
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
          },
          {
            "name": "funding",
            "type": {
              "defined": {
                "name": "fundingConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "storageLocationAdminTransferRequested",
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
      "name": "storageLocationAdminTransferred",
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
      "name": "storageLocations",
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
            "name": "locations",
            "docs": [
              "Opaque location identifiers — URLs in practice, but the program only stores and echoes them,",
              "so they are held as [`ByteString`] rather than `String` to avoid paying for UTF-8 validation",
              "on every load. Borsh-identical to `Vec<String>`, so this needs no account migration."
            ],
            "type": {
              "vec": {
                "defined": {
                  "name": "byteString"
                }
              }
            }
          }
        ]
      }
    },
    {
      "name": "storageLocationsSet",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "locations",
            "type": {
              "vec": {
                "defined": {
                  "name": "byteString"
                }
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
      "name": "tokenTransferV1",
      "docs": [
        "CCIP 2.0 cross-chain TokenTransferV1. Chain-agnostic wire format."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "version",
            "type": "u8"
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
            "name": "sourcePoolAddress",
            "type": "bytes"
          },
          {
            "name": "sourceTokenAddress",
            "type": "bytes"
          },
          {
            "name": "destTokenAddress",
            "type": "bytes"
          },
          {
            "name": "tokenReceiver",
            "type": "bytes"
          },
          {
            "name": "extraData",
            "type": "bytes"
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
      "name": "verifyParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "messageId",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          },
          {
            "name": "encodedMessage",
            "type": "bytes"
          },
          {
            "name": "verifierResults",
            "type": "bytes"
          }
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
