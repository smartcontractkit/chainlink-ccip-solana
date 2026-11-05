package adapters

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip/deployment/fees"
)

func TestWithdrawAmountFor(t *testing.T) {
	tests := []struct {
		name            string
		amount          *big.Int
		wantTransferAll bool
		wantAmount      uint64
		wantErr         string
	}{
		{name: "nil amount withdraws everything", amount: nil, wantTransferAll: true},
		{name: "exact amount", amount: big.NewInt(1_000), wantAmount: 1_000},
		{name: "zero amount", amount: big.NewInt(0), wantErr: "must be positive"},
		{name: "negative amount", amount: big.NewInt(-1), wantErr: "must be positive"},
		{name: "amount above uint64", amount: new(big.Int).Lsh(big.NewInt(1), 64), wantErr: "does not fit in uint64"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transferAll, amount, err := withdrawAmountFor(fees.FeeTokenWithdrawal{Token: "mint", Amount: tt.amount})
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantTransferAll, transferAll)
			require.Equal(t, tt.wantAmount, amount)
		})
	}
}
