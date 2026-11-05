package sequences

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/lanes"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	fqops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
	offrampops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/offramp"
)

var DisableRemoteChainSequence = cldf_ops.NewSequence(
	"DisableRemoteChain",
	semver.MustParse("2.0.0"),
	"Disables both sending to and receiving from a remote chain on a Solana chain",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input lanes.DisableRemoteChainInput) (sequences.OnChainOutput, error) {
		var result sequences.OnChainOutput
		b.Logger.Infof("SVM Disabling remote chain %d on chain %d", input.RemoteChainSelector, input.LocalChainSelector)

		feeQuoterAddress := solana.PublicKeyFromBytes(input.FeeQuoter)
		offRampAddress := solana.PublicKeyFromBytes(input.OffRamp)

		fqOut, err := cldf_ops.ExecuteOperation(
			b,
			fqops.DisableDestChain,
			chains.SolanaChains()[input.LocalChainSelector],
			fqops.DisableDestChainParams{
				FeeQuoter:           feeQuoterAddress,
				RemoteChainSelector: input.RemoteChainSelector,
			},
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to disable dest chain on FeeQuoter: %w", err)
		}
		result.BatchOps = append(result.BatchOps, fqOut.Output.BatchOps...)

		offrampOut, err := cldf_ops.ExecuteOperation(
			b,
			offrampops.DisableSourceChain,
			chains.SolanaChains()[input.LocalChainSelector],
			offrampops.DisableSourceChainParams{
				OffRamp:             offRampAddress,
				RemoteChainSelector: input.RemoteChainSelector,
			},
		)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to disable source chain on OffRamp: %w", err)
		}
		result.BatchOps = append(result.BatchOps, offrampOut.Output.BatchOps...)

		b.Logger.Infof("Remote chain %d disabled on Solana chain %d", input.RemoteChainSelector, input.LocalChainSelector)
		return result, nil
	},
)

func (a *SolanaAdapter) DisableRemoteChain() *cldf_ops.Sequence[lanes.DisableRemoteChainInput, sequences.OnChainOutput, cldf_chain.BlockChains] {
	return DisableRemoteChainSequence
}
