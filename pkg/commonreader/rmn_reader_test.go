package commonreader

import (
	"context"
	"errors"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/protocol"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/rmn_remote"
	"github.com/smartcontractkit/chainlink-ccip-solana/internal/mocks"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solanarpc"
)

// encodeCurses builds an rpc.GetAccountInfoResult containing a properly
// Borsh-encoded Curses account: the 8-byte Account_Curses discriminator
// followed by the Borsh-encoded Curses struct (Version + CursedSubjects vec).
func encodeCurses(t *testing.T, curses rmn_remote.Curses) *rpc.GetAccountInfoResult {
	t.Helper()
	body, err := curses.Marshal()
	require.NoError(t, err)
	data := append(append([]byte{}, rmn_remote.Account_Curses[:]...), body...)
	return &rpc.GetAccountInfoResult{
		Value: &rpc.Account{
			Data: rpc.DataBytesOrJSONFromBytes(data),
		},
	}
}

func TestReadRMNCursedSubjects(t *testing.T) {
	t.Parallel()

	rmnRemoteAddress := solana.NewWallet().PublicKey()

	globalCurseSubject := rmn_remote.CurseSubject{
		Value: [16]uint8{0x01, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01},
	}
	// On Solana, chain selectors are stored as LE uint64 in bytes [0:8].
	// 0x0102030405060708 in LE = {0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01}
	chainSelectorSubject := rmn_remote.CurseSubject{
		Value: [16]uint8{0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01, 0, 0, 0, 0, 0, 0, 0, 0},
	}

	// Global curse passes through unchanged.
	expectedGlobalCurse := protocol.Bytes16{0x01, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01}
	// Chain selector is normalized to BE in [8:16] with [0:8] zeroed.
	// 0x0102030405060708 in BE = {0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	expectedChainSelector := protocol.Bytes16{
		0, 0, 0, 0, 0, 0, 0, 0,
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
	}

	rpcErr := errors.New("connection refused")

	tests := []struct {
		name         string
		getAccountFn func(context.Context, solana.PublicKey, rpc.CommitmentType) (*rpc.GetAccountInfoResult, error)
		wantSubjects []protocol.Bytes16
		wantErr      bool
		errContains  string
	}{
		{
			name: "happy_path_with_global_curse_and_chain_selector",
			getAccountFn: func(_ context.Context, _ solana.PublicKey, _ rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
				return encodeCurses(t, rmn_remote.Curses{
					Version:        1,
					CursedSubjects: []rmn_remote.CurseSubject{globalCurseSubject, chainSelectorSubject},
				}), nil
			},
			wantSubjects: []protocol.Bytes16{expectedGlobalCurse, expectedChainSelector},
		},
		{
			name: "not_found_returns_error",
			getAccountFn: func(_ context.Context, _ solana.PublicKey, _ rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
				return nil, rpc.ErrNotFound
			},
			wantErr:     true,
			errContains: "get RMN curses account",
		},
		{
			name: "rpc_error_propagates",
			getAccountFn: func(_ context.Context, _ solana.PublicKey, _ rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
				return nil, rpcErr
			},
			wantErr:     true,
			errContains: "get RMN curses account",
		},
		{
			name: "decode_error_propagates",
			getAccountFn: func(_ context.Context, _ solana.PublicKey, _ rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
				return &rpc.GetAccountInfoResult{
					Value: &rpc.Account{
						Data: rpc.DataBytesOrJSONFromBytes([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}),
					},
				}, nil
			},
			wantErr:     true,
			errContains: "parse RMN curses account",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := &mocks.MockAccountReader{}
			m.On("GetAccountInfo", mock.Anything, mock.Anything, mock.Anything).
				Return(tc.getAccountFn).Once()
			t.Cleanup(func() { m.AssertExpectations(t) })

			got, err := ReadRMNCursedSubjects(
				context.Background(),
				m,
				rmnRemoteAddress,
			)

			if tc.wantErr {
				require.Error(t, err)
				if tc.errContains != "" {
					require.Contains(t, err.Error(), tc.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.wantSubjects, got)
		})
	}
}

// TestReadRMNCursedSubjects_EmptyCurses verifies that a Curses account
// with zero subjects returns an empty (but non-nil) slice.
func TestReadRMNCursedSubjects_EmptyCurses(t *testing.T) {
	t.Parallel()

	rmnRemoteAddress := solana.NewWallet().PublicKey()

	m := &mocks.MockAccountReader{}
	m.On("GetAccountInfo", mock.Anything, mock.Anything, mock.Anything).
		Return(func(_ context.Context, _ solana.PublicKey, _ rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
			return encodeCurses(t, rmn_remote.Curses{
				Version:        1,
				CursedSubjects: []rmn_remote.CurseSubject{},
			}), nil
		}).Once()
	t.Cleanup(func() { m.AssertExpectations(t) })

	got, err := ReadRMNCursedSubjects(
		context.Background(),
		m,
		rmnRemoteAddress,
	)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Empty(t, got)
}

// Compile-time check that the mock satisfies AccountReader.
var _ solanarpc.AccountReader = (*mocks.MockAccountReader)(nil)
