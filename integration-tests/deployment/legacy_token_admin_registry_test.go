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

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
)

// TestLegacyTokenAdminRegistry runs the token operations on registries with the v1, v2 and v3 layouts.
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
	// v3 is the v2 layout followed by bump and interface_version, but with the whole ResolutionType enum
	// stored in the byte v2 (and the 1.6 offramp) reads as a bool, so it can hold 2
	// (WithoutExtendedLookupTable). No published binding has that layout, so it is built from v2's.
	legacyV3 := func(mint solana.PublicKey, resolution ccip_router.ResolutionType) testutils.PreloadedAccount {
		account := legacy(mint, tar_v2.TokenAdminRegistry{Version: 3, Administrator: admin.PublicKey(), Mint: mint})
		_, bump, err := state.FindTokenAdminRegistryPDA(mint, ccip_router.ProgramID)
		require.NoError(t, err)
		account.Data[len(account.Data)-1] = uint8(resolution)
		account.Data = append(account.Data, bump, 2) // interface version 2
		return account
	}
	mintV1, mintV2 := solana.NewWallet().PrivateKey, solana.NewWallet().PrivateKey
	mintV3Standard, mintV3SkipExtras := solana.NewWallet().PrivateKey, solana.NewWallet().PrivateKey
	tarV1 := legacy(mintV1.PublicKey(), tar_v1.TokenAdminRegistry{Version: 1, Administrator: admin.PublicKey(), Mint: mintV1.PublicKey()})
	tarV2 := legacy(mintV2.PublicKey(), tar_v2.TokenAdminRegistry{Version: 2, Administrator: admin.PublicKey(), Mint: mintV2.PublicKey(), SupportsAutoDerivation: true})
	tarV3Standard := legacyV3(mintV3Standard.PublicKey(), ccip_router.ResolutionType_Standard)
	tarV3SkipExtras := legacyV3(mintV3SkipExtras.PublicKey(), ccip_router.ResolutionType_WithoutExtendedLookupTable)
	allMints := []solana.PrivateKey{mintV1, mintV2, mintV3Standard, mintV3SkipExtras}
	// Left alone until the end, where setting WithoutExtendedLookupTable is the first thing to touch it.
	mintV3ToSkipExtras := solana.NewWallet().PrivateKey
	tarV3ToSkipExtras := legacyV3(mintV3ToSkipExtras.PublicKey(), ccip_router.ResolutionType_Standard)

	client := testutils.DeployAllPrograms(t, testutils.PathToAnchorConfig, admin, tarV1, tarV2, tarV3Standard, tarV3SkipExtras, tarV3ToSkipExtras)
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

	for _, mint := range append(allMints, mintV3ToSkipExtras) {
		ixs, createErr := tokens.CreateToken(ctx, solana.TokenProgramID, mint.PublicKey(), admin.PublicKey(), 9, client, rpc.CommitmentConfirmed)
		require.NoError(t, createErr)
		testutils.SendAndConfirm(ctx, t, client, ixs, admin, rpc.CommitmentConfirmed, common.AddSigners(mint))
	}

	read := func(pda solana.PublicKey) (ccip_router.TokenAdminRegistry, uint8) {
		tar, version, readErr := routerops.ReadTokenAdminRegistry(ctx, client, pda)
		require.NoError(t, readErr)
		return tar, version
	}

	// Before the upgrade, the reader fills the fields an old layout lacks as the on-chain loader does,
	// and maps a v3 resolution byte of 2 onto the two latest-layout fields.
	for _, tc := range []struct {
		name             string
		account          testutils.PreloadedAccount
		version          uint8
		resolution       ccip_router.ResolutionType
		interfaceVersion uint8
	}{
		{"v1", tarV1, 1, ccip_router.ResolutionType_NoResolution, 1},
		{"v2", tarV2, 2, ccip_router.ResolutionType_Standard, 1},
		{"v3 standard", tarV3Standard, 3, ccip_router.ResolutionType_Standard, 2},
		{"v3 without extended lookup table", tarV3SkipExtras, 3, ccip_router.ResolutionType_WithoutExtendedLookupTable, 2},
	} {
		tar, version := read(tc.account.Address)
		require.Equal(t, tc.version, version, tc.name)
		require.Equal(t, tc.resolution, routerops.TokenAdminRegistryResolutionType(tar), tc.name)
		require.Equal(t, tc.interfaceVersion, tar.InterfaceVersion, tc.name)
	}

	// The registry exists with the same administrator, so the operation upgrades it and registers nothing.
	for _, mint := range allMints {
		bundle := operations.NewBundle(t.Context, logger.Test(t), operations.NewMemoryReporter())
		_, err := operations.ExecuteOperation(bundle, routerops.RegisterTokenAdminRegistry, chain, routerops.TokenAdminRegistryParams{
			Router: ccip_router.ProgramID, TokenMint: mint.PublicKey(), Admin: admin.PublicKey(),
		})
		require.NoError(t, err)
	}

	v1, version := read(tarV1.Address)
	require.Equal(t, uint8(routerops.LatestTokenAdminRegistryVersion), version)
	require.Equal(t, admin.PublicKey(), v1.Administrator)
	require.Equal(t, mintV1.PublicKey(), v1.Mint)
	require.Equal(t, ccip_router.ResolutionType_NoResolution, routerops.TokenAdminRegistryResolutionType(v1))
	require.Equal(t, uint8(1), v1.InterfaceVersion)

	v2, version := read(tarV2.Address)
	require.Equal(t, uint8(routerops.LatestTokenAdminRegistryVersion), version)
	require.Equal(t, admin.PublicKey(), v2.Administrator)
	require.Equal(t, ccip_router.ResolutionType_Standard, routerops.TokenAdminRegistryResolutionType(v2))

	// The v3 to v4 upgrade keeps the resolution mode and interface version, and leaves a 0/1 in the byte
	// the 1.6 offramp decodes as a bool: a v3 registry holding 2 there is unreadable by 1.6 until upgraded.
	for _, tc := range []struct {
		name       string
		account    testutils.PreloadedAccount
		mint       solana.PublicKey
		resolution ccip_router.ResolutionType
		skipExtras uint8
	}{
		{"v3 standard", tarV3Standard, mintV3Standard.PublicKey(), ccip_router.ResolutionType_Standard, 0},
		{"v3 without extended lookup table", tarV3SkipExtras, mintV3SkipExtras.PublicKey(), ccip_router.ResolutionType_WithoutExtendedLookupTable, 1},
	} {
		tar, version := read(tc.account.Address)
		require.Equal(t, uint8(routerops.LatestTokenAdminRegistryVersion), version, tc.name)
		require.Equal(t, admin.PublicKey(), tar.Administrator, tc.name)
		require.Equal(t, tc.mint, tar.Mint, tc.name)
		require.Equal(t, tc.resolution, routerops.TokenAdminRegistryResolutionType(tar), tc.name)
		require.Equal(t, uint8(2), tar.InterfaceVersion, tc.name)
		_, bump, err := state.FindTokenAdminRegistryPDA(tc.mint, ccip_router.ProgramID)
		require.NoError(t, err)
		require.Equal(t, bump, tar.Bump, tc.name)

		info, err := client.GetAccountInfoWithOpts(ctx, tc.account.Address, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentConfirmed})
		require.NoError(t, err)
		raw := info.Value.Data.GetBinary()
		// discriminator, version, administrator, pending administrator, lookup table, writable indexes, mint
		const resolutionOffset = 8 + 1 + 32 + 32 + 32 + 32 + 32
		require.Equal(t, []byte{1, bump, 2, tc.skipExtras}, raw[resolutionOffset:], tc.name)
	}

	// A Router write that requires v3 now succeeds.
	require.NoError(t, exec(t, chain, routerops.SetPoolInterfaceImplementation, routerops.SetPoolInterfaceImplParams{
		Router: ccip_router.ProgramID, TokenMint: mintV2.PublicKey(), InterfaceVersion: 2, ImplementsAccountResolution: 1,
	}))
	v2, _ = read(tarV2.Address)
	require.Equal(t, uint8(2), v2.InterfaceVersion)

	// Setting WithoutExtendedLookupTable upgrades a registry still on the v3 layout first. Written on v3, the mode
	// would be a 2 in the byte the 1.6 offramp decodes as a bool, and the token's 1.6 lanes would break.
	_, version = read(tarV3ToSkipExtras.Address)
	require.Equal(t, uint8(3), version)
	require.NoError(t, exec(t, chain, routerops.SetPoolInterfaceImplementation, routerops.SetPoolInterfaceImplParams{
		Router: ccip_router.ProgramID, TokenMint: mintV3ToSkipExtras.PublicKey(), InterfaceVersion: 2,
		ImplementsAccountResolution: uint8(ccip_router.ResolutionType_WithoutExtendedLookupTable),
	}))
	skipExtras, version := read(tarV3ToSkipExtras.Address)
	require.Equal(t, uint8(routerops.LatestTokenAdminRegistryVersion), version)
	require.Equal(t, ccip_router.ResolutionType_WithoutExtendedLookupTable, routerops.TokenAdminRegistryResolutionType(skipExtras))
	info, err := client.GetAccountInfoWithOpts(ctx, tarV3ToSkipExtras.Address, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentConfirmed})
	require.NoError(t, err)
	_, bump, err := state.FindTokenAdminRegistryPDA(mintV3ToSkipExtras.PublicKey(), ccip_router.ProgramID)
	require.NoError(t, err)
	// discriminator, version, administrator, pending administrator, lookup table, writable indexes, mint
	const resolutionOffset = 8 + 1 + 32 + 32 + 32 + 32 + 32
	require.Equal(t, []byte{1, bump, 2, 1}, info.Value.Data.GetBinary()[resolutionOffset:])
}
