package solcommon

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	solCommonUtil "github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/common"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
)

type testConfig struct {
	Owner solana.PublicKey
}

var testConfigDiscriminator = []byte{1, 2, 3, 4, 5, 6, 7, 8}

func parseTestConfig(data []byte) (*testConfig, error) {
	if len(data) < 8+solana.PublicKeyLength || string(data[:8]) != string(testConfigDiscriminator) {
		return nil, errors.New("not a test config")
	}
	return &testConfig{Owner: solana.PublicKeyFromBytes(data[8 : 8+solana.PublicKeyLength])}, nil
}

// accountInfoStub answers getAccountInfo with an account holding data and owned by owner, a null
// value when missing is set, or a JSON-RPC error when rpcErr is set.
func accountInfoStub(t *testing.T, owner solana.PublicKey, data []byte, missing, rpcErr bool) *rpc.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		switch {
		case rpcErr:
			resp["error"] = map[string]any{"code": -32000, "message": "node is unhealthy"}
		case missing:
			resp["result"] = map[string]any{"context": map[string]any{"slot": 1}, "value": nil}
		default:
			resp["result"] = map[string]any{
				"context": map[string]any{"slot": 1},
				"value": map[string]any{
					"data":       []string{base64.StdEncoding.EncodeToString(data), "base64"},
					"executable": false,
					"lamports":   1_000_000,
					"owner":      owner.String(),
					"rentEpoch":  0,
					"space":      len(data),
				},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(server.Close)
	return rpc.New(server.URL)
}

func TestConfigAuthority(t *testing.T) {
	t.Parallel()
	program := solana.NewWallet().PublicKey()
	configPDA := solana.NewWallet().PublicKey()
	owner := solana.NewWallet().PublicKey()
	deployer := solana.NewWallet().PrivateKey
	validConfig := append(append([]byte{}, testConfigDiscriminator...), owner.Bytes()...)

	tests := []struct {
		name         string
		accountOwner solana.PublicKey
		data         []byte
		missing      bool
		rpcErr       bool
		want         solana.PublicKey
		wantErr      string
	}{
		{name: "missing config returns the deployer", missing: true, want: deployer.PublicKey()},
		{name: "system-owned config returns the deployer", accountOwner: solana.SystemProgramID, want: deployer.PublicKey()},
		{name: "initialized config returns the picked key", accountOwner: program, data: validConfig, want: owner},
		{name: "rpc failure is an error", rpcErr: true, wantErr: "node is unhealthy"},
		{name: "config owned by another program is an error", accountOwner: solana.NewWallet().PublicKey(), data: validConfig, wantErr: "is owned by"},
		{name: "unparsable config is an error", accountOwner: program, data: []byte{9, 9, 9}, wantErr: "not a test config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			chain := cldf_solana.Chain{
				Client:      accountInfoStub(t, tt.accountOwner, tt.data, tt.missing, tt.rpcErr),
				DeployerKey: &deployer,
			}
			got, err := ConfigAuthority(t.Context(), chain, program, configPDA, parseTestConfig, func(c *testConfig) solana.PublicKey { return c.Owner })
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestConfirmOrBatch(t *testing.T) {
	t.Parallel()
	deployer := solana.NewWallet().PrivateKey
	timelock := solana.NewWallet().PublicKey()
	program := solana.NewWallet().PublicKey()
	ix := solana.NewInstruction(program, solana.AccountMetaSlice{solana.Meta(timelock).SIGNER()}, []byte{1, 2, 3})

	tests := []struct {
		name      string
		signer    solana.PublicKey
		ixns      []solana.Instruction
		wantSent  int
		wantBatch int
	}{
		{name: "deployer signer sends the instructions", signer: deployer.PublicKey(), ixns: []solana.Instruction{ix}, wantSent: 1},
		{name: "other signer batches the instructions", signer: timelock, ixns: []solana.Instruction{ix}, wantBatch: 1},
		{name: "no instructions for the deployer sends nothing", signer: deployer.PublicKey(), ixns: nil},
		{name: "no instructions for another signer batches nothing", signer: timelock, ixns: []solana.Instruction{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sent := 0
			chain := cldf_solana.Chain{
				Selector:    16423721717087811551, // solana-devnet
				DeployerKey: &deployer,
				Confirm: func(ixns []solana.Instruction, _ ...solCommonUtil.TxModifier) error {
					if len(ixns) == 0 {
						return errors.New("requires at-least one instruction to create a transaction")
					}
					sent++
					return nil
				},
			}
			out, err := ConfirmOrBatch(chain, tt.signer, tt.ixns, program, "TestProgram")
			require.NoError(t, err)
			require.Equal(t, tt.wantSent, sent)
			require.Len(t, out.BatchOps, tt.wantBatch)
			for _, batch := range out.BatchOps {
				require.Len(t, batch.Transactions, len(tt.ixns))
			}
		})
	}
}
