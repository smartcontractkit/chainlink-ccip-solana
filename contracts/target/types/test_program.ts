/**
 * Program IDL in camelCase format in order to be used in JS/TS.
 *
 * Note that this is only a type helper and is not the actual IDL. The original
 * IDL can be found at `target/idl/test_program.json`.
 */
export type TestProgram = {
  "address": "9yDw4zo6CxspgkUD5mGcs8LQiKQCZhkZCbaMhKEUHboU",
  "metadata": {
    "name": "testProgram",
    "version": "2.0.0",
    "spec": "0.1.0",
    "description": "Created with Anchor"
  },
  "instructions": [
    {
      "name": "emitCcipMessageSentV2",
      "discriminator": [
        229,
        29,
        77,
        118,
        109,
        83,
        19,
        30
      ],
      "accounts": [
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
    }
  ]
};
