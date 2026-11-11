/**
 * Program IDL in camelCase format in order to be used in JS/TS.
 *
 * Note that this is only a type helper and is not the actual IDL. The original
 * IDL can be found at `target/idl/mock_lombard_mailbox.json`.
 */
export type MockLombardMailbox = {
  "address": "Lomsq4ZNkZQGysC1pQc2NqNiAbXQm6C8nLmKcUoESEA",
  "metadata": {
    "name": "mockLombardMailbox",
    "version": "2.0.1",
    "spec": "0.1.0",
    "description": "Deliberately minimal mock of the real Lombard `mailbox` program's on-chain interface, used only by the devenv end-to-end test to exercise lombard-token-pool's bridge/mailbox CPIs. Never deployed for real."
  },
  "instructions": [
    {
      "name": "handleMessage",
      "discriminator": [
        91,
        215,
        39,
        71,
        108,
        217,
        94,
        89
      ],
      "accounts": [
        {
          "name": "handler",
          "docs": [
            "Checked against `message_info.destination_caller`, like the real mailbox program."
          ],
          "writable": true,
          "signer": true
        },
        {
          "name": "config"
        },
        {
          "name": "messageInfo",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  109,
                  101,
                  115,
                  115,
                  97,
                  103,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "payloadHash"
              }
            ]
          }
        },
        {
          "name": "recipientProgram"
        }
      ],
      "args": [
        {
          "name": "payloadHash",
          "type": {
            "array": [
              "u8",
              32
            ]
          }
        }
      ],
      "returns": {
        "option": "bytes"
      }
    },
    {
      "name": "markDelivered",
      "docs": [
        "Test-only:. Records a `DeliveredMessage` that `handle_message` will later consume."
      ],
      "discriminator": [
        240,
        118,
        188,
        142,
        64,
        85,
        107,
        18
      ],
      "accounts": [
        {
          "name": "payer",
          "writable": true,
          "signer": true
        },
        {
          "name": "messageInfo",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  109,
                  101,
                  115,
                  115,
                  97,
                  103,
                  101
                ]
              },
              {
                "kind": "arg",
                "path": "payloadHash"
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
          "name": "payloadHash",
          "type": {
            "array": [
              "u8",
              32
            ]
          }
        },
        {
          "name": "recipient",
          "type": "pubkey"
        },
        {
          "name": "amount",
          "type": "u64"
        },
        {
          "name": "message",
          "type": {
            "option": {
              "array": [
                "u8",
                36
              ]
            }
          }
        },
        {
          "name": "destinationCaller",
          "type": {
            "option": {
              "array": [
                "u8",
                32
              ]
            }
          }
        }
      ]
    },
    {
      "name": "testSetupConfig",
      "docs": [
        "Single condensed test-setup call."
      ],
      "discriminator": [
        26,
        136,
        184,
        200,
        32,
        202,
        159,
        241
      ],
      "accounts": [
        {
          "name": "payer",
          "writable": true,
          "signer": true
        },
        {
          "name": "config",
          "writable": true,
          "pda": {
            "seeds": [
              {
                "kind": "const",
                "value": [
                  109,
                  97,
                  105,
                  108,
                  98,
                  111,
                  120,
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
          "name": "globalNonce",
          "type": "u64"
        },
        {
          "name": "treasury",
          "type": "pubkey"
        }
      ]
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
      "name": "deliveredMessage",
      "discriminator": [
        206,
        39,
        186,
        14,
        206,
        80,
        140,
        39
      ]
    }
  ],
  "errors": [
    {
      "code": 6000,
      "name": "alreadyHandled",
      "msg": "message has already been handled"
    },
    {
      "code": 6001,
      "name": "missingRemainingAccounts",
      "msg": "not enough remaining_accounts to forward to mock-lombard-bridge's mock_gmp_receive"
    },
    {
      "code": 6002,
      "name": "invalidDestinationCaller",
      "msg": "handler does not match the message's destination caller"
    }
  ],
  "types": [
    {
      "name": "config",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "admin",
            "type": "pubkey"
          },
          {
            "name": "pendingAdmin",
            "type": "pubkey"
          },
          {
            "name": "treasury",
            "type": "pubkey"
          },
          {
            "name": "paused",
            "type": "bool"
          },
          {
            "name": "consortium",
            "type": "pubkey"
          },
          {
            "name": "globalNonce",
            "type": "u64"
          },
          {
            "name": "defaultMaxPayloadSize",
            "type": "u32"
          },
          {
            "name": "feePerByte",
            "type": "u64"
          }
        ]
      }
    },
    {
      "name": "deliveredMessage",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "status",
            "docs": [
              "0 = delivered (by `mark_delivered`, not yet handled), 1 = handled (by `handle_message`).",
              "Mock-internal encoding, not derived from the real mailbox."
            ],
            "type": "u8"
          },
          {
            "name": "recipient",
            "type": "pubkey"
          },
          {
            "name": "amount",
            "type": "u64"
          },
          {
            "name": "message",
            "type": {
              "option": {
                "array": [
                  "u8",
                  36
                ]
              }
            }
          },
          {
            "name": "destinationCaller",
            "docs": [
              "Mirrors the real mailbox's `MessageV1.destination_caller`: when set, only this handler may",
              "consume the message."
            ],
            "type": {
              "option": {
                "array": [
                  "u8",
                  32
                ]
              }
            }
          }
        ]
      }
    }
  ]
};
