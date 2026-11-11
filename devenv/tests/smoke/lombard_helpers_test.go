package smoke

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"testing"

	"github.com/Masterminds/semver/v3"
	ethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	gethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	evmlombardverifier "github.com/smartcontractkit/chainlink-ccip/chains/evm/gobindings/generated/latest/lombard_verifier"
	evmmocklombardbridge "github.com/smartcontractkit/chainlink-ccip/chains/evm/gobindings/generated/latest/mock_lombard_bridge"
	devenvcommon "github.com/smartcontractkit/chainlink-ccv/build/devenv/common"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	lombard "github.com/smartcontractkit/chainlink-ccv/verifier/pkg/token/lombard"
	cldfsolana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/deployment"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/mock_lombard_mailbox"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	soldevenv "github.com/smartcontractkit/chainlink-ccip-solana/devenv"
)

// lombardToken is one Solana mint / EVM pool pair under test: the Solana mint ref
// (type + qualifier), the EVM LombardTokenPool qualifier, the source amount to send, and the
// amount expected to arrive on the Solana destination (E2S only).
type lombardToken struct {
	mintType         string
	mintQualifier    string
	evmPoolQualifier string
	sendAmount       *big.Int
	destAmount       *big.Int
}

// lombardSolanaTokenBalance reads the owner's associated token account balance directly,
// deriving the ATA under the mint's owning token program (classic Token vs Token-2022).
// The shared TokenBalanceReader only derives classic ATAs, so Token-2022 mints read as
// missing there.
func lombardSolanaTokenBalance(ctx context.Context, t *testing.T, client *rpc.Client, owner, mint solana.PublicKey) *big.Int {
	t.Helper()
	mintAcc, err := client.GetAccountInfo(ctx, mint)
	require.NoError(t, err, "read mint account")
	require.NotNil(t, mintAcc, "mint account should exist")
	require.NotNil(t, mintAcc.Value, "mint account should exist")
	tokenProgram := mintAcc.Value.Owner
	ata, _, err := solana.FindProgramAddress([][]byte{owner.Bytes(), tokenProgram.Bytes(), mint.Bytes()},
		solana.SPLAssociatedTokenAccountProgramID)
	require.NoError(t, err, "derive associated token account")
	ataAcc, err := client.GetAccountInfo(ctx, ata)
	require.NoError(t, err, "read associated token account")
	if ataAcc == nil || ataAcc.Value == nil {
		return big.NewInt(0)
	}
	data := ataAcc.GetBinary()
	require.GreaterOrEqual(t, len(data), 72, "associated token account too short")
	return new(big.Int).SetUint64(binary.LittleEndian.Uint64(data[64:72]))
}

// lombardTransferAmount is 1 base unit: both Lombard mints are 8dp on both chains and no
// protocol minimum exists, so 10000 funded units give 10000 runs per token per direction.
// Fee-accuracy checks need >= 1000 units; at 1 the run proves wiring only.
var lombardTransferAmount = big.NewInt(1)

// resolveLombardEVMPoolToken reads the ERC-20 token address off a deployed EVM Lombard
// token pool via TokenPool.getToken(). On a real network the underlying token has no CCIP
// datastore ref, so it is read from the pool contract directly.
func resolveLombardEVMPoolToken(ctx context.Context, t *testing.T, rpcClient *gethrpc.Client, pool common.Address) common.Address {
	t.Helper()
	selector := crypto.Keccak256([]byte("getToken()"))[:4]
	var out hexutil.Bytes
	err := rpcClient.CallContext(ctx, &out, "eth_call", map[string]any{
		"to":   pool.Hex(),
		"data": hexutil.Bytes(selector),
	}, "latest")
	require.NoError(t, err, "eth_call pool.getToken() on %s", pool.Hex())
	require.Len(t, out, 32, "pool.getToken() should return one 32-byte word")
	return common.BytesToAddress(out)
}

// registerLombardAttestation stands in for Lombard's off-chain attestation API: it registers
// attestation under messageHash on the fake service so the token-verifier has something to
// fetch and hand back as verify()'s verifier_results.
func registerLombardAttestation(t *testing.T, fakeURL string, messageHash []byte, attestation string) {
	t.Helper()
	reqBody, err := json.Marshal(map[string]string{
		"messageHash": "0x" + hex.EncodeToString(messageHash),
		"attestation": attestation,
		"status":      string(lombard.AttestationStatusApproved),
	})
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		fakeURL+"/lombard/v1/attestations",
		bytes.NewReader(reqBody),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "register lombard attestation")
}

// lombardEVMAttestationArgs mirrors chainlink-ccv's own LombardAttestationArgs
// (build/devenv/tests/e2e/lombard_helpers.go), used by its EVM<->EVM Lombard smoke test to build
// a rawPayload that satisfies EVM's LombardVerifier.sol _validatePayload/_validateEnvelope.
type lombardEVMAttestationArgs struct {
	// Sender is the source chain sender address (32 bytes for Solana), left-padded to 32 in payload.
	Sender []byte
	// DestToken is the destination (EVM) token address (20 bytes), left-padded to 32.
	DestToken []byte
	// Receiver is the token receiver (20 bytes EVM address), left-padded to 32.
	Receiver []byte
	// Amount is the token amount; must match message.tokenTransfer[0].amount exactly.
	Amount *big.Int
	// MessageID is the CCIP message ID; the contract checks it against the mailbox's
	// bridgedMessage (set via SetLombardMailboxBridgedMessageIfSupported).
	MessageID protocol.Bytes32
	// SourceBridge is the source chain's Lombard bridge (32 bytes for Solana's mock-lombard-bridge
	// program), carried as the GMP envelope sender. Must equal the remoteBridgeSender configured
	// on the destination verifier's path for the source chain selector.
	SourceBridge []byte
	// DestBridge is the destination (EVM) chain's Lombard bridge address, carried as the GMP
	// envelope recipient. Must equal the bridge the destination verifier was deployed against.
	DestBridge []byte
}

// buildLombardAttestationForEVM constructs a Lombard attestation whose rawPayload is compatible
// with LombardVerifier.sol verifyMessage(), mirroring chainlink-ccv's buildLombardAttestation helper.
func buildLombardAttestationForEVM(args lombardEVMAttestationArgs) string {
	versionTag := lombard.DefaultVerifierVersion // VERSION_TAG_V2_1_0 = bytes4(keccak256("LombardVerifier 2.1.0"))

	msgBody := make([]byte, 165)
	msgBody[0] = 2 // SUPPORTED_BRIDGE_MSG_VERSION
	copy(msgBody[1:33], common.LeftPadBytes(args.DestToken, 32))
	copy(msgBody[33:65], common.LeftPadBytes(args.Sender, 32))
	copy(msgBody[65:97], common.LeftPadBytes(args.Receiver, 32))
	if args.Amount == nil {
		args.Amount = new(big.Int)
	}
	args.Amount.FillBytes(msgBody[97:129])
	copy(msgBody[129:133], versionTag)
	copy(msgBody[133:165], args.MessageID[:])

	zeroBytes32 := [32]byte{}
	zeroAddr := common.Address{}
	envelopeSender := [32]byte{}
	copy(envelopeSender[:], common.LeftPadBytes(args.SourceBridge, 32))
	envelopeRecipient := common.BytesToAddress(args.DestBridge)

	bytes32Type, err := ethabi.NewType("bytes32", "", nil)
	if err != nil {
		panic("abi bytes32: " + err.Error())
	}
	uint256Type, err := ethabi.NewType("uint256", "", nil)
	if err != nil {
		panic("abi uint256: " + err.Error())
	}
	addressType, err := ethabi.NewType("address", "", nil)
	if err != nil {
		panic("abi address: " + err.Error())
	}
	bytesType, err := ethabi.NewType("bytes", "", nil)
	if err != nil {
		panic("abi bytes: " + err.Error())
	}

	packArgs := ethabi.Arguments{
		{Type: bytes32Type},
		{Type: uint256Type},
		{Type: bytes32Type},
		{Type: addressType},
		{Type: addressType},
		{Type: bytesType},
	}
	packed, err := packArgs.Pack(zeroBytes32, big.NewInt(0), envelopeSender, envelopeRecipient, zeroAddr, msgBody)
	if err != nil {
		panic("lombard payload ABI pack: " + err.Error())
	}
	rawPayload := append(append([]byte(nil), versionTag...), packed...)
	proof := []byte{}

	attestationArgs := ethabi.Arguments{{Type: bytesType}, {Type: bytesType}}
	attestation, err := attestationArgs.Pack(rawPayload, proof)
	if err != nil {
		panic("lombard attestation ABI pack: " + err.Error())
	}
	return "0x" + hex.EncodeToString(attestation)
}

// deliverLombardMessage stands in for Lombard's own off-chain infra, which is what calls
// deliver_message on the real Solana mailbox ahead of time. It reads the payload hash
// the EVM bridge's deposit call (inside LombardVerifier.forwardToVerifier) minted for this
// message, marks it Delivered on the Solana mock mailbox, and registers that same hash as the
// attestation the fake Lombard API will hand back to the token-verifier.
func deliverLombardMessage(
	ctx context.Context,
	t *testing.T,
	fakeURL string,
	ds datastore.DataStore,
	cldfEnv *deployment.Environment,
	solChainCLDF cldfsolana.Chain,
	evmSelector, solSelector uint64,
	messageID protocol.Bytes32,
	recipient solana.PublicKey,
	amount uint64,
) {
	t.Helper()

	evmBridgeRef, err := ds.Addresses().Get(datastore.NewAddressRefKey(
		evmSelector, datastore.ContractType(soldevenv.MockLombardBridgeContractType),
		semver.MustParse("2.0.0"), devenvcommon.LombardContractsQualifier,
	))
	require.NoError(t, err, "resolve EVM lombard bridge")
	evmChainCLDF, ok := cldfEnv.BlockChains.EVMChains()[evmSelector]
	require.True(t, ok, "EVM chain (selector %d) not found in CLDF environment", evmSelector)
	evmLombardBridge, err := evmmocklombardbridge.NewMockLombardBridge(
		common.HexToAddress(evmBridgeRef.Address), evmChainCLDF.Client,
	)
	require.NoError(t, err, "bind evm lombard bridge")
	payloadHash, err := evmLombardBridge.SLastPayloadHash(&bind.CallOpts{Context: ctx})
	require.NoError(t, err, "read s_lastPayloadHash from evm lombard bridge")

	hexHash := "0x" + hex.EncodeToString(payloadHash[:])
	registerLombardAttestation(t, fakeURL, payloadHash[:], hexHash)

	// The real mailbox rejects a handler that differs from the message's destination caller, and
	// the EVM verifier writes its path.allowedCaller there. Read it from the deployed verifier so
	// the lane configuration is checked end to end.
	evmVerifierRefs := ds.Addresses().Filter(
		datastore.AddressRefByChainSelector(evmSelector),
		datastore.AddressRefByType("LombardVerifier"),
	)
	require.Len(t, evmVerifierRefs, 1, "resolve EVM lombard verifier")
	evmLombardVerifier, err := evmlombardverifier.NewLombardVerifier(
		common.HexToAddress(evmVerifierRefs[0].Address), evmChainCLDF.Client,
	)
	require.NoError(t, err, "bind evm lombard verifier")
	path, err := evmLombardVerifier.GetPath(&bind.CallOpts{Context: ctx}, solSelector)
	require.NoError(t, err, "read solana path from evm lombard verifier")
	destinationCaller := path.AllowedCaller

	solMailboxRef, err := ds.Addresses().Get(datastore.NewAddressRefKey(
		solSelector, datastore.ContractType(solcommon.LombardMailboxContractType),
		solcommon.LombardChainContractsVersion, "",
	))
	require.NoError(t, err, "resolve solana mock lombard mailbox")
	solMailbox, err := solana.PublicKeyFromBase58(solMailboxRef.Address)
	require.NoError(t, err)
	messageInfoPDA, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("message"), payloadHash[:]}, solMailbox,
	)
	require.NoError(t, err, "derive message info PDA")

	// Message binding: version_tag(4) || messageID(32), matching pack_message_binding /
	// lombard_verifier_version_tag on the Solana side and LombardVerifier's versionTag() on
	// EVM - both must derive from the same string
	var binding [36]uint8
	copy(binding[0:4], lombard.DefaultVerifierVersion)
	copy(binding[4:36], messageID[:])
	markDeliveredIx, err := mock_lombard_mailbox.NewMarkDeliveredInstruction(
		payloadHash, recipient, amount, &binding, &destinationCaller,
		solChainCLDF.DeployerKey.PublicKey(), messageInfoPDA, solana.SystemProgramID,
	)
	require.NoError(t, err, "build mark_delivered instruction")
	require.NoError(t, solChainCLDF.Confirm([]solana.Instruction{markDeliveredIx}), "confirm mark_delivered")
}
