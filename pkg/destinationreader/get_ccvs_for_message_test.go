package destinationreader

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/protocol"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
)

// These cover GetCCVSForMessage's simulate-and-decode path against a real *rpc.Client
// backed by a fake JSON-RPC transport, which the contracts/utils helpers require.
//
// buildTestMessage carries no TokenTransfer, so the token-pool account resolution returns
// immediately and an empty LookupTablesToSave short-circuits GetAddressLookupTableMap:
// a messaging-only message needs two simulate calls and zero account reads.

// simJSONRPC serves canned *rpc.SimulateTransactionResponse values one method call at a
// time (the last repeats). Any non-simulateTransaction method fails loudly.
type simJSONRPC struct {
	results []*rpc.SimulateTransactionResponse
	calls   *atomic.Int32
}

func (f *simJSONRPC) CallForInto(_ context.Context, out any, method string, _ []any) error {
	if method != "simulateTransaction" {
		panic("unexpected rpc method: " + method)
	}
	f.calls.Add(1)
	if len(f.results) == 0 {
		return errors.New("no simulate result configured")
	}
	i := min(int(f.calls.Load())-1, len(f.results)-1)
	bs, err := json.Marshal(f.results[i])
	if err != nil {
		return err
	}
	return (&jsonrpc.RPCResponse{Result: bs}).GetObject(out)
}

func (f *simJSONRPC) CallBatch(context.Context, jsonrpc.RPCRequests) (jsonrpc.RPCResponses, error) {
	return nil, errors.New("unexpected CallBatch")
}

func (f *simJSONRPC) CallWithCallback(context.Context, string, []any, func(*http.Request, *http.Response) error) error {
	return errors.New("unexpected CallWithCallback")
}

// newSimClient returns a real *rpc.Client whose simulateTransaction results are served in
// order (the last repeats). The counter tracks how many calls were made.
func newSimClient(t *testing.T, results ...*rpc.SimulateTransactionResponse) (*rpc.Client, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	return rpc.NewWithCustomRPCClient(&simJSONRPC{results: results, calls: &calls}), &calls
}

// simReturnLog formats the line ExtractReturnValue looks for: it matches on the
// "Program return: <programID>" prefix and base64-decodes the last space-separated token.
func simReturnLog(t *testing.T, programID solana.PublicKey, payload []byte) []string {
	t.Helper()
	return []string{"Program return: " + programID.String() + " " + base64.StdEncoding.EncodeToString(payload)}
}

func simResponse(logs []string, simErr any) *rpc.SimulateTransactionResponse {
	return &rpc.SimulateTransactionResponse{
		Value: &rpc.SimulateTransactionResult{Err: simErr, Logs: logs},
	}
}

// marshalAnchor encodes a response as the bare Borsh body with no discriminator, which is
// what ExtractAnchorTypedReturnValue expects.
func marshalAnchor(t *testing.T, v interface{ Marshal() ([]byte, error) }) []byte {
	t.Helper()
	body, err := v.Marshal()
	require.NoError(t, err)
	return body
}

// resolveDone ends resolution after one round: no NextIxDiscriminator, and no lookup
// tables so GetAddressLookupTableMap never hits the RPC.
func resolveDone() ccip_common.ResolveAccountsResponse {
	return ccip_common.ResolveAccountsResponse{
		AccountsToSave: []ccip_common.CcipAccountMeta{{Pubkey: solana.PublicKey(common.MakeRandom32ByteArray())}},
		Metadata:       []byte{},
	}
}

type directRPC struct {
	*rpc.Client
}

func (c directRPC) GetAccountInfo(ctx context.Context, addr solana.PublicKey, commitment rpc.CommitmentType) (*rpc.GetAccountInfoResult, error) {
	return c.GetAccountInfoWithOpts(ctx, addr, &rpc.GetAccountInfoOpts{Commitment: commitment})
}

func (c directRPC) GetTransaction(ctx context.Context, sig solana.Signature) (*rpc.GetTransactionResult, error) {
	maxTxVersion := uint64(0)
	return c.Client.GetTransaction(ctx, sig, &rpc.GetTransactionOpts{
		Commitment:                     rpc.CommitmentFinalized,
		Encoding:                       solana.EncodingBase64,
		MaxSupportedTransactionVersion: &maxTxVersion,
	})
}

func newReaderWithRPC(t *testing.T, rpcClient *rpc.Client, withFeePayer bool) *DestinationReader {
	t.Helper()
	p := newDefaultParams(t)
	p.RPC = directRPC{Client: rpcClient}
	dr, err := NewDestinationReader(p)
	require.NoError(t, err)
	if withFeePayer {
		dr.SetSimulationFeePayer(solana.PublicKey(common.MakeRandom32ByteArray()))
	}
	return dr
}

func TestGetCCVSForMessage_HappyPath(t *testing.T) {
	required := solana.PublicKey(common.MakeRandom32ByteArray())
	optional := solana.PublicKey(common.MakeRandom32ByteArray())

	rpcClient, calls := newSimClient(t,
		simResponse(simReturnLog(t, testOfframpProgramID, marshalAnchor(t, resolveDone())), nil),
		simResponse(simReturnLog(t, testOfframpProgramID, marshalAnchor(t, ccip_offramp.GetCcvsForMsgResponse{
			RequiredCcvs:      []solana.PublicKey{required},
			OptionalCcvs:      []solana.PublicKey{optional},
			OptionalThreshold: 1,
		})), nil),
	)
	dr := newReaderWithRPC(t, rpcClient, true)

	info, err := dr.GetCCVSForMessage(context.Background(), buildTestMessage(t, dr.chainSelector))
	require.NoError(t, err)

	require.Equal(t, int32(2), calls.Load(), "one resolver round plus the get_ccvs simulation")
	require.Equal(t, []protocol.UnknownAddress{protocol.UnknownAddress(required.Bytes())}, info.RequiredCCVs)
	require.Equal(t, []protocol.UnknownAddress{protocol.UnknownAddress(optional.Bytes())}, info.OptionalCCVs)
	require.Equal(t, uint8(1), info.OptionalThreshold)
}

func TestGetCCVSForMessage_Errors(t *testing.T) {
	okResolve := simResponse(simReturnLog(t, testOfframpProgramID, marshalAnchor(t, resolveDone())), nil)

	tests := []struct {
		name    string
		results []*rpc.SimulateTransactionResponse
		wantErr string
	}{
		{
			name:    "resolution simulation failed",
			results: []*rpc.SimulateTransactionResponse{simResponse([]string{"Program log: AccountNotFound"}, "InstructionError")},
			wantErr: "resolve get_ccvs_for_msg accounts",
		},
		{
			name:    "resolution response missing from logs",
			results: []*rpc.SimulateTransactionResponse{simResponse([]string{"Program log: nothing useful"}, nil)},
			wantErr: "failed to extract resolve accounts response",
		},
		{
			name:    "get_ccvs simulation failed",
			results: []*rpc.SimulateTransactionResponse{okResolve, simResponse([]string{"Program log: boom"}, "InstructionError")},
			wantErr: "get_ccvs_for_msg simulation failed",
		},
		{
			name:    "get_ccvs return value unparseable",
			results: []*rpc.SimulateTransactionResponse{okResolve, simResponse(simReturnLog(t, testOfframpProgramID, []byte{0xff}), nil)},
			wantErr: "parse get_ccvs_for_msg return value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rpcClient, _ := newSimClient(t, tt.results...)
			dr := newReaderWithRPC(t, rpcClient, true)

			_, err := dr.GetCCVSForMessage(context.Background(), buildTestMessage(t, dr.chainSelector))
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// Solana rejects an unfunded fee payer even for SigVerify=false simulation, so the guard
// must fire before any RPC call.
func TestGetCCVSForMessage_FeePayerNotConfigured(t *testing.T) {
	rpcClient, calls := newSimClient(t)
	dr := newReaderWithRPC(t, rpcClient, false)

	_, err := dr.GetCCVSForMessage(context.Background(), buildTestMessage(t, dr.chainSelector))
	require.ErrorContains(t, err, "simulation fee payer not configured")
	require.Zero(t, calls.Load())
}

// The guard must run before Encode/MessageID: with a malformed message an
// out-of-order guard would surface the encoding error instead of the payer one.
func TestGetCCVSForMessage_FeePayerGuardPrecedesEncoding(t *testing.T) {
	rpcClient, calls := newSimClient(t)
	dr := newReaderWithRPC(t, rpcClient, false)

	msg := buildTestMessage(t, dr.chainSelector)
	msg.DataLength = 99
	msg.Data = []byte{1, 2, 3}

	_, err := dr.GetCCVSForMessage(context.Background(), msg)
	require.ErrorContains(t, err, "simulation fee payer not configured")
	require.Zero(t, calls.Load())
}
