/**
 * Program IDL in camelCase format in order to be used in JS/TS.
 *
 * Note that this is only a type helper and is not the actual IDL. The original
 * IDL can be found at `target/idl/example_ccip_sender.json`.
 */
export type ExampleCcipSender = {
  "address": "E67f8sC5GbpeKCm3595t7sYVzd2k5frcqZAZZ1v4QECo",
  "metadata": {
    "name": "exampleCcipSender",
    "version": "2.0.1",
    "spec": "0.1.0",
    "description": "Created with Anchor"
  },
  "docs": [
    "This program an example of a CCIP Sender Program.",
    "Used to test CCIP Router ccip_send."
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
                "path": "destChainSelector"
              }
            ]
          }
        },
        {
          "name": "ccipSender",
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
          "name": "authorityFeeTokenAta",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "account",
                "path": "authority"
              },
              {
                "kind": "account",
                "path": "ccipFeeTokenProgram"
              },
              {
                "kind": "account",
                "path": "ccipFeeTokenMint"
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
          "name": "systemProgram",
          "address": "11111111111111111111111111111111"
        },
        {
          "name": "ccipRouter"
        },
        {
          "name": "ccipConfig"
        },
        {
          "name": "ccipDestChainState",
          "writable": true
        },
        {
          "name": "ccipSenderNonce",
          "writable": true
        },
        {
          "name": "ccipFeeTokenProgram"
        },
        {
          "name": "ccipFeeTokenMint"
        },
        {
          "name": "ccipFeeTokenUserAta",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "account",
                "path": "ccipSender"
              },
              {
                "kind": "account",
                "path": "ccipFeeTokenProgram"
              },
              {
                "kind": "account",
                "path": "ccipFeeTokenMint"
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
          "name": "ccipFeeTokenReceiver",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "account",
                "path": "ccipFeeBillingSigner"
              },
              {
                "kind": "account",
                "path": "ccipFeeTokenProgram"
              },
              {
                "kind": "account",
                "path": "ccipFeeTokenMint"
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
          "name": "ccipFeeBillingSigner"
        },
        {
          "name": "ccipFeeQuoter"
        },
        {
          "name": "ccipFeeQuoterConfig"
        },
        {
          "name": "ccipFeeQuoterDestChain"
        },
        {
          "name": "ccipFeeQuoterBillingTokenConfig"
        },
        {
          "name": "ccipFeeQuoterLinkTokenConfig"
        },
        {
          "name": "ccipRmnRemote"
        },
        {
          "name": "ccipRmnRemoteCurses"
        },
        {
          "name": "ccipRmnRemoteConfig"
        }
      ],
      "args": [
        {
          "name": "destChainSelector",
          "type": "u64"
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
          "name": "data",
          "type": "bytes"
        },
        {
          "name": "feeToken",
          "type": "pubkey"
        },
        {
          "name": "tokenIndexes",
          "type": "bytes"
        }
      ]
    },
    {
      "name": "initChainConfig",
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
          "name": "recipient",
          "type": "bytes"
        },
        {
          "name": "extraArgsBytes",
          "type": "bytes"
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
          "address": "E67f8sC5GbpeKCm3595t7sYVzd2k5frcqZAZZ1v4QECo"
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
          "name": "recipient",
          "type": "bytes"
        },
        {
          "name": "extraArgsBytes",
          "type": "bytes"
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
          "name": "amount",
          "type": "u64"
        },
        {
          "name": "decimals",
          "type": "u8"
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
      "name": "transferTokenDuplicated",
      "msg": "The same token is being transferred more than once"
    },
    {
      "code": 6001,
      "name": "invalidRouter",
      "msg": "Invalid router address"
    },
    {
      "code": 6002,
      "name": "onlyOwner",
      "msg": "Address is not owner"
    },
    {
      "code": 6003,
      "name": "onlyProposedOwner",
      "msg": "Address is not proposed_owner"
    },
    {
      "code": 6004,
      "name": "invalidProposedOwner",
      "msg": "Proposed owner is invalid"
    },
    {
      "code": 6005,
      "name": "invalidMessageId",
      "msg": "Invalid message ID returned from CPI"
    },
    {
      "code": 6006,
      "name": "missingCpiReturnData",
      "msg": "CPI target did not return data"
    },
    {
      "code": 6007,
      "name": "invalidCpiReturnProgram",
      "msg": "CPI return data came from an unexpected program"
    },
    {
      "code": 6008,
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
      "name": "messageSent",
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
