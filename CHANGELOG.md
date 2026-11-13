# Changelog

## [0.3.0](https://github.com/smartcontractkit/chainlink-ccip-solana/compare/v0.2.0...v0.3.0) (2026-11-13)


### Features

* **contracts:** fee-quoter: Add Canton support ([#14](https://github.com/smartcontractkit/chainlink-ccip-solana/issues/14)) ([1619109](https://github.com/smartcontractkit/chainlink-ccip-solana/commit/1619109179ce2b5077c5876b61511e5aa21a38cd))
* **executor:** report Solana transmitter balance as node_balance ([#7](https://github.com/smartcontractkit/chainlink-ccip-solana/issues/7)) ([9b73245](https://github.com/smartcontractkit/chainlink-ccip-solana/commit/9b732457053b9c7b8b8e62595e62bc23c8a5a2fb))

## 0.2.0 (2026-11-10)


### Features

* emit Solana EncodedLogCollector metrics
* Instrument executor GetCCVSForMessage call missing metrics


### Bug Fixes

* **deployment:** Improve datastore management in cctp
* **transmitter:** remove the buffered exec simulation

## 0.1.0 (2026-11-03)


### ⚠ BREAKING CHANGES

* **deployment:** return MCMS batch ops from committee verifier adapter

### Features

* **deployment:** return MCMS batch ops from committee verifier adapter


### Bug Fixes

* Add changesets for parallel lanes
