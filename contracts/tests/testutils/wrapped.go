package testutils

import (
	"context"
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/fees"
)

// this files includes wrapped methods to be used in testing without additional error checks
// this is used to keep a consistent interface to introduce less code churn in the tests

func SendAndConfirm(ctx context.Context, t *testing.T, rpcClient *rpc.Client, instructions []solana.Instruction, signer solana.PrivateKey, commitment rpc.CommitmentType, opts ...common.TxModifier) *rpc.GetTransactionResult {
	res, err := common.SendAndConfirm(ctx, rpcClient, instructions, signer, commitment, opts...)
	require.NoError(t, err)

	return res
}

func SendAndConfirmWithLookupTables(ctx context.Context, t *testing.T, rpcClient *rpc.Client, instructions []solana.Instruction,
	signer solana.PrivateKey, commitment rpc.CommitmentType, lookupTables map[solana.PublicKey]solana.PublicKeySlice, opts ...common.TxModifier,
) *rpc.GetTransactionResult {
	res, err := common.SendAndConfirmWithLookupTables(ctx, rpcClient, instructions, signer, commitment, lookupTables, opts...)
	require.NoError(t, err)

	return res
}

func SendAndFailWith(ctx context.Context, t *testing.T, rpcClient *rpc.Client, instructions []solana.Instruction,
	signer solana.PrivateKey, commitment rpc.CommitmentType, expectedErrors []string, opts ...common.TxModifier,
) *rpc.GetTransactionResult {
	res, err := common.SendAndFailWith(ctx, rpcClient, instructions, signer, commitment, expectedErrors, opts...)
	require.NoError(t, err)

	return res
}

func SendAndFailWithLookupTables(ctx context.Context, t *testing.T, rpcClient *rpc.Client, instructions []solana.Instruction,
	signer solana.PrivateKey, commitment rpc.CommitmentType, lookupTables map[solana.PublicKey]solana.PublicKeySlice, expectedErrors []string, opts ...common.TxModifier,
) *rpc.GetTransactionResult {
	res, err := common.SendAndFailWithLookupTables(ctx, rpcClient, instructions, signer, commitment, lookupTables, expectedErrors, opts...)
	require.NoError(t, err)

	return res
}

func SendAndFailWithRPCError(ctx context.Context, t *testing.T, rpcClient *rpc.Client, instructions []solana.Instruction,
	signer solana.PrivateKey, commitment rpc.CommitmentType, expectedErrors []string, opts ...common.TxModifier,
) {
	require.NoError(t, common.SendAndFailWithRPCError(ctx, rpcClient, instructions, signer, commitment, expectedErrors, opts...))
}

func SimulateTransaction(ctx context.Context, t *testing.T, rpcClient *rpc.Client, instructions []solana.Instruction, payer solana.PublicKey) *rpc.SimulateTransactionResponse {
	return SimulateTransactionWithLookupTables(ctx, t, rpcClient, instructions, payer, nil)
}

// SimulateTransactionWithLookupTables simulates instructions with lookup tables without signing them. Simulation runs with
// SigVerify off, so only the payer's public key matters — a private key was never needed here.
func SimulateTransactionWithLookupTables(ctx context.Context, t *testing.T, rpcClient *rpc.Client, instructions []solana.Instruction, payer solana.PublicKey, lookupTables map[solana.PublicKey]solana.PublicKeySlice) *rpc.SimulateTransactionResponse {
	tx, opts, err := common.BuildReadOnlySimulationTxWithLookupTables(payer, lookupTables, instructions...)
	require.NoError(t, err)

	simRes, err := rpcClient.SimulateTransactionWithOpts(ctx, tx, opts)
	require.NoError(t, err)

	return simRes
}

// GetRequiredCU measures the compute units a set of instructions consumes, by simulating them
// against the maximum CU limit so the run isn't truncated. The builder already prepends that
// limit, and simulation needs no signature.
func GetRequiredCU(ctx context.Context, t *testing.T, client *rpc.Client, ixs []solana.Instruction, payer solana.PublicKey) fees.ComputeUnitLimit {
	feeResult := SimulateTransaction(ctx, t, client, ixs, payer)
	require.Nil(t, feeResult.Value.Err)
	require.NotNil(t, feeResult.Value.UnitsConsumed)
	// maximum cu is 1_400_000, so we can safely cast to uint32
	//nolint:gosec // G115 max CU fits in uint32
	return fees.ComputeUnitLimit(*feeResult.Value.UnitsConsumed)
}

func AssertClosedAccount(ctx context.Context, t *testing.T, solanaGoClient *rpc.Client, accountKey solana.PublicKey, commitment rpc.CommitmentType) {
	isClosed := common.IsClosedAccount(ctx, solanaGoClient, accountKey, commitment)
	require.True(t, isClosed)
}

func MustMarshalBorsh(t *testing.T, v any) []byte {
	bz, err := bin.MarshalBorsh(v)
	require.NoError(t, err)
	return bz
}

func MustSerializeExtraArgs(t *testing.T, data any, tag string) []byte {
	b, err := ccip.SerializeExtraArgs(data, tag)
	require.NoError(t, err)
	return b
}

func MustDeserializeExtraArgs[A any](t *testing.T, obj A, data []byte, tag string) A {
	require.NoError(t, ccip.DeserializeExtraArgs(obj, data, tag))
	return obj
}
