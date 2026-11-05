/**
 * Program IDL in camelCase format in order to be used in JS/TS.
 *
 * Note that this is only a type helper and is not the actual IDL. The original
 * IDL can be found at `target/idl/ccip_offramp.json`.
 */
export type CcipOfframp = {
  "address": "off43sBHBsmWTU6VbfwKYCNwyJ4rSDhunYvz3QL7bF9",
  "metadata": {
    "name": "ccipOfframp",
    "version": "2.0.0",
    "spec": "0.1.0",
    "description": "Created with Anchor"
  },
  "instructions": [
    {
      "name": "acceptOwnership",
      "docs": [
        "Accepts the ownership of the router by the proposed owner.",
        "",
        "Shared func signature with other programs",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for accepting ownership.",
        "The new owner must be a signer of the transaction."
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
      "name": "addSourceChain",
      "docs": [
        "Adds a new source chain selector with its config to the offramp.",
        "",
        "The Admin needs to add any new chain supported.",
        "When adding a new chain, the Admin needs to specify if it's enabled or not.",
        "",
        "# Arguments",
        "* `ctx` - The context containing the accounts required for adding the chain selector.",
        "* `params` - The parameters for updating the source chain, including the selector and new configuration"
      ],
      "discriminator": [
        26,
        58,
        148,
        88,
        190,
        27,
        2,
        144
      ],
      "accounts": [
        {
          "name": "sourceChain",
          "docs": [
            "Adding a chain selector implies initializing the state for a new chain"
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  115,
                  111,
                  117,
                  114,
                  99,
                  101,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "params.newChainSelector"
              }
            ]
          }
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
              "name": "addSourceChainParams"
            }
          }
        }
      ]
    },
    {
      "name": "bufferExecutionInputs",
      "docs": [
        "Loads execution inputs into a buffer account for later execution. This is used for large messages that",
        "cannot be executed in a single transaction due to Solana's transaction size limits. Only the authority",
        "that created the buffer can later on use it to execute the message."
      ],
      "discriminator": [
        123,
        142,
        203,
        67,
        134,
        9,
        215,
        26
      ],
      "accounts": [
        {
          "name": "executionInputsBuffer",
          "writable": true
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
              "name": "bufferExecutionInputsParams"
            }
          }
        }
      ]
    },
    {
      "name": "closeExecutionInputsBuffer",
      "docs": [
        "Closes the execution inputs buffer account, refunding its rent to the authority.",
        "It may only be invoked by the authority that originally created that buffer."
      ],
      "discriminator": [
        198,
        228,
        50,
        91,
        166,
        43,
        53,
        198
      ],
      "accounts": [
        {
          "name": "executionInputsBuffer",
          "writable": true
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
              "name": "closeExecutionInputsBufferParams"
            }
          }
        }
      ]
    },
    {
      "name": "disableSourceChainSelector",
      "docs": [
        "Disables the source chain selector.",
        "",
        "The Admin is the only one able to disable the chain selector as source. This method is thought of as an emergency kill-switch.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for disabling the chain selector.",
        "* `source_chain_selector` - The source chain selector to be disabled."
      ],
      "discriminator": [
        58,
        101,
        54,
        252,
        248,
        31,
        226,
        121
      ],
      "accounts": [
        {
          "name": "sourceChain",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  115,
                  111,
                  117,
                  114,
                  99,
                  101,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "sourceChainSelector"
              }
            ]
          }
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
          "name": "sourceChainSelector",
          "type": "u64"
        }
      ]
    },
    {
      "name": "executeV2",
      "docs": [
        "CCIPv2 message execution. Invoke this method to execute a message, either directly or buffered.",
        "This method is permissionless - anyone can execute a message on behalf of anyone else, as long as the message",
        "is valid and all the required verifier results are valid."
      ],
      "discriminator": [
        110,
        12,
        255,
        88,
        93,
        207,
        223,
        214
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
          "name": "referenceAddresses",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  114,
                  101,
                  102,
                  101,
                  114,
                  101,
                  110,
                  99,
                  101,
                  95,
                  97,
                  100,
                  100,
                  114,
                  101,
                  115,
                  115,
                  101,
                  115
                ]
              }
            ]
          }
        },
        {
          "name": "sourceChain",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  115,
                  111,
                  117,
                  114,
                  99,
                  101,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              },
              {
                "kind": "account",
                "path": "sourceChain.chainSelector",
                "account": "sourceChain"
              }
            ]
          }
        },
        {
          "name": "messageExecState",
          "writable": true
        },
        {
          "name": "offramp",
          "address": "off43sBHBsmWTU6VbfwKYCNwyJ4rSDhunYvz3QL7bF9"
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
          "name": "sysvarInstructions",
          "address": "Sysvar1nstructions1111111111111111111111111"
        },
        {
          "name": "tokenAdminRegistry",
          "docs": [
            "token. When there isn't, it's the zero pubkey. This is stored separately to the token accounts as it's",
            "necessary to determine the schema for the token account layout."
          ]
        },
        {
          "name": "allowedOfframp",
          "docs": [
            "CHECK PDA of the router program verifying the signer is an allowed offramp.",
            "If PDA does not exist, the router doesn't allow this offramp. This is just used",
            "so that token pools and receivers can then check that the caller is an actual offramp that",
            "has been registered in the router as such for that source chain.",
            "The selector seed comes from `source_chain`, which `execute_v2` binds to the message, so this",
            "authorization is effectively checked against the message's own source chain."
          ]
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
              "name": "executeParams"
            }
          }
        }
      ]
    },
    {
      "name": "getCcvsForMsg",
      "docs": [
        "Get CCVs for a given message. To resolve the list, it takes into account the lane configurations, the",
        "pool's CCVs (when there is a token transfer), and the receiver contract's CCVs (when there is an arbitrary",
        "message), among other factors."
      ],
      "discriminator": [
        86,
        132,
        32,
        127,
        168,
        122,
        87,
        66
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
          "name": "referenceAddresses",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  114,
                  101,
                  102,
                  101,
                  114,
                  101,
                  110,
                  99,
                  101,
                  95,
                  97,
                  100,
                  100,
                  114,
                  101,
                  115,
                  115,
                  101,
                  115
                ]
              }
            ]
          }
        },
        {
          "name": "sourceChain",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  115,
                  111,
                  117,
                  114,
                  99,
                  101,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              },
              {
                "kind": "account",
                "path": "sourceChain.chainSelector",
                "account": "sourceChain"
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
        }
      ],
      "args": [
        {
          "name": "params",
          "type": {
            "defined": {
              "name": "getCcvsForMsgParams"
            }
          }
        }
      ],
      "returns": {
        "defined": {
          "name": "getCcvsForMsgResponse"
        }
      }
    },
    {
      "name": "initialize",
      "docs": [
        "Initializes the CCIP Offramp, except for the config account (due to stack size limitations).",
        "",
        "The initialization of the Offramp is responsibility of Admin, nothing more than calling these",
        "initialization methods should be done first.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for initialization."
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
          "name": "referenceAddresses",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  114,
                  101,
                  102,
                  101,
                  114,
                  101,
                  110,
                  99,
                  101,
                  95,
                  97,
                  100,
                  100,
                  114,
                  101,
                  115,
                  115,
                  101,
                  115
                ]
              }
            ]
          }
        },
        {
          "name": "router"
        },
        {
          "name": "feeQuoter"
        },
        {
          "name": "rmnRemote"
        },
        {
          "name": "offrampLookupTable"
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
          "address": "off43sBHBsmWTU6VbfwKYCNwyJ4rSDhunYvz3QL7bF9"
        },
        {
          "name": "programData"
        }
      ],
      "args": []
    },
    {
      "name": "initializeConfig",
      "docs": [
        "Initializes the CCIP Offramp Config account.",
        "",
        "The initialization of the Offramp is responsibility of Admin, nothing more than calling these",
        "initialization methods should be done first.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for initialization of the config.",
        "* `svm_chain_selector` - The chain selector for SVM."
      ],
      "discriminator": [
        208,
        127,
        21,
        1,
        194,
        190,
        196,
        70
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
          "address": "off43sBHBsmWTU6VbfwKYCNwyJ4rSDhunYvz3QL7bF9"
        },
        {
          "name": "programData"
        }
      ],
      "args": [
        {
          "name": "svmChainSelector",
          "type": "u64"
        }
      ]
    },
    {
      "name": "observeConfig",
      "docs": [
        "Returns the offramp's global configuration and reference addresses, for observability",
        "consumers.",
        "",
        "Permissionless and read-only: meant to be executed through `simulateTransaction`.",
        "",
        "# Arguments",
        "* `ctx` - The context containing the config and reference addresses accounts"
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
        },
        {
          "name": "referenceAddresses",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  114,
                  101,
                  102,
                  101,
                  114,
                  101,
                  110,
                  99,
                  101,
                  95,
                  97,
                  100,
                  100,
                  114,
                  101,
                  115,
                  115,
                  101,
                  115
                ]
              }
            ]
          }
        }
      ],
      "args": [],
      "returns": {
        "defined": {
          "name": "offRampConfigObservation"
        }
      }
    },
    {
      "name": "observeSourceChain",
      "docs": [
        "Returns the offramp's configuration for a single source chain, for observability consumers.",
        "",
        "Permissionless and read-only: meant to be executed through `simulateTransaction`. Fails if",
        "the lane has no source chain account, which callers can read as \"not configured\".",
        "",
        "# Arguments",
        "* `ctx` - The context containing the source chain account",
        "* `source_chain_selector` - The source chain selector the lane belongs to"
      ],
      "discriminator": [
        117,
        75,
        255,
        27,
        58,
        26,
        131,
        74
      ],
      "accounts": [
        {
          "name": "sourceChain",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  115,
                  111,
                  117,
                  114,
                  99,
                  101,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "sourceChainSelector"
              }
            ]
          }
        }
      ],
      "args": [
        {
          "name": "sourceChainSelector",
          "type": "u64"
        }
      ],
      "returns": {
        "defined": {
          "name": "offRampSourceChainObservation"
        }
      }
    },
    {
      "name": "resolveAccountsExecuteCcvStage",
      "docs": [
        "This is a step on the account resolution process, which repeats at least once",
        "per ccv (potentially more as it's nested). Users should call `resolve_accounts_start` first,",
        "and by following that response and continuing with the process they will automatically invoke this method next,",
        "which will return more accounts needed for the operation they want to perform, and continue the process."
      ],
      "discriminator": [
        223,
        200,
        155,
        36,
        253,
        38,
        121,
        145
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
          "name": "ccvProgram"
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
      "name": "resolveAccountsExecuteMessageStage",
      "docs": [
        "This is a second step on the account resolution process. Users should call `resolve_accounts_start` first,",
        "and by following that response and continuing with the process they will automatically invoke this method next,",
        "which will return more accounts needed for the operation they want to perform, and continue the process.",
        "",
        "For buffered `execute_v2` resolution, callers should pass exactly the accounts returned by",
        "`resolve_accounts_start.ask_again_with`. This includes the execution report buffer account as an additional",
        "remaining account after the declared `config` and `reference_addresses` accounts."
      ],
      "discriminator": [
        168,
        232,
        96,
        186,
        197,
        204,
        35,
        41
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
          "name": "referenceAddresses",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  114,
                  101,
                  102,
                  101,
                  114,
                  101,
                  110,
                  99,
                  101,
                  95,
                  97,
                  100,
                  100,
                  114,
                  101,
                  115,
                  115,
                  101,
                  115
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
      "name": "resolveAccountsExecuteNestedTokenStage",
      "docs": [
        "Account-resolution step that delegates to the token pool's own resolver, for pools that implement account resolution (CCTP, Lombard).",
        "",
        "Repeats until the pool reports it has nothing left to resolve: each round CPIs into the",
        "pool's resolver as the offramp's pool signer and forwards the inputs the pool returned for",
        "the next round. The first round seeds it with `TokenPoolCpiResolutionInputs::ReleaseOrMint`.",
        "",
        "Callers should pass exactly the accounts returned by the previous `ask_again_with`; in",
        "buffered mode the execution-inputs buffer comes first and the pool's resolver accounts",
        "follow."
      ],
      "discriminator": [
        4,
        197,
        187,
        37,
        217,
        185,
        100,
        5
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
          "name": "poolProgram"
        },
        {
          "name": "poolSigner"
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
      "name": "resolveAccountsExecuteTokenTransferRetrieveLutStage",
      "docs": [
        "Token-transfer account-resolution step that hands back the transferred token's lookup table.",
        "",
        "Reads the token's `TokenAdminRegistry` to learn its LUT, registers that LUT so the caller",
        "loads it, and asks again with the LUT included so the next stage can read the pool addresses",
        "out of it. Saves no accounts of its own.",
        "",
        "In buffered mode the execution-inputs buffer arrives as the first remaining account and is",
        "forwarded to the next stage, which is the first one that needs to read the message."
      ],
      "discriminator": [
        1,
        190,
        219,
        240,
        109,
        150,
        201,
        184
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
          "name": "adminRegistry"
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
      "name": "resolveAccountsExecuteTokenTransferStaticAccountsStage",
      "docs": [
        "Token-transfer account-resolution step that derives the fixed set of pool accounts.",
        "",
        "Runs after the retrieve-LUT stage, which supplies the token's lookup table. Reads the pool",
        "program, mint and token program out of the LUT and the token transfer out of the message,",
        "then saves the accounts the `release_or_mint` CPI needs. Pools that resolve their own extra",
        "accounts continue into the nested-token stage from here.",
        "",
        "A lookup table too large for one 1kB response is emitted a page at a time: the stage then",
        "points back at itself, so callers must keep following `next_ix_discriminator` rather than",
        "assuming a single round.",
        "",
        "Callers should pass exactly the accounts returned by the previous `ask_again_with`; in",
        "buffered mode that includes the execution-inputs buffer as the first remaining account."
      ],
      "discriminator": [
        179,
        83,
        243,
        87,
        206,
        66,
        84,
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
        },
        {
          "name": "referenceAddresses",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  114,
                  101,
                  102,
                  101,
                  114,
                  101,
                  110,
                  99,
                  101,
                  95,
                  97,
                  100,
                  100,
                  114,
                  101,
                  115,
                  115,
                  101,
                  115
                ]
              }
            ]
          }
        },
        {
          "name": "adminRegistry"
        },
        {
          "name": "tokenLut"
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
      "name": "resolveAccountsGetCcvsForMsgStage",
      "docs": [
        "Second account-resolution step for `get_ccvs_for_msg`.",
        "",
        "Callers should pass exactly the accounts returned by the previous",
        "`ask_again_with`. The resolver may ask for this same step once more with the receiver",
        "registry included so it can decide whether the receiver is v1 or v2 on-chain."
      ],
      "discriminator": [
        206,
        223,
        92,
        72,
        103,
        28,
        20,
        75
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
          "name": "referenceAddresses",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  114,
                  101,
                  102,
                  101,
                  114,
                  101,
                  110,
                  99,
                  101,
                  95,
                  97,
                  100,
                  100,
                  114,
                  101,
                  115,
                  115,
                  101,
                  115
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
      "name": "resolveAccountsStart",
      "docs": [
        "Entrypoint to resolve accounts of any of the supported operations in this program, to make it easier for",
        "offchain code to interact with the program without having to derive all accounts by itself, and future-proofing",
        "against changes in account structure.",
        "",
        "Some instructions may require additional accounts in the resolver context. Those accounts must be passed as",
        "extra accounts to the resolution instruction, after the accounts declared in the resolver context, and each",
        "instruction-specific resolver documents its own requirements.",
        "",
        "For `execute_v2`, if `ExecuteParams::exec_inputs` is `None`, account resolution is for buffered execution.",
        "In that case the first `resolve_accounts_start` call must include the execution inputs buffer PDA as an",
        "additional remaining account. The PDA is derived from",
        "`[\"execution_report_buffer\", <buffer_id>, <execute_caller_pubkey>]`, where `execute_caller_pubkey` is the",
        "signer that will call `execute_v2`. The resolver reads that account to recover the message, then includes the",
        "buffer account in the returned `ask_again_with`/final account list as needed. Direct execution, where",
        "`exec_inputs` is present in the instruction data, does not require any additional resolver accounts."
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
      "name": "transferOwnership",
      "docs": [
        "Transfers the ownership of the router to a new proposed owner.",
        "",
        "Shared func signature with other programs",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for the transfer.",
        "* `proposed_owner` - The public key of the new proposed owner."
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
      "name": "updateReferenceAddresses",
      "docs": [
        "Updates reference addresses in the offramp contract, such as",
        "the CCIP router, Fee Quoter, and the Offramp Lookup Table.",
        "Only the Admin may update these addresses.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for updating the reference addresses.",
        "* `router` - The router address to be set.",
        "* `fee_quoter` - The fee_quoter address to be set.",
        "* `offramp_lookup_table` - The offramp_lookup_table address to be set.",
        "* `rmn_remote` - The rmn_remote address to be set."
      ],
      "discriminator": [
        119,
        179,
        218,
        249,
        217,
        184,
        181,
        9
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
          "name": "referenceAddresses",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  114,
                  101,
                  102,
                  101,
                  114,
                  101,
                  110,
                  99,
                  101,
                  95,
                  97,
                  100,
                  100,
                  114,
                  101,
                  115,
                  115,
                  101,
                  115
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
          "name": "router",
          "type": "pubkey"
        },
        {
          "name": "feeQuoter",
          "type": "pubkey"
        },
        {
          "name": "offrampLookupTable",
          "type": "pubkey"
        },
        {
          "name": "rmnRemote",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "updateSourceChainConfig",
      "docs": [
        "Updates the configuration of the source chain selector.",
        "",
        "The Admin is the only one able to update the source chain config.",
        "",
        "The config is replaced in full, `on_ramps` included: the provided list becomes the complete",
        "set of OnRamps allowed to send from this source chain. When adding an OnRamp (e.g. because the",
        "source chain is upgrading its own), pass the OnRamps to keep alongside the new one — dropping",
        "an OnRamp that still has inflight messages leaves those messages unexecutable while it is",
        "missing. That is recoverable: re-adding the OnRamp makes them executable again, because the",
        "check runs at execution time and a rejected execution reverts without recording anything.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for updating the chain selector.",
        "* `params` - The parameters for updating the source chain, including the selector and new configuration."
      ],
      "discriminator": [
        52,
        85,
        37,
        124,
        209,
        140,
        181,
        104
      ],
      "accounts": [
        {
          "name": "sourceChain",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  115,
                  111,
                  117,
                  114,
                  99,
                  101,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  95,
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "params.sourceChainSelector"
              }
            ]
          }
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
              "name": "updateSourceChainParams"
            }
          }
        }
      ]
    },
    {
      "name": "updateSvmChainSelector",
      "docs": [
        "Updates the SVM chain selector in the offramp configuration.",
        "",
        "This method should only be used if there was an error with the initial configuration or if the solana chain selector changes.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for updating the configuration.",
        "* `new_chain_selector` - The new chain selector for SVM."
      ],
      "discriminator": [
        164,
        212,
        71,
        101,
        166,
        113,
        26,
        93
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
          "name": "newChainSelector",
          "type": "u64"
        }
      ]
    }
  ],
  "accounts": [
    {
      "name": "buffer",
      "discriminator": [
        115,
        5,
        212,
        192,
        85,
        30,
        46,
        41
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
      "name": "messageState",
      "discriminator": [
        197,
        165,
        251,
        177,
        41,
        114,
        136,
        150
      ]
    },
    {
      "name": "referenceAddresses",
      "discriminator": [
        99,
        5,
        216,
        212,
        250,
        75,
        74,
        12
      ]
    },
    {
      "name": "sourceChain",
      "discriminator": [
        242,
        235,
        220,
        98,
        252,
        121,
        191,
        216
      ]
    }
  ],
  "events": [
    {
      "name": "bufferClosed",
      "discriminator": [
        199,
        220,
        232,
        142,
        169,
        28,
        45,
        247
      ]
    },
    {
      "name": "bufferInitialized",
      "discriminator": [
        161,
        15,
        152,
        49,
        216,
        102,
        155,
        52
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
      "name": "executionStateChangedV2",
      "discriminator": [
        78,
        227,
        244,
        22,
        122,
        132,
        87,
        159
      ]
    },
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
      "name": "referenceAddressesSet",
      "discriminator": [
        146,
        234,
        139,
        115,
        99,
        143,
        216,
        191
      ]
    },
    {
      "name": "sourceChainAdded",
      "discriminator": [
        98,
        127,
        170,
        88,
        67,
        55,
        230,
        8
      ]
    },
    {
      "name": "sourceChainConfigUpdated",
      "discriminator": [
        31,
        205,
        106,
        132,
        10,
        220,
        181,
        30
      ]
    }
  ],
  "errors": [
    {
      "code": 9000,
      "name": "invalidRmnRemoteAddress",
      "msg": "Invalid RMN Remote Address"
    },
    {
      "code": 9001,
      "name": "unauthorized",
      "msg": "The signer is unauthorized"
    },
    {
      "code": 9002,
      "name": "invalidInputsMissingWritable",
      "msg": "Account should be writable"
    },
    {
      "code": 9003,
      "name": "onrampNotConfigured",
      "msg": "Onramp was not configured"
    },
    {
      "code": 9004,
      "name": "invalidVersion",
      "msg": "Invalid version of the onchain state"
    },
    {
      "code": 9005,
      "name": "redundantOwnerProposal",
      "msg": "Proposed owner is the current owner"
    },
    {
      "code": 9006,
      "name": "unsupportedSourceChainSelector",
      "msg": "Source chain selector not supported"
    },
    {
      "code": 9007,
      "name": "invalidMessageDestChainSelector",
      "msg": "Invalid message dest chain selector"
    },
    {
      "code": 9008,
      "name": "invalidInputsPoolAccounts",
      "msg": "Invalid pool accounts"
    },
    {
      "code": 9009,
      "name": "invalidInputsSysvarAccount",
      "msg": "Invalid sysvar instructions account"
    },
    {
      "code": 9010,
      "name": "invalidInputsAllowedOfframpAccount",
      "msg": "Invalid offramp authorization account"
    },
    {
      "code": 9011,
      "name": "offrampReleaseMintBalanceMismatch",
      "msg": "Release or mint balance mismatch"
    },
    {
      "code": 9012,
      "name": "offrampInvalidDataLength",
      "msg": "Invalid data length"
    },
    {
      "code": 9013,
      "name": "invalidCodeVersion",
      "msg": "Invalid code version"
    },
    {
      "code": 9014,
      "name": "invalidOnrampAddress",
      "msg": "Invalid onramp address"
    },
    {
      "code": 9015,
      "name": "invalidOfframpAddress",
      "msg": "Invalid offramp address"
    },
    {
      "code": 9016,
      "name": "bufferAlreadyContainsChunk",
      "msg": "The buffer already contains that chunk"
    },
    {
      "code": 9017,
      "name": "bufferAlreadyInitialized",
      "msg": "The buffer is already initialized"
    },
    {
      "code": 9018,
      "name": "bufferInvalidLength",
      "msg": "Invalid length for buffer"
    },
    {
      "code": 9019,
      "name": "bufferInvalidChunkIndex",
      "msg": "Chunk lies outside the buffer"
    },
    {
      "code": 9020,
      "name": "bufferInvalidChunkNumber",
      "msg": "Total number of chunks is not consistent"
    },
    {
      "code": 9021,
      "name": "bufferInvalidChunkSize",
      "msg": "Invalid chunk size"
    },
    {
      "code": 9022,
      "name": "bufferIncomplete",
      "msg": "Buffer is not complete: chunks are missing"
    },
    {
      "code": 9023,
      "name": "bufferUnavailable",
      "msg": "Buffer is not available"
    },
    {
      "code": 9024,
      "name": "invalidTokenPoolAccountDerivationResponse",
      "msg": "Token pool returned an unexpected derivation response"
    },
    {
      "code": 9025,
      "name": "defaultOwnerProposal",
      "msg": "Proposed owner is the default pubkey"
    },
    {
      "code": 9026,
      "name": "invalidAccountListLengths",
      "msg": "Invalid account list lengths"
    },
    {
      "code": 9027,
      "name": "invalidInstructionData",
      "msg": "Invalid instruction data"
    },
    {
      "code": 9028,
      "name": "invalidDestBlob",
      "msg": "Invalid dest blob"
    },
    {
      "code": 9029,
      "name": "invalidReceiverAddress",
      "msg": "Invalid receiver address"
    },
    {
      "code": 9030,
      "name": "invalidMetadata",
      "msg": "Invalid metadata"
    },
    {
      "code": 9031,
      "name": "missingCpiReturnData",
      "msg": "CPI target did not return data"
    },
    {
      "code": 9032,
      "name": "invalidCpiReturnProgram",
      "msg": "CPI return data came from an unexpected program"
    },
    {
      "code": 9033,
      "name": "invalidInputsNumberOfCcVs",
      "msg": "Invalid number of CCVs in input"
    },
    {
      "code": 9034,
      "name": "invalidCcvAccountDerivationResponse",
      "msg": "Invalid CCV account derivation response"
    },
    {
      "code": 9035,
      "name": "invalidInputPubkeyBytes",
      "msg": "Invalid address bytes in input"
    },
    {
      "code": 9036,
      "name": "invalidCcvAccountResolutionResponse",
      "msg": "Invalid CCV account resolution response"
    },
    {
      "code": 9037,
      "name": "invalidInputsUnexpectedSigner",
      "msg": "Unexpected signer account in account list"
    },
    {
      "code": 9038,
      "name": "tooManyMessagingAccounts",
      "msg": "Too many messaging accounts"
    },
    {
      "code": 9039,
      "name": "failedToDeserializeBuffer",
      "msg": "Failed to deserialize buffer"
    },
    {
      "code": 9040,
      "name": "mustSpecifyDefaultOrMandatedCcVs",
      "msg": "Must specify at least one default or lane-mandated CCV"
    },
    {
      "code": 9041,
      "name": "zeroPubkeyNotAllowedInCcvConfig",
      "msg": "Zero pubkey is not allowed in CCV config"
    },
    {
      "code": 9042,
      "name": "duplicateCcvNotAllowed",
      "msg": "Duplicate CCV is not allowed"
    },
    {
      "code": 9043,
      "name": "requiredCcvMissing",
      "msg": "Required CCV missing from execute input"
    },
    {
      "code": 9044,
      "name": "optionalCcvQuorumNotReached",
      "msg": "Optional CCV quorum not reached"
    },
    {
      "code": 9045,
      "name": "invalidOptionalThreshold",
      "msg": "Invalid optional CCV threshold"
    },
    {
      "code": 9046,
      "name": "requestedFinalityNotAllowed",
      "msg": "Message requested finality not allowed by the receiver"
    },
    {
      "code": 9047,
      "name": "unsupportedPoolV1ResolutionOnLaneV2",
      "msg": "V1 pools with account resolution (CCTP/Lombard) are not supported on the v2 lane"
    },
    {
      "code": 9048,
      "name": "invalidInputAssociatedTokenAccountProgram",
      "msg": "Invalid associated token account program"
    },
    {
      "code": 9049,
      "name": "unsupportedTokenReceiverUseAsIsForPoolV1",
      "msg": "Unsupported use of 'UseAsIs' token receiver usage for pool v1"
    },
    {
      "code": 9050,
      "name": "invalidExternalExecutionSigner",
      "msg": "Invalid external execution signer account for the receiver CPI"
    },
    {
      "code": 9051,
      "name": "invalidMessage",
      "msg": "Failed to serialize the receiver message"
    },
    {
      "code": 9052,
      "name": "invalidReceiverAccounts",
      "msg": "Receiver CPI accounts do not match the signed message dest blob"
    },
    {
      "code": 9053,
      "name": "duplicateOnRampNotAllowed",
      "msg": "Duplicate onramp is not allowed"
    },
    {
      "code": 9054,
      "name": "tooManyOnRamps",
      "msg": "Too many onramps configured for the source chain"
    },
    {
      "code": 9055,
      "name": "wrongBufferAuthority",
      "msg": "The buffer authority is not the caller"
    },
    {
      "code": 9056,
      "name": "redundantBuffer",
      "msg": "There is a redundant buffer account as the message is in the ix data already"
    },
    {
      "code": 9057,
      "name": "invalidMessageSourceChainSelector",
      "msg": "The source chain account does not match the message source chain selector"
    },
    {
      "code": 9058,
      "name": "receiverRegistryAccountMissing",
      "msg": "Receiver registry account missing for arbitrary message"
    }
  ],
  "types": [
    {
      "name": "addSourceChainParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "newChainSelector",
            "type": "u64"
          },
          {
            "name": "sourceChainConfig",
            "type": {
              "defined": {
                "name": "sourceChainConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "buffer",
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
            "name": "bufferId",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          },
          {
            "name": "authority",
            "type": "pubkey"
          },
          {
            "name": "chunkBitmap",
            "type": "u64"
          },
          {
            "name": "numChunks",
            "type": "u8"
          },
          {
            "name": "chunkLength",
            "type": "u32"
          },
          {
            "name": "data",
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "bufferClosed",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "bufferId",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          },
          {
            "name": "authority",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "bufferExecutionInputsParams",
      "docs": [
        "Buffers execution inputs in chunks to support large messages that exceed transaction size limits.",
        "Works with all ExecutionInputsV2 payloads regardless of message type:",
        "- Token transfer only messages",
        "- Arbitrary message only messages",
        "- Combined token transfer + arbitrary message (largest, most common case for buffering)",
        "",
        "# Parameters",
        "",
        "* `buffer_id` - A 32-byte buffer id defined by the caller (typically the message_id).",
        "* `total_length` - Total length in bytes of the serialized execution inputs.",
        "* `chunk` - The specific chunk to add to the buffer. Chunk must have a consistent size, except",
        "the last one in the buffer, which may be smaller.",
        "* `chunk_index` - The index of this chunk.",
        "* `num_chunks` - The total number of chunks in the report."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "bufferId",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          },
          {
            "name": "totalLength",
            "type": "u32"
          },
          {
            "name": "chunk",
            "type": "bytes"
          },
          {
            "name": "chunkIndex",
            "type": "u8"
          },
          {
            "name": "numChunks",
            "type": "u8"
          }
        ]
      }
    },
    {
      "name": "bufferInitialized",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "bufferId",
            "type": {
              "array": [
                "u8",
                32
              ]
            }
          },
          {
            "name": "authority",
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
      "name": "closeExecutionInputsBufferParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "bufferId",
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
            "name": "svmChainSelector",
            "type": "u64"
          },
          {
            "name": "owner",
            "type": "pubkey"
          },
          {
            "name": "proposedOwner",
            "type": "pubkey"
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
            "name": "svmChainSelector",
            "type": "u64"
          }
        ]
      }
    },
    {
      "name": "crossChainAmount",
      "docs": [
        "Legacy Cross-chain amount representation (u256 in little-endian bytes).",
        "Kept for backwards compatibility with V1 pools that still expect little-endian.",
        "Note that CCIP 2.0 code uses ProtocolAmount instead, which is BE in accordance",
        "to the CCIP cross-chain spec."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "leBytes",
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
      "name": "executeParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "execInputs",
            "docs": [
              "When present, it is the execution inputs directly. When missing, there _must_ be a buffer account with the",
              "exec_inputs bytes. Account resolution for buffered execution must receive that same buffer account as an",
              "additional remaining account on `resolve_accounts_start`, so the resolver can read the inputs."
            ],
            "type": {
              "option": {
                "defined": {
                  "name": "executionInputsV2"
                }
              }
            }
          },
          {
            "name": "resolutionMetadata",
            "docs": [
              "Opaque account-resolution metadata produced by account resolution. The wire format is the",
              "borsh-encoded [`ExecuteResolutionMetadata`] struct, which carries both the per-section",
              "account-list lengths and the message_id used to derive the `message_exec_state` PDA at constraint time."
            ],
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "executionInputsV2",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "encodedMessage",
            "type": "bytes"
          },
          {
            "name": "ccvs",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "verifierResults",
            "type": {
              "vec": "bytes"
            }
          }
        ]
      }
    },
    {
      "name": "executionStateChangedV2",
      "docs": [
        "Mirrors EVM's `OffRamp.ExecutionStateChanged(sourceChainSelector, messageNumber, messageId,",
        "state, returnData)` — same fields, same order.",
        "",
        "`message_number` follows EVM, which names this field `messageNumber` in both the event and",
        "`MessageV1Codec.MessageV1`. Stellar calls it `sequence_number`; EVM is the reference."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "sourceChainSelector",
            "type": "u64"
          },
          {
            "name": "messageNumber",
            "type": "u64"
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
            "name": "state",
            "type": {
              "defined": {
                "name": "messageExecutionState"
              }
            }
          },
          {
            "name": "returnData",
            "type": "bytes"
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
      "name": "getCcvsForMsgParams",
      "type": {
        "kind": "struct",
        "fields": [
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
            "name": "messageReceiver",
            "type": "pubkey"
          },
          {
            "name": "dataLen",
            "docs": [
              "Length of the message's `data` field, and the callback gas it requests. Together with",
              "`message_receiver` these decide whether the receiver is consulted at all."
            ],
            "type": "u32"
          },
          {
            "name": "ccipReceiveGasLimit",
            "type": "u32"
          },
          {
            "name": "sender",
            "docs": [
              "Cross-chain sender, forwarded to a v2 receiver's `get_ccvs_and_finality_config` query so the",
              "predicted CCV set matches what `execute_v2` computes for the same message."
            ],
            "type": "bytes"
          },
          {
            "name": "resolutionMetadata",
            "type": "bytes"
          },
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "requestedFinality",
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
      "name": "getCcvsForMsgResponse",
      "docs": [
        "Response from CCV resolution."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "requiredCcvs",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "optionalCcvs",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "optionalThreshold",
            "type": "u8"
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
            "name": "crossChainAmount",
            "type": {
              "defined": {
                "name": "crossChainAmount"
              }
            }
          },
          {
            "name": "protocolAmount",
            "type": {
              "defined": {
                "name": "protocolAmount"
              }
            }
          }
        ]
      }
    },
    {
      "name": "messageExecutionState",
      "type": {
        "kind": "enum",
        "variants": [
          {
            "name": "untouched"
          },
          {
            "name": "inProgress"
          },
          {
            "name": "success"
          },
          {
            "name": "failure"
          }
        ]
      }
    },
    {
      "name": "messageState",
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
            "name": "executionState",
            "type": {
              "defined": {
                "name": "messageExecutionState"
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
          }
        ]
      }
    },
    {
      "name": "offRampConfigObservation",
      "docs": [
        "Global configuration of the offramp, as observed by off-chain consumers. Combines the `config`",
        "and `reference_addresses` PDAs, which are always initialized together."
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
            "name": "svmChainSelector",
            "type": "u64"
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
            "name": "referenceAddressesVersion",
            "type": "u8"
          },
          {
            "name": "router",
            "type": "pubkey"
          },
          {
            "name": "feeQuoter",
            "type": "pubkey"
          },
          {
            "name": "offrampLookupTable",
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
      "name": "offRampSourceChainObservation",
      "docs": [
        "Per-source-chain configuration of the offramp, as observed by off-chain consumers."
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
            "name": "chainSelector",
            "type": "u64"
          },
          {
            "name": "isEnabled",
            "type": "bool"
          },
          {
            "name": "onRamps",
            "docs": [
              "OnRamp addresses allowed to send from the source chain, as raw bytes (not ABI encoded)."
            ],
            "type": {
              "vec": "bytes"
            }
          },
          {
            "name": "defaultCcvs",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "laneMandatedCcvs",
            "type": {
              "vec": "pubkey"
            }
          }
        ]
      }
    },
    {
      "name": "onRampAddress",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "bytes",
            "docs": [
              "Private: only the first `len` bytes are meaningful, the rest is zero padding.",
              "Go through [`OnRampAddress::bytes`] to read the address."
            ],
            "type": {
              "array": [
                "u8",
                64
              ]
            }
          },
          {
            "name": "len",
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
      "name": "referenceAddresses",
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
            "name": "router",
            "type": "pubkey"
          },
          {
            "name": "feeQuoter",
            "type": "pubkey"
          },
          {
            "name": "offrampLookupTable",
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
      "name": "referenceAddressesSet",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "router",
            "type": "pubkey"
          },
          {
            "name": "feeQuoter",
            "type": "pubkey"
          },
          {
            "name": "offrampLookupTable",
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
      "name": "sourceChain",
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
            "name": "config",
            "type": {
              "defined": {
                "name": "sourceChainConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "sourceChainAdded",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "sourceChainSelector",
            "type": "u64"
          },
          {
            "name": "sourceChainConfig",
            "type": {
              "defined": {
                "name": "sourceChainConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "sourceChainConfig",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "isEnabled",
            "type": "bool"
          },
          {
            "name": "onRamps",
            "docs": [
              "OnRamps allowed to send messages from this source chain. A message is accepted when it",
              "comes from any of them. Must be non-empty, hold at most [`MAX_ON_RAMPS`] entries, and",
              "contain no duplicate, empty or zero addresses."
            ],
            "type": {
              "vec": {
                "defined": {
                  "name": "onRampAddress"
                }
              }
            }
          },
          {
            "name": "defaultCcvs",
            "docs": [
              "Default CCVs for the lane. Must be non-empty.",
              "No Pubkey::default() or duplicates allowed."
            ],
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "laneMandatedCcvs",
            "docs": [
              "Lane-mandated CCVs that are always required for this lane.",
              "No Pubkey::default() or duplicates allowed (with default_ccvs or in lane_mandated_ccvs)."
            ],
            "type": {
              "vec": "pubkey"
            }
          }
        ]
      }
    },
    {
      "name": "sourceChainConfigUpdated",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "sourceChainSelector",
            "type": "u64"
          },
          {
            "name": "sourceChainConfig",
            "type": {
              "defined": {
                "name": "sourceChainConfig"
              }
            }
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
      "name": "updateSourceChainParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "sourceChainSelector",
            "type": "u64"
          },
          {
            "name": "sourceChainConfig",
            "type": {
              "defined": {
                "name": "sourceChainConfig"
              }
            }
          }
        ]
      }
    }
  ]
};
