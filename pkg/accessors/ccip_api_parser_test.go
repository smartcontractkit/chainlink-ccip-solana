package accessors

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/protocol"
)

func TestSolanaAccessorCCIPAPIParser(t *testing.T) {
	t.Parallel()

	a := &accessor{}
	parser, err := a.CCIPAPIParser()
	require.NoError(t, err)
	require.Same(t, a, parser)

	got, err := parser.DecodeAddress(strings.Repeat("1", 32))
	require.NoError(t, err)
	require.Equal(t, protocol.UnknownAddress(make([]byte, 32)), got)

	_, err = parser.DecodeAddress("not-base58")
	require.Error(t, err)
}
