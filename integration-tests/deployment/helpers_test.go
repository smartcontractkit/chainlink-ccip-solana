package deployment

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	solcommon "github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/common"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
)

// newLocalChain deploys all programs to a local validator and returns a chain whose deployer is
// testutils.Admin, together with the number of transactions the chain has sent.
func newLocalChain(t *testing.T) (cldf_solana.Chain, *int) {
	ctx := t.Context()
	admin := testutils.Admin
	client := testutils.DeployAllPrograms(t, testutils.PathToAnchorConfig, admin)
	require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{admin}, client))

	sent := new(int)
	return cldf_solana.Chain{
		Selector:    124615329519749607, // solana-mainnet
		Client:      client,
		DeployerKey: &admin,
		Confirm: func(ixs []solana.Instruction, opts ...solcommon.TxModifier) error {
			*sent++
			_, err := solcommon.SendAndConfirm(ctx, client, ixs, admin, rpc.CommitmentConfirmed, opts...)
			return err
		},
	}, sent
}

// run executes op in a new bundle, because a bundle returns the cached report for the same input.
func run[IN any](t *testing.T, chain cldf_solana.Chain, op *operations.Operation[IN, sequences.OnChainOutput, cldf_solana.Chain], input IN) (sequences.OnChainOutput, error) {
	bundle := operations.NewBundle(t.Context, logger.Test(t), operations.NewMemoryReporter())
	report, err := operations.ExecuteOperation(bundle, op, chain, input)
	return report.Output, err
}

// exec is run for callers that only check the error.
func exec[IN any](t *testing.T, chain cldf_solana.Chain, op *operations.Operation[IN, sequences.OnChainOutput, cldf_solana.Chain], input IN) error {
	_, err := run(t, chain, op, input)
	return err
}
