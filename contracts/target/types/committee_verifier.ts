/**
 * Program IDL in camelCase format in order to be used in JS/TS.
 *
 * Note that this is only a type helper and is not the actual IDL. The original
 * IDL can be found at `target/idl/committee_verifier.json`.
 */
export type CommitteeVerifier = {
  "address": "CV2s8jsYhx9th8pyZ4pN7apLELAB42e5j4XqWhqTdWCm",
  "metadata": {
    "name": "committeeVerifier",
    "version": "2.0.0",
    "spec": "0.1.0",
    "description": "CCIP Committee Verifier - Cross-Chain Verifier for CCIP 2.0"
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
      "name": "acceptStorageLocationAdmin",
      "docs": [
        "Accepts a pending storage location admin transfer. Must be called by the",
        "proposed storage location admin."
      ],
      "discriminator": [
        203,
        192,
        252,
        228,
        60,
        209,
        225,
        69
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
      "name": "editChainConfig",
      "docs": [
        "Overwrites the admin configuration of an already configured remote chain, leaving its",
        "signer and allowlist configuration untouched.",
        "",
        "Can only be called by the owner."
      ],
      "discriminator": [
        147,
        237,
        137,
        189,
        114,
        229,
        129,
        109
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
              "name": "setChainConfigParams"
            }
          }
        }
      ]
    },
    {
      "name": "getStorageLocations",
      "docs": [
        "Returns the storage location identifiers for this CCV. Each entry",
        "uniquely identifies a storage location — this can be an address, a URL,",
        "or any other identifier that makes sense for the CCV.",
        "",
        "This is used by the executor(s) to know where to look for the proof",
        "data that the CCV has produced.",
        "",
        "Entries are returned as raw bytes: the program never interprets them, so it does not validate",
        "that they are UTF-8. Callers that expect text must decode them and handle invalid input."
      ],
      "discriminator": [
        179,
        240,
        141,
        203,
        208,
        84,
        6,
        94
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
      "name": "getVerifierFee",
      "docs": [
        "Quotes the fee, including gas and payload size, for verifying a CCIP",
        "message to a destination chain.",
        "",
        "Takes the user-facing [`SVM2AnyMessage`] rather than a wire-format",
        "message, since this is called on the sending (onramp) side before the",
        "message is finalized.",
        "",
        "This uses identical context as `start_verification_at_source`, as both instructions are",
        "intended to be executed as CPIs from the same caller. Account resolution",
        "for `start_verification_at_source` should suffice for both."
      ],
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
      "name": "initChainConfig",
      "docs": [
        "Creates the configuration for a remote chain and sets its admin configuration.",
        "Fails if the remote chain is already configured; use `edit_chain_config` instead.",
        "",
        "Can only be called by the owner."
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
      "name": "initialize",
      "docs": [
        "Initializes the Committee Verifier Config account.",
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
          "address": "CV2s8jsYhx9th8pyZ4pN7apLELAB42e5j4XqWhqTdWCm"
        },
        {
          "name": "programData"
        }
      ],
      "args": []
    },
    {
      "name": "nestedResolveAccountsStart",
      "docs": [
        "Entrypoint to resolve accounts for the instructions invoked via CPI",
        "(`verify`, `get_verifier_fee` and `start_verification_at_source`)."
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
      "name": "observeConfig",
      "docs": [
        "Returns this CCV's global configuration, for observability consumers.",
        "",
        "Permissionless and read-only: meant to be executed through `simulateTransaction`."
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
          "name": "committeeVerifierConfigObservation"
        }
      }
    },
    {
      "name": "observeRemoteChain",
      "docs": [
        "Returns this CCV's configuration for a single remote (source) chain, for observability",
        "consumers.",
        "",
        "Permissionless and read-only: meant to be executed through `simulateTransaction`. Fails if",
        "the lane has no configuration, which callers can read as \"not configured\"."
      ],
      "discriminator": [
        114,
        199,
        152,
        183,
        120,
        219,
        9,
        238
      ],
      "accounts": [
        {
          "name": "remoteChainConfig",
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
          "name": "committeeVerifierRemoteChainObservation"
        }
      }
    },
    {
      "name": "removeSignerConfig",
      "docs": [
        "Clears the signer configuration for a remote source chain by setting",
        "the threshold to 0 and emptying the signer set.",
        "",
        "Can only be called by the owner."
      ],
      "discriminator": [
        54,
        192,
        103,
        10,
        135,
        1,
        255,
        68
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
                "path": "params.sourceChainSelector"
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
              "name": "removeSignerConfigParams"
            }
          }
        }
      ]
    },
    {
      "name": "resolveAccountsStart",
      "docs": [
        "Entrypoint to resolve accounts for any supported top-level instruction."
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
      "name": "setAllowedFinalityConfig",
      "docs": [
        "Sets the allowed finality config. Can only be called by the owner."
      ],
      "discriminator": [
        218,
        232,
        203,
        238,
        30,
        80,
        111,
        194
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
      "name": "setAllowlist",
      "docs": [
        "Sets the allowlist configuration. Overwrites the entire allowlist",
        "with the provided senders and enabled flag.",
        "",
        "Can only be called by the allowlist admin."
      ],
      "discriminator": [
        141,
        30,
        41,
        131,
        132,
        7,
        216,
        134
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
              "name": "setAllowlistParams"
            }
          }
        }
      ]
    },
    {
      "name": "setAllowlistAdmin",
      "docs": [
        "Sets the allowlist admin. Can only be called by the owner."
      ],
      "discriminator": [
        37,
        8,
        255,
        139,
        56,
        187,
        15,
        179
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
      "name": "setDefaultCodeVersion",
      "docs": [
        "Sets the default code version used for instructions. This may affect",
        "account resolution for instructions that define different contexts on newer",
        "versions."
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
          "name": "eventAuthority"
        },
        {
          "name": "program"
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
      "name": "setFeeAggregator",
      "docs": [
        "Sets the fee aggregator address. Can only be called by the owner."
      ],
      "discriminator": [
        67,
        117,
        70,
        255,
        175,
        129,
        188,
        117
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
      "name": "setSignerConfig",
      "docs": [
        "Sets the signer configuration for a remote source chain.",
        "Overwrites the entire signer set and threshold. Signers are sorted",
        "on-chain for efficient binary search during signature verification.",
        "",
        "Can only be called by the owner."
      ],
      "discriminator": [
        221,
        107,
        160,
        96,
        84,
        59,
        74,
        114
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
                "path": "params.sourceChainSelector"
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
              "name": "setSignerConfigParams"
            }
          }
        }
      ]
    },
    {
      "name": "setStorageLocations",
      "docs": [
        "Sets the storage location identifiers for this CCV.",
        "",
        "Can only be called by the storage_location_admin."
      ],
      "discriminator": [
        83,
        121,
        15,
        61,
        43,
        132,
        33,
        237
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
              "name": "setStorageLocationsParams"
            }
          }
        }
      ]
    },
    {
      "name": "startVerificationAtSource",
      "docs": [
        "Source-side verification hook. Called on the onramp side when a message",
        "is being sent, giving the verifier an opportunity to inspect or",
        "transform the message and collect fees.",
        "",
        "Returns opaque verifier data that will be included in the cross-chain",
        "message metadata."
      ],
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
      "name": "transferStorageLocationAdmin",
      "docs": [
        "Proposes a new storage location admin. The proposed admin must call",
        "`accept_storage_location_admin` to complete the transfer."
      ],
      "discriminator": [
        236,
        150,
        171,
        119,
        248,
        195,
        122,
        131
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
      "name": "verify",
      "docs": [
        "Verification of a cross-chain message. The verifier can implement this",
        "in any way it wants: signature verification, a quorum of signatures,",
        "native interop, or a ZK light client. Any proof required for",
        "verification is supplied through the `verifier_results` field in params.",
        "",
        "The `message_id` is a 32-byte hash of the entire message. It can be",
        "recomputed from the encoded message at the cost of a not-insignificant",
        "amount of compute. Any CCV MUST include the message ID or the entire",
        "message as part of its proof."
      ],
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
      "name": "withdrawFeeTokens",
      "docs": [
        "Sweeps this verifier's accrued fees for one fee token into the fee aggregator's ATA.",
        "",
        "Permissionless: the destination is pinned to `config.fee_aggregator`."
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
      "name": "defaultCodeVersionSet",
      "discriminator": [
        249,
        170,
        220,
        119,
        72,
        194,
        216,
        116
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
      "name": "signerConfigSet",
      "discriminator": [
        14,
        241,
        113,
        74,
        71,
        199,
        131,
        45
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
      "name": "invalidCodeVersion",
      "msg": "Invalid code version"
    },
    {
      "code": 13007,
      "name": "defaultStorageLocationAdminProposal",
      "msg": "Proposed storage location admin cannot be the default pubkey"
    },
    {
      "code": 13008,
      "name": "redundantStorageLocationAdminProposal",
      "msg": "Proposed storage location admin is the current storage location admin"
    },
    {
      "code": 13009,
      "name": "invalidAllowedSenderPda",
      "msg": "Invalid allowed sender PDA"
    },
    {
      "code": 13010,
      "name": "invalidSignerThreshold",
      "msg": "Threshold must be > 0 and <= number of signers"
    },
    {
      "code": 13011,
      "name": "zeroAddressSigner",
      "msg": "Signer cannot be the zero address"
    },
    {
      "code": 13012,
      "name": "duplicateSigner",
      "msg": "Duplicate signer address"
    },
    {
      "code": 13013,
      "name": "sourceNotConfigured",
      "msg": "Source chain is not configured"
    },
    {
      "code": 13014,
      "name": "invalidVersionTag",
      "msg": "Invalid verifier results version tag"
    },
    {
      "code": 13015,
      "name": "malformedVerifierResults",
      "msg": "Malformed verifier results"
    },
    {
      "code": 13016,
      "name": "insufficientSignatures",
      "msg": "Not enough signatures to meet threshold"
    },
    {
      "code": 13017,
      "name": "invalidSignature",
      "msg": "Invalid signature"
    },
    {
      "code": 13018,
      "name": "unauthorizedSigner",
      "msg": "Unauthorized signer"
    },
    {
      "code": 13019,
      "name": "signaturesNotSorted",
      "msg": "Signatures not in ascending signer order"
    },
    {
      "code": 13020,
      "name": "senderNotAllowed",
      "msg": "Sender not allowed by the destination chain allowlist"
    },
    {
      "code": 13021,
      "name": "zeroRouterSigner",
      "msg": "Router signer cannot be the default (zero-value) pubkey"
    },
    {
      "code": 13022,
      "name": "invalidFinality",
      "msg": "Invalid Finality"
    },
    {
      "code": 13023,
      "name": "invalidAccountForMigration",
      "msg": "Account to migrate contains invalid data"
    },
    {
      "code": 13024,
      "name": "feeAggregatorNotSet",
      "msg": "Fee aggregator not set"
    },
    {
      "code": 13025,
      "name": "insufficientFunds",
      "msg": "Insufficient funds (vault empty)"
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
      "name": "committeeVerifierConfigObservation",
      "docs": [
        "Global configuration of this CCV, as observed by off-chain consumers.",
        "",
        "Storage locations are not included here: they are unbounded in size and already exposed by",
        "`get_storage_locations`."
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
          }
        ]
      }
    },
    {
      "name": "committeeVerifierRemoteChainObservation",
      "docs": [
        "Per-source-chain configuration of this CCV, as observed by off-chain consumers.",
        "",
        "The allow list itself is reported as a count only, as its size is unbounded by configuration."
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
          },
          {
            "name": "allowlistEnabled",
            "type": "bool"
          },
          {
            "name": "allowedSendersCount",
            "type": "u32"
          },
          {
            "name": "signerThreshold",
            "type": {
              "option": "u8"
            }
          },
          {
            "name": "signers",
            "type": {
              "vec": {
                "array": [
                  "u8",
                  20
                ]
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
      "name": "defaultCodeVersionSet",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "old",
            "type": {
              "defined": {
                "name": "codeVersion"
              }
            }
          },
          {
            "name": "new",
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
      "name": "getFeeResult",
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
      "name": "remoteChainAdminConfig",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "routerSigner",
            "docs": [
              "PDA of the router allowed to sign the start of a verification process."
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
            "name": "signerConfig",
            "type": {
              "defined": {
                "name": "remoteChainSignerConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "remoteChainSignerConfig",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "threshold",
            "type": "u8"
          },
          {
            "name": "signers",
            "type": {
              "vec": {
                "array": [
                  "u8",
                  20
                ]
              }
            }
          }
        ]
      }
    },
    {
      "name": "removeSignerConfigParams",
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
      "name": "setAllowlistParams",
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
      "name": "setChainConfigParams",
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
      "name": "setSignerConfigParams",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "sourceChainSelector",
            "type": "u64"
          },
          {
            "name": "threshold",
            "type": "u8"
          },
          {
            "name": "signers",
            "type": {
              "vec": {
                "array": [
                  "u8",
                  20
                ]
              }
            }
          }
        ]
      }
    },
    {
      "name": "setStorageLocationsParams",
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
      "name": "signerConfigSet",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "sourceChainSelector",
            "type": "u64"
          },
          {
            "name": "signers",
            "type": {
              "vec": {
                "array": [
                  "u8",
                  20
                ]
              }
            }
          },
          {
            "name": "threshold",
            "type": "u8"
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
    }
  ]
};
