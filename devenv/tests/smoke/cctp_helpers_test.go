package smoke

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"strconv"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	evmcctpverifierops "github.com/smartcontractkit/chainlink-ccip/chains/evm/deployment/v2_1_0/operations/cctp_verifier"
	"github.com/smartcontractkit/chainlink-ccip/chains/evm/gobindings/generated/latest/cctp_verifier"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-ccv/verifier/pkg/token/cctp"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils/eth"
	soltokens "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	cctpverifierops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/cctpverifier"
	soldevenv "github.com/smartcontractkit/chainlink-ccip-solana/devenv"
)

// cctpInbound holds the message fields that Circle's Solana programs check on receive. The EVM
// mocks ignore them, so a Solana -> EVM message passes nil.
type cctpInbound struct {
	sourceTokenMessenger protocol.UnknownAddress
	amount               *big.Int
}

// cctpStandardFinality is Circle's finality threshold for a standard (finalized) transfer.
const cctpStandardFinality = 2000

func registerCCTPAttestation(
	t *testing.T,
	httpURL string,
	messageID [32]byte,
	messageSender protocol.UnknownAddress,
	receiver protocol.UnknownAddress,
	sourceSelector, destSelector uint64,
	sourceBurnToken, destTokenMessenger protocol.UnknownAddress,
	inbound *cctpInbound,
) {
	t.Helper()

	sourceDomain, err := soldevenv.CCTPDomainFor(sourceSelector)
	require.NoError(t, err)
	destDomain, err := soldevenv.CCTPDomainFor(destSelector)
	require.NoError(t, err)

	message := make([]byte, 412)
	binary.BigEndian.PutUint32(message[0:4], 1)
	binary.BigEndian.PutUint32(message[4:8], sourceDomain)
	binary.BigEndian.PutUint32(message[8:12], destDomain)
	copy(message[12:44], messageID[:])
	putCCTPBytes32(t, message[76:108], destTokenMessenger)
	binary.BigEndian.PutUint32(message[148:152], 1)
	putCCTPBytes32(t, message[152:184], sourceBurnToken)
	putCCTPBytes32(t, message[184:216], receiver)
	putCCTPBytes32(t, message[248:280], messageSender)
	copy(message[376:380], cctp.DefaultVerifierVersion)
	copy(message[380:412], messageID[:])
	if inbound != nil {
		putCCTPBytes32(t, message[44:76], inbound.sourceTokenMessenger)
		binary.BigEndian.PutUint32(message[140:144], cctpStandardFinality)
		binary.BigEndian.PutUint32(message[144:148], cctpStandardFinality)
		inbound.amount.FillBytes(message[216:248])
	}

	attester, err := eth.GetSignerFromPk(soldevenv.CCTPAttesterPrivateKey)
	require.NoError(t, err)
	attestation, err := testutils.AttestCCTP(message, []eth.Signer{attester})
	require.NoError(t, err)

	reqBody, err := json.Marshal(map[string]string{
		"sourceDomain": strconv.FormatUint(uint64(sourceDomain), 10),
		"messageID":    "0x" + hex.EncodeToString(messageID[:]),
		"status":       "complete",
		// The messageSender format must match the source chain's CCTP codec: Circle
		// returns Solana senders as base58, EVM senders as 0x-hex.
		"messageSender": cctpSenderString(messageSender),
		"message":       "0x" + hex.EncodeToString(message),
		"attestation":   "0x" + hex.EncodeToString(attestation),
	})
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		httpURL+"/cctp/v2/attestations",
		bytes.NewReader(reqBody),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "register CCTP attestation")
}

// cctpSenderString formats a CCTP message sender the way Circle reports it:
// base58 for Solana addresses, 0x-hex for everything else (EVM).
func cctpSenderString(sender protocol.UnknownAddress) string {
	if len(sender.Bytes()) == 32 {
		return solana.PublicKeyFromBytes(sender.Bytes()).String()
	}
	return sender.String()
}

func putCCTPBytes32(t *testing.T, dst []byte, address protocol.UnknownAddress) {
	t.Helper()
	require.Len(t, dst, 32)
	addressBytes := address.Bytes()
	require.LessOrEqual(t, len(addressBytes), len(dst))
	copy(dst[len(dst)-len(addressBytes):], addressBytes)
}

type solanaCCTPRefs struct {
	USDCMint        solana.PublicKey
	VerifierProgram solana.PublicKey
	// MessageSender is the pool signer PDA: it signs deposit_for_burn_with_hook, so Circle
	// records it as the burn's messageSender.
	MessageSender solana.PublicKey
}

// resolveSolanaCCTP reads the Solana CCTP addresses from the datastore. The verifier ref uses
// the canonical qualifier (qualifier == contract type).
func resolveSolanaCCTP(t *testing.T, ds datastore.DataStore, solSelector uint64) solanaCCTPRefs {
	t.Helper()

	usdcRef, err := solcommon.PoolRefByQualifier(
		ds, solSelector, string(solcommon.SPLTokens), soldevenv.USDCTokenQualifier,
	)
	require.NoError(t, err, "resolve Solana USDC mint")
	usdcMint, err := solana.PublicKeyFromBase58(usdcRef.Address)
	require.NoError(t, err)

	verifierRef, err := ds.Addresses().Get(datastore.NewAddressRefKey(
		solSelector,
		datastore.ContractType(cctpverifierops.ContractType),
		cctpverifierops.Version,
		string(cctpverifierops.ContractType),
	))
	require.NoError(t, err, "resolve Solana CCTP verifier program")
	verifierProgram, err := solana.PublicKeyFromBase58(verifierRef.Address)
	require.NoError(t, err)

	messageSender, err := soltokens.TokenPoolSignerAddress(usdcMint, verifierProgram)
	require.NoError(t, err)

	return solanaCCTPRefs{USDCMint: usdcMint, VerifierProgram: verifierProgram, MessageSender: messageSender}
}

// resolveEVMUSDC reads USDC off the deployed CCTPVerifier: on a real network it is Circle's
// token and has no CCIP datastore ref.
func resolveEVMUSDC(
	t *testing.T,
	ds datastore.DataStore,
	evmClient any,
	evmSelector uint64,
) common.Address {
	t.Helper()

	rpcClient := evmRPCClient(t, evmClient)

	refs := ds.Addresses().Filter(
		datastore.AddressRefByChainSelector(evmSelector),
		datastore.AddressRefByType(datastore.ContractType(evmcctpverifierops.ContractType)),
		datastore.AddressRefByVersion(evmcctpverifierops.Version),
	)
	require.Len(t, refs, 1, "expected exactly one EVM CCTPVerifier ref on chain %d", evmSelector)

	verifier, err := cctp_verifier.NewCCTPVerifier(
		common.HexToAddress(refs[0].Address), ethclient.NewClient(rpcClient),
	)
	require.NoError(t, err)

	staticCfg, err := verifier.GetStaticConfig(&bind.CallOpts{Context: t.Context()})
	require.NoError(t, err, "read CCTPVerifier static config")
	require.NotEqual(t, common.Address{}, staticCfg.UsdcToken, "verifier reports a zero USDC token")

	return staticCfg.UsdcToken
}

// resolveEVMCCTPVerifier reads the EVM CCTPVerifier's own address: it is the contract that calls
// ITokenMessenger.depositForBurnWithHook, so it is the CCTP message's messageSender on an
// EVM->Solana burn.
func resolveEVMCCTPVerifier(t *testing.T, ds datastore.DataStore, evmSelector uint64) common.Address {
	t.Helper()

	refs := ds.Addresses().Filter(
		datastore.AddressRefByChainSelector(evmSelector),
		datastore.AddressRefByType(datastore.ContractType(evmcctpverifierops.ContractType)),
		datastore.AddressRefByVersion(evmcctpverifierops.Version),
	)
	require.Len(t, refs, 1, "expected exactly one EVM CCTPVerifier ref on chain %d", evmSelector)

	return common.HexToAddress(refs[0].Address)
}
