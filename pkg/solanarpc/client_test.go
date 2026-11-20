package solanarpc_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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

	// firstAvailCount counts getFirstAvailableBlock requests when set.
	firstAvailCount *atomic.Int64
	// blockFirstAvail makes getFirstAvailableBlock block until the request
	// context is done.
	blockFirstAvail bool
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
	case "getFirstAvailableBlock":
		if s.firstAvailCount != nil {
			s.firstAvailCount.Add(1)
		}
		if s.blockFirstAvail {
			<-r.Context().Done()
			return
		}
		result = uint64(0)
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

func TestNewClient_HistoryMonitorEnabledByDefault(t *testing.T) {
	t.Parallel()

	require.Equal(t, time.Minute, solanarpc.DefaultHistoryMonitorPeriod)

	chainID := solana.Hash{2}.String()
	firstAvailCount := new(atomic.Int64)
	server := httptest.NewServer(&rpcStub{chainID: chainID, firstAvailCount: firstAvailCount})
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
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	require.Eventually(t, func() bool {
		return firstAvailCount.Load() > 0
	}, 5*time.Second, 10*time.Millisecond)
}

func TestClient_HistoryMonitorRunsAndStops(t *testing.T) {
	t.Parallel()

	chainID := solana.Hash{3}.String()
	firstAvailCount := new(atomic.Int64)
	server := httptest.NewServer(&rpcStub{chainID: chainID, firstAvailCount: firstAvailCount})
	t.Cleanup(server.Close)

	client, err := solanarpc.NewClient(t.Context(), solanarpc.Config{
		ChainID: chainID,
		Nodes: []*solconfig.Node{{
			Name: new("primary"),
			URL:  config.MustParseURL(server.URL),
		}},
		RPCTimeout:           10 * time.Second,
		HistoryMonitorPeriod: 50 * time.Millisecond,
	}, logger.Nop())
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return firstAvailCount.Load() > 0
	}, 5*time.Second, 10*time.Millisecond)

	require.NoError(t, client.Close())

	afterClose := firstAvailCount.Load()
	time.Sleep(300 * time.Millisecond)
	require.Equal(t, afterClose, firstAvailCount.Load())
}

func TestClient_HistoryMonitorCloseCancelsActiveMeasurement(t *testing.T) {
	t.Parallel()

	chainID := solana.Hash{4}.String()
	firstAvailCount := new(atomic.Int64)
	server := httptest.NewServer(&rpcStub{
		chainID:         chainID,
		firstAvailCount: firstAvailCount,
		blockFirstAvail: true,
	})
	t.Cleanup(server.Close)

	client, err := solanarpc.NewClient(t.Context(), solanarpc.Config{
		ChainID: chainID,
		Nodes: []*solconfig.Node{{
			Name: new("primary"),
			URL:  config.MustParseURL(server.URL),
		}},
		RPCTimeout:           10 * time.Second,
		HistoryMonitorPeriod: 50 * time.Millisecond,
	}, logger.Nop())
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return firstAvailCount.Load() == 1
	}, 5*time.Second, 10*time.Millisecond)

	require.NoError(t, client.Close())

	afterClose := firstAvailCount.Load()
	time.Sleep(300 * time.Millisecond)
	require.Equal(t, afterClose, firstAvailCount.Load())
}

func TestNewClient_StartFailureClosesStartedMonitor(t *testing.T) {
	t.Parallel()

	client, err := solanarpc.NewClient(t.Context(), solanarpc.Config{
		ChainID:              solana.Hash{5}.String(),
		RPCTimeout:           time.Second,
		HistoryMonitorPeriod: time.Minute,
	}, logger.Nop())
	require.Nil(t, client)
	require.ErrorContains(t, err, "start multinode: failed to start service MultiNode: no available nodes")
	// A started monitor closes cleanly: its close must not surface as an error.
	require.NotContains(t, err.Error(), "HistoryMonitor has not been started")
}
