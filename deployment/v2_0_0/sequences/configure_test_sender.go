package sequences

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"

	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	mcmstypes "github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	testsenderops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testsender"
)

// TestSenderChainConfig is a destination chain to enable on a deployed sender. ExtraArgs is the
// hex blob the sender stores, already built for the sender version.
type TestSenderChainConfig struct {
	DestChainSelector uint64
	Recipient         string
	ExtraArgs         string
}

type ConfigureTestSenderInput struct {
	ChainSelector     uint64
	ExistingAddresses []datastore.AddressRef
	SenderQualifier   string
	// SenderVersion is testsenderops.Version for the v2 sender and testsenderops.LegacyVersion for v1.
	SenderVersion *semver.Version
	DestChains    []TestSenderChainConfig
	// TokenMints and CcipSenderLamports are as in DeployV2TestSenderInput.
	TokenMints         []string
	CcipSenderLamports uint64
}

// ConfigureTestSender enables destination chains on an already initialized sender and provisions
// its ccip_sender PDA. The chain configs are signed by the state owner, so they come back as MCMS
// batch operations when the timelock owns the sender. The PDA provisioning is permissionless and
// always sent directly.
var ConfigureTestSender = cldf_ops.NewSequence(
	"configure-test-sender",
	testsenderops.Version,
	"Enables destination chains on a deployed TestSender and provisions its ccip_sender PDA",
	func(b cldf_ops.Bundle, chains cldf_chain.BlockChains, input ConfigureTestSenderInput) (seq_core.OnChainOutput, error) {
		solChain, ok := chains.SolanaChains()[input.ChainSelector]
		if !ok {
			return seq_core.OnChainOutput{}, fmt.Errorf("solana chain with selector %d not found", input.ChainSelector)
		}
		if input.SenderQualifier == "" {
			return seq_core.OnChainOutput{}, errors.New("sender qualifier is required")
		}
		sender, err := solcommon.PubkeyFromRefs(input.ExistingAddresses, datastore.NewAddressRefKey(
			input.ChainSelector,
			datastore.ContractType(testsenderops.ContractType),
			input.SenderVersion,
			input.SenderQualifier,
		))
		if err != nil {
			return seq_core.OnChainOutput{}, fmt.Errorf("resolve sender: %w", err)
		}
		initialized, err := testsenderops.IsInitialized(b.GetContext(), solChain, sender, input.SenderVersion)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}
		if !initialized {
			return seq_core.OnChainOutput{}, fmt.Errorf("sender %s is not initialized: execute its initialize proposal first", sender)
		}
		mints, err := resolveTestSenderMints(b, solChain, input.TokenMints)
		if err != nil {
			return seq_core.OnChainOutput{}, err
		}

		if err = provisionTestSender(b, solChain, sender, input.SenderVersion, mints, input.CcipSenderLamports); err != nil {
			return seq_core.OnChainOutput{}, err
		}
		var batchOps []mcmstypes.BatchOperation
		for _, dest := range input.DestChains {
			report, opErr := cldf_ops.ExecuteOperation(b, testsenderops.SetChainConfig, solChain, testsenderops.SetChainConfigParams{
				Sender:            sender,
				DestChainSelector: dest.DestChainSelector,
				Recipient:         dest.Recipient,
				ExtraArgs:         dest.ExtraArgs,
				SenderVersion:     input.SenderVersion,
			})
			if opErr != nil {
				return seq_core.OnChainOutput{}, fmt.Errorf("set sender chain config for dest chain %d: %w", dest.DestChainSelector, opErr)
			}
			batchOps = append(batchOps, report.Output.BatchOps...)
		}
		return seq_core.OnChainOutput{BatchOps: batchOps}, nil
	},
)
