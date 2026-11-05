package fees

import (
	"fmt"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/programs/token"
	"github.com/stretchr/testify/require"
)

func TestSet(t *testing.T) {
	t.Run("ComputeUnitPrice", func(t *testing.T) {
		t.Parallel()
		testSet(t, func(v uint) ComputeUnitPrice {
			return ComputeUnitPrice(v)
		}, SetComputeUnitPrice, true)
	})
	t.Run("ComputeUnitLimit", func(t *testing.T) {
		t.Parallel()
		testSet(t, func(v uint) ComputeUnitLimit {
			return ComputeUnitLimit(v) //nolint:gosec // G115
		}, SetComputeUnitLimit, false)
	})
}

func testSet[V instruction](t *testing.T, builder func(uint) V, setter func(*solana.Transaction, V) error, expectFirstInstruction bool) {
	key, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	getIndex := func(count int) int {
		index := count - 1
		if expectFirstInstruction {
			index = 0
		}
		return index
	}

	t.Run("noAccount_nofee", func(t *testing.T) {
		t.Parallel()
		// build base tx (no fee)
		tx, err := solana.NewTransaction([]solana.Instruction{
			system.NewTransferInstruction(
				0,
				key.PublicKey(),
				key.PublicKey(),
			).Build(),
		}, solana.Hash{})
		require.NoError(t, err)
		instructionCount := len(tx.Message.Instructions)

		// add fee
		require.NoError(t, setter(tx, builder(1)))

		// evaluate
		currentCount := len(tx.Message.Instructions)
		require.Greater(t, currentCount, instructionCount)
		require.Equal(t, 2, currentCount)
		i := getIndex(currentCount)
		require.Equal(t, ComputeBudgetProgram, tx.Message.AccountKeys[tx.Message.Instructions[i].ProgramIDIndex])
		data, err := builder(1).Data()
		require.NoError(t, err)
		require.Equal(t, data, []byte(tx.Message.Instructions[i].Data))
	})

	t.Run("accountExists_noFee", func(t *testing.T) {
		t.Parallel()
		// build base tx (no fee)
		tx, err := solana.NewTransaction([]solana.Instruction{
			system.NewTransferInstruction(
				0,
				key.PublicKey(),
				key.PublicKey(),
			).Build(),
		}, solana.Hash{})
		require.NoError(t, err)
		accountCount := len(tx.Message.AccountKeys)
		tx.Message.AccountKeys = append(tx.Message.AccountKeys, ComputeBudgetProgram)
		accountCount++

		// add fee
		require.NoError(t, setter(tx, builder(1)))

		// accounts should not have changed
		require.Len(t, tx.Message.AccountKeys, accountCount)
		require.Len(t, tx.Message.Instructions, 2)
		i := getIndex(len(tx.Message.Instructions))
		require.Equal(t, ComputeBudgetProgram, tx.Message.AccountKeys[tx.Message.Instructions[i].ProgramIDIndex])
		data, err := builder(1).Data()
		require.NoError(t, err)
		require.Equal(t, data, []byte(tx.Message.Instructions[i].Data))
	})

	// // not a valid test, account must exist for tx to be added
	// t.Run("noAccount_feeExists", func(t *testing.T) {})

	t.Run("exists_unknownOrder", func(t *testing.T) {
		t.Parallel()
		// build base tx (no fee)
		tx, err := solana.NewTransaction([]solana.Instruction{
			system.NewTransferInstruction(
				0,
				key.PublicKey(),
				key.PublicKey(),
			).Build(),
		}, solana.Hash{})
		require.NoError(t, err)
		transferInstruction := tx.Message.Instructions[0]

		// add fee
		require.NoError(t, setter(tx, builder(0)))

		// swap order of instructions
		tx.Message.Instructions[0], tx.Message.Instructions[1] = tx.Message.Instructions[1], tx.Message.Instructions[0]

		// after swap
		computeIndex := 0
		transferIndex := 1
		if expectFirstInstruction {
			computeIndex = 1
			transferIndex = 0
		}

		require.Equal(t, transferInstruction, tx.Message.Instructions[transferIndex])
		oldComputeInstruction := tx.Message.Instructions[computeIndex]
		accountCount := len(tx.Message.AccountKeys)

		// set fee with existing fee instruction
		require.NoError(t, setter(tx, builder(100)))
		require.Equal(t, transferInstruction, tx.Message.Instructions[transferIndex]) // transfer should not have been touched
		require.NotEqual(t, oldComputeInstruction, tx.Message.Instructions[computeIndex])
		require.Len(t, tx.Message.AccountKeys, accountCount)
		require.Len(t, tx.Message.Instructions, 2) // instruction count did not change
		data, err := builder(100).Data()
		require.NoError(t, err)
		require.Equal(t, data, []byte(tx.Message.Instructions[computeIndex].Data))
	})

	t.Run("with_lookuptables", func(t *testing.T) {
		t.Parallel()

		// build base tx (no fee)
		tx, err := solana.NewTransaction([]solana.Instruction{
			system.NewTransferInstruction(
				1,
				solana.PublicKey{1},
				solana.PublicKey{2},
			).Build(),
			token.NewTransferInstruction(
				uint64(1),
				solana.PublicKey{11},
				solana.PublicKey{12},
				solana.PublicKey{13},
				[]solana.PublicKey{
					{14},
				},
			).Build(),
		},
			solana.Hash{},
			solana.TransactionAddressTables(map[solana.PublicKey]solana.PublicKeySlice{
				{}: {solana.PublicKey{1}, solana.PublicKey{2}, solana.PublicKey{11}, solana.PublicKey{12}, solana.PublicKey{13}, solana.PublicKey{14}},
			}),
		)
		require.NoError(t, err)

		// check current account indices
		require.Len(t, tx.Message.Instructions, 2)
		require.Equal(t, []uint16{0, 4}, tx.Message.Instructions[0].Accounts)
		require.Equal(t, []uint16{5, 6, 7, 1}, tx.Message.Instructions[1].Accounts)
		require.Len(t, tx.Message.AccountKeys, 4)

		// add fee
		require.NoError(t, setter(tx, builder(0)))

		// evaluate
		require.Len(t, tx.Message.Instructions, 3)
		computeUnitIndex := getIndex(len(tx.Message.Instructions))
		transferIndex := 0
		if computeUnitIndex == transferIndex {
			transferIndex = 1
		}
		require.Len(t, tx.Message.AccountKeys, 5)
		require.Equal(t, uint16(4), tx.Message.Instructions[computeUnitIndex].ProgramIDIndex)
		require.Equal(t, []uint16{0, 5}, tx.Message.Instructions[transferIndex].Accounts)
		require.Equal(t, []uint16{6, 7, 8, 1}, tx.Message.Instructions[transferIndex+1].Accounts)
	})
}

func TestParse(t *testing.T) {
	t.Run("ComputeUnitPrice", func(t *testing.T) {
		t.Parallel()
		testParse(t, func(v uint) ComputeUnitPrice {
			return ComputeUnitPrice(v)
		}, ParseComputeUnitPrice)
	})
	t.Run("ComputeUnitLimit", func(t *testing.T) {
		t.Parallel()
		testParse(t, func(v uint) ComputeUnitLimit {
			return ComputeUnitLimit(v) //nolint:gosec // G115
		}, ParseComputeUnitLimit)
	})
}

func testParse[V instruction](t *testing.T, builder func(uint) V, parser func([]byte) (V, error)) {
	data, err := builder(100).Data()
	require.NoError(t, err)

	v, err := parser(data)
	require.NoError(t, err)
	require.Equal(t, builder(100), v)

	_, err = parser([]byte{})
	require.ErrorContains(t, err, "invalid length")
	tooLong := [10]byte{}
	_, err = parser(tooLong[:])
	require.ErrorContains(t, err, "invalid length")

	invalidData := data
	invalidData[0] = uint8(InstructionRequestHeapFrame)
	_, err = parser(invalidData)
	require.ErrorContains(t, err, fmt.Sprintf("not %s identifier", builder(0).Selector()))
}
