package adapters

import (
	"fmt"

	"github.com/Masterminds/semver/v3"

	dsutil "github.com/smartcontractkit/chainlink-ccip/deployment/utils/datastore"
	ccvadapters "github.com/smartcontractkit/chainlink-ccv/deployment/adapters"
	"github.com/smartcontractkit/chainlink-ccv/executor"
	"github.com/smartcontractkit/chainlink-ccv/pkg/chainaccess"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	executorops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/executor"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/offramp"
)

const (
	rmnRemoteContractType = "RMNRemote"
	rmnRemoteVersion      = "2.0.0"
)

var rmnRemoteSemver = semver.MustParse(rmnRemoteVersion)

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
	executorAddr, err := dsutil.FindAndFormatRef(ds, datastore.AddressRef{
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
	offRampAddr, err := dsutil.FindAndFormatRef(ds, datastore.AddressRef{
		Type:    datastore.ContractType(offramp.ContractType),
		Version: offramp.Version,
	}, chainSelector, solcommon.RefTo0xHex)
	if err != nil {
		return executor.ChainConfiguration{}, fmt.Errorf("off ramp address for chain %d: %w", chainSelector, err)
	}

	rmnRemoteAddr, err := dsutil.FindAndFormatRef(ds, datastore.AddressRef{
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
