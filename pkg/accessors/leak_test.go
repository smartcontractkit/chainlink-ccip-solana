// Goroutine leak tests use go.uber.org/goleak and chainlink-common/pkg/utils/tests.VerifyNoLeaks.
//
// Do NOT add t.Parallel() to any test in this file. goleak checks goroutines for the
// currently running test; t.Parallel() in the same package can cause false positives
// or negatives when other tests start background workers.
package accessors

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/smartcontractkit/chainlink-ccv/pkg/chainaccess"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commontests "github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/committee_verifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solutil"
)

func newSourceOnlyAccessor(t *testing.T) chainaccess.Accessor {
	t.Helper()
	ctx := context.Background()
	sel, strSel, settings := testSolLocalSetting(t)
	f := NewFactory(
		logger.Nop(),
		remoteJDConfigs{
			OnRampAddresses: map[string]string{
				strSel: solutil.PublicKeyToHex(ccip_offramp.ProgramID),
			},
			RMNRemoteAddresses: map[string]string{
				strSel: solutil.PublicKeyToHex(committee_verifier.ProgramID),
			},
		},
		map[protocol.ChainSelector]localConfig{sel: settings},
	)
	acc, err := f.GetAccessor(ctx, sel)
	require.NoError(t, err)
	_, err = acc.SourceReader()
	require.NoError(t, err)
	return acc
}

// TestGoleak_detectsUnclosedSourceLogpoller asserts goleak.Find reports extra goroutines
// when GetAccessor starts the source logpoller but accessor.Close is not called (red path).
func TestGoleak_detectsUnclosedSourceLogpoller(t *testing.T) {
	// Do not add t.Parallel(): see file comment.
	baseline := goleak.IgnoreCurrent()

	acc := newSourceOnlyAccessor(t)
	t.Cleanup(func() { require.NoError(t, acc.Close()) })

	err := goleak.Find(baseline)
	require.Error(t, err, "goleak should detect goroutines left by unclosed source logpoller")
	require.Contains(t, err.Error(), "found unexpected goroutines")
}

// TestGetAccessor_sourceOnly_noLeaks verifies GetAccessor + accessor.Close leave no
// background goroutines when only the source logpoller is wired.
func TestGetAccessor_sourceOnly_noLeaks(t *testing.T) {
	// Do not add t.Parallel(): see file comment.
	commontests.VerifyNoLeaks(t)

	acc := newSourceOnlyAccessor(t)
	t.Cleanup(func() { require.NoError(t, acc.Close()) })
}

// TestGetAccessor_destFailureAfterSourceStart_noLeaks verifies destination wiring
// failure rolls back a started source logpoller (no accessor returned, no leak).
// Uses invalid offRamp hex so newDestinationComponents fails before offramp logpoller creation.
func TestGetAccessor_destFailureAfterSourceStart_noLeaks(t *testing.T) {
	// Do not add t.Parallel(): see file comment.
	commontests.VerifyNoLeaks(t)

	ctx := context.Background()
	sel, strSel, settings := testSolLocalSetting(t)

	f := NewFactory(
		logger.Nop(),
		remoteJDConfigs{
			OnRampAddresses: map[string]string{
				strSel: solutil.PublicKeyToHex(ccip_offramp.ProgramID),
			},
			RMNRemoteAddresses: map[string]string{
				strSel: solutil.PublicKeyToHex(committee_verifier.ProgramID),
			},
			DestinationChain: map[string]chainaccess.DestinationChainConfig{
				strSel: {OffRampAddress: "not-valid-hex"},
			},
		},
		map[protocol.ChainSelector]localConfig{sel: settings},
	)

	acc, err := f.GetAccessor(ctx, sel)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid offRamp address")
	require.Nil(t, acc)
}
