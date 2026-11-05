package destinationreader

import (
	"context"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/protocol"
	solconfig "github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"
)

type orderCaptureClient struct {
	RPCClient
	last *solana.Transaction
}

func (c *orderCaptureClient) SimulateTransactionWithOpts(ctx context.Context, tx *solana.Transaction, opts *rpc.SimulateTransactionOpts) (*rpc.SimulateTransactionResponse, error) {
	c.last = tx
	return c.RPCClient.SimulateTransactionWithOpts(ctx, tx, opts)
}

func TestSimulationMatchesTxmInstructionOrder(t *testing.T) {
	fx := newSimFixture(t, protocol.ChainSelector(9837465928374658293))
	okResolve := simResponse(simReturnLog(t, testOfframpProgramID, marshalAnchor(t, resolveDone())), nil)
	client, _ := newSimClient(t, okResolve, simResponse(nil, nil))
	reader := newReaderWithRPC(t, client, true)
	capture := &orderCaptureClient{RPCClient: reader.rpc}
	reader.rpc = capture
	err := reader.simulateExecutionAttempt(context.Background(), fx.msg, []protocol.VerifierResult{fx.vResultA}, protocol.CCVAddressInfo{RequiredCCVs: []protocol.UnknownAddress{fx.ccvA}})
	require.NoError(t, err)
	ix := solana.NewInstruction(testOfframpProgramID, solana.AccountMetaSlice{solana.Meta(reader.simFeePayer).SIGNER().WRITE()}, []byte{1})
	actual, err := solana.NewTransaction([]solana.Instruction{ix}, solana.Hash{}, solana.TransactionPayer(reader.simFeePayer))
	require.NoError(t, err)
	//nolint:gosec // G115: honestComputeUnitLimit caps the value at MaxComputeUnitLimit.
	require.NoError(t, fees.SetComputeUnitLimit(actual, fees.ComputeUnitLimit(honestComputeUnitLimit(fx.msg))))
	// The expected price is TXM's initial broadcast price, not a hardcoded
	// value: the default estimator mode is fixed, so the base is
	// ComputeUnitPriceDefault, and the initial broadcast has no bumps.
	var txmCfg solconfig.TOMLConfig
	txmCfg.SetDefaults()
	txmPrice := fees.ComputeUnitPrice(fees.CalculateFee(
		txmCfg.ComputeUnitPriceDefault(),
		txmCfg.ComputeUnitPriceMax(),
		txmCfg.ComputeUnitPriceMin(),
		0,
	))
	require.NoError(t, fees.SetComputeUnitPrice(actual, txmPrice))
	label := func(tx *solana.Transaction, index int) string {
		instruction := tx.Message.Instructions[index]
		program := tx.Message.AccountKeys[instruction.ProgramIDIndex]
		if program == computebudget.ProgramID {
			return "compute:" + instruction.Data.String()
		}
		return "execute"
	}
	require.Len(t, capture.last.Message.Instructions, len(actual.Message.Instructions))
	for i := range actual.Message.Instructions {
		got, want := label(capture.last, i), label(actual, i)
		require.Equal(t, want, got, "instruction %d", i)
	}
}
