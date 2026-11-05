package sequences

import (
	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	"github.com/smartcontractkit/mcms/types"
)

// runOp executes op and appends the MCMS batches it returns to batches. Operations confirm
// directly when the deployer is the on-chain authority and return a batch otherwise, so a
// sequence that dropped the batch would silently skip the change once MCMS owns the contract.
func runOp[IN any](
	b cldf_ops.Bundle,
	op *cldf_ops.Operation[IN, seq_core.OnChainOutput, cldf_solana.Chain],
	chain cldf_solana.Chain,
	input IN,
	batches *[]types.BatchOperation,
) error {
	report, err := cldf_ops.ExecuteOperation(b, op, chain, input)
	if err != nil {
		return err
	}
	*batches = append(*batches, report.Output.BatchOps...)
	return nil
}
