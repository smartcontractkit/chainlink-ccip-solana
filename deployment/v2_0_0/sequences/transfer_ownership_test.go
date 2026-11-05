package sequences

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/Masterminds/semver/v3"
	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	solcommonutil "github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/common"
	deployops "github.com/smartcontractkit/chainlink-ccip/deployment/deploy"
	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	mcms_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	cldf_datastore "github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations/optest"
	mcms_types "github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	cctpverifierops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/cctpverifier"
	cvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
	executorops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/executor"
	feequoterops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
	lvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/lombardverifier"
	offrampops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/offramp"
	rmnremoteops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/rmnremote"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
)

const mcmsReaderTestSelector = uint64(12463857294658392847)

var (
	mcmsTestProgram     = solana.NewWallet().PublicKey()
	mcmsTestTimelock    = solana.NewWallet().PublicKey()
	mcmsTestProposerAC  = solana.NewWallet().PublicKey()
	mcmsTestCancellerAC = solana.NewWallet().PublicKey()
	mcmsTestBypasserAC  = solana.NewWallet().PublicKey()
)

// mcmsInstanceRefs returns the refs one MCMS deployment writes, at the given version and
// qualifier. The MCM program is shared by every instance on the chain, so it carries no
// qualifier.
func mcmsInstanceRefs(version, qualifier string) []cldf_datastore.AddressRef {
	refs := []cldf_datastore.AddressRef{
		{Type: cldf_datastore.ContractType(common_utils.RBACTimelock), Address: mcmsTestTimelock.String() + ".timelock", Qualifier: qualifier},
		{Type: cldf_datastore.ContractType(common_utils.ProposerManyChainMultisig), Address: mcmsTestProgram.String() + ".proposer", Qualifier: qualifier},
		{Type: cldf_datastore.ContractType(common_utils.CancellerManyChainMultisig), Address: mcmsTestProgram.String() + ".canceller", Qualifier: qualifier},
		{Type: cldf_datastore.ContractType(common_utils.BypasserManyChainMultisig), Address: mcmsTestProgram.String() + ".bypasser", Qualifier: qualifier},
		{Type: cldf_datastore.ContractType(solcommon.ProposerAccessControllerAccount), Address: mcmsTestProposerAC.String(), Qualifier: qualifier},
		{Type: cldf_datastore.ContractType(solcommon.CancellerAccessControllerAccount), Address: mcmsTestCancellerAC.String(), Qualifier: qualifier},
		{Type: cldf_datastore.ContractType(solcommon.BypasserAccessControllerAccount), Address: mcmsTestBypasserAC.String(), Qualifier: qualifier},
		{Type: cldf_datastore.ContractType(solcommon.McmProgramType), Address: mcmsTestProgram.String(), Qualifier: ""},
	}
	for i := range refs {
		refs[i].ChainSelector = mcmsReaderTestSelector
		refs[i].Version = semver.MustParse(version)
	}
	return refs
}

// mcmsReaderEnv builds an environment whose RPC endpoint fails every call. GetChainMetadata
// resolves and parses every ref it needs before touching the chain, so reaching the RPC
// stage - surfaced as "failed to get op count" - proves the refs resolved.
func mcmsReaderEnv(t *testing.T, refs ...cldf_datastore.AddressRef) deployment.Environment {
	t.Helper()
	ds := cldf_datastore.NewMemoryDataStore()
	for _, ref := range refs {
		require.NoError(t, ds.Addresses().Add(ref))
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": req.ID,
			"error": map[string]any{"code": -32000, "message": "stub: no chain"},
		})
	}))
	t.Cleanup(server.Close)
	return deployment.Environment{
		GetContext: t.Context,
		DataStore:  ds.Seal(),
		BlockChains: cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{
			mcmsReaderTestSelector: cldf_solana.Chain{Selector: mcmsReaderTestSelector, Client: rpc.New(server.URL)},
		}),
	}
}

// There is no v2 MCMS deployment: the reader must find the shared 1.6 MCMS that every
// environment carries, and must not resolve refs at any other version.
func TestMCMSReaderReadsThe16Refs(t *testing.T) {
	t.Parallel()
	adapter := &SolanaAdapter{}

	t.Run("1.6.0 refs resolve", func(t *testing.T) {
		t.Parallel()
		env := mcmsReaderEnv(t, mcmsInstanceRefs("1.6.0", common_utils.CLLQualifier)...)
		for _, action := range []mcms_types.TimelockAction{
			mcms_types.TimelockActionSchedule,
			mcms_types.TimelockActionCancel,
			mcms_types.TimelockActionBypass,
		} {
			input := mcms_utils.Input{TimelockAction: action, Qualifier: common_utils.CLLQualifier}

			timelock, err := adapter.GetTimelockRef(env, mcmsReaderTestSelector, input)
			require.NoError(t, err, action)
			require.Equal(t, mcmsTestTimelock.String()+".timelock", timelock.Address, action)

			mcm, err := adapter.GetMCMSRef(env, mcmsReaderTestSelector, input)
			require.NoError(t, err, action)
			require.Equal(t, mcmsTestProgram.String(), mcm.Address, action)

			_, err = adapter.GetChainMetadata(env, mcmsReaderTestSelector, input)
			require.ErrorContains(t, err, "failed to get op count", action)
		}
	})

	t.Run("2.0.0-only refs do not resolve", func(t *testing.T) {
		t.Parallel()
		env := mcmsReaderEnv(t, mcmsInstanceRefs("2.0.0", common_utils.CLLQualifier)...)
		input := mcms_utils.Input{TimelockAction: mcms_types.TimelockActionSchedule, Qualifier: common_utils.CLLQualifier}

		timelock, err := adapter.GetTimelockRef(env, mcmsReaderTestSelector, input)
		require.NoError(t, err)
		require.Empty(t, timelock.Address)

		mcm, err := adapter.GetMCMSRef(env, mcmsReaderTestSelector, input)
		require.NoError(t, err)
		require.Empty(t, mcm.Address)

		_, err = adapter.GetChainMetadata(env, mcmsReaderTestSelector, input)
		require.ErrorContains(t, err, "no ProposerManyChainMultiSig address")
	})

	// An environment deployed without MCMS (the devenv) has no refs at all: the readers
	// probe with empty refs and no error, while GetChainMetadata reports the missing MCM.
	t.Run("no MCMS deployed", func(t *testing.T) {
		t.Parallel()
		env := mcmsReaderEnv(t)
		input := mcms_utils.Input{TimelockAction: mcms_types.TimelockActionSchedule, Qualifier: common_utils.CLLQualifier}

		timelock, err := adapter.GetTimelockRef(env, mcmsReaderTestSelector, input)
		require.NoError(t, err)
		require.Empty(t, timelock.Address)

		mcm, err := adapter.GetMCMSRef(env, mcmsReaderTestSelector, input)
		require.NoError(t, err)
		require.Empty(t, mcm.Address)

		_, err = adapter.GetChainMetadata(env, mcmsReaderTestSelector, input)
		require.ErrorContains(t, err, "no ProposerManyChainMultiSig address")
	})
}

// An empty input qualifier defaults to the CLLCCIP instance instead of matching every
// MCMS instance in the store.
func TestMCMSReaderDefaultsEmptyQualifier(t *testing.T) {
	t.Parallel()
	env := mcmsReaderEnv(t, mcmsInstanceRefs("1.6.0", common_utils.CLLQualifier)...)
	adapter := &SolanaAdapter{}
	input := mcms_utils.Input{TimelockAction: mcms_types.TimelockActionSchedule, Qualifier: ""}

	timelock, err := adapter.GetTimelockRef(env, mcmsReaderTestSelector, input)
	require.NoError(t, err)
	require.Equal(t, common_utils.CLLQualifier, timelock.Qualifier)

	_, err = adapter.GetChainMetadata(env, mcmsReaderTestSelector, input)
	require.ErrorContains(t, err, "failed to get op count")
}

// With only another instance's refs in the store, the default CLLCCIP qualifier must not
// match them: mixing instances would anchor a proposal to the wrong MCM.
func TestMCMSReaderEmptyQualifierNeverMixesInstances(t *testing.T) {
	t.Parallel()
	env := mcmsReaderEnv(t, mcmsInstanceRefs("1.6.0", "RMNMCMS")...)
	adapter := &SolanaAdapter{}
	input := mcms_utils.Input{TimelockAction: mcms_types.TimelockActionSchedule, Qualifier: ""}

	timelock, err := adapter.GetTimelockRef(env, mcmsReaderTestSelector, input)
	require.NoError(t, err)
	require.Empty(t, timelock.Address, "the default CLLCCIP qualifier must not match another instance's refs")

	_, err = adapter.GetChainMetadata(env, mcmsReaderTestSelector, input)
	require.ErrorContains(t, err, "no ProposerManyChainMultiSig address")
	require.ErrorContains(t, err, `qualifier "CLLCCIP"`)
}

// A missing access-controller ref is an error naming the ref, not a panic on an empty
// address.
func TestGetChainMetadataMissingAccessControllerRef(t *testing.T) {
	t.Parallel()
	refs := slices.DeleteFunc(mcmsInstanceRefs("1.6.0", common_utils.CLLQualifier), func(ref cldf_datastore.AddressRef) bool {
		return ref.Type == cldf_datastore.ContractType(solcommon.BypasserAccessControllerAccount)
	})
	env := mcmsReaderEnv(t, refs...)
	adapter := &SolanaAdapter{}
	input := mcms_utils.Input{TimelockAction: mcms_types.TimelockActionSchedule, Qualifier: common_utils.CLLQualifier}

	var err error
	require.NotPanics(t, func() {
		_, err = adapter.GetChainMetadata(env, mcmsReaderTestSelector, input)
	})
	require.ErrorContains(t, err, "no BypasserAccessControllerAccount address")
}

func TestGetChainMetadataRejectsMalformedRefs(t *testing.T) {
	t.Parallel()
	refs := mcmsInstanceRefs("1.6.0", common_utils.CLLQualifier)
	for i := range refs {
		if refs[i].Type == cldf_datastore.ContractType(common_utils.ProposerManyChainMultisig) {
			refs[i].Address = "not-an-mcm-address"
		}
		if refs[i].Type == cldf_datastore.ContractType(solcommon.ProposerAccessControllerAccount) {
			refs[i].Address = "not-an-access-controller"
		}
	}
	adapter := &SolanaAdapter{}
	input := mcms_utils.Input{TimelockAction: mcms_types.TimelockActionSchedule, Qualifier: common_utils.CLLQualifier}

	var err error
	require.NotPanics(t, func() {
		_, err = adapter.GetChainMetadata(mcmsReaderEnv(t, refs...), mcmsReaderTestSelector, input)
	})
	require.ErrorContains(t, err, "failed to parse ProposerManyChainMultiSig address")

	// drop the malformed MCM ref so resolution reaches the access controllers
	refs = slices.DeleteFunc(refs, func(ref cldf_datastore.AddressRef) bool {
		return ref.Type == cldf_datastore.ContractType(common_utils.ProposerManyChainMultisig)
	})
	refs = append(refs, cldf_datastore.AddressRef{
		ChainSelector: mcmsReaderTestSelector,
		Type:          cldf_datastore.ContractType(common_utils.ProposerManyChainMultisig),
		Version:       semver.MustParse("1.6.0"),
		Qualifier:     common_utils.CLLQualifier,
		Address:       mcmsTestProgram.String() + ".proposer",
	})
	require.NotPanics(t, func() {
		_, err = adapter.GetChainMetadata(mcmsReaderEnv(t, refs...), mcmsReaderTestSelector, input)
	})
	require.ErrorContains(t, err, "failed to parse ProposerAccessControllerAccount address")
}

// ownableTestRefs returns a ref per program type the v2 flow hands to the timelock.
func ownableTestRefs() []cldf_datastore.AddressRef {
	types := []string{
		routerops.ContractType.String(),
		offrampops.ContractType.String(),
		feequoterops.ContractType.String(),
		rmnremoteops.ContractType.String(),
		executorops.ContractType.String(),
		cvops.ContractType.String(),
		cctpverifierops.ContractType.String(),
		lvops.ContractType.String(),
	}
	refs := make([]cldf_datastore.AddressRef, 0, len(types))
	for _, contractType := range types {
		refs = append(refs, cldf_datastore.AddressRef{
			ChainSelector: mcmsReaderTestSelector,
			Type:          cldf_datastore.ContractType(contractType),
			Version:       semver.MustParse("2.0.0"),
			Address:       solana.NewWallet().PublicKey().String(),
		})
	}
	return refs
}

// missingAccountStub answers every getAccountInfo with a null value (so config reads fall
// back to the deployer key) and fails every other call.
func missingAccountStub(t *testing.T) *rpc.Client {
	t.Helper()
	return rpcStub(t, nil, solana.PublicKey{})
}

// routerConfigStub answers getAccountInfo with a router Config account owned by owner,
// held by program, and fails every other call.
func routerConfigStub(t *testing.T, owner, program solana.PublicKey) *rpc.Client {
	t.Helper()
	data, err := bin.MarshalBorsh(ccip_router.Config{Owner: owner})
	require.NoError(t, err)
	return rpcStub(t, append(ccip_router.Account_Config[:], data...), program)
}

func rpcStub(t *testing.T, accountData []byte, accountOwner solana.PublicKey) *rpc.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if req.Method == "getAccountInfo" {
			var value any
			if accountData != nil {
				value = map[string]any{
					"data":       []string{base64.StdEncoding.EncodeToString(accountData), "base64"},
					"executable": false,
					"lamports":   1_000_000,
					"owner":      accountOwner.String(),
					"rentEpoch":  0,
					"space":      len(accountData),
				}
			}
			resp["result"] = map[string]any{"context": map[string]any{"slot": 1}, "value": value}
		} else {
			resp["error"] = map[string]any{"code": -32000, "message": "stub: no chain"}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(server.Close)
	return rpc.New(server.URL)
}

func ownershipTestChains(deployer *solana.PrivateKey, client *rpc.Client, confirm func([]solana.Instruction, ...solcommonutil.TxModifier) error) cldf_chain.BlockChains {
	return cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{
		mcmsReaderTestSelector: cldf_solana.Chain{Selector: mcmsReaderTestSelector, Client: client, DeployerKey: deployer, Confirm: confirm},
	})
}

// deploy.TransferToTimelock passes the timelock's datastore address ("program.seed") as
// ProposedOwner and leaves CurrentOwner empty; the accept step must then batch one
// operation per deployed program for the timelock to execute.
func TestSequenceAcceptOwnershipBatchesEveryOwnableProgram(t *testing.T) {
	deployer := solana.NewWallet().PrivateKey
	chains := ownershipTestChains(&deployer, nil, nil)
	input := deployops.TransferOwnershipPerChainInput{
		ChainSelector: mcmsReaderTestSelector,
		ContractRef:   ownableTestRefs(),
		CurrentOwner:  "", // what deploy.TransferToTimelock passes
		ProposedOwner: mcmsTestTimelock.String() + ".timelock",
	}

	out, err := cldf_ops.ExecuteSequence(optest.NewBundle(t), (&SolanaAdapter{}).SequenceAcceptOwnership(), chains, input)
	require.NoError(t, err)
	require.Len(t, out.Output.BatchOps, len(input.ContractRef), "one accept batch per program")
	for i, batch := range out.Output.BatchOps {
		require.Len(t, batch.Transactions, 1)
		require.Equal(t, input.ContractRef[i].Address, batch.Transactions[0].To)
	}
}

// Refs the v2 flow does not transfer are rejected with a clear error pointing at the flow
// that owns them, so the changeset cannot report success while a program silently stays
// with the deployer key.
func TestSequenceOwnershipRejectsNonOwnableTypes(t *testing.T) {
	mcmsRef := cldf_datastore.AddressRef{ChainSelector: mcmsReaderTestSelector, Type: cldf_datastore.ContractType(solcommon.AccessControllerProgramType), Version: semver.MustParse("1.6.0"), Address: solana.NewWallet().PublicKey().String()}
	timelockRef := cldf_datastore.AddressRef{ChainSelector: mcmsReaderTestSelector, Type: cldf_datastore.ContractType(common_utils.RBACTimelock), Version: semver.MustParse("1.6.0"), Address: mcmsTestTimelock.String() + ".timelock"}
	poolRef := cldf_datastore.AddressRef{ChainSelector: mcmsReaderTestSelector, Type: cldf_datastore.ContractType(common_utils.BurnMintTokenPool), Version: semver.MustParse("2.0.0"), Address: solana.NewWallet().PublicKey().String()}
	unknownRef := cldf_datastore.AddressRef{ChainSelector: mcmsReaderTestSelector, Type: "SomeFutureProgram", Version: semver.MustParse("2.0.0"), Address: solana.NewWallet().PublicKey().String()}

	tests := []struct {
		name    string
		ref     cldf_datastore.AddressRef
		wantErr string
	}{
		{"MCMS program rejected with policy", mcmsRef, "shared 1.6 infrastructure"},
		{"timelock rejected with policy", timelockRef, "shared 1.6 infrastructure"},
		{"token pool rejected towards UpdateAuthorities", poolRef, "UpdateAuthorities"},
		{"unknown type rejected", unknownRef, "unsupported contract type SomeFutureProgram"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployer := solana.NewWallet().PrivateKey
			chains := ownershipTestChains(&deployer, nil, nil)
			input := deployops.TransferOwnershipPerChainInput{
				ChainSelector: mcmsReaderTestSelector,
				ContractRef:   []cldf_datastore.AddressRef{tt.ref},
				ProposedOwner: mcmsTestTimelock.String() + ".timelock",
			}

			_, err := cldf_ops.ExecuteSequence(optest.NewBundle(t), (&SolanaAdapter{}).SequenceTransferOwnershipViaMCMS(), chains, input)
			require.ErrorContains(t, err, tt.wantErr, "transfer")

			_, err = cldf_ops.ExecuteSequence(optest.NewBundle(t), (&SolanaAdapter{}).SequenceAcceptOwnership(), chains, input)
			require.ErrorContains(t, err, tt.wantErr, "accept")
		})
	}
}

// With the deployer still owning the program and the empty CurrentOwner that
// deploy.TransferToTimelock passes, the transfer resolves the deployer as current owner
// and sends directly.
func TestSequenceTransferOwnershipEmptyCurrentOwnerResolvesToDeployer(t *testing.T) {
	var confirmed [][]solana.Instruction
	confirm := func(ixns []solana.Instruction, _ ...solcommonutil.TxModifier) error {
		confirmed = append(confirmed, ixns)
		return nil
	}
	deployer := solana.NewWallet().PrivateKey
	refs := ownableTestRefs()[:1]
	program := solana.MustPublicKeyFromBase58(refs[0].Address)
	chains := ownershipTestChains(&deployer, routerConfigStub(t, deployer.PublicKey(), program), confirm)
	input := deployops.TransferOwnershipPerChainInput{
		ChainSelector: mcmsReaderTestSelector,
		ContractRef:   refs,
		CurrentOwner:  "",
		ProposedOwner: mcmsTestTimelock.String() + ".timelock",
	}

	var err error
	require.NotPanics(t, func() {
		_, err = cldf_ops.ExecuteSequence(optest.NewBundle(t), (&SolanaAdapter{}).SequenceTransferOwnershipViaMCMS(), chains, input)
	})
	require.NoError(t, err)
	require.Len(t, confirmed, 1, "the deployer-owned transfer confirms directly, not via a proposal")
}

// When the timelock already owns the program, the transfer is batched for the proposal
// instead of sent with the deployer key.
func TestSequenceTransferOwnershipBatchesWhenGoverned(t *testing.T) {
	governed := solana.NewWallet().PublicKey()
	deployer := solana.NewWallet().PrivateKey
	refs := ownableTestRefs()[:1]
	program := solana.MustPublicKeyFromBase58(refs[0].Address)
	chains := ownershipTestChains(&deployer, routerConfigStub(t, governed, program), nil)
	input := deployops.TransferOwnershipPerChainInput{
		ChainSelector: mcmsReaderTestSelector,
		ContractRef:   refs,
		CurrentOwner:  governed.String(),
		ProposedOwner: mcmsTestTimelock.String() + ".timelock",
	}

	out, err := cldf_ops.ExecuteSequence(optest.NewBundle(t), (&SolanaAdapter{}).SequenceTransferOwnershipViaMCMS(), chains, input)
	require.NoError(t, err)
	require.Len(t, out.Output.BatchOps, 1)
	require.Len(t, out.Output.BatchOps[0].Transactions, 1)
	require.Equal(t, input.ContractRef[0].Address, out.Output.BatchOps[0].Transactions[0].To)
}

// A CurrentOwner that disagrees with the on-chain authority is a hard error.
func TestSequenceTransferOwnershipRejectsOwnerMismatch(t *testing.T) {
	deployer := solana.NewWallet().PrivateKey
	chains := ownershipTestChains(&deployer, missingAccountStub(t), nil)
	input := deployops.TransferOwnershipPerChainInput{
		ChainSelector: mcmsReaderTestSelector,
		ContractRef:   ownableTestRefs()[:1],
		CurrentOwner:  solana.NewWallet().PublicKey().String(), // config is missing, so the authority is the deployer
		ProposedOwner: mcmsTestTimelock.String() + ".timelock",
	}

	_, err := cldf_ops.ExecuteSequence(optest.NewBundle(t), (&SolanaAdapter{}).SequenceTransferOwnershipViaMCMS(), chains, input)
	require.ErrorContains(t, err, "does not match on-chain authority")
}

func TestShouldAcceptOwnershipWithTransferOwnership(t *testing.T) {
	deployer := solana.NewWallet().PrivateKey
	env := deployment.Environment{BlockChains: ownershipTestChains(&deployer, nil, nil)}
	adapter := &SolanaAdapter{}

	// deploy.TransferToTimelock leaves CurrentOwner empty: the deployer owns freshly
	// deployed contracts, so accept must run.
	accept, err := adapter.ShouldAcceptOwnershipWithTransferOwnership(env, deployops.TransferOwnershipPerChainInput{ChainSelector: mcmsReaderTestSelector})
	require.NoError(t, err)
	require.True(t, accept)

	accept, err = adapter.ShouldAcceptOwnershipWithTransferOwnership(env, deployops.TransferOwnershipPerChainInput{
		ChainSelector: mcmsReaderTestSelector,
		CurrentOwner:  deployer.PublicKey().String(),
	})
	require.NoError(t, err)
	require.True(t, accept)

	accept, err = adapter.ShouldAcceptOwnershipWithTransferOwnership(env, deployops.TransferOwnershipPerChainInput{
		ChainSelector: mcmsReaderTestSelector,
		CurrentOwner:  solana.NewWallet().PublicKey().String(),
	})
	require.NoError(t, err)
	require.False(t, accept)

	_, err = adapter.ShouldAcceptOwnershipWithTransferOwnership(env, deployops.TransferOwnershipPerChainInput{
		ChainSelector: mcmsReaderTestSelector,
		CurrentOwner:  "not-a-key",
	})
	require.ErrorContains(t, err, "failed to parse current owner")
}

var errReadFailed = errors.New("read failed")

func TestRefsNotOwnedBy(t *testing.T) {
	t.Parallel()
	owner := solana.NewWallet().PublicKey()
	other := solana.NewWallet().PublicKey()
	refs := []cldf_datastore.AddressRef{{Address: "a"}, {Address: "b"}, {Address: "c"}}
	authorityOf := func(ref cldf_datastore.AddressRef) (solana.PublicKey, error) {
		if ref.Address == "b" {
			return owner, nil
		}
		return other, nil
	}

	kept, err := refsNotOwnedBy(refs, owner, authorityOf)
	require.NoError(t, err)
	require.Equal(t, []cldf_datastore.AddressRef{{Address: "a"}, {Address: "c"}}, kept, "the already-owned ref is dropped")

	_, err = refsNotOwnedBy(refs, owner, func(cldf_datastore.AddressRef) (solana.PublicKey, error) {
		return solana.PublicKey{}, errReadFailed
	})
	require.ErrorIs(t, err, errReadFailed)
}
