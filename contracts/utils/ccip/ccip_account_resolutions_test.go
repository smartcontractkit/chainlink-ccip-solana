package ccip

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckResolveAccountsRoundLimit(t *testing.T) {
	t.Parallel()

	require.NoError(t, checkResolveAccountsRoundLimit(maxResolveAccountsRounds-1))

	err := checkResolveAccountsRoundLimit(maxResolveAccountsRounds)
	require.Error(t, err)
	require.ErrorContains(t, err, "account resolution exceeded maximum rounds")
}
