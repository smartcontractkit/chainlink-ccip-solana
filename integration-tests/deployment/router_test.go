package deployment

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	offrampops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/offramp"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
)

// TestAddOffRamp runs the Router AddOffRamp operation on a local validator.
// The Router allowed_offramp account authorizes an OffRamp for one source chain.
// Token pools and receivers accept the account only when the Router owns it.
func TestAddOffRamp(t *testing.T) {
	ctx := t.Context()
	admin := testutils.Admin
	chain, _ := newLocalChain(t)
	client := chain.Client
	addOffRamp := func(input routerops.ConnectChainsParams) error {
		return exec(t, chain, routerops.AddOffRamp, input)
	}
	routerOwned := func(offRamp solana.PublicKey, remoteSelector uint64) {
		pda, err := state.FindAllowedOfframpPDA(remoteSelector, offRamp, ccip_router.ProgramID)
		require.NoError(t, err)
		account, err := client.GetAccountInfoWithOpts(ctx, pda, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentConfirmed})
		require.NoError(t, err)
		require.Equal(t, ccip_router.ProgramID, account.Value.Owner)
	}

	// Initialize the Router. The deployer becomes the owner, so AddOffRamp signs with the deployer key.
	err := exec(t, chain, routerops.Initialize, routerops.Params{
		Router:    ccip_router.ProgramID,
		FeeQuoter: solana.NewWallet().PublicKey(),
		LinkToken: solana.NewWallet().PublicKey(),
		RMNRemote: solana.NewWallet().PublicKey(),
	})
	require.NoError(t, err)

	const remoteSelector uint64 = 16015286601757825753

	// The first run creates the allowed_offramp account.
	offRamp := solana.NewWallet().PublicKey()
	input := routerops.ConnectChainsParams{Router: ccip_router.ProgramID, OffRamp: offRamp, RemoteChainSelector: remoteSelector}
	require.NoError(t, addOffRamp(input))
	routerOwned(offRamp, remoteSelector)

	// A second run finds the Router-owned account and does not send add_offramp again.
	// add_offramp uses init, so a second send fails.
	require.NoError(t, addOffRamp(input))
	routerOwned(offRamp, remoteSelector)

	// Any party can send lamports to the account address. The System program then owns it.
	// The operation must still send add_offramp, and init must take over the funded account.
	fundedOffRamp := solana.NewWallet().PublicKey()
	fundedPDA, err := state.FindAllowedOfframpPDA(remoteSelector, fundedOffRamp, ccip_router.ProgramID)
	require.NoError(t, err)
	transfer, err := system.NewTransferInstruction(1_000_000, admin.PublicKey(), fundedPDA).ValidateAndBuild()
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{transfer}, admin, rpc.CommitmentConfirmed)

	require.NoError(t, addOffRamp(routerops.ConnectChainsParams{Router: ccip_router.ProgramID, OffRamp: fundedOffRamp, RemoteChainSelector: remoteSelector}))
	routerOwned(fundedOffRamp, remoteSelector)
}

// TestConnectChainsAllowlist checks how ConnectChains creates a v2.0 lane on a pre-funded
// account and applies sender allowlist changes to it.
func TestConnectChainsAllowlist(t *testing.T) {
	ctx := t.Context()
	admin := testutils.Admin
	chain, _ := newLocalChain(t)
	client := chain.Client
	feeQuoter, rmnRemote := solana.NewWallet().PublicKey(), solana.NewWallet().PublicKey()
	err := exec(t, chain, routerops.Initialize, routerops.Params{
		Router:    ccip_router.ProgramID,
		FeeQuoter: feeQuoter,
		LinkToken: solana.NewWallet().PublicKey(),
		RMNRemote: rmnRemote,
	})
	require.NoError(t, err)
	// Adding a lane extends the OffRamp lookup table, which the OffRamp creates on initialize.
	err = exec(t, chain, offrampops.Initialize, offrampops.Params{
		FeeQuoter: feeQuoter,
		Router:    ccip_router.ProgramID,
		OffRamp:   ccip_offramp.ProgramID,
		RMNRemote: rmnRemote,
	})
	require.NoError(t, err)

	const remoteSelector uint64 = 16015286601757825753
	destChainPDA, err := state.FindDestChainStatePDAV2(remoteSelector, ccip_router.ProgramID)
	require.NoError(t, err)

	// Any party can send lamports to the lane address. The System program then owns it, and
	// ConnectChains must still add the lane.
	transfer, err := system.NewTransferInstruction(1_000_000, admin.PublicKey(), destChainPDA).ValidateAndBuild()
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{transfer}, admin, rpc.CommitmentConfirmed)

	laneConfig := testutils.DefaultV2DestChainConfig()
	// connect runs ConnectChains for the lane with the given allowlist changes.
	connect := func(enabled *bool, added, removed []solana.PublicKey) error {
		return exec(t, chain, routerops.ConnectChains, routerops.ConnectChainsParams{
			Router:                          ccip_router.ProgramID,
			OffRamp:                         ccip_offramp.ProgramID,
			RemoteChainSelector:             remoteSelector,
			RemoteAddressBytesLength:        laneConfig.AddressBytesLength,
			DestOffRamp:                     laneConfig.Offramp,
			DefaultExecutor:                 laneConfig.DefaultExecutor,
			DefaultCcvs:                     laneConfig.DefaultCcvs,
			MessageNetworkFeeUSDCents:       testutils.V2LaneMessageNetworkFeeUsdCents,
			TokenTransferNetworkFeeUSDCents: testutils.V2LaneTokenTransferNetworkFeeUsdCents,
			BaseExecutionGasCost:            laneConfig.BaseExecutionGasCost.V0,
			MaxFeePerMessageUSDCents:        testutils.V2LaneMaxFeePerMessageUsdCents,
			TokenReceiverAllowed:            laneConfig.TokenReceiverAllowed,
			Allowlist: &routerops.DestChainAllowlist{
				Enabled:                enabled,
				AllowedSendersToAdd:    added,
				AllowedSendersToRemove: removed,
			},
		})
	}
	requireAllowlist := func(enabled bool, senders []solana.PublicKey) {
		var lane ccip_router.DestChainCcipV2
		require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, destChainPDA, rpc.CommitmentConfirmed, &lane))
		require.Equal(t, enabled, lane.Config.AllowListEnabled)
		require.Equal(t, senders, lane.Config.AllowedSenders)
	}

	// The first run adds the lane over the funded account, with an enabled allowlist.
	enabled, disabled := true, false
	sender := solana.NewWallet().PublicKey()
	require.NoError(t, connect(&enabled, []solana.PublicKey{sender}, nil))
	requireAllowlist(true, []solana.PublicKey{sender})

	// An update without allowlist input keeps the lane's allowlist.
	require.NoError(t, connect(nil, nil, nil))
	requireAllowlist(true, []solana.PublicKey{sender})

	// Senders to add and to remove are applied to the current list.
	other := solana.NewWallet().PublicKey()
	require.NoError(t, connect(nil, []solana.PublicKey{other}, []solana.PublicKey{sender}))
	requireAllowlist(true, []solana.PublicKey{other})

	// Adding senders while disabling the allowlist is rejected and leaves the lane unchanged.
	require.ErrorContains(t, connect(&disabled, []solana.PublicKey{sender}, nil), "while the allowlist is disabled")
	requireAllowlist(true, []solana.PublicKey{other})

	// Disabling the allowlist keeps the stored senders.
	require.NoError(t, connect(&disabled, nil, nil))
	requireAllowlist(false, []solana.PublicKey{other})
}

// TestSetDestChainAllowlist runs the Router SetDestChainAllowlist operation on a local validator
// through a staged lane's lifecycle: promote it by disabling the allowlist, roll it back by
// enabling the kept list again, and rotate the senders on it.
func TestSetDestChainAllowlist(t *testing.T) {
	ctx := t.Context()
	chain, sent := newLocalChain(t)
	client := chain.Client
	feeQuoter, rmnRemote := solana.NewWallet().PublicKey(), solana.NewWallet().PublicKey()
	err := exec(t, chain, routerops.Initialize, routerops.Params{
		Router:    ccip_router.ProgramID,
		FeeQuoter: feeQuoter,
		LinkToken: solana.NewWallet().PublicKey(),
		RMNRemote: rmnRemote,
	})
	require.NoError(t, err)
	// Adding a lane extends the OffRamp lookup table, which the OffRamp creates on initialize.
	err = exec(t, chain, offrampops.Initialize, offrampops.Params{
		FeeQuoter: feeQuoter,
		Router:    ccip_router.ProgramID,
		OffRamp:   ccip_offramp.ProgramID,
		RMNRemote: rmnRemote,
	})
	require.NoError(t, err)

	laneConfig := testutils.DefaultV2DestChainConfig()
	connectLane := func(remoteSelector uint64, allowlist *routerops.DestChainAllowlist) error {
		return exec(t, chain, routerops.ConnectChains, routerops.ConnectChainsParams{
			Router:                          ccip_router.ProgramID,
			OffRamp:                         ccip_offramp.ProgramID,
			RemoteChainSelector:             remoteSelector,
			RemoteAddressBytesLength:        laneConfig.AddressBytesLength,
			DestOffRamp:                     laneConfig.Offramp,
			DefaultExecutor:                 laneConfig.DefaultExecutor,
			DefaultCcvs:                     laneConfig.DefaultCcvs,
			MessageNetworkFeeUSDCents:       testutils.V2LaneMessageNetworkFeeUsdCents,
			TokenTransferNetworkFeeUSDCents: testutils.V2LaneTokenTransferNetworkFeeUsdCents,
			BaseExecutionGasCost:            laneConfig.BaseExecutionGasCost.V0,
			MaxFeePerMessageUSDCents:        testutils.V2LaneMaxFeePerMessageUsdCents,
			TokenReceiverAllowed:            laneConfig.TokenReceiverAllowed,
			Allowlist:                       allowlist,
		})
	}
	setAllowlist := func(remoteSelector uint64, input routerops.SetDestChainAllowlistParams) error {
		input.LocalChainSelector = chain.Selector
		input.Router = ccip_router.ProgramID
		input.RemoteChainSelector = remoteSelector
		return exec(t, chain, routerops.SetDestChainAllowlist, input)
	}
	requireAllowlist := func(remoteSelector uint64, enabled bool, senders []solana.PublicKey) {
		destChainPDA, err := state.FindDestChainStatePDAV2(remoteSelector, ccip_router.ProgramID)
		require.NoError(t, err)
		var lane ccip_router.DestChainCcipV2
		require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, client, destChainPDA, rpc.CommitmentConfirmed, &lane))
		require.Equal(t, enabled, lane.Config.AllowListEnabled)
		require.Equal(t, senders, lane.Config.AllowedSenders)
	}

	// Stage the lane: the allowlist is enabled with the test sender only.
	enabled, disabled := true, false
	sender, other := solana.NewWallet().PublicKey(), solana.NewWallet().PublicKey()
	const remoteSelector uint64 = 16015286601757825753
	require.NoError(t, connectLane(remoteSelector, &routerops.DestChainAllowlist{
		Enabled:             &enabled,
		AllowedSendersToAdd: []solana.PublicKey{sender},
	}))
	requireAllowlist(remoteSelector, true, []solana.PublicKey{sender})

	// Promote: the list is disabled, and the senders are kept for a rollback.
	require.NoError(t, setAllowlist(remoteSelector, routerops.SetDestChainAllowlistParams{Enabled: &disabled}))
	requireAllowlist(remoteSelector, false, []solana.PublicKey{sender})

	// A lane already in the requested state is skipped and sends no transaction.
	before := *sent
	require.NoError(t, setAllowlist(remoteSelector, routerops.SetDestChainAllowlistParams{Enabled: &disabled}))
	require.Equal(t, before, *sent)

	// Adding senders while the allowlist is disabled is rejected and leaves the lane unchanged.
	require.ErrorContains(t, setAllowlist(remoteSelector, routerops.SetDestChainAllowlistParams{
		AllowedSendersToAdd: []solana.PublicKey{other},
	}), "while the allowlist is disabled")
	requireAllowlist(remoteSelector, false, []solana.PublicKey{sender})

	// Rollback: the kept list is enabled again without restating it.
	require.NoError(t, setAllowlist(remoteSelector, routerops.SetDestChainAllowlistParams{Enabled: &enabled}))
	requireAllowlist(remoteSelector, true, []solana.PublicKey{sender})

	// Senders to add and to remove apply to the current list.
	require.NoError(t, setAllowlist(remoteSelector, routerops.SetDestChainAllowlistParams{
		AllowedSendersToAdd:    []solana.PublicKey{other},
		AllowedSendersToRemove: []solana.PublicKey{sender},
	}))
	requireAllowlist(remoteSelector, true, []solana.PublicKey{other})

	// A lane that does not exist is reported.
	require.ErrorContains(t, setAllowlist(222782988166878823, routerops.SetDestChainAllowlistParams{Enabled: &disabled}),
		"is not configured")

	// A lane created open has no stored senders: enabling its allowlist would reject everyone,
	// the test wallet included, so it is rejected.
	const openSelector uint64 = 1642372171708781532
	require.NoError(t, connectLane(openSelector, nil))
	requireAllowlist(openSelector, false, nil)
	require.ErrorContains(t, setAllowlist(openSelector, routerops.SetDestChainAllowlistParams{Enabled: &enabled}),
		"at least one allowed sender")
}
