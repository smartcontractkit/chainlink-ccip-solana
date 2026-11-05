package sequences

import (
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	tokenapi "github.com/smartcontractkit/chainlink-ccip/deployment/tokens"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

func TestParseSourceOnRamps(t *testing.T) {
	got, err := parseSourceOnRamps([]string{"0x0102", " 0x0102 ", "0XAB"})
	require.NoError(t, err)
	require.Equal(t, [][]byte{{0x01, 0x02}, {0xab}}, got, "duplicates are dropped and input is trimmed")

	for _, bad := range [][]string{{}, {"0102"}, {"0xzz"}, {"0x"}} {
		_, err := parseSourceOnRamps(bad)
		require.Error(t, err, "input %v", bad)
	}
}

func TestSameOnRampSet(t *testing.T) {
	a := [][]byte{{1}, {2}}
	require.True(t, sameOnRampSet(a, [][]byte{{2}, {1}}), "order is ignored")
	require.False(t, sameOnRampSet(a, [][]byte{{1}}))
	require.False(t, sameOnRampSet(a, [][]byte{{1}, {3}}))
}

func TestArtificialPoolAddressRefLabel(t *testing.T) {
	pda := solana.NewWallet().PublicKey()
	got, ok := decodeArtificialPoolAddressRef(encodeArtificialPoolAddressRefLabel(pda))
	require.True(t, ok)
	require.Equal(t, pda, got)

	_, ok = decodeArtificialPoolAddressRef("other:" + pda.String())
	require.False(t, ok)
	_, ok = decodeArtificialPoolAddressRef(tokenapi.ArtificialAddressRefLabel + ":not-a-key")
	require.False(t, ok)
}

func TestTokenProgramType(t *testing.T) {
	got, err := tokenProgramType(solana.TokenProgramID)
	require.NoError(t, err)
	require.Equal(t, solcommon.SPLTokens.String(), got)

	got, err = tokenProgramType(solana.Token2022ProgramID)
	require.NoError(t, err)
	require.Equal(t, solcommon.SPL2022Tokens.String(), got)

	_, err = tokenProgramType(solana.SystemProgramID)
	require.Error(t, err)
}

func TestLatestRefByAddress(t *testing.T) {
	const sel = uint64(1)
	program := solana.NewWallet().PublicKey().String()
	ds := datastore.NewMemoryDataStore()
	for _, v := range []string{"1.6.1", "2.0.0"} {
		require.NoError(t, ds.Addresses().Add(datastore.AddressRef{
			ChainSelector: sel, Address: program, Type: "BurnMintTokenPool", Version: semver.MustParse(v), Qualifier: "q",
		}))
	}

	got, err := latestRefByAddress(ds.Seal(), sel, program)
	require.NoError(t, err)
	require.Equal(t, "2.0.0", got.Version.String())

	_, err = latestRefByAddress(ds.Seal(), sel, solana.NewWallet().PublicKey().String())
	require.Error(t, err)
}
