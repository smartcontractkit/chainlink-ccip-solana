package sequences

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/fee_quoter"
	fqops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
)

type FeeQuoterSetTokenTransferFeeConfigSequenceInput struct {
	RemoteChainConfigs map[uint64]map[solana.PublicKey]fee_quoter.TokenTransferFeeConfig
	DataStore          datastore.DataStore
	FeeQuoter          solana.PublicKey
	Selector           uint64
}

var SetTokenTransferFeeConfig = operations.NewSequence(
	"fee-quoter:set-token-transfer-fee-config",
	semver.MustParse("2.0.0"),
	"Sets token transfer fee configs on the FeeQuoter contract",
	func(b operations.Bundle, chains cldf_chain.BlockChains, input FeeQuoterSetTokenTransferFeeConfigSequenceInput) (sequences.OnChainOutput, error) {
		var result sequences.OnChainOutput
		chain, ok := chains.SolanaChains()[input.Selector]
		if !ok {
			return sequences.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not found", input.Selector)
		}
		fqAddrBytes, err := (&SolanaAdapter{}).GetFQAddress(input.DataStore, input.Selector)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to get FeeQuoter address: %w", err)
		}
		fqPubkey := solana.PublicKeyFromBytes(fqAddrBytes)
		if fqPubkey != input.FeeQuoter {
			return sequences.OnChainOutput{}, fmt.Errorf("fee quoter address mismatch: expected %s, got %s", input.FeeQuoter, fqPubkey)
		}
		for dst, dstCfg := range input.RemoteChainConfigs {
			for tok, feeCfg := range dstCfg {
				out, err := operations.ExecuteOperation(b,
					fqops.SetTokenTransferFeeConfig,
					chain,
					fqops.SetTokenTransferFeeConfigInput{
						SrcSelector: input.Selector,
						DstSelector: dst,
						FeeQuoter:   fqPubkey,
						Config:      feeCfg,
						Token:       tok,
					},
				)
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to set token transfer fee config for token %s and destination %d: %w", tok, dst, err)
				}
				result.Addresses = append(result.Addresses, out.Output.Addresses...)
				result.BatchOps = append(result.BatchOps, out.Output.BatchOps...)
			}
		}
		return result, nil
	},
)
