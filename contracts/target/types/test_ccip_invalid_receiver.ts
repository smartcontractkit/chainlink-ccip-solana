/**
 * Program IDL in camelCase format in order to be used in JS/TS.
 *
 * Note that this is only a type helper and is not the actual IDL. The original
 * IDL can be found at `target/idl/test_ccip_invalid_receiver.json`.
 */
export type TestCcipInvalidReceiver = {
  "address": "MhPhxwRHtZt9rkHEEFJ9eFjEna8fwbMHr8KYByvJkUm",
  "metadata": {
    "name": "testCcipInvalidReceiver",
    "version": "2.0.2",
    "spec": "0.1.0",
    "description": "Created with Anchor"
  },
  "instructions": [
    {
      "name": "addOfframp",
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
      "name": "ccipReceive",
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
          "writable": true,
          "signer": true
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
                "path": "message.sourceChainSelector"
              },
              {
                "kind": "account",
                "path": "offrampProgram"
              }
            ],
            "program": {
              "kind": "const",
              "value": [
                172,
                151,
                41,
                119,
                225,
                186,
                181,
                255,
                246,
                198,
                134,
                41,
                31,
                153,
                79,
                245,
                47,
                52,
                171,
                237,
                27,
                199,
                223,
                136,
                247,
                111,
                67,
                249,
                38,
                89,
                44,
                143
              ]
            }
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
    }
  ],
  "types": [
    {
      "name": "allowedOfframp",
      "type": {
        "kind": "struct",
        "fields": []
      }
    },
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
      "name": "counter",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "value",
            "type": "u8"
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
