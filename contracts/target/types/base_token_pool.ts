/**
 * Program IDL in camelCase format in order to be used in JS/TS.
 *
 * Note that this is only a type helper and is not the actual IDL. The original
 * IDL can be found at `target/idl/base_token_pool.json`.
 */
export type BaseTokenPool = {
  "address": "DtHUwbNFN8RSeEBje4eH5gpqmheA9S86KaSWoqpQKzZu",
  "metadata": {
    "name": "baseTokenPool",
    "version": "2.0.1",
    "spec": "0.1.0",
    "description": "Created with Anchor"
  },
  "instructions": [],
  "events": [
    {
      "name": "burned",
      "discriminator": [
        207,
        37,
        251,
        154,
        239,
        229,
        14,
        67
      ]
    },
    {
      "name": "configChanged",
      "discriminator": [
        147,
        25,
        86,
        98,
        98,
        77,
        78,
        192
      ]
    },
    {
      "name": "configV2Updated",
      "discriminator": [
        0,
        31,
        42,
        25,
        64,
        180,
        246,
        88
      ]
    },
    {
      "name": "globalConfigUpdated",
      "discriminator": [
        232,
        238,
        158,
        123,
        210,
        172,
        159,
        46
      ]
    },
    {
      "name": "liquidityDeposited",
      "discriminator": [
        218,
        155,
        74,
        193,
        59,
        66,
        94,
        122
      ]
    },
    {
      "name": "liquidityWithdrawn",
      "discriminator": [
        240,
        120,
        73,
        139,
        154,
        31,
        218,
        68
      ]
    },
    {
      "name": "locked",
      "discriminator": [
        188,
        53,
        118,
        62,
        64,
        12,
        198,
        84
      ]
    },
    {
      "name": "lockedOrBurned",
      "discriminator": [
        97,
        87,
        156,
        77,
        163,
        187,
        250,
        39
      ]
    },
    {
      "name": "mintAuthorityTransferred",
      "discriminator": [
        23,
        136,
        155,
        223,
        27,
        166,
        51,
        85
      ]
    },
    {
      "name": "minted",
      "discriminator": [
        174,
        131,
        21,
        57,
        88,
        117,
        114,
        121
      ]
    },
    {
      "name": "overrideChainConfigConfigured",
      "discriminator": [
        14,
        63,
        56,
        14,
        131,
        43,
        6,
        200
      ]
    },
    {
      "name": "overrideChainConfigRemoved",
      "discriminator": [
        56,
        162,
        57,
        203,
        13,
        110,
        47,
        191
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
      "name": "rateLimitAdminSet",
      "discriminator": [
        253,
        130,
        227,
        78,
        124,
        47,
        48,
        49
      ]
    },
    {
      "name": "rateLimitConfigured",
      "discriminator": [
        249,
        210,
        194,
        93,
        236,
        75,
        175,
        59
      ]
    },
    {
      "name": "rebalancerSet",
      "discriminator": [
        254,
        55,
        202,
        188,
        218,
        141,
        220,
        59
      ]
    },
    {
      "name": "released",
      "discriminator": [
        232,
        229,
        255,
        136,
        101,
        189,
        15,
        220
      ]
    },
    {
      "name": "releasedOrMinted",
      "discriminator": [
        36,
        47,
        25,
        10,
        194,
        190,
        111,
        54
      ]
    },
    {
      "name": "remoteChainConfigured",
      "discriminator": [
        231,
        252,
        78,
        228,
        152,
        49,
        233,
        226
      ]
    },
    {
      "name": "remoteChainRemoved",
      "discriminator": [
        4,
        212,
        235,
        138,
        165,
        232,
        75,
        32
      ]
    },
    {
      "name": "remotePoolsAppended",
      "discriminator": [
        248,
        177,
        249,
        167,
        14,
        247,
        25,
        223
      ]
    },
    {
      "name": "rmnRemoteUpdated",
      "discriminator": [
        89,
        28,
        232,
        1,
        170,
        12,
        114,
        141
      ]
    },
    {
      "name": "routerUpdated",
      "discriminator": [
        230,
        116,
        235,
        209,
        74,
        144,
        208,
        95
      ]
    },
    {
      "name": "tokenPoolInitialized",
      "discriminator": [
        209,
        45,
        174,
        162,
        235,
        91,
        252,
        32
      ]
    },
    {
      "name": "tokenTransferFeeConfigRemoved",
      "discriminator": [
        208,
        204,
        189,
        64,
        216,
        40,
        133,
        52
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
      "name": "tokensConsumed",
      "discriminator": [
        126,
        8,
        242,
        245,
        121,
        78,
        210,
        0
      ]
    }
  ],
  "errors": [
    {
      "code": 6000,
      "name": "invalidInitPoolPermissions",
      "msg": "Pool authority does not match token mint owner"
    },
    {
      "code": 6001,
      "name": "invalidRmnRemoteAddress",
      "msg": "Invalid RMN Remote Address"
    },
    {
      "code": 6002,
      "name": "unauthorized",
      "msg": "Unauthorized"
    },
    {
      "code": 6003,
      "name": "invalidInputs",
      "msg": "Invalid inputs"
    },
    {
      "code": 6004,
      "name": "invalidVersion",
      "msg": "Invalid state version"
    },
    {
      "code": 6005,
      "name": "invalidPoolCaller",
      "msg": "Caller is not ramp on router"
    },
    {
      "code": 6006,
      "name": "invalidSender",
      "msg": "Sender not allowed"
    },
    {
      "code": 6007,
      "name": "invalidSourcePoolAddress",
      "msg": "Invalid source pool address"
    },
    {
      "code": 6008,
      "name": "invalidToken",
      "msg": "Invalid token"
    },
    {
      "code": 6009,
      "name": "invalidTokenAmountConversion",
      "msg": "Invalid token amount conversion"
    },
    {
      "code": 6010,
      "name": "allowlistKeyAlreadyExisted",
      "msg": "Key already existed in the allowlist"
    },
    {
      "code": 6011,
      "name": "allowlistKeyDidNotExist",
      "msg": "Key did not exist in the allowlist"
    },
    {
      "code": 6012,
      "name": "remotePoolAddressAlreadyExisted",
      "msg": "Remote pool address already exists"
    },
    {
      "code": 6013,
      "name": "nonemptyPoolAddressesInit",
      "msg": "Expected empty pool addresses during initialization"
    },
    {
      "code": 6014,
      "name": "invalidDerivationStage",
      "msg": "Unexpected account derivation stage"
    },
    {
      "code": 6015,
      "name": "rlBucketOverfilled",
      "msg": "RateLimit: bucket overfilled"
    },
    {
      "code": 6016,
      "name": "rlMaxCapacityExceeded",
      "msg": "RateLimit: max capacity exceeded"
    },
    {
      "code": 6017,
      "name": "rlRateLimitReached",
      "msg": "RateLimit: rate limit reached"
    },
    {
      "code": 6018,
      "name": "rlInvalidRateLimitRate",
      "msg": "RateLimit: invalid rate limit rate"
    },
    {
      "code": 6019,
      "name": "rlDisabledNonZeroRateLimit",
      "msg": "RateLimit: disabled non-zero rate limit"
    },
    {
      "code": 6020,
      "name": "liquidityNotAccepted",
      "msg": "Liquidity not accepted"
    },
    {
      "code": 6021,
      "name": "transferZeroTokensNotAllowed",
      "msg": "Transferring zero tokens is not allowed"
    },
    {
      "code": 6022,
      "name": "insufficientPoolBalance",
      "msg": "Insufficient pool balance"
    },
    {
      "code": 6023,
      "name": "duplicateCcvNotAllowed",
      "msg": "Invalid duplicated ccvs"
    },
    {
      "code": 6024,
      "name": "mustSpecifyUnderThresholdCcVsForThresholdCcVs",
      "msg": "Invalid Threshold CCVs config"
    },
    {
      "code": 6025,
      "name": "inboundOutboundDirectionOnly",
      "msg": "Invalid Direction for Get Required CCVs"
    },
    {
      "code": 6026,
      "name": "ccvZeroAddressNotAllowed",
      "msg": "CCV address cannot be the zero/default pubkey"
    },
    {
      "code": 6027,
      "name": "thresholdAmountRequiredForThresholdCcvs",
      "msg": "Threshold amount must be non-zero when threshold CCVs are configured"
    },
    {
      "code": 6028,
      "name": "invalidRequestedFinality",
      "msg": "Requested finality is not allowed by the pool's configured allowed finality"
    },
    {
      "code": 6029,
      "name": "zeroTokenAddress",
      "msg": "Remote token address cannot be zero/empty"
    },
    {
      "code": 6030,
      "name": "invalidTokenTransferFeeBps",
      "msg": "Token transfer fee bps must not exceed 10000 (100%)"
    },
    {
      "code": 6031,
      "name": "invalidTokenTransferFeeDestBytesOverhead",
      "msg": "Token transfer fee destination bytes overhead is below the required minimum"
    },
    {
      "code": 6032,
      "name": "rlArithmeticError",
      "msg": "RateLimit: arithmetic error"
    },
    {
      "code": 6033,
      "name": "noFeesToWithdraw",
      "msg": "No fees available to withdraw (vault empty)"
    },
    {
      "code": 6034,
      "name": "invalidFeeWithdrawAmount",
      "msg": "Invalid requested amount of fees to withdraw"
    },
    {
      "code": 6035,
      "name": "insufficientFeesToWithdraw",
      "msg": "Not enough stored fees to withdraw the requested amount"
    },
    {
      "code": 6036,
      "name": "invalidSourcePoolData",
      "msg": "Encoded source pool data is invalid"
    },
    {
      "code": 6037,
      "name": "configAndRemotePoolDisagreeOnDecimals",
      "msg": "Remote chain config and remote pool show different decimals for token"
    },
    {
      "code": 6038,
      "name": "missingFeeVaultTokenAccount",
      "msg": "The fee vault token account for the pool's own mint does not exist"
    },
    {
      "code": 6039,
      "name": "duplicatedKeys",
      "msg": "A key appears more than once"
    }
  ],
  "types": [
    {
      "name": "burned",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "sender",
            "type": "pubkey"
          },
          {
            "name": "amount",
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
      "name": "ccvConfig",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "thresholdAmount",
            "type": "u64"
          },
          {
            "name": "outboundCcvs",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "thresholdOutboundCcvs",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "inboundCcvs",
            "type": {
              "vec": "pubkey"
            }
          },
          {
            "name": "thresholdInboundCcvs",
            "type": {
              "vec": "pubkey"
            }
          }
        ]
      }
    },
    {
      "name": "configChanged",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "config",
            "type": {
              "defined": {
                "name": "rateLimitConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "configV2Updated",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "ccv",
            "type": {
              "defined": {
                "name": "ccvConfig"
              }
            }
          },
          {
            "name": "allowedFinality",
            "type": {
              "defined": {
                "name": "finalityConfig"
              }
            }
          },
          {
            "name": "tokenTransferFee",
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
      "name": "crossChainGas",
      "type": {
        "kind": "struct",
        "fields": [
          "u32"
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
      "name": "globalConfigUpdated",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "selfServedAllowed",
            "type": "bool"
          },
          {
            "name": "router",
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
      "name": "liquidityDeposited",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "provider",
            "type": "pubkey"
          },
          {
            "name": "amount",
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
      "name": "liquidityWithdrawn",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "provider",
            "type": "pubkey"
          },
          {
            "name": "amount",
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
      "name": "locked",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "sender",
            "type": "pubkey"
          },
          {
            "name": "amount",
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
      "name": "lockedOrBurned",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "sender",
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
      "name": "mintAuthorityTransferred",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "oldMintAuthority",
            "type": "pubkey"
          },
          {
            "name": "newMintAuthority",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "minted",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "sender",
            "type": "pubkey"
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
            "name": "mint",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "overrideChainConfigConfigured",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "chainSelector",
            "type": "u64"
          },
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "overrideType",
            "type": {
              "defined": {
                "name": "poolChainConfigOverrideType"
              }
            }
          },
          {
            "name": "outboundRateLimit",
            "type": {
              "defined": {
                "name": "rateLimitConfig"
              }
            }
          },
          {
            "name": "inboundRateLimit",
            "type": {
              "defined": {
                "name": "rateLimitConfig"
              }
            }
          }
        ]
      }
    },
    {
      "name": "overrideChainConfigRemoved",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "chainSelector",
            "type": "u64"
          },
          {
            "name": "overrideType",
            "type": {
              "defined": {
                "name": "poolChainConfigOverrideType"
              }
            }
          },
          {
            "name": "mint",
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
          },
          {
            "name": "mint",
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
          },
          {
            "name": "mint",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "poolChainConfigOverrideType",
      "type": {
        "kind": "enum",
        "variants": [
          {
            "name": "ftf"
          }
        ]
      }
    },
    {
      "name": "rateLimitAdminSet",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "oldRateLimitAdmin",
            "type": "pubkey"
          },
          {
            "name": "newRateLimitAdmin",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "rateLimitConfig",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "enabled",
            "type": "bool"
          },
          {
            "name": "capacity",
            "type": "u64"
          },
          {
            "name": "rate",
            "type": "u64"
          }
        ]
      }
    },
    {
      "name": "rateLimitConfigured",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "chainSelector",
            "type": "u64"
          },
          {
            "name": "outboundRateLimit",
            "type": {
              "defined": {
                "name": "rateLimitConfig"
              }
            }
          },
          {
            "name": "inboundRateLimit",
            "type": {
              "defined": {
                "name": "rateLimitConfig"
              }
            }
          },
          {
            "name": "mint",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "rebalancerSet",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "oldRebalancer",
            "type": "pubkey"
          },
          {
            "name": "newRebalancer",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "released",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "sender",
            "type": "pubkey"
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
            "name": "mint",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "releasedOrMinted",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "remoteChainSelector",
            "type": "u64"
          },
          {
            "name": "sender",
            "type": "pubkey"
          },
          {
            "name": "recipient",
            "type": "pubkey"
          },
          {
            "name": "userTokenAccount",
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
      "name": "remoteAddress",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "address",
            "type": "bytes"
          }
        ]
      }
    },
    {
      "name": "remoteChainConfigured",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "chainSelector",
            "type": "u64"
          },
          {
            "name": "token",
            "type": {
              "defined": {
                "name": "remoteAddress"
              }
            }
          },
          {
            "name": "previousToken",
            "type": {
              "defined": {
                "name": "remoteAddress"
              }
            }
          },
          {
            "name": "poolAddresses",
            "type": {
              "vec": {
                "defined": {
                  "name": "remoteAddress"
                }
              }
            }
          },
          {
            "name": "previousPoolAddresses",
            "type": {
              "vec": {
                "defined": {
                  "name": "remoteAddress"
                }
              }
            }
          },
          {
            "name": "mint",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "remoteChainRemoved",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "chainSelector",
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
      "name": "remotePoolsAppended",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "chainSelector",
            "type": "u64"
          },
          {
            "name": "poolAddresses",
            "type": {
              "vec": {
                "defined": {
                  "name": "remoteAddress"
                }
              }
            }
          },
          {
            "name": "previousPoolAddresses",
            "type": {
              "vec": {
                "defined": {
                  "name": "remoteAddress"
                }
              }
            }
          },
          {
            "name": "mint",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "rmnRemoteUpdated",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "oldRmnRemote",
            "type": "pubkey"
          },
          {
            "name": "newRmnRemote",
            "type": "pubkey"
          },
          {
            "name": "mint",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "routerUpdated",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "oldRouter",
            "type": "pubkey"
          },
          {
            "name": "newRouter",
            "type": "pubkey"
          },
          {
            "name": "mint",
            "type": "pubkey"
          }
        ]
      }
    },
    {
      "name": "tokenPoolInitialized",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "tokenProgram",
            "type": "pubkey"
          },
          {
            "name": "owner",
            "type": "pubkey"
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
            "name": "finalityFee",
            "type": {
              "defined": {
                "name": "usdCents"
              }
            }
          },
          {
            "name": "fastFinalityFee",
            "type": {
              "defined": {
                "name": "usdCents"
              }
            }
          },
          {
            "name": "finalityBpsFee",
            "type": "u16"
          },
          {
            "name": "fastFinalityBpsFee",
            "type": "u16"
          },
          {
            "name": "isEnabled",
            "type": "bool"
          }
        ]
      }
    },
    {
      "name": "tokenTransferFeeConfigRemoved",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "chainSelector",
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
      "name": "tokenTransferFeeConfigUpdated",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "chainSelector",
            "type": "u64"
          },
          {
            "name": "mint",
            "type": "pubkey"
          },
          {
            "name": "config",
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
      "name": "tokensConsumed",
      "type": {
        "kind": "struct",
        "fields": [
          {
            "name": "tokens",
            "type": "u64"
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
    }
  ]
};
