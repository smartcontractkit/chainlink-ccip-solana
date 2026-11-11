package changesets

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
)

func TestExtendLookupTableVerify(t *testing.T) {
	sel := chainsel.SOLANA_DEVNET.Selector
	env := cldf.Environment{BlockChains: cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{
		sel: cldf_solana.Chain{Selector: sel},
	})}
	table := solana.NewWallet().PublicKey().String()
	entry := solana.NewWallet().PublicKey().String()
	input := func(table string, entries ...string) ExtendLookupTableChangesetInput {
		return ExtendLookupTableChangesetInput{ChainSelector: sel, LookupTable: table, Entries: entries}
	}
	many := make([]string, 257)
	for i := range many {
		many[i] = solana.NewWallet().PublicKey().String()
	}

	require.NoError(t, extendLookupTableVerify(env, input(table, entry)))
	got, err := parseExtendLookupTableInput(input(table, entry))
	require.NoError(t, err)
	require.Equal(t, []solana.PublicKey{solana.MustPublicKeyFromBase58(entry)}, got.Entries)

	require.ErrorContains(t, extendLookupTableVerify(env, input("not-a-key", entry)), "invalid lookup table")
	require.ErrorContains(t, extendLookupTableVerify(env, input(solana.PublicKey{}.String(), entry)), "zero key")
	require.ErrorContains(t, extendLookupTableVerify(env, input(table)), "at least one entry")
	require.ErrorContains(t, extendLookupTableVerify(env, input(table, "not-a-key")), "invalid entry")
	require.ErrorContains(t, extendLookupTableVerify(env, input(table, entry, entry)), "duplicate entry")
	require.ErrorContains(t, extendLookupTableVerify(env, input(table, many...)), "exceed the 256")
	require.ErrorContains(t, extendLookupTableVerify(env, ExtendLookupTableChangesetInput{
		ChainSelector: chainsel.SOLANA_MAINNET.Selector, LookupTable: table, Entries: []string{entry},
	}), "does not exist")
}

func TestExtendOfframpLookupTableVerify(t *testing.T) {
	sel := chainsel.SOLANA_DEVNET.Selector
	env := cldf.Environment{BlockChains: cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{
		sel: cldf_solana.Chain{Selector: sel},
	})}
	require.NoError(t, extendOfframpLookupTableVerify(env, ExtendOfframpLookupTableChangesetInput{ChainSelector: sel}))
	require.ErrorContains(t, extendOfframpLookupTableVerify(env, ExtendOfframpLookupTableChangesetInput{ChainSelector: chainsel.SOLANA_MAINNET.Selector}), "does not exist")
}
