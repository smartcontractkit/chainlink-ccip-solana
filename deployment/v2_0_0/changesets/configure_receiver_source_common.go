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

// accountExists reports whether the address has an on-chain account.
func accountExists(env cldf.Environment, chain cldf_solana.Chain, addr solana.PublicKey) (bool, error) {
	acc, err := chain.Client.GetAccountInfoWithOpts(env.GetContext(), addr, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentConfirmed})
	if err != nil && !errors.Is(err, rpc.ErrNotFound) {
		return false, fmt.Errorf("read account %s: %w", addr, err)
	}
	return acc != nil && acc.Value != nil, nil
}

// senderApprovals identifies the remote senders a changeset approves on a receiver.
type senderApprovals struct {
	Receiver            solana.PublicKey
	SourceChainSelector uint64
	Senders             []string
	ReceiverVersion     *semver.Version
}

// validate checks that every sender decodes and derives its approved-sender PDAs.
func (a senderApprovals) validate() error {
	for _, sender := range a.Senders {
		if _, err := sequences.SenderApprovals(a.Receiver, a.SourceChainSelector, sender, a.ReceiverVersion); err != nil {
			return fmt.Errorf("derive approved sender PDAs for %q: %w", sender, err)
		}
	}
	return nil
}

// pending returns the approvals whose PDA does not exist yet. ApproveSender creates that PDA, so
// approving an existing one fails with account-already-in-use.
func (a senderApprovals) pending(env cldf.Environment, chain cldf_solana.Chain) ([]sequences.SenderApproval, error) {
	var pending []sequences.SenderApproval
	for _, sender := range a.Senders {
		approvals, err := sequences.SenderApprovals(a.Receiver, a.SourceChainSelector, sender, a.ReceiverVersion)
		if err != nil {
			return nil, fmt.Errorf("derive approved sender PDAs for %q: %w", sender, err)
		}
		for _, approval := range approvals {
			exists, err := accountExists(env, chain, approval.PDA)
			if err != nil {
				return nil, fmt.Errorf("approved sender %q: %w", sender, err)
			}
			if exists {
				env.Logger.Infof("remote sender %s is already approved on receiver %s (PDA %s), skipping", sender, a.Receiver, approval.PDA)
				continue
			}
			pending = append(pending, approval)
		}
	}
	return pending, nil
}
