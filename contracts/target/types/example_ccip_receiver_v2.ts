/**
 * Program IDL in camelCase format in order to be used in JS/TS.
 *
 * Note that this is only a type helper and is not the actual IDL. The original
 * IDL can be found at `target/idl/example_ccip_receiver_v2.json`.
 */
export type ExampleCcipReceiverV2 = {
  "address": "CbYiUaBi9nx1m3XhADWTdPoN25zeg5qDFBpbEfWKmrCL",
  "metadata": {
    "name": "exampleCcipReceiverV2",
    "version": "2.0.0",
    "spec": "0.1.0",
    "description": "Created with Anchor"
  },
  "docs": [
    "This program is an example of a CCIP v2 Receiver Program, intended as starter code for v2",
    "customers. It keeps the example receiver's behavior (ownership, router, sender approval, token",
    "withdrawal, and a `MessageReceived` event) under the renamed `ccip_receive_v2` entrypoint, and",
    "adds the CCIP v2 receiver interface: `get_ccvs_and_finality_config`, backed by per-source-chain",
    "CCV/finality configuration."
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
      "name": "approveSender",
      "docs": [
        "Creates a PDA approving a (source chain, remote sender) pair to deliver messages."
      ],
      "discriminator": [
        110,
        115,
        180,
        233,
        200,
        99,
        131,
        255
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
          "name": "approvedSender",
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
          "name": "params",
          "type": {
            "defined": {
              "name": "senderParams"
            }
          }
        }
      ]
    },
    {
      "name": "ccipReceiveV2",
      "docs": [
        "Called by the CCIP OffRamp to execute a v2 CCIP message. The method name is",
        "`ccip_receive_v2` (distinct discriminator from v1 `ccip_receive`), so the OffRamp dispatches",
        "to it based on the receiver's stored interface version."
      ],
      "discriminator": [
        193,
        155,
        158,
        156,
        161,
        212,
        122,
        116
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
                  101,
                  120,
                  101,
                  99,
                  117,
                  116,
                  105,
                  111,
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
                "kind": "const",
                "value": [
                  172,
                  74,
                  42,
                  0,
                  95,
                  37,
                  42,
                  77,
                  95,
                  10,
                  215,
                  182,
                  190,
                  169,
                  20,
                  190,
                  3,
                  108,
                  137,
                  66,
                  14,
                  117,
                  148,
                  59,
                  78,
                  175,
                  84,
                  12,
                  12,
                  45,
                  185,
                  149
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
            "Must be second."
          ]
        },
        {
          "name": "allowedOfframp",
          "docs": [
            "source chain. Owned by the configured router iff it exists (i.e. the offramp is allowed).",
            "Must be third."
          ]
        },
        {
          "name": "approvedSender"
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
          "name": "eventAuthority"
        },
        {
          "name": "program"
        }
      ],
      "args": [
        {
          "name": "message",
          "type": {
            "defined": {
              "name": "any2SvmMessage"
            }
          }
        }
      ]
    },
    {
      "name": "getCcvsAndFinalityConfig",
      "docs": [
        "CCIP v2 receiver interface: returns the required/optional CCVs, optional threshold, and",
        "allowed finality config for a source chain. Solana analog of EVM",
        "`IAny2EVMMessageReceiverV2.getCCVsAndFinalityConfig`."
      ],
      "discriminator": [
        9,
        11,
        215,
        24,
        67,
        214,
        164,
        38
      ],
      "accounts": [
        {
          "name": "sourceChainCcvConfig",
          "docs": [
            "seeds only. Derivable from `input.source_chain_selector`, so the OffRamp can supply it.",
            "This is the only account the query needs — it is keyed solely by source chain."
          ],
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  118,
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
                "path": "input.sourceChainSelector"
              }
            ]
          }
        }
      ],
      "args": [
        {
          "name": "input",
          "type": {
            "defined": {
              "name": "getReceiverCcvsIn"
            }
          }
        }
      ],
      "returns": {
        "defined": {
          "name": "getReceiverCcvsOut"
        }
      }
    },
    {
      "name": "initialize",
      "docs": [
        "Initialization is the responsibility of the external user; CCIP does not initialize",
        "receiver accounts."
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
          "address": "CbYiUaBi9nx1m3XhADWTdPoN25zeg5qDFBpbEfWKmrCL"
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
      "name": "registerWithRouter",
      "docs": [
        "Registers this program as a CCIP v2 receiver in the router. Callable only by",
        "the program's upgrade authority; CPIs the router's `register_receiver`, signing with this",
        "program's registration PDA. The OffRamp then reads that registry to route messages through",
        "the v2 receiver flow."
      ],
      "discriminator": [
        170,
        59,
        95,
        93,
        99,
        191,
        133,
        58
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
            "The owner; also the rent payer forwarded to the router CPI."
          ],
          "writable": true,
          "signer": true
        },
        {
          "name": "registrationSigner",
          "docs": [
            "The receiver-side signer PDA that authenticates the CPI to the router. Only this program can",
            "`invoke_signed` for it, so the router accepts it as proof of self-registration."
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
                  97,
                  116,
                  105,
                  111,
                  110
                ]
              },
              {
                "kind": "account",
                "path": "routerProgram"
              }
            ]
          }
        },
        {
          "name": "routerProgram",
          "docs": [
            "The router program to register with. Bound to the receiver's configured router."
          ]
        },
        {
          "name": "receiverRegistry",
          "docs": [
            "The router-owned `ReceiverRegistry` PDA, created/updated by the router CPI. Validated by its",
            "PDA derivation under the router (`seeds::program`); `owner` can't be asserted here because on",
            "first registration the router creates it via `init_if_needed`, so it's still System-owned",
            "when this context is checked."
          ],
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
                "kind": "const",
                "value": [
                  172,
                  74,
                  42,
                  0,
                  95,
                  37,
                  42,
                  77,
                  95,
                  10,
                  215,
                  182,
                  190,
                  169,
                  20,
                  190,
                  3,
                  108,
                  137,
                  66,
                  14,
                  117,
                  148,
                  59,
                  78,
                  175,
                  84,
                  12,
                  12,
                  45,
                  185,
                  149
                ]
              }
            ],
            "program": {
              "kind": "account",
              "path": "routerProgram"
            }
          }
        },
        {
          "name": "routerConfig",
          "docs": [
            "The router `Config` PDA, validated by the router's `RegisterReceiver` context."
          ]
        },
        {
          "name": "routerEventAuthority",
          "docs": [
            "The router's `__event_authority` PDA, forwarded to satisfy the router's `#[event_cpi]` on",
            "`register_receiver` (used for the `ReceiverRegistered` event)."
          ]
        },
        {
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        }
      ],
      "args": []
    },
    {
      "name": "removeCcvConfig",
      "docs": [
        "Removes the CCV config for a source chain, reverting it to the lane defaults + full finality."
      ],
      "discriminator": [
        18,
        52,
        80,
        71,
        178,
        138,
        209,
        55
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
          "name": "sourceChainCcvConfig",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  118,
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
                "path": "sourceChainSelector"
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
      "name": "setCcvConfig",
      "docs": [
        "Sets (or updates) the CCV + finality requirements for messages from a source chain."
      ],
      "discriminator": [
        201,
        250,
        132,
        189,
        225,
        27,
        127,
        164
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
          "name": "sourceChainCcvConfig",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  99,
                  118,
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
                "path": "params.sourceChainSelector"
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
              "name": "setCcvConfigParams"
            }
          }
        }
      ]
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
      "name": "unapproveSender",
      "discriminator": [
        156,
        35,
        66,
        182,
        129,
        232,
        105,
        176
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
          "name": "approvedSender",
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
          "name": "params",
          "type": {
            "defined": {
              "name": "senderParams"
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
          "name": "tokenAdmin",
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
                  110
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
      "name": "approvedSender",
      "discriminator": [
        141,
        66,
        47,
        213,
        85,
        194,
        71,
        166
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
      "name": "sourceChainCcvConfig",
      "discriminator": [
        184,
        192,
        216,
        116,
        223,
        146,
        124,
        13
      ]
    }
  ],
  "events": [
    {
      "name": "ccvConfigRemoved",
      "discriminator": [
        231,
        13,
        193,
        142,
        21,
        172,
        199,
        73
      ]
    },
    {
      "name": "ccvConfigSet",
      "discriminator": [
        97,
        138,
        177,
        251,
        203,
        124,
        151,
        158
      ]
    },
    {
      "name": "messageReceived",
      "discriminator": [
        231,
        68,
        47,
        77,
        173,
        241,
        157,
        166
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
      "name": "invalidCaller",
      "msg": "Caller is not allowed"
    },
    {
      "code": 6004,
      "name": "invalidProposedOwner",
      "msg": "Proposed owner is invalid"
    },
    {
      "code": 6005,
      "name": "senderAddressTooLong",
      "msg": "Remote sender address exceeds the maximum PDA seed length"
    },
    {
      "code": 6100,
      "name": "duplicateCcv",
      "msg": "Duplicate CCV in configuration"
    },
    {
      "code": 6101,
      "name": "invalidOptionalThreshold",
      "msg": "Optional threshold is invalid for the given optional CCVs"
    },
    {
      "code": 6102,
      "name": "zeroPubkeyNotAllowedAsOptional",
      "msg": "Zero pubkey is not allowed as an optional CCV"
    },
    {
      "code": 6103,
      "name": "invalidFinalityConfig",
      "msg": "Invalid finality config"
    },
    {
      "code": 6104,
      "name": "tooManyCcVs",
      "msg": "Too many CCVs configured"
    },
    {
      "code": 6105,
      "name": "unsupportedConfigVersion",
      "msg": "Unsupported SourceChainCcvConfig account version"
    },
    {
      "code": 6106,
      "name": "unauthorized",
      "msg": "Address is not the program upgrade authority"
    }
  ],
  "types": [
    {
      "name": "any2SvmMessage",
      "docs": [
        "Message delivered to this receiver via the `ccip_receive_v2` CPI.",
        "",
        "WIRE-FORMAT CONTRACT: the OffRamp must serialize a byte-identical struct when it CPIs",
        "`ccip_receive_v2`. Note this deliberately carries the token transfer as `Option<TokenTransfer>`",
        "(a v2 message carries at most one token transfer), which is NOT Borsh-compatible with the",
        "OffRamp's legacy v1 `Any2SVMMessage` (`token_amounts: Vec<SVMTokenAmount>` — `Option` is a 1-byte",
        "tag, `Vec` a 4-byte length prefix). This type is currently copied here rather than shared.",
        "",
        "The OffRamp serializes a matching struct (`ccip-offramp`'s `Any2SVMMessageV2`) when it CPIs",
        "`ccip_receive_v2`."
      ],
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
            "name": "sender",
            "type": "bytes"
          },
          {
            "name": "data",
            "type": "bytes"
          },
          {
            "name": "tokenTransfer",
            "type": {
              "option": {
                "defined": {
                  "name": "tokenTransfer"
                }
              }
            }
          }
        ]
      }
    },
    {
      "name": "approvedSender",
      "docs": [
        "Marker PDA proving a (source chain, remote sender) pair is approved. Approval is conveyed by the",
        "account's mere existence — like the router's `AllowedOfframp` marker, it stores no data and is",
        "validated purely by its PDA derivation plus program ownership (enforced by `Account<_>`)."
      ],
      "type": {
        "kind": "struct",
        "fields": []
      }
    },
    {
      "name": "ccvConfigRemoved",
      "docs": [
        "Emitted when the CCV/finality configuration for a source chain is removed."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "sourceChainSelector",
            "type": "u64"
          }
        ]
      }
    },
    {
      "name": "ccvConfigSet",
      "docs": [
        "Emitted when the CCV/finality configuration for a source chain is set or updated.",
        "Mirrors EVM `CCIPClientExampleWithCCVs.CCVConfigSet`."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "sourceChainSelector",
            "type": "u64"
          },
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
          },
          {
            "name": "allowedFinalityConfig",
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
      "name": "config",
      "docs": [
        "Config contains the Config for core safety checks that can be leveraged by the implementer."
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
            "name": "router",
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
      "name": "getReceiverCcvsIn",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "sourceChainSelector",
            "type": "u64"
          },
          {
            "name": "sender",
            "docs": [
              "Cross-chain sender address, provided so receivers can implement sender-specific policies."
            ],
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "getReceiverCcvsOut",
      "docs": [
        "Receiver's CCV + finality requirements for a given source chain.",
        "",
        "Placeholder semantics (mirrors EVM `address(0)` via [`crate::INCLUDE_DEFAULT_CCVS`]):",
        "an empty `required_ccvs`/`optional_ccvs` with `optional_threshold == 0` means \"use the lane",
        "default CCVs\". An `INCLUDE_DEFAULT_CCVS` entry inside `required_ccvs` is the explicit \"also",
        "include the lane defaults\" marker. `INCLUDE_DEFAULT_CCVS` is not permitted in `optional_ccvs`."
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
          },
          {
            "name": "allowedFinalityConfig",
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
      "name": "messageReceived",
      "docs": [
        "Emitted by `ccip_receive_v2` when a message is delivered. Carries the source chain and sender",
        "(in addition to the message id) so indexers can attribute the delivery without re-fetching the",
        "message — the EVM receiver interface likewise hands the full message context to the callback."
      ],
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
            "name": "sender",
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
      "name": "senderParams",
      "docs": [
        "Params for [`approve_sender`] / [`unapprove_sender`] (INV-LINT-PARAMS: single struct arg)."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "chainSelector",
            "type": "u64"
          },
          {
            "name": "remoteSender",
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "setCcvConfigParams",
      "docs": [
        "Arguments for [`set_ccv_config`]. Bundled into a single struct (INV-LINT-PARAMS) and to mirror",
        "the token pool's `SetCcvConfigParams`."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "sourceChainSelector",
            "type": "u64"
          },
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
          },
          {
            "name": "allowedFinalityConfig",
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
      "name": "sourceChainCcvConfig",
      "docs": [
        "Per-source-chain CCV + finality requirements, queried by the OffRamp via",
        "`get_ccvs_and_finality_config` (CCIP v2). Derivable purely from `source_chain_selector`, so",
        "account resolution for the query CPI is trivial. May be absent (uninitialized) — in which case",
        "the receiver falls back to the lane default CCVs and full finality."
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
          },
          {
            "name": "allowedFinalityConfig",
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
      "name": "tokenTransfer",
      "docs": [
        "The single (optional) token transfer carried by a v2 `Any2SVMMessage`.",
        "",
        "A v2 message carries at most one token transfer. Beyond the amount, this exposes WHERE the",
        "OffRamp delivered the tokens so the receiver can verify who was credited on a (programmatic)",
        "token transfer — it mirrors the OffRamp's own `token_receiver` / `token_receiver_usage` inputs",
        "(see `ccip_common::v2::validate_and_parse_offramp_token_accounts_v2`)."
      ],
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "tokenAmount",
            "type": {
              "defined": {
                "name": "svmTokenAmount"
              }
            }
          },
          {
            "name": "tokenReceiver",
            "docs": [
              "The account the OffRamp credited, interpreted per `token_receiver_usage`: with `UseAsIs` it",
              "is the token account directly; otherwise it is the wallet the ATA is derived from (with",
              "mint = `token_amount.token`)."
            ],
            "type": "pubkey"
          },
          {
            "name": "tokenReceiverUsage",
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
      "name": "withdrawTokensParams",
      "docs": [
        "Params for [`withdraw_tokens`] (INV-LINT-PARAMS: single struct arg)."
      ],
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
