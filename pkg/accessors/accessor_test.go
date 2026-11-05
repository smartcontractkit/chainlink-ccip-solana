package accessors

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/keystore"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-ccip-solana/internal/mocks"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solanarpc"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/transmitter"
)

type contextCheckingKeystore struct {
	keystore.UnimplementedKeystore
}

func (contextCheckingKeystore) GetKeys(ctx context.Context, _ keystore.GetKeysRequest) (keystore.GetKeysResponse, error) {
	return keystore.GetKeysResponse{}, ctx.Err()
}

func TestAccessor_SetKeystore_usesCallerContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	a := &accessor{
		lggr:           logger.Nop(),
		chainID:        "localnet",
		rpc:            &solanarpc.Client{},
		keyName:        transmitter.DefaultKeyName,
		destConfigured: true,
	}
	require.ErrorIs(t, a.SetKeystore(ctx, contextCheckingKeystore{}), context.Canceled)
}

func TestAccessor_Close_closesSourceLogPoller(t *testing.T) {
	lp := mocks.NewMockService(t)
	lp.On("Close").Return(nil).Once()

	a := &accessor{sourceLogPoller: lp}
	require.NoError(t, a.Close())
	require.NoError(t, a.Close(), "Close must be idempotent")
}

func TestAccessor_Close_propagatesCloseError(t *testing.T) {
	want := errors.New("close failed")
	lp := mocks.NewMockService(t)
	lp.On("Close").Return(want).Once()

	a := &accessor{sourceLogPoller: lp}
	require.ErrorIs(t, a.Close(), want)
	require.ErrorIs(t, a.Close(), want, "idempotent Close returns same error")
}

func TestAccessor_Close_nilLogPoller(t *testing.T) {
	a := &accessor{}
	require.NoError(t, a.Close())
	require.NoError(t, a.Close())
}

func TestAccessorUnsupportedCapabilities(t *testing.T) {
	a := &accessor{}

	reader, err := a.FeeQuoterReader()
	require.Nil(t, reader)
	require.EqualError(t, err, "solana fee quoter reader is not configured")

	writer, err := a.FeeQuoterWriter()
	require.Nil(t, writer)
	require.EqualError(t, err, "solana fee quoter writer is not configured")

	gasReader, err := a.GasPriceReader()
	require.Nil(t, gasReader)
	require.EqualError(t, err, "solana gas price reader is not configured")
}
