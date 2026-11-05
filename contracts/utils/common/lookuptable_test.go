//revive:disable:var-naming // legacy package name
package common

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"
	"github.com/stretchr/testify/require"
)

// lutJSONRPC fakes the JSON-RPC transport behind a *rpc.Client (via
// rpc.NewWithCustomRPCClient). getAccountInfo responses are keyed by pubkey; unknown
// accounts return result:null (what a real node returns for a missing account).
// Any other method fails loudly.
type lutJSONRPC struct {
	results map[solana.PublicKey]*rpc.GetAccountInfoResult
	err     error
}

func (f *lutJSONRPC) CallForInto(_ context.Context, out any, method string, params []any) error {
	if method != "getAccountInfo" {
		panic("unexpected rpc method: " + method)
	}
	if f.err != nil {
		return f.err
	}
	key := params[0].(solana.PublicKey)
	var result json.RawMessage
	if res := f.results[key]; res != nil {
		bs, err := json.Marshal(res)
		if err != nil {
			return err
		}
		result = bs
	} else {
		result = json.RawMessage("null")
	}
	return (&jsonrpc.RPCResponse{Result: result}).GetObject(out)
}

func (f *lutJSONRPC) CallBatch(context.Context, jsonrpc.RPCRequests) (jsonrpc.RPCResponses, error) {
	return nil, errors.New("unexpected CallBatch")
}

func (f *lutJSONRPC) CallWithCallback(context.Context, string, []any, func(*http.Request, *http.Response) error) error {
	return errors.New("unexpected CallWithCallback")
}

func (f *lutJSONRPC) Close() error { return nil }

// lookupTableAccount builds a raw address-lookup-table account: 56 bytes of metadata
// (type tag, deactivation slot, last-extended slot + start index, a None authority option,
// padding) followed by the 32-byte addresses. Layout per solana-go
// programs/address-lookup-table/address-lookup.go.
func lookupTableAccount(addresses []solana.PublicKey) *rpc.GetAccountInfoResult {
	data := make([]byte, 56) // LOOKUP_TABLE_META_SIZE
	data[0] = 1              // TypeIndex = ProgramStateLookupTable
	for _, addr := range addresses {
		data = append(data, addr.Bytes()...)
	}
	return &rpc.GetAccountInfoResult{
		Value: &rpc.Account{Data: rpc.DataBytesOrJSONFromBytes(data)},
	}
}

// TestGetAddressLookupTable covers the fetch-and-decode path against a real
// *rpc.Client backed by a fake JSON-RPC transport.
func TestGetAddressLookupTable(t *testing.T) {
	lut := solana.PublicKey(MakeRandom32ByteArray())
	want := []solana.PublicKey{
		solana.PublicKey(MakeRandom32ByteArray()),
		solana.PublicKey(MakeRandom32ByteArray()),
	}

	t.Run("decodes addresses", func(t *testing.T) {
		client := rpc.NewWithCustomRPCClient(&lutJSONRPC{results: map[solana.PublicKey]*rpc.GetAccountInfoResult{lut: lookupTableAccount(want)}})
		got, err := GetAddressLookupTable(context.Background(), client, lut)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("empty table", func(t *testing.T) {
		client := rpc.NewWithCustomRPCClient(&lutJSONRPC{results: map[solana.PublicKey]*rpc.GetAccountInfoResult{lut: lookupTableAccount(nil)}})
		got, err := GetAddressLookupTable(context.Background(), client, lut)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("propagates rpc error", func(t *testing.T) {
		client := rpc.NewWithCustomRPCClient(&lutJSONRPC{err: errors.New("rpc unreachable")})
		_, err := GetAddressLookupTable(context.Background(), client, lut)
		require.ErrorContains(t, err, "rpc unreachable")
	})

	t.Run("missing account does not panic", func(t *testing.T) {
		// Node returns result:null for a nonexistent account; solana-go surfaces that
		// as an explicit error rather than a nil-deref during LUT decode.
		client := rpc.NewWithCustomRPCClient(&lutJSONRPC{results: map[solana.PublicKey]*rpc.GetAccountInfoResult{}})
		_, err := GetAddressLookupTable(context.Background(), client, lut)
		require.ErrorContains(t, err, "expected a value, got null result")
	})
}

func TestGetAddressLookupTableMap(t *testing.T) {
	t.Run("no lookup tables skips the rpc entirely", func(t *testing.T) {
		client := rpc.NewWithCustomRPCClient(&lutJSONRPC{err: errors.New("must not be called")})
		got, err := GetAddressLookupTableMap(context.Background(), client, nil)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("one entry per table", func(t *testing.T) {
		lutA := solana.PublicKey(MakeRandom32ByteArray())
		lutB := solana.PublicKey(MakeRandom32ByteArray())
		entriesA := []solana.PublicKey{solana.PublicKey(MakeRandom32ByteArray())}
		entriesB := []solana.PublicKey{solana.PublicKey(MakeRandom32ByteArray())}
		client := rpc.NewWithCustomRPCClient(&lutJSONRPC{results: map[solana.PublicKey]*rpc.GetAccountInfoResult{
			lutA: lookupTableAccount(entriesA),
			lutB: lookupTableAccount(entriesB),
		}})

		got, err := GetAddressLookupTableMap(context.Background(), client, []solana.PublicKey{lutA, lutB})
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.Equal(t, entriesA, []solana.PublicKey(got[lutA]))
		require.Equal(t, entriesB, []solana.PublicKey(got[lutB]))
	})
}
