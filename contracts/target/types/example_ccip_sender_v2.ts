/**
 * Program IDL in camelCase format in order to be used in JS/TS.
 *
 * Note that this is only a type helper and is not the actual IDL. The original
 * IDL can be found at `target/idl/example_ccip_sender_v2.json`.
 */
export type ExampleCcipSenderV2 = {
  "address": "7VM6AK4DwejGLZ1x28zHr8GCQhtTuHcQwQdHGVpi12eh",
  "metadata": {
    "name": "exampleCcipSenderV2",
    "version": "2.0.1",
    "spec": "0.1.0",
    "description": "Created with Anchor"
  },
  "docs": [
    "An example CCIP 2.0 Sender Program,",
    "",
    "This file contains no logic, only the entrypoints into `instructions/`. Adding an",
    "instruction means adding a module there and an entrypoint here."
  ],
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
                  115,
                  116,
                  97,
                  116,
                  101
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
      "name": "ccipSend",
      "docs": [
        "Sends a CCIP 2.0 message, paying for it out of the caller's tokens.",
        "",
        "# Accounts",
        "",
        "Resolve them with [`resolve_accounts`], which produces the whole list.",
        "",
        "# Fees",
        "",
        "The caller states an upper bound (`params.max_fee`) rather than having this program quote the",
        "message onchain: `get_fee_v2` repeats most of `ccip_send_v2`'s work, and paying for it twice",
        "inside a CPI leaves very little of the compute budget for the send itself. Quote offchain,",
        "add slippage, and pass the result. Whatever the router does not bill is returned to the",
        "caller before this instruction ends, and the amount actually billed is reported in",
        "[`MessageSent`](crate::state::MessageSent)."
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
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  115,
                  116,
                  97,
                  116,
                  101
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
          "name": "authorityFeeTokenAta",
          "docs": [
            "intentionally untyped: on the native-SOL fee path there is no fee ATA to pass, so",
            "deserialising it as a token account would fail."
          ],
          "writable": true
        },
        {
          "name": "ccipSender",
          "docs": [
            "pulls them."
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
                  115,
                  101,
                  110,
                  100,
                  101,
                  114
                ]
              }
            ]
          }
        },
        {
          "name": "router"
        },
        {
          "name": "feeTokenProgram"
        },
        {
          "name": "feeTokenMint",
          "docs": [
            "The mint fees are quoted in. Paying in native SOL still quotes against wrapped SOL, which is",
            "what the router normalizes the zero address to, so this is never the zero address itself."
          ]
        },
        {
          "name": "ccipSenderFeeTokenAta",
          "docs": [
            "`max_fee` and the router then bills, validated by derivation in the handler. Untyped because",
            "native SOL passes the zero address here."
          ]
        },
        {
          "name": "feeBillingSigner"
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
              "name": "ccipSendParams"
            }
          }
        }
      ]
    },
    {
      "name": "initCcipSenderAta",
      "docs": [
        "Creates this program's associated token account for `mint`, idempotently."
      ],
      "discriminator": [
        143,
        102,
        12,
        23,
        28,
        3,
        203,
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
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              }
            ]
          }
        },
        {
          "name": "ccipSenderAta",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "account",
                "path": "ccipSender"
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
          "name": "mint"
        },
        {
          "name": "ccipSender",
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
                  115,
                  101,
                  110,
                  100,
                  101,
                  114
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
          "name": "tokenProgram"
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
      "args": []
    },
    {
      "name": "initChainConfig",
      "docs": [
        "Creates the config for a destination chain. Its existence is what allows sends to that",
        "chain; `remove_chain_config` closes it again."
      ],
      "discriminator": [
        21,
        94,
        4,
        115,
        130,
        211,
        10,
        229
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
                  115,
                  116,
                  97,
                  116,
                  101
                ]
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
        }
      ],
      "args": [
        {
          "name": "params",
          "type": {
            "defined": {
              "name": "chainConfigParams"
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
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              }
            ]
          }
        },
        {
          "name": "ccipSender",
          "docs": [
            "read here, to be stored in `state`."
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
                  115,
                  101,
                  110,
                  100,
                  101,
                  114
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
          "address": "7VM6AK4DwejGLZ1x28zHr8GCQhtTuHcQwQdHGVpi12eh"
        },
        {
          "name": "programData"
        }
      ],
      "args": [
        {
          "name": "router",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "removeChainConfig",
      "docs": [
        "Disables a destination chain by closing its config account. Reversible via",
        "`init_chain_config`."
      ],
      "discriminator": [
        212,
        220,
        209,
        147,
        118,
        171,
        38,
        48
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
                  115,
                  116,
                  97,
                  116,
                  101
                ]
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
        }
      ]
    },
    {
      "name": "resolveAccountsForwardRouterStage",
      "docs": [
        "Continues resolution by running the router's next `ccip_send_v2` stage."
      ],
      "discriminator": [
        149,
        140,
        176,
        80,
        244,
        70,
        155,
        7
      ],
      "accounts": [
        {
          "name": "chainConfig"
        },
        {
          "name": "router",
          "docs": [
            "list that a wrong one would spoil."
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
      "name": "resolveAccountsStart",
      "docs": [
        "Entrypoint to resolve the accounts `ccip_send` needs. See [`resolve_accounts`] for the flow and for",
        "the accounts this stage itself takes."
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
          "name": "state",
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              }
            ]
          }
        },
        {
          "name": "chainConfig",
          "docs": [
            "The destination chain's config, source of the recipient and extra args `ccip_send` puts in",
            "the message. Its PDA is checked in the handler, once the selector has been parsed out of",
            "`params.ix_data`."
          ]
        },
        {
          "name": "feeTokenMint",
          "docs": [
            "program that owns it, which the caller's fee ATA is derived with. Checked against the",
            "message's fee token in the handler."
          ]
        },
        {
          "name": "router"
        },
        {
          "name": "routerConfig",
          "docs": [
            "Checked by derivation."
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
                  115,
                  116,
                  97,
                  116,
                  101
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
      "name": "updateChainConfig",
      "docs": [
        "Updates a destination chain's config. Separate from `init_chain_config` because changing",
        "the extra args needs a realloc."
      ],
      "discriminator": [
        192,
        127,
        91,
        206,
        38,
        245,
        41,
        121
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
                  115,
                  116,
                  97,
                  116,
                  101
                ]
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
        }
      ],
      "args": [
        {
          "name": "params",
          "type": {
            "defined": {
              "name": "chainConfigParams"
            }
          }
        }
      ]
    },
    {
      "name": "updateRouter",
      "discriminator": [
        32,
        109,
        12,
        153,
        101,
        129,
        64,
        70
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
                  115,
                  116,
                  97,
                  116,
                  101
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
          "name": "newRouter",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "withdrawTokens",
      "discriminator": [
        2,
        4,
        225,
        61,
        19,
        182,
        106,
        170
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
                  115,
                  116,
                  97,
                  116,
                  101
                ]
              }
            ]
          }
        },
        {
          "name": "programTokenAccount",
          "writable": true
        },
        {
          "name": "toTokenAccount",
          "writable": true
        },
        {
          "name": "mint"
        },
        {
          "name": "tokenProgram"
        },
        {
          "name": "ccipSender",
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
                  115,
                  101,
                  110,
                  100,
                  101,
                  114
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
          "name": "params",
          "type": {
            "defined": {
              "name": "withdrawTokensParams"
            }
          }
        }
      ]
    }
  ],
  "accounts": [
    {
      "name": "baseState",
      "discriminator": [
        46,
        139,
        13,
        192,
        80,
        181,
        96,
        46
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
    }
  ],
  "events": [
    {
      "name": "messageSent",
      "discriminator": [
        116,
        70,
        224,
        76,
        128,
        28,
        110,
        55
      ]
    }
  ],
  "errors": [
    {
      "code": 6000,
      "name": "invalidRouter",
      "msg": "Invalid router address"
    },
    {
      "code": 6001,
      "name": "onlyOwner",
      "msg": "Address is not owner"
    },
    {
      "code": 6002,
      "name": "onlyProposedOwner",
      "msg": "Address is not proposed_owner"
    },
    {
      "code": 6003,
      "name": "invalidProposedOwner",
      "msg": "Proposed owner is invalid"
    },
    {
      "code": 6004,
      "name": "invalidForwardedAccounts",
      "msg": "The forwarded router account list is malformed"
    },
    {
      "code": 6005,
      "name": "invalidSourceTokenAccount",
      "msg": "The source token account is not the caller's associated token account"
    },
    {
      "code": 6006,
      "name": "invalidFeeTokenAccount",
      "msg": "The fee token account is not the caller's associated token account"
    },
    {
      "code": 6007,
      "name": "invalidCcipSenderAta",
      "msg": "The token account is not the ccip_sender PDA's associated token account"
    },
    {
      "code": 6008,
      "name": "feeOverflow",
      "msg": "The max fee plus transfer amount overflows"
    },
    {
      "code": 6009,
      "name": "feeAccountingFailed",
      "msg": "The fee the router billed could not be reconciled against max_fee"
    },
    {
      "code": 6010,
      "name": "missingCpiReturnData",
      "msg": "CPI target did not return data"
    },
    {
      "code": 6011,
      "name": "invalidCpiReturnProgram",
      "msg": "CPI return data came from an unexpected program"
    },
    {
      "code": 6012,
      "name": "invalidResolutionInstructionData",
      "msg": "The instruction data to resolve accounts for is malformed or unsupported"
    },
    {
      "code": 6013,
      "name": "invalidChainConfig",
      "msg": "The chain config is not the one for the message's destination chain"
    },
    {
      "code": 6014,
      "name": "invalidRouterConfig",
      "msg": "The account passed as the router's config is not its config PDA"
    },
    {
      "code": 6015,
      "name": "missingTransferTokenMint",
      "msg": "The transferred token's mint is missing from the resolution accounts"
    },
    {
      "code": 6016,
      "name": "invalidResolutionResponse",
      "msg": "The router returned a malformed account resolution response"
    },
    {
      "code": 6017,
      "name": "maxFeeExceeded",
      "msg": "The native SOL fee billed by the router exceeds max_fee"
    },
    {
      "code": 6018,
      "name": "serializationFailed",
      "msg": "Failed to serialize the router instruction data"
    },
    {
      "code": 6019,
      "name": "invalidMessageId",
      "msg": "The router returned a malformed message id"
    },
    {
      "code": 6020,
      "name": "unauthorized",
      "msg": "Address is not the program upgrade authority"
    }
  ],
  "types": [
    {
      "name": "baseState",
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
            "name": "ccipSenderBump",
            "docs": [
              "Bump of the `[b\"ccip_sender\"]` PDA, which signs the CPI into the router and custodies the",
              "tokens being transferred. Stored here so `ccip_send` can `invoke_signed` without declaring",
              "that PDA as one of its own accounts."
            ],
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
            "name": "router",
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
      "name": "ccipSendParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "tokenAmount",
            "docs": [
              "The CCIP 2.0 onramp carries at most one token transfer per message."
            ],
            "type": {
              "option": {
                "defined": {
                  "name": "svmTokenAmount"
                }
              }
            }
          },
          {
            "name": "data",
            "type": "bytes"
          },
          {
            "name": "feeToken",
            "docs": [
              "`Pubkey::default()` pays fees in native SOL, deposited by the caller into the `ccip_sender`",
              "PDA for the router to bill."
            ],
            "type": "pubkey"
          },
          {
            "name": "maxFee",
            "docs": [
              "Upper bound on what the router may bill for this message, in `fee_token` units. The router bills the exact",
              "quoted fee, and the unspent remainder is returned to the caller before the instruction ends.",
              "",
              "A fee above it reverts the send. In native SOL that is `MaxFeeExceeded` when the",
              "`ccip_sender` PDA's own balance covers the excess, and the System Program's insufficient",
              "lamports error otherwise; in a token it is the token program's insufficient funds error.",
              "",
              "To dimension this fee, the caller can call `get_fee_v2` beforehand and add whatever",
              "slippage they want to accept on top."
            ],
            "type": "u64"
          },
          {
            "name": "resolutionMetadata",
            "docs": [
              "Opaque blob produced by the router's account resolution, which this program's own",
              "[`resolve_accounts`] returns from its final round alongside the account list."
            ],
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "chainConfigParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "chainSelector",
            "type": "u64"
          },
          {
            "name": "recipient",
            "type": "bytes"
          },
          {
            "name": "extraArgsBytes",
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "messageSent",
      "docs": [
        "Emitted after the router accepted the message.",
        "",
        "`fee_amount` is what the router actually billed, not the `max_fee` the caller allowed."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "destChainSelector",
            "type": "u64"
          },
          {
            "name": "feeToken",
            "type": "pubkey"
          },
          {
            "name": "feeAmount",
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
            "name": "recipient",
            "type": "bytes"
          },
          {
            "name": "extraArgsBytes",
            "type": "bytes"
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
      "name": "withdrawTokensParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "amount",
            "type": "u64"
          },
          {
            "name": "decimals",
            "type": "u8"
          }
        ]
      }
    }
  ]
};
