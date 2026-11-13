/**
 * Program IDL in camelCase format in order to be used in JS/TS.
 *
 * Note that this is only a type helper and is not the actual IDL. The original
 * IDL can be found at `target/idl/lombard_token_pool.json`.
 */
export type LombardTokenPool = {
  "address": "Lomb8TTCwJKEhZrJ1J8UbsCRjNSf7NNQUEr4qo3dkSk",
  "metadata": {
    "name": "lombardTokenPool",
    "version": "2.1.0",
    "spec": "0.1.0",
    "description": "Lombard LBTC Token Pool - CCIP 1.6/2.0 with an embedded Lombard CCV"
  },
  "instructions": [
    {
      "name": "acceptOwnership",
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
          "name": "verifierConfig",
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
          "name": "verifierConfig",
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
        }
      ],
      "args": []
    },
    {
      "name": "appendRemotePoolAddresses",
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
      "name": "editChainRemoteConfigLombard",
      "discriminator": [
        158,
        189,
        6,
        226,
        46,
        176,
        99,
        164
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
              "name": "lombardChain"
            }
          }
        }
      ]
    },
    {
      "name": "editVerifierChainConfig",
      "docs": [
        "Overwrites the admin configuration of an already configured remote chain, leaving the",
        "rest of its configuration untouched."
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
          "name": "verifierConfig",
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
                  108,
                  111,
                  109,
                  98,
                  97,
                  114,
                  100,
                  95,
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
            "defaults to `is_enabled = false`; an unreadable one fails the instruction."
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
            "rather than erroring when the account doesn't exist yet."
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
          "name": "verifierConfig",
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
            "The router-derived `ccv_signer` PDA the router signs this CPI with, matching",
            "`committee-verifier`'s own `authority: Signer` requirement on `GetFee`."
          ],
          "signer": true
        },
        {
          "name": "remoteChainConfig",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  108,
                  111,
                  109,
                  98,
                  97,
                  114,
                  100,
                  95,
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
              "name": "verifierGetFeeParams"
            }
          }
        }
      ],
      "returns": {
        "defined": {
          "name": "verifierGetFeeResult"
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
          "name": "verifierConfig",
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
          "name": "verifierStorageLocations",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  108,
                  111,
                  109,
                  98,
                  97,
                  114,
                  100,
                  95,
                  118,
                  101,
                  114,
                  105,
                  102,
                  105,
                  101,
                  114,
                  95,
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
        },
        {
          "name": "destChainId",
          "type": {
            "array": [
              "u8",
              32
            ]
          }
        },
        {
          "name": "destCaller",
          "type": {
            "array": [
              "u8",
              32
            ]
          }
        }
      ]
    },
    {
      "name": "initGlobalConfig",
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
                  108,
                  111,
                  109,
                  98,
                  97,
                  114,
                  100,
                  95,
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
          "address": "Lomb8TTCwJKEhZrJ1J8UbsCRjNSf7NNQUEr4qo3dkSk"
        },
        {
          "name": "programData"
        }
      ],
      "args": []
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
          "name": "verifierConfig",
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
                  108,
                  111,
                  109,
                  98,
                  97,
                  114,
                  100,
                  95,
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
          "name": "program",
          "address": "Lomb8TTCwJKEhZrJ1J8UbsCRjNSf7NNQUEr4qo3dkSk"
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
                  108,
                  111,
                  109,
                  98,
                  97,
                  114,
                  100,
                  95,
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
              }
            ]
          }
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
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
        },
        {
          "name": "bridge",
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
          "name": "verifierConfig",
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
          "name": "verifierStorageLocations",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  108,
                  111,
                  109,
                  98,
                  97,
                  114,
                  100,
                  95,
                  118,
                  101,
                  114,
                  105,
                  102,
                  105,
                  101,
                  114,
                  95,
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
          "address": "Lomb8TTCwJKEhZrJ1J8UbsCRjNSf7NNQUEr4qo3dkSk"
        },
        {
          "name": "programData"
        }
      ],
      "args": []
    },
    {
      "name": "lockOrBurnTokens",
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
            "hold a balance to pay for the rent of initializing the Lombard MessageSentEvent account"
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
        },
        {
          "name": "mintAuthority"
        },
        {
          "name": "tokenAuthority"
        },
        {
          "name": "bridge",
          "address": "Lomva5kTftuXE8992qRufkaeq3XrXV47qv3C1W9xW6Z"
        },
        {
          "name": "bridgeConfig"
        },
        {
          "name": "mailbox"
        },
        {
          "name": "mailboxConfig",
          "writable": true
        },
        {
          "name": "localTokenConfig"
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        },
        {
          "name": "remoteBridgeConfig"
        },
        {
          "name": "remoteTokenConfig"
        },
        {
          "name": "bridgeSenderConfig"
        },
        {
          "name": "mailboxSenderConfig"
        },
        {
          "name": "outboundMessagePath"
        },
        {
          "name": "treasury",
          "optional": true
        },
        {
          "name": "outboundMessage",
          "writable": true
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
          "name": "poolTokenAccount"
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
                "path": "state.config.mint",
                "account": "state"
              }
            ]
          }
        },
        {
          "name": "chainConfigV2",
          "docs": [
            "defaults to WAIT_FOR_FINALITY (full-finality-only) and a disabled fee config; an unreadable",
            "one fails the instruction."
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
                "path": "state.config.mint",
                "account": "state"
              }
            ]
          }
        },
        {
          "name": "chainConfigOverride",
          "docs": [
            "== this program), its outbound rate limit bucket overrides the default one for",
            "faster-than-finality transfers."
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
                "path": "state.config.mint",
                "account": "state"
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
        "`instructions/nested_resolution.rs`."
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
          "name": "stateOrVerifierConfig",
          "docs": [
            "`State` or its `VerifierConfig`, per the module doc above. Whichever account resolution is",
            "being asked about (pool or verifier role) determines which one the caller passes in."
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
        "last transfer."
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
        "refill accrued since the last transfer."
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
        "consumers."
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
        "Returns a pool's per-mint configuration, for observability consumers."
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
      "name": "releaseOrMintTokens",
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
                  5,
                  18,
                  234,
                  191,
                  129,
                  96,
                  132,
                  158,
                  14,
                  125,
                  187,
                  241,
                  170,
                  159,
                  2,
                  72,
                  108,
                  117,
                  117,
                  149,
                  210,
                  90,
                  170,
                  107,
                  234,
                  138,
                  83,
                  251,
                  203,
                  91,
                  103,
                  225
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
        },
        {
          "name": "mintAuthority"
        },
        {
          "name": "tokenAuthority"
        },
        {
          "name": "bridge"
        },
        {
          "name": "bridgeConfig"
        },
        {
          "name": "mailbox",
          "address": "Lomsq4ZNkZQGysC1pQc2NqNiAbXQm6C8nLmKcUoESEA"
        },
        {
          "name": "mailboxConfig"
        },
        {
          "name": "localTokenConfig"
        },
        {
          "name": "systemProgram",
          "docs": [
            "The system program (needed for the 'init' constraint of the 'data' account)"
          ],
          "address": "11111111111111111111111111111111"
        },
        {
          "name": "remoteBridgeConfig"
        },
        {
          "name": "remoteTokenConfig"
        },
        {
          "name": "inboundMessagePath"
        },
        {
          "name": "messageInfo",
          "writable": true
        },
        {
          "name": "messageHandled",
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
                  5,
                  18,
                  234,
                  191,
                  129,
                  96,
                  132,
                  158,
                  14,
                  125,
                  187,
                  241,
                  170,
                  159,
                  2,
                  72,
                  108,
                  117,
                  117,
                  149,
                  210,
                  90,
                  170,
                  107,
                  234,
                  138,
                  83,
                  251,
                  203,
                  91,
                  103,
                  225
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
          "name": "offrampProgram"
        },
        {
          "name": "allowedOfframp",
          "docs": [
            "the router doesn't allow this offramp for this remote chain."
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
          "name": "poolTokenAccount"
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
                "path": "state.config.mint",
                "account": "state"
              }
            ]
          }
        },
        {
          "name": "chainConfigOverride",
          "docs": [
            "== this program), its inbound rate limit bucket overrides the default one for",
            "faster-than-finality transfers."
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
                "path": "state.config.mint",
                "account": "state"
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
          "name": "receiverTokenAccount"
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
      "name": "removeVerifierPath",
      "discriminator": [
        174,
        190,
        69,
        254,
        80,
        98,
        94,
        15
      ],
      "accounts": [
        {
          "name": "verifierConfig",
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
                  108,
                  111,
                  109,
                  98,
                  97,
                  114,
                  100,
                  95,
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
      ]
    },
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
          "name": "verifierConfig",
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
      "name": "setAlt",
      "discriminator": [
        124,
        193,
        229,
        78,
        223,
        34,
        84,
        12
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
          "name": "alt",
          "type": {
            "option": "pubkey"
          }
        }
      ]
    },
    {
      "name": "setChainConfigOverrideRateLimit",
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
          "address": "Lomb8TTCwJKEhZrJ1J8UbsCRjNSf7NNQUEr4qo3dkSk"
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
          "address": "Lomb8TTCwJKEhZrJ1J8UbsCRjNSf7NNQUEr4qo3dkSk"
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
          "name": "verifierConfig",
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
          "name": "verifierConfig",
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
            "The owner or the allowlist admin, like the committee and CCTP verifiers."
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
                  108,
                  111,
                  109,
                  98,
                  97,
                  114,
                  100,
                  95,
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
          "name": "verifierConfig",
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
          "name": "verifierConfig",
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
      "name": "setVerifierLookupTable",
      "discriminator": [
        207,
        240,
        50,
        61,
        40,
        249,
        35,
        183
      ],
      "accounts": [
        {
          "name": "verifierConfig",
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
                  108,
                  111,
                  109,
                  98,
                  97,
                  114,
                  100,
                  95,
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
        },
        {
          "name": "lookupTable",
          "type": {
            "option": "pubkey"
          }
        }
      ]
    },
    {
      "name": "setVerifierPath",
      "discriminator": [
        128,
        192,
        254,
        226,
        91,
        185,
        25,
        130
      ],
      "accounts": [
        {
          "name": "verifierConfig",
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
                  108,
                  111,
                  109,
                  98,
                  97,
                  114,
                  100,
                  95,
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
        },
        {
          "name": "lChainId",
          "type": {
            "array": [
              "u8",
              32
            ]
          }
        },
        {
          "name": "allowedCaller",
          "type": {
            "array": [
              "u8",
              32
            ]
          }
        },
        {
          "name": "remoteBridgeSender",
          "type": {
            "array": [
              "u8",
              32
            ]
          }
        }
      ]
    },
    {
      "name": "setVerifierRemoteAdapters",
      "discriminator": [
        101,
        162,
        140,
        26,
        152,
        152,
        210,
        251
      ],
      "accounts": [
        {
          "name": "verifierConfig",
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
                  108,
                  111,
                  109,
                  98,
                  97,
                  114,
                  100,
                  95,
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
                "path": "remoteChainSelector"
              }
            ]
          }
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
          "name": "remoteAdapters",
          "type": {
            "vec": {
              "defined": {
                "name": "remoteAdapter"
              }
            }
          }
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
          "name": "verifierConfig",
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
          "name": "verifierStorageLocations",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  108,
                  111,
                  109,
                  98,
                  97,
                  114,
                  100,
                  95,
                  118,
                  101,
                  114,
                  105,
                  102,
                  105,
                  101,
                  114,
                  95,
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
                  115
                ]
              }
            ]
          }
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
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
      "name": "setVerifierSupportedTokens",
      "discriminator": [
        104,
        246,
        195,
        225,
        235,
        103,
        111,
        107
      ],
      "accounts": [
        {
          "name": "verifierConfig",
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
        }
      ],
      "args": [
        {
          "name": "tokensToRemove",
          "type": {
            "vec": "pubkey"
          }
        },
        {
          "name": "tokensToSet",
          "type": {
            "vec": {
              "defined": {
                "name": "supportedToken"
              }
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
          "name": "verifierConfig",
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
            "The router-derived `ccv_signer` PDA the router signs this CPI with, matching",
            "`committee-verifier`'s own `authority: Signer` requirement."
          ],
          "signer": true
        },
        {
          "name": "remoteChainConfig",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  108,
                  111,
                  109,
                  98,
                  97,
                  114,
                  100,
                  95,
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
          "name": "tokenProgram"
        },
        {
          "name": "mint",
          "docs": [
            "CHECK against `params.message.token_transfer.source_token_address` in the handler - not",
            "tied to it via a static seed/address constraint because that field lives inside a Borsh",
            "`Vec<u8>` nested in the instruction args."
          ],
          "writable": true
        },
        {
          "name": "poolSigner",
          "docs": [
            "lock_or_burn contexts, since verifier and pool are embedded in the same program.",
            "",
            "Used as both `fee_payer` and `sender` in the `bridge::deposit` CPI,",
            "both are `Signer` + `#[account(mut)]` on the bridge program."
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
          "docs": [
            "The custody account `lock_or_burn_tokens_v2` left the (post-fee) tokens in; this is what",
            "`bridge::deposit` actually burns from."
          ],
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
          "name": "bridge",
          "address": "Lomva5kTftuXE8992qRufkaeq3XrXV47qv3C1W9xW6Z"
        },
        {
          "name": "bridgeConfig"
        },
        {
          "name": "bridgeSenderConfig"
        },
        {
          "name": "remoteBridgeConfig",
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
                  98,
                  114,
                  105,
                  100,
                  103,
                  101,
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
                "path": "remoteChainConfig.path.lChainId",
                "account": "verifierRemoteChainConfig"
              }
            ],
            "program": {
              "kind": "const",
              "value": [
                5,
                18,
                236,
                107,
                196,
                127,
                173,
                174,
                75,
                107,
                151,
                11,
                239,
                182,
                166,
                169,
                1,
                41,
                91,
                114,
                85,
                83,
                194,
                19,
                123,
                155,
                188,
                206,
                3,
                241,
                240,
                14
              ]
            }
          }
        },
        {
          "name": "localTokenConfig"
        },
        {
          "name": "remoteTokenConfig",
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
              },
              {
                "kind": "account",
                "path": "remoteChainConfig.path.lChainId",
                "account": "verifierRemoteChainConfig"
              }
            ],
            "program": {
              "kind": "const",
              "value": [
                5,
                18,
                236,
                107,
                196,
                127,
                173,
                174,
                75,
                107,
                151,
                11,
                239,
                182,
                166,
                169,
                1,
                41,
                91,
                114,
                85,
                83,
                194,
                19,
                123,
                155,
                188,
                206,
                3,
                241,
                240,
                14
              ]
            }
          }
        },
        {
          "name": "mailbox"
        },
        {
          "name": "mailboxConfig",
          "writable": true
        },
        {
          "name": "outboundMessagePath"
        },
        {
          "name": "outboundMessage",
          "writable": true
        },
        {
          "name": "mailboxSenderConfig"
        },
        {
          "name": "treasury",
          "writable": true,
          "optional": true
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
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
          "name": "verifierConfig",
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
          "name": "verifierConfig",
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
        "Returns the program type (name) and version.",
        "Used by offchain code to easily determine which program & version is being interacted with.",
        "",
        "# Arguments",
        "* `ctx` - The context"
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
          "name": "verifierConfig",
          "docs": [
            "Writable: forwarded as the real mailbox's `HandleMessage.handler`",
            "(`Signer` + `#[account(mut)]`)."
          ],
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
          "name": "remoteChainConfig",
          "docs": [
            "`source_chain_selector` is known from the decoded message; `owner` is checked here since",
            "it doesn't depend on that."
          ]
        },
        {
          "name": "authority",
          "docs": [
            "The offramp-derived `ccv_signer` PDA the offramp signs this CPI with - proves whoever",
            "signed is under the control of `offramp_program`."
          ],
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
                  5,
                  18,
                  234,
                  191,
                  129,
                  96,
                  132,
                  158,
                  14,
                  125,
                  187,
                  241,
                  170,
                  159,
                  2,
                  72,
                  108,
                  117,
                  117,
                  149,
                  210,
                  90,
                  170,
                  107,
                  234,
                  138,
                  83,
                  251,
                  203,
                  91,
                  103,
                  225
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
          "name": "offrampProgram"
        },
        {
          "name": "allowedOfframp",
          "docs": [
            "offramp for this message's source chain - if the PDA doesn't exist, the router doesn't",
            "allow it. Exact address is verified manually in the handler."
          ]
        },
        {
          "name": "state",
          "docs": [
            "This program's own pool `State` for the mint being verified."
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
          "name": "tokenProgram"
        },
        {
          "name": "mint",
          "writable": true
        },
        {
          "name": "poolSigner",
          "docs": [
            "",
            "Forwarded as the bridge's `GMPReceive.handler`, is `Signer` + `#[account(mut)]` (pays `message_handled`'s rent",
            "via `init`), so this must be writable."
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
          "name": "receiverTokenAccount",
          "writable": true
        },
        {
          "name": "mintAuthority"
        },
        {
          "name": "tokenAuthority"
        },
        {
          "name": "bridge",
          "address": "Lomva5kTftuXE8992qRufkaeq3XrXV47qv3C1W9xW6Z"
        },
        {
          "name": "bridgeConfig"
        },
        {
          "name": "mailbox",
          "address": "Lomsq4ZNkZQGysC1pQc2NqNiAbXQm6C8nLmKcUoESEA"
        },
        {
          "name": "mailboxConfig"
        },
        {
          "name": "localTokenConfig"
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        },
        {
          "name": "remoteBridgeConfig"
        },
        {
          "name": "remoteTokenConfig",
          "docs": [
            "bridge's `GMPReceive.remote_token_config` is `#[account(mut, ...)]` - `gmp_receive`",
            "consumes `inbound_rate_limit` on it, so it must be writable or the nested CPI fails."
          ],
          "writable": true
        },
        {
          "name": "chainConfig",
          "docs": [
            "known once the message is decoded."
          ]
        },
        {
          "name": "inboundMessagePath"
        },
        {
          "name": "messageInfo",
          "writable": true
        },
        {
          "name": "messageHandled",
          "writable": true
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
                "account": "verifierConfig"
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
    },
    {
      "name": "verifierConfig",
      "discriminator": [
        176,
        103,
        248,
        36,
        138,
        167,
        176,
        220
      ]
    },
    {
      "name": "verifierRemoteChainConfig",
      "discriminator": [
        9,
        247,
        188,
        213,
        200,
        65,
        39,
        118
      ]
    },
    {
      "name": "verifierStorageLocations",
      "discriminator": [
        67,
        76,
        148,
        15,
        7,
        57,
        241,
        168
      ]
    }
  ],
  "events": [
    {
      "name": "altEnabled",
      "discriminator": [
        87,
        107,
        247,
        55,
        114,
        81,
        164,
        160
      ]
    },
    {
      "name": "ccipLombardMessageEventAccountClosed",
      "discriminator": [
        29,
        86,
        129,
        89,
        159,
        163,
        215,
        220
      ]
    },
    {
      "name": "ccipLombardMessageSentEvent",
      "discriminator": [
        180,
        79,
        58,
        253,
        250,
        117,
        109,
        20
      ]
    },
    {
      "name": "remoteChainLombardConfigChanged",
      "discriminator": [
        156,
        14,
        169,
        112,
        239,
        188,
        253,
        72
      ]
    },
    {
      "name": "verifierAllowedFinalityConfigSet",
      "discriminator": [
        8,
        160,
        108,
        253,
        20,
        167,
        169,
        252
      ]
    },
    {
      "name": "verifierAllowlistAdminSet",
      "discriminator": [
        141,
        143,
        28,
        226,
        50,
        94,
        35,
        29
      ]
    },
    {
      "name": "verifierAllowlistSet",
      "discriminator": [
        174,
        9,
        89,
        105,
        124,
        244,
        141,
        232
      ]
    },
    {
      "name": "verifierChainConfigSet",
      "discriminator": [
        142,
        43,
        75,
        102,
        238,
        70,
        112,
        235
      ]
    },
    {
      "name": "verifierFeeAggregatorSet",
      "discriminator": [
        105,
        214,
        68,
        79,
        163,
        176,
        181,
        167
      ]
    },
    {
      "name": "verifierLookupTableSet",
      "discriminator": [
        15,
        92,
        199,
        246,
        181,
        138,
        91,
        163
      ]
    },
    {
      "name": "verifierOwnershipTransferRequested",
      "discriminator": [
        60,
        148,
        47,
        25,
        58,
        42,
        76,
        150
      ]
    },
    {
      "name": "verifierOwnershipTransferred",
      "discriminator": [
        193,
        7,
        3,
        34,
        208,
        165,
        147,
        248
      ]
    },
    {
      "name": "verifierPathRemoved",
      "discriminator": [
        17,
        59,
        242,
        249,
        160,
        120,
        159,
        3
      ]
    },
    {
      "name": "verifierPathSet",
      "discriminator": [
        137,
        172,
        16,
        148,
        26,
        79,
        37,
        139
      ]
    },
    {
      "name": "verifierRemoteAdaptersSet",
      "discriminator": [
        51,
        102,
        128,
        158,
        67,
        108,
        166,
        51
      ]
    },
    {
      "name": "verifierStorageLocationAdminTransferRequested",
      "discriminator": [
        191,
        244,
        222,
        218,
        250,
        147,
        127,
        82
      ]
    },
    {
      "name": "verifierStorageLocationAdminTransferred",
      "discriminator": [
        61,
        239,
        243,
        230,
        152,
        220,
        253,
        39
      ]
    },
    {
      "name": "verifierStorageLocationsSet",
      "discriminator": [
        247,
        6,
        192,
        134,
        112,
        147,
        224,
        196
      ]
    },
    {
      "name": "verifierSupportedTokensUpdated",
      "discriminator": [
        106,
        223,
        139,
        43,
        214,
        144,
        28,
        206
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
      "name": "invalidTokenMessengerMinter",
      "msg": "Invalid Token Messenger Minter"
    },
    {
      "code": 12006,
      "name": "invalidBridge",
      "msg": "Invalid Bridge"
    },
    {
      "code": 12007,
      "name": "invalidMessageSentEventAccount",
      "msg": "Invalid Message Sent Event Account"
    },
    {
      "code": 12008,
      "name": "invalidTokenPoolExtraData",
      "msg": "Invalid Token Pool Extra Data"
    },
    {
      "code": 12009,
      "name": "failedGmpMessageHandle",
      "msg": "Failed to handle GMP message"
    },
    {
      "code": 12010,
      "name": "invalidFundManager",
      "msg": "Fund Manager is invalid or misconfigured"
    },
    {
      "code": 12011,
      "name": "invalidReclaimDestination",
      "msg": "Invalid destination for funds reclaim"
    },
    {
      "code": 12012,
      "name": "insufficientFunds",
      "msg": "Insufficient funds"
    },
    {
      "code": 12013,
      "name": "invalidSolAmount",
      "msg": "Invalid SOL amount"
    },
    {
      "code": 12014,
      "name": "malformedAttestationData",
      "msg": "Malformed Attestation data"
    },
    {
      "code": 12015,
      "name": "remoteChainMismatch",
      "msg": "Remote chain id mismatch"
    },
    {
      "code": 12016,
      "name": "amountMismatch",
      "msg": "Amount mismatch"
    },
    {
      "code": 12017,
      "name": "invalidPayloadLength",
      "msg": "Invalid payload size"
    },
    {
      "code": 12018,
      "name": "invalidPayload",
      "msg": "Invalid payload"
    },
    {
      "code": 12019,
      "name": "abiWordOverflow",
      "msg": "ABI word value overflows the target integer type"
    },
    {
      "code": 12020,
      "name": "malformedReceiver",
      "msg": "Unexpected size of receiver byte vector"
    },
    {
      "code": 12021,
      "name": "defaultAdminProposal",
      "msg": "Cannot propose the default (zero) pubkey as a new admin"
    },
    {
      "code": 12022,
      "name": "redundantAdminProposal",
      "msg": "Proposed admin is already the current admin"
    },
    {
      "code": 12023,
      "name": "zeroRouterSigner",
      "msg": "Router signer must not be the default (zero) pubkey"
    },
    {
      "code": 12024,
      "name": "pathNotConfigured",
      "msg": "Path not configured for this remote chain"
    },
    {
      "code": 12025,
      "name": "invalidPath",
      "msg": "Path lChainId/allowedCaller/remoteBridgeSender must not be zero"
    },
    {
      "code": 12026,
      "name": "unsupportedToken",
      "msg": "Token is not in the verifier's supported token list"
    },
    {
      "code": 12027,
      "name": "senderNotAllowed",
      "msg": "Sender is not in the allowlist for this destination chain"
    },
    {
      "code": 12028,
      "name": "duplicateOrZeroCcv",
      "msg": "Duplicate or zero-address CCV entry"
    },
    {
      "code": 12029,
      "name": "invalidResolutionInputs",
      "msg": "Could not decode nested resolution inputs, or the passed-in account is neither this program's pool State nor its VerifierConfig"
    },
    {
      "code": 12030,
      "name": "remoteTokenOrAdapterMismatch",
      "msg": "The bridge's configured destination token for this mint doesn't match the message's declared destination token, and no matching remote adapter override is configured"
    },
    {
      "code": 12031,
      "name": "localAdapterNotSupported",
      "msg": "local_adapter is not supported"
    },
    {
      "code": 12032,
      "name": "unsupportedBpsFee",
      "msg": "Unsupported BPS fee"
    },
    {
      "code": 12033,
      "name": "unexpectedSourcePool",
      "msg": "Unexpected source pool address"
    },
    {
      "code": 12034,
      "name": "feeAggregatorNotSet",
      "msg": "Fee aggregator not set"
    },
    {
      "code": 12035,
      "name": "invalidResolutionAccountOwner",
      "msg": "Account owned by the wrong program"
    }
  ],
  "types": [
    {
      "name": "altEnabled",
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
      "name": "ccipLombardMessageEventAccountClosed",
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
      "name": "ccipLombardMessageSentEvent",
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
            "name": "bridgeNonce",
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
            "name": "bridge",
            "type": {
              "defined": {
                "name": "lombardChain"
              }
            }
          }
        ]
      }
    },
    {
      "name": "chainConfigOverride",
      "docs": [
        "Optional per-(mint, remote chain, override type) rate-limit override (currently only `FTF`).",
        "The override type is part of the PDA seeds so future override kinds get their own PDA without",
        "migrating existing accounts.",
        "PDA seeds: `[POOL_OVERRIDE_CHAINCONFIG_SEED, remote_chain_selector, mint, override_type]`."
      ],
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
      "docs": [
        "Per-(mint, remote chain) v2 lane config"
      ],
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
            "name": "mint",
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
      "name": "lombardChain",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destinationChainId",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          },
          {
            "name": "destinationCaller",
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
      "name": "lombardVerifierPath",
      "docs": [
        "Lombard GMP path config for a remote chain",
        "(`allowedCaller`, `lChainId`, `remoteBridgeSender`)."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "allowedCaller",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          },
          {
            "name": "lChainId",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          },
          {
            "name": "remoteBridgeSender",
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
      "name": "remoteAdapter",
      "docs": [
        "A remote adapter override for one local token on one remote chain",
        "`LombardVerifier.RemoteAdapterArgs` / `s_remoteAdapters[destChainSelector][token]`."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "token",
            "type": "pubkey"
          },
          {
            "name": "remoteAdapter",
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
      "name": "remoteBridgeConfig",
      "docs": [
        "The explicit `\"account\"` namespace reproduces the exact same discriminator the default",
        "(no namespace) `#[account]` expansion would compute (`sighash(\"account\",",
        "\"RemoteBridgeConfig\")` either way - see",
        "`anchor-syn::codegen::program::common::{sighash,gen_discriminator}`), but *skips* the",
        "auto-generated `Owner` impl, which anchor-lang always ties to `crate::ID` (the enclosing",
        "`lombard_token_pool` crate root, not this module's own `declare_id!()` - see",
        "`anchor-attribute-account::lib.rs`'s `owner_impl`, which only runs when the namespace arg",
        "is empty). Without this, `Account<'info, RemoteBridgeConfig>` would reject every real",
        "`RemoteBridgeConfig` account (owned by the real/mock `bridge` program) as an ownership",
        "mismatch."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "bump",
            "type": "u8"
          },
          {
            "name": "chainId",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          },
          {
            "name": "bridge",
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
      "name": "remoteChainLombardConfigChanged",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "config",
            "type": {
              "defined": {
                "name": "lombardChain"
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
      "name": "remoteTokenConfig",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "bump",
            "type": "u8"
          },
          {
            "name": "chainId",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          },
          {
            "name": "token",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          },
          {
            "name": "direction",
            "type": "u8"
          },
          {
            "name": "inboundRateLimit",
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
                "name": "verifierRemoteChainAdminConfig"
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
            "name": "bridge",
            "type": "pubkey"
          },
          {
            "name": "alt",
            "type": {
              "option": "pubkey"
            }
          }
        ]
      }
    },
    {
      "name": "supportedToken",
      "docs": [
        "A local token this verifier supports, with an optional adapter substitution.",
        "`local_adapter` is `None` when the token itself is used directly with the Lombard bridge."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "localToken",
            "type": "pubkey"
          },
          {
            "name": "localAdapter",
            "type": {
              "option": "pubkey"
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
      "name": "verifierAllowedFinalityConfigSet",
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
      "name": "verifierAllowlistAdminSet",
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
      "name": "verifierAllowlistSet",
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
      "name": "verifierChainConfigSet",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "routerSigner",
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
      "name": "verifierConfig",
      "docs": [
        "Global verifier config."
      ],
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
            "name": "proposedOwner",
            "type": "pubkey"
          },
          {
            "name": "storageLocationAdmin",
            "type": "pubkey"
          },
          {
            "name": "proposedStorageLocationAdmin",
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
            "name": "allowedFinalityConfig",
            "type": {
              "defined": {
                "name": "finalityConfig"
              }
            }
          },
          {
            "name": "versionTag",
            "docs": [
              "This verifier's own version tag, mirroring EVM's immutable `versionTag()`"
            ],
            "type": {
              "array": [
                "u8",
                4
              ]
            }
          },
          {
            "name": "supportedTokens",
            "type": {
              "vec": {
                "defined": {
                  "name": "supportedToken"
                }
              }
            }
          }
        ]
      }
    },
    {
      "name": "verifierFeeAggregatorSet",
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
      "name": "verifierGetFeeParams",
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
      "name": "verifierGetFeeResult",
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
      "name": "verifierLookupTableSet",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "old",
            "type": {
              "option": "pubkey"
            }
          },
          {
            "name": "new",
            "type": {
              "option": "pubkey"
            }
          }
        ]
      }
    },
    {
      "name": "verifierOwnershipTransferRequested",
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
      "name": "verifierOwnershipTransferred",
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
      "name": "verifierPathRemoved",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "path",
            "type": {
              "defined": {
                "name": "lombardVerifierPath"
              }
            }
          }
        ]
      }
    },
    {
      "name": "verifierPathSet",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "path",
            "type": {
              "defined": {
                "name": "lombardVerifierPath"
              }
            }
          }
        ]
      }
    },
    {
      "name": "verifierRemoteAdaptersSet",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "remoteAdapters",
            "type": {
              "vec": {
                "defined": {
                  "name": "remoteAdapter"
                }
              }
            }
          }
        ]
      }
    },
    {
      "name": "verifierRemoteChainAdminConfig",
      "docs": [
        "Per-remote-chain verifier admin config"
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "routerSigner",
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
      "name": "verifierRemoteChainAllowlistConfig",
      "docs": [
        "Per-dest-chain sender allowlist for the outbound path",
        "`RemoteChainAllowlistConfig` / EVM's `BaseVerifier` allowlist."
      ],
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
      "name": "verifierRemoteChainConfig",
      "docs": [
        "Per-remote-chain verifier config"
      ],
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
                "name": "verifierRemoteChainAdminConfig"
              }
            }
          },
          {
            "name": "allowlistConfig",
            "type": {
              "defined": {
                "name": "verifierRemoteChainAllowlistConfig"
              }
            }
          },
          {
            "name": "path",
            "type": {
              "defined": {
                "name": "lombardVerifierPath"
              }
            }
          },
          {
            "name": "lookupTable",
            "docs": [
              "Address lookup table holding this remote chain's own resolution accounts."
            ],
            "type": {
              "option": "pubkey"
            }
          },
          {
            "name": "remoteAdapters",
            "type": {
              "vec": {
                "defined": {
                  "name": "remoteAdapter"
                }
              }
            }
          }
        ]
      }
    },
    {
      "name": "verifierStorageLocationAdminTransferRequested",
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
      "name": "verifierStorageLocationAdminTransferred",
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
      "name": "verifierStorageLocations",
      "docs": [
        "Off-chain proof-lookup locations"
      ],
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
      "name": "verifierStorageLocationsSet",
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
      "name": "verifierSupportedTokensUpdated",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "tokensRemoved",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "tokensSet",
            "type": {
              "vec": {
                "defined": {
                  "name": "supportedToken"
                }
              }
            }
          }
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
            "docs": [
              "32-byte `payload_hash` of an already-`Delivered` Lombard mailbox message"
            ],
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
