package destinationreader

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-solana/pkg/solana/fees"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	solcommon "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
)

var (
	errSimulatedExecutionReverted = errors.New("simulated execute_v2 reverted")
	errSimulatedExecutionFailed   = errors.New("simulated execution failed")
)

// simulateExecutionAttempt simulates the execute_v2 this executor would send. It returns
// nil when the simulation passes or the tx is oversized (buffered path),
// errSimulatedExecutionReverted when the execute_v2 instruction fails,
// errSimulatedExecutionFailed for any other failure of the simulated tx, and any other
// error when the simulation could not run.
func (d *DestinationReader) simulateExecutionAttempt(ctx context.Context, message protocol.Message, verifierResults []protocol.VerifierResult, quorum protocol.CCVAddressInfo) error {
	d.simFeePayerMu.RLock()
	payer := d.simFeePayer
	d.simFeePayerMu.RUnlock()
	if payer.IsZero() {
		return errors.New("simulation fee payer not configured")
	}

	encodedMsg, err := message.Encode()
	if err != nil {
		return fmt.Errorf("encode message: %w", err)
	}
	ccvs, ccvData, err := quorumInputs(verifierResults, quorum)
	if err != nil {
		return fmt.Errorf("select quorum inputs: %w", err)
	}
	execInputs, err := ccip.BuildExecutionInputsV2(encodedMsg, ccvs, ccvData)
	if err != nil {
		return fmt.Errorf("build execution inputs: %w", err)
	}

	resolved, err := ccip.ResolveExecAccounts(ctx, d.rpc, d.offrampProgramID, execInputs, payer, logger.Named(d.lggr, "attempt-sim-resolution"))
	if err != nil {
		return fmt.Errorf("resolve execute accounts: %w", err)
	}

	lutMap, err := solcommon.GetAddressLookupTableMap(ctx, d.rpc, resolved.LookupTables)
	if err != nil {
		return fmt.Errorf("get address lookup tables: %w", err)
	}

	tx, err := solana.NewTransaction(
		[]solana.Instruction{resolved.Ix},
		solana.Hash{}, // replaced by RPC because ReplaceRecentBlockhash is true
		solana.TransactionPayer(payer),
		solana.TransactionAddressTables(lutMap),
	)
	if err != nil {
		return fmt.Errorf("new simulation transaction: %w", err)
	}
	// Use the same instruction changes as TXM; programs can read the instruction sysvar.
	//nolint:gosec // G115: honestComputeUnitLimit caps the value at MaxComputeUnitLimit.
	if err = fees.SetComputeUnitLimit(tx, fees.ComputeUnitLimit(honestComputeUnitLimit(message))); err != nil {
		return fmt.Errorf("set compute unit limit: %w", err)
	}
	// TXM bumps the price on retries only. The node's TXM TOML is not visible
	// here, so use the initial broadcast price. TXM defaults give a base of 0,
	// and programs can read the instruction sysvar and tell the value apart.
	if err = fees.SetComputeUnitPrice(tx, 0); err != nil {
		return fmt.Errorf("set compute unit price: %w", err)
	}
	tx.Signatures = []solana.Signature{{}}
	if size, mErr := tx.MarshalBinary(); mErr != nil {
		return fmt.Errorf("marshal simulation transaction: %w", mErr)
	} else if len(size) > solcommon.MaxSolanaTxSize {
		d.lggr.Debugw("execution attempt simulation skipped: oversized inline tx (executor would take the buffered flow)",
			"messageID", message.MustMessageID().String(), "size", len(size))
		return nil
	}

	res, err := d.rpc.SimulateTransactionWithOpts(ctx, tx, &rpc.SimulateTransactionOpts{
		ReplaceRecentBlockhash: true,
		SigVerify:              false,
		Commitment:             rpc.CommitmentConfirmed,
	})
	if err != nil {
		return fmt.Errorf("simulate execution attempt: %w", err)
	}
	if res == nil || res.Value == nil {
		return errors.New("simulate execution attempt: empty response")
	}
	if res.Value.Err != nil {
		sentinel := errSimulatedExecutionFailed
		if d.executeInstructionReverted(tx, res.Value.Err) {
			sentinel = errSimulatedExecutionReverted
		}
		return fmt.Errorf("%w: %v, logs: %q", sentinel, res.Value.Err, res.Value.Logs)
	}
	return nil
}

// quorumInputs selects the CCVs and blobs that chainlink-ccv orderCCVData gives the transmitter.
// It mirrors unexported executor.orderCCVData; replace it when upstream exports a selector.
func quorumInputs(results []protocol.VerifierResult, quorum protocol.CCVAddressInfo) ([]protocol.UnknownAddress, [][]byte, error) {
	byCCV := make(map[string][]byte, len(results))
	for _, r := range results {
		byCCV[r.VerifierDestAddress.String()] = r.CCVData
	}
	var ccvs []protocol.UnknownAddress
	var ccvData [][]byte
	for _, addr := range quorum.RequiredCCVs {
		data, ok := byCCV[addr.String()]
		if !ok {
			return nil, nil, fmt.Errorf("no verifier result for required CCV %s", addr)
		}
		ccvs, ccvData = append(ccvs, addr), append(ccvData, data)
	}
	optional := 0
	for _, addr := range quorum.OptionalCCVs {
		if optional >= int(quorum.OptionalThreshold) {
			break
		}
		if data, ok := byCCV[addr.String()]; ok {
			ccvs, ccvData = append(ccvs, addr), append(ccvData, data)
			optional++
		}
	}
	if optional < int(quorum.OptionalThreshold) {
		return nil, nil, fmt.Errorf("%d of %d optional CCVs have a verifier result", optional, quorum.OptionalThreshold)
	}
	return ccvs, ccvData, nil
}

// executeInstructionReverted reports whether simErr is an InstructionError at the
// position of an execute_v2 instruction. findExecuteV2Instructions identifies execute_v2
// by its program ID and discriminator. The check does not read the error details, so any
// instruction error at that position counts, including a compute budget shortfall caused
// by earlier instructions. It does not check whether execution reaches InProgress.
// Transaction level errors and errors of other instructions do not match.
func (d *DestinationReader) executeInstructionReverted(tx *solana.Transaction, simErr any) bool {
	errMap, ok := simErr.(map[string]any)
	if !ok {
		return false
	}
	failed, ok := errMap["InstructionError"].([]any)
	if !ok || len(failed) != 2 {
		return false
	}
	index, ok := failed[0].(float64)
	if !ok {
		return false
	}
	return slices.ContainsFunc(
		findExecuteV2Instructions(tx, d.offrampProgramID),
		func(i int) bool { return index == float64(i) },
	)
}
