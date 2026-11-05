package changesets

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

type DeployV2TestSenderChangesetInput struct {
	ChainSelector   uint64                            `json:"chainSelector"`
	SenderQualifier string                            `json:"senderQualifier"`
	DestChains      []sequences.V2TestSenderDestChain `json:"destChains"`
	// TokenMints are the base58 mints to create ccip_sender token accounts for: every fee token
	// (wrapped SOL for native fees included) and every transferred token.
	TokenMints []string `json:"tokenMints"`
	// CcipSenderLamports, when non-zero, is the balance the ccip_sender PDA is topped up to.
	CcipSenderLamports uint64 `json:"ccipSenderLamports"`
}

func DeployV2TestSenderChangeset() cldf.ChangeSetV2[DeployV2TestSenderChangesetInput] {
	return cldf.CreateChangeSet(deployV2TestSenderApply, deployV2TestSenderVerify)
}

func deployV2TestSenderVerify(env cldf.Environment, input DeployV2TestSenderChangesetInput) error {
	if err := verifyTestSenderChain(env, input.ChainSelector, input.SenderQualifier); err != nil {
		return err
	}
	if len(input.DestChains) == 0 {
		return errors.New("at least one destination chain is required")
	}
	for _, dest := range input.DestChains {
		if err := verifyTestSenderDestChain(dest.DestChainSelector, dest.Recipient); err != nil {
			return err
		}
	}
	return nil
}

func deployV2TestSenderApply(e cldf.Environment, input DeployV2TestSenderChangesetInput) (cldf.ChangesetOutput, error) {
	// Only this chain's refs: the deploy helper matches on type, version and qualifier alone, so a
	// sender registered on another chain would otherwise be reused here.
	addresses := e.DataStore.Addresses().Filter(datastore.AddressRefByChainSelector(input.ChainSelector))
	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.DeployV2TestSender, e.BlockChains, sequences.DeployV2TestSenderInput{
		ChainSelector:      input.ChainSelector,
		ExistingAddresses:  addresses,
		SenderQualifier:    input.SenderQualifier,
		DestChains:         input.DestChains,
		TokenMints:         input.TokenMints,
		CcipSenderLamports: input.CcipSenderLamports,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("deploy example sender: %w", err)
	}
	ds := datastore.NewMemoryDataStore()
	for _, address := range report.Output.Addresses {
		if err := ds.Addresses().Add(address); err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("add sender address to datastore: %w", err)
		}
	}
	return cldf.ChangesetOutput{
		Reports:   append([]cldf_ops.Report[any, any](nil), report.ExecutionReports...),
		DataStore: ds,
	}, nil
}

type DeployLegacyTestSenderChangesetInput struct {
	ChainSelector   uint64                                `json:"chainSelector"`
	SenderQualifier string                                `json:"senderQualifier"`
	DestChains      []sequences.LegacyTestSenderDestChain `json:"destChains"`
	// TokenMints are the base58 mints to create ccip_sender token accounts for. The fee mint
	// (wrapped SOL for native fees) is required before the first send.
	TokenMints []string `json:"tokenMints"`
	// CcipSenderLamports, when non-zero, is the balance the ccip_sender PDA is topped up to, which
	// native SOL fees are paid from.
	CcipSenderLamports uint64 `json:"ccipSenderLamports"`
}

func DeployLegacyTestSenderChangeset() cldf.ChangeSetV2[DeployLegacyTestSenderChangesetInput] {
	return cldf.CreateChangeSet(deployLegacyTestSenderApply, deployLegacyTestSenderVerify)
}

func deployLegacyTestSenderVerify(env cldf.Environment, input DeployLegacyTestSenderChangesetInput) error {
	if err := verifyTestSenderChain(env, input.ChainSelector, input.SenderQualifier); err != nil {
		return err
	}
	if len(input.DestChains) == 0 {
		return errors.New("at least one destination chain is required")
	}
	for _, dest := range input.DestChains {
		if err := verifyTestSenderDestChain(dest.DestChainSelector, dest.Recipient); err != nil {
			return err
		}
		if _, err := hex.DecodeString(strings.TrimPrefix(dest.ExtraArgs, "0x")); err != nil {
			return fmt.Errorf("invalid extra args %q for destination chain %d: %w", dest.ExtraArgs, dest.DestChainSelector, err)
		}
	}
	return nil
}

func deployLegacyTestSenderApply(e cldf.Environment, input DeployLegacyTestSenderChangesetInput) (cldf.ChangesetOutput, error) {
	// Only this chain's refs: the deploy helper matches on type, version and qualifier alone, so a
	// sender registered on another chain would otherwise be reused here.
	addresses := e.DataStore.Addresses().Filter(datastore.AddressRefByChainSelector(input.ChainSelector))
	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.DeployLegacyTestSender, e.BlockChains, sequences.DeployLegacyTestSenderInput{
		ChainSelector:      input.ChainSelector,
		ExistingAddresses:  addresses,
		SenderQualifier:    input.SenderQualifier,
		DestChains:         input.DestChains,
		TokenMints:         input.TokenMints,
		CcipSenderLamports: input.CcipSenderLamports,
	})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("deploy legacy test sender: %w", err)
	}
	ds := datastore.NewMemoryDataStore()
	for _, address := range report.Output.Addresses {
		if err := ds.Addresses().Add(address); err != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("add sender address to datastore: %w", err)
		}
	}
	return cldf.ChangesetOutput{
		Reports:   append([]cldf_ops.Report[any, any](nil), report.ExecutionReports...),
		DataStore: ds,
	}, nil
}

func verifyTestSenderChain(env cldf.Environment, chainSelector uint64, qualifier string) error {
	if err := cldf.IsValidChainSelector(chainSelector); err != nil {
		return fmt.Errorf("invalid chain selector %d: %w", chainSelector, err)
	}
	if !env.BlockChains.Exists(chainSelector) {
		return fmt.Errorf("chain with selector %d does not exist", chainSelector)
	}
	if _, ok := env.BlockChains.SolanaChains()[chainSelector]; !ok {
		return fmt.Errorf("chain with selector %d is not a Solana chain", chainSelector)
	}
	if qualifier == "" {
		return errors.New("sender qualifier is required")
	}
	return nil
}

func verifyTestSenderDestChain(destChainSelector uint64, recipient string) error {
	if err := cldf.IsValidChainSelector(destChainSelector); err != nil {
		return fmt.Errorf("invalid destination chain selector %d: %w", destChainSelector, err)
	}
	if recipient == "" {
		return fmt.Errorf("recipient is required for destination chain %d", destChainSelector)
	}
	// Same decoding SetChainConfig applies, so a malformed recipient fails before anything is
	// deployed.
	if _, err := solcommon.DecodeRemoteSender(recipient); err != nil {
		return fmt.Errorf("invalid recipient for destination chain %d: %w", destChainSelector, err)
	}
	return nil
}
