/**
 * Program IDL in camelCase format in order to be used in JS/TS.
 *
 * Note that this is only a type helper and is not the actual IDL. The original
 * IDL can be found at `target/idl/test_ccip_receiver.json`.
 */
export type TestCcipReceiver = {
  "address": "GsgXbSfG226vCVMfamUhwFgY4kF9HkNiu3wk8Ho8cky5",
  "metadata": {
    "name": "testCcipReceiver",
    "version": "2.0.1",
    "spec": "0.1.0",
    "description": "Created with Anchor"
  },
  "docs": [
    "This program an example of a CCIP Receiver Program.",
    "Used to test CCIP Router execute."
  ],
  "instructions": [
    {
      "name": "ccipReceive",
      "docs": [
        "This function is called by the CCIP Router to execute the CCIP message.",
        "The method name needs to be ccip_receive with Anchor encoding,",
        "if not using Anchor the discriminator needs to be [0x0b, 0xf4, 0x09, 0xf9, 0x2c, 0x53, 0x2f, 0xf5]",
        "You can send as many accounts as you need, specifying if mutable or not.",
        "But none of them could be an init, realloc or close.",
        "In this case, it increments the counter value by 1 and logs the parsed message."
      ],
      "discriminator": [
        11,
        244,
        9,
        249,
        44,
        83,
        47,
        245
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
                  235,
                  219,
                  9,
                  200,
                  84,
                  173,
                  162,
                  137,
                  202,
                  13,
                  77,
                  162,
                  72,
                  73,
                  72,
                  179,
                  156,
                  88,
                  205,
                  1,
                  92,
                  179,
                  101,
                  168,
                  250,
                  125,
                  132,
                  158,
                  36,
                  36,
                  127,
                  232
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
            "and the authority PDA. Must be second."
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
          "name": "externalExecutionConfig",
          "writable": true,
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
              }
            ]
          }
        },
        {
          "name": "counter",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  117,
                  110,
                  116,
                  101,
                  114
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
      "name": "ccipTokenLockBurn",
      "discriminator": [
        200,
        14,
        50,
        9,
        44,
        91,
        121,
        37
      ],
      "accounts": [
        {
          "name": "authority",
          "signer": true
        },
        {
          "name": "poolTokenAccount",
          "writable": true
        },
        {
          "name": "mint"
        },
        {
          "name": "tokenProgram"
        }
      ],
      "args": [
        {
          "name": "input",
          "type": {
            "defined": {
              "name": "lockOrBurnInV1"
            }
          }
        }
      ]
    },
    {
      "name": "ccipTokenReleaseMint",
      "discriminator": [
        20,
        148,
        113,
        198,
        229,
        170,
        71,
        48
      ],
      "accounts": [
        {
          "name": "authority",
          "signer": true
        },
        {
          "name": "poolTokenAccount",
          "writable": true
        },
        {
          "name": "mint"
        },
        {
          "name": "tokenProgram"
        }
      ],
      "args": [
        {
          "name": "input",
          "type": {
            "defined": {
              "name": "releaseOrMintInV1"
            }
          }
        }
      ]
    },
    {
      "name": "initialize",
      "docs": [
        "The initialization is responsibility of the External User, CCIP is not handling initialization of Accounts"
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
          "name": "counter",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  117,
                  110,
                  116,
                  101,
                  114
                ]
              }
            ]
          }
        },
        {
          "name": "externalExecutionConfig",
          "writable": true,
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
          "name": "router",
          "type": "pubkey"
        }
      ]
    },
    {
      "name": "setRejectAll",
      "discriminator": [
        42,
        90,
        30,
        32,
        7,
        99,
        130,
        151
      ],
      "accounts": [
        {
          "name": "counter",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  99,
                  111,
                  117,
                  110,
                  116,
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
          "name": "rejectAll",
          "type": "bool"
        }
      ]
    }
  ],
  "accounts": [
    {
      "name": "counter",
      "discriminator": [
        255,
        176,
        4,
        245,
        188,
        253,
        124,
        25
      ]
    },
    {
      "name": "externalExecutionConfig",
      "discriminator": [
        159,
        157,
        150,
        212,
        168,
        103,
        117,
        39
      ]
    }
  ],
  "errors": [
    {
      "code": 6000,
      "name": "rejectAll",
      "msg": "Rejecting all messages"
    }
  ],
  "types": [
    {
      "name": "any2SvmMessage",
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
            "name": "tokenAmounts",
            "type": {
              "vec": {
                "defined": {
                  "name": "svmTokenAmount"
                }
              }
            }
          }
        ]
      }
    },
    {
      "name": "baseState",
      "type": {
        "kind": "struct",
        "fields": [
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
      "name": "counter",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "value",
            "type": "u64"
          },
          {
            "name": "rejectAll",
            "type": "bool"
          },
          {
            "name": "state",
            "type": {
              "defined": {
                "name": "baseState"
              }
            }
          }
        ]
      }
    },
    {
      "name": "externalExecutionConfig",
      "type": {
        "kind": "struct",
        "fields": []
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
            "type": "bytes"
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
            "type": "bytes"
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
    }
  ]
};
