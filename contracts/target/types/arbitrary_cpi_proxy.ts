/**
 * Program IDL in camelCase format in order to be used in JS/TS.
 *
 * Note that this is only a type helper and is not the actual IDL. The original
 * IDL can be found at `target/idl/arbitrary_cpi_proxy.json`.
 */
export type ArbitraryCpiProxy = {
  "address": "4tiRp7mqGJwihhW7W32U9jx2h24grdgj67WwutyF2Yg6",
  "metadata": {
    "name": "arbitraryCpiProxy",
    "version": "2.0.1",
    "spec": "0.1.0",
    "description": "Generic CPI proxy for testing. Executes arbitrary CPIs signed with a PDA, enabling tests of programs that authenticate callers via PDA (pools, executor, verifier, etc.)."
  },
  "instructions": [
    {
      "name": "addOfframp",
      "docs": [
        "Register an offramp as allowed for a given source chain. This creates the",
        "`allowed_offramp` PDA that token pools check during `release_or_mint`."
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
      "name": "executeCpi",
      "docs": [
        "Execute an arbitrary CPI, signing with a PDA derived from",
        "`[pda_seed_prefix, target_program_account.key()]`.",
        "",
        "# Arguments",
        "* `ix_data` - Raw instruction data (discriminator + serialized args) to send.",
        "* `pda_seed_prefix` - Seed prefix for PDA derivation. The target program",
        "account key is automatically appended, and the bump is found internally.",
        "",
        "# Accounts",
        "* `target_program_account` - The program to invoke via CPI. Its key is",
        "also used as the second PDA seed component.",
        "* `remaining_accounts` - All accounts to pass to the target program's",
        "instruction. The caller must include the PDA signer at the position",
        "the target program expects."
      ],
      "discriminator": [
        94,
        35,
        166,
        102,
        37,
        19,
        14,
        89
      ],
      "accounts": [
        {
          "name": "targetProgramAccount",
          "docs": [
            "The program to invoke via CPI. Must be present in the transaction's",
            "account list so invoke_signed can resolve it."
          ]
        }
      ],
      "args": [
        {
          "name": "ixData",
          "type": "bytes"
        },
        {
          "name": "pdaSeedPrefix",
          "type": "bytes"
        }
      ],
      "returns": "bytes"
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
    }
  ],
  "errors": [
    {
      "code": 6000,
      "name": "invalidCpiReturnProgram",
      "msg": "Return data came from a different program than the one invoked"
    }
  ],
  "types": [
    {
      "name": "allowedOfframp",
      "type": {
        "kind": "struct",
        "fields": []
      }
    }
  ]
};
