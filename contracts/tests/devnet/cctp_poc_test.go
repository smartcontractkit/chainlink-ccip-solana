//go:build devnet
// +build devnet

package contracts

// TestCctpPoc bridges USDC from Solana devnet to ETH Sepolia via Circle CCTP v2:
// it burns USDC on Solana (deposit_for_burn_with_hook, standard transfer), polls
// Circle's sandbox Iris API for the attestation, submits receiveMessage on the
// Sepolia MessageTransmitterV2, and asserts the recipient balance increased.
//
// Prerequisites (see devnet.config.yaml):
//   - private_keys.solana: funded with devnet SOL (fees + event-account rent) and
//     at least `amount` devnet USDC in its ATA (https://faucet.circle.com)
//   - private_keys.sepolia: funded with Sepolia ETH for receiveMessage gas
//
// Run with:
//
//	go test -tags devnet ./contracts/tests/devnet/ -run TestCctpPoc -v -timeout 20m

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	message_transmitter_v2 "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/cctp_message_transmitter_v2"
	token_messenger_minter_v2 "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/cctp_token_messenger_minter_v2"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/cctp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
)

const messageTransmitterV2ABI = `[{"inputs":[{"internalType":"bytes","name":"message","type":"bytes"},{"internalType":"bytes","name":"attestation","type":"bytes"}],"name":"receiveMessage","outputs":[{"internalType":"bool","name":"success","type":"bool"}],"stateMutability":"nonpayable","type":"function"}]`

const tokenMessengerV2ABI = `[{"inputs":[{"internalType":"uint256","name":"amount","type":"uint256"},{"internalType":"uint32","name":"destinationDomain","type":"uint32"},{"internalType":"bytes32","name":"mintRecipient","type":"bytes32"},{"internalType":"address","name":"burnToken","type":"address"},{"internalType":"bytes32","name":"destinationCaller","type":"bytes32"},{"internalType":"uint256","name":"maxFee","type":"uint256"},{"internalType":"uint32","name":"minFinalityThreshold","type":"uint32"},{"internalType":"bytes","name":"hookData","type":"bytes"}],"name":"depositForBurnWithHook","outputs":[],"stateMutability":"nonpayable","type":"function"}]`

const erc20BalanceOfABI = `[{"inputs":[{"internalType":"address","name":"account","type":"address"}],"name":"balanceOf","outputs":[{"internalType":"uint256","name":"","type":"uint256"}],"stateMutability":"view","type":"function"},{"inputs":[{"internalType":"address","name":"spender","type":"address"},{"internalType":"uint256","name":"amount","type":"uint256"}],"name":"approve","outputs":[{"internalType":"bool","name":"","type":"bool"}],"stateMutability":"nonpayable","type":"function"}]`

func TestCctpPoc(t *testing.T) {
	ctx := t.Context()
	devnet := getDevnetValues(t)

	const amount = uint64(10_000) // 0.01 USDC (6 decimals)

	// The shared cctp helpers derive PDAs from the config-package program IDs, so
	// the devnet config must agree with them.
	require.Equal(t, config.CctpMessageTransmitterV2, devnet.Solana.MessageTransmitterV2)
	require.Equal(t, config.CctpTokenMessengerMinterV2, devnet.Solana.TokenMessengerMinterV2)
	message_transmitter_v2.ProgramID = devnet.Solana.MessageTransmitterV2
	token_messenger_minter_v2.ProgramID = devnet.Solana.TokenMessengerMinterV2

	// The EVM recipient address, left-padded to bytes32 as CCTP encodes it.
	var recipientBytes32 [32]byte
	copy(recipientBytes32[12:], devnet.EVM.UserAddress.Bytes())
	mintRecipient := solana.PublicKeyFromBytes(recipientBytes32[:])

	mtPDAs := cctp.GetMessageTransmitterV2PDAs(t)
	tmmPDAs := cctp.GetTokenMessengerMinterV2PDAs(t, devnet.EVM.Domain, devnet.Solana.USDC, devnet.EVM.USDC.Bytes())
	denylistPDA := cctp.GetDenylistV2PDA(t, devnet.Solana.User.PublicKey())
	usdcATA, _, err := tokens.FindAssociatedTokenAddress(solana.TokenProgramID, devnet.Solana.USDC, devnet.Solana.User.PublicKey())
	require.NoError(t, err)

	messageSentEventKeypair, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	// State shared between the sequential subtests below.
	var solanaTxSig string
	var irisMessage, irisAttestation []byte

	t.Run("Deposit for burn on Solana", func(t *testing.T) {
		ix, err := token_messenger_minter_v2.NewDepositForBurnWithHookInstruction(
			token_messenger_minter_v2.DepositForBurnWithHookParams{
				Amount:               amount,
				DestinationDomain:    devnet.EVM.Domain,
				MintRecipient:        mintRecipient,
				DestinationCaller:    solana.PublicKey{}, // zero: anyone may relay on Sepolia
				MaxFee:               0,                  // standard transfer carries no fee
				MinFinalityThreshold: 2000,               // standard transfer (finalized)
				HookData:             []byte("something"),
			},
			devnet.Solana.User.PublicKey(), // owner
			devnet.Solana.User.PublicKey(), // event rent payer
			tmmPDAs.AuthorityPda,
			usdcATA,
			denylistPDA,
			mtPDAs.MessageTransmitter,
			tmmPDAs.TokenMessenger,
			tmmPDAs.RemoteTokenMessenger,
			tmmPDAs.TokenMinter,
			tmmPDAs.LocalToken,
			devnet.Solana.USDC,
			messageSentEventKeypair.PublicKey(),
			mtPDAs.Program,
			tmmPDAs.Program,
			solana.TokenProgramID,
			solana.SystemProgramID,
			tmmPDAs.EventAuthority,
			tmmPDAs.Program,
		)
		require.NoError(t, err)

		// deposit_for_burn_with_hook CPIs into the message transmitter's send_message,
		// whose CPI event needs the transmitter's event authority + program appended.
		genericIx, ok := ix.(*solana.GenericInstruction)
		require.True(t, ok)
		genericIx.AccountValues = append(genericIx.AccountValues,
			solana.NewAccountMeta(mtPDAs.EventAuthority, false, false),
			solana.NewAccountMeta(mtPDAs.Program, false, false),
		)

		result := testutils.SendAndConfirm(ctx, t, devnet.Solana.Client, []solana.Instruction{genericIx},
			devnet.Solana.User, config.DefaultCommitment, common.AddSigners(messageSentEventKeypair))
		require.NotNil(t, result)
		tx, err := result.Transaction.GetTransaction()
		require.NoError(t, err)
		solanaTxSig = tx.Signatures[0].String()
		t.Log("Burn transaction signature:", solanaTxSig)

		var messageSent message_transmitter_v2.MessageSent
		require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, devnet.Solana.Client,
			messageSentEventKeypair.PublicKey(), config.DefaultCommitment, &messageSent))
		t.Logf("MessageSent: rentPayer=%s createdAt=%d message=%s",
			messageSent.RentPayer, messageSent.CreatedAt, hex.EncodeToString(messageSent.Message))
	})

	t.Run("Await Circle attestation", func(t *testing.T) {
		require.NotEmpty(t, solanaTxSig, "deposit subtest must have succeeded")
		irisMessage, irisAttestation = awaitCircleAttestation(t, devnet.IrisAPIURL, devnet.Solana.Domain, solanaTxSig)
	})

	t.Run("Receive on Sepolia", func(t *testing.T) {
		require.NotEmpty(t, irisMessage, "attestation subtest must have succeeded")

		usdcAbi, err := abi.JSON(strings.NewReader(erc20BalanceOfABI))
		require.NoError(t, err)
		usdc := bind.NewBoundContract(devnet.EVM.USDC, usdcAbi, devnet.EVM.Client, devnet.EVM.Client, devnet.EVM.Client)

		balanceOf := func() *big.Int {
			var out []interface{}
			require.NoError(t, usdc.Call(&bind.CallOpts{Context: ctx}, &out, "balanceOf", devnet.EVM.UserAddress))
			return abi.ConvertType(out[0], new(big.Int)).(*big.Int)
		}
		balanceBefore := balanceOf()
		t.Log("Sepolia USDC balance before:", balanceBefore)

		mtAbi, err := abi.JSON(strings.NewReader(messageTransmitterV2ABI))
		require.NoError(t, err)
		messageTransmitter := bind.NewBoundContract(devnet.EVM.MessageTransmitterV2, mtAbi, devnet.EVM.Client, devnet.EVM.Client, devnet.EVM.Client)

		auth, err := bind.NewKeyedTransactorWithChainID(devnet.EVM.UserKey, devnet.EVM.ChainID)
		require.NoError(t, err)
		auth.Context = ctx

		evmTx, err := messageTransmitter.Transact(auth, "receiveMessage", irisMessage, irisAttestation)
		require.NoError(t, err)
		t.Log("Sepolia receiveMessage tx:", evmTx.Hash())

		receipt, err := bind.WaitMined(ctx, devnet.EVM.Client, evmTx)
		require.NoError(t, err)
		require.Equal(t, uint64(1), receipt.Status, "receiveMessage reverted")

		balanceAfter := balanceOf()
		t.Log("Sepolia USDC balance after:", balanceAfter)
		require.Equal(t, new(big.Int).SetUint64(amount), new(big.Int).Sub(balanceAfter, balanceBefore))
	})

	t.Run("Reclaim event account rent", func(t *testing.T) {
		// Do not run this, as it requires awaiting a few days before the event account can be reclaimed.
		// The code is left here for reference.
		t.Skip()

		require.NotEmpty(t, irisAttestation, "attestation subtest must have succeeded")

		ix, err := message_transmitter_v2.NewReclaimEventAccountInstruction(
			message_transmitter_v2.ReclaimEventAccountParams{
				Attestation:        irisAttestation,
				DestinationMessage: irisMessage,
			},
			devnet.Solana.User.PublicKey(), // payee, must match the original rent payer
			mtPDAs.MessageTransmitter,
			messageSentEventKeypair.PublicKey(),
		)
		require.NoError(t, err)

		// Best-effort: reclaiming may be subject to timing constraints on-chain, and
		// losing the ~0.003 SOL rent shouldn't fail an otherwise successful bridge.
		if _, err := common.SendAndConfirm(ctx, devnet.Solana.Client, []solana.Instruction{ix},
			devnet.Solana.User, config.DefaultCommitment); err != nil {
			t.Log("Reclaiming the event account failed; rent stays with the event account:", err)
		} else {
			t.Log("Event account rent reclaimed")
		}
	})
}

// TestCctpPocReverse bridges USDC from ETH Sepolia to Solana devnet via Circle CCTP v2:
// it approves and burns USDC on Sepolia (depositForBurn, fast transfer), polls Circle's
// sandbox Iris API for the attestation, submits receive_message on the Solana
// MessageTransmitterV2, and asserts the recipient ATA balance increased.
//
// Prerequisites (see devnet.config.yaml):
//   - private_keys.sepolia: funded with Sepolia ETH (approve + burn gas) and at
//     least `amount` Sepolia USDC (https://faucet.circle.com)
//   - private_keys.solana: funded with devnet SOL (receive fees + used-nonce rent)
//     and holding an initialized USDC ATA
//
// Run with:
//
//	go test -tags devnet ./contracts/tests/devnet/ -run TestCctpPocReverse -v -timeout 20m
func TestCctpPocReverse(t *testing.T) {
	ctx := t.Context()
	devnet := getDevnetValues(t)

	const amount = uint64(10_000) // 0.01 USDC (6 decimals)
	// Fast transfer (minFinalityThreshold 1000) charges a fee deducted from the
	// minted amount; cap it at 1% which is far above Circle's actual fast fee.
	const maxFee = amount / 100

	require.Equal(t, config.CctpMessageTransmitterV2, devnet.Solana.MessageTransmitterV2)
	require.Equal(t, config.CctpTokenMessengerMinterV2, devnet.Solana.TokenMessengerMinterV2)
	message_transmitter_v2.ProgramID = devnet.Solana.MessageTransmitterV2
	token_messenger_minter_v2.ProgramID = devnet.Solana.TokenMessengerMinterV2

	mtPDAs := cctp.GetMessageTransmitterV2PDAs(t)
	tmmPDAs := cctp.GetTokenMessengerMinterV2PDAs(t, devnet.EVM.Domain, devnet.Solana.USDC, devnet.EVM.USDC.Bytes())

	// On Solana the mint recipient is the USDC token account (not the wallet).
	usdcATA, _, err := tokens.FindAssociatedTokenAddress(solana.TokenProgramID, devnet.Solana.USDC, devnet.Solana.User.PublicKey())
	require.NoError(t, err)

	// The receive leg pays the fast-transfer fee to the token messenger's
	// fee recipient, into its USDC ATA.
	var tokenMessenger token_messenger_minter_v2.TokenMessenger
	require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, devnet.Solana.Client,
		tmmPDAs.TokenMessenger, config.DefaultCommitment, &tokenMessenger))
	feeRecipientATA, _, err := tokens.FindAssociatedTokenAddress(solana.TokenProgramID, devnet.Solana.USDC, tokenMessenger.FeeRecipient)
	require.NoError(t, err)
	t.Log("Fee recipient:", tokenMessenger.FeeRecipient, "ATA:", feeRecipientATA)

	// State shared between the sequential subtests below.
	var evmTxHash string
	var irisMessage, irisAttestation []byte

	t.Run("Approve and burn on Sepolia", func(t *testing.T) {
		auth, err := bind.NewKeyedTransactorWithChainID(devnet.EVM.UserKey, devnet.EVM.ChainID)
		require.NoError(t, err)
		auth.Context = ctx

		usdcAbi, err := abi.JSON(strings.NewReader(erc20BalanceOfABI))
		require.NoError(t, err)
		usdc := bind.NewBoundContract(devnet.EVM.USDC, usdcAbi, devnet.EVM.Client, devnet.EVM.Client, devnet.EVM.Client)

		approveTx, err := usdc.Transact(auth, "approve", devnet.EVM.TokenMessengerV2, new(big.Int).SetUint64(amount))
		require.NoError(t, err)
		receipt, err := bind.WaitMined(ctx, devnet.EVM.Client, approveTx)
		require.NoError(t, err)
		require.Equal(t, uint64(1), receipt.Status, "approve reverted")
		t.Log("Sepolia approve tx:", approveTx.Hash())

		tmAbi, err := abi.JSON(strings.NewReader(tokenMessengerV2ABI))
		require.NoError(t, err)
		tokenMessengerV2 := bind.NewBoundContract(devnet.EVM.TokenMessengerV2, tmAbi, devnet.EVM.Client, devnet.EVM.Client, devnet.EVM.Client)

		var mintRecipient [32]byte
		copy(mintRecipient[:], usdcATA.Bytes())

		// Hook data is carried in the burn message body but not executed on Solana
		// (hook execution is an EVM-destination feature). The EVM contract rejects
		// empty hook data, so any non-empty payload works.
		burnTx, err := tokenMessengerV2.Transact(auth, "depositForBurnWithHook",
			new(big.Int).SetUint64(amount),
			devnet.Solana.Domain, // destination domain
			mintRecipient,
			devnet.EVM.USDC,
			[32]byte{}, // destinationCaller zero: anyone may relay on Solana
			new(big.Int).SetUint64(maxFee),
			uint32(1000), // fast transfer
			[]byte("somethin"),
		)
		require.NoError(t, err)
		receipt, err = bind.WaitMined(ctx, devnet.EVM.Client, burnTx)
		require.NoError(t, err)
		require.Equal(t, uint64(1), receipt.Status, "depositForBurn reverted")
		evmTxHash = burnTx.Hash().Hex()
		t.Log("Sepolia burn tx:", evmTxHash)
	})

	t.Run("Await Circle attestation", func(t *testing.T) {
		require.NotEmpty(t, evmTxHash, "burn subtest must have succeeded")
		irisMessage, irisAttestation = awaitCircleAttestation(t, devnet.IrisAPIURL, devnet.EVM.Domain, evmTxHash)
	})

	t.Run("Receive on Solana", func(t *testing.T) {
		require.NotEmpty(t, irisMessage, "attestation subtest must have succeeded")

		_, balanceBefore, err := tokens.TokenBalance(ctx, devnet.Solana.Client, usdcATA, config.DefaultCommitment)
		require.NoError(t, err)
		t.Log("Solana USDC balance before:", balanceBefore)

		ix, err := message_transmitter_v2.NewReceiveMessageInstruction(
			message_transmitter_v2.ReceiveMessageParams{
				Message:     irisMessage,
				Attestation: irisAttestation,
			},
			devnet.Solana.User.PublicKey(), // payer
			devnet.Solana.User.PublicKey(), // caller
			mtPDAs.AuthorityPda,
			mtPDAs.MessageTransmitter,
			cctp.GetUsedNonceV2PDAFromMsg(t, irisMessage),
			tmmPDAs.Program, // receiver
			solana.SystemProgramID,
			mtPDAs.EventAuthority,
			mtPDAs.Program,
		)
		require.NoError(t, err)

		// Remaining accounts for the CPI into the token messenger minter's
		// handle_receive_{finalized,unfinalized}_message (identical account lists;
		// the transmitter routes on the attested finality threshold and prepends
		// its own authority PDA itself).
		genericIx, ok := ix.(*solana.GenericInstruction)
		require.True(t, ok)
		genericIx.AccountValues = append(genericIx.AccountValues,
			solana.Meta(tmmPDAs.TokenMessenger),
			solana.Meta(tmmPDAs.RemoteTokenMessenger),
			solana.Meta(tmmPDAs.TokenMinter),
			solana.Meta(tmmPDAs.LocalToken).WRITE(),
			solana.Meta(tmmPDAs.TokenPair),
			solana.Meta(feeRecipientATA).WRITE(),
			solana.Meta(usdcATA).WRITE(),
			solana.Meta(tmmPDAs.CustodyTokenAccount).WRITE(),
			solana.Meta(solana.TokenProgramID),
			solana.Meta(tmmPDAs.EventAuthority),
			solana.Meta(tmmPDAs.Program),
		)

		result := testutils.SendAndConfirm(ctx, t, devnet.Solana.Client, []solana.Instruction{genericIx},
			devnet.Solana.User, config.DefaultCommitment)
		require.NotNil(t, result)
		tx, err := result.Transaction.GetTransaction()
		require.NoError(t, err)
		t.Log("Solana receive_message tx:", tx.Signatures[0].String())

		_, balanceAfter, err := tokens.TokenBalance(ctx, devnet.Solana.Client, usdcATA, config.DefaultCommitment)
		require.NoError(t, err)
		received := uint64(balanceAfter - balanceBefore) //nolint:gosec // G115
		t.Logf("Solana USDC balance after: %d (received %d, fast-transfer fee %d)", balanceAfter, received, amount-received)
		require.True(t, received >= amount-maxFee, "received %d < amount %d - maxFee %d", received, amount, maxFee)
		require.True(t, received <= amount, "received %d > burned amount %d", received, amount)
	})
}

// awaitCircleAttestation polls Circle's Iris v2 API until the message burned in the
// given source-chain transaction is attested, returning the attested message bytes
// (authoritative over the source-chain ones, as Circle finalizes fee/finality fields)
// and the attestation itself.
func awaitCircleAttestation(t *testing.T, irisAPIURL string, sourceDomain uint32, txHash string) (message, attestation []byte) {
	t.Helper()

	type irisMessageEntry struct {
		Message     string `json:"message"`
		Attestation string `json:"attestation"`
		Status      string `json:"status"`
	}
	type irisResponse struct {
		Messages []irisMessageEntry `json:"messages"`
	}

	url := fmt.Sprintf("%s/v2/messages/%d?transactionHash=%s", irisAPIURL, sourceDomain, txHash)
	t.Log("Polling attestation:", url)

	httpClient := &http.Client{Timeout: 15 * time.Second}
	deadline := time.Now().Add(10 * time.Minute)
	for {
		require.True(t, time.Now().Before(deadline), "timed out waiting for Circle attestation")

		entry, err := func() (*irisMessageEntry, error) {
			resp, err := httpClient.Get(url)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("status %s", resp.Status)
			}
			var decoded irisResponse
			if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
				return nil, err
			}
			if len(decoded.Messages) == 0 {
				return nil, fmt.Errorf("no messages in response")
			}
			return &decoded.Messages[0], nil
		}()

		switch {
		case err != nil:
			t.Log("Attestation not ready:", err)
		case entry.Status != "complete":
			t.Log("Attestation status:", entry.Status)
		default:
			message, err = hex.DecodeString(strings.TrimPrefix(entry.Message, "0x"))
			require.NoError(t, err)
			attestation, err = hex.DecodeString(strings.TrimPrefix(entry.Attestation, "0x"))
			require.NoError(t, err)
			require.NotEmpty(t, message)
			require.NotEmpty(t, attestation)
			t.Log("Attestation retrieved successfully")
			return message, attestation
		}
		time.Sleep(10 * time.Second)
	}
}
