package deployment

import (
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	tar_v1 "github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/v0_1_0/ccip_common"
	tar_v2 "github.com/smartcontractkit/chainlink-ccip/chains/solana/gobindings/v0_1_1/ccip_common"
	solcommon "github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/common"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
)

// TestLegacyTokenAdminRegistry runs the token operations on registries with the v1 and v2 layouts.
// The Router upgraded in place from 1.6 keeps these registries until something upgrades them,
// and a Router instruction cannot create them, so the validator loads them at start.
func TestLegacyTokenAdminRegistry(t *testing.T) {
	ctx := t.Context()
	admin := testutils.Admin

	legacy := func(mint solana.PublicKey, registry any) testutils.PreloadedAccount {
		pda, _, err := state.FindTokenAdminRegistryPDA(mint, ccip_router.ProgramID)
		require.NoError(t, err)
		data, err := bin.MarshalBorsh(registry)
		require.NoError(t, err)
		return testutils.PreloadedAccount{Address: pda, Owner: ccip_router.ProgramID, Data: data}
	}
	mintV1, mintV2 := solana.NewWallet().PrivateKey, solana.NewWallet().PrivateKey
	tarV1 := legacy(mintV1.PublicKey(), tar_v1.TokenAdminRegistry{Version: 1, Administrator: admin.PublicKey(), Mint: mintV1.PublicKey()})
	tarV2 := legacy(mintV2.PublicKey(), tar_v2.TokenAdminRegistry{Version: 2, Administrator: admin.PublicKey(), Mint: mintV2.PublicKey(), SupportsAutoDerivation: true})

	client := testutils.DeployAllPrograms(t, testutils.PathToAnchorConfig, admin, tarV1, tarV2)
	require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{admin}, client))
	chain := cldf_solana.Chain{
		Selector:    124615329519749607, // solana-devnet
		Client:      client,
		DeployerKey: &admin,
		Confirm: func(ixs []solana.Instruction, opts ...solcommon.TxModifier) error {
			_, err := solcommon.SendAndConfirm(ctx, client, ixs, admin, rpc.CommitmentConfirmed, opts...)
			return err
		},
	}
	require.NoError(t, exec(t, chain, routerops.Initialize, routerops.Params{
		Router:    ccip_router.ProgramID,
		FeeQuoter: solana.NewWallet().PublicKey(),
		LinkToken: solana.NewWallet().PublicKey(),
		RMNRemote: solana.NewWallet().PublicKey(),
	}))

	for _, mint := range []solana.PrivateKey{mintV1, mintV2} {
		ixs, createErr := tokens.CreateToken(ctx, solana.TokenProgramID, mint.PublicKey(), admin.PublicKey(), 9, client, rpc.CommitmentConfirmed)
		require.NoError(t, createErr)
		testutils.SendAndConfirm(ctx, t, client, ixs, admin, rpc.CommitmentConfirmed, common.AddSigners(mint))
	}

	// The registry exists with the same administrator, so the operation upgrades it and registers nothing.
	for _, mint := range []solana.PrivateKey{mintV1, mintV2} {
		bundle := operations.NewBundle(t.Context, logger.Test(t), operations.NewMemoryReporter())
		_, err := operations.ExecuteOperation(bundle, routerops.RegisterTokenAdminRegistry, chain, routerops.TokenAdminRegistryParams{
			Router: ccip_router.ProgramID, TokenMint: mint.PublicKey(), Admin: admin.PublicKey(),
		})
		require.NoError(t, err)
	}

	read := func(pda solana.PublicKey) (ccip_router.TokenAdminRegistry, uint8) {
		tar, version, readErr := routerops.ReadTokenAdminRegistry(ctx, client, pda)
		require.NoError(t, readErr)
		return tar, version
	}
	v1, version := read(tarV1.Address)
	require.Equal(t, uint8(3), version)
	require.Equal(t, admin.PublicKey(), v1.Administrator)
	require.Equal(t, mintV1.PublicKey(), v1.Mint)
	require.Equal(t, ccip_router.ResolutionType_NoResolution, v1.AccountResolution)
	require.Equal(t, uint8(1), v1.InterfaceVersion)

	v2, version := read(tarV2.Address)
	require.Equal(t, uint8(3), version)
	require.Equal(t, admin.PublicKey(), v2.Administrator)
	require.Equal(t, ccip_router.ResolutionType_Standard, v2.AccountResolution)

	// A Router write that requires v3 now succeeds.
	require.NoError(t, exec(t, chain, routerops.SetPoolInterfaceImplementation, routerops.SetPoolInterfaceImplParams{
		Router: ccip_router.ProgramID, TokenMint: mintV2.PublicKey(), InterfaceVersion: 2, ImplementsAccountResolution: 1,
	}))
	v2, _ = read(tarV2.Address)
	require.Equal(t, uint8(2), v2.InterfaceVersion)
}
