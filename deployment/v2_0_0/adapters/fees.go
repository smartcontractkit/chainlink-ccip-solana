package adapters

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-ccip/deployment/fees"
	"github.com/smartcontractkit/chainlink-ccip/deployment/lanes"
	datastore_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils/datastore"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/fee_quoter"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	fqops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
	solseq "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

var _ fees.FeeAdapter = (*FeesAdapter)(nil)

type FeesAdapter struct {
	sol *solseq.SolanaAdapter
}

func NewFeesAdapter(solAdapter *solseq.SolanaAdapter) *FeesAdapter {
	return &FeesAdapter{
		sol: solAdapter,
	}
}

func (a *FeesAdapter) GetFeeContractRef(_ operations.Bundle, _ cldf_chain.BlockChains, ds datastore.DataStore, _ datastore.AddressRef, src uint64, _ uint64) (datastore.AddressRef, error) {
	fqAddr, err := a.sol.GetFQAddress(ds, src)
	if err != nil {
		return datastore.AddressRef{}, fmt.Errorf("failed to get FeeQuoter address for chain selector %d: %w", src, err)
	}

	filter := datastore.AddressRef{
		Type:          datastore.ContractType(fqops.ContractType),
		Address:       solana.PublicKeyFromBytes(fqAddr).String(),
		ChainSelector: src,
	}

	feeContractRef, err := datastore_utils.FindAndFormatRef(
		ds,
		filter,
		src,
		datastore_utils.FullRef,
	)
	if err != nil {
		return datastore.AddressRef{}, fmt.Errorf("failed to find FeeQuoter address ref for chain selector %d: %w", src, err)
	}

	return feeContractRef, nil
}

func (a *FeesAdapter) GetDefaultTokenTransferFeeConfig(src uint64, dst uint64) fees.TokenTransferFeeArgs {
	return fees.GetDefaultChainAgnosticTokenTransferFeeConfig(src, dst)
}

func (a *FeesAdapter) GetOnchainTokenTransferFeeConfig(b operations.Bundle, chains cldf_chain.BlockChains, fq datastore.AddressRef, src uint64, dst uint64, address string) (fees.TokenTransferFeeArgs, error) {
	chain, ok := chains.SolanaChains()[src]
	if !ok {
		return fees.TokenTransferFeeArgs{}, fmt.Errorf("solana chain not found for selector %d", src)
	}

	fqAddr := solana.MustPublicKeyFromBase58(fq.Address)

	token, err := solana.PublicKeyFromBase58(address)
	if err != nil {
		return fees.TokenTransferFeeArgs{}, fmt.Errorf("invalid base58 token address: %s", address)
	}

	remoteBillingPDA, _, err := state.FindFqPerChainPerTokenConfigPDA(dst, token, fqAddr)
	if err != nil {
		return fees.TokenTransferFeeArgs{}, fmt.Errorf("failed to find remote billing token config pda (src = %d, dst = %d, token = %s): %w", src, dst, token, err)
	}

	var cfg fee_quoter.PerChainPerTokenConfig
	err = common.GetAccountDataBorshIntoAnchor(b.GetContext(), chain.Client, remoteBillingPDA, cldf_solana.SolDefaultCommitment, &cfg)
	if err != nil && !errors.Is(err, rpc.ErrNotFound) {
		return fees.TokenTransferFeeArgs{}, fmt.Errorf("failed to deserialize PerChainPerTokenConfig (src = %d, dst = %d, token = %s, pda = %s): %w", src, dst, token, remoteBillingPDA, err)
	}

	return fees.TokenTransferFeeArgs{
		DestBytesOverhead: cfg.TokenTransferConfig.DestBytesOverhead,
		DestGasOverhead:   cfg.TokenTransferConfig.DestGasOverhead.V0,
		MinFeeUSDCents:    cfg.TokenTransferConfig.MinFee.V0,
		MaxFeeUSDCents:    cfg.TokenTransferConfig.MaxFee.V0,
		IsEnabled:         cfg.TokenTransferConfig.IsEnabled,
		DeciBps:           cfg.TokenTransferConfig.DeciBps,
	}, nil
}

// GetDefaultDestChainConfig implements fees.FeeAdapter. src/dst are ignored: Solana defaults are
// chain-agnostic here, matching the EVM FeeAdapter pattern.
func (a *FeesAdapter) GetDefaultDestChainConfig(_, _ uint64) lanes.FeeQuoterDestChainConfig {
	return a.sol.GetFeeQuoterDestChainConfigForLanes()
}

func (a *FeesAdapter) GetOnchainDestChainConfig(b operations.Bundle, chains cldf_chain.BlockChains, fq datastore.AddressRef, src uint64, dst uint64) (lanes.FeeQuoterDestChainConfig, error) {
	chain, ok := chains.SolanaChains()[src]
	if !ok {
		return lanes.FeeQuoterDestChainConfig{}, fmt.Errorf("solana chain not found for selector %d", src)
	}

	fqPubkey := solana.MustPublicKeyFromBase58(fq.Address)

	destChainPDA, _, err := state.FindFqDestChainV2PDA(dst, fqPubkey)
	if err != nil {
		return lanes.FeeQuoterDestChainConfig{}, fmt.Errorf("failed to derive DestChainV2 PDA for dst %d on chain %d: %w", dst, src, err)
	}

	var destChainAccount fee_quoter.DestChainV2
	err = common.GetAccountDataBorshIntoAnchor(b.GetContext(), chain.Client, destChainPDA, cldf_solana.SolDefaultCommitment, &destChainAccount)
	if err != nil {
		if errors.Is(err, rpc.ErrNotFound) {
			return lanes.FeeQuoterDestChainConfig{IsEnabled: false}, nil
		}
		return lanes.FeeQuoterDestChainConfig{}, fmt.Errorf("failed to read DestChainV2 account for dst %d on chain %d: %w", dst, src, err)
	}

	cfg, err := solseq.ReverseTranslateFQV2(destChainAccount.Config)
	if err != nil {
		return lanes.FeeQuoterDestChainConfig{}, fmt.Errorf("invalid DestChainV2 config for dst %d on chain %d: %w", dst, src, err)
	}
	return cfg, nil
}

func (a *FeesAdapter) ApplyDestChainConfigUpdates(ds datastore.DataStore, _ datastore.AddressRef) *operations.Sequence[fees.ApplyDestChainConfigSequenceInput, sequences.OnChainOutput, cldf_chain.BlockChains] {
	return operations.NewSequence(
		"ApplyDestChainConfigUpdatesSolana",
		semver.MustParse("2.0.0"),
		"Applies FeeQuoter destination chain config updates on Solana",
		func(b operations.Bundle, chains cldf_chain.BlockChains, input fees.ApplyDestChainConfigSequenceInput) (sequences.OnChainOutput, error) {
			var result sequences.OnChainOutput

			solChain, ok := chains.SolanaChains()[input.Selector]
			if !ok {
				return result, fmt.Errorf("solana chain with selector %d not defined", input.Selector)
			}

			fqAddr, err := a.sol.GetFQAddress(ds, input.Selector)
			if err != nil {
				return result, fmt.Errorf("failed to get FeeQuoter address for chain %d: %w", input.Selector, err)
			}
			fqPubkey := solana.PublicKeyFromBytes(fqAddr)

			offRampAddr, err := a.sol.GetOffRampAddress(ds, input.Selector)
			if err != nil {
				return result, fmt.Errorf("failed to get OffRamp address for chain %d: %w", input.Selector, err)
			}
			offRampPubkey := solana.PublicKeyFromBytes(offRampAddr)

			for dst, cfg := range input.Settings {
				report, err := operations.ExecuteOperation(b, fqops.ConnectChains, solChain, fqops.ConnectChainsParams{
					LocalChainSelector:  solChain.Selector,
					FeeQuoter:           fqPubkey,
					OffRamp:             offRampPubkey,
					RemoteChainSelector: dst,
					DestChainConfig:     solseq.FeeQuoterDestChainConfigV2FromLanes(cfg),
				})
				if err != nil {
					return result, fmt.Errorf("failed to apply dest chain config for dst %d on Solana chain %d: %w", dst, input.Selector, err)
				}
				result.Addresses = append(result.Addresses, report.Output.Addresses...)
				result.BatchOps = append(result.BatchOps, report.Output.BatchOps...)

				// A USDPerUnitGas carried in the config update is not applied here: gas prices
				// are mutable state on the 1.6 dest_chain account (which quote_gas_for_exec_v2
				// reads from until a 2.0 price updater exists), written by the offchain price
				// updater through the offramp's allowed-price-updater role.
			}

			return result, nil
		},
	)
}

func (a *FeesAdapter) SetTokenTransferFee(ds datastore.DataStore, _ datastore.AddressRef) *operations.Sequence[fees.SetTokenTransferFeeSequenceInput, sequences.OnChainOutput, cldf_chain.BlockChains] {
	return operations.NewSequence(
		"SetTokenTransferFee",
		semver.MustParse("2.0.0"),
		"Sets token transfer fee configuration on CCIP 2.0.0 FeeQuoter contracts",
		func(b operations.Bundle, chains cldf_chain.BlockChains, input fees.SetTokenTransferFeeSequenceInput) (sequences.OnChainOutput, error) {
			var result sequences.OnChainOutput
			src := input.Selector

			fqRef, err := a.GetFeeContractRef(b, chains, ds, datastore.AddressRef{}, src, 0)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to get FeeQuoter address for chain selector %d: %w", src, err)
			}

			fqAddr := solana.MustPublicKeyFromBase58(fqRef.Address)

			remoteChainConfigs := map[uint64]map[solana.PublicKey]fee_quoter.TokenTransferFeeConfig{}
			for dst, dstCfg := range input.Settings {
				remoteChainConfigs[dst] = map[solana.PublicKey]fee_quoter.TokenTransferFeeConfig{}
				for rawTokenAddress, feeCfg := range dstCfg {
					token, parseErr := solana.PublicKeyFromBase58(rawTokenAddress)
					if parseErr != nil {
						return sequences.OnChainOutput{}, fmt.Errorf("invalid base58 token address for src %d and dst %d: %s", src, dst, rawTokenAddress)
					}

					if feeCfg == nil {
						defaults := a.GetDefaultTokenTransferFeeConfig(src, dst)
						remoteChainConfigs[dst][token] = fee_quoter.TokenTransferFeeConfig{
							DestBytesOverhead: defaults.DestBytesOverhead,
							DestGasOverhead:   fee_quoter.CrossChainGas{V0: defaults.DestGasOverhead},
							MinFee:            fee_quoter.UsdCents{V0: defaults.MinFeeUSDCents},
							MaxFee:            fee_quoter.UsdCents{V0: defaults.MaxFeeUSDCents},
							IsEnabled:         false,
							DeciBps:           defaults.DeciBps,
						}
					} else {
						remoteChainConfigs[dst][token] = fee_quoter.TokenTransferFeeConfig{
							DestBytesOverhead: feeCfg.DestBytesOverhead,
							DestGasOverhead:   fee_quoter.CrossChainGas{V0: feeCfg.DestGasOverhead},
							MinFee:            fee_quoter.UsdCents{V0: feeCfg.MinFeeUSDCents},
							MaxFee:            fee_quoter.UsdCents{V0: feeCfg.MaxFeeUSDCents},
							IsEnabled:         feeCfg.IsEnabled,
							DeciBps:           feeCfg.DeciBps,
						}
					}
				}
			}

			if len(remoteChainConfigs) == 0 {
				return result, nil
			}

			result, err = sequences.RunAndMergeSequence(b, chains,
				solseq.SetTokenTransferFeeConfig,
				solseq.FeeQuoterSetTokenTransferFeeConfigSequenceInput{
					RemoteChainConfigs: remoteChainConfigs,
					DataStore:          ds,
					FeeQuoter:          fqAddr,
					Selector:           input.Selector,
				},
				result,
			)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to set token transfer fee config on FeeQuoter %s for chain selector %d: %w", fqAddr, src, err)
			}

			return result, nil
		},
	)
}
