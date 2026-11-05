package deployment

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/committee_verifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	cvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
)

// TestCommitteeVerifierOperations runs the CommitteeVerifier deploy operations two times
// on a local validator. The second run must send no transaction.
func TestCommitteeVerifierOperations(t *testing.T) {
	ctx := t.Context()
	admin := testutils.Admin
	chain, sent := newLocalChain(t)
	client := chain.Client
	readConfig := func() *committee_verifier.Config {
		pda, _, err := state.FindConfigPDA(committee_verifier.ProgramID)
		require.NoError(t, err)
		account, err := client.GetAccountInfoWithOpts(ctx, pda, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentConfirmed})
		require.NoError(t, err)
		require.Equal(t, committee_verifier.ProgramID, account.Value.Owner)
		cfg, err := committee_verifier.ParseAccount_Config(account.Value.Data.GetBinary())
		require.NoError(t, err)
		return cfg
	}

	// Any party can send lamports to the Config address. The System program then owns it.
	// The operation must still send initialize, and init must take over the funded account.
	configPDA, _, err := state.FindConfigPDA(committee_verifier.ProgramID)
	require.NoError(t, err)
	transfer, err := system.NewTransferInstruction(1_000_000, admin.PublicKey(), configPDA).ValidateAndBuild()
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{transfer}, admin, rpc.CommitmentConfirmed)

	require.NoError(t, exec(t, chain, cvops.Initialize, cvops.Params{CommitteeVerifier: committee_verifier.ProgramID}))
	require.Equal(t, admin.PublicKey(), readConfig().AllowlistAdmin)

	// The deployer owns the CommitteeVerifier, so set_allowlist would be sent now. A lane without
	// a RemoteChainConfig, including a pre-funded one, is rejected before anything is sent.
	const remoteSelector uint64 = 16015286601757825753
	remotePDA, _, err := state.FindCVRemoteChainConfigPDA(remoteSelector, committee_verifier.ProgramID)
	require.NoError(t, err)
	transfer, err = system.NewTransferInstruction(1_000_000, admin.PublicKey(), remotePDA).ValidateAndBuild()
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{transfer}, admin, rpc.CommitmentConfirmed)
	before := *sent
	err = exec(t, chain, cvops.SetAllowlist, cvops.SetAllowlistParams{
		CommitteeVerifier:   committee_verifier.ProgramID,
		DestChainSelector:   remoteSelector,
		Enabled:             true,
		AllowedSendersToAdd: []solana.PublicKey{solana.NewWallet().PublicKey()},
	})
	require.ErrorContains(t, err, "is not initialized")
	require.Equal(t, before, *sent)

	// A disabled, empty allowlist already matches the uninitialized lane, so nothing is sent.
	require.NoError(t, exec(t, chain, cvops.SetAllowlist, cvops.SetAllowlistParams{CommitteeVerifier: committee_verifier.ProgramID, DestChainSelector: remoteSelector}))
	require.Equal(t, before, *sent)

	// A second run finds the initialized Config and does not send initialize again.
	before = *sent
	require.NoError(t, exec(t, chain, cvops.Initialize, cvops.Params{CommitteeVerifier: committee_verifier.ProgramID}))
	require.Equal(t, before, *sent)

	newAdmin := solana.NewWallet().PublicKey()
	require.NoError(t, exec(t, chain, cvops.SetAllowlistAdmin, cvops.SetAllowlistAdminParams{CommitteeVerifier: committee_verifier.ProgramID, AllowlistAdmin: newAdmin}))
	require.Equal(t, newAdmin, readConfig().AllowlistAdmin)

	// A second run finds the same admin and sends nothing.
	before = *sent
	require.NoError(t, exec(t, chain, cvops.SetAllowlistAdmin, cvops.SetAllowlistAdminParams{CommitteeVerifier: committee_verifier.ProgramID, AllowlistAdmin: newAdmin}))
	require.Equal(t, before, *sent)
	require.Equal(t, newAdmin, readConfig().AllowlistAdmin)

	feeAggregator := solana.NewWallet().PublicKey()
	require.NoError(t, exec(t, chain, cvops.SetFeeAggregator, cvops.SetFeeAggregatorParams{CommitteeVerifier: committee_verifier.ProgramID, FeeAggregator: feeAggregator}))
	require.Equal(t, feeAggregator, readConfig().FeeAggregator)
	before = *sent
	require.NoError(t, exec(t, chain, cvops.SetFeeAggregator, cvops.SetFeeAggregatorParams{CommitteeVerifier: committee_verifier.ProgramID, FeeAggregator: feeAggregator}))
	require.Equal(t, before, *sent)

	locations := []string{"postgres://aggregator-a", "postgres://aggregator-b"}
	before = *sent
	require.NoError(t, exec(t, chain, cvops.SetStorageLocations, cvops.SetStorageLocationsParams{CommitteeVerifier: committee_verifier.ProgramID, StorageLocations: locations}))
	require.Equal(t, before+1, *sent)
	before = *sent
	require.NoError(t, exec(t, chain, cvops.SetStorageLocations, cvops.SetStorageLocationsParams{CommitteeVerifier: committee_verifier.ProgramID, StorageLocations: locations}))
	require.Equal(t, before, *sent)

	// Hand ownership to another key, which stands in for MCMS. The lane's set_chain_config is
	// then only proposed, so set_allowlist for the same lane goes into the proposal under the
	// owner instead of being sent by the deployer.
	owner := solana.NewWallet().PrivateKey
	require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{owner}, client))
	eventAuthorityPDA, _, err := state.FindEventAuthorityPDA(committee_verifier.ProgramID)
	require.NoError(t, err)
	transferIx, err := committee_verifier.NewTransferOwnershipInstruction(owner.PublicKey(), configPDA, admin.PublicKey(), eventAuthorityPDA, committee_verifier.ProgramID)
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{transferIx}, admin, rpc.CommitmentConfirmed)
	acceptIx, err := committee_verifier.NewAcceptOwnershipInstruction(configPDA, owner.PublicKey(), eventAuthorityPDA, committee_verifier.ProgramID)
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, client, []solana.Instruction{acceptIx}, owner, rpc.CommitmentConfirmed)

	before = *sent
	output, err := run(t, chain, cvops.SetAllowlist, cvops.SetAllowlistParams{
		CommitteeVerifier:   committee_verifier.ProgramID,
		DestChainSelector:   remoteSelector,
		Enabled:             true,
		AllowedSendersToAdd: []solana.PublicKey{solana.NewWallet().PublicKey()},
	})
	require.NoError(t, err)
	require.Equal(t, before, *sent)
	require.Len(t, output.BatchOps, 1)
}
