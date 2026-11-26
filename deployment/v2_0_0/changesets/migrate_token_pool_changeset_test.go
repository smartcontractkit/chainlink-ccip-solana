package changesets

import (
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"
	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	mcms_types "github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

// The checks below run before the datastore and the chain are read; the datastore resolution is
// covered by sequences.TestResolveTokenPoolMigration, and the on-chain checks need a validator.
func TestMigrateTokenPoolToV2ValidateInput(t *testing.T) {
	sel := chainsel.SOLANA_DEVNET.Selector
	remote := chainsel.ETHEREUM_TESTNET_SEPOLIA.Selector
	env := cldf.Environment{
		BlockChains: cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{
			sel: cldf_solana.Chain{Selector: sel},
		}),
		DataStore: datastore.NewMemoryDataStore().Seal(),
	}
	mint := solana.NewWallet().PublicKey().String()
	mcmsInput := mcms.Input{
		TimelockAction: mcms_types.TimelockActionSchedule,
		ValidUntil:     uint32(time.Now().Add(time.Hour).Unix()), //nolint:gosec // fits in uint32 until 2106
	}
	valid := func() MigrateTokenPoolToV2ChangesetInput {
		return MigrateTokenPoolToV2ChangesetInput{
			ChainSelector:        sel,
			PoolType:             common_utils.BurnMintTokenPool.String(),
			TokenMint:            mint,
			RemoteChainSelectors: []uint64{remote},
			MCMS:                 mcmsInput,
		}
	}
	with := func(edit func(*MigrateTokenPoolToV2ChangesetInput)) MigrateTokenPoolToV2ChangesetInput {
		in := valid()
		edit(&in)
		return in
	}

	_, err := validateMigrateTokenPoolInput(env, valid())
	require.NoError(t, err)
	_, err = validateMigrateTokenPoolInput(env, with(func(in *MigrateTokenPoolToV2ChangesetInput) { in.MCMS = mcms.Input{} }))
	require.NoError(t, err, "MCMS is optional")
	_, err = validateMigrateTokenPoolInput(env, with(func(in *MigrateTokenPoolToV2ChangesetInput) {
		in.PoolType = common_utils.LockReleaseTokenPool.String()
	}))
	require.NoError(t, err)

	cases := []struct {
		name string
		edit func(*MigrateTokenPoolToV2ChangesetInput)
		err  string
	}{
		{"unknown chain", func(in *MigrateTokenPoolToV2ChangesetInput) { in.ChainSelector = chainsel.SOLANA_MAINNET.Selector }, "does not exist"},
		{"cctp pool", func(in *MigrateTokenPoolToV2ChangesetInput) { in.PoolType = common_utils.CCTPTokenPool.String() }, "MigrateCctpTokenPoolToV2Changeset"},
		{"unknown pool type", func(in *MigrateTokenPoolToV2ChangesetInput) { in.PoolType = "Nope" }, "not supported"},
		{"bad mint", func(in *MigrateTokenPoolToV2ChangesetInput) { in.TokenMint = "not-a-key" }, "invalid token mint"},
		{"zero mint", func(in *MigrateTokenPoolToV2ChangesetInput) { in.TokenMint = solana.PublicKey{}.String() }, "invalid token mint"},
		{"no remotes", func(in *MigrateTokenPoolToV2ChangesetInput) { in.RemoteChainSelectors = nil }, "at least one remote chain selector"},
		{"self as remote", func(in *MigrateTokenPoolToV2ChangesetInput) { in.RemoteChainSelectors = []uint64{sel} }, "is the chain itself"},
		{"duplicate remote", func(in *MigrateTokenPoolToV2ChangesetInput) { in.RemoteChainSelectors = []uint64{remote, remote} }, "duplicate remote chain selector"},
		{"invalid remote", func(in *MigrateTokenPoolToV2ChangesetInput) { in.RemoteChainSelectors = []uint64{12345} }, "invalid remote chain selector"},
		{"bad mcms", func(in *MigrateTokenPoolToV2ChangesetInput) { in.MCMS = mcms.Input{TimelockAction: "nope"} }, "failed to validate MCMS input"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateMigrateTokenPoolInput(env, with(tc.edit))
			require.ErrorContains(t, err, tc.err)
		})
	}
}

func TestMigrateTokenPoolToV2VerifyDatastore(t *testing.T) {
	sel := chainsel.SOLANA_DEVNET.Selector
	remote := chainsel.ETHEREUM_TESTNET_SEPOLIA.Selector
	program := solana.NewWallet().PublicKey().String()
	mint := solana.NewWallet().PublicKey().String()
	bnm := datastore.ContractType(common_utils.BurnMintTokenPool)
	build := func(refs ...datastore.AddressRef) datastore.DataStore {
		ds := datastore.NewMemoryDataStore()
		for _, r := range refs {
			require.NoError(t, ds.Addresses().Add(r))
		}
		return ds.Seal()
	}
	blockChains := cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{sel: cldf_solana.Chain{Selector: sel}})
	input := MigrateTokenPoolToV2ChangesetInput{
		ChainSelector:        sel,
		PoolType:             common_utils.BurnMintTokenPool.String(),
		TokenMint:            mint,
		RemoteChainSelectors: []uint64{remote},
	}
	token := datastore.AddressRef{ChainSelector: sel, Type: datastore.ContractType(solcommon.SPL2022Tokens), Version: semver.MustParse("1.6.0"), Qualifier: "CCIP-BnM", Address: mint}
	instance := datastore.AddressRef{ChainSelector: sel, Type: bnm, Version: semver.MustParse("1.6.0"), Qualifier: "CLLCCIP", Address: program}
	upgradedProgram := datastore.AddressRef{ChainSelector: sel, Type: bnm, Version: semver.MustParse("2.0.0"), Qualifier: solcommon.PoolProgramQualifier, Address: program}

	env := cldf.Environment{BlockChains: blockChains, DataStore: build(token, instance)}
	require.ErrorContains(t, migrateTokenPoolToV2Verify(env, input), "has no 2.x ref", "program not upgraded")

	env = cldf.Environment{BlockChains: blockChains, DataStore: build(token, instance, upgradedProgram)}
	require.ErrorContains(t, migrateTokenPoolToV2Verify(env, input), "resolve 2.0 Router", "Router not upgraded")

	env = cldf.Environment{BlockChains: blockChains, DataStore: build(instance, upgradedProgram)}
	require.ErrorContains(t, migrateTokenPoolToV2Verify(env, input), "no token ref")
}
