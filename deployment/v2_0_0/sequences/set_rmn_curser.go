package sequences

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	rmnops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/rmnremote"
)

type RMNRemoteSetCurserSequenceInput struct {
	DataStore datastore.DataStore
	Curser    solana.PublicKey
	Selector  uint64
}

var SetRMNRemoteCurser = cldf_ops.NewSequence(
	"rmn-remote:set-curser",
	semver.MustParse("2.0.0"),
	"Sets the curser on the RMNRemote contract",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input RMNRemoteSetCurserSequenceInput) (sequences.OnChainOutput, error) {
		chain, ok := chains.SolanaChains()[input.Selector]
		if !ok {
			return sequences.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not found", input.Selector)
		}

		rmnAddrBytes, err := (&SolanaAdapter{}).GetRMNRemoteAddress(input.DataStore, input.Selector)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to get RMNRemote address: %w", err)
		}

		out, err := cldf_ops.ExecuteOperation(b, rmnops.SetCurser, chain, rmnops.SetCurserInput{
			Curser:    input.Curser,
			RMNRemote: solana.PublicKeyFromBytes(rmnAddrBytes),
		})
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to set curser: %w", err)
		}
		return out.Output, nil
	},
)
