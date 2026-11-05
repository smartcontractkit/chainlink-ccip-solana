package changesets

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	cvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
	testsenderops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testsender"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

// These changesets configure a sender that is already deployed and initialized: they enable its
// destination chains and provision its ccip_sender PDA (token accounts and, optionally, lamports).
// Run them once the sender's initialize proposal has executed. The chain configs are signed by the
// state owner, so when the timelock owns the sender they come back as an MCMS proposal built from
// MCMS; the PDA provisioning is permissionless and always sent directly.

type ConfigureV2TestSenderChangesetInput struct {
	ChainSelector   uint64                            `json:"chainSelector"`
	SenderQualifier string                            `json:"senderQualifier"`
	DestChains      []sequences.V2TestSenderDestChain `json:"destChains"`
	// TokenMints are the base58 mints to create ccip_sender token accounts for: every fee token
	// (wrapped SOL for native fees included) and every transferred token.
	TokenMints []string `json:"tokenMints"`
	// CcipSenderLamports, when non-zero, is the balance the ccip_sender PDA is topped up to.
	CcipSenderLamports uint64     `json:"ccipSenderLamports"`
	MCMS               mcms.Input `json:"mcms"`
}

func ConfigureV2TestSenderChangeset() cldf.ChangeSetV2[ConfigureV2TestSenderChangesetInput] {
	return cldf.CreateChangeSet(configureV2TestSenderApply, configureV2TestSenderVerify)
}

func configureV2TestSenderVerify(env cldf.Environment, input ConfigureV2TestSenderChangesetInput) error {
	for _, dest := range input.DestChains {
		if err := verifyTestSenderDestChain(dest.DestChainSelector, dest.Recipient); err != nil {
			return err
		}
	}
	return verifyConfigureTestSender(env, input.ChainSelector, input.SenderQualifier, len(input.DestChains), input.TokenMints, input.CcipSenderLamports, input.MCMS)
}

func configureV2TestSenderApply(e cldf.Environment, input ConfigureV2TestSenderChangesetInput) (cldf.ChangesetOutput, error) {
	addresses, err := e.DataStore.Addresses().Fetch()
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("fetch address refs: %w", err)
	}
	destChains := make([]sequences.TestSenderChainConfig, len(input.DestChains))
	for i, dest := range input.DestChains {
		ccvs, ccvErr := cvops.ResolveCommitteeVerifiers(addresses, input.ChainSelector, dest.CCVQualifiers)
		if ccvErr != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("resolve CCVs for dest chain %d: %w", dest.DestChainSelector, ccvErr)
		}
		args, argsErr := sequences.V2TestSenderExtraArgs(dest.GasLimit, dest.Finality, ccvs)
		if argsErr != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("dest chain %d: %w", dest.DestChainSelector, argsErr)
		}
		destChains[i] = sequences.TestSenderChainConfig{DestChainSelector: dest.DestChainSelector, Recipient: dest.Recipient, ExtraArgs: args}
	}
	return configureTestSender(e, addresses, sequences.ConfigureTestSenderInput{
		ChainSelector:      input.ChainSelector,
		SenderQualifier:    input.SenderQualifier,
		SenderVersion:      testsenderops.Version,
		DestChains:         destChains,
		TokenMints:         input.TokenMints,
		CcipSenderLamports: input.CcipSenderLamports,
	}, input.MCMS)
}

type ConfigureLegacyTestSenderChangesetInput struct {
	ChainSelector   uint64                                `json:"chainSelector"`
	SenderQualifier string                                `json:"senderQualifier"`
	DestChains      []sequences.LegacyTestSenderDestChain `json:"destChains"`
	// TokenMints are the base58 mints to create ccip_sender token accounts for. The fee mint
	// (wrapped SOL for native fees) is required before the first send.
	TokenMints []string `json:"tokenMints"`
	// CcipSenderLamports, when non-zero, is the balance the ccip_sender PDA is topped up to, which
	// native SOL fees are paid from.
	CcipSenderLamports uint64     `json:"ccipSenderLamports"`
	MCMS               mcms.Input `json:"mcms"`
}

func ConfigureLegacyTestSenderChangeset() cldf.ChangeSetV2[ConfigureLegacyTestSenderChangesetInput] {
	return cldf.CreateChangeSet(configureLegacyTestSenderApply, configureLegacyTestSenderVerify)
}

func configureLegacyTestSenderVerify(env cldf.Environment, input ConfigureLegacyTestSenderChangesetInput) error {
	for _, dest := range input.DestChains {
		if err := verifyTestSenderDestChain(dest.DestChainSelector, dest.Recipient); err != nil {
			return err
		}
		if _, err := hex.DecodeString(strings.TrimPrefix(dest.ExtraArgs, "0x")); err != nil {
			return fmt.Errorf("invalid extra args %q for destination chain %d: %w", dest.ExtraArgs, dest.DestChainSelector, err)
		}
	}
	return verifyConfigureTestSender(env, input.ChainSelector, input.SenderQualifier, len(input.DestChains), input.TokenMints, input.CcipSenderLamports, input.MCMS)
}

func configureLegacyTestSenderApply(e cldf.Environment, input ConfigureLegacyTestSenderChangesetInput) (cldf.ChangesetOutput, error) {
	addresses, err := e.DataStore.Addresses().Fetch()
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("fetch address refs: %w", err)
	}
	destChains := make([]sequences.TestSenderChainConfig, len(input.DestChains))
	for i, dest := range input.DestChains {
		destChains[i] = sequences.TestSenderChainConfig(dest)
	}
	return configureTestSender(e, addresses, sequences.ConfigureTestSenderInput{
		ChainSelector:      input.ChainSelector,
		SenderQualifier:    input.SenderQualifier,
		SenderVersion:      testsenderops.LegacyVersion,
		DestChains:         destChains,
		TokenMints:         input.TokenMints,
		CcipSenderLamports: input.CcipSenderLamports,
	}, input.MCMS)
}

func verifyConfigureTestSender(env cldf.Environment, chainSelector uint64, qualifier string, destChains int, tokenMints []string, lamports uint64, mcmsInput mcms.Input) error {
	if err := verifyTestSenderChain(env, chainSelector, qualifier); err != nil {
		return err
	}
	if destChains == 0 && len(tokenMints) == 0 && lamports == 0 {
		return errors.New("nothing to configure: set destChains, tokenMints or ccipSenderLamports")
	}
	for _, mint := range tokenMints {
		if _, err := solana.PublicKeyFromBase58(mint); err != nil {
			return fmt.Errorf("invalid token mint %q: %w", mint, err)
		}
	}
	return validateMCMSInput(env.Logger, mcmsInput)
}

func configureTestSender(e cldf.Environment, addresses []datastore.AddressRef, input sequences.ConfigureTestSenderInput, mcmsInput mcms.Input) (cldf.ChangesetOutput, error) {
	input.ExistingAddresses = addresses
	report, err := cldf_ops.ExecuteSequence(e.OperationsBundle, sequences.ConfigureTestSender, e.BlockChains, input)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("configure test sender: %w", err)
	}
	return buildProposalOutput(e, mcmsInput, append([]cldf_ops.Report[any, any](nil), report.ExecutionReports...), report.Output.BatchOps)
}
