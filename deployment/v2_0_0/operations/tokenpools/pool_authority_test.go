package tokenpools

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	solCommonUtil "github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/common"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations/optest"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/burnmint_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/cctp_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/lombard_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
)

type stubAccount struct {
	owner solana.PublicKey
	data  []byte
}

// rpcStub answers getAccountInfo from accounts. Keys in failing get a JSON-RPC error, and any
// other key is a missing account. It records every key it is asked for.
type rpcStub struct {
	accounts map[solana.PublicKey]stubAccount
	failing  map[solana.PublicKey]bool

	mu        sync.Mutex
	requested []solana.PublicKey
}

func (s *rpcStub) client(t *testing.T) *rpc.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage   `json:"id"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Params) == 0 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var keyStr string
		if err := json.Unmarshal(req.Params[0], &keyStr); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		key := solana.MustPublicKeyFromBase58(keyStr)
		s.mu.Lock()
		s.requested = append(s.requested, key)
		s.mu.Unlock()

		resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		account, found := s.accounts[key]
		switch {
		case s.failing[key]:
			resp["error"] = map[string]any{"code": -32000, "message": "node is unhealthy"}
		case !found:
			resp["result"] = map[string]any{"context": map[string]any{"slot": 1}, "value": nil}
		default:
			resp["result"] = map[string]any{
				"context": map[string]any{"slot": 1},
				"value": map[string]any{
					"data":       []string{base64.StdEncoding.EncodeToString(account.data), "base64"},
					"executable": false,
					"lamports":   1_000_000,
					"owner":      account.owner.String(),
					"rentEpoch":  0,
					"space":      len(account.data),
				},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(server.Close)
	return rpc.New(server.URL)
}

func (s *rpcStub) wasRequested(key solana.PublicKey) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Contains(s.requested, key)
}

// poolFixture is a pool program with an upgrade authority and, optionally, an initialized pool
// for mint.
type poolFixture struct {
	program, programData, upgradeAuthority, mint, poolConfig solana.PublicKey
}

func newPoolFixture(t *testing.T, upgradeAuthority solana.PublicKey) poolFixture {
	t.Helper()
	f := poolFixture{
		program:          solana.NewWallet().PublicKey(),
		programData:      solana.NewWallet().PublicKey(),
		upgradeAuthority: upgradeAuthority,
		mint:             solana.NewWallet().PublicKey(),
	}
	var err error
	f.poolConfig, err = tokens.TokenPoolConfigAddress(f.mint, f.program)
	require.NoError(t, err)
	return f
}

// programAccounts returns the upgradeable-loader Program and ProgramData accounts that make
// upgradeAuthority the program's upgrade authority.
func (f poolFixture) programAccounts() map[solana.PublicKey]stubAccount {
	program := binary.LittleEndian.AppendUint32(nil, 2)
	program = append(program, f.programData.Bytes()...)
	programData := binary.LittleEndian.AppendUint32(nil, 3)
	programData = binary.LittleEndian.AppendUint64(programData, 1)
	programData = append(programData, 1)
	programData = append(programData, f.upgradeAuthority.Bytes()...)
	return map[solana.PublicKey]stubAccount{
		f.program:     {owner: solana.BPFLoaderUpgradeableProgramID, data: program},
		f.programData: {owner: solana.BPFLoaderUpgradeableProgramID, data: programData},
	}
}

func encodeAccount(t *testing.T, discriminator [8]byte, account interface {
	MarshalWithEncoder(*bin.Encoder) error
}) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.Write(discriminator[:])
	require.NoError(t, account.MarshalWithEncoder(bin.NewBorshEncoder(&buf)))
	return buf.Bytes()
}

func burnMintState(t *testing.T, owner, rateLimitAdmin solana.PublicKey) []byte {
	t.Helper()
	return encodeAccount(t, burnmint_token_pool.Account_State, burnmint_token_pool.State{
		Version: 1,
		Config:  burnmint_token_pool.BaseConfig{Owner: owner, RateLimitAdmin: rateLimitAdmin},
	})
}

func TestPoolAuthority(t *testing.T) {
	t.Parallel()
	owner := solana.NewWallet().PublicKey()
	upgradeAuthority := solana.NewWallet().PublicKey()
	alt := solana.NewWallet().PublicKey()

	lombardState := encodeAccount(t, lombard_token_pool.Account_State, lombard_token_pool.State{
		Version: 1,
		Config:  lombard_token_pool.BaseConfig{Owner: owner},
		Bridge:  solana.NewWallet().PublicKey(),
		Alt:     &alt,
	})
	cctpState := encodeAccount(t, cctp_token_pool.Account_State, cctp_token_pool.State{
		Version: 1,
		Config:  cctp_token_pool.BaseConfig{Owner: owner},
	})

	tests := []struct {
		name string
		// poolState is the pool config account data; nil leaves the account missing.
		poolState       []byte
		poolStateOwner  *solana.PublicKey // defaults to the pool program
		poolStateFails  bool
		want            solana.PublicKey
		wantErr         string
		wantUpgradeRead bool
	}{
		{name: "initialized burn-mint pool returns the owner", poolState: burnMintState(t, owner, solana.PublicKey{}), want: owner},
		{name: "initialized lombard pool returns the owner", poolState: lombardState, want: owner},
		{name: "initialized cctp pool returns the owner", poolState: cctpState, want: owner},
		{name: "uninitialized pool returns the upgrade authority", want: upgradeAuthority, wantUpgradeRead: true},
		{name: "rpc failure is an error", poolStateFails: true, wantErr: "node is unhealthy"},
		{name: "state owned by another program is an error", poolState: burnMintState(t, owner, solana.PublicKey{}), poolStateOwner: &alt, wantErr: "is owned by"},
		{name: "undecodable state is an error", poolState: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}, wantErr: "failed to parse account"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newPoolFixture(t, upgradeAuthority)
			stub := &rpcStub{accounts: f.programAccounts(), failing: map[solana.PublicKey]bool{}}
			if tt.poolState != nil {
				stateOwner := f.program
				if tt.poolStateOwner != nil {
					stateOwner = *tt.poolStateOwner
				}
				stub.accounts[f.poolConfig] = stubAccount{owner: stateOwner, data: tt.poolState}
			}
			if tt.poolStateFails {
				stub.failing[f.poolConfig] = true
			}
			chain := cldf_solana.Chain{Client: stub.client(t)}

			got, err := poolAuthority(t.Context(), chain, f.program, f.mint)
			require.Equal(t, tt.wantUpgradeRead, stub.wasRequested(f.program),
				"the upgrade authority must be read only for an uninitialized pool")
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// Operation tests are not parallel: each operation rewrites the binding's global ProgramID.

func opTestChain(t *testing.T, client *rpc.Client, deployer solana.PrivateKey, sent *[][]solana.Instruction) cldf_solana.Chain {
	t.Helper()
	return cldf_solana.Chain{
		Selector:    16423721717087811551, // solana-devnet
		Client:      client,
		DeployerKey: &deployer,
		Confirm: func(ixs []solana.Instruction, _ ...solCommonUtil.TxModifier) error {
			*sent = append(*sent, ixs)
			return nil
		},
	}
}

func TestUpsertRemoteChainConfigBurnMint_Authority(t *testing.T) {
	deployer := solana.NewWallet().PrivateKey
	customer := solana.NewWallet().PublicKey()
	timelock := solana.NewWallet().PublicKey()

	tests := []struct {
		name             string
		upgradeAuthority solana.PublicKey
		poolOwner        *solana.PublicKey // nil leaves the pool uninitialized
		poolStateFails   bool
		wantErr          string
		wantSent         bool
		wantBatch        bool
	}{
		{
			name:             "rpc failure is returned instead of signing as the upgrade authority",
			upgradeAuthority: timelock,
			poolStateFails:   true,
			wantErr:          "node is unhealthy",
		},
		{
			name:             "customer-owned pool on a timelock-upgradeable program is batched for the customer",
			upgradeAuthority: timelock,
			poolOwner:        &customer,
			wantBatch:        true,
		},
		{
			name:             "uninitialized pool upgradeable by the deployer is sent directly",
			upgradeAuthority: deployer.PublicKey(),
			wantSent:         true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPoolFixture(t, tt.upgradeAuthority)
			stub := &rpcStub{accounts: f.programAccounts(), failing: map[solana.PublicKey]bool{}}
			if tt.poolOwner != nil {
				stub.accounts[f.poolConfig] = stubAccount{owner: f.program, data: burnMintState(t, *tt.poolOwner, solana.PublicKey{})}
			}
			if tt.poolStateFails {
				stub.failing[f.poolConfig] = true
			}
			var sent [][]solana.Instruction
			chain := opTestChain(t, stub.client(t), deployer, &sent)

			report, err := operations.ExecuteOperation(optest.NewBundle(t), UpsertRemoteChainConfigBurnMint, chain, RemoteChainConfig{
				TokenPool:          f.program,
				TokenMint:          f.mint,
				TokenProgramID:     solana.TokenProgramID,
				RemoteSelector:     16015286601757825753, // ethereum-testnet-sepolia
				RemoteTokenAddress: bytes.Repeat([]byte{1}, 32),
				RemotePoolAddress:  bytes.Repeat([]byte{2}, 32),
				RemoteDecimals:     18,
			})
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Empty(t, sent, "nothing may be sent after a failed authority read")
				require.False(t, stub.wasRequested(f.program), "a failed read must not fall back to the upgrade authority")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantSent, len(sent) == 1)
			require.Equal(t, tt.wantBatch, len(report.Output.BatchOps) == 1)
			if tt.wantBatch {
				require.False(t, stub.wasRequested(f.program), "an initialized pool's owner signs, not the upgrade authority")
			}
		})
	}
}

func TestUpdateRateLimitAdminLockRelease_Authority(t *testing.T) {
	deployer := solana.NewWallet().PrivateKey
	admin := solana.NewWallet().PublicKey()

	t.Run("rpc failure is returned", func(t *testing.T) {
		f := newPoolFixture(t, deployer.PublicKey())
		stub := &rpcStub{accounts: f.programAccounts(), failing: map[solana.PublicKey]bool{f.poolConfig: true}}
		var sent [][]solana.Instruction
		chain := opTestChain(t, stub.client(t), deployer, &sent)

		_, err := operations.ExecuteOperation(optest.NewBundle(t), UpdateRateLimitAdminLockRelease, chain, TokenPoolTransferOwnershipInput{
			Program: f.program, TokenMint: f.mint, NewOwner: admin,
		})
		require.ErrorContains(t, err, "node is unhealthy")
		require.Empty(t, sent)
		require.False(t, stub.wasRequested(f.program))
	})

	t.Run("current admin is a no-op", func(t *testing.T) {
		f := newPoolFixture(t, deployer.PublicKey())
		stub := &rpcStub{accounts: f.programAccounts(), failing: map[solana.PublicKey]bool{}}
		stub.accounts[f.poolConfig] = stubAccount{owner: f.program, data: burnMintState(t, deployer.PublicKey(), admin)}
		var sent [][]solana.Instruction
		chain := opTestChain(t, stub.client(t), deployer, &sent)

		report, err := operations.ExecuteOperation(optest.NewBundle(t), UpdateRateLimitAdminLockRelease, chain, TokenPoolTransferOwnershipInput{
			Program: f.program, TokenMint: f.mint, NewOwner: admin,
		})
		require.NoError(t, err)
		require.Empty(t, sent)
		require.Empty(t, report.Output.BatchOps)
	})

	t.Run("new admin is set by the pool owner", func(t *testing.T) {
		f := newPoolFixture(t, solana.NewWallet().PublicKey())
		stub := &rpcStub{accounts: f.programAccounts(), failing: map[solana.PublicKey]bool{}}
		stub.accounts[f.poolConfig] = stubAccount{owner: f.program, data: burnMintState(t, deployer.PublicKey(), solana.PublicKey{})}
		var sent [][]solana.Instruction
		chain := opTestChain(t, stub.client(t), deployer, &sent)

		_, err := operations.ExecuteOperation(optest.NewBundle(t), UpdateRateLimitAdminLockRelease, chain, TokenPoolTransferOwnershipInput{
			Program: f.program, TokenMint: f.mint, NewOwner: admin,
		})
		require.NoError(t, err)
		require.Len(t, sent, 1)
		require.False(t, stub.wasRequested(f.program))
	})
}
