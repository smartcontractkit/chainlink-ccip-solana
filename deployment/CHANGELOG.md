# Changelog

## [0.1.0](https://github.com/smartcontractkit/chainlink-ccip-solana/compare/deployment/v0.0.1...deployment/v0.1.0) (2026-11-27)


### Features

* **deployment:** Add changeset to migrate token pool to v2 ([#32](https://github.com/smartcontractkit/chainlink-ccip-solana/issues/32)) ([02de195](https://github.com/smartcontractkit/chainlink-ccip-solana/commit/02de195edeff325dfdce9aebf1ad7800b5500107))
* **deployment:** add fee quoter DestChainConfigUpdate changeset ([#30](https://github.com/smartcontractkit/chainlink-ccip-solana/issues/30)) ([9ee654d](https://github.com/smartcontractkit/chainlink-ccip-solana/commit/9ee654db2472c00e67b67742c2c8ec8fb779a3a4))
* **deployment:** changeset for setting dest chain config ([#19](https://github.com/smartcontractkit/chainlink-ccip-solana/issues/19)) ([041edb5](https://github.com/smartcontractkit/chainlink-ccip-solana/commit/041edb550ea519836bc004286bdcf3773196a788))
* **deployment:** implement CommitteeVerifierChainDiscovery for Solana ([#36](https://github.com/smartcontractkit/chainlink-ccip-solana/issues/36)) ([3e5a26d](https://github.com/smartcontractkit/chainlink-ccip-solana/commit/3e5a26d322bdbbb0d3d329749278af8207ba43f1))
* **deployment:** support a remote (KMS) Solana signer ([#16](https://github.com/smartcontractkit/chainlink-ccip-solana/issues/16)) ([e9329c3](https://github.com/smartcontractkit/chainlink-ccip-solana/commit/e9329c33a0fea19bc7ba4529a457056d5c0bff78))
* **rpc:** wire up rpc history monitor ([#18](https://github.com/smartcontractkit/chainlink-ccip-solana/issues/18)) ([3f5d182](https://github.com/smartcontractkit/chainlink-ccip-solana/commit/3f5d182fe619087bdebaf5f3d388c4248dea9f14))


### Bug Fixes

* **deployment:** cap default Solana dest maxDataBytes at 5000 ([#33](https://github.com/smartcontractkit/chainlink-ccip-solana/issues/33)) ([3157cab](https://github.com/smartcontractkit/chainlink-ccip-solana/commit/3157cab47e6a9b6c5b05e4e59ab13eb01a27de77))
* **deployment:** Fix changesets for legacy sender/receiver ([#24](https://github.com/smartcontractkit/chainlink-ccip-solana/issues/24)) ([ff03b9b](https://github.com/smartcontractkit/chainlink-ccip-solana/commit/ff03b9bd0dd139a4cee6dde1885bb38feefe35e5))
* **deployment:** Implement fixes ([#4](https://github.com/smartcontractkit/chainlink-ccip-solana/issues/4)) ([9abdfda](https://github.com/smartcontractkit/chainlink-ccip-solana/commit/9abdfda015315ef456aaeea8b3c8a8bd4cdbb6f0))
* **deployment:** lombard adapter ref resolution ([#25](https://github.com/smartcontractkit/chainlink-ccip-solana/issues/25)) ([0808a4e](https://github.com/smartcontractkit/chainlink-ccip-solana/commit/0808a4e363c9fae31b76e48277446995512821f1))
* **deployment:** match the min nops value to EVM ([#22](https://github.com/smartcontractkit/chainlink-ccip-solana/issues/22)) ([548309e](https://github.com/smartcontractkit/chainlink-ccip-solana/commit/548309e5a8f6e8356adde980cfa798576b5bfd68))
* **deployment:** record the contracts mount path in verified builds ([#23](https://github.com/smartcontractkit/chainlink-ccip-solana/issues/23)) ([e155a38](https://github.com/smartcontractkit/chainlink-ccip-solana/commit/e155a387ab595836c1faca6290962c4738f7f294))
* **deployment:** resolve CommitteeVerifier refs across 2.x upgrades ([#34](https://github.com/smartcontractkit/chainlink-ccip-solana/issues/34)) ([ac3f725](https://github.com/smartcontractkit/chainlink-ccip-solana/commit/ac3f72570d5cd46827ad31c4514ff5bac7f87ada))
* **deployment:** verify build ([#27](https://github.com/smartcontractkit/chainlink-ccip-solana/issues/27)) ([bc75b34](https://github.com/smartcontractkit/chainlink-ccip-solana/commit/bc75b34705d3d2944fa7c2b9437201b254bfb947))
* flaky rpc that caused BufferAlreadyContainsChunk error ([#29](https://github.com/smartcontractkit/chainlink-ccip-solana/issues/29)) ([4c2b0b4](https://github.com/smartcontractkit/chainlink-ccip-solana/commit/4c2b0b4f22b2006ba0262e81b1f9b93b210f144e))
* retrieve Burn/Mint multisig mint authority from resolver ([#8](https://github.com/smartcontractkit/chainlink-ccip-solana/issues/8)) ([cee4159](https://github.com/smartcontractkit/chainlink-ccip-solana/commit/cee41599ca64625b3164c42d5442f12419cd8e4b))
