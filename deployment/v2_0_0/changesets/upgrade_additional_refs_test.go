package changesets

import (
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	cctpverifierops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/cctpverifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

// Per-program contents are covered in sequences.TestUpgradeAdditionalRefs; this covers the address lookup.
func TestAdditionalRefsForUpgrade(t *testing.T) {
	selector := chainsel.SOLANA_DEVNET.Selector
	v2 := semver.MustParse("2.0.0")
	address := solana.NewWallet().PublicKey().String()
	cctp := datastore.ContractType(sequences.UpgradeTypeCCTPTokenPool)
	poolRef := func(qualifier string) datastore.AddressRef {
		return datastore.AddressRef{Address: address, ChainSelector: selector, Type: cctp, Version: v2, Qualifier: qualifier}
	}
	input := UpgradeProgramChangesetInput{ChainSelector: selector, ProgramType: sequences.UpgradeTypeCCTPTokenPool, Qualifier: "USDC"}

	t.Run("takes the address from the ref of the instance being upgraded", func(t *testing.T) {
		got := additionalRefsForUpgrade([]datastore.AddressRef{poolRef("USDC")}, input, v2)
		require.Empty(t, got.Conflicting)
		require.ElementsMatch(t, []datastore.AddressRef{
			poolRef(solcommon.PoolProgramQualifier),
			{Address: address, ChainSelector: selector, Type: datastore.ContractType(cctpverifierops.ContractType), Version: v2, Qualifier: string(cctpverifierops.ContractType)},
		}, got.Add)
	})

	t.Run("the alias follows the version of the release being deployed", func(t *testing.T) {
		v201 := semver.MustParse("2.0.1")
		deployed := poolRef("USDC")
		deployed.Version = v201
		got := additionalRefsForUpgrade([]datastore.AddressRef{deployed}, input, v201)
		alias := poolRef(solcommon.PoolProgramQualifier)
		alias.Version = v201
		require.Contains(t, got.Add, alias)
	})

	t.Run("uses the default qualifier when the input has none", func(t *testing.T) {
		in := input
		in.Qualifier = ""
		got := additionalRefsForUpgrade([]datastore.AddressRef{poolRef(solcommon.PoolProgramQualifier)}, in, v2)
		require.Len(t, got.Add, 1, "the alias is the ref itself, so only the verifier is added")
	})

	t.Run("nothing without a ref of the instance at that version", func(t *testing.T) {
		got := additionalRefsForUpgrade([]datastore.AddressRef{poolRef("OTHER")}, input, v2)
		require.Empty(t, got.Add)
		require.Empty(t, got.Conflicting)
	})

	t.Run("nothing for a program type the changeset does not know", func(t *testing.T) {
		in := input
		in.ProgramType = "Unknown"
		got := additionalRefsForUpgrade([]datastore.AddressRef{poolRef("USDC")}, in, v2)
		require.Empty(t, got.Add)
		require.Empty(t, got.Conflicting)
	})
}
