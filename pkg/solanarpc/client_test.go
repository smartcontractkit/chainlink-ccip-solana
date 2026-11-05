package solanarpc_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	solconfig "github.com/smartcontractkit/chainlink-solana/pkg/solana/config"

	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solanarpc"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
}

type rpcStub struct {
	chainID string
}

func (s *rpcStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var result any
	switch req.Method {
	case "getGenesisHash":
		result = s.chainID
	case "getSlot":
		result = uint64(42)
	case "getVersion":
		result = map[string]any{"solana-core": "test"}
	default:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"error":   map[string]any{"code": -32601, "message": "method not found"},
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"result":  result,
	})
}

func TestNewClient_RejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		nodes   []*solconfig.Node
		wantErr string
	}{
		{
			name:    "missing nodes",
			wantErr: "start multinode: failed to start service MultiNode: no available nodes",
		},
		{
			name:    "nil node",
			nodes:   []*solconfig.Node{nil},
			wantErr: "validate nodes: rpc node 0: missing node config",
		},
		{
			name: "missing URL",
			nodes: []*solconfig.Node{{
				Name: new("primary"),
			}},
			wantErr: "validate nodes: rpc node 0: URL: missing: required for all nodes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client, err := solanarpc.NewClient(t.Context(), solanarpc.Config{
				ChainID:    solana.Hash{1}.String(),
				Nodes:      tt.nodes,
				RPCTimeout: time.Second,
			}, logger.Nop())
			require.Nil(t, client)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestClient_Lifecycle(t *testing.T) {
	t.Parallel()

	chainID := solana.Hash{1}.String()
	server := httptest.NewServer(&rpcStub{chainID: chainID})
	t.Cleanup(server.Close)

	client, err := solanarpc.NewClient(t.Context(), solanarpc.Config{
		ChainID: chainID,
		Nodes: []*solconfig.Node{{
			Name: new("primary"),
			URL:  config.MustParseURL(server.URL),
		}},
		RPCTimeout: time.Second,
	}, logger.Nop())
	require.NoError(t, err)

	slot, err := client.GetSlot(t.Context(), rpc.CommitmentFinalized)
	require.NoError(t, err)
	require.Equal(t, uint64(42), slot)

	require.NoError(t, client.Close())

	_, err = client.GetSlot(t.Context(), rpc.CommitmentFinalized)
	require.Error(t, err)
}
