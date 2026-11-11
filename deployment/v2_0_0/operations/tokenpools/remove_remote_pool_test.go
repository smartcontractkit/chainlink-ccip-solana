package tokenpools

import (
	"bytes"
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/burnmint_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/cctp_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/lombard_token_pool"
)

func testBaseChain() burnmint_token_pool.BaseChain {
	return burnmint_token_pool.BaseChain{
		Remote: burnmint_token_pool.RemoteConfig{
			PoolAddresses: []burnmint_token_pool.RemoteAddress{{Address: []byte{1, 2}}, {Address: []byte{3}}},
			TokenAddress:  burnmint_token_pool.RemoteAddress{Address: []byte{4, 5, 6}},
			Decimals:      6,
		},
		InboundRateLimit: burnmint_token_pool.RateLimitTokenBucket{
			Tokens: 10, LastUpdated: 11,
			Cfg: burnmint_token_pool.RateLimitConfig{Enabled: true, Capacity: 100, Rate: 1},
		},
		OutboundRateLimit: burnmint_token_pool.RateLimitTokenBucket{
			Tokens: 20, LastUpdated: 21,
			Cfg: burnmint_token_pool.RateLimitConfig{Enabled: true, Capacity: 200, Rate: 2},
		},
	}
}

// The CCTP and Lombard chain configs start with a version byte, so decoding them as a BurnMint
// BaseChain directly would be off by one. Decoding with their own binding and converting the base
// must give back the same BaseChain.
func TestReencodeBorsh_ChainConfigBase(t *testing.T) {
	want := testBaseChain()

	var cctpBase cctp_token_pool.BaseChain
	require.NoError(t, reencodeBorsh(want, &cctpBase))
	cctpAccount := encodeBorsh(t, cctp_token_pool.ChainConfig{Version: 1, Base: cctpBase})
	var cctpCfg cctp_token_pool.ChainConfig
	require.NoError(t, bin.NewBorshDecoder(cctpAccount).Decode(&cctpCfg))
	var gotCctp burnmint_token_pool.BaseChain
	require.NoError(t, reencodeBorsh(cctpCfg.Base, &gotCctp))
	assert.Equal(t, want, gotCctp)

	var lombardBase lombard_token_pool.BaseChain
	require.NoError(t, reencodeBorsh(want, &lombardBase))
	lombardAccount := encodeBorsh(t, lombard_token_pool.ChainConfig{Version: 1, Base: lombardBase})
	var lombardCfg lombard_token_pool.ChainConfig
	require.NoError(t, bin.NewBorshDecoder(lombardAccount).Decode(&lombardCfg))
	var gotLombard burnmint_token_pool.BaseChain
	require.NoError(t, reencodeBorsh(lombardCfg.Base, &gotLombard))
	assert.Equal(t, want, gotLombard)

	// Guard: the BurnMint binding alone misreads a CCTP chain config.
	var misread burnmint_token_pool.BaseChain
	if err := bin.NewBorshDecoder(cctpAccount).Decode(&misread); err == nil {
		assert.NotEqual(t, want, misread)
	}
}

func TestReencodeBorsh_RemoteConfig(t *testing.T) {
	want := testBaseChain().Remote

	var cctpCfg cctp_token_pool.RemoteConfig
	require.NoError(t, reencodeBorsh(want, &cctpCfg))
	assert.Equal(t, want.Decimals, cctpCfg.Decimals)
	require.Len(t, cctpCfg.PoolAddresses, 2)
	assert.Equal(t, want.PoolAddresses[1].Address, cctpCfg.PoolAddresses[1].Address)

	var lombardCfg lombard_token_pool.RemoteConfig
	require.NoError(t, reencodeBorsh(want, &lombardCfg))
	assert.Equal(t, want.TokenAddress.Address, lombardCfg.TokenAddress.Address)
}

func encodeBorsh(t *testing.T, v any) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, bin.NewBorshEncoder(&buf).Encode(v))
	return buf.Bytes()
}
