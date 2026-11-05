package changesets

import (
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/changesets"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/lombard_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	lvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/lombardverifier"
)

// LombardTokenRef identifies a deployed token mint by its datastore qualifier
// and mint type (SPLTokens or SPL2022Tokens).
type LombardTokenRef struct {
	TokenQualifier string `json:"tokenQualifier" yaml:"tokenQualifier"`
	MintType       string `json:"mintType" yaml:"mintType"`
}

// ConfigureLombardVerifierInput adds local token mints to the Lombard
// verifier's supported-token list (set_verifier_supported_tokens on the
// LombardTokenPool program, signed by the verifier config owner).
type ConfigureLombardVerifierInput struct {
	ChainSelector uint64            `json:"chainSelector" yaml:"chainSelector"`
	Tokens        []LombardTokenRef `json:"tokens" yaml:"tokens"`
	MCMS          mcms.Input        `json:"mcms" yaml:"mcms"`
}

// ConfigureLombardVerifier wraps lvops.SetSupportedTokens for the durable
// pipeline registry. Deploy calls SetSupportedTokens for the tokens it knows
// about; this changeset covers tokens onboarded after deploy. The on-chain
// instruction is a set-union, so reruns are idempotent. When the verifier
// owner is not the deployer, the instruction is returned as an MCMS batch
// instead of being sent; input.MCMS drives proposal assembly.
func ConfigureLombardVerifier() cldf.ChangeSetV2[ConfigureLombardVerifierInput] {
	return cldf.CreateChangeSet(configureLombardVerifierApply, configureLombardVerifierVerify)
}

func configureLombardVerifierVerify(env cldf.Environment, input ConfigureLombardVerifierInput) error {
	if err := cldf.IsValidChainSelector(input.ChainSelector); err != nil {
		return fmt.Errorf("invalid chain selector %d: %w", input.ChainSelector, err)
	}
	if !env.BlockChains.Exists(input.ChainSelector) {
		return fmt.Errorf("chain with selector %d does not exist", input.ChainSelector)
	}
	if _, ok := env.BlockChains.SolanaChains()[input.ChainSelector]; !ok {
		return fmt.Errorf("chain with selector %d is not a Solana chain", input.ChainSelector)
	}
	if len(input.Tokens) == 0 {
		return errors.New("at least one token is required")
	}
	for _, t := range input.Tokens {
		if t.TokenQualifier == "" {
			return errors.New("token qualifier must not be empty")
		}
		if t.MintType != string(solcommon.SPLTokens) && t.MintType != string(solcommon.SPL2022Tokens) {
			return fmt.Errorf("invalid mint type %q: must be %s or %s",
				t.MintType, solcommon.SPLTokens, solcommon.SPL2022Tokens)
		}
	}
	if err := validateMCMSInput(env.Logger, input.MCMS); err != nil {
		return fmt.Errorf("invalid MCMS configuration: %w", err)
	}
	return nil
}

func configureLombardVerifierApply(e cldf.Environment, input ConfigureLombardVerifierInput) (cldf.ChangesetOutput, error) {
	chain := e.BlockChains.SolanaChains()[input.ChainSelector]

	// Filter by the canonical qualifier: deploy also registers a
	// LombardVerifierResolver alias under the same chain and type.
	verifierRefs := e.DataStore.Addresses().Filter(
		datastore.AddressRefByChainSelector(input.ChainSelector),
		datastore.AddressRefByType(datastore.ContractType(lvops.ContractType)),
		datastore.AddressRefByQualifier(lvops.ContractType.String()),
	)
	if len(verifierRefs) != 1 {
		return cldf.ChangesetOutput{}, fmt.Errorf("expected 1 LombardVerifier address ref on chain %d, got %d",
			input.ChainSelector, len(verifierRefs))
	}
	verifier, err := solana.PublicKeyFromBase58(verifierRefs[0].Address)
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("parse LombardVerifier address: %w", err)
	}

	tokens := make([]lombard_token_pool.SupportedToken, 0, len(input.Tokens))
	for _, t := range input.Tokens {
		ref, findErr := solcommon.FindLatestRef(e.DataStore, datastore.AddressRef{
			Type:      datastore.ContractType(t.MintType),
			Qualifier: t.TokenQualifier,
		}, input.ChainSelector)
		if findErr != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("find token %q (%s) on chain %d: %w", t.TokenQualifier, t.MintType, input.ChainSelector, findErr)
		}
		mint, parseErr := solana.PublicKeyFromBase58(ref.Address)
		if parseErr != nil {
			return cldf.ChangesetOutput{}, fmt.Errorf("parse mint for %q: %w", t.TokenQualifier, parseErr)
		}
		tokens = append(tokens, lombard_token_pool.SupportedToken{LocalToken: mint})
	}

	report, err := cldf_ops.ExecuteOperation(e.OperationsBundle, lvops.SetSupportedTokens, chain,
		lvops.SetSupportedTokensParams{
			LombardVerifier: verifier,
			TokensToSet:     tokens,
		})
	if err != nil {
		return cldf.ChangesetOutput{}, fmt.Errorf("set supported tokens: %w", err)
	}
	// Forward the batch: with a governed (non-deployer) verifier owner the op
	// does not send, it returns the instruction in BatchOps for MCMS assembly.
	if err := requireMCMSForBatchOps(e.Logger, input.MCMS, report.Output.BatchOps); err != nil {
		return cldf.ChangesetOutput{}, err
	}
	return changesets.
		NewOutputBuilder(e, changesets.GetRegistry()).
		WithReports([]cldf_ops.Report[any, any]{report.ToGenericReport()}).
		WithBatchOps(report.Output.BatchOps).
		Build(input.MCMS)
}
