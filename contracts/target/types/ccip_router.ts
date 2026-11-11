/**
 * Program IDL in camelCase format in order to be used in JS/TS.
 *
 * Note that this is only a type helper and is not the actual IDL. The original
 * IDL can be found at `target/idl/ccip_router.json`.
 */
export type CcipRouter = {
  "address": "Ccip842gzYHhvdDkSyi2YVCoAWPbYJoApMFzSxQroE9C",
  "metadata": {
    "name": "ccipRouter",
    "version": "2.0.1",
    "spec": "0.1.0",
    "description": "Created with Anchor"
  },
  "docs": [
    "The `ccip_router` module contains the implementation of the Cross-Chain Interoperability Protocol (CCIP) Router.",
    "",
    "This is the Collapsed Router Program for CCIP.",
    "As it's upgradable persisting the same program id, there is no need to have an indirection of a Proxy Program.",
    "This Router handles the OnRamp flow of the CCIP Messages.",
    "",
    "NOTE to devs: This file however should contain *no logic*, only the entrypoints to the different versioned modules,",
    "thus making it easier to ensure later on that logic can be changed during upgrades without affecting the interface."
  ],
  "instructions": [
    {
      "name": "acceptAdminRoleTokenAdminRegistry",
      "docs": [
        "Accepts the admin role of the token admin registry.",
        "",
        "The Pending Admin must call this function to accept the admin role of the Token Admin Registry.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for accepting the admin role.",
        "* `mint` - The public key of the token mint."
      ],
      "discriminator": [
        106,
        240,
        16,
        173,
        137,
        213,
        163,
        246
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
          "name": "tokenAdminRegistry",
          "writable": true,
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
                  97,
                  100,
                  109,
                  105,
                  110,
                  95,
                  114,
                  101,
                  103,
                  105,
                  115,
                  116,
                  114,
                  121
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
        }
      ],
      "args": []
    },
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
        }
      ],
      "args": []
    },
    {
      "name": "addChainSelector",
      "docs": [
        "Adds a new chain selector to the router.",
        "",
        "The Admin needs to add any new chain supported (this means both OnRamp and OffRamp).",
        "When adding a new chain, the Admin needs to specify if it's enabled or not.",
        "They may enable only source, or only destination, or neither, or both.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for adding the chain selector.",
        "* `new_chain_selector` - The new chain selector to be added.",
        "* `source_chain_config` - The configuration for the chain as source.",
        "* `dest_chain_config` - The configuration for the chain as destination."
      ],
      "discriminator": [
        28,
        60,
        171,
        0,
        195,
        113,
        56,
        7
      ],
      "accounts": [
        {
          "name": "destChainState",
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
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "newChainSelector"
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
          "name": "newChainSelector",
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
      "name": "addDestChainV2",
      "docs": [
        "Adds a new destination chain to the router, for v2 lanes.",
        "",
        "Destination side only: this initializes the lane's `DestChainCcipV2` PDA, which is what",
        "`ccip_send_v2` reads. Source-chain configuration is the offramp's (`add_source_chain`), so",
        "enabling a lane in both directions means calling both programs.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for adding the destination chain.",
        "* `params` - Parameters required for this operation"
      ],
      "discriminator": [
        235,
        50,
        222,
        189,
        224,
        231,
        102,
        51
      ],
      "accounts": [
        {
          "name": "destChainState",
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
                  115,
                  116,
                  97,
                  116,
                  101,
                  95,
                  118,
                  50
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
              "name": "addDestChainV2Params"
            }
          }
        }
      ]
    },
    {
      "name": "addOfframp",
      "docs": [
        "Add an offramp address to the list of offramps allowed by the router, for a",
        "particular source chain. External users will check this list before accepting",
        "a `ccip_receive` CPI.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for this operation.",
        "* `source_chain_selector` - The source chain for the offramp's lane.",
        "* `offramp` - The offramp's address."
      ],
      "discriminator": [
        164,
        255,
        154,
        96,
        204,
        239,
        24,
        2
      ],
      "accounts": [
        {
          "name": "allowedOfframp",
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
                  111,
                  102,
                  102,
                  114,
                  97,
                  109,
                  112
                ]
              },
              {
                "kind": "arg",
                "path": "sourceChainSelector"
              },
              {
                "kind": "arg",
                "path": "offramp"
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
          "name": "sourceChainSelector",
          "type": "u64"
        },
        {
          "name": "offramp",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "bumpCcipVersionForDestChain",
      "docs": [
        "Bumps the CCIP version for a destination chain.",
        "This effectively just resets the message number of the destination chain state.",
        "If there had been a previous rollback, on re-upgrade the message number will resume from where it was",
        "prior to the rollback.",
        "",
        "# Arguments",
        "* `ctx` - The context containing the accounts required for the bump.",
        "* `dest_chain_selector` - The destination chain selector to bump version for."
      ],
      "discriminator": [
        120,
        25,
        6,
        201,
        42,
        224,
        235,
        187
      ],
      "accounts": [
        {
          "name": "destChainState",
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
                  115,
                  116,
                  97,
                  116,
                  101
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
        }
      ],
      "args": [
        {
          "name": "destChainSelector",
          "type": "u64"
        }
      ]
    },
    {
      "name": "ccipAdminOverridePendingAdministrator",
      "docs": [
        "Overrides the pending admin of the Token Admin Registry",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for registration.",
        "* `token_admin_registry_admin` - The public key of the token admin registry admin to propose."
      ],
      "discriminator": [
        163,
        206,
        164,
        199,
        248,
        92,
        36,
        46
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
          "name": "tokenAdminRegistry",
          "writable": true,
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
                  97,
                  100,
                  109,
                  105,
                  110,
                  95,
                  114,
                  101,
                  103,
                  105,
                  115,
                  116,
                  114,
                  121
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
          "name": "tokenAdminRegistryAdmin",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "ccipAdminProposeAdministrator",
      "docs": [
        "Token Admin Registry //",
        "Registers the Token Admin Registry via the CCIP Admin",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for registration.",
        "* `token_admin_registry_admin` - The public key of the token admin registry admin to propose."
      ],
      "discriminator": [
        218,
        37,
        139,
        107,
        142,
        228,
        51,
        219
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
          "name": "tokenAdminRegistry",
          "writable": true,
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
                  97,
                  100,
                  109,
                  105,
                  110,
                  95,
                  114,
                  101,
                  103,
                  105,
                  115,
                  116,
                  114,
                  121
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
          "name": "tokenAdminRegistryAdmin",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "ccipSend",
      "docs": [
        "On Ramp Flow //",
        "Sends a message to the destination chain.",
        "",
        "Request a message to be sent to the destination chain.",
        "The method name needs to be ccip_send with Anchor encoding.",
        "This function is called by the CCIP Sender Contract (or final user) to send a message to the CCIP Router.",
        "The message will be sent to the receiver on the destination chain selector.",
        "This message emits the event CCIPMessageSent with all the necessary data to be retrieved by the OffChain Code",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for sending the message.",
        "* `dest_chain_selector` - The chain selector for the destination chain.",
        "* `message` - The message to be sent. The size limit of data is 256 bytes.",
        "* `token_indexes` - Indices into the remaining accounts vector where the subslice for a token begins."
      ],
      "discriminator": [
        108,
        216,
        134,
        191,
        249,
        234,
        33,
        84
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
          "name": "destChainState",
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
                  115,
                  116,
                  97,
                  116,
                  101
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
          "name": "nonce",
          "docs": [
            "CHECK this represents the PDA where the message counters are stored. As it may be initialized or not,",
            "and it may be in it's v1 or v2 form, it is an UncheckedAccount and the code handles all cases manually."
          ],
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  110,
                  111,
                  110,
                  99,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "destChainSelector"
              },
              {
                "kind": "account",
                "path": "authority"
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
          "name": "feeTokenProgram"
        },
        {
          "name": "feeTokenMint"
        },
        {
          "name": "feeTokenUserAssociatedAccount",
          "docs": [
            "If paying with native SOL, this must be the zero address."
          ]
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
          "name": "feeBillingSigner",
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
                  115,
                  105,
                  103,
                  110,
                  101,
                  114
                ]
              }
            ]
          }
        },
        {
          "name": "feeQuoter"
        },
        {
          "name": "feeQuoterConfig"
        },
        {
          "name": "feeQuoterDestChain"
        },
        {
          "name": "feeQuoterBillingTokenConfig"
        },
        {
          "name": "feeQuoterLinkTokenConfig"
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
          "name": "tokenIndexes",
          "type": "bytes"
        }
      ],
      "returns": {
        "array": [
          "u8",
          32
        ]
      }
    },
    {
      "name": "ccipSendV2",
      "docs": [
        "Sends a message to the destination chain through the CCIP v2.0 Onramp",
        "",
        "Request a message to be sent to the destination chain.",
        "The method name needs to be ccip_send_v2 with Anchor encoding.",
        "This function is called by the CCIP Sender Contract (or final user) to send a message to the CCIP Router.",
        "The message will be sent to the receiver on the destination chain selector.",
        "This message emits the event CCIPMessageSentV2 with all the necessary data to be retrieved by the OffChain Code",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for sending the message.",
        "* `params` - Parameters required for sending the message, including:",
        "* `dest_chain_selector` - The chain selector for the destination chain.",
        "* `message` - The message to be sent.",
        "* `resolution_metadata` - An opaque value obtained from calling `resolve_accounts_start`, which must be done",
        "before invoking this instruction."
      ],
      "discriminator": [
        175,
        63,
        145,
        145,
        211,
        111,
        21,
        183
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
          "name": "destChainState",
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
                  115,
                  116,
                  97,
                  116,
                  101,
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
          "name": "tokenAdminRegistry",
          "docs": [
            "token. When there isn't, it's the zero pubkey. This is stored separately to the token accounts as it's",
            "necessary to determine the schema for the token account layout."
          ]
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
          "name": "feeQuoter"
        },
        {
          "name": "feeQuoterConfig"
        },
        {
          "name": "feeQuoterDestChain"
        },
        {
          "name": "feeQuoterDestChainLegacy"
        },
        {
          "name": "feeQuoterBillingTokenConfig"
        },
        {
          "name": "feeQuoterLinkTokenConfig"
        },
        {
          "name": "feeTokenProgram"
        },
        {
          "name": "feeTokenMint"
        },
        {
          "name": "feeTokenUserAssociatedAccount",
          "docs": [
            "If paying with native SOL, this must be the zero address."
          ]
        },
        {
          "name": "feeVault",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "account",
                "path": "feeBillingSigner"
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
          "name": "feeBillingSigner",
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
                  115,
                  105,
                  103,
                  110,
                  101,
                  114
                ]
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
              "name": "ccipSendV2Params"
            }
          }
        }
      ],
      "returns": {
        "array": [
          "u8",
          32
        ]
      }
    },
    {
      "name": "deriveAccountsCcipSend",
      "docs": [
        "Automatically derives all accounts required to call `ccip_send`.",
        "",
        "This method receives the bare minimum amount of information needed to construct",
        "the entire account list to send a transaction, and builds it iteratively",
        "over the course of multiple calls.",
        "",
        "The return type contains:",
        "",
        "* `accounts_to_save`: The caller must append these accounts to a list they maintain.",
        "When complete, this list will contain all accounts needed to call `ccip_send`.",
        "* `ask_again_with`: When `next_stage` is not empty, the caller must call `derive_accounts_ccip_send`",
        "again, including exactly these accounts as the `remaining_accounts`.",
        "* `lookup_tables_to_save`: The caller must save those LUTs. They can be used for `ccip_send`.",
        "* `current_stage`: A string describing the current stage of the derivation process. When the stage",
        "is `TokenTransferStaticAccounts/<N>/0`, it means the `accounts_to_save` block in this response contains",
        "all accounts relating to the Nth token being transferred. Use this information to construct",
        "the `token_indexes` vector that `ccip_send` requires.",
        "* `next_stage`: If nonempty, this means the instruction must get called again with this value",
        "as the `stage` argument.",
        "",
        "Therefore, and starting with an empty `remaining_accounts` list, the caller must repeatedly",
        "call `derive_accounts_ccip_send` until `next_stage` is returned empty.",
        "",
        "# Arguments",
        "",
        "* `ctx`: Context containing only the config.",
        "* `stage`: Requested derivation stage. Pass \"Start\" the first time, then for each subsequent",
        "call, pass the value returned in `response.next_stage` until empty.",
        "* `params`:",
        "* `ccip_send_caller`: Public key of the account that will sign the call to `ccip_send`.",
        "* `dest_chain_selector`: CCIP chain selector for the dest chain.",
        "* `fee_token_mint`: The mint address for the token used for fees. Pubkey::default() if native SOL.",
        "* `mints_of_transferred_token`: List of all token mints for tokens being transferred."
      ],
      "discriminator": [
        251,
        176,
        50,
        26,
        193,
        231,
        156,
        242
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
              "name": "deriveAccountsCcipSendParams"
            }
          }
        },
        {
          "name": "stage",
          "type": "string"
        }
      ],
      "returns": {
        "defined": {
          "name": "deriveAccountsResponse"
        }
      }
    },
    {
      "name": "getFee",
      "docs": [
        "Queries the onramp for the fee required to send a message.",
        "",
        "This call is permissionless. Note it does not verify whether there's a curse active",
        "in order to avoid the RMN CPI overhead.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for obtaining the message fee.",
        "* `dest_chain_selector` - The chain selector for the destination chain.",
        "* `message` - The message to be sent. The size limit of data is 256 bytes."
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
          "name": "destChainState",
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
                  115,
                  116,
                  97,
                  116,
                  101
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
          "name": "feeQuoter"
        },
        {
          "name": "feeQuoterConfig"
        },
        {
          "name": "feeQuoterDestChain"
        },
        {
          "name": "feeQuoterBillingTokenConfig"
        },
        {
          "name": "feeQuoterLinkTokenConfig"
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
      "name": "getFeeV2",
      "docs": [
        "CCIPv2 message fee retrieval"
      ],
      "discriminator": [
        178,
        49,
        15,
        7,
        35,
        31,
        31,
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
          "name": "destChainState",
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
                  115,
                  116,
                  97,
                  116,
                  101,
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
          "name": "tokenAdminRegistry",
          "docs": [
            "token. When there isn't, it's the zero pubkey. This is stored separately to the token accounts as it's",
            "necessary to determine the schema for the token account layout. The handler validates it against the",
            "registry PDA derived from the transferred token before using it."
          ]
        },
        {
          "name": "feeQuoter",
          "docs": [
            ""
          ]
        },
        {
          "name": "feeQuoterConfig"
        },
        {
          "name": "feeQuoterDestChain"
        },
        {
          "name": "feeQuoterDestChainLegacy"
        },
        {
          "name": "feeQuoterBillingTokenConfig"
        },
        {
          "name": "feeQuoterLinkTokenConfig"
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
              "name": "getFeeParams"
            }
          }
        }
      ],
      "returns": {
        "defined": {
          "name": "getFeeResultV2"
        }
      }
    },
    {
      "name": "initialize",
      "docs": [
        "Initialization Flow //",
        "Initializes the CCIP Router.",
        "",
        "The initialization of the Router is responsibility of Admin, nothing more than calling this method should be done first.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for initialization.",
        "* `svm_chain_selector` - The chain selector for SVM.",
        "* `fee_aggregator` - The public key of the fee aggregator.",
        "* `fee_quoter` - The public key of the fee quoter.",
        "* `link_token_mint` - The public key of the LINK token mint.",
        "* `rmn_remote` - The public key of the RMN remote."
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
          "name": "program",
          "address": "Ccip842gzYHhvdDkSyi2YVCoAWPbYJoApMFzSxQroE9C"
        },
        {
          "name": "programData"
        }
      ],
      "args": [
        {
          "name": "svmChainSelector",
          "type": "u64"
        },
        {
          "name": "feeAggregator",
          "type": "pubkey"
        },
        {
          "name": "feeQuoter",
          "type": "pubkey"
        },
        {
          "name": "linkTokenMint",
          "type": "pubkey"
        },
        {
          "name": "rmnRemote",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "multisigOwnerOverridePendingAdministrator",
      "docs": [
        "Overrides the pending admin of the Token Admin Registry by a signer of the mint authority multisig",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for registration.",
        "* `token_admin_registry_admin` - The public key of the token admin registry admin to propose."
      ],
      "discriminator": [
        208,
        168,
        51,
        3,
        54,
        166,
        253,
        185
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
          "name": "tokenAdminRegistry",
          "writable": true,
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
                  97,
                  100,
                  109,
                  105,
                  110,
                  95,
                  114,
                  101,
                  103,
                  105,
                  115,
                  116,
                  114,
                  121
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
          "name": "mintAuthorityMultisig",
          "docs": [
            "Only 1-of-N multisig mint authorities are supported in this flow."
          ]
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "tokenAdminRegistryAdmin",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "multisigOwnerProposeAdministrator",
      "docs": [
        "Registers the Token Admin Registry by a signer of the mint authority multisig.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for registration.",
        "* `token_admin_registry_admin` - The public key of the token admin registry admin to propose."
      ],
      "discriminator": [
        121,
        66,
        149,
        244,
        103,
        98,
        205,
        49
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
          "name": "tokenAdminRegistry",
          "writable": true,
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
                  97,
                  100,
                  109,
                  105,
                  110,
                  95,
                  114,
                  101,
                  103,
                  105,
                  115,
                  116,
                  114,
                  121
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
          "name": "mintAuthorityMultisig",
          "docs": [
            "Only 1-of-N multisigs (threshold `m == 1`) are supported in this flow."
          ]
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "tokenAdminRegistryAdmin",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "observeConfig",
      "docs": [
        "Returns the router's global configuration, for observability consumers.",
        "",
        "Permissionless and read-only: meant to be executed through `simulateTransaction`.",
        "",
        "# Arguments",
        "* `ctx` - The context containing the router config account"
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
          "name": "routerConfigObservation"
        }
      }
    },
    {
      "name": "observeDestChainV2",
      "docs": [
        "Returns the CCIP 2.0 onramp state and configuration for a single destination chain, for",
        "observability consumers.",
        "",
        "Permissionless and read-only: meant to be executed through `simulateTransaction`. Fails if",
        "the lane has no `dest_chain_state_v2` account, which callers can read as \"not configured\".",
        "",
        "# Arguments",
        "* `ctx` - The context containing the dest chain state account",
        "* `dest_chain_selector` - The destination chain selector the lane belongs to"
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
          "name": "destChainState",
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
                  115,
                  116,
                  97,
                  116,
                  101,
                  95,
                  118,
                  50
                ]
              },
              {
                "kind": "arg",
                "path": "destChainSelector"
              }
            ]
          }
        }
      ],
      "args": [
        {
          "name": "destChainSelector",
          "type": "u64"
        }
      ],
      "returns": {
        "defined": {
          "name": "routerDestChainV2Observation"
        }
      }
    },
    {
      "name": "ownerOverridePendingAdministrator",
      "docs": [
        "Overrides the pending admin of the Token Admin Registry by the token owner",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for registration.",
        "* `token_admin_registry_admin` - The public key of the token admin registry admin to propose."
      ],
      "discriminator": [
        230,
        111,
        134,
        149,
        203,
        168,
        118,
        201
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
          "name": "tokenAdminRegistry",
          "writable": true,
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
                  97,
                  100,
                  109,
                  105,
                  110,
                  95,
                  114,
                  101,
                  103,
                  105,
                  115,
                  116,
                  114,
                  121
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
          "name": "tokenAdminRegistryAdmin",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "ownerProposeAdministrator",
      "docs": [
        "Registers the Token Admin Registry by the token owner.",
        "",
        "The Authority of the Mint Token can claim the registry of the token.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for registration.",
        "* `token_admin_registry_admin` - The public key of the token admin registry admin to propose."
      ],
      "discriminator": [
        175,
        81,
        160,
        246,
        206,
        132,
        18,
        22
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
          "name": "tokenAdminRegistry",
          "writable": true,
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
                  97,
                  100,
                  109,
                  105,
                  110,
                  95,
                  114,
                  101,
                  103,
                  105,
                  115,
                  116,
                  114,
                  121
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
          "name": "tokenAdminRegistryAdmin",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "registerReceiver",
      "docs": [
        "Registers (or updates) the CCIP interface version of a receiver program. The",
        "OffRamp consults the resulting `ReceiverRegistry` PDA to select the v1 vs v2 receiver flow;",
        "an unregistered receiver defaults to v1, so only v2 receivers need to register.",
        "",
        "Authorized either by a PDA of `receiver_program` (self-registration via CPI) or by the",
        "router's upgrade authority — never by the receiver's own admin as a direct caller. See",
        "`register_receiver::register_receiver` for the authorization check itself.",
        "",
        "# Arguments",
        "* `ctx` - The context containing the accounts required for registration.",
        "* `receiver_program` - The receiver program whose interface version is being registered.",
        "* `receiver_version` - The receiver's CCIP interface version; must be `1` or `2`."
      ],
      "discriminator": [
        106,
        6,
        232,
        177,
        224,
        105,
        25,
        128
      ],
      "accounts": [
        {
          "name": "receiverRegistry",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  114,
                  101,
                  99,
                  101,
                  105,
                  118,
                  101,
                  114,
                  95,
                  114,
                  101,
                  103,
                  105,
                  115,
                  116,
                  114,
                  121
                ]
              },
              {
                "kind": "arg",
                "path": "registerReceiverParams.receiverProgram"
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
          "name": "receiverRegistrationSigner"
        },
        {
          "name": "authority",
          "docs": [
            "The authorizing signer: either the receiver program's `RECEIVER_REGISTRATION_SIGNER` PDA",
            "(self-registration via CPI) or the router's owner (override)."
          ],
          "signer": true
        },
        {
          "name": "payer",
          "docs": [
            "Rent payer for the registry PDA on first registration. Kept separate from `authority` so the",
            "receiver-PDA path (whose signer is a non-fee-paying PDA) can still fund the account via the",
            "forwarded transaction payer."
          ],
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
              "name": "registerReceiverParams"
            }
          }
        }
      ]
    },
    {
      "name": "removeOfframp",
      "docs": [
        "Remove an offramp address from the list of offramps allowed by the router, for a",
        "particular source chain. External users will check this list before accepting",
        "a `ccip_receive` CPI.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for this operation.",
        "* `source_chain_selector` - The source chain for the offramp's lane.",
        "* `offramp` - The offramp's address."
      ],
      "discriminator": [
        252,
        152,
        51,
        170,
        241,
        13,
        199,
        8
      ],
      "accounts": [
        {
          "name": "allowedOfframp",
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
                  111,
                  102,
                  102,
                  114,
                  97,
                  109,
                  112
                ]
              },
              {
                "kind": "arg",
                "path": "sourceChainSelector"
              },
              {
                "kind": "arg",
                "path": "offramp"
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
          "name": "sourceChainSelector",
          "type": "u64"
        },
        {
          "name": "offramp",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "resolveAccountsCcvStage",
      "docs": [
        "Continues nested account resolution for CCV source hooks used by `ccip_send_v2`."
      ],
      "discriminator": [
        154,
        182,
        30,
        223,
        76,
        68,
        152,
        32
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
          "docs": [
            "Seeds are validated in the handler below (requires parsing params first)."
          ]
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
      "name": "resolveAccountsConstructFinalCcvListStage",
      "docs": [
        "Continues nested account resolution for `ccip_send_v2`."
      ],
      "discriminator": [
        119,
        172,
        96,
        69,
        95,
        84,
        227,
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
          "name": "destChain",
          "docs": [
            "Seeds are validated in the handler below (requires parsing params first)."
          ]
        },
        {
          "name": "feeTokenMint"
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
      "name": "resolveAccountsExecutorStage",
      "docs": [
        "Continues nested account resolution for the executor fee query used by `ccip_send_v2`."
      ],
      "discriminator": [
        216,
        170,
        191,
        171,
        60,
        52,
        132,
        208
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
          "docs": [
            "Seeds are validated in the handler below (requires parsing params first)."
          ]
        },
        {
          "name": "feeTokenMint"
        },
        {
          "name": "referenceAddresses",
          "docs": [
            "The router's `ReferenceAddresses`, which do not exist until `update_reference_addresses`",
            "creates them. Their address is validated in the handler below."
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
    },
    {
      "name": "resolveAccountsFeeTokenBillingStage",
      "docs": [
        "Continues nested account resolution for the fee token billing components used by `ccip_send_v2`."
      ],
      "discriminator": [
        87,
        98,
        7,
        159,
        213,
        131,
        155,
        118
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
          "name": "feeTokenMint"
        },
        {
          "name": "billingSigner"
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
      "name": "resolveAccountsNestedTokenStage",
      "docs": [
        "Continues nested account resolution for token pool hooks used by `ccip_send_v2`."
      ],
      "discriminator": [
        255,
        82,
        75,
        120,
        35,
        244,
        206,
        226
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
        },
        {
          "name": "destChain"
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
        "against changes in account structure."
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
      "name": "resolveAccountsTokenTransferRetrieveLutStage",
      "docs": [
        "Continues nested account resolution for token transfer used `ccip_send_v2`."
      ],
      "discriminator": [
        36,
        59,
        3,
        169,
        253,
        158,
        48,
        25
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
      "name": "resolveAccountsTokenTransferStaticAccountsStage",
      "docs": [
        "Continues nested account resolution for token transfer static accounts used by `ccip_send_v2`.",
        "",
        "A lookup table too large for one 1kB response is emitted a page at a time: the stage then",
        "points back at itself, so callers must keep following `next_ix_discriminator` rather than",
        "assuming a single round."
      ],
      "discriminator": [
        234,
        71,
        234,
        194,
        73,
        83,
        152,
        159
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
        },
        {
          "name": "tokenLut"
        },
        {
          "name": "feeTokenMint"
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
      "name": "rollbackCcipVersionForDestChain",
      "docs": [
        "Rolls back the CCIP version for a destination chain.",
        "This effectively just restores the old version's message number of the destination chain state.",
        "We only support 1 consecutive rollback. If a rollback has occurred for that lane, the version can't",
        "be rolled back again without bumping the version first.",
        "",
        "# Arguments",
        "* `ctx` - The context containing the accounts required for the rollback.",
        "* `dest_chain_selector` - The destination chain selector to rollback the version for."
      ],
      "discriminator": [
        95,
        107,
        33,
        138,
        26,
        57,
        154,
        110
      ],
      "accounts": [
        {
          "name": "destChainState",
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
                  115,
                  116,
                  97,
                  116,
                  101
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
        }
      ],
      "args": [
        {
          "name": "destChainSelector",
          "type": "u64"
        }
      ]
    },
    {
      "name": "setDefaultCodeVersion",
      "docs": [
        "Config //",
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
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
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
        "Sets the address of the LINK token mint.",
        "The Admin is the only one able to set it.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for updating the configuration.",
        "* `link_token_mint` - The new address of the LINK token mint."
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
          "name": "authority",
          "signer": true
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "linkTokenMint",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "setPool",
      "docs": [
        "Sets the pool lookup table for a given token mint.",
        "",
        "The administrator of the token admin registry can set the pool lookup table for a given token mint.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for setting the pool.",
        "* `writable_indexes` - a bit map of the indexes of the accounts in lookup table that are writable"
      ],
      "discriminator": [
        119,
        30,
        14,
        180,
        115,
        225,
        167,
        238
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
          "name": "tokenAdminRegistry",
          "writable": true,
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
                  97,
                  100,
                  109,
                  105,
                  110,
                  95,
                  114,
                  101,
                  103,
                  105,
                  115,
                  116,
                  114,
                  121
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
          "name": "poolLookuptable"
        },
        {
          "name": "authority",
          "writable": true,
          "signer": true
        }
      ],
      "args": [
        {
          "name": "writableIndexes",
          "type": "bytes"
        }
      ]
    },
    {
      "name": "setPoolInterfaceImplementation",
      "docs": [
        "Sets the pool interface implementation for a given token pool in the Token Admin Registry.",
        "Used to configure non-default values on new fields of the Token Admin Registry that refer to",
        "the pool's implemented interface, particularly which version of the pool interface is implemented",
        "and whether it implements account resolution or not."
      ],
      "discriminator": [
        121,
        95,
        65,
        76,
        190,
        0,
        152,
        64
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
          "name": "tokenAdminRegistry",
          "writable": true,
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
                  97,
                  100,
                  109,
                  105,
                  110,
                  95,
                  114,
                  101,
                  103,
                  105,
                  115,
                  116,
                  114,
                  121
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
              "name": "setPoolInterfaceImplementationParams"
            }
          }
        }
      ]
    },
    {
      "name": "transferAdminRoleTokenAdminRegistry",
      "docs": [
        "Transfers the admin role of the token admin registry to a new admin.",
        "",
        "Only the Admin can transfer the Admin Role of the Token Admin Registry, this setups the Pending Admin and then it's their responsibility to accept the role.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for the transfer.",
        "* `mint` - The public key of the token mint.",
        "* `new_admin` - The public key of the new admin."
      ],
      "discriminator": [
        178,
        98,
        203,
        181,
        203,
        107,
        106,
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
          "name": "tokenAdminRegistry",
          "writable": true,
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
                  97,
                  100,
                  109,
                  105,
                  110,
                  95,
                  114,
                  101,
                  103,
                  105,
                  115,
                  116,
                  114,
                  121
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
        }
      ],
      "args": [
        {
          "name": "newAdmin",
          "type": "pubkey"
        }
      ]
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
      "name": "updateDestChainConfig",
      "docs": [
        "Updates the configuration of the destination chain selector.",
        "",
        "The Admin is the only one able to update the destination chain config.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for updating the chain selector.",
        "* `dest_chain_selector` - The destination chain selector to be updated.",
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
          "name": "destChainState",
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
                  115,
                  116,
                  97,
                  116,
                  101
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
    },
    {
      "name": "updateDestChainConfigV2",
      "docs": [
        "Updates the configuration of the destination chain selector, for v2",
        "lanes.",
        "",
        "The Admin is the only one able to update the destination chain config.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for updating the chain selector.",
        "* `params` - Parameters required for this operation"
      ],
      "discriminator": [
        81,
        35,
        70,
        253,
        169,
        224,
        6,
        104
      ],
      "accounts": [
        {
          "name": "destChainState",
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
                  115,
                  116,
                  97,
                  116,
                  101,
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
              "name": "updateChainSelectorV2Params"
            }
          }
        }
      ]
    },
    {
      "name": "updateFeeAggregator",
      "docs": [
        "Updates the fee aggregator in the router configuration.",
        "The Admin is the only one able to update the fee aggregator.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for updating the configuration.",
        "* `fee_aggregator` - The new fee aggregator address (ATAs will be derived for it for each token)."
      ],
      "discriminator": [
        85,
        112,
        115,
        60,
        22,
        95,
        230,
        56
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
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
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
      "name": "updateReferenceAddresses",
      "docs": [
        "Updates the router's reference addresses, creating their PDA on first use.",
        "",
        "The Admin is the only one able to update them.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for this operation.",
        "* `onramp_lookup_table` - The address lookup table that account resolution returns for",
        "`ccip_send_v2` and `get_fee_v2`, or `Pubkey::default()` to stop returning one. It is only",
        "read during resolution, so it adds no accounts to those instructions."
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
          "name": "onrampLookupTable",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "updateRmnRemote",
      "docs": [
        "Updates the RMN remote program in the router configuration.",
        "The Admin is the only one able to update the RMN remote program.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for updating the configuration.",
        "* `rmn_remote,` - The new RMN remote address."
      ],
      "discriminator": [
        66,
        12,
        215,
        147,
        14,
        176,
        55,
        214
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
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "rmnRemote",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "updateSvmChainSelector",
      "docs": [
        "Updates the SVM chain selector in the router configuration.",
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
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        }
      ],
      "args": [
        {
          "name": "newChainSelector",
          "type": "u64"
        }
      ]
    },
    {
      "name": "upgradeTokenAdminRegistry",
      "docs": [
        "Upgrades the Token Admin Registry from version 1 or 2 to the current version.",
        "",
        "Anyone may invoke this method, as the upgrade has safe defaults for any new value,",
        "and those can then be changed by the Token Admin Registry Admin via separate instructions.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for the upgrade."
      ],
      "discriminator": [
        124,
        26,
        79,
        44,
        190,
        150,
        213,
        49
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
          "name": "tokenAdminRegistry",
          "docs": [
            "types Anchor would attempt to deserialize the data _before_ realloc'ing it, which would fail.",
            "The code will load it and realloc it to the new size manually, and migrate its data."
          ],
          "writable": true,
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
                  97,
                  100,
                  109,
                  105,
                  110,
                  95,
                  114,
                  101,
                  103,
                  105,
                  115,
                  116,
                  114,
                  121
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
      "args": []
    },
    {
      "name": "withdrawBilledFunds",
      "docs": [
        "Billing //",
        "Transfers the accumulated billed fees in a particular token to an arbitrary token account.",
        "Only the CCIP Admin can withdraw billed funds.",
        "",
        "# Arguments",
        "",
        "* `ctx` - The context containing the accounts required for the transfer of billed fees.",
        "* `transfer_all` - A flag indicating whether to transfer all the accumulated fees in that token or not.",
        "* `desired_amount` - The amount to transfer. If `transfer_all` is true, this value must be 0."
      ],
      "discriminator": [
        16,
        116,
        73,
        38,
        77,
        232,
        6,
        28
      ],
      "accounts": [
        {
          "name": "feeTokenMint"
        },
        {
          "name": "feeTokenAccum",
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
          "name": "recipient",
          "writable": true
        },
        {
          "name": "tokenProgram"
        },
        {
          "name": "feeBillingSigner",
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
                  115,
                  105,
                  103,
                  110,
                  101,
                  114
                ]
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
        }
      ],
      "args": [
        {
          "name": "transferAll",
          "type": "bool"
        },
        {
          "name": "desiredAmount",
          "type": "u64"
        }
      ]
    }
  ],
  "accounts": [
    {
      "name": "allowedOfframp",
      "discriminator": [
        247,
        97,
        179,
        16,
        207,
        36,
        236,
        132
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
      "name": "destChainCcipV2",
      "discriminator": [
        8,
        81,
        151,
        73,
        188,
        159,
        25,
        160
      ]
    },
    {
      "name": "receiverRegistry",
      "discriminator": [
        143,
        191,
        40,
        192,
        11,
        255,
        173,
        235
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
      "name": "tokenAdminRegistry",
      "discriminator": [
        70,
        92,
        207,
        200,
        76,
        17,
        57,
        114
      ]
    }
  ],
  "events": [
    {
      "name": "administratorTransferRequested",
      "discriminator": [
        159,
        30,
        110,
        86,
        22,
        35,
        70,
        125
      ]
    },
    {
      "name": "administratorTransferred",
      "discriminator": [
        103,
        127,
        255,
        114,
        168,
        163,
        159,
        124
      ]
    },
    {
      "name": "ccipMessageSent",
      "discriminator": [
        23,
        77,
        73,
        183,
        123,
        185,
        115,
        57
      ]
    },
    {
      "name": "ccipMessageSentV2",
      "discriminator": [
        112,
        32,
        93,
        248,
        29,
        92,
        99,
        188
      ]
    },
    {
      "name": "ccipVersionForDestChainVersionBumped",
      "discriminator": [
        81,
        97,
        90,
        70,
        154,
        163,
        255,
        78
      ]
    },
    {
      "name": "ccipVersionForDestChainVersionRolledBack",
      "discriminator": [
        50,
        79,
        44,
        175,
        232,
        241,
        225,
        171
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
      "name": "offrampAdded",
      "discriminator": [
        158,
        77,
        52,
        73,
        113,
        247,
        76,
        150
      ]
    },
    {
      "name": "offrampRemoved",
      "discriminator": [
        231,
        81,
        202,
        9,
        89,
        193,
        154,
        37
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
      "name": "pdaUpgraded",
      "discriminator": [
        204,
        155,
        59,
        105,
        171,
        130,
        192,
        169
      ]
    },
    {
      "name": "poolInterfaceImplementationUpdated",
      "discriminator": [
        255,
        162,
        27,
        247,
        206,
        154,
        107,
        184
      ]
    },
    {
      "name": "poolSet",
      "discriminator": [
        135,
        203,
        185,
        106,
        113,
        87,
        177,
        32
      ]
    },
    {
      "name": "receiverRegistered",
      "discriminator": [
        52,
        108,
        171,
        143,
        77,
        145,
        204,
        64
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
    }
  ],
  "errors": [
    {
      "code": 7000,
      "name": "unauthorized",
      "msg": "The signer is unauthorized"
    },
    {
      "code": 7001,
      "name": "invalidRmnRemoteAddress",
      "msg": "Invalid RMN Remote Address"
    },
    {
      "code": 7002,
      "name": "invalidInputsMint",
      "msg": "Mint account input is invalid"
    },
    {
      "code": 7003,
      "name": "invalidVersion",
      "msg": "Invalid version of the onchain state"
    },
    {
      "code": 7004,
      "name": "feeTokenMismatch",
      "msg": "Fee token doesn't match transfer token"
    },
    {
      "code": 7005,
      "name": "redundantOwnerProposal",
      "msg": "Proposed owner is the current owner"
    },
    {
      "code": 7006,
      "name": "reachedMaxMessageNumber",
      "msg": "Reached max message number"
    },
    {
      "code": 7007,
      "name": "invalidInputsTokenIndices",
      "msg": "Invalid pool account account indices"
    },
    {
      "code": 7008,
      "name": "invalidInputsPoolAccounts",
      "msg": "Invalid pool accounts"
    },
    {
      "code": 7009,
      "name": "invalidInputsTokenAccounts",
      "msg": "Invalid token accounts"
    },
    {
      "code": 7010,
      "name": "invalidInputsTokenAdminRegistryAccounts",
      "msg": "Invalid Token Admin Registry account"
    },
    {
      "code": 7011,
      "name": "invalidInputsLookupTableAccounts",
      "msg": "Invalid LookupTable account"
    },
    {
      "code": 7012,
      "name": "invalidInputsLookupTableAccountWritable",
      "msg": "Invalid LookupTable account writable access"
    },
    {
      "code": 7013,
      "name": "invalidInputsTokenAmount",
      "msg": "Cannot send zero tokens"
    },
    {
      "code": 7014,
      "name": "invalidInputsTransferAllAmount",
      "msg": "Must specify zero amount to send alongside transfer_all"
    },
    {
      "code": 7015,
      "name": "invalidInputsAtaAddress",
      "msg": "Invalid Associated Token Account address"
    },
    {
      "code": 7016,
      "name": "invalidInputsAtaWritable",
      "msg": "Invalid Associated Token Account writable flag"
    },
    {
      "code": 7017,
      "name": "invalidInputsChainSelector",
      "msg": "Chain selector is invalid"
    },
    {
      "code": 7018,
      "name": "insufficientLamports",
      "msg": "Insufficient lamports"
    },
    {
      "code": 7019,
      "name": "insufficientFunds",
      "msg": "Insufficient funds"
    },
    {
      "code": 7020,
      "name": "sourceTokenDataTooLarge",
      "msg": "Source token data is too large"
    },
    {
      "code": 7021,
      "name": "invalidTokenAdminRegistryInputsZeroAddress",
      "msg": "New Admin can not be zero address"
    },
    {
      "code": 7022,
      "name": "invalidTokenAdminRegistryProposedAdmin",
      "msg": "An already owned registry can not be proposed"
    },
    {
      "code": 7023,
      "name": "senderNotAllowed",
      "msg": "Sender not allowed for that destination chain"
    },
    {
      "code": 7024,
      "name": "invalidCodeVersion",
      "msg": "Invalid code version"
    },
    {
      "code": 7025,
      "name": "invalidCcipVersionRollback",
      "msg": "Invalid rollback attempt on the CCIP version of the onramp to the destination chain"
    },
    {
      "code": 7026,
      "name": "invalidAccountListForPdaDerivation",
      "msg": "Invalid account list for PDA derivation"
    },
    {
      "code": 7027,
      "name": "invalidDerivationStage",
      "msg": "Unexpected account derivation stage"
    },
    {
      "code": 7028,
      "name": "invalidNonceVersion",
      "msg": "Invalid version of the Nonce account"
    },
    {
      "code": 7029,
      "name": "invalidTokenPoolAccountDerivationResponse",
      "msg": "Token pool returned an unexpected derivation response"
    },
    {
      "code": 7030,
      "name": "accountDerivationResponseTooLarge",
      "msg": "Can't fit account derivation response."
    },
    {
      "code": 7031,
      "name": "defaultOwnerProposal",
      "msg": "Proposed owner is the default pubkey"
    },
    {
      "code": 7032,
      "name": "invalidAccountListLengths",
      "msg": "Invalid account list lengths"
    },
    {
      "code": 7033,
      "name": "invalidInstructionData",
      "msg": "Invalid instruction data"
    },
    {
      "code": 7034,
      "name": "invalidFeeQuoterAddress",
      "msg": "Invalid FeeQuoter address"
    },
    {
      "code": 7035,
      "name": "invalidCcvReturnData",
      "msg": "Invalid CCV return data"
    },
    {
      "code": 7036,
      "name": "invalidCcvAccountResolutionResponse",
      "msg": "Invalid CCV account resolution response"
    },
    {
      "code": 7037,
      "name": "missingCpiReturnData",
      "msg": "CPI target did not return data"
    },
    {
      "code": 7038,
      "name": "invalidCpiReturnProgram",
      "msg": "CPI return data came from an unexpected program"
    },
    {
      "code": 7039,
      "name": "invalidInputsMultipleTokenTransfers",
      "msg": "Invalid amount of token transfers"
    },
    {
      "code": 7040,
      "name": "invalidInputPubkeyBytes",
      "msg": "Invalid address bytes in input"
    },
    {
      "code": 7041,
      "name": "unsupportedPoolV1ResolutionOnLaneV2",
      "msg": "V1 pools with account resolution (CCTP/Lombard) are not supported on the v2 lane"
    },
    {
      "code": 7042,
      "name": "accountResolutionRequiresV2",
      "msg": "Account resolution requires pool interface version >= 2"
    },
    {
      "code": 7043,
      "name": "duplicateCcvInExtraArgs",
      "msg": "Duplicate CCV in user-provided extra args"
    },
    {
      "code": 7044,
      "name": "invalidInputsCcvAccounts",
      "msg": "Invalid CCV accounts"
    },
    {
      "code": 7045,
      "name": "invalidReceiptData",
      "msg": "Invalid receipt data"
    },
    {
      "code": 7046,
      "name": "invalidFinality",
      "msg": "Invalid finality value"
    },
    {
      "code": 7047,
      "name": "invalidAccountForMigration",
      "msg": "Account to migrate contains invalid data"
    },
    {
      "code": 7048,
      "name": "mustSpecifyDefaultOrMandatedCcvs",
      "msg": "Must specify at least one default or lane-mandated CCV"
    },
    {
      "code": 7049,
      "name": "ccvZeroAddressNotAllowed",
      "msg": "CCV address cannot be the zero/default pubkey"
    },
    {
      "code": 7050,
      "name": "duplicateCcvNotAllowed",
      "msg": "Duplicate CCV not allowed in default/lane-mandated lists"
    },
    {
      "code": 7051,
      "name": "invalidTokenPrice",
      "msg": "Invalid token price"
    },
    {
      "code": 7052,
      "name": "missingOfframpAddress",
      "msg": "Destination chain config contains an empty offramp address"
    },
    {
      "code": 7053,
      "name": "invalidInputsAddressBytesLength",
      "msg": "Invalid destination address length"
    },
    {
      "code": 7054,
      "name": "invalidReceiverAddress",
      "msg": "Invalid destination receiver address"
    },
    {
      "code": 7055,
      "name": "invalidDestTokenAddress",
      "msg": "Invalid destination token address"
    },
    {
      "code": 7056,
      "name": "invalidDestTokenReceiverAddress",
      "msg": "Invalid destination token receiver address"
    },
    {
      "code": 7057,
      "name": "noExecMode",
      "msg": "Message is configured to opt out of automated execution"
    },
    {
      "code": 7058,
      "name": "missingAtaForReceiptIssuer",
      "msg": "Missing associated token account for receipt issuer"
    },
    {
      "code": 7059,
      "name": "unsupportedFeeToken",
      "msg": "Fee token is unsupported"
    },
    {
      "code": 7060,
      "name": "invalidTokenArithmetic",
      "msg": "Token arithmetic resulted in an invalid value"
    },
    {
      "code": 7061,
      "name": "feeExceedsMaxAllowed",
      "msg": "Fee for this message exceeds the maximum allowed"
    },
    {
      "code": 7062,
      "name": "duplicateReceiptIssuer",
      "msg": "The same issuer appears in more than one receipt"
    },
    {
      "code": 7063,
      "name": "finalityNotSupportedOnPoolV1",
      "msg": "Faster-than-finality is not supported on a v1 token pool"
    },
    {
      "code": 7064,
      "name": "tokenArgsNotSupportedOnPoolV1",
      "msg": "Token args are not supported on a v1 token pool"
    },
    {
      "code": 7065,
      "name": "invalidExecutor",
      "msg": "Default executor is invalid"
    },
    {
      "code": 7066,
      "name": "tokenReceiverNotAllowed",
      "msg": "Token Receiver not allowed for current destination chain"
    },
    {
      "code": 7067,
      "name": "tokenOwnedByMultisig",
      "msg": "Token uses a multisig as mint authority - use the multisig-specific method instead"
    }
  ],
  "types": [
    {
      "name": "addDestChainV2Params",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "newChainSelector",
            "type": "u64"
          },
          {
            "name": "destChainConfig",
            "type": {
              "defined": {
                "name": "destChainConfigCcipV2"
              }
            }
          }
        ]
      }
    },
    {
      "name": "administratorTransferRequested",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "token",
            "type": "pubkey"
          },
          {
            "name": "currentAdmin",
            "type": "pubkey"
          },
          {
            "name": "newAdmin",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "administratorTransferred",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "token",
            "type": "pubkey"
          },
          {
            "name": "newAdmin",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "allowedOfframp",
      "type": {
        "kind": "struct",
        "fields": []
      }
    },
    {
      "name": "ccipMessageSent",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "sequenceNumber",
            "type": "u64"
          },
          {
            "name": "message",
            "type": {
              "defined": {
                "name": "svm2AnyRampMessage"
              }
            }
          }
        ]
      }
    },
    {
      "name": "ccipMessageSentV2",
      "docs": [
        "CCIP 2.0 CCIPMessageSent event with receipts and verifier blobs."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "sender",
            "type": "pubkey"
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
            "type": "pubkey"
          },
          {
            "name": "tokenAmountBeforeTokenPoolFees",
            "type": {
              "defined": {
                "name": "protocolAmount"
              }
            }
          },
          {
            "name": "encodedMessage",
            "type": "bytes"
          },
          {
            "name": "receipts",
            "type": {
              "vec": {
                "defined": {
                  "name": "receipt"
                }
              }
            }
          },
          {
            "name": "verifierBlobs",
            "type": {
              "vec": "bytes"
            }
          }
        ]
      }
    },
    {
      "name": "ccipSendV2Params",
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
            "name": "resolutionMetadata",
            "docs": [
              "Opaque account-resolution metadata produced by account resolution."
            ],
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "ccipVersionForDestChainVersionBumped",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "previousSequenceNumber",
            "type": "u64"
          },
          {
            "name": "newSequenceNumber",
            "type": "u64"
          }
        ]
      }
    },
    {
      "name": "ccipVersionForDestChainVersionRolledBack",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "previousSequenceNumber",
            "type": "u64"
          },
          {
            "name": "newSequenceNumber",
            "type": "u64"
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
            "name": "defaultCodeVersion",
            "type": {
              "defined": {
                "name": "codeVersion"
              }
            }
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
            "name": "feeQuoter",
            "type": "pubkey"
          },
          {
            "name": "rmnRemote",
            "type": "pubkey"
          },
          {
            "name": "linkTokenMint",
            "type": "pubkey"
          },
          {
            "name": "feeAggregator",
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
          },
          {
            "name": "feeQuoter",
            "type": "pubkey"
          },
          {
            "name": "rmnRemote",
            "type": "pubkey"
          },
          {
            "name": "linkTokenMint",
            "type": "pubkey"
          },
          {
            "name": "feeAggregator",
            "type": "pubkey"
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
      "name": "crossChainGas",
      "type": {
        "kind": "struct",
        "fields": [
          "u32"
        ]
      }
    },
    {
      "name": "deriveAccountsCcipSendParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "ccipSendCaller",
            "type": "pubkey"
          },
          {
            "name": "message",
            "type": {
              "defined": {
                "name": "svm2AnyMessage"
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
            "docs": [
              "If this vector is not empty, you must call the `derive_` method again including",
              "exactly these accounts as the `remaining_accounts` field."
            ],
            "type": {
              "vec": {
                "defined": {
                  "name": "ccipRouterStateCcipAccountMeta"
                }
              }
            }
          },
          {
            "name": "accountsToSave",
            "docs": [
              "You must append these accounts at the end of a separate list. When `next_stage`",
              "is finally empty, this separate list will contain all the accounts to use for the",
              "instruction of interest."
            ],
            "type": {
              "vec": {
                "defined": {
                  "name": "ccipRouterStateCcipAccountMeta"
                }
              }
            }
          },
          {
            "name": "lookUpTablesToSave",
            "docs": [
              "Append these look up tables at the end of a list. It will contain all LUTs",
              "that the instruction of interest can use."
            ],
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "currentStage",
            "docs": [
              "Identifies the derivation stage."
            ],
            "type": "string"
          },
          {
            "name": "nextStage",
            "docs": [
              "Identifies the next derivation stage. If empty, the derivation is complete."
            ],
            "type": "string"
          }
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
                "name": "destChainConfigCcipV2"
              }
            }
          }
        ]
      }
    },
    {
      "name": "destChainCcipV2",
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
                "name": "destChainConfigCcipV2"
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
            "name": "laneCodeVersion",
            "type": {
              "defined": {
                "name": "codeVersion"
              }
            }
          },
          {
            "name": "allowedSenders",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "allowListEnabled",
            "type": "bool"
          }
        ]
      }
    },
    {
      "name": "destChainConfigCcipV2",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "laneCodeVersion",
            "type": {
              "defined": {
                "name": "codeVersion"
              }
            }
          },
          {
            "name": "addressBytesLength",
            "type": "u8"
          },
          {
            "name": "tokenReceiverAllowed",
            "type": "bool"
          },
          {
            "name": "allowedSenders",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "allowListEnabled",
            "type": "bool"
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
          },
          {
            "name": "defaultExecutor",
            "type": "pubkey"
          },
          {
            "name": "offramp",
            "type": "bytes"
          },
          {
            "name": "messageNetworkFee",
            "type": {
              "defined": {
                "name": "usdCents"
              }
            }
          },
          {
            "name": "tokenTransferNetworkFee",
            "type": {
              "defined": {
                "name": "usdCents"
              }
            }
          },
          {
            "name": "baseExecutionGasCost",
            "type": {
              "defined": {
                "name": "crossChainGas"
              }
            }
          },
          {
            "name": "maxFeePerMessage",
            "type": {
              "defined": {
                "name": "usdCents"
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
                "name": "destChainConfigCcipV2"
              }
            }
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
            "name": "messageNumber",
            "type": "u64"
          },
          {
            "name": "messageNumberToRestore",
            "type": "u64"
          },
          {
            "name": "restoreOnAction",
            "type": {
              "defined": {
                "name": "restoreOnAction"
              }
            }
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
      "name": "getFeeParams",
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
            "name": "resolutionMetadata",
            "docs": [
              "Opaque account-resolution metadata produced by account resolution."
            ],
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
            "name": "amount",
            "type": "u64"
          },
          {
            "name": "juels",
            "type": "u128"
          },
          {
            "name": "token",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "getFeeResultV2",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "amount",
            "type": "u64"
          },
          {
            "name": "token",
            "type": "pubkey"
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
            "name": "svm2anyRampMessage",
            "type": {
              "defined": {
                "name": "svm2AnyRampMessage"
              }
            }
          },
          {
            "name": "svm2anyTokenTransfer",
            "type": {
              "defined": {
                "name": "svm2AnyTokenTransfer"
              }
            }
          },
          {
            "name": "rampMessageHeader",
            "type": {
              "defined": {
                "name": "rampMessageHeader"
              }
            }
          },
          {
            "name": "crossChainAmount",
            "type": {
              "defined": {
                "name": "crossChainAmount"
              }
            }
          },
          {
            "name": "nonce",
            "type": {
              "defined": {
                "name": "nonce"
              }
            }
          },
          {
            "name": "receipt",
            "type": {
              "defined": {
                "name": "receipt"
              }
            }
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
            "name": "orderedNonce",
            "type": "u64"
          },
          {
            "name": "totalNonce",
            "type": "u64"
          }
        ]
      }
    },
    {
      "name": "offrampAdded",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "sourceChainSelector",
            "type": "u64"
          },
          {
            "name": "offramp",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "offrampRemoved",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "sourceChainSelector",
            "type": "u64"
          },
          {
            "name": "offramp",
            "type": "pubkey"
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
      "name": "pdaUpgraded",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "address",
            "type": "pubkey"
          },
          {
            "name": "oldVersion",
            "type": "u8"
          },
          {
            "name": "newVersion",
            "type": "u8"
          },
          {
            "name": "name",
            "type": "string"
          },
          {
            "name": "seeds",
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "poolInterfaceImplementationUpdated",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "token",
            "type": "pubkey"
          },
          {
            "name": "interfaceVersion",
            "type": "u8"
          },
          {
            "name": "accountResolution",
            "type": {
              "defined": {
                "name": "resolutionType"
              }
            }
          }
        ]
      }
    },
    {
      "name": "poolSet",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "token",
            "type": "pubkey"
          },
          {
            "name": "previousPoolLookupTable",
            "type": "pubkey"
          },
          {
            "name": "newPoolLookupTable",
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
      "name": "rampMessageHeader",
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
            "name": "sourceChainSelector",
            "type": "u64"
          },
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "sequenceNumber",
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
      "name": "receipt",
      "docs": [
        "CCIP 2.0 fee/gas receipt for a single entity (verifier, token pool, executor, network)."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "issuer",
            "type": "pubkey"
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
          },
          {
            "name": "feeTokenAmount",
            "type": "u64"
          },
          {
            "name": "extraArgs",
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "receiverRegistered",
      "type": {
        "kind": "struct",
        "fields": [
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
            "name": "onrampLookupTable",
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
            "name": "onrampLookupTable",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "registerReceiverParams",
      "type": {
        "kind": "struct",
        "fields": [
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
      "name": "resolutionType",
      "type": {
        "kind": "enum",
        "variants": [
          {
            "name": "noResolution"
          },
          {
            "name": "standard"
          },
          {
            "name": "withoutExtendedLookupTable"
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
      "name": "restoreOnAction",
      "type": {
        "kind": "enum",
        "variants": [
          {
            "name": "none"
          },
          {
            "name": "upgrade"
          },
          {
            "name": "rollback"
          }
        ]
      }
    },
    {
      "name": "routerConfigObservation",
      "docs": [
        "Global configuration of the router, as observed by off-chain consumers."
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
            "name": "feeQuoter",
            "type": "pubkey"
          },
          {
            "name": "rmnRemote",
            "type": "pubkey"
          },
          {
            "name": "linkTokenMint",
            "type": "pubkey"
          },
          {
            "name": "feeAggregator",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "routerDestChainV2Observation",
      "docs": [
        "Per-destination-chain state and configuration of the CCIP 2.0 onramp path, as observed by",
        "off-chain consumers. Read from the `dest_chain_state_v2` PDA.",
        "",
        "The list of allowed senders is reported as a count only, as its size is unbounded by",
        "configuration."
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
            "name": "messageNumber",
            "type": "u64"
          },
          {
            "name": "messageNumberToRestore",
            "type": "u64"
          },
          {
            "name": "restoreOnAction",
            "type": {
              "defined": {
                "name": "restoreOnAction"
              }
            }
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
            "name": "allowListEnabled",
            "type": "bool"
          },
          {
            "name": "allowedSendersCount",
            "type": "u32"
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
          },
          {
            "name": "defaultExecutor",
            "type": "pubkey"
          },
          {
            "name": "offramp",
            "docs": [
              "Destination offramp address as raw bytes (not ABI encoded)."
            ],
            "type": "bytes"
          },
          {
            "name": "messageNetworkFee",
            "type": {
              "defined": {
                "name": "usdCents"
              }
            }
          },
          {
            "name": "tokenTransferNetworkFee",
            "type": {
              "defined": {
                "name": "usdCents"
              }
            }
          },
          {
            "name": "baseExecutionGasCost",
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
      "name": "svm2AnyRampMessage",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "header",
            "type": {
              "defined": {
                "name": "rampMessageHeader"
              }
            }
          },
          {
            "name": "sender",
            "type": "pubkey"
          },
          {
            "name": "data",
            "type": "bytes"
          },
          {
            "name": "receiver",
            "type": "bytes"
          },
          {
            "name": "extraArgs",
            "type": "bytes"
          },
          {
            "name": "feeToken",
            "type": "pubkey"
          },
          {
            "name": "tokenAmounts",
            "type": {
              "vec": {
                "defined": {
                  "name": "svm2AnyTokenTransfer"
                }
              }
            }
          },
          {
            "name": "feeTokenAmount",
            "type": {
              "defined": {
                "name": "crossChainAmount"
              }
            }
          },
          {
            "name": "feeValueJuels",
            "type": {
              "defined": {
                "name": "crossChainAmount"
              }
            }
          }
        ]
      }
    },
    {
      "name": "svm2AnyTokenTransfer",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "sourcePoolAddress",
            "type": "pubkey"
          },
          {
            "name": "destTokenAddress",
            "type": "bytes"
          },
          {
            "name": "extraData",
            "type": "bytes"
          },
          {
            "name": "amount",
            "type": {
              "defined": {
                "name": "crossChainAmount"
              }
            }
          },
          {
            "name": "destExecData",
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
      "name": "setPoolInterfaceImplementationParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "interfaceVersion",
            "type": "u8"
          },
          {
            "name": "accountResolution",
            "type": {
              "defined": {
                "name": "resolutionType"
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
      "name": "updateChainSelectorV2Params",
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
                "name": "destChainConfigCcipV2"
              }
            }
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
    },
    {
      "name": "ccipRouterStateCcipAccountMeta",
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
