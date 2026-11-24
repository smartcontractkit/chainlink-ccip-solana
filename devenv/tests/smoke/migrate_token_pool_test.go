package smoke

import (
	"os"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"
	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	ccv "github.com/smartcontractkit/chainlink-ccv/build/devenv"
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/chainreg"
	e2ehelpers "github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	soltokens "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	solchangesets "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/changesets"
	soldevenv "github.com/smartcontractkit/chainlink-ccip-solana/devenv"
	"github.com/smartcontractkit/chainlink-ccip-solana/devenv/ccvregistry"
	devenvutils "github.com/smartcontractkit/chainlink-ccip-solana/devenv/tests/utils"
)

// migrateTokenPoolEnv opts in to TestSolanaMigrateTokenPoolToV2. The test moves a 1.6.1 pool to the
// 2.0 interface for good, so other tests that send that token expect a different pool afterwards.
const migrateTokenPoolEnv = "SOLANA_MIGRATE_TOKEN_POOL_E2E"

// TestSolanaMigrateTokenPoolToV2 migrates the devenv's 1.6.1 BurnMint pool with
// MigrateTokenPoolToV2Changeset, checks the on-chain result, and runs the changeset again to check
// that the second run sends nothing and registers nothing. Run it on a local devenv only.
func TestSolanaMigrateTokenPoolToV2(t *testing.T) {
	if os.Getenv(migrateTokenPoolEnv) == "" {
		t.Skipf("set %s to run; it changes the environment for later tests", migrateTokenPoolEnv)
	}
	if os.Getenv("SOLANA_PRIVATE_KEY") != "" {
		t.Skip("attached environment: the migration only runs on a local devenv")
	}

	factory := soldevenv.NewImplFactory()
	require.NoError(t, chainreg.Register(chainsel.FamilySolana, chainreg.Registration{
		ImplFactory:     factory,
		ExecutorInfo:    factory,
		CLDFProvider:    ccvregistry.SolanaCLDFProviderFactory,
		AddressResolver: soldevenv.NewAddressResolver(),
	}))
	ctx := ccv.Plog.WithContext(t.Context())
	lib, err := ccv.NewLibFromCCVEnv(zerolog.Ctx(ctx), e2ehelpers.GetSmokeTestConfig())
	require.NoError(t, err)
	env, err := lib.CLDFEnvironment()
	require.NoError(t, err)
	cfg, err := smokeConfig()
	require.NoError(t, err)
	solSelector, evmSelector, err := devenvutils.ResolveSolanaEVMLanes(cfg.Blockchains)
	require.NoError(t, err)
	chain, ok := env.BlockChains.SolanaChains()[solSelector]
	require.True(t, ok)

	poolType := common_utils.BurnMintTokenPool.String()
	var legacy *datastore.AddressRef
	for _, ref := range env.DataStore.Addresses().Filter(
		datastore.AddressRefByChainSelector(solSelector),
		datastore.AddressRefByType(datastore.ContractType(poolType)),
	) {
		if ref.Qualifier != solcommon.PoolProgramQualifier && ref.Version != nil && ref.Version.Major() == 1 {
			legacy = &ref
			break
		}
	}
	if legacy == nil {
		t.Skip("no 1.6 BurnMint pool instance deployed")
	}
	mint := solanaMintForPoolPair(t, env.DataStore, solSelector, legacy.Qualifier)

	input := solchangesets.MigrateTokenPoolToV2ChangesetInput{
		ChainSelector:        solSelector,
		PoolType:             poolType,
		TokenMint:            mint.String(),
		PoolProgramQualifier: solcommon.PoolProgramQualifier,
		RemoteChainSelectors: []uint64{evmSelector},
	}
	changeset := solchangesets.MigrateTokenPoolToV2Changeset()

	require.NoError(t, changeset.VerifyPreconditions(*env, input))
	first, err := changeset.Apply(*env, input)
	require.NoError(t, err)
	require.Empty(t, first.MCMSTimelockProposals, "the devenv deployer controls the pool, so nothing is proposed")

	merged := datastore.NewMemoryDataStore()
	require.NoError(t, merged.Merge(env.DataStore))
	if first.DataStore != nil {
		require.NoError(t, merged.Merge(first.DataStore.Seal()))
	}
	migratedEnv := *env
	migratedEnv.DataStore = merged.Seal()
	instance, err := solcommon.PoolRefByQualifier(migratedEnv.DataStore, solSelector, poolType, legacy.Qualifier)
	require.NoError(t, err)
	require.Equal(t, uint64(2), instance.Version.Major(), "the instance resolves at 2.x after the migration")

	router, err := solcommon.PubkeyFromRefs(migratedEnv.DataStore.Addresses().Filter(), datastore.NewAddressRefKey(
		solSelector, datastore.ContractType(routerops.ContractType), routerops.Version, ""))
	require.NoError(t, err)
	tarPDA, _, err := state.FindTokenAdminRegistryPDA(mint, router)
	require.NoError(t, err)
	tar, _, err := routerops.ReadTokenAdminRegistry(ctx, chain.Client, tarPDA)
	require.NoError(t, err)
	require.Equal(t, uint8(2), tar.InterfaceVersion)
	require.Equal(t, ccip_router.ResolutionType_WithoutExtendedLookupTable, routerops.TokenAdminRegistryResolutionType(tar))

	program := solana.MustPublicKeyFromBase58(legacy.Address)
	chainConfigV2, _, err := soltokens.TokenPoolChainConfigV2PDA(evmSelector, mint, program)
	require.NoError(t, err)
	_, err = chain.Client.GetAccountInfoWithOpts(ctx, chainConfigV2, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentConfirmed})
	require.NoError(t, err, "ChainConfigV2 of the migrated lane exists")

	deployer := chain.DeployerKey.PublicKey()
	before, err := chain.Client.GetBalance(ctx, deployer, rpc.CommitmentConfirmed)
	require.NoError(t, err)
	require.NoError(t, changeset.VerifyPreconditions(migratedEnv, input))
	second, err := changeset.Apply(migratedEnv, input)
	require.NoError(t, err)
	require.Empty(t, second.MCMSTimelockProposals)
	if second.DataStore != nil {
		refs, fetchErr := second.DataStore.Addresses().Fetch()
		require.NoError(t, fetchErr)
		require.Empty(t, refs, "a second run registers nothing")
	}
	after, err := chain.Client.GetBalance(ctx, deployer, rpc.CommitmentConfirmed)
	require.NoError(t, err)
	require.Equal(t, before.Value, after.Value, "a second run sends no transaction")
}
