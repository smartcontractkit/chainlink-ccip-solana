// token_admin_registry_config_test.go tests the configuration of the Token Admin Registry.
// This includes proposing and accepting token admins, setting token pools,
// transferring admin roles, and token-to-pool mapping management.
package contracts

import (
	"fmt"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
)

func TestTokenAdminRegistryConfig(t *testing.T) {
	t.Parallel()
	env := testutils.NewTestEnv(t).WithPools().WithOCR().Build()
	ctx := env.Ctx()
	client := env.Client
	user := testutils.User
	ccipAdmin := testutils.CcipAdmin
	token0PoolAdmin := testutils.Token0Admin
	token1PoolAdmin := testutils.Token1Admin
	token2PoolAdmin := testutils.Token2Admin
	usdcPoolAdmin := testutils.UsdcPoolAdmin
	legacyAdmin := testutils.Admin
	getTransmitter := env.GetTransmitter
	token0 := env.Token0
	token1 := env.Token1
	usdcPool := env.UsdcPool
	linkPool := env.LinkPool

	// Initialize router (needed for token admin registry PDAs)
	env.InitRouter(config.SvmChainSelector, testutils.FeeAggregator.PublicKey(), env.Link22.Mint)

	// Transfer router ownership to ccipAdmin (token admin registry operations require ccipAdmin as owner)
	ixTransfer, err := ccip_router.NewTransferOwnershipInstruction(ccipAdmin.PublicKey(), config.RouterConfigPDA, legacyAdmin.PublicKey())
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ixTransfer}, legacyAdmin, config.DefaultCommitment)
	ixAccept, err := ccip_router.NewAcceptOwnershipInstruction(config.RouterConfigPDA, ccipAdmin.PublicKey())
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ixAccept}, ccipAdmin, config.DefaultCommitment)

	t.Run("Token Admin Registry by Admin", func(t *testing.T) {
		t.Run("propose token admin registry as ccip admin", func(t *testing.T) {
			t.Run("When any user wants to set up the token admin registry, it fails", func(t *testing.T) {
				instruction, err := ccip_router.NewCcipAdminProposeAdministratorInstruction(
					token0PoolAdmin.PublicKey(),
					config.RouterConfigPDA,
					token0.AdminRegistryPDA,
					token0.Mint,
					user.PublicKey(),
					solana.SystemProgramID,
				)
				require.NoError(t, err)

				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
			})

			t.Run("When transmitter wants to set up the token admin registry, it fails", func(t *testing.T) {
				transmitter := getTransmitter()
				instruction, err := ccip_router.NewCcipAdminProposeAdministratorInstruction(
					token0PoolAdmin.PublicKey(),
					config.RouterConfigPDA,
					token0.AdminRegistryPDA,
					token0.Mint,
					transmitter.PublicKey(),
					solana.SystemProgramID,
				)
				require.NoError(t, err)

				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, transmitter, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
			})

			t.Run("When admin wants to set up the token admin registry, it succeeds", func(t *testing.T) {
				instruction, err := ccip_router.NewCcipAdminProposeAdministratorInstruction(
					token0PoolAdmin.PublicKey(),
					config.RouterConfigPDA,
					token0.AdminRegistryPDA,
					token0.Mint,
					ccipAdmin.PublicKey(),
					solana.SystemProgramID,
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, ccipAdmin, config.DefaultCommitment)

				// Validate Token Pool Registry PDA
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, token0.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.Equal(t, uint8(4), tokenAdminRegistry.Version)
				require.Equal(t, token0PoolAdmin.PublicKey(), tokenAdminRegistry.PendingAdministrator)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.Administrator)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.LookupTable)
			})
		})

		t.Run("accept token admin registry as token admin", func(t *testing.T) {
			t.Run("When any user wants to accept the token admin registry, it fails", func(t *testing.T) {
				instruction, err := ccip_router.NewAcceptAdminRoleTokenAdminRegistryInstruction(
					config.RouterConfigPDA,
					token0.AdminRegistryPDA,
					token0.Mint,
					user.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
			})

			t.Run("When ccip admin wants to accept the token admin registry, it fails", func(t *testing.T) {
				instruction, err := ccip_router.NewAcceptAdminRoleTokenAdminRegistryInstruction(
					config.RouterConfigPDA,
					token0.AdminRegistryPDA,
					token0.Mint,
					ccipAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, ccipAdmin, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
			})

			t.Run("When proposed admin wants to accept the token admin registry, it succeeds", func(t *testing.T) {
				instruction, err := ccip_router.NewAcceptAdminRoleTokenAdminRegistryInstruction(
					config.RouterConfigPDA,
					token0.AdminRegistryPDA,
					token0.Mint,
					token0PoolAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, token0PoolAdmin, config.DefaultCommitment)

				// Validate Token Pool Registry PDA
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, token0.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.Equal(t, uint8(4), tokenAdminRegistry.Version)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.PendingAdministrator)
				require.Equal(t, token0PoolAdmin.PublicKey(), tokenAdminRegistry.Administrator)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.LookupTable)
			})
		})

		t.Run("when admin is set, proposing a new one fails", func(t *testing.T) {
			instruction, err := ccip_router.NewCcipAdminOverridePendingAdministratorInstruction(
				token0PoolAdmin.PublicKey(),
				config.RouterConfigPDA,
				token0.AdminRegistryPDA,
				token0.Mint,
				ccipAdmin.PublicKey(),
				solana.SystemProgramID,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, ccipAdmin, config.DefaultCommitment, []string{ccip.InvalidTokenAdminRegistryProposedAdmin_CcipRouterError.String()})
		})

		t.Run("set pool", func(t *testing.T) {
			t.Run("When any user wants to set up the pool, it fails", func(t *testing.T) {
				instruction, err := ccip_router.NewSetPoolInstruction(
					token0.WritableIndexes,
					config.RouterConfigPDA,
					token0.AdminRegistryPDA,
					token0.Mint,
					token0.PoolLookupTable,
					user.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
			})

			t.Run("When transmitter wants to set up the pool, it fails", func(t *testing.T) {
				transmitter := getTransmitter()
				instruction, err := ccip_router.NewSetPoolInstruction(
					token0.WritableIndexes,
					config.RouterConfigPDA,
					token0.AdminRegistryPDA,
					token0.Mint,
					token0.PoolLookupTable,
					transmitter.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, transmitter, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
			})

			t.Run("When admin wants to set up the pool, it fails", func(t *testing.T) {
				instruction, err := ccip_router.NewSetPoolInstruction(
					token0.WritableIndexes,
					config.RouterConfigPDA,
					token0.AdminRegistryPDA,
					token0.Mint,
					token0.PoolLookupTable,
					ccipAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, ccipAdmin, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
			})

			t.Run("When setting pool to incorrect addresses in lookup table, it fails", func(t *testing.T) {
				instruction, err := ccip_router.NewSetPoolInstruction(
					token0.WritableIndexes,
					config.RouterConfigPDA,
					token0.AdminRegistryPDA,
					token0.Mint,
					token1.PoolLookupTable, // accounts do not match the expected mint related accounts
					token0PoolAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, token0PoolAdmin, config.DefaultCommitment, []string{ccip.InvalidInputsMint_CcipRouterError.String()})
			})

			t.Run("When Token Pool Admin wants to set up the pool, it succeeds", func(t *testing.T) {
				instruction, err := ccip_router.NewSetPoolInstruction(
					token0.WritableIndexes,
					config.RouterConfigPDA,
					token0.AdminRegistryPDA,
					token0.Mint,
					token0.PoolLookupTable,
					token0PoolAdmin.PublicKey(),
				)

				genericInstruction, _ := instruction.(*solana.GenericInstruction)
				genericInstruction.AccountValues = append(genericInstruction.AccountValues, solana.Meta(token0.PoolLookupTable))
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, token0PoolAdmin, config.DefaultCommitment)

				// Validate Token Pool Registry PDA
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, token0.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.Equal(t, token0PoolAdmin.PublicKey(), tokenAdminRegistry.Administrator)
				require.Equal(t, uint8(4), tokenAdminRegistry.Version)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.PendingAdministrator)
				require.Equal(t, token0.PoolLookupTable, tokenAdminRegistry.LookupTable)
			})

			t.Run("Setting up token autoderivation", func(t *testing.T) {
				instruction, err := ccip_router.NewSetPoolInterfaceImplementationInstruction(
					ccip_router.SetPoolInterfaceImplementationParams{
						Mint:              token0.Mint,
						InterfaceVersion:  2,
						AccountResolution: ccip_router.ResolutionType_Standard,
					},
					config.RouterConfigPDA,
					token0.AdminRegistryPDA,
					token0PoolAdmin.PublicKey(),
					config.RouterEventAuthorityPDA,
					config.CcipRouterProgram,
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, token0PoolAdmin, config.DefaultCommitment)

				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, token0.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.True(t, tokenAdminRegistry.AccountResolutionEnabled)
				require.False(t, tokenAdminRegistry.SkipLookupTableAdditionalEntries)
			})

			t.Run("When Token Pool Admin wants to set up the pool again to zero, it is none", func(t *testing.T) {
				instruction, err := ccip_router.NewSetPoolInstruction(
					token0.WritableIndexes,
					config.RouterConfigPDA,
					token0.AdminRegistryPDA,
					token0.Mint,
					solana.PublicKey{},
					token0PoolAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, token0PoolAdmin, config.DefaultCommitment)

				// Validate Token Pool Registry PDA
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, token0.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.Equal(t, token0PoolAdmin.PublicKey(), tokenAdminRegistry.Administrator)
				require.Equal(t, uint8(4), tokenAdminRegistry.Version)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.PendingAdministrator)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.LookupTable)

				// Rollback to previous state
				instruction, err = ccip_router.NewSetPoolInstruction(
					token0.WritableIndexes,
					config.RouterConfigPDA,
					token0.AdminRegistryPDA,
					token0.Mint,
					token0.PoolLookupTable,
					token0PoolAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, token0PoolAdmin, config.DefaultCommitment)
			})
		})

		t.Run("Transfer admin role for token admin registry", func(t *testing.T) {
			t.Run("When any user wants to transfer the token admin registry, it fails", func(t *testing.T) {
				instruction, err := ccip_router.NewTransferAdminRoleTokenAdminRegistryInstruction(
					token1PoolAdmin.PublicKey(),
					config.RouterConfigPDA,
					token0.AdminRegistryPDA,
					token0.Mint,
					user.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
			})

			t.Run("When admin wants to transfer the token admin registry, it succeeds and permissions stay with no changes", func(t *testing.T) {
				instruction, err := ccip_router.NewTransferAdminRoleTokenAdminRegistryInstruction(
					token1PoolAdmin.PublicKey(),
					config.RouterConfigPDA,
					token0.AdminRegistryPDA,
					token0.Mint,
					token0PoolAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, token0PoolAdmin, config.DefaultCommitment)

				// Validate Token Pool Registry PDA
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, token0.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.Equal(t, token0PoolAdmin.PublicKey(), tokenAdminRegistry.Administrator)
				require.Equal(t, uint8(4), tokenAdminRegistry.Version)
				require.Equal(t, token1PoolAdmin.PublicKey(), tokenAdminRegistry.PendingAdministrator)
				require.Equal(t, token0.PoolLookupTable, tokenAdminRegistry.LookupTable)

				// check if the admin is still the same
				instruction, err = ccip_router.NewSetPoolInstruction(
					token0.WritableIndexes,
					config.RouterConfigPDA,
					token0.AdminRegistryPDA,
					token0.Mint,
					token0.PoolLookupTable,
					token0PoolAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, token0PoolAdmin, config.DefaultCommitment)

				// new one cant make changes yet
				instruction, err = ccip_router.NewSetPoolInstruction(
					token0.WritableIndexes,
					config.RouterConfigPDA,
					token0.AdminRegistryPDA,
					token0.Mint,
					token0.PoolLookupTable,
					token1PoolAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, token1PoolAdmin, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
			})

			t.Run("When new admin accepts the token admin registry, it succeeds and permissions are updated", func(t *testing.T) {
				instruction, err := ccip_router.NewAcceptAdminRoleTokenAdminRegistryInstruction(
					config.RouterConfigPDA,
					token0.AdminRegistryPDA,
					token0.Mint,
					token1PoolAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, token1PoolAdmin, config.DefaultCommitment)

				// Validate Token Pool Registry PDA
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, token0.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.Equal(t, token1PoolAdmin.PublicKey(), tokenAdminRegistry.Administrator)
				require.Equal(t, uint8(4), tokenAdminRegistry.Version)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.PendingAdministrator)
				require.Equal(t, token0.PoolLookupTable, tokenAdminRegistry.LookupTable)

				// check old admin can not make changes anymore
				instruction, err = ccip_router.NewSetPoolInstruction(
					token0.WritableIndexes,
					config.RouterConfigPDA,
					token0.AdminRegistryPDA,
					token0.Mint,
					token0.PoolLookupTable,
					token0PoolAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, token0PoolAdmin, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})

				// new one can make changes now
				instruction, err = ccip_router.NewSetPoolInstruction(
					token0.WritableIndexes,
					config.RouterConfigPDA,
					token0.AdminRegistryPDA,
					token0.Mint,
					token0.PoolLookupTable,
					token1PoolAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, token1PoolAdmin, config.DefaultCommitment)
			})
		})

		t.Run("setup: CCTP USDC pool", func(t *testing.T) {
			t.Run("propose pool as CCIP Admin", func(t *testing.T) {
				instruction, err := ccip_router.NewCcipAdminProposeAdministratorInstruction(
					usdcPoolAdmin.PublicKey(),
					config.RouterConfigPDA,
					usdcPool.AdminRegistryPDA,
					usdcPool.Mint,
					ccipAdmin.PublicKey(),
					solana.SystemProgramID,
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, ccipAdmin, config.DefaultCommitment)

				// Validate Token Pool Registry PDA
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, usdcPool.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.Equal(t, uint8(4), tokenAdminRegistry.Version)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.Administrator)
				require.Equal(t, usdcPoolAdmin.PublicKey(), tokenAdminRegistry.PendingAdministrator)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.LookupTable)
			})

			t.Run("accept token admin registry as token admin", func(t *testing.T) {
				instruction, err := ccip_router.NewAcceptAdminRoleTokenAdminRegistryInstruction(
					config.RouterConfigPDA,
					usdcPool.AdminRegistryPDA,
					usdcPool.Mint,
					usdcPoolAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, usdcPoolAdmin, config.DefaultCommitment)

				// Validate Token Pool Registry PDA
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, usdcPool.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.Equal(t, uint8(4), tokenAdminRegistry.Version)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.PendingAdministrator)
				require.Equal(t, usdcPoolAdmin.PublicKey(), tokenAdminRegistry.Administrator)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.LookupTable)
			})

			t.Run("set pool", func(t *testing.T) {
				ix, err := ccip_router.NewSetPoolInstruction(
					usdcPool.WritableIndexes,
					config.RouterConfigPDA,
					usdcPool.AdminRegistryPDA,
					usdcPool.Mint,
					usdcPool.PoolLookupTable,
					usdcPoolAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, usdcPoolAdmin, config.DefaultCommitment)

				// Validate Token Pool Registry PDA
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, usdcPool.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.Equal(t, usdcPoolAdmin.PublicKey(), tokenAdminRegistry.Administrator)
				require.Equal(t, uint8(4), tokenAdminRegistry.Version)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.PendingAdministrator)
				require.Equal(t, usdcPool.PoolLookupTable, tokenAdminRegistry.LookupTable)
				require.False(t, tokenAdminRegistry.AccountResolutionEnabled)
				require.Equal(t, uint8(1), tokenAdminRegistry.InterfaceVersion)
			})

			t.Run("set pool account auto-derivation support", func(t *testing.T) {
				ix, err := ccip_router.NewSetPoolInterfaceImplementationInstruction(
					ccip_router.SetPoolInterfaceImplementationParams{
						Mint:              usdcPool.Mint,
						InterfaceVersion:  2,
						AccountResolution: ccip_router.ResolutionType_Standard,
					},
					config.RouterConfigPDA,
					usdcPool.AdminRegistryPDA,
					usdcPoolAdmin.PublicKey(),
					config.RouterEventAuthorityPDA,
					config.CcipRouterProgram,
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, usdcPoolAdmin, config.DefaultCommitment)

				// Validate Token Pool Registry PDA
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, usdcPool.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.Equal(t, usdcPoolAdmin.PublicKey(), tokenAdminRegistry.Administrator)
				require.Equal(t, uint8(4), tokenAdminRegistry.Version)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.PendingAdministrator)
				require.Equal(t, usdcPool.PoolLookupTable, tokenAdminRegistry.LookupTable)
				require.True(t, tokenAdminRegistry.AccountResolutionEnabled)
				require.False(t, tokenAdminRegistry.SkipLookupTableAdditionalEntries)
			})
		})
	})

	t.Run("Token Admin Registry by Mint Authority", func(t *testing.T) {
		t.Run("propose token admin registry as token mint authority", func(t *testing.T) {
			t.Run("When any user wants to set up the token admin registry, it fails", func(t *testing.T) {
				instruction, err := ccip_router.NewOwnerProposeAdministratorInstruction(
					token1PoolAdmin.PublicKey(),
					config.RouterConfigPDA,
					token1.AdminRegistryPDA,
					token1.Mint,
					user.PublicKey(),
					solana.SystemProgramID,
				)
				require.NoError(t, err)

				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
			})

			t.Run("When transmitter wants to set up the token admin registry, it fails", func(t *testing.T) {
				transmitter := getTransmitter()
				instruction, err := ccip_router.NewOwnerProposeAdministratorInstruction(
					token1PoolAdmin.PublicKey(),
					config.RouterConfigPDA,
					token1.AdminRegistryPDA,
					token1.Mint,
					transmitter.PublicKey(),
					solana.SystemProgramID,
				)
				require.NoError(t, err)

				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, transmitter, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
			})

			t.Run("When ccip admin wants to set up the token admin registry, it fails", func(t *testing.T) {
				instruction, err := ccip_router.NewOwnerProposeAdministratorInstruction(
					token1PoolAdmin.PublicKey(),
					config.RouterConfigPDA,
					token1.AdminRegistryPDA,
					token1.Mint,
					ccipAdmin.PublicKey(),
					solana.SystemProgramID,
				)
				require.NoError(t, err)

				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, ccipAdmin, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
			})

			t.Run("When token mint_authority wants to set up the token admin registry, it succeeds", func(t *testing.T) {
				instruction, err := ccip_router.NewOwnerProposeAdministratorInstruction(
					token1PoolAdmin.PublicKey(),
					config.RouterConfigPDA,
					token1.AdminRegistryPDA,
					token1.Mint,
					token1PoolAdmin.PublicKey(),
					solana.SystemProgramID,
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, token1PoolAdmin, config.DefaultCommitment)

				// Validate Token Pool Registry PDA
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, token1.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.Equal(t, uint8(4), tokenAdminRegistry.Version)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.Administrator)
				require.Equal(t, token1PoolAdmin.PublicKey(), tokenAdminRegistry.PendingAdministrator)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.LookupTable)
			})
		})

		t.Run("after admin is proposed but not accepted, overriding it succeeds", func(t *testing.T) {
			instruction, err := ccip_router.NewOwnerOverridePendingAdministratorInstruction(
				token2PoolAdmin.PublicKey(),
				config.RouterConfigPDA,
				token1.AdminRegistryPDA,
				token1.Mint,
				token1PoolAdmin.PublicKey(),
				solana.SystemProgramID,
			)
			require.NoError(t, err)

			testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, token1PoolAdmin, config.DefaultCommitment)

			tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
			err = common.GetAccountDataBorshIntoAnchor(ctx, client, token1.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
			require.NoError(t, err)
			require.Equal(t, uint8(4), tokenAdminRegistry.Version)
			require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.Administrator)
			require.Equal(t, token2PoolAdmin.PublicKey(), tokenAdminRegistry.PendingAdministrator)
			require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.LookupTable)

			// We override it back to the previous proposal
			instruction, err = ccip_router.NewOwnerOverridePendingAdministratorInstruction(
				token1PoolAdmin.PublicKey(),
				config.RouterConfigPDA,
				token1.AdminRegistryPDA,
				token1.Mint,
				token1PoolAdmin.PublicKey(),
				solana.SystemProgramID,
			)
			require.NoError(t, err)

			testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, token1PoolAdmin, config.DefaultCommitment)

			tokenAdminRegistry = ccip_common.TokenAdminRegistry{}
			err = common.GetAccountDataBorshIntoAnchor(ctx, client, token1.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
			require.NoError(t, err)
			require.Equal(t, uint8(4), tokenAdminRegistry.Version)
			require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.Administrator)
			require.Equal(t, token1PoolAdmin.PublicKey(), tokenAdminRegistry.PendingAdministrator)
			require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.LookupTable)
		})

		t.Run("accept token admin registry as token admin", func(t *testing.T) {
			t.Run("When any user wants to accept the token admin registry, it fails", func(t *testing.T) {
				instruction, err := ccip_router.NewAcceptAdminRoleTokenAdminRegistryInstruction(
					config.RouterConfigPDA,
					token1.AdminRegistryPDA,
					token1.Mint,
					user.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
			})

			t.Run("When ccip admin wants to accept the token admin registry, it fails", func(t *testing.T) {
				instruction, err := ccip_router.NewAcceptAdminRoleTokenAdminRegistryInstruction(
					config.RouterConfigPDA,
					token1.AdminRegistryPDA,
					token1.Mint,
					ccipAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, ccipAdmin, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
			})

			t.Run("When proposed admin wants to accept the token admin registry, it succeeds", func(t *testing.T) {
				instruction, err := ccip_router.NewAcceptAdminRoleTokenAdminRegistryInstruction(
					config.RouterConfigPDA,
					token1.AdminRegistryPDA,
					token1.Mint,
					token1PoolAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, token1PoolAdmin, config.DefaultCommitment)

				// Validate Token Pool Registry PDA
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, token1.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.Equal(t, uint8(4), tokenAdminRegistry.Version)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.PendingAdministrator)
				require.Equal(t, token1PoolAdmin.PublicKey(), tokenAdminRegistry.Administrator)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.LookupTable)
			})
		})

		t.Run("when admin is set, proposing a new one fails", func(t *testing.T) {
			instruction, err := ccip_router.NewOwnerOverridePendingAdministratorInstruction(
				token1PoolAdmin.PublicKey(),
				config.RouterConfigPDA,
				token1.AdminRegistryPDA,
				token1.Mint,
				token1PoolAdmin.PublicKey(),
				solana.SystemProgramID,
			)
			require.NoError(t, err)

			testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, token1PoolAdmin, config.DefaultCommitment, []string{ccip.InvalidTokenAdminRegistryProposedAdmin_CcipRouterError.String()})
		})

		t.Run("set pool", func(t *testing.T) {
			t.Run("When Mint Authority wants to set up the pool, it succeeds", func(t *testing.T) {
				instruction, err := ccip_router.NewSetPoolInstruction(
					token1.WritableIndexes,
					config.RouterConfigPDA,
					token1.AdminRegistryPDA,
					token1.Mint,
					token1.PoolLookupTable,
					token1PoolAdmin.PublicKey(),
				)
				require.NoError(t, err)

				// transfer mint authority to pool once admin registry is set
				ixAuth, err := tokens.SetTokenMintAuthority(token1.Program, token1.PoolSigner, token1.Mint, token1PoolAdmin.PublicKey())
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction, ixAuth}, token1PoolAdmin, config.DefaultCommitment)

				// Validate Token Pool Registry PDA
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, token1.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.Equal(t, token1PoolAdmin.PublicKey(), tokenAdminRegistry.Administrator)
				require.Equal(t, uint8(4), tokenAdminRegistry.Version)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.PendingAdministrator)
				require.Equal(t, token1.PoolLookupTable, tokenAdminRegistry.LookupTable)
			})

			t.Run("Setting an invalid pool interface implementation fails", func(t *testing.T) {
				for _, v := range []uint8{0, 3, 255} {
					t.Run(fmt.Sprintf("interface version %d", v), func(t *testing.T) {
						ix, err := ccip_router.NewSetPoolInterfaceImplementationInstruction(
							ccip_router.SetPoolInterfaceImplementationParams{
								Mint:              token1.Mint,
								InterfaceVersion:  v,
								AccountResolution: ccip_router.ResolutionType_Standard,
							},
							config.RouterConfigPDA,
							token1.AdminRegistryPDA,
							token1PoolAdmin.PublicKey(),
							config.RouterEventAuthorityPDA,
							config.CcipRouterProgram,
						)
						require.NoError(t, err)

						testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, token1PoolAdmin, config.DefaultCommitment, []string{"InvalidPoolInterfaceVersion."})
					})
				}
			})

			t.Run("Setting account resolution with v1 interface fails", func(t *testing.T) {
				ix, err := ccip_router.NewSetPoolInterfaceImplementationInstruction(
					ccip_router.SetPoolInterfaceImplementationParams{
						Mint:              token1.Mint,
						InterfaceVersion:  1,
						AccountResolution: ccip_router.ResolutionType_Standard,
					},
					config.RouterConfigPDA,
					token1.AdminRegistryPDA,
					token1PoolAdmin.PublicKey(),
					config.RouterEventAuthorityPDA,
					config.CcipRouterProgram,
				)
				require.NoError(t, err)

				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{ix}, token1PoolAdmin, config.DefaultCommitment, []string{ccip.AccountResolutionRequiresV2_CcipRouterError.String()})
			})

			t.Run("Setting up token account resolution by a random user fails", func(t *testing.T) {
				instruction, err := ccip_router.NewSetPoolInterfaceImplementationInstruction(
					ccip_router.SetPoolInterfaceImplementationParams{
						Mint:              token1.Mint,
						InterfaceVersion:  2,
						AccountResolution: ccip_router.ResolutionType_Standard,
					},
					config.RouterConfigPDA,
					token1.AdminRegistryPDA,
					user.PublicKey(), // not the admin
					config.RouterEventAuthorityPDA,
					config.CcipRouterProgram,
				)
				require.NoError(t, err)

				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, user, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
			})

			t.Run("Setting up token account resolution", func(t *testing.T) {
				instruction, err := ccip_router.NewSetPoolInterfaceImplementationInstruction(
					ccip_router.SetPoolInterfaceImplementationParams{
						Mint:              token1.Mint,
						InterfaceVersion:  2,
						AccountResolution: ccip_router.ResolutionType_Standard,
					},
					config.RouterConfigPDA,
					token1.AdminRegistryPDA,
					token1PoolAdmin.PublicKey(),
					config.RouterEventAuthorityPDA,
					config.CcipRouterProgram,
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, token1PoolAdmin, config.DefaultCommitment)
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, token1.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.True(t, tokenAdminRegistry.AccountResolutionEnabled)
				require.False(t, tokenAdminRegistry.SkipLookupTableAdditionalEntries)
				require.Equal(t, uint8(2), tokenAdminRegistry.InterfaceVersion)
			})

			t.Run("Setting up token account resolution as disabled for token1", func(t *testing.T) {
				instruction, err := ccip_router.NewSetPoolInterfaceImplementationInstruction(
					ccip_router.SetPoolInterfaceImplementationParams{
						Mint:              token1.Mint,
						InterfaceVersion:  1,
						AccountResolution: ccip_router.ResolutionType_NoResolution,
					},
					config.RouterConfigPDA,
					token1.AdminRegistryPDA,
					token1PoolAdmin.PublicKey(),
					config.RouterEventAuthorityPDA,
					config.CcipRouterProgram,
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, token1PoolAdmin, config.DefaultCommitment)
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, token1.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.False(t, tokenAdminRegistry.AccountResolutionEnabled)
				require.Equal(t, uint8(1), tokenAdminRegistry.InterfaceVersion)
			})
		})

		t.Run("Transfer admin role for token admin registry", func(t *testing.T) {
			t.Run("When invalid wants to transfer the token admin registry, it fails", func(t *testing.T) {
				instruction, err := ccip_router.NewTransferAdminRoleTokenAdminRegistryInstruction(
					token0PoolAdmin.PublicKey(),
					config.RouterConfigPDA,
					token1.AdminRegistryPDA,
					token1.Mint,
					token0PoolAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, token0PoolAdmin, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
			})
			t.Run("When mint authority wants to transfer the token admin registry, it succeeds and permissions stay with no changes", func(t *testing.T) {
				instruction, err := ccip_router.NewTransferAdminRoleTokenAdminRegistryInstruction(
					token0PoolAdmin.PublicKey(),
					config.RouterConfigPDA,
					token1.AdminRegistryPDA,
					token1.Mint,
					token1PoolAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, token1PoolAdmin, config.DefaultCommitment)

				// Validate Token Pool Registry PDA
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, token1.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.Equal(t, token1PoolAdmin.PublicKey(), tokenAdminRegistry.Administrator)
				require.Equal(t, uint8(4), tokenAdminRegistry.Version)
				require.Equal(t, token0PoolAdmin.PublicKey(), tokenAdminRegistry.PendingAdministrator)
				require.Equal(t, token1.PoolLookupTable, tokenAdminRegistry.LookupTable)

				// check if the admin is still the same
				instruction, err = ccip_router.NewSetPoolInstruction(
					token1.WritableIndexes,
					config.RouterConfigPDA,
					token1.AdminRegistryPDA,
					token1.Mint,
					token1.PoolLookupTable,
					token1PoolAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, token1PoolAdmin, config.DefaultCommitment)

				// new one cant make changes yet
				instruction, err = ccip_router.NewSetPoolInstruction(
					token1.WritableIndexes,
					config.RouterConfigPDA,
					token1.AdminRegistryPDA,
					token1.Mint,
					token1.PoolLookupTable,
					token0PoolAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, token0PoolAdmin, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
			})

			t.Run("When new admin accepts the token admin registry, it succeeds and permissions are updated", func(t *testing.T) {
				instruction, err := ccip_router.NewAcceptAdminRoleTokenAdminRegistryInstruction(
					config.RouterConfigPDA,
					token1.AdminRegistryPDA,
					token1.Mint,
					token0PoolAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, token0PoolAdmin, config.DefaultCommitment)

				// Validate Token Pool Registry PDA
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, token1.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.Equal(t, token0PoolAdmin.PublicKey(), tokenAdminRegistry.Administrator)
				require.Equal(t, uint8(4), tokenAdminRegistry.Version)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.PendingAdministrator)
				require.Equal(t, token1.PoolLookupTable, tokenAdminRegistry.LookupTable)

				// check old admin can not make changes anymore
				instruction, err = ccip_router.NewSetPoolInstruction(
					token1.WritableIndexes,
					config.RouterConfigPDA,
					token1.AdminRegistryPDA,
					token1.Mint,
					token1.PoolLookupTable,
					token1PoolAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, token1PoolAdmin, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})

				// new one can make changes now
				instruction, err = ccip_router.NewSetPoolInstruction(
					token1.WritableIndexes,
					config.RouterConfigPDA,
					token1.AdminRegistryPDA,
					token1.Mint,
					token1.PoolLookupTable,
					token0PoolAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, token0PoolAdmin, config.DefaultCommitment)
			})
		})

		t.Run("Token Admin Registry by Multisig Mint Authority", func(t *testing.T) {
			// Create a new SPL token with a regular keypair as mint authority
			multisigTestMint, mintKeyErr := solana.NewRandomPrivateKey()
			require.NoError(t, mintKeyErr)
			multisigTestAdmin, adminKeyErr := solana.NewRandomPrivateKey()
			require.NoError(t, adminKeyErr)

			// Fund the admin
			require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{multisigTestAdmin}, client))

			// Create the token
			ixCreate, createErr := tokens.CreateToken(ctx, config.SPLTokenProgram, multisigTestMint.PublicKey(), multisigTestAdmin.PublicKey(), 9, client, config.DefaultCommitment)
			require.NoError(t, createErr)
			testutils.SendAndConfirm(ctx, t, client, ixCreate, multisigTestAdmin, config.DefaultCommitment, common.AddSigners(multisigTestMint))

			// Create a multisig (1-of-2) with multisigTestAdmin and a random signer
			otherSigner, otherKeyErr := solana.NewRandomPrivateKey()
			require.NoError(t, otherKeyErr)
			require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{otherSigner}, client))

			multisigAccount, msigKeyErr := solana.NewRandomPrivateKey()
			require.NoError(t, msigKeyErr)
			ixMsig, msigErr := tokens.CreateMultisig(ctx, multisigTestAdmin.PublicKey(), config.SPLTokenProgram, multisigAccount.PublicKey(), 1, []solana.PublicKey{multisigTestAdmin.PublicKey(), otherSigner.PublicKey()}, client, config.DefaultCommitment)
			require.NoError(t, msigErr)
			testutils.SendAndConfirm(ctx, t, client, ixMsig, multisigTestAdmin, config.DefaultCommitment, common.AddSigners(multisigAccount))

			// Transfer mint authority to the multisig
			ixSetAuth, setAuthErr := tokens.SetTokenMintAuthority(config.SPLTokenProgram, multisigAccount.PublicKey(), multisigTestMint.PublicKey(), multisigTestAdmin.PublicKey())
			require.NoError(t, setAuthErr)
			testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ixSetAuth}, multisigTestAdmin, config.DefaultCommitment)

			// Derive the token admin registry PDA for this new token
			adminRegistryPDA, _, pErr := state.FindTokenAdminRegistryPDA(multisigTestMint.PublicKey(), config.CcipRouterProgram)
			require.NoError(t, pErr)

			t.Run("When a non-signer of the multisig tries to propose, it fails", func(t *testing.T) {
				nonSigner, nonSignerErr := solana.NewRandomPrivateKey()
				require.NoError(t, nonSignerErr)
				require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{nonSigner}, client))

				instruction, err := ccip_router.NewMultisigOwnerProposeAdministratorInstruction(
					multisigTestAdmin.PublicKey(),
					config.RouterConfigPDA,
					adminRegistryPDA,
					multisigTestMint.PublicKey(),
					nonSigner.PublicKey(),
					multisigAccount.PublicKey(), // multisig account
					solana.SystemProgramID,
				)
				require.NoError(t, err)
				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, nonSigner, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
			})

			t.Run("When the multisig account proposes with the multisig key, it fails", func(t *testing.T) {
				// prefund the adminRegistryPDA - this is because the multisigAccount has data,
				// so it can sign for the call but cannot fund the PDA itself.
				ix, err := tokens.NativeTransfer(1*solana.LAMPORTS_PER_SOL, multisigTestAdmin.PublicKey(), adminRegistryPDA)
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ix}, multisigTestAdmin, config.DefaultCommitment)

				instruction, err := ccip_router.NewOwnerProposeAdministratorInstruction(
					multisigTestAdmin.PublicKey(),
					config.RouterConfigPDA,
					adminRegistryPDA,
					multisigTestMint.PublicKey(),
					multisigAccount.PublicKey(), // using multisig accounts as authority
					solana.SystemProgramID,
				)
				require.NoError(t, err)
				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, multisigTestAdmin, config.DefaultCommitment, []string{"TokenOwnedByMultisig"}, common.AddSigners(multisigAccount))
			})

			t.Run("When a valid multisig signer proposes, it succeeds", func(t *testing.T) {
				instruction, err := ccip_router.NewMultisigOwnerProposeAdministratorInstruction(
					multisigTestAdmin.PublicKey(),
					config.RouterConfigPDA,
					adminRegistryPDA,
					multisigTestMint.PublicKey(),
					multisigTestAdmin.PublicKey(),
					multisigAccount.PublicKey(), // multisig account
					solana.SystemProgramID,
				)
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, multisigTestAdmin, config.DefaultCommitment)

				// Validate
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, adminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.Equal(t, uint8(4), tokenAdminRegistry.Version)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.Administrator)
				require.Equal(t, multisigTestAdmin.PublicKey(), tokenAdminRegistry.PendingAdministrator)
			})

			t.Run("When another valid multisig signer overrides pending admin, it succeeds", func(t *testing.T) {
				instruction, err := ccip_router.NewMultisigOwnerOverridePendingAdministratorInstruction(
					otherSigner.PublicKey(), // new proposed admin
					config.RouterConfigPDA,
					adminRegistryPDA,
					multisigTestMint.PublicKey(),
					otherSigner.PublicKey(),
					multisigAccount.PublicKey(), // multisig account
					solana.SystemProgramID,
				)
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, otherSigner, config.DefaultCommitment)

				// Validate
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, adminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.Equal(t, uint8(4), tokenAdminRegistry.Version)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.Administrator)
				require.Equal(t, otherSigner.PublicKey(), tokenAdminRegistry.PendingAdministrator)
			})

			t.Run("When passing a wrong multisig account, it fails", func(t *testing.T) {
				// Create a different multisig that doesn't match the mint authority
				fakeMultisig, fakeKeyErr := solana.NewRandomPrivateKey()
				require.NoError(t, fakeKeyErr)
				ixFakeMsig, fakeErr := tokens.CreateMultisig(ctx, multisigTestAdmin.PublicKey(), config.SPLTokenProgram, fakeMultisig.PublicKey(), 1, []solana.PublicKey{multisigTestAdmin.PublicKey(), otherSigner.PublicKey()}, client, config.DefaultCommitment)
				require.NoError(t, fakeErr)
				testutils.SendAndConfirm(ctx, t, client, ixFakeMsig, multisigTestAdmin, config.DefaultCommitment, common.AddSigners(fakeMultisig))

				instruction, err := ccip_router.NewMultisigOwnerOverridePendingAdministratorInstruction(
					multisigTestAdmin.PublicKey(),
					config.RouterConfigPDA,
					adminRegistryPDA,
					multisigTestMint.PublicKey(),
					multisigTestAdmin.PublicKey(),
					fakeMultisig.PublicKey(), // wrong multisig
					solana.SystemProgramID,
				)
				require.NoError(t, err)
				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, multisigTestAdmin, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
			})

			t.Run("When multisig threshold is greater than 1, it fails", func(t *testing.T) {
				// Create a new token for this sub-test
				m2Mint, m2MintErr := solana.NewRandomPrivateKey()
				require.NoError(t, m2MintErr)
				m2Admin, m2AdminErr := solana.NewRandomPrivateKey()
				require.NoError(t, m2AdminErr)
				require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{m2Admin}, client))

				ixCreateM2, createM2Err := tokens.CreateToken(ctx, config.SPLTokenProgram, m2Mint.PublicKey(), m2Admin.PublicKey(), 9, client, config.DefaultCommitment)
				require.NoError(t, createM2Err)
				testutils.SendAndConfirm(ctx, t, client, ixCreateM2, m2Admin, config.DefaultCommitment, common.AddSigners(m2Mint))

				// Create a 2-of-2 multisig
				thirdSigner, thirdErr := solana.NewRandomPrivateKey()
				require.NoError(t, thirdErr)
				require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{thirdSigner}, client))

				m2Multisig, m2MsigKeyErr := solana.NewRandomPrivateKey()
				require.NoError(t, m2MsigKeyErr)
				ixM2Msig, m2MsigErr := tokens.CreateMultisig(ctx, m2Admin.PublicKey(), config.SPLTokenProgram, m2Multisig.PublicKey(), 2, []solana.PublicKey{m2Admin.PublicKey(), thirdSigner.PublicKey()}, client, config.DefaultCommitment)
				require.NoError(t, m2MsigErr)
				testutils.SendAndConfirm(ctx, t, client, ixM2Msig, m2Admin, config.DefaultCommitment, common.AddSigners(m2Multisig))

				// Transfer mint authority to the 2-of-2 multisig
				ixSetM2Auth, setM2AuthErr := tokens.SetTokenMintAuthority(config.SPLTokenProgram, m2Multisig.PublicKey(), m2Mint.PublicKey(), m2Admin.PublicKey())
				require.NoError(t, setM2AuthErr)
				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{ixSetM2Auth}, m2Admin, config.DefaultCommitment)

				m2AdminRegistryPDA, _, m2PErr := state.FindTokenAdminRegistryPDA(m2Mint.PublicKey(), config.CcipRouterProgram)
				require.NoError(t, m2PErr)

				instruction, err := ccip_router.NewMultisigOwnerProposeAdministratorInstruction(
					m2Admin.PublicKey(),
					config.RouterConfigPDA,
					m2AdminRegistryPDA,
					m2Mint.PublicKey(),
					m2Admin.PublicKey(),
					m2Multisig.PublicKey(), // 2-of-2 multisig - should be rejected
					solana.SystemProgramID,
				)
				require.NoError(t, err)
				testutils.SendAndFailWith(ctx, t, client, []solana.Instruction{instruction}, m2Admin, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
			})
		})

		t.Run("setup: Link pool", func(t *testing.T) {
			t.Run("propose link pool as token mint authority", func(t *testing.T) {
				instruction, err := ccip_router.NewOwnerProposeAdministratorInstruction(
					legacyAdmin.PublicKey(),
					config.RouterConfigPDA,
					linkPool.AdminRegistryPDA,
					linkPool.Mint,
					legacyAdmin.PublicKey(),
					solana.SystemProgramID,
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, legacyAdmin, config.DefaultCommitment)

				// Validate Token Pool Registry PDA
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, linkPool.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.Equal(t, uint8(4), tokenAdminRegistry.Version)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.Administrator)
				require.Equal(t, legacyAdmin.PublicKey(), tokenAdminRegistry.PendingAdministrator)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.LookupTable)
			})

			t.Run("accept token admin registry as token admin", func(t *testing.T) {
				instruction, err := ccip_router.NewAcceptAdminRoleTokenAdminRegistryInstruction(
					config.RouterConfigPDA,
					linkPool.AdminRegistryPDA,
					linkPool.Mint,
					legacyAdmin.PublicKey(),
				)
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction}, legacyAdmin, config.DefaultCommitment)

				// Validate Token Pool Registry PDA
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, linkPool.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.Equal(t, uint8(4), tokenAdminRegistry.Version)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.PendingAdministrator)
				require.Equal(t, legacyAdmin.PublicKey(), tokenAdminRegistry.Administrator)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.LookupTable)
			})

			t.Run("set pool", func(t *testing.T) {
				instruction, err := ccip_router.NewSetPoolInstruction(
					linkPool.WritableIndexes,
					config.RouterConfigPDA,
					linkPool.AdminRegistryPDA,
					linkPool.Mint,
					linkPool.PoolLookupTable,
					legacyAdmin.PublicKey(),
				)
				require.NoError(t, err)

				// transfer mint authority to pool once admin registry is set
				ixAuth, err := tokens.SetTokenMintAuthority(linkPool.Program, linkPool.PoolSigner, linkPool.Mint, legacyAdmin.PublicKey())
				require.NoError(t, err)

				testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{instruction, ixAuth}, legacyAdmin, config.DefaultCommitment)

				// Validate Token Pool Registry PDA
				tokenAdminRegistry := ccip_common.TokenAdminRegistry{}
				err = common.GetAccountDataBorshIntoAnchor(ctx, client, linkPool.AdminRegistryPDA, config.DefaultCommitment, &tokenAdminRegistry)
				require.NoError(t, err)
				require.Equal(t, legacyAdmin.PublicKey(), tokenAdminRegistry.Administrator)
				require.Equal(t, uint8(4), tokenAdminRegistry.Version)
				require.Equal(t, solana.PublicKey{}, tokenAdminRegistry.PendingAdministrator)
				require.Equal(t, linkPool.PoolLookupTable, tokenAdminRegistry.LookupTable)
			})
		})
	})
}
