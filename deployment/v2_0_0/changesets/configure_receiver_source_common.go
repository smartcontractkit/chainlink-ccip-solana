package changesets

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

func validateConfigureSourceCommon(chainSelector, sourceChainSelector uint64, receiverQualifier string, approvedRemoteSenders []string) error {
	if err := cldf.IsValidChainSelector(chainSelector); err != nil {
		return fmt.Errorf("invalid chain selector %d: %w", chainSelector, err)
	}
	if err := cldf.IsValidChainSelector(sourceChainSelector); err != nil {
		return fmt.Errorf("invalid source chain selector %d: %w", sourceChainSelector, err)
	}
	if receiverQualifier == "" {
		return errors.New("receiver qualifier is required")
	}
	if len(approvedRemoteSenders) == 0 {
		return errors.New("at least one approved remote sender is required")
	}
	return nil
}

func fetchSolanaChain(env cldf.Environment, chainSelector uint64, version *semver.Version, receiverQualifier string) (cldf_solana.Chain, []datastore.AddressRef, error) {
	solChain, ok := env.BlockChains.SolanaChains()[chainSelector]
	if !ok {
		return cldf_solana.Chain{}, nil, fmt.Errorf("chain with selector %d is not a Solana chain", chainSelector)
	}
	addresses, err := env.DataStore.Addresses().Fetch()
	if err != nil {
		return cldf_solana.Chain{}, nil, fmt.Errorf("fetch existing addresses: %w", err)
	}
	if _, err := sequences.ResolveTestReceiver(addresses, chainSelector, receiverQualifier, version); err != nil {
		return cldf_solana.Chain{}, nil, fmt.Errorf("resolve receiver (%q): %w", receiverQualifier, err)
	}
	return solChain, addresses, nil
}

// assertPDAAbsent fails if the PDA already has an on-chain account.
func assertPDAAbsent(env cldf.Environment, chain cldf_solana.Chain, pda solana.PublicKey, what string) error {
	acc, err := chain.Client.GetAccountInfoWithOpts(env.GetContext(), pda, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentConfirmed})
	if err != nil && !errors.Is(err, rpc.ErrNotFound) {
		return fmt.Errorf("read %s PDA %s: %w", what, pda, err)
	}
	if acc != nil && acc.Value != nil {
		return fmt.Errorf("%s %s already configured (PDA %s)", what, pda, pda)
	}
	return nil
}
