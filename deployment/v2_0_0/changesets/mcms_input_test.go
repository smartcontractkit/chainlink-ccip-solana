package changesets

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	mcmstypes "github.com/smartcontractkit/mcms/types"
)

func TestValidateMCMSInput(t *testing.T) {
	future := uint32(time.Now().Add(time.Hour).Unix()) //nolint:gosec // fits in uint32 until 2106
	past := uint32(time.Now().Add(-time.Hour).Unix())  //nolint:gosec // fits in uint32 until 2106

	tests := []struct {
		name    string
		input   mcms.Input
		wantErr string
	}{
		{name: "an empty input is skipped", input: mcms.Input{}},
		{name: "unset valid until and timelock action get their defaults", input: mcms.Input{Qualifier: "CLLCCIP", Description: "d"}},
		{name: "explicit values are kept", input: mcms.Input{TimelockAction: mcmstypes.TimelockActionSchedule, ValidUntil: future}},
		{
			name:    "an explicit valid until in the past is rejected",
			input:   mcms.Input{TimelockAction: mcmstypes.TimelockActionSchedule, ValidUntil: past},
			wantErr: "ValidUntil must be in the future",
		},
		{
			name:    "an invalid timelock action is rejected",
			input:   mcms.Input{TimelockAction: "nope", ValidUntil: future},
			wantErr: "invalid timelock action",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := tt.input
			err := validateMCMSInput(nil, tt.input)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
			require.Equal(t, before, tt.input, "the caller's input must not be modified")
		})
	}
}

func TestRequireMCMSForBatchOps(t *testing.T) {
	withTxs := []mcmstypes.BatchOperation{{Transactions: []mcmstypes.Transaction{{}}}}
	noTxs := []mcmstypes.BatchOperation{{}}

	require.NoError(t, requireMCMSForBatchOps(nil, mcms.Input{}, nil), "the deployer signed everything")
	require.NoError(t, requireMCMSForBatchOps(nil, mcms.Input{}, noTxs), "empty batch operations need no proposal")
	require.NoError(t, requireMCMSForBatchOps(nil, mcms.Input{Qualifier: "CLLCCIP"}, withTxs), "an mcms input was given")
	require.ErrorContains(t, requireMCMSForBatchOps(nil, mcms.Input{}, withTxs), "an mcms input is required")
}
