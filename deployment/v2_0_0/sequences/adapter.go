package sequences

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"strconv"

	chainsel "github.com/smartcontractkit/chain-selectors"
	deployapi "github.com/smartcontractkit/chainlink-ccip/deployment/deploy"
	"github.com/smartcontractkit/chainlink-ccip/deployment/finality"
	"github.com/smartcontractkit/chainlink-ccip/deployment/lanes"
	tokensapi "github.com/smartcontractkit/chainlink-ccip/deployment/tokens"
	mcmsreaderapi "github.com/smartcontractkit/chainlink-ccip/deployment/utils/changesets"
	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	ccvadapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/executor"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/offramp"
	rmnremoteops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/rmnremote"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	tokensops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokens"
)

var (
	_ ccvadapters.ChainFamily            = (*SolanaAdapter)(nil)
	_ tokensapi.TokenAdapter             = (*SolanaAdapter)(nil)
	_ mcmsreaderapi.MCMSReader           = (*SolanaAdapter)(nil)
	_ deployapi.TransferOwnershipAdapter = (*SolanaAdapter)(nil)
)

// Minimum unique NOPs per committee and executor pool entry for a chain in a production environment.
const (
	minMainnetChainNOPs = 16
	// Prod testnet runs 16 verifiers and 4 executors. ValidateNOPsTopology cannot tell a committee
	// entry from an executor pool entry, so this minimum applies to both.
	minTestnetChainNOPs = 4
)

type SolanaAdapter struct{}

func (a *SolanaAdapter) GetOnRampAddress(ds datastore.DataStore, chainSelector uint64) ([]byte, error) {
	return a.GetRouterAddress(ds, chainSelector)
}

func (a *SolanaAdapter) GetOffRampAddress(ds datastore.DataStore, chainSelector uint64) ([]byte, error) {
	addr, err := solcommon.FindAndFormatCompatibleRef(ds, datastore.AddressRef{
		ChainSelector: chainSelector,
		Type:          datastore.ContractType(offramp.ContractType),
		Version:       offramp.Version,
	}, chainSelector, solcommon.ToByteArray)
	if err != nil {
		return nil, err
	}
	return addr, nil
}

func (a *SolanaAdapter) GetFQAddress(ds datastore.DataStore, chainSelector uint64) ([]byte, error) {
	addr, err := solcommon.FindAndFormatCompatibleRef(ds, datastore.AddressRef{
		ChainSelector: chainSelector,
		Type:          datastore.ContractType(feequoter.ContractType),
		Version:       feequoter.Version,
	}, chainSelector, solcommon.ToByteArray)
	if err != nil {
		return nil, err
	}
	return addr, nil
}

func (a *SolanaAdapter) GetRouterAddress(ds datastore.DataStore, chainSelector uint64) ([]byte, error) {
	addr, err := solcommon.FindAndFormatCompatibleRef(ds, datastore.AddressRef{
		ChainSelector: chainSelector,
		Type:          datastore.ContractType(router.ContractType),
		Version:       router.Version,
	}, chainSelector, solcommon.ToByteArray)
	if err != nil {
		return nil, err
	}
	return addr, nil
}

func (a *SolanaAdapter) GetRMNRemoteAddress(ds datastore.DataStore, chainSelector uint64) ([]byte, error) {
	addr, err := solcommon.FindAndFormatCompatibleRef(ds, datastore.AddressRef{
		ChainSelector: chainSelector,
		Type:          datastore.ContractType(rmnremoteops.ContractType),
		Version:       rmnremoteops.Version,
	}, chainSelector, solcommon.ToByteArray)
	if err != nil {
		return nil, err
	}
	return addr, nil
}

func (a *SolanaAdapter) GetTestRouter(ds datastore.DataStore, chainSelector uint64) ([]byte, error) {
	return a.GetRouterAddress(ds, chainSelector)
}

func (a *SolanaAdapter) ResolveExecutor(ds datastore.DataStore, chainSelector uint64, qualifier string) (string, error) {
	toAddress := func(ref datastore.AddressRef) (string, error) { return ref.Address, nil }
	addr, err := solcommon.FindAndFormatCompatibleRef(ds, datastore.AddressRef{
		Type:      datastore.ContractType(executor.ContractType),
		Qualifier: qualifier,
		Version:   executor.Version,
	}, chainSelector, toAddress)
	if err != nil {
		return "", fmt.Errorf("failed to resolve executor (qualifier %q) on chain %d: %w", qualifier, chainSelector, err)
	}
	return addr, nil
}

func (a *SolanaAdapter) ConfigureChainForLanes() *cldf_ops.Sequence[ccvadapters.ConfigureChainForLanesInput, seq_core.OnChainOutput, cldf_chain.BlockChains] {
	return ConfigureChainForLanesSequence
}

func (a *SolanaAdapter) GetAddressBytesLength() uint8 {
	return 32
}

func (a *SolanaAdapter) GetChainFamilySelector() [4]byte {
	return solcommon.SVMChainFamilySelector
}

// GetDefaultTokenPrices implements the optional TokenPriceProvider interface.
// Test-environment default: $20 per LINK, as 18-decimals USD per 1e18 smallest
// units (the Solana mint has 9 decimals, so $/whole-token scales by 1e9).
func (a *SolanaAdapter) GetDefaultTokenPrices() map[datastore.ContractType]*big.Int {
	return map[datastore.ContractType]*big.Int{
		datastore.ContractType(tokensops.LinkContractType): new(big.Int).Mul(big.NewInt(20e9), big.NewInt(1e18)),
	}
}

// GetDefaultFeeQuoterDestChainConfig returns the default FeeQuoter dest chain config for Solana.
// reference of the config value from https://github.com/smartcontractkit/chainlink-ccip/blob/3963310d2112b7873f92310807685c11feda8b81/chains/solana/deployment/v1_6_0/sequences/adapter.go#L96
func (a *SolanaAdapter) GetDefaultFeeQuoterDestChainConfig(_, _ uint64, chainFamilySelector [4]byte) ccvadapters.FeeQuoterDestChainConfigOverrides {
	return ccvadapters.FeeQuoterDestChainConfigOverrides{
		IsEnabled: new(true),
		// TODO(NONEVM-2544): with the custom heap allocator the offramp is no longer heap-bound, but
		// 30_000 is still unverified end-to-end (compute budget is the practical ceiling). Verify
		// this value is what we want and reconcile it with GetMaxDataBytes (devenv/messaging.go).
		MaxDataBytes:                new(uint32(30_000)),
		MaxPerMsgGasLimit:           new(uint32(3_000_000)),
		DestGasOverhead:             new(uint32(300_000)),
		DestGasPerPayloadByteBase:   new(uint8(16)),
		ChainFamilySelector:         chainFamilySelector,
		DefaultTokenFeeUSDCents:     new(uint16(25)),
		DefaultTokenDestGasOverhead: new(uint32(90_000)),
		DefaultTxGasLimit:           new(uint32(200_000)),
		NetworkFeeUSDCents:          new(uint16(10)),
		LinkFeeMultiplierPercent:    new(uint8(90)),
		USDPerUnitGas:               big.NewInt(1e6),
	}
}

// This needs to be deprecated. use GetDefaultFeeQuoterDestChainConfig instead
func (a *SolanaAdapter) GetFeeQuoterDestChainConfigForLanes() lanes.FeeQuoterDestChainConfig {
	return lanes.FeeQuoterDestChainConfig{
		IsEnabled:                   true,
		MaxDataBytes:                30_000,
		MaxPerMsgGasLimit:           3_000_000,
		DestGasOverhead:             300_000,
		DestGasPerPayloadByteBase:   16,
		ChainFamilySelector:         binary.BigEndian.Uint32(solcommon.SVMChainFamilySelector[:]),
		DefaultTokenFeeUSDCents:     25,
		DefaultTokenDestGasOverhead: 90_000,
		DefaultTxGasLimit:           200_000,
		NetworkFeeUSDCents:          10,
		V2Params: &lanes.FeeQuoterV2Params{
			LinkFeeMultiplierPercent: 90,
			USDPerUnitGas:            big.NewInt(1e6),
		},
	}
}

// DefaultMaxFeePerMessageUSDCents caps the total fee of a single message on every lane this
// package wires (the per-lane equivalent of EVM's global maxFeeJuelsPerMsg). The upstream
// RemoteChainConfig has no field for it yet, so it can't be overridden per lane and
// ConfigureChainForLanesSequence applies this value to all of them; a lane left at zero would
// reject every send. $10.00 is well above any fee the 2.0 lanes currently quote.
const DefaultMaxFeePerMessageUSDCents = uint16(1000)

func (a *SolanaAdapter) GetDefaultRemoteChainConfig(_, _ uint64) ccvadapters.RemoteChainDefaults {
	return ccvadapters.RemoteChainDefaults{
		AllowTrafficFrom:          true,
		ExecutorDestChainConfig:   ccvadapters.ExecutorDestChainConfig{Enabled: true},
		BaseExecutionGasCost:      150_000,
		TokenReceiverAllowed:      true,
		MessageNetworkFeeUSDCents: 10,
		TokenNetworkFeeUSDCents:   25,
	}
}

func (a *SolanaAdapter) GetDefaultCommitteeVerifierRemoteChainConfig() ccvadapters.CommitteeVerifierRemoteChainDefaults {
	return ccvadapters.CommitteeVerifierRemoteChainDefaults{
		AllowlistEnabled:   false,
		FeeUSDCents:        0,
		GasForVerification: 500_000,
		PayloadSizeBytes:   390,
	}
}

func (a *SolanaAdapter) GetDefaultFinalityConfig() finality.Config {
	return finality.Config{
		WaitForFinality: true,
		WaitForSafe:     true,
		BlockDepth:      1,
	}
}

func (a *SolanaAdapter) ValidateNOPsTopology(chainSelector string, nopCount int) error {
	selector, err := strconv.ParseUint(chainSelector, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chain selector %q: %w", chainSelector, err)
	}
	isTestnet, err := chainsel.IsTestnetChain(selector)
	if err != nil {
		return fmt.Errorf("chain %q: %w", chainSelector, err)
	}
	minNOPs := minMainnetChainNOPs
	if isTestnet {
		minNOPs = minTestnetChainNOPs
	}
	if nopCount < minNOPs {
		return fmt.Errorf(
			"chain %q requires at least %d unique NOPs for production environments, got %d",
			chainSelector,
			minNOPs,
			nopCount,
		)
	}
	return nil
}
