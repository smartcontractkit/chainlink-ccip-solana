package changesets

import (
	"fmt"

	"github.com/Masterminds/semver/v3"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	feequoterops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
	rmnremoteops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/rmnremote"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
)

// legacyProgramVersion is the version the in-place upgraded programs are registered under before
// 2.0.
var legacyProgramVersion = semver.MustParse("1.6.0")

// inPlaceUpgradedPrograms are the programs that move to 2.0 by upgrading their 1.6 deployment
// rather than deploying a new one, so they keep their address.
var inPlaceUpgradedPrograms = []struct {
	contractType datastore.ContractType
	version      *semver.Version
}{
	{datastore.ContractType(routerops.ContractType), routerops.Version},
	{datastore.ContractType(feequoterops.ContractType), feequoterops.Version},
	{datastore.ContractType(rmnremoteops.ContractType), rmnremoteops.Version},
}

// upgradedProgramRefs returns the 2.0.0 refs missing on chainSelector for the programs upgraded in
// place, each copied from the program's 1.6.0 ref. A program that already has a 2.0.0 ref at the
// same address is skipped; one registered at a different address is an error, since the in-place
// upgrade must not leave two addresses for the same program.
func upgradedProgramRefs(refs []datastore.AddressRef, chainSelector uint64) ([]datastore.AddressRef, error) {
	find := func(contractType datastore.ContractType, version *semver.Version) (datastore.AddressRef, bool) {
		for _, ref := range refs {
			if ref.ChainSelector == chainSelector && ref.Type == contractType && ref.Qualifier == "" &&
				ref.Version != nil && ref.Version.Equal(version) {
				return ref, true
			}
		}
		return datastore.AddressRef{}, false
	}

	var toAdd []datastore.AddressRef
	for _, program := range inPlaceUpgradedPrograms {
		legacy, ok := find(program.contractType, legacyProgramVersion)
		if !ok {
			return nil, fmt.Errorf("no %s %s ref on chain %d to upgrade in place", program.contractType, legacyProgramVersion, chainSelector)
		}
		if current, ok := find(program.contractType, program.version); ok {
			if current.Address != legacy.Address {
				return nil, fmt.Errorf("%s %s is registered at %s on chain %d, but its %s ref is at %s",
					program.contractType, program.version, current.Address, chainSelector, legacyProgramVersion, legacy.Address)
			}
			continue
		}
		toAdd = append(toAdd, datastore.AddressRef{
			Address:       legacy.Address,
			ChainSelector: chainSelector,
			Type:          program.contractType,
			Version:       program.version,
			Labels:        legacy.Labels,
		})
	}

	// CCTP pool refs are qualified by pool program, whereas legacy deployments may have a
	// token-qualified 1.x ref. Preserve the address, but register the 2.0 ref under the qualifier
	// expected by UpgradeProgram and the v2 lane configuration sequence.
	var legacyCctp []datastore.AddressRef
	for _, ref := range refs {
		if ref.ChainSelector == chainSelector && ref.Type == datastore.ContractType(common_utils.CCTPTokenPool) &&
			ref.Version != nil && ref.Version.Major() == 1 {
			legacyCctp = append(legacyCctp, ref)
		}
	}
	if len(legacyCctp) > 1 {
		return nil, fmt.Errorf("multiple 1.x CCTPTokenPool refs on chain %d; expected one", chainSelector)
	}
	if len(legacyCctp) == 1 {
		var currentCctp []datastore.AddressRef
		for _, ref := range refs {
			if ref.ChainSelector == chainSelector && ref.Type == datastore.ContractType(common_utils.CCTPTokenPool) &&
				ref.Version != nil && ref.Version.Equal(solcommon.Version2_0_0) && ref.Qualifier == solcommon.PoolProgramQualifier {
				currentCctp = append(currentCctp, ref)
			}
		}
		if len(currentCctp) > 1 {
			return nil, fmt.Errorf("multiple %s CCTPTokenPool refs on chain %d", solcommon.Version2_0_0, chainSelector)
		}
		if len(currentCctp) == 1 {
			if currentCctp[0].Address != legacyCctp[0].Address {
				return nil, fmt.Errorf("CCTPTokenPool %s is registered at %s on chain %d, but its 1.x ref is at %s",
					solcommon.Version2_0_0, currentCctp[0].Address, chainSelector, legacyCctp[0].Address)
			}
		} else {
			toAdd = append(toAdd, datastore.AddressRef{
				Address:       legacyCctp[0].Address,
				ChainSelector: chainSelector,
				Type:          datastore.ContractType(common_utils.CCTPTokenPool),
				Version:       solcommon.Version2_0_0,
				Qualifier:     solcommon.PoolProgramQualifier,
				Labels:        legacyCctp[0].Labels,
			})
		}
	}
	return toAdd, nil
}
