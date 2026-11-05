//revive:disable:var-naming // legacy package name
package common

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"
)

func TestInstructionWithProgramID(t *testing.T) {
	t.Parallel()

	accounts := []*solana.AccountMeta{solana.Meta(solana.NewWallet().PublicKey()).WRITE()}
	ix := solana.NewInstruction(solana.NewWallet().PublicKey(), accounts, []byte{1, 2, 3})
	programID := solana.NewWallet().PublicKey()

	got, err := InstructionWithProgramID(ix, programID)
	require.NoError(t, err)
	require.Equal(t, programID, got.ProgramID())
	require.Equal(t, accounts, got.Accounts())
	data, err := got.Data()
	require.NoError(t, err)
	require.Equal(t, []byte{1, 2, 3}, data)
}

func TestBuildReadOnlySimulationTx(t *testing.T) {
	t.Parallel()

	payer := solana.NewWallet().PublicKey()
	programID := solana.NewWallet().PublicKey()
	ix := &solana.GenericInstruction{
		AccountValues: []*solana.AccountMeta{
			solana.Meta(solana.NewWallet().PublicKey()),
		},
		ProgID:    programID,
		DataBytes: []byte{1, 2, 3},
	}

	tx, opts, err := BuildReadOnlySimulationTx(payer, ix)
	require.NoError(t, err)

	require.Equal(t, solana.Hash{}, tx.Message.RecentBlockhash)
	require.Equal(t, payer, tx.Message.AccountKeys[0])
	require.Len(t, tx.Signatures, 1)
	require.Equal(t, solana.Signature{}, tx.Signatures[0])

	require.Len(t, tx.Message.Instructions, 3)
	require.Equal(t, computebudget.ProgramID, tx.Message.AccountKeys[tx.Message.Instructions[0].ProgramIDIndex])
	require.Equal(t, computebudget.ProgramID, tx.Message.AccountKeys[tx.Message.Instructions[1].ProgramIDIndex])
	require.Equal(t, programID, tx.Message.AccountKeys[tx.Message.Instructions[2].ProgramIDIndex])
	require.Equal(t, []byte{1, 2, 3}, []byte(tx.Message.Instructions[2].Data))

	require.Equal(t, &rpc.SimulateTransactionOpts{
		ReplaceRecentBlockhash: true,
		SigVerify:              false,
		Commitment:             rpc.CommitmentConfirmed,
	}, opts)
}

// Simulating only the compute-budget preamble always succeeds while exercising nothing, so the
// builders reject an empty list rather than returning a tx that silently tests nothing.
func TestBuildReadOnlySimulationTxRejectsNoInstructions(t *testing.T) {
	t.Parallel()

	payer := solana.NewWallet().PublicKey()

	_, _, err := BuildReadOnlySimulationTx(payer)
	require.ErrorContains(t, err, "no instructions to simulate")

	_, _, err = BuildReadOnlySimulationTxWithLookupTables(payer, nil)
	require.ErrorContains(t, err, "no instructions to simulate")
}
