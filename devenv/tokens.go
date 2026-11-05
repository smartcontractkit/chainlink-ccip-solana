package devenv

import (
	"fmt"
	"maps"
	"math/big"

	"github.com/Masterminds/semver/v3"

	tokenscore "github.com/smartcontractkit/chainlink-ccip/deployment/tokens"
	devenvcommon "github.com/smartcontractkit/chainlink-ccv/build/devenv/common"
	ccvdeployment "github.com/smartcontractkit/chainlink-ccv/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/deployment"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

const (
	// EVMDefaultDecimals is the standard decimal count for devenv EVM test tokens.
	EVMDefaultDecimals = 18
	// SolanaDefaultDecimals is the standard decimal count for SPL test tokens.
	SolanaDefaultDecimals = 6
	// solanaPreMintTokens is the number of whole tokens to pre-mint for each test token.
	solanaPreMintTokens = 1_000
)

// TokenRefForPool maps a token pool address ref to the SPL token address ref
// stored in the datastore. This mirrors TokenRef in buildSolanaTokenTransferConfig.
func TokenRefForPool(poolRef datastore.AddressRef) datastore.AddressRef {
	return datastore.AddressRef{
		Type:      datastore.ContractType(solcommon.SPLTokens),
		Version:   solcommon.Version2_0_0,
		Qualifier: poolRef.Qualifier,
	}
}

// GetSupportedPools returns the pool types Solana can deploy in the devenv.
func (c *Chain) GetSupportedPools() []devenvcommon.PoolCapability {
	return []devenvcommon.PoolCapability{
		{PoolType: devenvcommon.BurnMintTokenPoolType, PoolVersion: semver.MustParse("1.6.1")},
		{PoolType: devenvcommon.LockReleaseTokenPoolType, PoolVersion: semver.MustParse("1.6.1")},
		{PoolType: devenvcommon.BurnMintTokenPoolType, PoolVersion: semver.MustParse("2.0.0")},
		{PoolType: devenvcommon.LockReleaseTokenPoolType, PoolVersion: semver.MustParse("2.0.0")},
	}
}

// GetTokenExpansionConfigs returns one TokenExpansionInputPerChain per unique pool
// that should be deployed on this Solana chain. It follows the same de-dup logic
// as the EVM implementation: iterate combos, de-dup by pool key.
func (c *Chain) GetTokenExpansionConfigs(
	env *deployment.Environment,
	selector uint64,
	combos []devenvcommon.TokenCombination,
) ([]tokenscore.TokenExpansionInputPerChain, error) {
	chain, ok := env.BlockChains.SolanaChains()[selector]
	if !ok {
		return nil, fmt.Errorf("solana chain not found for selector %d", selector)
	}

	preMint := tokenscore.ScaleTokenAmount(big.NewInt(solanaPreMintTokens), SolanaDefaultDecimals).Uint64()

	seen := make(map[string]bool)
	var configs []tokenscore.TokenExpansionInputPerChain

	supported := devenvcommon.BuildSupportedPoolsMap(c.GetSupportedPools())

	for _, combo := range combos {
		for _, poolRef := range []datastore.AddressRef{combo.LocalPoolAddressRef(), combo.RemotePoolAddressRef()} {
			key := devenvcommon.AddressRefFullKey(poolRef)
			if seen[key] {
				continue
			}
			// Only deploy pools that this chain supports.
			if !devenvcommon.IsPoolSupported(supported, poolRef) {
				continue
			}
			seen[key] = true

			configs = append(configs, tokenscore.TokenExpansionInputPerChain{
				TokenPoolVersion: poolRef.Version,
				// TODO: set to false once MCMS/timelock ownership transfer is implemented for Solana devenv.
				SkipOwnershipTransfer: true,
				DeployTokenInput: &tokenscore.DeployTokenInput{
					Symbol:        poolRef.Qualifier,
					Name:          poolRef.Qualifier,
					Decimals:      SolanaDefaultDecimals,
					Type:          solcommon.SPLTokens,
					ExternalAdmin: chain.DeployerKey.PublicKey().String(),
					CCIPAdmin:     chain.DeployerKey.PublicKey().String(),
					PreMint:       &preMint,
					Senders:       []string{chain.DeployerKey.PublicKey().String()},
					// No MCMS here either (same TODO as above), so there is no timelock to hold
					// the freeze authority - revoke it rather than requiring a timelock ref.
					DisableFreezeAuthority: true,
				},
				DeployTokenPoolInput: &tokenscore.DeployTokenPoolInput{
					PoolType:           string(poolRef.Type),
					TokenPoolQualifier: poolRef.Qualifier,
				},
			})
		}
	}

	return configs, nil
}

// GetTokenTransferConfigs builds TokenTransferConfig entries for all pools
// deployed on this chain, using real refs from the datastore.
func (c *Chain) GetTokenTransferConfigs(
	env *deployment.Environment,
	selector uint64,
	remoteSelectors []uint64,
	topology *ccvdeployment.EnvironmentTopology,
) ([]tokenscore.TokenTransferConfig, error) {
	applicableCombos := devenvcommon.FilterTokenCombinations(
		devenvcommon.AllTokenCombinations(), topology, env.DataStore, append([]uint64{selector}, remoteSelectors...),
	)

	hasAddressRef := func(chainSelector uint64, ref datastore.AddressRef) bool {
		_, err := env.DataStore.Addresses().Get(datastore.NewAddressRefKey(
			chainSelector,
			ref.Type,
			ref.Version,
			ref.Qualifier,
		))
		return err == nil
	}

	merged := make(map[string]tokenscore.TokenTransferConfig)
	for _, combo := range applicableCombos {
		eligibleRemoteSelectors := make([]uint64, 0, len(remoteSelectors))
		for _, rs := range remoteSelectors {
			if hasAddressRef(rs, combo.RemotePoolAddressRef()) {
				eligibleRemoteSelectors = append(eligibleRemoteSelectors, rs)
			}
		}
		if len(eligibleRemoteSelectors) == 0 {
			continue
		}

		cfg := buildSolanaTokenTransferConfig(selector, eligibleRemoteSelectors, combo.LocalPoolAddressRef(), combo.RemotePoolAddressRef())
		key := devenvcommon.AddressRefFullKey(cfg.TokenPoolRef)
		if existing, ok := merged[key]; ok {
			maps.Copy(existing.RemoteChains, cfg.RemoteChains)
			merged[key] = existing
		} else {
			merged[key] = cfg
		}
	}

	configs := make([]tokenscore.TokenTransferConfig, 0, len(merged))
	for _, cfg := range merged {
		configs = append(configs, cfg)
	}
	return configs, nil
}

func buildSolanaTokenTransferConfig(
	selector uint64,
	remoteSelectors []uint64,
	localRef datastore.AddressRef,
	remoteRef datastore.AddressRef,
) tokenscore.TokenTransferConfig {
	remoteChains := make(map[uint64]tokenscore.RemoteChainConfig[*datastore.AddressRef, datastore.AddressRef])
	for _, rs := range remoteSelectors {
		ref := remoteRef // copy before taking address
		remoteChains[rs] = tokenscore.RemoteChainConfig[*datastore.AddressRef, datastore.AddressRef]{
			RemotePool: &ref,
		}
	}
	return tokenscore.TokenTransferConfig{
		ChainSelector: selector,
		TokenPoolRef:  localRef,
		TokenRef:      TokenRefForPool(localRef),
		// RegistryRef left empty — Solana's ConfigureTokenForTransfersSequence does not use it.
		RemoteChains: remoteChains,
	}
}
