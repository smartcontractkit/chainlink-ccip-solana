package adapters

import (
	"fmt"

	ccvadapters "github.com/smartcontractkit/chainlink-ccv/deployment/adapters"
	"github.com/smartcontractkit/chainlink-ccv/executor"
	"github.com/smartcontractkit/chainlink-ccv/pkg/chainaccess"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	executorops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/executor"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/offramp"
)

const rmnRemoteContractType = "RMNRemote"

var rmnRemoteSemver = solcommon.Version2_0_0

type SolanaExecutorConfigAdapter struct{}

var _ ccvadapters.ExecutorConfigAdapter = (*SolanaExecutorConfigAdapter)(nil)

func (a *SolanaExecutorConfigAdapter) RequiresNodeChainSupportInJD() bool {
	return false
}

func (a *SolanaExecutorConfigAdapter) GetDeployedChains(ds datastore.DataStore, qualifier string) []uint64 {
	return solcommon.GetDeployedChainSelectors(ds,
		datastore.AddressRefByQualifier(qualifier),
		datastore.AddressRefByType(datastore.ContractType(executorops.ContractType)),
	)
}

func (a *SolanaExecutorConfigAdapter) ResolveExecutorAddress(ds datastore.DataStore, chainSelector uint64, qualifier string) (string, error) {
	executorAddr, err := solcommon.FindAndFormatCompatibleRef(ds, datastore.AddressRef{
		Type:      datastore.ContractType(executorops.ContractType),
		Qualifier: qualifier,
		Version:   executorops.Version,
	}, chainSelector, solcommon.RefTo0xHex)
	if err != nil {
		return "", fmt.Errorf("executor address for chain %d: %w", chainSelector, err)
	}
	return executorAddr, nil
}

func (a *SolanaExecutorConfigAdapter) BuildChainConfig(ds datastore.DataStore, chainSelector uint64, qualifier string) (executor.ChainConfiguration, error) {
	offRampAddr, err := solcommon.FindAndFormatCompatibleRef(ds, datastore.AddressRef{
		Type:    datastore.ContractType(offramp.ContractType),
		Version: offramp.Version,
	}, chainSelector, solcommon.RefTo0xHex)
	if err != nil {
		return executor.ChainConfiguration{}, fmt.Errorf("off ramp address for chain %d: %w", chainSelector, err)
	}

	rmnRemoteAddr, err := solcommon.FindAndFormatCompatibleRef(ds, datastore.AddressRef{
		Type:    rmnRemoteContractType,
		Version: rmnRemoteSemver,
	}, chainSelector, solcommon.RefTo0xHex)
	if err != nil {
		return executor.ChainConfiguration{}, fmt.Errorf("rmn remote address for chain %d: %w", chainSelector, err)
	}

	executorAddr, err := a.ResolveExecutorAddress(ds, chainSelector, qualifier)
	if err != nil {
		return executor.ChainConfiguration{}, err
	}

	return executor.ChainConfiguration{
		DestinationChainConfig: chainaccess.DestinationChainConfig{
			OffRampAddress: offRampAddr,
			RmnAddress:     rmnRemoteAddr,
		},
		DefaultExecutorAddress: executorAddr,
	}, nil
}
