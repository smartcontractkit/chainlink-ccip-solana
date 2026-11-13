/**
 * Program IDL in camelCase format in order to be used in JS/TS.
 *
 * Note that this is only a type helper and is not the actual IDL. The original
 * IDL can be found at `target/idl/fee_quoter.json`.
 */
export type FeeQuoter = {
  "address": "FeeQPGkKDeRV1MgoYfMH6L8o3KeuYjwUZrgn4LRKfjHi",
  "metadata": {
    "name": "feeQuoter",
    "version": "2.1.0",
    "spec": "0.1.0",
    "description": "Created with Anchor"
  },
  "instructions": [
    {
      "name": "acceptOwnership",
      "docs": [
        "Accepts the ownership of the fee quoter by the proposed owner.",
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
        }
      ],
      "args": []
    },
    {
      "name": "addBillingTokenConfig",
      "docs": [
        "Adds a billing token configuration.",
        "Only CCIP Admin can add a billing token configuration.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for adding the billing token configuration.",
        "* `billing_token_config_input` - The billing token configuration to be added."
      ],
      "discriminator": [
        63,
        156,
        254,
        216,
        227,
        53,
        0,
        69
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
          "name": "billingTokenConfig",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  102,
                  101,
                  101,
                  95,
                  98,
                  105,
                  108,
                  108,
                  105,
                  110,
                  103,
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
                "kind": "arg",
                "path": "billingTokenConfigInput.mint"
              }
            ]
          }
        },
        {
          "name": "tokenProgram"
        },
        {
          "name": "feeTokenMint"
        },
        {
          "name": "feeTokenReceiver",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "account",
                "path": "feeBillingSigner"
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
          "name": "authority",
          "writable": true,
          "signer": true
        },
        {
          "name": "feeBillingSigner"
        },
        {
          "name": "associatedTokenProgram",
          "address": "ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL"
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "billingTokenConfigInput",
          "type": {
            "defined": {
              "name": "billingTokenConfig"
            }
          }
        }
      ]
    },
    {
      "name": "addDestChain",
      "docs": [
        "Adds a new destination chain selector to the fee quoter.",
        "",
        "The Admin needs to add any new chain supported.",
        "When adding a new chain, the Admin needs to specify if it's enabled or not.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for adding the chain selector.",
        "* `chain_selector` - The new chain selector to be added.",
        "* `dest_chain_config` - The configuration for the chain as destination."
      ],
      "discriminator": [
        122,
        202,
        174,
        155,
        55,
        100,
        102,
        36
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
          "name": "destChain",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  100,
                  101,
                  115,
                  116,
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
                "path": "chainSelector"
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
          "name": "chainSelector",
          "type": "u64"
        },
        {
          "name": "destChainConfig",
          "type": {
            "defined": {
              "name": "destChainConfig"
            }
          }
        }
      ]
    },
    {
      "name": "addPriceUpdater",
      "docs": [
        "Add a price updater address to the list of allowed price updaters.",
        "On price updates, the fee quoter will check the that caller is allowed.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for this operation.",
        "* `price_updater` - The price updater address."
      ],
      "discriminator": [
        200,
        26,
        13,
        120,
        226,
        182,
        64,
        16
      ],
      "accounts": [
        {
          "name": "allowedPriceUpdater",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  97,
                  108,
                  108,
                  111,
                  119,
                  101,
                  100,
                  95,
                  112,
                  114,
                  105,
                  99,
                  101,
                  95,
                  117,
                  112,
                  100,
                  97,
                  116,
                  101,
                  114
                ]
              },
              {
                "kind": "arg",
                "path": "priceUpdater"
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
        }
      ],
      "args": [
        {
          "name": "priceUpdater",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "disableDestChain",
      "docs": [
        "Disables the destination chain selector.",
        "",
        "The Admin is the only one able to disable the chain selector as destination. This method is thought of as an emergency kill-switch.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for disabling the chain selector.",
        "* `chain_selector` - The destination chain selector to be disabled."
      ],
      "discriminator": [
        200,
        195,
        114,
        206,
        152,
        86,
        50,
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
          "name": "destChain",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  100,
                  101,
                  115,
                  116,
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
                "path": "chainSelector"
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
          "name": "chainSelector",
          "type": "u64"
        }
      ]
    },
    {
      "name": "disableDestChainV2",
      "docs": [
        "Disables the destination chain selector (V2).",
        "",
        "The Admin is the only one able to disable the chain selector as destination.",
        "This method is thought of as an emergency kill-switch.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for disabling the chain selector.",
        "* `chain_selector` - The destination chain selector to be disabled."
      ],
      "discriminator": [
        191,
        64,
        87,
        48,
        56,
        140,
        221,
        30
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
          "name": "destChain",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  100,
                  101,
                  115,
                  116,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  95,
                  118,
                  50
                ]
              },
              {
                "kind": "arg",
                "path": "chainSelector"
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
          "name": "chainSelector",
          "type": "u64"
        }
      ]
    },
    {
      "name": "getFallbackTokenTransferFeeV2",
      "docs": [
        "Returns the resolved token transfer fee components for a token transfer.",
        "",
        "This function will check token-specific config first, then fall back to destination chain defaults.",
        "",
        "IMPORTANT: This is a fallback used when the pool doesn't support custom fees and/or the token",
        "pool is V1 in a V2 lane. For V2 token pools, always check it for a receipt first.",
        "",
        "# Arguments",
        "* `dest_chain_selector` - The destination chain selector.",
        "* `mint` - The transferred token mint"
      ],
      "discriminator": [
        38,
        172,
        34,
        91,
        170,
        53,
        118,
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
          "name": "destChain",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  100,
                  101,
                  115,
                  116,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  95,
                  118,
                  50
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
          "name": "perChainPerTokenConfig",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  112,
                  101,
                  114,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  95,
                  112,
                  101,
                  114,
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
                "kind": "arg",
                "path": "params.destChainSelector"
              },
              {
                "kind": "arg",
                "path": "params.mint"
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
              "name": "getFallbackTokenTransferFeeV2Params"
            }
          }
        }
      ],
      "returns": {
        "defined": {
          "name": "fallbackTokenTransferFeeV2"
        }
      }
    },
    {
      "name": "getFee",
      "docs": [
        "Calculates the fee for sending a message to the destination chain.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for the fee calculation.",
        "* `dest_chain_selector` - The chain selector for the destination chain.",
        "* `message` - The message to be sent.",
        "",
        "# Additional accounts",
        "",
        "In addition to the fixed amount of accounts defined in the `GetFee` context,",
        "the following accounts must be provided:",
        "",
        "* First, the billing token config accounts for each token sent with the message, sequentially.",
        "For each token with no billing config account (i.e. tokens that cannot be possibly used as fee",
        "tokens, which also have no BPS fees enabled) the ZERO address must be provided instead.",
        "* Then, the per chain / per token config of every token sent with the message, sequentially",
        "in the same order.",
        "",
        "# Returns",
        "",
        "GetFeeResult struct with:",
        "- the fee token mint address,",
        "- the fee amount of said token,",
        "- the fee value in juels,",
        "- additional data required when performing the cross-chain transfer of tokens in that message",
        "- deserialized and processed extra args"
      ],
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
          "name": "destChain",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  100,
                  101,
                  115,
                  116,
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
                "path": "destChainSelector"
              }
            ]
          }
        },
        {
          "name": "billingTokenConfig",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  102,
                  101,
                  101,
                  95,
                  98,
                  105,
                  108,
                  108,
                  105,
                  110,
                  103,
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
                "kind": "arg",
                "path": "message.feeToken"
              }
            ]
          }
        },
        {
          "name": "linkTokenConfig",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  102,
                  101,
                  101,
                  95,
                  98,
                  105,
                  108,
                  108,
                  105,
                  110,
                  103,
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
                "path": "config.linkTokenMint",
                "account": "config"
              }
            ]
          }
        }
      ],
      "args": [
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
        }
      ],
      "returns": {
        "defined": {
          "name": "getFeeResult"
        }
      }
    },
    {
      "name": "getValidatedTokenPriceV2",
      "docs": [
        "Returns the token's validated USD price (18 decimals) from its billing token config.",
        "",
        "Fails with `InvalidTokenPrice` if the price was never set (zero value or timestamp).",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the billing token config account for the mint.",
        "* `mint` - The token mint to get the price for."
      ],
      "discriminator": [
        237,
        11,
        68,
        190,
        126,
        242,
        221,
        197
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
          "name": "billingTokenConfig",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  102,
                  101,
                  101,
                  95,
                  98,
                  105,
                  108,
                  108,
                  105,
                  110,
                  103,
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
          "name": "validatedTokenPriceV2"
        }
      }
    },
    {
      "name": "initialize",
      "docs": [
        "Initializes the Fee Quoter.",
        "",
        "The initialization is responsibility of Admin, nothing more than calling this method should be done first.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for initialization.",
        "* `max_fee_juels_per_msg` - The maximum fee in juels that can be charged per message.",
        "* `onramp` - The public key of the onramp.",
        "",
        "The function also uses the link_token_mint account from the context."
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
          "name": "linkTokenMint"
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
          "address": "FeeQPGkKDeRV1MgoYfMH6L8o3KeuYjwUZrgn4LRKfjHi"
        },
        {
          "name": "programData"
        }
      ],
      "args": [
        {
          "name": "maxFeeJuelsPerMsg",
          "type": "u128"
        },
        {
          "name": "onramp",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "observeConfig",
      "docs": [
        "Returns the fee quoter's global configuration, for observability consumers.",
        "",
        "Permissionless and read-only: meant to be executed through `simulateTransaction`.",
        "",
        "# Arguments",
        "* `ctx` - The context containing the fee quoter config account"
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
          "name": "feeQuoterConfigObservation"
        }
      }
    },
    {
      "name": "observeDestChainV2",
      "docs": [
        "Returns the CCIP 2.0 fee configuration and gas price state for a single destination chain,",
        "for observability consumers.",
        "",
        "Permissionless and read-only: meant to be executed through `simulateTransaction`. Fails if",
        "the lane has no `dest_chain_v2` account, which callers can read as \"not configured\".",
        "",
        "# Arguments",
        "* `ctx` - The context containing the dest chain account",
        "* `chain_selector` - The destination chain selector the lane belongs to"
      ],
      "discriminator": [
        10,
        114,
        200,
        19,
        108,
        52,
        129,
        170
      ],
      "accounts": [
        {
          "name": "destChain",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  100,
                  101,
                  115,
                  116,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  95,
                  118,
                  50
                ]
              },
              {
                "kind": "arg",
                "path": "chainSelector"
              }
            ]
          }
        },
        {
          "name": "destChainLegacy",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  100,
                  101,
                  115,
                  116,
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
                "path": "chainSelector"
              }
            ]
          }
        }
      ],
      "args": [
        {
          "name": "chainSelector",
          "type": "u64"
        }
      ],
      "returns": {
        "defined": {
          "name": "feeQuoterDestChainV2Observation"
        }
      }
    },
    {
      "name": "quoteGasForExecV2",
      "docs": [
        "Returns the gas quote for execution. This involves making a correction",
        "of the total gas required, as well as returning the complete cost for",
        "all (corrected) gas.",
        "",
        "It also returns the premium multiplier to use for the particular fee",
        "token.",
        "",
        "# Arguments",
        "* `params` - Parameters describing the execution, including the total byte",
        "overhead, total gas before correction, and fee information."
      ],
      "discriminator": [
        129,
        107,
        201,
        224,
        100,
        254,
        46,
        123
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
          "name": "destChain",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  100,
                  101,
                  115,
                  116,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  95,
                  118,
                  50
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
          "name": "legacyDestChain",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  100,
                  101,
                  115,
                  116,
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
          "name": "billingTokenConfig",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  102,
                  101,
                  101,
                  95,
                  98,
                  105,
                  108,
                  108,
                  105,
                  110,
                  103,
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
                "kind": "arg",
                "path": "params.feeTokenMint"
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
              "name": "quoteGasForExecV2Params"
            }
          }
        }
      ],
      "returns": {
        "defined": {
          "name": "quoteGasForExecV2Result"
        }
      }
    },
    {
      "name": "removePriceUpdater",
      "docs": [
        "Remove a price updater address from the list of allowed price updaters.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for this operation.",
        "* `price_updater` - The price updater address."
      ],
      "discriminator": [
        10,
        61,
        172,
        48,
        110,
        8,
        162,
        198
      ],
      "accounts": [
        {
          "name": "allowedPriceUpdater",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  97,
                  108,
                  108,
                  111,
                  119,
                  101,
                  100,
                  95,
                  112,
                  114,
                  105,
                  99,
                  101,
                  95,
                  117,
                  112,
                  100,
                  97,
                  116,
                  101,
                  114
                ]
              },
              {
                "kind": "arg",
                "path": "priceUpdater"
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
        }
      ],
      "args": [
        {
          "name": "priceUpdater",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "setDefaultCodeVersion",
      "docs": [
        "Sets the default code version to be used. This is then used by the slim routing layer to determine",
        "which version of the versioned business logic module (`instructions`) to use. Only the admin may set this.",
        "",
        "Shared func signature with other programs",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for updating the configuration.",
        "* `code_version` - The new code version to be set as default."
      ],
      "discriminator": [
        47,
        151,
        233,
        254,
        121,
        82,
        206,
        152
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
        }
      ],
      "args": [
        {
          "name": "codeVersion",
          "type": {
            "defined": {
              "name": "codeVersion"
            }
          }
        }
      ]
    },
    {
      "name": "setLinkTokenMint",
      "docs": [
        "Sets the link_token_mint and updates the link_token_local_decimals.",
        "",
        "Only the admin may set this.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for updating the configuration."
      ],
      "discriminator": [
        190,
        216,
        49,
        254,
        200,
        81,
        12,
        17
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
          "name": "linkTokenMint"
        },
        {
          "name": "authority",
          "signer": true
        }
      ],
      "args": []
    },
    {
      "name": "setMaxFeeJuelsPerMsg",
      "docs": [
        "Sets the max_fee_juels_per_msg, which is an upper bound on how much can be billed for any message.",
        "(1 juels = 1e-18 LINK)",
        "",
        "Only the admin may set this.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for updating the configuration.",
        "* `max_fee_juels_per_msg` - The new value for the max_feel_juels_per_msg config."
      ],
      "discriminator": [
        50,
        235,
        110,
        147,
        169,
        199,
        69,
        46
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
        }
      ],
      "args": [
        {
          "name": "maxFeeJuelsPerMsg",
          "type": "u128"
        }
      ]
    },
    {
      "name": "setTokenTransferFeeConfig",
      "docs": [
        "Sets the token transfer fee configuration for a particular token when it's transferred to a particular dest chain.",
        "It is an upsert, initializing the per-chain-per-token config account if it doesn't exist",
        "and overwriting it if it does.",
        "",
        "Only the Admin can perform this operation.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for setting the token billing configuration.",
        "* `chain_selector` - The chain selector.",
        "* `mint` - The public key of the token mint.",
        "* `cfg` - The token transfer fee configuration."
      ],
      "discriminator": [
        76,
        243,
        16,
        214,
        126,
        11,
        254,
        77
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
          "name": "perChainPerTokenConfig",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  112,
                  101,
                  114,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  95,
                  112,
                  101,
                  114,
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
                "kind": "arg",
                "path": "chainSelector"
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
          "name": "chainSelector",
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
              "name": "tokenTransferFeeConfig"
            }
          }
        }
      ]
    },
    {
      "name": "transferOwnership",
      "docs": [
        "Transfers the ownership of the fee quoter to a new proposed owner.",
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
        }
      ],
      "args": [
        {
          "name": "newOwner",
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
      "name": "updateBillingTokenConfig",
      "docs": [
        "Updates the billing token configuration.",
        "Only CCIP Admin can update a billing token configuration.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for updating the billing token configuration.",
        "* `billing_token_config_input` - The new billing token configuration."
      ],
      "discriminator": [
        140,
        184,
        124,
        146,
        204,
        62,
        244,
        79
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
          "name": "billingTokenConfig",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  102,
                  101,
                  101,
                  95,
                  98,
                  105,
                  108,
                  108,
                  105,
                  110,
                  103,
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
                "kind": "arg",
                "path": "billingTokenConfigInput.mint"
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
          "name": "billingTokenConfigInput",
          "type": {
            "defined": {
              "name": "billingTokenConfig"
            }
          }
        }
      ]
    },
    {
      "name": "updateDestChainConfig",
      "docs": [
        "Updates the configuration of the destination chain selector.",
        "",
        "The Admin is the only one able to update the destination chain config.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for updating the chain selector.",
        "* `chain_selector` - The destination chain selector to be updated.",
        "* `dest_chain_config` - The new configuration for the destination chain."
      ],
      "discriminator": [
        215,
        122,
        81,
        22,
        190,
        58,
        219,
        13
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
          "name": "destChain",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  100,
                  101,
                  115,
                  116,
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
                "path": "chainSelector"
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
          "name": "chainSelector",
          "type": "u64"
        },
        {
          "name": "destChainConfig",
          "type": {
            "defined": {
              "name": "destChainConfig"
            }
          }
        }
      ]
    },
    {
      "name": "updatePrices",
      "docs": [
        "Updates prices for tokens and gas. This method may only be called by an allowed price updater.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts always required for the price updates",
        "* `token_updates` - Vector of token price updates",
        "* `gas_updates` - Vector of gas price updates",
        "",
        "# Additional accounts",
        "",
        "In addition to the fixed amount of accounts defined in the `UpdatePrices` context,",
        "the following accounts must be provided:",
        "",
        "* First, the billing token config accounts for each token whose price is being updated, in the same order",
        "as the token_updates vector.",
        "* Then, the dest chain accounts of every chain whose gas price is being updated, in the same order as the",
        "gas_updates vector."
      ],
      "discriminator": [
        62,
        161,
        234,
        136,
        106,
        26,
        18,
        160
      ],
      "accounts": [
        {
          "name": "authority",
          "signer": true
        },
        {
          "name": "allowedPriceUpdater",
          "docs": [
            "was added by the owner as an allowed price updater. The constraints enforced guarantee that it is the right PDA",
            "and that it was initialized."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  97,
                  108,
                  108,
                  111,
                  119,
                  101,
                  100,
                  95,
                  112,
                  114,
                  105,
                  99,
                  101,
                  95,
                  117,
                  112,
                  100,
                  97,
                  116,
                  101,
                  114
                ]
              },
              {
                "kind": "account",
                "path": "authority"
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
        }
      ],
      "args": [
        {
          "name": "tokenUpdates",
          "type": {
            "vec": {
              "defined": {
                "name": "tokenPriceUpdate"
              }
            }
          }
        },
        {
          "name": "gasUpdates",
          "type": {
            "vec": {
              "defined": {
                "name": "gasPriceUpdate"
              }
            }
          }
        }
      ]
    },
    {
      "name": "upsertDestChainConfigV2",
      "docs": [
        "Updates the configuration of the destination chain selector.",
        "",
        "The Admin is the only one able to update the destination chain config.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for updating the chain selector.",
        "* `params` - The params including the chain selector and the new configs."
      ],
      "discriminator": [
        132,
        50,
        154,
        141,
        174,
        134,
        152,
        90
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
          "name": "destChain",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  100,
                  101,
                  115,
                  116,
                  95,
                  99,
                  104,
                  97,
                  105,
                  110,
                  95,
                  118,
                  50
                ]
              },
              {
                "kind": "arg",
                "path": "params.chainSelector"
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
              "name": "upsertDestChainConfigV2Params"
            }
          }
        }
      ]
    }
  ],
  "accounts": [
    {
      "name": "allowedPriceUpdater",
      "discriminator": [
        51,
        136,
        222,
        161,
        38,
        7,
        184,
        190
      ]
    },
    {
      "name": "billingTokenConfigWrapper",
      "discriminator": [
        63,
        178,
        72,
        57,
        171,
        66,
        44,
        151
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
      "name": "destChain",
      "discriminator": [
        77,
        18,
        241,
        132,
        212,
        54,
        218,
        16
      ]
    },
    {
      "name": "destChainV2",
      "discriminator": [
        245,
        191,
        143,
        179,
        2,
        74,
        218,
        15
      ]
    },
    {
      "name": "perChainPerTokenConfig",
      "discriminator": [
        183,
        88,
        20,
        99,
        246,
        46,
        51,
        230
      ]
    }
  ],
  "events": [
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
      "name": "destChainAdded",
      "discriminator": [
        59,
        154,
        48,
        81,
        230,
        41,
        80,
        200
      ]
    },
    {
      "name": "destChainAddedV2",
      "discriminator": [
        141,
        53,
        181,
        58,
        32,
        54,
        201,
        246
      ]
    },
    {
      "name": "destChainConfigDisabledV2",
      "discriminator": [
        180,
        116,
        32,
        61,
        203,
        255,
        76,
        97
      ]
    },
    {
      "name": "destChainConfigUpdated",
      "discriminator": [
        3,
        141,
        73,
        190,
        73,
        231,
        51,
        80
      ]
    },
    {
      "name": "destChainConfigUpdatedV2",
      "discriminator": [
        140,
        95,
        152,
        252,
        101,
        130,
        96,
        78
      ]
    },
    {
      "name": "feeTokenAdded",
      "discriminator": [
        181,
        180,
        252,
        21,
        215,
        79,
        93,
        237
      ]
    },
    {
      "name": "feeTokenDisabled",
      "discriminator": [
        34,
        139,
        66,
        75,
        30,
        17,
        45,
        151
      ]
    },
    {
      "name": "feeTokenEnabled",
      "discriminator": [
        106,
        180,
        145,
        189,
        113,
        180,
        21,
        15
      ]
    },
    {
      "name": "feeTokenRemoved",
      "discriminator": [
        40,
        31,
        230,
        252,
        183,
        150,
        147,
        201
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
      "name": "premiumMultiplierWeiPerEthUpdated",
      "discriminator": [
        151,
        5,
        223,
        182,
        215,
        187,
        249,
        225
      ]
    },
    {
      "name": "priceUpdaterAdded",
      "discriminator": [
        87,
        31,
        151,
        133,
        151,
        187,
        97,
        186
      ]
    },
    {
      "name": "priceUpdaterRemoved",
      "discriminator": [
        225,
        194,
        40,
        213,
        212,
        39,
        76,
        148
      ]
    },
    {
      "name": "tokenPriceUpdateIgnored",
      "discriminator": [
        68,
        119,
        161,
        131,
        128,
        65,
        69,
        201
      ]
    },
    {
      "name": "tokenTransferFeeConfigUpdated",
      "discriminator": [
        253,
        199,
        166,
        1,
        178,
        150,
        242,
        253
      ]
    },
    {
      "name": "usdPerTokenUpdated",
      "discriminator": [
        67,
        154,
        252,
        56,
        104,
        14,
        192,
        219
      ]
    },
    {
      "name": "usdPerUnitGasUpdated",
      "discriminator": [
        174,
        255,
        2,
        41,
        197,
        110,
        31,
        40
      ]
    }
  ],
  "errors": [
    {
      "code": 8000,
      "name": "unauthorized",
      "msg": "The signer is unauthorized"
    },
    {
      "code": 8001,
      "name": "invalidInputs",
      "msg": "Invalid inputs"
    },
    {
      "code": 8002,
      "name": "zeroGasLimit",
      "msg": "Gas limit is zero"
    },
    {
      "code": 8003,
      "name": "defaultGasLimitExceedsMaximum",
      "msg": "Default gas limit exceeds the maximum"
    },
    {
      "code": 8004,
      "name": "invalidVersion",
      "msg": "Invalid version of the onchain state"
    },
    {
      "code": 8005,
      "name": "redundantOwnerProposal",
      "msg": "Proposed owner is the current owner"
    },
    {
      "code": 8006,
      "name": "invalidInputsMissingWritable",
      "msg": "Account should be writable"
    },
    {
      "code": 8007,
      "name": "invalidInputsChainSelector",
      "msg": "Chain selector is invalid"
    },
    {
      "code": 8008,
      "name": "invalidInputsMint",
      "msg": "Mint account input is invalid"
    },
    {
      "code": 8009,
      "name": "invalidInputsMintOwner",
      "msg": "Mint account input has an invalid owner"
    },
    {
      "code": 8010,
      "name": "invalidInputsTokenConfigAccount",
      "msg": "Token config account is invalid"
    },
    {
      "code": 8011,
      "name": "invalidInputsMissingExtraArgs",
      "msg": "Missing extra args in message to SVM receiver"
    },
    {
      "code": 8012,
      "name": "invalidInputsMissingDataAfterExtraArgs",
      "msg": "Missing data after extra args tag"
    },
    {
      "code": 8013,
      "name": "invalidInputsDestChainStateAccount",
      "msg": "Destination chain state account is invalid"
    },
    {
      "code": 8014,
      "name": "invalidInputsPerChainPerTokenConfig",
      "msg": "Per chain per token config account is invalid"
    },
    {
      "code": 8015,
      "name": "invalidInputsBillingTokenConfig",
      "msg": "Billing token config account is invalid"
    },
    {
      "code": 8016,
      "name": "invalidInputsAccountCount",
      "msg": "Number of accounts provided is incorrect"
    },
    {
      "code": 8017,
      "name": "invalidInputsNoUpdates",
      "msg": "No price or gas update provided"
    },
    {
      "code": 8018,
      "name": "invalidInputsTokenAccounts",
      "msg": "Invalid token accounts"
    },
    {
      "code": 8019,
      "name": "destinationChainDisabled",
      "msg": "Destination chain disabled"
    },
    {
      "code": 8020,
      "name": "feeTokenDisabled",
      "msg": "Fee token disabled"
    },
    {
      "code": 8021,
      "name": "messageTooLarge",
      "msg": "Message exceeds maximum data size"
    },
    {
      "code": 8022,
      "name": "unsupportedNumberOfTokens",
      "msg": "Message contains an unsupported number of tokens"
    },
    {
      "code": 8023,
      "name": "invalidTokenPrice",
      "msg": "Invalid token price"
    },
    {
      "code": 8024,
      "name": "staleGasPrice",
      "msg": "Stale gas price"
    },
    {
      "code": 8025,
      "name": "invalidInputsMissingTokenConfig",
      "msg": "Inputs are missing token configuration"
    },
    {
      "code": 8026,
      "name": "messageFeeTooHigh",
      "msg": "Message fee is too high"
    },
    {
      "code": 8027,
      "name": "messageGasLimitTooHigh",
      "msg": "Message gas limit too high"
    },
    {
      "code": 8028,
      "name": "extraArgOutOfOrderExecutionMustBeTrue",
      "msg": "Extra arg out of order execution must be true"
    },
    {
      "code": 8029,
      "name": "invalidExtraArgsTag",
      "msg": "Invalid extra args tag"
    },
    {
      "code": 8030,
      "name": "invalidExtraArgsAccounts",
      "msg": "Invalid amount of accounts in extra args"
    },
    {
      "code": 8031,
      "name": "invalidExtraArgsWritabilityBitmap",
      "msg": "Invalid writability bitmap in extra args"
    },
    {
      "code": 8032,
      "name": "invalidTokenReceiver",
      "msg": "Invalid token receiver"
    },
    {
      "code": 8033,
      "name": "unauthorizedPriceUpdater",
      "msg": "The caller is not an authorized price updater"
    },
    {
      "code": 8034,
      "name": "invalidTokenTransferFeeMaxMin",
      "msg": "Minimum token transfer fee exceeds maximum"
    },
    {
      "code": 8035,
      "name": "invalidTokenTransferFeeDestBytesOverhead",
      "msg": "Insufficient dest bytes overhead on transfer fee config"
    },
    {
      "code": 8036,
      "name": "invalidCodeVersion",
      "msg": "Invalid code version"
    },
    {
      "code": 8037,
      "name": "defaultOwnerProposal",
      "msg": "Proposed owner is the default pubkey"
    },
    {
      "code": 8038,
      "name": "destChainDisabled",
      "msg": "Dest chain is disabled"
    },
    {
      "code": 8039,
      "name": "priceNeverSet",
      "msg": "Price was never set"
    },
    {
      "code": 8040,
      "name": "invalidSuiReceiverObjectIds",
      "msg": "Invalid Sui receiver object ids"
    },
    {
      "code": 8041,
      "name": "zeroLinkFeeMultiplier",
      "msg": "LINK fee multiplier percent must be greater than zero"
    },
    {
      "code": 8042,
      "name": "destGasPerPayloadByteBaseTooLarge",
      "msg": "Dest gas per payload byte base must not exceed 255"
    },
    {
      "code": 8043,
      "name": "networkFeeTooLarge",
      "msg": "Network fee must not exceed 65535 USD cents"
    }
  ],
  "types": [
    {
      "name": "allowedPriceUpdater",
      "type": {
        "kind": "struct",
        "fields": []
      }
    },
    {
      "name": "billingTokenConfig",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "enabled",
            "type": "bool"
          },
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "usdPerToken",
            "type": {
              "defined": {
                "name": "timestampedPackedU224"
              }
            }
          },
          {
            "name": "premiumMultiplierWeiPerEth",
            "type": "u64"
          }
        ]
      }
    },
    {
      "name": "billingTokenConfigWrapper",
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
                "name": "billingTokenConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "codeVersion",
      "repr": {
        "kind": "rust"
      },
      "type": {
        "kind": "enum",
        "variants": [
          {
            "name": "default"
          },
          {
            "name": "v1"
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
            "name": "owner",
            "type": "pubkey"
          },
          {
            "name": "proposedOwner",
            "type": "pubkey"
          },
          {
            "name": "maxFeeJuelsPerMsg",
            "type": "u128"
          },
          {
            "name": "linkTokenMint",
            "type": "pubkey"
          },
          {
            "name": "linkTokenLocalDecimals",
            "type": "u8"
          },
          {
            "name": "onramp",
            "type": "pubkey"
          },
          {
            "name": "defaultCodeVersion",
            "type": {
              "defined": {
                "name": "codeVersion"
              }
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
            "name": "maxFeeJuelsPerMsg",
            "type": "u128"
          },
          {
            "name": "linkTokenMint",
            "type": "pubkey"
          },
          {
            "name": "linkTokenLocalDecimals",
            "type": "u8"
          },
          {
            "name": "onramp",
            "type": "pubkey"
          },
          {
            "name": "defaultCodeVersion",
            "type": {
              "defined": {
                "name": "codeVersion"
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
      "name": "destChain",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "version",
            "type": "u8"
          },
          {
            "name": "chainSelector",
            "type": "u64"
          },
          {
            "name": "state",
            "type": {
              "defined": {
                "name": "destChainState"
              }
            }
          },
          {
            "name": "config",
            "type": {
              "defined": {
                "name": "destChainConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "destChainAdded",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "destChainConfig",
            "type": {
              "defined": {
                "name": "destChainConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "destChainAddedV2",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "destChainConfig",
            "type": {
              "defined": {
                "name": "destChainConfigV2"
              }
            }
          }
        ]
      }
    },
    {
      "name": "destChainConfig",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "isEnabled",
            "type": "bool"
          },
          {
            "name": "laneCodeVersion",
            "type": {
              "defined": {
                "name": "codeVersion"
              }
            }
          },
          {
            "name": "maxNumberOfTokensPerMsg",
            "type": "u16"
          },
          {
            "name": "maxDataBytes",
            "type": "u32"
          },
          {
            "name": "maxPerMsgGasLimit",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "destGasOverhead",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "destGasPerPayloadByteBase",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "destGasPerPayloadByteHigh",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "destGasPerPayloadByteThreshold",
            "type": "u32"
          },
          {
            "name": "destDataAvailabilityOverheadGas",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "destGasPerDataAvailabilityByte",
            "type": "u16"
          },
          {
            "name": "destDataAvailabilityMultiplierBps",
            "type": "u16"
          },
          {
            "name": "defaultTokenFeeUsdcents",
            "type": "u16"
          },
          {
            "name": "defaultTokenDestGasOverhead",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "defaultTxGasLimit",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "gasMultiplierWeiPerEth",
            "type": "u64"
          },
          {
            "name": "networkFee",
            "type": {
              "defined": {
                "name": "usdCents"
              }
            }
          },
          {
            "name": "gasPriceStalenessThreshold",
            "type": "u32"
          },
          {
            "name": "enforceOutOfOrder",
            "type": "bool"
          },
          {
            "name": "chainFamilySelector",
            "type": {
              "array": [
                "u8",
                4
              ]
            }
          }
        ]
      }
    },
    {
      "name": "destChainConfigDisabledV2",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "destChainConfig",
            "type": {
              "defined": {
                "name": "destChainConfigV2"
              }
            }
          }
        ]
      }
    },
    {
      "name": "destChainConfigUpdated",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "destChainConfig",
            "type": {
              "defined": {
                "name": "destChainConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "destChainConfigUpdatedV2",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "destChainConfig",
            "type": {
              "defined": {
                "name": "destChainConfigV2"
              }
            }
          }
        ]
      }
    },
    {
      "name": "destChainConfigV2",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "isEnabled",
            "type": "bool"
          },
          {
            "name": "laneCodeVersion",
            "type": {
              "defined": {
                "name": "codeVersion"
              }
            }
          },
          {
            "name": "maxDataBytes",
            "type": "u32"
          },
          {
            "name": "maxPerMsgGasLimit",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "destGasOverhead",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "destGasPerPayloadByteBase",
            "type": "u32"
          },
          {
            "name": "chainFamilySelector",
            "type": {
              "array": [
                "u8",
                4
              ]
            }
          },
          {
            "name": "defaultTokenFeeUsdCents",
            "type": "u16"
          },
          {
            "name": "defaultTokenDestGasOverhead",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "defaultTxGasLimit",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "networkFee",
            "type": {
              "defined": {
                "name": "usdCents"
              }
            }
          },
          {
            "name": "linkFeeMultiplierPercent",
            "type": "u8"
          }
        ]
      }
    },
    {
      "name": "destChainState",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "usdPerUnitGas",
            "type": {
              "defined": {
                "name": "timestampedPackedU224"
              }
            }
          }
        ]
      }
    },
    {
      "name": "destChainStateV2",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "usdPerUnitGas",
            "type": {
              "defined": {
                "name": "timestampedPackedU224"
              }
            }
          }
        ]
      }
    },
    {
      "name": "destChainV2",
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
            "name": "state",
            "type": {
              "defined": {
                "name": "destChainStateV2"
              }
            }
          },
          {
            "name": "config",
            "type": {
              "defined": {
                "name": "destChainConfigV2"
              }
            }
          }
        ]
      }
    },
    {
      "name": "fallbackTokenTransferFeeV2",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "flatFee",
            "type": {
              "defined": {
                "name": "usdCents"
              }
            }
          },
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
          }
        ]
      }
    },
    {
      "name": "feeQuoterConfigObservation",
      "docs": [
        "Global configuration of the fee quoter, as observed by off-chain consumers."
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
            "name": "defaultCodeVersion",
            "type": {
              "defined": {
                "name": "codeVersion"
              }
            }
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
            "name": "maxFeeJuelsPerMsg",
            "docs": [
              "Maximum fee per message, with 18 decimals of precision (LINK juels)."
            ],
            "type": "u128"
          },
          {
            "name": "linkTokenMint",
            "type": "pubkey"
          },
          {
            "name": "linkTokenLocalDecimals",
            "type": "u8"
          },
          {
            "name": "onramp",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "feeQuoterDestChainV2Observation",
      "docs": [
        "Per-destination-chain CCIP 2.0 fee configuration and price state, as observed by off-chain",
        "consumers. Read from the `dest_chain_v2` PDA.",
        "",
        "Note that unlike the 1.6 `dest_chain` config, this one carries no gas price staleness",
        "threshold: there is no 2.0 equivalent to report."
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
            "name": "usdPerUnitGas",
            "type": {
              "defined": {
                "name": "timestampedPackedU224"
              }
            }
          },
          {
            "name": "isEnabled",
            "type": "bool"
          },
          {
            "name": "laneCodeVersion",
            "type": {
              "defined": {
                "name": "codeVersion"
              }
            }
          },
          {
            "name": "maxDataBytes",
            "type": "u32"
          },
          {
            "name": "maxPerMsgGasLimit",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "destGasOverhead",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "destGasPerPayloadByteBase",
            "type": "u32"
          },
          {
            "name": "chainFamilySelector",
            "type": {
              "array": [
                "u8",
                4
              ]
            }
          },
          {
            "name": "defaultTokenFeeUsdCents",
            "type": "u16"
          },
          {
            "name": "defaultTokenDestGasOverhead",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "defaultTxGasLimit",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "networkFee",
            "type": {
              "defined": {
                "name": "usdCents"
              }
            }
          },
          {
            "name": "linkFeeMultiplierPercent",
            "type": "u8"
          }
        ]
      }
    },
    {
      "name": "feeTokenAdded",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "feeToken",
            "type": "pubkey"
          },
          {
            "name": "enabled",
            "type": "bool"
          }
        ]
      }
    },
    {
      "name": "feeTokenDisabled",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "feeToken",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "feeTokenEnabled",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "feeToken",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "feeTokenRemoved",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "feeToken",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "gasPriceUpdate",
      "docs": [
        "Gas price for a given chain in USD; its value may contain tightly packed fields."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "usdPerUnitGas",
            "type": {
              "array": [
                "u8",
                28
              ]
            }
          }
        ]
      }
    },
    {
      "name": "getFallbackTokenTransferFeeV2Params",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "mint",
            "type": "pubkey"
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
            "name": "token",
            "type": "pubkey"
          },
          {
            "name": "amount",
            "type": "u64"
          },
          {
            "name": "juels",
            "type": "u128"
          },
          {
            "name": "tokenTransferAdditionalData",
            "type": {
              "vec": {
                "defined": {
                  "name": "tokenTransferAdditionalData"
                }
              }
            }
          },
          {
            "name": "processedExtraArgs",
            "type": {
              "defined": {
                "name": "processedExtraArgs"
              }
            }
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
      "name": "perChainPerTokenConfig",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "version",
            "type": "u8"
          },
          {
            "name": "chainSelector",
            "type": "u64"
          },
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "tokenTransferConfig",
            "type": {
              "defined": {
                "name": "tokenTransferFeeConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "premiumMultiplierWeiPerEthUpdated",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "token",
            "type": "pubkey"
          },
          {
            "name": "premiumMultiplierWeiPerEth",
            "type": "u64"
          }
        ]
      }
    },
    {
      "name": "priceUpdaterAdded",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "priceUpdater",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "priceUpdaterRemoved",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "priceUpdater",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "processedExtraArgs",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "bytes",
            "type": "bytes"
          },
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
              "option": "bytes"
            }
          }
        ]
      }
    },
    {
      "name": "quoteGasForExecV2Params",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "feeTokenMint",
            "type": "pubkey"
          },
          {
            "name": "gasLimitSum",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "bytesOverheadSum",
            "type": "u32"
          }
        ]
      }
    },
    {
      "name": "quoteGasForExecV2Result",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "totalGas",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "totalGasCost",
            "type": {
              "defined": {
                "name": "usdCents"
              }
            }
          },
          {
            "name": "premiumPercentMultiplier",
            "type": "u64"
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
      "name": "timestampedPackedU224",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "value",
            "type": {
              "array": [
                "u8",
                28
              ]
            }
          },
          {
            "name": "timestamp",
            "type": "i64"
          }
        ]
      }
    },
    {
      "name": "tokenPriceUpdate",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "sourceToken",
            "type": "pubkey"
          },
          {
            "name": "usdPerToken",
            "type": {
              "array": [
                "u8",
                28
              ]
            }
          }
        ]
      }
    },
    {
      "name": "tokenPriceUpdateIgnored",
      "docs": [
        "Emitted when the price update corresponds to a token that isn't registered",
        "for price tracking."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "token",
            "type": "pubkey"
          },
          {
            "name": "value",
            "type": {
              "array": [
                "u8",
                28
              ]
            }
          }
        ]
      }
    },
    {
      "name": "tokenTransferAdditionalData",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destBytesOverhead",
            "type": "u32"
          },
          {
            "name": "destGasOverhead",
            "type": {
              "defined": {
                "name": "crossChainGas"
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
            "name": "minFee",
            "type": {
              "defined": {
                "name": "usdCents"
              }
            }
          },
          {
            "name": "maxFee",
            "type": {
              "defined": {
                "name": "usdCents"
              }
            }
          },
          {
            "name": "deciBps",
            "type": "u16"
          },
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
            "name": "isEnabled",
            "type": "bool"
          }
        ]
      }
    },
    {
      "name": "tokenTransferFeeConfigUpdated",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "token",
            "type": "pubkey"
          },
          {
            "name": "tokenTransferFeeConfig",
            "type": {
              "defined": {
                "name": "tokenTransferFeeConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "upsertDestChainConfigV2Params",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "chainSelector",
            "type": "u64"
          },
          {
            "name": "destChainConfig",
            "type": {
              "defined": {
                "name": "destChainConfigV2"
              }
            }
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
      "name": "usdPerTokenUpdated",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "token",
            "type": "pubkey"
          },
          {
            "name": "value",
            "type": {
              "array": [
                "u8",
                28
              ]
            }
          },
          {
            "name": "timestamp",
            "type": "i64"
          }
        ]
      }
    },
    {
      "name": "usdPerUnitGasUpdated",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destChain",
            "type": "u64"
          },
          {
            "name": "value",
            "type": {
              "array": [
                "u8",
                28
              ]
            }
          },
          {
            "name": "timestamp",
            "type": "i64"
          }
        ]
      }
    },
    {
      "name": "validatedTokenPriceV2",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "price",
            "type": {
              "defined": {
                "name": "usd18Decimals"
              }
            }
          },
          {
            "name": "billingEnabled",
            "type": "bool"
          }
        ]
      }
    }
  ]
};
