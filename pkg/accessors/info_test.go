package accessors

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
)

func TestInfo_Empty(t *testing.T) {
	t.Parallel()
	require.True(t, Info{}.Empty())
	require.False(t, Info{ChainID: "1"}.Empty())
}

func TestInfo_String(t *testing.T) {
	t.Parallel()
	info := Info{
		ChainID:         "1337",
		Type:            "sol",
		Family:          "solana",
		UniqueChainName: "solChainA",
	}
	s := info.String()
	require.Contains(t, s, "1337")
	require.Contains(t, s, "sol")
	require.Contains(t, s, "solana")
	require.Contains(t, s, "solChainA")
}

func TestInfo_TOMLDecoding(t *testing.T) {
	t.Parallel()
	input := `
[blockchain_infos."12345"]
ChainID         = "1337"
Type            = "sol"
Family          = "solana"
UniqueChainName = "solChainA"
`
	var container struct {
		BlockchainInfos map[string]Info `toml:"blockchain_infos"`
	}
	_, err := toml.Decode(input, &container)
	require.NoError(t, err)

	info, ok := container.BlockchainInfos["12345"]
	require.True(t, ok)
	require.Equal(t, "1337", info.ChainID)
	require.Equal(t, "sol", info.Type)
	require.Equal(t, "solana", info.Family)
	require.Equal(t, "solChainA", info.UniqueChainName)
}

func TestInfo_TOMLRoundtrip(t *testing.T) {
	t.Parallel()
	original := Info{
		ChainID:         "1337",
		Type:            "sol",
		Family:          "solana",
		UniqueChainName: "solChainA",
	}

	var buf strings.Builder
	require.NoError(t, toml.NewEncoder(&buf).Encode(original))

	var decoded Info
	_, err := toml.Decode(buf.String(), &decoded)
	require.NoError(t, err)
	require.Equal(t, original, decoded)
}
