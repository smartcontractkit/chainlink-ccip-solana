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
)

func TestSetBillingTokenVerify(t *testing.T) {
	selector := chainsel.SOLANA_DEVNET.Selector
	env := cldf.Environment{BlockChains: cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{
		selector: cldf_solana.Chain{Selector: selector},
	})}
	valid := SetBillingTokenChangesetInput{
		ChainSelector: selector,
		Mint:          solana.NewWallet().PublicKey().String(),
		UsdPerToken:   "150000000000000000000000000000",
	}
	require.NoError(t, setBillingTokenVerify(env, valid))

	for name, mutate := range map[string]func(*SetBillingTokenChangesetInput){
		"bad mint":       func(i *SetBillingTokenChangesetInput) { i.Mint = "nope" },
		"bad price":      func(i *SetBillingTokenChangesetInput) { i.UsdPerToken = "1.5" },
		"zero price":     func(i *SetBillingTokenChangesetInput) { i.UsdPerToken = "0" },
		"negative time":  func(i *SetBillingTokenChangesetInput) { i.Timestamp = -1 },
		"unknown chain":  func(i *SetBillingTokenChangesetInput) { i.ChainSelector = chainsel.ETHEREUM_MAINNET.Selector },
		"mcms w/o stamp": func(i *SetBillingTokenChangesetInput) { i.MCMS = mcms.Input{Qualifier: "x"} },
	} {
		in := valid
		mutate(&in)
		require.Error(t, setBillingTokenVerify(env, in), name)
	}
}
