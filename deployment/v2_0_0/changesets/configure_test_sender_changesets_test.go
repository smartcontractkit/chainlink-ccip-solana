package changesets

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	mcms_types "github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

func TestBuildProposalOutputRequiresMCMSWhenBatched(t *testing.T) {
	sel := chainsel.SOLANA_DEVNET.Selector
	batched := []mcms_types.BatchOperation{{
		ChainSelector: mcms_types.ChainSelector(sel),
		Transactions:  []mcms_types.Transaction{{}},
	}}
	_, err := buildProposalOutput(cldf.Environment{}, mcms.Input{}, nil, batched)
	require.ErrorContains(t, err, "mcms input is required")

	out, err := buildProposalOutput(cldf.Environment{}, mcms.Input{}, nil, nil)
	require.NoError(t, err, "no batch operations means nothing to propose")
	require.Empty(t, out.MCMSTimelockProposals)
}

func TestConfigureTestSenderVerify(t *testing.T) {
	sel := chainsel.SOLANA_DEVNET.Selector
	dest := chainsel.ETHEREUM_TESTNET_SEPOLIA.Selector
	env := cldf.Environment{BlockChains: cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{
		sel: cldf_solana.Chain{Selector: sel},
	})}
	const recipient = "0x54081602645704EE2B76FEe30E8B4d4F2D82d4E0"
	mint := solana.NewWallet().PublicKey().String()
	mcmsInput := mcms.Input{TimelockAction: mcms_types.TimelockActionSchedule}

	t.Run("v2", func(t *testing.T) {
		valid := ConfigureV2TestSenderChangesetInput{
			ChainSelector:   sel,
			SenderQualifier: "default",
			DestChains:      []sequences.V2TestSenderDestChain{{DestChainSelector: dest, Recipient: recipient, GasLimit: 200000}},
			TokenMints:      []string{mint},
			MCMS:            mcmsInput,
		}
		require.NoError(t, configureV2TestSenderVerify(env, valid))

		onlyFunding := ConfigureV2TestSenderChangesetInput{ChainSelector: sel, SenderQualifier: "default", CcipSenderLamports: 1}
		require.NoError(t, configureV2TestSenderVerify(env, onlyFunding), "provisioning alone is a valid configuration")

		noQualifier := valid
		noQualifier.SenderQualifier = ""
		require.ErrorContains(t, configureV2TestSenderVerify(env, noQualifier), "sender qualifier is required")

		nothing := ConfigureV2TestSenderChangesetInput{ChainSelector: sel, SenderQualifier: "default"}
		require.ErrorContains(t, configureV2TestSenderVerify(env, nothing), "nothing to configure")

		badMint := valid
		badMint.TokenMints = []string{"not-a-mint"}
		require.ErrorContains(t, configureV2TestSenderVerify(env, badMint), "invalid token mint")

		noRecipient := valid
		noRecipient.DestChains = []sequences.V2TestSenderDestChain{{DestChainSelector: dest}}
		require.ErrorContains(t, configureV2TestSenderVerify(env, noRecipient), "recipient is required")
	})

	t.Run("legacy", func(t *testing.T) {
		valid := ConfigureLegacyTestSenderChangesetInput{
			ChainSelector:   sel,
			SenderQualifier: "default",
			DestChains:      []sequences.LegacyTestSenderDestChain{{DestChainSelector: dest, Recipient: recipient}},
			TokenMints:      []string{mint},
			MCMS:            mcmsInput,
		}
		require.NoError(t, configureLegacyTestSenderVerify(env, valid), "empty extra args are valid")

		badExtraArgs := valid
		badExtraArgs.DestChains = []sequences.LegacyTestSenderDestChain{{DestChainSelector: dest, Recipient: recipient, ExtraArgs: "zz"}}
		require.ErrorContains(t, configureLegacyTestSenderVerify(env, badExtraArgs), "invalid extra args")

		wrongChain := valid
		wrongChain.ChainSelector = chainsel.SOLANA_MAINNET.Selector
		require.ErrorContains(t, configureLegacyTestSenderVerify(env, wrongChain), "does not exist")
	})
}
