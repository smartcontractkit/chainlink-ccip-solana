package deployment

import (
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"
	mcms_types "github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/burnmint_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/lockrelease_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	deploysolcommon "github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/changesets"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
)

// selfServedPool is one token pool program under test.
type selfServedPool struct {
	name       string
	program    solana.PublicKey
	poolType   datastore.ContractType
	initialize *operations.Operation[tokenpoolops.Params, tokenpoolops.PoolInitializeOut, cldf_solana.Chain]
	// getSelfServeAllowed reads SelfServedAllowed from the pool's global config.
	getSelfServeAllowed func(program solana.PublicKey) (bool, error)
}

// TestUpdateSelfServedAllowed deploys a BurnMint and a LockRelease token pool to a local validator and runs
// UpdateSelfServedAllowedChangeset against each one. The flag lives in each program's global config, so every
// toggle must change only the pool it targets.
func TestUpdateSelfServedAllowed(t *testing.T) {
	ctx := t.Context()
	admin := testutils.Admin
	chain, sent := newLocalChain(t)
	client := chain.Client
	sel := chain.Selector

	readBurnMint := func(program solana.PublicKey) (bool, error) {
		pda, _, err := state.FindConfigPDA(program)
		require.NoError(t, err)
		var cfg burnmint_token_pool.PoolConfig
		err = common.GetAccountDataBorshIntoAnchor(ctx, client, pda, rpc.CommitmentConfirmed, &cfg)
		return cfg.SelfServedAllowed, err
	}
	readLockRelease := func(program solana.PublicKey) (bool, error) {
		pda, _, err := state.FindConfigPDA(program)
		require.NoError(t, err)
		var cfg lockrelease_token_pool.PoolConfig
		err = common.GetAccountDataBorshIntoAnchor(ctx, client, pda, rpc.CommitmentConfirmed, &cfg)
		return cfg.SelfServedAllowed, err
	}

	pools := []selfServedPool{
		{
			name:                "burnmint",
			program:             burnmint_token_pool.ProgramID,
			poolType:            datastore.ContractType(common_utils.BurnMintTokenPool),
			initialize:          tokenpoolops.InitializeBurnMint,
			getSelfServeAllowed: readBurnMint,
		},
		{
			name:                "lockrelease",
			program:             lockrelease_token_pool.ProgramID,
			poolType:            datastore.ContractType(common_utils.LockReleaseTokenPool),
			initialize:          tokenpoolops.InitializeLockRelease,
			getSelfServeAllowed: readLockRelease,
		},
	}

	// Deploy each pool: create its mint and initialize the pool, which also creates the global config.
	rmnRemote := solana.NewWallet().PublicKey()
	for _, p := range pools {
		mint := solana.NewWallet().PrivateKey
		ixs, err := tokens.CreateToken(ctx, solana.TokenProgramID, mint.PublicKey(), admin.PublicKey(), 9, client, rpc.CommitmentConfirmed)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, client, ixs, admin, rpc.CommitmentConfirmed, common.AddSigners(mint))

		bundle := operations.NewBundle(t.Context, logger.Test(t), operations.NewMemoryReporter())
		_, err = operations.ExecuteOperation(bundle, p.initialize, chain, tokenpoolops.Params{
			TokenPool:      p.program,
			TokenMint:      mint.PublicKey(),
			TokenProgramID: solana.TokenProgramID,
			Router:         ccip_router.ProgramID,
			RMNRemote:      rmnRemote,
		})
		require.NoError(t, err, "initialize %s pool", p.name)
	}

	// The changeset resolves each pool through the datastore, using the same refs the deployment writes.
	ds := datastore.NewMemoryDataStore()
	poolRef := func(p selfServedPool) datastore.AddressRef {
		return datastore.AddressRef{
			Type:      p.poolType,
			Version:   deploysolcommon.Version2_0_0,
			Qualifier: deploysolcommon.PoolProgramQualifier,
		}
	}
	for _, p := range pools {
		ref := poolRef(p)
		ref.ChainSelector = sel
		ref.Address = p.program.String()
		require.NoError(t, ds.Addresses().Add(ref))
	}

	// Each run needs its own bundle, because a bundle returns the cached report for the same input.
	newEnv := func() cldf.Environment {
		return cldf.Environment{
			DataStore:        ds.Seal(),
			BlockChains:      cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{sel: chain}),
			OperationsBundle: operations.NewBundle(t.Context, logger.Test(t), operations.NewMemoryReporter()),
			Logger:           logger.Test(t),
		}
	}
	mcmsInput := mcms.Input{
		TimelockAction: mcms_types.TimelockActionSchedule,
		ValidUntil:     uint32(time.Now().Add(time.Hour).Unix()), //nolint:gosec // fits in uint32 until 2106
	}
	setSelfServed := func(p selfServedPool, allowed bool) {
		t.Helper()
		cs := changesets.UpdateSelfServedAllowedChangeset()
		input := changesets.UpdateSelfServedAllowedChangesetInput{
			ChainSelector:     sel,
			TokenPoolRef:      poolRef(p),
			SelfServedAllowed: allowed,
			MCMS:              mcmsInput,
		}
		env := newEnv()
		require.NoError(t, cs.VerifyPreconditions(env, input))
		_, err := cs.Apply(env, input)
		require.NoError(t, err, "set self served allowed=%t on %s pool", allowed, p.name)
	}
	for _, p := range pools {
		// The flag starts disabled.
		got, err := p.getSelfServeAllowed(p.program)
		require.NoError(t, err)
		require.False(t, got, "%s pool starts with self served disabled", p.name)

		// Enable it and check the new value.
		setSelfServed(p, true)
		got, err = p.getSelfServeAllowed(p.program)
		require.NoError(t, err)
		require.True(t, got, "%s pool self served is enabled", p.name)

		// Enabling again must not send any transaction, and the flag stays enabled.
		sentBefore := *sent
		setSelfServed(p, true)
		require.Equal(t, sentBefore, *sent, "%s pool: no transaction is sent when the flag is already enabled", p.name)
		got, err = p.getSelfServeAllowed(p.program)
		require.NoError(t, err)
		require.True(t, got, "%s pool self served is still enabled", p.name)

		// Disable it and check the change happened.
		setSelfServed(p, false)
		got, err = p.getSelfServeAllowed(p.program)
		require.NoError(t, err)
		require.False(t, got, "%s pool self served is disabled", p.name)
	}
}
