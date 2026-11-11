package sequences

import (
	"github.com/Masterminds/semver/v3"

	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

// additionalRef is a ref a fresh deploy saves at the program's address besides its own. An in-place upgrade
// skips the deploy, so it saves these itself. Zero contractType/version default to the program's own.
type additionalRef struct {
	contractType datastore.ContractType
	version      *semver.Version
	qualifier    string
}

// poolProgramAlias is the TokenPoolProgram alias that 2.0 pool flows resolve the shared program by.
// A chain can have several legacy pools: the first one upgraded becomes the alias.
var poolProgramAlias = additionalRef{qualifier: solcommon.PoolProgramQualifier}

// AdditionalRefs are the refs an upgrade should save besides the program's own.
type AdditionalRefs struct {
	Add []datastore.AddressRef
	// Superseded are same-major refs at the same address that an added ref replaces.
	Superseded []datastore.AddressRef
	// Conflicting are existing refs of the same key at another address; they are not replaced.
	Conflicting []datastore.AddressRef
}

// UpgradeAdditionalRefs returns the additional refs of programType at address that are not in refs yet.
// programVersion is the ref version of the upgraded program. An empty address yields nothing.
func UpgradeAdditionalRefs(
	programType ProgramUpgradeType,
	refs []datastore.AddressRef,
	chainSelector uint64,
	programVersion *semver.Version,
	address string,
) AdditionalRefs {
	var result AdditionalRefs
	entry, ok := programRegistry[programType]
	if !ok || address == "" {
		return result
	}
	for _, additional := range entry.additionalRefs {
		contractType := additional.contractType
		if contractType == "" {
			contractType = entry.contractType
		}
		version := additional.version
		if version == nil {
			version = programVersion
		}
		qualifier := additional.qualifier

		var existing *datastore.AddressRef
		var stale []datastore.AddressRef
		for i := range refs {
			r := &refs[i]
			if r.ChainSelector != chainSelector || r.Type != contractType || r.Qualifier != qualifier || r.Version == nil {
				continue
			}
			switch {
			case r.Version.Equal(version):
				existing = r
			case r.Version.Major() == version.Major():
				stale = append(stale, *r)
			}
		}
		for _, old := range stale {
			if old.Address != address {
				existing = &old // reported as conflicting below; the program is not re-registered over it
				continue
			}
			result.Superseded = append(result.Superseded, old)
		}
		switch {
		case existing == nil:
			result.Add = append(result.Add, datastore.AddressRef{
				Address:       address,
				ChainSelector: chainSelector,
				Type:          contractType,
				Version:       version,
				Qualifier:     qualifier,
			})
		case existing.Address != address:
			result.Conflicting = append(result.Conflicting, *existing)
		}
	}
	return result
}
