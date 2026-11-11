package adapters

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	solcommonutil "github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/common"
	ccvadapters "github.com/smartcontractkit/chainlink-ccv/deployment/adapters"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations/optest"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/committee_verifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	cvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
)

func TestConvertEVMSignerHexes(t *testing.T) {
	t.Parallel()

	t.Run("converts list", func(t *testing.T) {
		got, err := convertEVMSignerHexes([]string{
			"0x0000000000000000000000000000000000000001",
			"0x0000000000000000000000000000000000000002",
		})
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.Equal(t, [20]uint8{19: 1}, got[0])
		require.Equal(t, [20]uint8{19: 2}, got[1])
	})

	t.Run("propagates first error", func(t *testing.T) {
		_, err := convertEVMSignerHexes([]string{
			"0x0000000000000000000000000000000000000001",
			"not-hex",
		})
		require.Error(t, err)
	})

	t.Run("empty input returns empty slice", func(t *testing.T) {
		got, err := convertEVMSignerHexes(nil)
		require.NoError(t, err)
		require.Empty(t, got)
	})
}

// TestApplySignatureConfigs_EarlyValidation exercises the error paths of
// ApplySignatureConfigs that don't require a live Solana RPC. The successful
// on-chain submission path is covered by the program-level integration tests in
// contracts/tests/ccip/committee_verifier_test.go (RemoveSignerConfig + SetSignerConfig).
func TestApplySignatureConfigs_EarlyValidation(t *testing.T) {
	t.Parallel()

	const destChainSelector = uint64(124615329519749607) // arbitrary uint64
	const qualifier = "test-committee"

	committeeVerifierAddr := solana.NewWallet().PrivateKey.PublicKey().String()
	makeDataStoreWithCV := func(t *testing.T) *datastore.MemoryDataStore {
		ds := datastore.NewMemoryDataStore()
		require.NoError(t, ds.Addresses().Add(datastore.AddressRef{
			ChainSelector: destChainSelector,
			Type:          datastore.ContractType(cvops.ContractType),
			Version:       cvops.Version,
			Qualifier:     qualifier,
			Address:       committeeVerifierAddr,
		}))
		return ds
	}

	a := &SolanaCommitteeVerifierOnchainAdapter{}
	change := ccvadapters.SignatureConfigChange{
		NewConfigs: []ccvadapters.SignatureConfig{
			{
				SourceChainSelector: 10,
				Threshold:           1,
				Signers:             []string{"0x0000000000000000000000000000000000000001"},
			},
		},
	}

	t.Run("returns error when datastore is nil", func(t *testing.T) {
		env := deployment.Environment{}
		_, err := a.ApplySignatureConfigs(context.Background(), env, destChainSelector, qualifier, change)
		require.Error(t, err)
		require.Contains(t, err.Error(), "datastore")
	})

	t.Run("returns error when committee verifier ref is missing", func(t *testing.T) {
		env := deployment.Environment{
			DataStore:   datastore.NewMemoryDataStore().Seal(),
			BlockChains: cldf_chain.NewBlockChains(nil),
		}
		_, err := a.ApplySignatureConfigs(context.Background(), env, destChainSelector, qualifier, change)
		require.Error(t, err)
		require.Contains(t, err.Error(), "committee verifier")
	})

	t.Run("returns error when Solana chain is not in environment", func(t *testing.T) {
		env := deployment.Environment{
			DataStore:   makeDataStoreWithCV(t).Seal(),
			BlockChains: cldf_chain.NewBlockChains(nil),
		}
		_, err := a.ApplySignatureConfigs(context.Background(), env, destChainSelector, qualifier, change)
		require.Error(t, err)
		require.Contains(t, err.Error(), "solana chain")
	})

	t.Run("returns error when deployer key is missing", func(t *testing.T) {
		ds := makeDataStoreWithCV(t)
		env := deployment.Environment{
			DataStore: ds.Seal(),
			BlockChains: cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{
				destChainSelector: cldf_solana.Chain{
					Selector:    destChainSelector,
					DeployerKey: nil,
				},
			}),
		}
		_, err := a.ApplySignatureConfigs(context.Background(), env, destChainSelector, qualifier, change)
		require.Error(t, err)
		require.Contains(t, err.Error(), "deployer key")
	})

	t.Run("invalid signer hex in NewConfigs is rejected before any RPC call", func(t *testing.T) {
		ds := makeDataStoreWithCV(t)
		key := solana.NewWallet().PrivateKey
		env := deployment.Environment{
			DataStore: ds.Seal(),
			BlockChains: cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{
				destChainSelector: cldf_solana.Chain{
					Selector:    destChainSelector,
					DeployerKey: &key,
				},
			}),
		}
		bad := ccvadapters.SignatureConfigChange{
			NewConfigs: []ccvadapters.SignatureConfig{
				{
					SourceChainSelector: 10,
					Threshold:           1,
					Signers:             []string{"not-hex"},
				},
			},
		}
		_, err := a.ApplySignatureConfigs(context.Background(), env, destChainSelector, qualifier, bad)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid signers")
	})
}

const (
	committeeVerifierAdapterTestChainSelector = uint64(124615329519749607)
	committeeVerifierAdapterTestQualifier     = "test-committee"
)

func marshalCommitteeVerifierAdapterAccount(t *testing.T, discriminator [8]byte, account interface{ Marshal() ([]byte, error) }) []byte {
	t.Helper()
	body, err := account.Marshal()
	require.NoError(t, err)
	return append(discriminator[:], body...)
}

func newCommitteeVerifierAdapterClient(t *testing.T, program solana.PublicKey, accounts map[solana.PublicKey][]byte) *rpc.Client {
	t.Helper()
	resolution, err := (ccip_common.ResolveAccountsResponse{}).Marshal()
	require.NoError(t, err)
	returnLog := "Program return: " + program.String() + " " + base64.StdEncoding.EncodeToString(resolution)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		response := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		switch req.Method {
		case "getAccountInfo":
			if len(req.Params) == 0 {
				http.Error(w, "missing account address", http.StatusBadRequest)
				return
			}
			var address string
			if err := json.Unmarshal(req.Params[0], &address); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			account, err := solana.PublicKeyFromBase58(address)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			var value any
			if data, ok := accounts[account]; ok {
				value = map[string]any{
					"data":       []string{base64.StdEncoding.EncodeToString(data), "base64"},
					"executable": false,
					"lamports":   1_000_000,
					"owner":      program.String(),
					"rentEpoch":  0,
					"space":      len(data),
				}
			}
			response["result"] = map[string]any{"context": map[string]any{"slot": 1}, "value": value}
		case "simulateTransaction":
			response["result"] = map[string]any{
				"context": map[string]any{"slot": 1},
				"value":   map[string]any{"err": nil, "logs": []string{returnLog}},
			}
		default:
			response["error"] = map[string]any{"code": -32601, "message": "unexpected method: " + req.Method}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	t.Cleanup(server.Close)
	return rpc.New(server.URL)
}

func newCommitteeVerifierAdapterEnv(
	t *testing.T,
	program solana.PublicKey,
	deployer *solana.PrivateKey,
	client *rpc.Client,
	confirm func([]solana.Instruction, ...solcommonutil.TxModifier) error,
) deployment.Environment {
	t.Helper()
	ds := datastore.NewMemoryDataStore()
	require.NoError(t, ds.Addresses().Add(datastore.AddressRef{
		ChainSelector: committeeVerifierAdapterTestChainSelector,
		Type:          datastore.ContractType(cvops.ContractType),
		Version:       cvops.Version,
		Qualifier:     committeeVerifierAdapterTestQualifier,
		Address:       program.String(),
	}))
	return deployment.Environment{
		GetContext:       t.Context,
		DataStore:        ds.Seal(),
		OperationsBundle: optest.NewBundle(t),
		BlockChains: cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{
			committeeVerifierAdapterTestChainSelector: cldf_solana.Chain{
				Selector:    committeeVerifierAdapterTestChainSelector,
				Client:      client,
				DeployerKey: deployer,
				Confirm:     confirm,
			},
		}),
	}
}

func TestApplySignatureConfigsBatchOps(t *testing.T) {
	adapter := &SolanaCommitteeVerifierOnchainAdapter{}

	t.Run("no changes returns no batches", func(t *testing.T) {
		program := solana.NewWallet().PublicKey()
		deployer := solana.NewWallet().PrivateKey
		env := newCommitteeVerifierAdapterEnv(t, program, &deployer, nil, nil)

		batchOps, err := adapter.ApplySignatureConfigs(t.Context(), env, committeeVerifierAdapterTestChainSelector, committeeVerifierAdapterTestQualifier, ccvadapters.SignatureConfigChange{})
		require.NoError(t, err)
		require.Nil(t, batchOps)
	})

	t.Run("governed removals and additions are returned in order", func(t *testing.T) {
		program := solana.NewWallet().PublicKey()
		deployer := solana.NewWallet().PrivateKey
		owner := solana.NewWallet().PublicKey()
		configPDA, _, err := state.FindConfigPDA(program)
		require.NoError(t, err)
		accounts := map[solana.PublicKey][]byte{
			configPDA: marshalCommitteeVerifierAdapterAccount(t, committee_verifier.Account_Config, committee_verifier.Config{Owner: owner}),
		}
		client := newCommitteeVerifierAdapterClient(t, program, accounts)
		confirmCalls := 0
		env := newCommitteeVerifierAdapterEnv(t, program, &deployer, client, func([]solana.Instruction, ...solcommonutil.TxModifier) error {
			confirmCalls++
			return nil
		})
		change := ccvadapters.SignatureConfigChange{
			RemovedSourceChainSelectors: []uint64{10, 11},
			NewConfigs: []ccvadapters.SignatureConfig{
				{SourceChainSelector: 20, Threshold: 1, Signers: []string{"0x0000000000000000000000000000000000000001"}},
				{SourceChainSelector: 21, Threshold: 1, Signers: []string{"0x0000000000000000000000000000000000000002"}},
			},
		}

		batchOps, err := adapter.ApplySignatureConfigs(t.Context(), env, committeeVerifierAdapterTestChainSelector, committeeVerifierAdapterTestQualifier, change)
		require.NoError(t, err)
		require.Zero(t, confirmCalls)
		require.Len(t, batchOps, 4)
		removeDiscriminator := committee_verifier.Instruction_RemoveSignerConfig
		setDiscriminator := committee_verifier.Instruction_SetSignerConfig
		wantDiscriminators := [][]byte{removeDiscriminator[:], removeDiscriminator[:], setDiscriminator[:], setDiscriminator[:]}
		wantSourceSelectors := []uint64{10, 11, 20, 21}
		for i, batch := range batchOps {
			require.Equal(t, committeeVerifierAdapterTestChainSelector, uint64(batch.ChainSelector))
			require.Len(t, batch.Transactions, 1)
			transaction := batch.Transactions[0]
			require.Equal(t, program.String(), transaction.To)
			require.GreaterOrEqual(t, len(transaction.Data), 16)
			require.Equal(t, wantDiscriminators[i], transaction.Data[:8])
			require.Equal(t, wantSourceSelectors[i], binary.LittleEndian.Uint64(transaction.Data[8:16]))
		}
	})
}

func TestSetAllowedFinalityConfigBatchOps(t *testing.T) {
	adapter := &SolanaCommitteeVerifierOnchainAdapter{}
	for _, tt := range []struct {
		name     string
		governed bool
	}{
		{name: "direct write returns no batches"},
		{name: "governed write returns its batch", governed: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			program := solana.NewWallet().PublicKey()
			deployer := solana.NewWallet().PrivateKey
			owner := deployer.PublicKey()
			if tt.governed {
				owner = solana.NewWallet().PublicKey()
			}
			configPDA, _, err := state.FindConfigPDA(program)
			require.NoError(t, err)
			accounts := map[solana.PublicKey][]byte{
				configPDA: marshalCommitteeVerifierAdapterAccount(t, committee_verifier.Account_Config, committee_verifier.Config{Owner: owner}),
			}
			client := newCommitteeVerifierAdapterClient(t, program, accounts)
			confirmCalls := 0
			env := newCommitteeVerifierAdapterEnv(t, program, &deployer, client, func([]solana.Instruction, ...solcommonutil.TxModifier) error {
				confirmCalls++
				return nil
			})

			batchOps, err := adapter.SetAllowedFinalityConfig(t.Context(), env, committeeVerifierAdapterTestChainSelector, committeeVerifierAdapterTestQualifier, true, false, 0)
			require.NoError(t, err)
			if !tt.governed {
				require.Nil(t, batchOps)
				require.Equal(t, 1, confirmCalls)
				return
			}
			require.Zero(t, confirmCalls)
			require.Len(t, batchOps, 1)
			require.Equal(t, committeeVerifierAdapterTestChainSelector, uint64(batchOps[0].ChainSelector))
			require.Len(t, batchOps[0].Transactions, 1)
			require.Equal(t, program.String(), batchOps[0].Transactions[0].To)
		})
	}
}

func TestApplyAllowlistUpdatesBatchOps(t *testing.T) {
	adapter := &SolanaCommitteeVerifierOnchainAdapter{}
	const destChainSelector = uint64(42)

	t.Run("no-op returns no batches", func(t *testing.T) {
		program := solana.NewWallet().PublicKey()
		deployer := solana.NewWallet().PrivateKey
		configPDA, _, err := state.FindConfigPDA(program)
		require.NoError(t, err)
		remotePDA, _, err := state.FindCVRemoteChainConfigPDA(destChainSelector, program)
		require.NoError(t, err)
		accounts := map[solana.PublicKey][]byte{
			configPDA: marshalCommitteeVerifierAdapterAccount(t, committee_verifier.Account_Config, committee_verifier.Config{Owner: deployer.PublicKey(), AllowlistAdmin: deployer.PublicKey()}),
			remotePDA: marshalCommitteeVerifierAdapterAccount(t, committee_verifier.Account_RemoteChainConfig, committee_verifier.RemoteChainConfig{
				RemoteChainSelector: destChainSelector,
				AllowlistConfig:     committee_verifier.RemoteChainAllowlistConfig{AllowlistEnabled: true},
			}),
		}
		client := newCommitteeVerifierAdapterClient(t, program, accounts)
		confirmCalls := 0
		env := newCommitteeVerifierAdapterEnv(t, program, &deployer, client, func([]solana.Instruction, ...solcommonutil.TxModifier) error {
			confirmCalls++
			return nil
		})

		batchOps, err := adapter.ApplyAllowlistUpdates(t.Context(), env, committeeVerifierAdapterTestChainSelector, committeeVerifierAdapterTestQualifier, destChainSelector, true, nil, nil)
		require.NoError(t, err)
		require.Nil(t, batchOps)
		require.Zero(t, confirmCalls)
	})

	t.Run("governed write returns its batch", func(t *testing.T) {
		program := solana.NewWallet().PublicKey()
		deployer := solana.NewWallet().PrivateKey
		owner := solana.NewWallet().PublicKey()
		sender := solana.NewWallet().PublicKey()
		configPDA, _, err := state.FindConfigPDA(program)
		require.NoError(t, err)
		remotePDA, _, err := state.FindCVRemoteChainConfigPDA(destChainSelector, program)
		require.NoError(t, err)
		accounts := map[solana.PublicKey][]byte{
			configPDA: marshalCommitteeVerifierAdapterAccount(t, committee_verifier.Account_Config, committee_verifier.Config{Owner: owner, AllowlistAdmin: owner}),
			remotePDA: marshalCommitteeVerifierAdapterAccount(t, committee_verifier.Account_RemoteChainConfig, committee_verifier.RemoteChainConfig{
				RemoteChainSelector: destChainSelector,
				AllowlistConfig:     committee_verifier.RemoteChainAllowlistConfig{AllowlistEnabled: true},
			}),
		}
		client := newCommitteeVerifierAdapterClient(t, program, accounts)
		confirmCalls := 0
		env := newCommitteeVerifierAdapterEnv(t, program, &deployer, client, func([]solana.Instruction, ...solcommonutil.TxModifier) error {
			confirmCalls++
			return nil
		})

		batchOps, err := adapter.ApplyAllowlistUpdates(t.Context(), env, committeeVerifierAdapterTestChainSelector, committeeVerifierAdapterTestQualifier, destChainSelector, true, []string{sender.String()}, nil)
		require.NoError(t, err)
		require.Zero(t, confirmCalls)
		require.Len(t, batchOps, 1)
		require.Equal(t, committeeVerifierAdapterTestChainSelector, uint64(batchOps[0].ChainSelector))
		require.Len(t, batchOps[0].Transactions, 1)
		require.Equal(t, program.String(), batchOps[0].Transactions[0].To)
	})
}

func TestParseRemoteChainConfig(t *testing.T) {
	t.Parallel()

	makeAccountData := func(t *testing.T, cfg committee_verifier.RemoteChainConfig) []byte {
		t.Helper()
		disc := committee_verifier.Account_RemoteChainConfig
		body, err := cfg.Marshal()
		require.NoError(t, err)
		return append(disc[:], body...)
	}

	t.Run("skips threshold-0 accounts", func(t *testing.T) {
		data := makeAccountData(t, committee_verifier.RemoteChainConfig{
			RemoteChainSelector: 42,
			SignerConfig:        committee_verifier.RemoteChainSignerConfig{Threshold: 0},
		})
		got, err := parseRemoteChainConfig(data)
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("parses active config", func(t *testing.T) {
		signer := [20]uint8{19: 1}
		data := makeAccountData(t, committee_verifier.RemoteChainConfig{
			RemoteChainSelector: 42,
			SignerConfig: committee_verifier.RemoteChainSignerConfig{
				Threshold: 2,
				Signers:   [][20]uint8{signer},
			},
		})
		got, err := parseRemoteChainConfig(data)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, uint64(42), got.SourceChainSelector)
		require.Equal(t, uint8(2), got.Threshold)
		require.Equal(t, []string{"0x0000000000000000000000000000000000000001"}, got.Signers)
	})

	t.Run("returns error on bad discriminator", func(t *testing.T) {
		_, err := parseRemoteChainConfig([]byte("not-an-account"))
		require.Error(t, err)
	})
}
