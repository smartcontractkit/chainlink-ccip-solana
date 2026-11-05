package utils

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-testing-framework/framework/components/blockchain"
)

func TestResolveSolanaEVMLanes(t *testing.T) {
	sol := &blockchain.Input{Type: "solana", ChainID: "22222222222222222222222222222222222222222222"}
	fuji := &blockchain.Input{Type: "anvil", ChainID: "43113"}
	sepolia := &blockchain.Input{Type: "anvil", ChainID: "11155111"}
	fujiSel := chainsel.AVALANCHE_TESTNET_FUJI.Selector
	sepoliaSel := chainsel.ETHEREUM_TESTNET_SEPOLIA.Selector

	t.Run("one EVM chain and no variable", func(t *testing.T) {
		t.Setenv("TARGET_EVM_SELECTOR", "")
		solSel, evmSel, err := ResolveSolanaEVMLanes([]*blockchain.Input{fuji, sol})
		require.NoError(t, err)
		require.Equal(t, uint64(12463857294658392847), solSel)
		require.Equal(t, fujiSel, evmSel)
	})

	t.Run("several EVM chains and no variable", func(t *testing.T) {
		t.Setenv("TARGET_EVM_SELECTOR", "")
		_, _, err := ResolveSolanaEVMLanes([]*blockchain.Input{fuji, sol, sepolia})
		require.ErrorContains(t, err, "the config has several EVM chains")
		require.ErrorContains(t, err, "set TARGET_EVM_SELECTOR")
	})

	t.Run("valid variable", func(t *testing.T) {
		t.Setenv("TARGET_EVM_SELECTOR", strconv.FormatUint(sepoliaSel, 10))
		_, evmSel, err := ResolveSolanaEVMLanes([]*blockchain.Input{fuji, sol, sepolia})
		require.NoError(t, err)
		require.Equal(t, sepoliaSel, evmSel)
	})

	t.Run("variable not in manifest", func(t *testing.T) {
		t.Setenv("TARGET_EVM_SELECTOR", strconv.FormatUint(sepoliaSel, 10))
		_, _, err := ResolveSolanaEVMLanes([]*blockchain.Input{fuji, sol})
		require.ErrorContains(t, err, "is not in the manifest EVM selectors")
	})

	t.Run("unparsable variable", func(t *testing.T) {
		t.Setenv("TARGET_EVM_SELECTOR", "not-a-selector")
		_, _, err := ResolveSolanaEVMLanes([]*blockchain.Input{fuji, sol})
		require.ErrorContains(t, err, "is not a valid chain selector")
	})

	t.Run("no EVM chain", func(t *testing.T) {
		t.Setenv("TARGET_EVM_SELECTOR", "")
		_, _, err := ResolveSolanaEVMLanes([]*blockchain.Input{sol})
		require.ErrorContains(t, err, "need at least one EVM chain")
	})

	t.Run("two Solana chains", func(t *testing.T) {
		t.Setenv("TARGET_EVM_SELECTOR", "")
		_, _, err := ResolveSolanaEVMLanes([]*blockchain.Input{sol, sol, fuji})
		require.ErrorContains(t, err, "need exactly one Solana chain")
	})
}
