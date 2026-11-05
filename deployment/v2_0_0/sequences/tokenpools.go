package sequences

import (
	"fmt"

	"github.com/gagliardetto/solana-go"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
)

// UpdateSelfServedAllowedInput identifies a Solana token pool program and the desired self-served flag.
type UpdateSelfServedAllowedInput struct {
	ChainSelector     uint64
	TokenPoolRef      datastore.AddressRef
	SelfServedAllowed bool
}

// UpdateSelfServedAllowed enables or disables self-served pool initialization on a Solana token pool global config.
// It dispatches to the BurnMint or LockRelease operation based on the pool type.
var UpdateSelfServedAllowed = operations.NewSequence(
	"UpdateSelfServedAllowed",
	solcommon.Version2_0_0,
	"Enables or disables self-served pool initialization on a Solana token pool",
	func(b operations.Bundle, chains cldf_chain.BlockChains, input UpdateSelfServedAllowedInput) (sequences.OnChainOutput, error) {
		chain := chains.SolanaChains()[input.ChainSelector]

		var op *operations.Operation[tokenpoolops.SelfServedAllowedInput, sequences.OnChainOutput, cldf_solana.Chain]
		switch input.TokenPoolRef.Type.String() {
		case common_utils.BurnMintTokenPool.String():
			op = tokenpoolops.UpdateSelfServedAllowedBurnMint
		case common_utils.LockReleaseTokenPool.String():
			op = tokenpoolops.UpdateSelfServedAllowedLockRelease
		default:
			return sequences.OnChainOutput{}, unsupportedTokenPoolTypeError(input.TokenPoolRef.Type.String())
		}

		tokenPool, err := solana.PublicKeyFromBase58(input.TokenPoolRef.Address)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("invalid token pool address %q: %w", input.TokenPoolRef.Address, err)
		}

		report, err := operations.ExecuteOperation(b, op, chain, tokenpoolops.SelfServedAllowedInput{
			TokenPool:         tokenPool,
			SelfServedAllowed: input.SelfServedAllowed,
		})

		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to update self served allowed for token pool: %w", err)
		}
		return report.Output, nil
	},
)
