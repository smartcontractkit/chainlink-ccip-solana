package sourcereader

import (
	"encoding/binary"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/protocol"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/rmn_remote"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/commonreader"
)

// TestReadRMNCursedSubjects reads the RMN Remote curses account on a local validator.
// The RMN Remote initialize instruction creates the curses account with an empty subject list.
// The curse and uncurse instructions add and remove subjects in that account. No instruction closes it.
func TestReadRMNCursedSubjects(t *testing.T) {
	// Deploy the programs, but do not initialize RMN Remote.
	ctx := t.Context()
	admin := testutils.Admin
	rpcClient := testutils.DeployAllPrograms(t, testutils.PathToAnchorConfig, admin)
	require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{admin}, rpcClient))

	reader := sourceReaderRPC{Client: rpcClient}
	configPDA, _, err := state.FindRMNRemoteConfigPDA(rmn_remote.ProgramID)
	require.NoError(t, err)
	cursesPDA, _, err := state.FindRMNRemoteCursesPDA(rmn_remote.ProgramID)
	require.NoError(t, err)

	// The curses account does not exist yet.
	// The reader must return an error, because the curse state is unknown.
	_, err = commonreader.ReadRMNCursedSubjects(ctx, reader, rmn_remote.ProgramID)
	require.ErrorIs(t, err, rpc.ErrNotFound, "not initialized")

	// Initialize RMN Remote. This creates the config and curses accounts.
	ix, err := rmn_remote.NewInitializeInstruction(
		configPDA,
		cursesPDA,
		admin.PublicKey(),
		solana.SystemProgramID,
		rmn_remote.ProgramID,
		testutils.GetProgramDataAddress(ctx, t, rpcClient, rmn_remote.ProgramID),
	)
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, rpcClient, []solana.Instruction{ix}, admin, rpc.CommitmentConfirmed)

	// The curses account exists and has no subjects.
	// The reader must return an empty list and no error.
	subjects, err := commonreader.ReadRMNCursedSubjects(ctx, reader, rmn_remote.ProgramID)
	require.NoError(t, err)
	require.Empty(t, subjects, "initialized, no curses")

	// Curse all lanes (global subject) and one chain selector.
	const cursedSelector uint64 = 5009297550715157269
	globalCurse, chainCurse := testutils.GlobalCurseSubject(), testutils.ChainCurseSubject(cursedSelector)
	curseGlobal, err := rmn_remote.NewCurseInstruction(globalCurse, configPDA, admin.PublicKey(), cursesPDA, solana.SystemProgramID)
	require.NoError(t, err)
	curseChain, err := rmn_remote.NewCurseInstruction(chainCurse, configPDA, admin.PublicKey(), cursesPDA, solana.SystemProgramID)
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, rpcClient, []solana.Instruction{curseGlobal, curseChain}, admin, rpc.CommitmentConfirmed)

	// The reader must return both subjects.
	// The global subject does not change.
	// The chain selector changes from little endian in bytes 0-7 to big endian in bytes 8-15.
	var chainSubject protocol.Bytes16
	binary.BigEndian.PutUint64(chainSubject[8:], cursedSelector)
	subjects, err = commonreader.ReadRMNCursedSubjects(ctx, reader, rmn_remote.ProgramID)
	require.NoError(t, err)
	require.ElementsMatch(t, []protocol.Bytes16{protocol.Bytes16(globalCurse.Value), chainSubject}, subjects)

	// Uncurse both subjects. The curses account stays, with no subjects.
	// The reader must return an empty list and no error again.
	uncurseGlobal, err := rmn_remote.NewUncurseInstruction(globalCurse, configPDA, admin.PublicKey(), cursesPDA, solana.SystemProgramID)
	require.NoError(t, err)
	uncurseChain, err := rmn_remote.NewUncurseInstruction(chainCurse, configPDA, admin.PublicKey(), cursesPDA, solana.SystemProgramID)
	require.NoError(t, err)
	testutils.SendAndConfirm(ctx, t, rpcClient, []solana.Instruction{uncurseGlobal, uncurseChain}, admin, rpc.CommitmentConfirmed)

	subjects, err = commonreader.ReadRMNCursedSubjects(ctx, reader, rmn_remote.ProgramID)
	require.NoError(t, err)
	require.Empty(t, subjects, "all subjects uncursed")
}
