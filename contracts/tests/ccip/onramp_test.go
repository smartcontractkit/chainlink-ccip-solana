// onramp_test.go tests the outbound message sending flow via the Router's ccip_send instruction.
// This includes message validation, fee calculation, token lock/burn on send,
// and end-to-end outbound CCIP message lifecycle.
package contracts

import (
	"encoding/binary"
	"fmt"
	"maps"
	"math/big"
	"strings"
	"testing"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/base_token_pool"
	burnmint_tokenpool "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/burnmint_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/cctp_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/committee_verifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/example_ccip_sender"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/executor"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/fee_quoter"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/rmn_remote"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/cctp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
)

func TestOnRamp(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	env := testutils.NewTestEnv(t).WithTrafficConfig().WithCCTPv2().Build()

	token0Decimals := uint8(9)
	token1Decimals := uint8(18)
	v2TokenDecimals := uint8(9)
	usdcTokenDecimals := uint8(6)

	t.Run("OnRamp ccipSend", func(t *testing.T) {
		type UserFullNonce struct {
			user        uint64
			admin       uint64
			anotherUser uint64
		}
		NewUserFullNonces := func() UserFullNonce {
			return UserFullNonce{
				user:        0,
				admin:       0,
				anotherUser: 0,
			}
		}
		type ChainUserFullNonce struct {
			evm UserFullNonce
			svm UserFullNonce
		}
		fullNonces := ChainUserFullNonce{
			evm: NewUserFullNonces(),
			svm: NewUserFullNonces(),
		}

		t.Run("When sending to an invalid destination chain selector it fails", func(t *testing.T) {
			destinationChainSelector := uint64(189)
			destinationChainStatePDA, err := state.FindDestChainStatePDA(destinationChainSelector, config.CcipRouterProgram)
			require.NoError(t, err)
			fqDestChainPDA, _, err := state.FindFqDestChainPDA(destinationChainSelector, config.FeeQuoterProgram)
			require.NoError(t, err)
			message := ccip_router.Svm2AnyMessage{
				FeeToken:  env.WSOL.Mint,
				Receiver:  env.ValidEvmAbiReceiverAddress[:],
				Data:      []byte{4, 5, 6},
				ExtraArgs: []byte{},
			}

			instruction, err := ccip_router.NewCcipSendInstruction(
				destinationChainSelector,
				message,
				[]byte{},
				config.RouterConfigPDA,
				destinationChainStatePDA,
				env.NonceEvmPDA,
				testutils.User.PublicKey(),
				solana.SystemProgramID,
				env.WSOL.Program,
				env.WSOL.Mint,
				env.WSOL.UserATA,
				env.WSOL.BillingATA,
				config.BillingSignerPDA,
				config.FeeQuoterProgram,
				config.FqConfigPDA,
				fqDestChainPDA,
				env.WSOL.FqBillingCfgPDA,
				env.Link22.FqBillingCfgPDA,
				config.RMNRemoteProgram,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)

			require.NoError(t, err)
			result := testutils.SendAndFailWith(ctx, t, env.Client, []solana.Instruction{instruction}, testutils.User, config.DefaultCommitment, []string{"Error Code: " + common.AccountNotInitialized_AnchorError.String()})
			require.NotNil(t, result)
		})

		t.Run("When sending with an empty but enabled allowlist, it fails", func(t *testing.T) {
			var initialDestChain ccip_router.DestChain
			err := common.GetAccountDataBorshIntoAnchor(ctx, env.Client, config.EvmDestChainStatePDA, config.DefaultCommitment, &initialDestChain)
			require.NoError(t, err, "failed to get account info")
			modifiedDestChain := initialDestChain
			modifiedDestChain.Config.AllowListEnabled = true

			updateDestChainIx, err := ccip_router.NewUpdateDestChainConfigInstruction(
				config.EvmChainSelector,
				modifiedDestChain.Config,
				config.EvmDestChainStatePDA,
				config.RouterConfigPDA,
				testutils.CcipAdmin.PublicKey(),
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			result := testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{updateDestChainIx}, testutils.CcipAdmin, config.DefaultCommitment)
			require.NotNil(t, result)

			destinationChainSelector := config.EvmChainSelector
			destinationChainStatePDA := config.EvmDestChainStatePDA
			message := ccip_router.Svm2AnyMessage{
				FeeToken:  env.WSOL.Mint,
				Receiver:  env.ValidEvmAbiReceiverAddress[:],
				Data:      []byte{4, 5, 6},
				ExtraArgs: []byte{},
			}

			instruction, err := ccip_router.NewCcipSendInstruction(
				destinationChainSelector,
				message,
				[]byte{},
				config.RouterConfigPDA,
				destinationChainStatePDA,
				env.NonceEvmPDA,
				testutils.User.PublicKey(),
				solana.SystemProgramID,
				env.WSOL.Program,
				env.WSOL.Mint,
				env.WSOL.UserATA,
				env.WSOL.BillingATA,
				config.BillingSignerPDA,
				config.FeeQuoterProgram,
				config.FqConfigPDA,
				config.FqEvmDestChainPDA,
				env.WSOL.FqBillingCfgPDA,
				env.Link22.FqBillingCfgPDA,
				config.RMNRemoteProgram,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			instruction.(*solana.GenericInstruction).AccountValues[7].IsWritable = true
			result = testutils.SendAndFailWith(ctx, t, env.Client, []solana.Instruction{instruction}, testutils.User, config.DefaultCommitment, []string{"Error Code: SenderNotAllowed"})
			require.NotNil(t, result)

			// Restore the config to keep the test state-neutral
			updateDestChainIx, err = ccip_router.NewUpdateDestChainConfigInstruction(
				config.EvmChainSelector,
				initialDestChain.Config,
				config.EvmDestChainStatePDA,
				config.RouterConfigPDA,
				testutils.CcipAdmin.PublicKey(),
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			result = testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{updateDestChainIx}, testutils.CcipAdmin, config.DefaultCommitment)
			require.NotNil(t, result)
		})

		t.Run("When sending a Valid CCIP Message Emits CCIPMessageSent", func(t *testing.T) {
			destinationChainSelector := config.EvmChainSelector
			destinationChainStatePDA := config.EvmDestChainStatePDA
			message := ccip_router.Svm2AnyMessage{
				FeeToken:  env.WSOL.Mint,
				Receiver:  env.ValidEvmAbiReceiverAddress[:],
				Data:      []byte{4, 5, 6},
				ExtraArgs: []byte{},
			}

			instruction, err := ccip_router.NewCcipSendInstruction(
				destinationChainSelector,
				message,
				[]byte{},
				config.RouterConfigPDA,
				destinationChainStatePDA,
				env.NonceEvmPDA,
				testutils.User.PublicKey(),
				solana.SystemProgramID,
				env.WSOL.Program,
				env.WSOL.Mint,
				env.WSOL.UserATA,
				env.WSOL.BillingATA,
				config.BillingSignerPDA,
				config.FeeQuoterProgram,
				config.FqConfigPDA,
				config.FqEvmDestChainPDA,
				env.WSOL.FqBillingCfgPDA,
				env.Link22.FqBillingCfgPDA,
				config.RMNRemoteProgram,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			instruction.(*solana.GenericInstruction).AccountValues[7].IsWritable = true

			result := testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{instruction}, testutils.User, config.DefaultCommitment)
			require.NotNil(t, result)

			var chainStateAccount ccip_router.DestChain
			err = common.GetAccountDataBorshIntoAnchor(ctx, env.Client, destinationChainStatePDA, config.DefaultCommitment, &chainStateAccount)
			require.NoError(t, err, "failed to get account info")
			require.Equal(t, uint64(1), chainStateAccount.State.MessageNumber)

			var nonceCounterAccount ccip_router.Nonce
			err = common.GetAccountDataBorshIntoAnchor(ctx, env.Client, env.NonceEvmPDA, config.DefaultCommitment, &nonceCounterAccount)
			require.NoError(t, err, "failed to get account info")
			require.Equal(t, uint64(1), nonceCounterAccount.OrderedNonce)

			fullNonces.evm.user++
			require.Equal(t, fullNonces.evm.user, nonceCounterAccount.TotalNonce)

			ccipMessageSentEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, ccip_router.ParseEvent_CcipMessageSent)
			require.NoError(t, err)
			require.Equal(t, uint64(21), ccipMessageSentEvent.DestChainSelector)
			require.Equal(t, uint64(1), ccipMessageSentEvent.SequenceNumber)
			require.Equal(t, testutils.User.PublicKey(), ccipMessageSentEvent.Message.Sender)
			require.Equal(t, env.ValidEvmAbiReceiverAddress[:], ccipMessageSentEvent.Message.Receiver)
			data := [3]uint8{4, 5, 6}
			require.Equal(t, data[:], ccipMessageSentEvent.Message.Data)
			require.Equal(t, bin.Uint128{Lo: uint64(env.ValidFqDestChainConfig.DefaultTxGasLimit.V0), Hi: 0}, testutils.MustDeserializeExtraArgs(t, &ccip_common.GenericExtraArgsV2{}, ccipMessageSentEvent.Message.ExtraArgs, ccip.GenericExtraArgsV2Tag).GasLimit)
			require.False(t, testutils.MustDeserializeExtraArgs(t, &ccip_common.GenericExtraArgsV2{}, ccipMessageSentEvent.Message.ExtraArgs, ccip.GenericExtraArgsV2Tag).AllowOutOfOrderExecution)
			require.Equal(t, uint64(15), ccipMessageSentEvent.Message.Header.SourceChainSelector)
			require.Equal(t, uint64(21), ccipMessageSentEvent.Message.Header.DestChainSelector)
			require.Equal(t, uint64(1), ccipMessageSentEvent.Message.Header.SequenceNumber)
			require.Equal(t, uint64(1), ccipMessageSentEvent.Message.Header.Nonce)
			hash, err := ccip.HashSVMToAnyMessage(ccipMessageSentEvent.Message)
			require.NoError(t, err)
			require.Equal(t, hash, ccipMessageSentEvent.Message.Header.MessageId[:])

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)

			foundEvent := false
			for _, innerIx := range result.Meta.InnerInstructions {
				for _, ix := range innerIx.Instructions {
					programID, err := testutils.GetProgramID(tx, result.Meta, ix.ProgramIDIndex)
					require.NoError(t, err)
					if programID == config.RMNRemoteProgram && [8]byte(ix.Data[0:8]) == [8]byte(rmn_remote.Instruction_CpiEvent[:]) {
						foundEvent = true
						lenPrefix := binary.LittleEndian.Uint32(ix.Data[8:12])
						require.Equal(t, uint32(288), lenPrefix)
						eventData := ix.Data[12:]
						cpiEvent, err := ccip_router.ParseEvent_CcipMessageSent(eventData)
						require.NoError(t, err)
						require.Equal(t, ccipMessageSentEvent, cpiEvent)
						break
					}
				}
			}
			require.True(t, foundEvent, "Did not find CCIPMessageSent event emitted via CPI to RMNRemote")
		})

		t.Run("When sending a CCIP Message with ExtraArgs overrides Emits CCIPMessageSent", func(t *testing.T) {
			destinationChainSelector := config.EvmChainSelector
			destinationChainStatePDA := config.EvmDestChainStatePDA
			trueValue := true
			message := ccip_router.Svm2AnyMessage{
				FeeToken: env.WSOL.Mint,
				Receiver: env.ValidEvmAbiReceiverAddress[:],
				Data:     []byte{4, 5, 6},
				ExtraArgs: testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV2{
					GasLimit:                 bin.Uint128{Lo: 99, Hi: 0},
					AllowOutOfOrderExecution: trueValue,
				}, ccip.GenericExtraArgsV2Tag),
			}

			instruction, err := ccip_router.NewCcipSendInstruction(
				destinationChainSelector,
				message,
				[]byte{},
				config.RouterConfigPDA,
				destinationChainStatePDA,
				env.NonceEvmPDA,
				testutils.User.PublicKey(),
				solana.SystemProgramID,
				env.WSOL.Program,
				env.WSOL.Mint,
				env.WSOL.UserATA,
				env.WSOL.BillingATA,
				config.BillingSignerPDA,
				config.FeeQuoterProgram,
				config.FqConfigPDA,
				config.FqEvmDestChainPDA,
				env.WSOL.FqBillingCfgPDA,
				env.Link22.FqBillingCfgPDA,
				config.RMNRemoteProgram,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			instruction.(*solana.GenericInstruction).AccountValues[7].IsWritable = true

			result := testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{instruction}, testutils.User, config.DefaultCommitment)
			require.NotNil(t, result)

			var chainStateAccount ccip_router.DestChain
			err = common.GetAccountDataBorshIntoAnchor(ctx, env.Client, destinationChainStatePDA, config.DefaultCommitment, &chainStateAccount)
			require.NoError(t, err, "failed to get account info")
			require.Equal(t, uint64(2), chainStateAccount.State.MessageNumber)

			var nonceCounterAccount ccip_router.Nonce
			err = common.GetAccountDataBorshIntoAnchor(ctx, env.Client, env.NonceEvmPDA, config.DefaultCommitment, &nonceCounterAccount)
			require.NoError(t, err, "failed to get account info")
			require.Equal(t, uint64(1), nonceCounterAccount.OrderedNonce) // OOO does not increment ordered nonce

			fullNonces.evm.user++
			require.Equal(t, fullNonces.evm.user, nonceCounterAccount.TotalNonce)

			ccipMessageSentEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, ccip_router.ParseEvent_CcipMessageSent)
			require.NoError(t, err)
			require.Equal(t, uint64(21), ccipMessageSentEvent.DestChainSelector)
			require.Equal(t, uint64(2), ccipMessageSentEvent.SequenceNumber)
			require.Equal(t, testutils.User.PublicKey(), ccipMessageSentEvent.Message.Sender)
			require.Equal(t, env.ValidEvmAbiReceiverAddress[:], ccipMessageSentEvent.Message.Receiver)
			data := [3]uint8{4, 5, 6}
			require.Equal(t, data[:], ccipMessageSentEvent.Message.Data)
			require.Equal(t, bin.Uint128{Lo: 99, Hi: 0}, testutils.MustDeserializeExtraArgs(t, &ccip_common.GenericExtraArgsV2{}, ccipMessageSentEvent.Message.ExtraArgs, ccip.GenericExtraArgsV2Tag).GasLimit)
			require.True(t, testutils.MustDeserializeExtraArgs(t, &ccip_common.GenericExtraArgsV2{}, ccipMessageSentEvent.Message.ExtraArgs, ccip.GenericExtraArgsV2Tag).AllowOutOfOrderExecution)
			require.Equal(t, uint64(15), ccipMessageSentEvent.Message.Header.SourceChainSelector)
			require.Equal(t, uint64(21), ccipMessageSentEvent.Message.Header.DestChainSelector)
			require.Equal(t, uint64(2), ccipMessageSentEvent.Message.Header.SequenceNumber)
			require.Equal(t, uint64(0), ccipMessageSentEvent.Message.Header.Nonce) // OOO: nonce is 0
		})

		t.Run("When sending a CCIP Message with only gas limit ExtraArgs overrides Emits CCIPMessageSent", func(t *testing.T) {
			destinationChainSelector := config.EvmChainSelector
			destinationChainStatePDA := config.EvmDestChainStatePDA
			message := ccip_router.Svm2AnyMessage{
				FeeToken: env.WSOL.Mint,
				Receiver: env.ValidEvmAbiReceiverAddress[:],
				Data:     []byte{4, 5, 6},
				ExtraArgs: testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV2{
					GasLimit: bin.Uint128{Lo: 99, Hi: 0},
				}, ccip.GenericExtraArgsV2Tag),
			}

			instruction, err := ccip_router.NewCcipSendInstruction(
				destinationChainSelector,
				message,
				[]byte{},
				config.RouterConfigPDA,
				destinationChainStatePDA,
				env.NonceEvmPDA,
				testutils.User.PublicKey(),
				solana.SystemProgramID,
				env.WSOL.Program,
				env.WSOL.Mint,
				env.WSOL.UserATA,
				env.WSOL.BillingATA,
				config.BillingSignerPDA,
				config.FeeQuoterProgram,
				config.FqConfigPDA,
				config.FqEvmDestChainPDA,
				env.WSOL.FqBillingCfgPDA,
				env.Link22.FqBillingCfgPDA,
				config.RMNRemoteProgram,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			instruction.(*solana.GenericInstruction).AccountValues[7].IsWritable = true
			result := testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{instruction}, testutils.User, config.DefaultCommitment)
			require.NotNil(t, result)

			var chainStateAccount ccip_router.DestChain
			err = common.GetAccountDataBorshIntoAnchor(ctx, env.Client, destinationChainStatePDA, config.DefaultCommitment, &chainStateAccount)
			require.NoError(t, err, "failed to get account info")
			require.Equal(t, uint64(3), chainStateAccount.State.MessageNumber)

			var nonceCounterAccount ccip_router.Nonce
			err = common.GetAccountDataBorshIntoAnchor(ctx, env.Client, env.NonceEvmPDA, config.DefaultCommitment, &nonceCounterAccount)
			require.NoError(t, err, "failed to get account info")
			require.Equal(t, uint64(2), nonceCounterAccount.OrderedNonce)
			fullNonces.evm.user++
			require.Equal(t, fullNonces.evm.user, nonceCounterAccount.TotalNonce)

			ccipMessageSentEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, ccip_router.ParseEvent_CcipMessageSent)
			require.NoError(t, err)
			require.Equal(t, uint64(21), ccipMessageSentEvent.DestChainSelector)
			require.Equal(t, uint64(3), ccipMessageSentEvent.SequenceNumber)
			require.Equal(t, testutils.User.PublicKey(), ccipMessageSentEvent.Message.Sender)
			require.Equal(t, env.ValidEvmAbiReceiverAddress[:], ccipMessageSentEvent.Message.Receiver)
			data := [3]uint8{4, 5, 6}
			require.Equal(t, data[:], ccipMessageSentEvent.Message.Data)
			require.Equal(t, bin.Uint128{Lo: 99, Hi: 0}, testutils.MustDeserializeExtraArgs(t, &ccip_common.GenericExtraArgsV2{}, ccipMessageSentEvent.Message.ExtraArgs, ccip.GenericExtraArgsV2Tag).GasLimit)
			require.False(t, testutils.MustDeserializeExtraArgs(t, &ccip_common.GenericExtraArgsV2{}, ccipMessageSentEvent.Message.ExtraArgs, ccip.GenericExtraArgsV2Tag).AllowOutOfOrderExecution)
			require.Equal(t, uint64(15), ccipMessageSentEvent.Message.Header.SourceChainSelector)
			require.Equal(t, uint64(21), ccipMessageSentEvent.Message.Header.DestChainSelector)
			require.Equal(t, uint64(3), ccipMessageSentEvent.Message.Header.SequenceNumber)
			require.Equal(t, uint64(2), ccipMessageSentEvent.Message.Header.Nonce)
		})

		t.Run("When sending a CCIP Message with allow out of order ExtraArgs overrides Emits CCIPMessageSent", func(t *testing.T) {
			destinationChainSelector := config.EvmChainSelector
			destinationChainStatePDA := config.EvmDestChainStatePDA
			message := ccip_router.Svm2AnyMessage{
				FeeToken: env.WSOL.Mint,
				Receiver: env.ValidEvmAbiReceiverAddress[:],
				Data:     []byte{4, 5, 6},
				ExtraArgs: testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV2{
					AllowOutOfOrderExecution: true,
					GasLimit:                 bin.Uint128{Lo: 5000, Hi: 0},
				}, ccip.GenericExtraArgsV2Tag),
			}

			instruction, err := ccip_router.NewCcipSendInstruction(
				destinationChainSelector,
				message,
				[]byte{},
				config.RouterConfigPDA,
				destinationChainStatePDA,
				env.NonceEvmPDA,
				testutils.User.PublicKey(),
				solana.SystemProgramID,
				env.WSOL.Program,
				env.WSOL.Mint,
				env.WSOL.UserATA,
				env.WSOL.BillingATA,
				config.BillingSignerPDA,
				config.FeeQuoterProgram,
				config.FqConfigPDA,
				config.FqEvmDestChainPDA,
				env.WSOL.FqBillingCfgPDA,
				env.Link22.FqBillingCfgPDA,
				config.RMNRemoteProgram,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			instruction.(*solana.GenericInstruction).AccountValues[7].IsWritable = true
			result := testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{instruction}, testutils.User, config.DefaultCommitment)
			require.NotNil(t, result)

			var chainStateAccount ccip_router.DestChain
			err = common.GetAccountDataBorshIntoAnchor(ctx, env.Client, destinationChainStatePDA, config.DefaultCommitment, &chainStateAccount)
			require.NoError(t, err, "failed to get account info")
			require.Equal(t, uint64(4), chainStateAccount.State.MessageNumber)

			var nonceCounterAccount ccip_router.Nonce
			err = common.GetAccountDataBorshIntoAnchor(ctx, env.Client, env.NonceEvmPDA, config.DefaultCommitment, &nonceCounterAccount)
			require.NoError(t, err, "failed to get account info")
			require.Equal(t, uint64(2), nonceCounterAccount.OrderedNonce)
			fullNonces.evm.user++
			require.Equal(t, fullNonces.evm.user, nonceCounterAccount.TotalNonce)

			ccipMessageSentEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, ccip_router.ParseEvent_CcipMessageSent)
			require.NoError(t, err)
			require.Equal(t, uint64(21), ccipMessageSentEvent.DestChainSelector)
			require.Equal(t, uint64(4), ccipMessageSentEvent.SequenceNumber)
			require.Equal(t, testutils.User.PublicKey(), ccipMessageSentEvent.Message.Sender)
			require.Equal(t, env.ValidEvmAbiReceiverAddress[:], ccipMessageSentEvent.Message.Receiver)
			data := [3]uint8{4, 5, 6}
			require.Equal(t, data[:], ccipMessageSentEvent.Message.Data)
			require.Equal(t, bin.Uint128{Lo: 5000, Hi: 0}, testutils.MustDeserializeExtraArgs(t, &ccip_common.GenericExtraArgsV2{}, ccipMessageSentEvent.Message.ExtraArgs, ccip.GenericExtraArgsV2Tag).GasLimit)
			require.True(t, testutils.MustDeserializeExtraArgs(t, &ccip_common.GenericExtraArgsV2{}, ccipMessageSentEvent.Message.ExtraArgs, ccip.GenericExtraArgsV2Tag).AllowOutOfOrderExecution)
			require.Equal(t, uint64(15), ccipMessageSentEvent.Message.Header.SourceChainSelector)
			require.Equal(t, uint64(21), ccipMessageSentEvent.Message.Header.DestChainSelector)
			require.Equal(t, uint64(4), ccipMessageSentEvent.Message.Header.SequenceNumber)
			require.Equal(t, uint64(0), ccipMessageSentEvent.Message.Header.Nonce) // OOO: nonce is 0
		})

		t.Run("When gasLimit is set to zero, it overrides Emits CCIPMessageSent", func(t *testing.T) {
			destinationChainSelector := config.EvmChainSelector
			destinationChainStatePDA := config.EvmDestChainStatePDA
			message := ccip_router.Svm2AnyMessage{
				FeeToken: env.Link22.Mint,
				Receiver: env.ValidEvmAbiReceiverAddress[:],
				Data:     []byte{4, 5, 6},
				ExtraArgs: testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV2{
					GasLimit: bin.Uint128{Lo: 0, Hi: 0},
				}, ccip.GenericExtraArgsV2Tag),
			}

			instruction, err := ccip_router.NewCcipSendInstruction(
				destinationChainSelector,
				message,
				[]byte{},
				config.RouterConfigPDA,
				destinationChainStatePDA,
				env.NonceEvmPDA,
				testutils.User.PublicKey(),
				solana.SystemProgramID,
				env.Link22.Program,
				env.Link22.Mint,
				env.Link22.UserATA,
				env.Link22.BillingATA,
				config.BillingSignerPDA,
				config.FeeQuoterProgram,
				config.FqConfigPDA,
				config.FqEvmDestChainPDA,
				env.Link22.FqBillingCfgPDA,
				env.Link22.FqBillingCfgPDA,
				config.RMNRemoteProgram,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			instruction.(*solana.GenericInstruction).AccountValues[7].IsWritable = true
			result := testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{instruction}, testutils.User, config.DefaultCommitment)
			require.NotNil(t, result)

			var chainStateAccount ccip_router.DestChain
			err = common.GetAccountDataBorshIntoAnchor(ctx, env.Client, destinationChainStatePDA, config.DefaultCommitment, &chainStateAccount)
			require.NoError(t, err, "failed to get account info")
			require.Equal(t, uint64(5), chainStateAccount.State.MessageNumber)

			var nonceCounterAccount ccip_router.Nonce
			err = common.GetAccountDataBorshIntoAnchor(ctx, env.Client, env.NonceEvmPDA, config.DefaultCommitment, &nonceCounterAccount)
			require.NoError(t, err, "failed to get account info")
			require.Equal(t, uint64(3), nonceCounterAccount.OrderedNonce)
			fullNonces.evm.user++
			require.Equal(t, fullNonces.evm.user, nonceCounterAccount.TotalNonce)

			ccipMessageSentEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, ccip_router.ParseEvent_CcipMessageSent)
			require.NoError(t, err)
			require.Equal(t, uint64(21), ccipMessageSentEvent.DestChainSelector)
			require.Equal(t, uint64(5), ccipMessageSentEvent.SequenceNumber)
			require.Equal(t, testutils.User.PublicKey(), ccipMessageSentEvent.Message.Sender)
			require.Equal(t, env.ValidEvmAbiReceiverAddress[:], ccipMessageSentEvent.Message.Receiver)
			data := [3]uint8{4, 5, 6}
			require.Equal(t, data[:], ccipMessageSentEvent.Message.Data)
			require.Equal(t, bin.Uint128{Lo: 0, Hi: 0}, testutils.MustDeserializeExtraArgs(t, &ccip_common.GenericExtraArgsV2{}, ccipMessageSentEvent.Message.ExtraArgs, ccip.GenericExtraArgsV2Tag).GasLimit)
			require.False(t, testutils.MustDeserializeExtraArgs(t, &ccip_common.GenericExtraArgsV2{}, ccipMessageSentEvent.Message.ExtraArgs, ccip.GenericExtraArgsV2Tag).AllowOutOfOrderExecution)
			require.Equal(t, uint64(15), ccipMessageSentEvent.Message.Header.SourceChainSelector)
			require.Equal(t, uint64(21), ccipMessageSentEvent.Message.Header.DestChainSelector)
			require.Equal(t, uint64(5), ccipMessageSentEvent.Message.Header.SequenceNumber)
			require.Equal(t, uint64(3), ccipMessageSentEvent.Message.Header.Nonce)
		})

		t.Run("When sending a message with an invalid nonce account, it fails", func(t *testing.T) {
			destinationChainSelector := config.EvmChainSelector
			destinationChainStatePDA := config.EvmDestChainStatePDA
			message := ccip_router.Svm2AnyMessage{
				FeeToken:  env.WSOL.Mint,
				Receiver:  env.ValidEvmAbiReceiverAddress[:],
				Data:      []byte{4, 5, 6},
				ExtraArgs: []byte{},
			}

			instruction, err := ccip_router.NewCcipSendInstruction(
				destinationChainSelector,
				message,
				[]byte{},
				config.RouterConfigPDA,
				destinationChainStatePDA,
				env.NonceEvmPDA,
				testutils.AnotherUser.PublicKey(),
				solana.SystemProgramID,
				env.WSOL.Program,
				env.WSOL.Mint,
				env.WSOL.UserATA,
				env.WSOL.BillingATA,
				config.BillingSignerPDA,
				config.FeeQuoterProgram,
				config.FqConfigPDA,
				config.FqEvmDestChainPDA,
				env.WSOL.FqBillingCfgPDA,
				env.Link22.FqBillingCfgPDA,
				config.RMNRemoteProgram,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)

			result := testutils.SendAndFailWith(ctx, t, env.Client, []solana.Instruction{instruction}, testutils.AnotherUser, config.DefaultCommitment, []string{"Error Message: A seeds constraint was violated"})
			require.NotNil(t, result)
		})

		t.Run("When sending a message impersonating another user, it fails", func(t *testing.T) {
			destinationChainSelector := config.EvmChainSelector
			destinationChainStatePDA := config.EvmDestChainStatePDA
			message := ccip_router.Svm2AnyMessage{
				FeeToken:  env.WSOL.Mint,
				Receiver:  env.ValidEvmAbiReceiverAddress[:],
				Data:      []byte{4, 5, 6},
				ExtraArgs: []byte{},
			}

			instruction, err := ccip_router.NewCcipSendInstruction(
				destinationChainSelector,
				message,
				[]byte{},
				config.RouterConfigPDA,
				destinationChainStatePDA,
				env.NonceEvmPDA,
				testutils.User.PublicKey(),
				solana.SystemProgramID,
				env.WSOL.Program,
				env.WSOL.Mint,
				env.WSOL.UserATA,
				env.WSOL.BillingATA,
				config.BillingSignerPDA,
				config.FeeQuoterProgram,
				config.FqConfigPDA,
				config.FqEvmDestChainPDA,
				env.WSOL.FqBillingCfgPDA,
				env.Link22.FqBillingCfgPDA,
				config.RMNRemoteProgram,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)

			testutils.SendAndFailWithRPCError(ctx, t, env.Client, []solana.Instruction{instruction}, testutils.AnotherUser, config.DefaultCommitment, []string{"Transaction signature verification failure"})
		})

		t.Run("When sending a CCIP Message without flagging the user ATA as writable, it fails", func(t *testing.T) {
			destinationChainSelector := config.EvmChainSelector
			destinationChainStatePDA := config.EvmDestChainStatePDA
			message := ccip_router.Svm2AnyMessage{
				FeeToken:  env.WSOL.Mint,
				Receiver:  env.ValidEvmAbiReceiverAddress[:],
				Data:      []byte{4, 5, 6},
				ExtraArgs: []byte{},
			}

			instruction, err := ccip_router.NewCcipSendInstruction(
				destinationChainSelector,
				message,
				[]byte{},
				config.RouterConfigPDA,
				destinationChainStatePDA,
				env.NonceEvmPDA,
				testutils.User.PublicKey(),
				solana.SystemProgramID,
				env.WSOL.Program,
				env.WSOL.Mint,
				env.WSOL.UserATA,
				env.WSOL.BillingATA,
				config.BillingSignerPDA,
				config.FeeQuoterProgram,
				config.FqConfigPDA,
				config.FqEvmDestChainPDA,
				env.WSOL.FqBillingCfgPDA,
				env.Link22.FqBillingCfgPDA,
				config.RMNRemoteProgram,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			// deliberately NOT setting AccountValues[7].IsWritable = true
			testutils.SendAndFailWithRPCError(ctx, t, env.Client, []solana.Instruction{instruction}, testutils.User, config.DefaultCommitment, []string{ccip.InvalidInputsAtaWritable_CcipRouterError.String()})
		})

		t.Run("When sending a CCIP Message paying with inconsistent fee token accounts, it fails", func(t *testing.T) {
			destinationChainSelector := config.EvmChainSelector
			destinationChainStatePDA := config.EvmDestChainStatePDA

			billingTokens := []testutils.BillingTokenState{env.WSOL, env.Link22}

			for _, feeProgram := range billingTokens {
				for _, feeMint := range billingTokens {
					for _, messageFeeToken := range billingTokens {
						for _, feeUserATA := range billingTokens {
							for _, feeBillingATA := range billingTokens {
								for _, feeBillingConfigPDA := range billingTokens {
									for _, linkBillingConfigPDA := range billingTokens {
										if feeProgram.Name == feeMint.Name && feeMint.Name == messageFeeToken.Name && messageFeeToken.Name == feeUserATA.Name && feeUserATA.Name == feeBillingATA.Name && feeBillingATA.Name == feeBillingConfigPDA.Name && linkBillingConfigPDA.Name == env.Link22.Name {
											continue // skip valid combinations: all fee accounts consistent AND link billing config is correct
										}
										t.Run(fmt.Sprintf("%s/%s/%s/%s/%s/%s/%s", feeProgram.Name, feeMint.Name, messageFeeToken.Name, feeUserATA.Name, feeBillingATA.Name, feeBillingConfigPDA.Name, linkBillingConfigPDA.Name), func(t *testing.T) {
											t.Parallel()
											message := ccip_router.Svm2AnyMessage{
												FeeToken:  messageFeeToken.Mint,
												Receiver:  env.ValidEvmAbiReceiverAddress[:],
												Data:      []byte{4, 5, 6},
												ExtraArgs: []byte{},
											}
											instruction, err := ccip_router.NewCcipSendInstruction(
												destinationChainSelector,
												message,
												[]byte{},
												config.RouterConfigPDA,
												destinationChainStatePDA,
												env.NonceEvmPDA,
												testutils.User.PublicKey(),
												solana.SystemProgramID,
												feeProgram.Program,
												feeMint.Mint,
												feeUserATA.UserATA,
												feeBillingATA.BillingATA,
												config.BillingSignerPDA,
												config.FeeQuoterProgram,
												config.FqConfigPDA,
												config.FqEvmDestChainPDA,
												feeBillingConfigPDA.FqBillingCfgPDA,
												linkBillingConfigPDA.FqBillingCfgPDA,
												config.RMNRemoteProgram,
												config.RMNRemoteCursesPDA,
												config.RMNRemoteConfigPDA,
											)
											require.NoError(t, err)
											instruction.(*solana.GenericInstruction).AccountValues[7].IsWritable = true
											testutils.SendAndFailWith(ctx, t, env.Client, []solana.Instruction{instruction}, testutils.User, config.DefaultCommitment, []string{""}) // just assert it fails
										})
									}
								}
							}
						}
					}
				}
			}
		})

		t.Run("When sending a CCIP Message and tries to pay with some else's tokens it fails", func(t *testing.T) {
			destinationChainSelector := config.EvmChainSelector
			destinationChainStatePDA := config.EvmDestChainStatePDA
			message := ccip_router.Svm2AnyMessage{
				FeeToken:  env.Link22.Mint,
				Receiver:  env.ValidEvmAbiReceiverAddress[:],
				Data:      []byte{4, 5, 6},
				ExtraArgs: []byte{},
			}

			anotherUserNonce, err := state.FindNoncePDA(config.EvmChainSelector, testutils.AnotherUser.PublicKey(), config.CcipRouterProgram)
			require.NoError(t, err)

			instruction, err := ccip_router.NewCcipSendInstruction(
				destinationChainSelector,
				message,
				[]byte{},
				config.RouterConfigPDA,
				destinationChainStatePDA,
				anotherUserNonce,
				testutils.AnotherUser.PublicKey(),
				solana.SystemProgramID,
				env.Link22.Program,
				env.Link22.Mint,
				env.Link22.UserATA, // wrong: this is User's ATA, not AnotherUser's
				env.Link22.BillingATA,
				config.BillingSignerPDA,
				config.FeeQuoterProgram,
				config.FqConfigPDA,
				config.FqEvmDestChainPDA,
				env.Link22.FqBillingCfgPDA,
				env.Link22.FqBillingCfgPDA,
				config.RMNRemoteProgram,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			instruction.(*solana.GenericInstruction).AccountValues[7].IsWritable = true
			testutils.SendAndFailWith(ctx, t, env.Client, []solana.Instruction{instruction}, testutils.AnotherUser, config.DefaultCommitment, []string{ccip.InvalidInputsAtaAddress_CcipRouterError.String()})
		})

		t.Run("When another user sending a Valid CCIP Message Emits CCIPMessageSent", func(t *testing.T) {
			destinationChainSelector := config.EvmChainSelector
			destinationChainStatePDA := config.EvmDestChainStatePDA
			anotherUserNonce, err := state.FindNoncePDA(config.EvmChainSelector, testutils.AnotherUser.PublicKey(), config.CcipRouterProgram)
			require.NoError(t, err)

			message := ccip_router.Svm2AnyMessage{
				FeeToken:  env.Link22.Mint,
				Receiver:  env.ValidEvmAbiReceiverAddress[:],
				Data:      []byte{4, 5, 6},
				ExtraArgs: []byte{},
			}

			instruction, err := ccip_router.NewCcipSendInstruction(
				destinationChainSelector,
				message,
				[]byte{},
				config.RouterConfigPDA,
				destinationChainStatePDA,
				anotherUserNonce,
				testutils.AnotherUser.PublicKey(),
				solana.SystemProgramID,
				env.Link22.Program,
				env.Link22.Mint,
				env.Link22.AnotherUserATA,
				env.Link22.BillingATA,
				config.BillingSignerPDA,
				config.FeeQuoterProgram,
				config.FqConfigPDA,
				config.FqEvmDestChainPDA,
				env.Link22.FqBillingCfgPDA,
				env.Link22.FqBillingCfgPDA,
				config.RMNRemoteProgram,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			instruction.(*solana.GenericInstruction).AccountValues[7].IsWritable = true

			result := testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{instruction}, testutils.AnotherUser, config.DefaultCommitment)
			require.NotNil(t, result)

			var chainStateAccount ccip_router.DestChain
			err = common.GetAccountDataBorshIntoAnchor(ctx, env.Client, destinationChainStatePDA, config.DefaultCommitment, &chainStateAccount)
			require.NoError(t, err, "failed to get account info")
			require.Equal(t, uint64(6), chainStateAccount.State.MessageNumber)

			var nonceCounterAccount ccip_router.Nonce
			err = common.GetAccountDataBorshIntoAnchor(ctx, env.Client, anotherUserNonce, config.DefaultCommitment, &nonceCounterAccount)
			require.NoError(t, err, "failed to get account info")
			require.Equal(t, uint64(1), nonceCounterAccount.OrderedNonce)
			fullNonces.evm.anotherUser++
			require.Equal(t, fullNonces.evm.anotherUser, nonceCounterAccount.TotalNonce)

			ccipMessageSentEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, ccip_router.ParseEvent_CcipMessageSent)
			require.NoError(t, err)
			require.Equal(t, uint64(21), ccipMessageSentEvent.DestChainSelector)
			require.Equal(t, uint64(6), ccipMessageSentEvent.SequenceNumber)
			require.Equal(t, testutils.AnotherUser.PublicKey(), ccipMessageSentEvent.Message.Sender)
			require.Equal(t, env.ValidEvmAbiReceiverAddress[:], ccipMessageSentEvent.Message.Receiver)
			data := [3]uint8{4, 5, 6}
			require.Equal(t, data[:], ccipMessageSentEvent.Message.Data)
			extraArgs := testutils.MustDeserializeExtraArgs(t, &ccip_common.GenericExtraArgsV2{}, ccipMessageSentEvent.Message.ExtraArgs, ccip.GenericExtraArgsV2Tag)
			require.Equal(t, bin.Uint128{Lo: uint64(env.ValidFqDestChainConfig.DefaultTxGasLimit.V0), Hi: 0}, extraArgs.GasLimit)
			require.False(t, extraArgs.AllowOutOfOrderExecution)
			require.Equal(t, uint64(15), ccipMessageSentEvent.Message.Header.SourceChainSelector)
			require.Equal(t, uint64(21), ccipMessageSentEvent.Message.Header.DestChainSelector)
			require.Equal(t, uint64(6), ccipMessageSentEvent.Message.Header.SequenceNumber)
			require.Equal(t, uint64(1), ccipMessageSentEvent.Message.Header.Nonce)
		})

		t.Run("token happy path", func(t *testing.T) {
			t.Run("single token", func(t *testing.T) {
				_, initSupply, err := tokens.TokenSupply(ctx, env.Client, env.Token0.Mint, config.DefaultCommitment)
				require.NoError(t, err)
				_, initBal, err := tokens.TokenBalance(ctx, env.Client, env.Token0.User[testutils.User.PublicKey()], config.DefaultCommitment)
				require.NoError(t, err)

				destinationChainSelector := config.EvmChainSelector
				message := ccip_router.Svm2AnyMessage{
					FeeToken: env.WSOL.Mint,
					Receiver: env.ValidEvmAbiReceiverAddress[:],
					Data:     []byte{4, 5, 6},
					TokenAmounts: []ccip_router.SvmTokenAmount{
						{
							Token:  env.Token0.Mint,
							Amount: 1,
						},
					},
					ExtraArgs: []byte{},
				}

				userTokenAccount, ok := env.Token0.User[testutils.User.PublicKey()]
				require.True(t, ok)

				derivedAccounts, derivedLookUpTables, tokenIndices := testutils.DeriveSendAccounts(ctx,
					t,
					testutils.User,
					message,
					destinationChainSelector,
					env.Client,
					config.CcipRouterProgram)

				ix, err := ccip_router.NewCcipSendInstruction(
					destinationChainSelector, message, tokenIndices,
					solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{}, solana.PublicKey{},
				)
				require.NoError(t, err)
				ix.(*solana.GenericInstruction).AccountValues = derivedAccounts

				lookupTables := env.CcipSendLookupTable
				for _, table := range derivedLookUpTables {
					entries, lutErr := common.GetAddressLookupTable(ctx, env.Client, table)
					require.NoError(t, lutErr)
					lookupTables[table] = entries
				}

				ixApprove, err := tokens.TokenApproveChecked(1, token0Decimals, env.Token0.Program, userTokenAccount, env.Token0.Mint, config.BillingSignerPDA, testutils.User.PublicKey(), nil)
				require.NoError(t, err)

				result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{ixApprove, ix}, testutils.User, config.DefaultCommitment, lookupTables, common.AddComputeUnitLimit(300_000))
				require.NotNil(t, result)

				fullNonces.evm.user++

				// check CCIP event
				ccipMessageSentEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, ccip_router.ParseEvent_CcipMessageSent)
				require.NoError(t, err)
				require.Len(t, ccipMessageSentEvent.Message.TokenAmounts, 1)
				ta := ccipMessageSentEvent.Message.TokenAmounts[0]

				require.Equal(t, env.WSOL.Mint, ccipMessageSentEvent.Message.FeeToken)
				require.Equal(t, tokens.ToLittleEndianU256(363431260), ccipMessageSentEvent.Message.FeeTokenAmount.LeBytes)
				require.Equal(t, tokens.ToLittleEndianU256(36343126000000000), ccipMessageSentEvent.Message.FeeValueJuels.LeBytes)
				require.Equal(t, env.Token0.PoolConfig, ta.SourcePoolAddress)
				require.Equal(t, config.EVMToken0AddressBytes, ta.DestTokenAddress)
				expectedExtraData := make([]byte, 32)
				expectedExtraData[31] = token0Decimals
				require.Len(t, ta.ExtraData, 32)
				require.Equal(t, expectedExtraData, ta.ExtraData)

				require.Equal(t, tokens.ToLittleEndianU256(1), ta.Amount.LeBytes)
				require.Len(t, ta.DestExecData, 4)
				expectedDestExecData := make([]byte, 4)
				binary.BigEndian.PutUint32(expectedDestExecData, 100)
				require.Equal(t, expectedDestExecData, ta.DestExecData)

				// check pool event
				poolEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, base_token_pool.ParseEvent_Burned)
				require.NoError(t, err)
				require.Equal(t, env.Token0.RouterSigner, poolEvent.Sender)
				require.Equal(t, uint64(1), poolEvent.Amount)
				require.Equal(t, env.Token0.Mint, poolEvent.Mint)

				// check balances
				_, currSupply, err := tokens.TokenSupply(ctx, env.Client, env.Token0.Mint, config.DefaultCommitment)
				require.NoError(t, err)
				require.Equal(t, 1, initSupply-currSupply)
				_, currBal, err := tokens.TokenBalance(ctx, env.Client, env.Token0.User[testutils.User.PublicKey()], config.DefaultCommitment)
				require.NoError(t, err)
				require.Equal(t, 1, initBal-currBal)
				_, poolBal, err := tokens.TokenBalance(ctx, env.Client, env.Token0.PoolTokenAccount, config.DefaultCommitment)
				require.NoError(t, err)
				require.Equal(t, 0, poolBal)
			})
		})

		t.Run("When sending with an enabled allowlist including the sender, it succeeds", func(t *testing.T) {
			var initialDestChain ccip_router.DestChain
			err := common.GetAccountDataBorshIntoAnchor(ctx, env.Client, config.EvmDestChainStatePDA, config.DefaultCommitment, &initialDestChain)
			require.NoError(t, err, "failed to get account info")
			modifiedDestChain := initialDestChain
			modifiedDestChain.Config.AllowListEnabled = true
			modifiedDestChain.Config.AllowedSenders = []solana.PublicKey{
				testutils.User.PublicKey(),
				testutils.AnotherUser.PublicKey(),
			}

			updateDestChainIx, err := ccip_router.NewUpdateDestChainConfigInstruction(
				config.EvmChainSelector,
				modifiedDestChain.Config,
				config.EvmDestChainStatePDA,
				config.RouterConfigPDA,
				testutils.CcipAdmin.PublicKey(),
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{updateDestChainIx}, testutils.CcipAdmin, config.DefaultCommitment)

			destinationChainSelector := config.EvmChainSelector
			destinationChainStatePDA := config.EvmDestChainStatePDA
			message := ccip_router.Svm2AnyMessage{
				FeeToken:  env.WSOL.Mint,
				Receiver:  env.ValidEvmAbiReceiverAddress[:],
				Data:      []byte{4, 5, 6},
				ExtraArgs: []byte{},
			}

			instruction, err := ccip_router.NewCcipSendInstruction(
				destinationChainSelector,
				message,
				[]byte{},
				config.RouterConfigPDA,
				destinationChainStatePDA,
				env.NonceEvmPDA,
				testutils.User.PublicKey(),
				solana.SystemProgramID,
				env.WSOL.Program,
				env.WSOL.Mint,
				env.WSOL.UserATA,
				env.WSOL.BillingATA,
				config.BillingSignerPDA,
				config.FeeQuoterProgram,
				config.FqConfigPDA,
				config.FqEvmDestChainPDA,
				env.WSOL.FqBillingCfgPDA,
				env.Link22.FqBillingCfgPDA,
				config.RMNRemoteProgram,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			instruction.(*solana.GenericInstruction).AccountValues[7].IsWritable = true

			result := testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{instruction}, testutils.User, config.DefaultCommitment)
			require.NotNil(t, result)
			fullNonces.evm.user++

			// Validate allowed senders round-trip
			var destChain ccip_router.DestChain
			err = common.GetAccountDataBorshIntoAnchor(ctx, env.Client, config.EvmDestChainStatePDA, config.DefaultCommitment, &destChain)
			require.NoError(t, err)
			require.True(t, destChain.Config.AllowListEnabled)
			require.ElementsMatch(t, []solana.PublicKey{testutils.User.PublicKey(), testutils.AnotherUser.PublicKey()}, destChain.Config.AllowedSenders)

			// Restore config
			updateDestChainIx, err = ccip_router.NewUpdateDestChainConfigInstruction(
				config.EvmChainSelector,
				initialDestChain.Config,
				config.EvmDestChainStatePDA,
				config.RouterConfigPDA,
				testutils.CcipAdmin.PublicKey(),
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{updateDestChainIx}, testutils.CcipAdmin, config.DefaultCommitment)
		})

		t.Run("token pool accounts: validation", func(t *testing.T) {
			destinationChainSelector := config.EvmChainSelector
			destinationChainStatePDA := config.EvmDestChainStatePDA
			message := ccip_router.Svm2AnyMessage{
				FeeToken: env.WSOL.Mint,
				Receiver: env.ValidEvmAbiReceiverAddress[:],
				Data:     []byte{4, 5, 6},
				TokenAmounts: []ccip_router.SvmTokenAmount{
					{
						Token:  env.Token0.Mint,
						Amount: 1,
					},
				},
				ExtraArgs: []byte{},
			}

			userTokenAccount, ok := env.Token0.User[testutils.User.PublicKey()]
			require.True(t, ok)

			inputs := []struct {
				name        string
				index       uint
				replaceWith *solana.AccountMeta
				errorStr    string
			}{
				{
					name:     "incorrect user token account",
					index:    0,
					errorStr: ccip.InvalidInputsTokenAccounts_CcipCommonError.String(),
				},
				{
					name:     "invalid billing config",
					index:    1,
					errorStr: common.ConstraintSeeds_AnchorError.String(),
				},
				{
					name:     "invalid token pool chain config",
					index:    2,
					errorStr: common.ConstraintSeeds_AnchorError.String(),
				},
				{
					name:     "pool config is not owned by pool program",
					index:    6,
					errorStr: common.ConstraintSeeds_AnchorError.String(),
				},
				{
					name:        "is pool config but for wrong token",
					index:       6,
					replaceWith: solana.Meta(env.Token1.PoolConfig),
					errorStr:    common.ConstraintSeeds_AnchorError.String(),
				},
				{
					name:        "is pool lookup table but has write permissions",
					index:       3,
					replaceWith: solana.Meta(env.Token0.PoolLookupTable).WRITE(),
					errorStr:    ccip.InvalidInputsLookupTableAccountWritable_CcipRouterError.String(),
				},
				{
					name:     "incorrect pool signer",
					index:    8,
					errorStr: ccip.InvalidInputsTokenAccounts_CcipRouterError.String(),
				},
				{
					name:     "invalid token program",
					index:    9,
					errorStr: "InvalidProgramId.",
				},
				{
					name:     "incorrect pool token account",
					index:    7,
					errorStr: common.AccountOwnedByWrongProgram_AnchorError.String(),
				},
				{
					name:        "incorrect token pool lookup table",
					index:       3,
					replaceWith: solana.Meta(env.Token1.PoolLookupTable),
					errorStr:    ccip.InvalidInputsLookupTableAccounts_CcipRouterError.String(),
				},
				{
					name:     "invalid fee token config",
					index:    11,
					errorStr: common.ConstraintSeeds_AnchorError.String(),
				},
				{
					name:     "remaining accounts mismatch",
					index:    13,
					errorStr: ccip.InvalidInputsLookupTableAccounts_CcipRouterError.String(),
				},
				{
					name:     "invalid token pool signer",
					index:    12,
					errorStr: common.ConstraintSeeds_AnchorError.String(),
				},
			}

			for _, in := range inputs {
				t.Run(in.name, func(t *testing.T) {
					t.Parallel()
					instruction, err := ccip_router.NewCcipSendInstruction(
						destinationChainSelector,
						message,
						[]byte{0},
						config.RouterConfigPDA,
						destinationChainStatePDA,
						env.NonceEvmPDA,
						testutils.User.PublicKey(),
						solana.SystemProgramID,
						env.WSOL.Program,
						env.WSOL.Mint,
						env.WSOL.UserATA,
						env.WSOL.BillingATA,
						config.BillingSignerPDA,
						config.FeeQuoterProgram,
						config.FqConfigPDA,
						config.FqEvmDestChainPDA,
						env.WSOL.FqBillingCfgPDA,
						env.Link22.FqBillingCfgPDA,
						config.RMNRemoteProgram,
						config.RMNRemoteCursesPDA,
						config.RMNRemoteConfigPDA,
					)
					require.NoError(t, err)
					instruction.(*solana.GenericInstruction).AccountValues[7].IsWritable = true

					tokenMetas, addressTables, err := tokens.ParseTokenLookupTable(ctx, env.Client, env.Token0, userTokenAccount)
					require.NoError(t, err)
					if in.replaceWith == nil {
						in.replaceWith = solana.Meta(solana.PublicKey{})
					}
					if in.index >= uint(len(tokenMetas)) {
						tokenMetas = append(tokenMetas, in.replaceWith)
					} else {
						tokenMetas[in.index] = in.replaceWith
					}

					genericInst, ok := instruction.(*solana.GenericInstruction)
					require.True(t, ok, "instruction must be *solana.GenericInstruction")
					genericInst.AccountValues = append(genericInst.AccountValues, tokenMetas...)

					testutils.SendAndFailWithLookupTables(ctx, t, env.Client, []solana.Instruction{instruction}, testutils.User, config.DefaultCommitment, addressTables, []string{in.errorStr})
				})
			}

			t.Run("token pool accounts are valid but inconsistent with message token", func(t *testing.T) {
				instruction, err := ccip_router.NewCcipSendInstruction(
					destinationChainSelector,
					message, // bridging token0
					[]byte{0},
					config.RouterConfigPDA,
					destinationChainStatePDA,
					env.NonceEvmPDA,
					testutils.User.PublicKey(),
					solana.SystemProgramID,
					env.WSOL.Program,
					env.WSOL.Mint,
					env.WSOL.UserATA,
					env.WSOL.BillingATA,
					config.BillingSignerPDA,
					config.FeeQuoterProgram,
					config.FqConfigPDA,
					config.FqEvmDestChainPDA,
					env.WSOL.FqBillingCfgPDA,
					env.Link22.FqBillingCfgPDA,
					config.RMNRemoteProgram,
					config.RMNRemoteCursesPDA,
					config.RMNRemoteConfigPDA,
				)
				require.NoError(t, err)
				instruction.(*solana.GenericInstruction).AccountValues[7].IsWritable = true

				// using accounts from token 1
				tokenMetas, addressTables, err := tokens.ParseTokenLookupTable(ctx, env.Client, env.Token1, env.Token1.User[testutils.User.PublicKey()])
				require.NoError(t, err)
				genericInst, ok := instruction.(*solana.GenericInstruction)
				require.True(t, ok, "instruction must be *solana.GenericInstruction")
				genericInst.AccountValues = append(genericInst.AccountValues, tokenMetas...)

				testutils.SendAndFailWithLookupTables(ctx, t, env.Client, []solana.Instruction{instruction}, testutils.User, config.DefaultCommitment, addressTables, []string{ccip.InvalidInputsTokenAccounts_CcipCommonError.String()})
			})
		})

		toFqMsg := func(msg ccip_router.Svm2AnyMessage) fee_quoter.Svm2AnyMessage {
			fqTokenAmounts := make([]fee_quoter.SvmTokenAmount, len(msg.TokenAmounts))
			for i, ta := range msg.TokenAmounts {
				fqTokenAmounts[i] = fee_quoter.SvmTokenAmount{
					Token:  ta.Token,
					Amount: ta.Amount,
				}
			}
			return fee_quoter.Svm2AnyMessage{
				Receiver:     msg.Receiver,
				Data:         msg.Data,
				TokenAmounts: fqTokenAmounts,
				FeeToken:     msg.FeeToken,
				ExtraArgs:    msg.ExtraArgs,
			}
		}

		t.Run("When sending a Valid CCIP Message it bills the amount that getFee previously returned", func(t *testing.T) {
			destinationChainSelector := config.EvmChainSelector
			destinationChainStatePDA := config.EvmDestChainStatePDA

			billingTokens := []testutils.BillingTokenState{env.WSOL, env.Link22}
			for _, token := range billingTokens {
				t.Run("using "+token.Name, func(t *testing.T) {
					message := ccip_router.Svm2AnyMessage{
						FeeToken:  token.Mint,
						Receiver:  env.ValidEvmAbiReceiverAddress[:],
						Data:      []byte{4, 5, 6},
						ExtraArgs: []byte{},
					}
					fqMsg := toFqMsg(message)
					ix, err := fee_quoter.NewGetFeeInstruction(config.EvmChainSelector, fqMsg, config.FqConfigPDA, config.FqEvmDestChainPDA, token.FqBillingCfgPDA, env.Link22.FqBillingCfgPDA)
					require.NoError(t, err)

					feeResult := testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.User, config.DefaultCommitment)
					require.NotNil(t, feeResult)
					fee, err := common.ExtractAnchorTypedReturnValue[fee_quoter.GetFeeResult](ctx, feeResult.Meta.LogMessages, config.FeeQuoterProgram.String())
					require.NoError(t, err)
					require.Positive(t, fee.Amount)

					initialBalance := env.GetBalance(token.BillingATA)

					instruction, err := ccip_router.NewCcipSendInstruction(
						destinationChainSelector,
						message,
						[]byte{},
						config.RouterConfigPDA,
						destinationChainStatePDA,
						env.NonceEvmPDA,
						testutils.User.PublicKey(),
						solana.SystemProgramID,
						token.Program,
						token.Mint,
						token.UserATA,
						token.BillingATA,
						config.BillingSignerPDA,
						config.FeeQuoterProgram,
						config.FqConfigPDA,
						config.FqEvmDestChainPDA,
						token.FqBillingCfgPDA,
						env.Link22.FqBillingCfgPDA,
						config.RMNRemoteProgram,
						config.RMNRemoteCursesPDA,
						config.RMNRemoteConfigPDA,
					)
					require.NoError(t, err)
					instruction.(*solana.GenericInstruction).AccountValues[7].IsWritable = true
					result := testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{instruction}, testutils.User, config.DefaultCommitment)
					require.NotNil(t, result)

					fullNonces.evm.user++

					finalBalance := env.GetBalance(token.BillingATA)
					require.Equal(t, fee.Amount, finalBalance-initialBalance)
				})
			}
		})

		t.Run("When sending a Valid CCIP Message but the user does not have enough funds of the fee token, it fails", func(t *testing.T) {
			message := ccip_router.Svm2AnyMessage{
				FeeToken:  env.Link22.Mint,
				Receiver:  env.ValidEvmAbiReceiverAddress[:],
				Data:      []byte{4, 5, 6},
				ExtraArgs: []byte{},
			}

			noncePDA, err := state.FindNoncePDA(config.EvmChainSelector, testutils.TokenlessUser.PublicKey(), config.CcipRouterProgram)
			require.NoError(t, err)

			instruction, err := ccip_router.NewCcipSendInstruction(
				config.EvmChainSelector,
				message,
				[]byte{},
				config.RouterConfigPDA,
				config.EvmDestChainStatePDA,
				noncePDA,
				testutils.TokenlessUser.PublicKey(),
				solana.SystemProgramID,
				env.Link22.Program,
				env.Link22.Mint,
				env.Link22.TokenlessUserATA,
				env.Link22.BillingATA,
				config.BillingSignerPDA,
				config.FeeQuoterProgram,
				config.FqConfigPDA,
				config.FqEvmDestChainPDA,
				env.Link22.FqBillingCfgPDA,
				env.Link22.FqBillingCfgPDA,
				config.RMNRemoteProgram,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			instruction.(*solana.GenericInstruction).AccountValues[7].IsWritable = true
			testutils.SendAndFailWith(ctx, t, env.Client, []solana.Instruction{instruction}, testutils.TokenlessUser, config.DefaultCommitment, []string{"insufficient funds"})
		})

		t.Run("When sending a valid CCIP message and paying in native SOL, it bills the same amount that getFee previously returned and it's accumulated as Wrapped SOL", func(t *testing.T) {
			getLamports := func(account solana.PublicKey) uint64 {
				out, err := env.Client.GetBalance(ctx, account, rpc.CommitmentConfirmed)
				require.NoError(t, err)
				return out.Value
			}

			zeroPubkey := solana.PublicKeyFromBytes(make([]byte, 32))

			message := ccip_router.Svm2AnyMessage{
				FeeToken:  zeroPubkey,
				Receiver:  env.ValidEvmAbiReceiverAddress[:],
				Data:      []byte{4, 5, 6},
				ExtraArgs: []byte{},
			}
			fqMsg := toFqMsg(message)

			ix, err := fee_quoter.NewGetFeeInstruction(config.EvmChainSelector, fqMsg, config.FqConfigPDA, config.FqEvmDestChainPDA, env.WSOL.FqBillingCfgPDA, env.Link22.FqBillingCfgPDA)
			require.NoError(t, err)

			feeResult := testutils.SimulateTransaction(ctx, t, env.Client, []solana.Instruction{ix}, testutils.User.PublicKey())
			require.NotNil(t, feeResult)
			fee, err := common.ExtractAnchorTypedReturnValue[fee_quoter.GetFeeResult](ctx, feeResult.Value.Logs, config.FeeQuoterProgram.String())
			require.NoError(t, err)
			require.Positive(t, fee.Amount)

			initialBalance := env.GetBalance(env.WSOL.BillingATA)
			initialLamports := getLamports(testutils.User.PublicKey())

			instruction, err := ccip_router.NewCcipSendInstruction(
				config.EvmChainSelector,
				message,
				[]byte{},
				config.RouterConfigPDA,
				config.EvmDestChainStatePDA,
				env.NonceEvmPDA,
				testutils.User.PublicKey(),
				solana.SystemProgramID,
				env.WSOL.Program,
				env.WSOL.Mint,
				zeroPubkey,
				env.WSOL.BillingATA,
				config.BillingSignerPDA,
				config.FeeQuoterProgram,
				config.FqConfigPDA,
				config.FqEvmDestChainPDA,
				env.WSOL.FqBillingCfgPDA,
				env.Link22.FqBillingCfgPDA,
				config.RMNRemoteProgram,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)

			require.NoError(t, err)
			result := testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{instruction}, testutils.User, config.DefaultCommitment)
			require.NotNil(t, result)

			fullNonces.evm.user++

			finalBalance := env.GetBalance(env.WSOL.BillingATA)
			finalLamports := getLamports(testutils.User.PublicKey())

			require.Equal(t, fee.Amount, finalBalance-initialBalance)
			require.Equal(t, fee.Amount+result.Meta.Fee, initialLamports-finalLamports)

			event, err := common.ParseEventFromLogs(result.Meta.LogMessages, ccip_router.ParseEvent_CcipMessageSent)
			require.NoError(t, err)
			require.Equal(t, env.WSOL.Mint, event.Message.FeeToken)
		})

		t.Run("Sending a Valid CCIP Message with derived accounts emits CCIPMessageSent", func(t *testing.T) {
			destinationChainSelector := config.EvmChainSelector
			message := ccip_router.Svm2AnyMessage{
				FeeToken:  env.WSOL.Mint,
				Receiver:  env.ValidEvmAbiReceiverAddress[:],
				Data:      []byte{4, 5, 6},
				ExtraArgs: []byte{},
			}

			derivedAccounts, derivedLookUpTables, tokenIndices := testutils.DeriveSendAccounts(ctx,
				t,
				testutils.User,
				message,
				destinationChainSelector,
				env.Client,
				config.CcipRouterProgram)

			instruction := testutils.BuildCcipSendInstruction(t, destinationChainSelector, message, tokenIndices, derivedAccounts)

			lookupTables := env.CcipSendLookupTable
			for _, table := range derivedLookUpTables {
				entries, lutErr := common.GetAddressLookupTable(ctx, env.Client, table)
				require.NoError(t, lutErr)
				lookupTables[table] = entries
			}

			result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{instruction}, testutils.User, config.DefaultCommitment, lookupTables)
			require.NotNil(t, result)

			ccipMessageSentEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, ccip_router.ParseEvent_CcipMessageSent)
			require.NoError(t, err)
			require.Equal(t, uint64(21), ccipMessageSentEvent.DestChainSelector)
			require.Equal(t, testutils.User.PublicKey(), ccipMessageSentEvent.Message.Sender)
			require.Equal(t, env.ValidEvmAbiReceiverAddress[:], ccipMessageSentEvent.Message.Receiver)
			data := [3]uint8{4, 5, 6}
			require.Equal(t, data[:], ccipMessageSentEvent.Message.Data)
			require.Equal(t, bin.Uint128{Lo: uint64(env.ValidFqDestChainConfig.DefaultTxGasLimit.V0), Hi: 0}, testutils.MustDeserializeExtraArgs(t, &ccip_common.GenericExtraArgsV2{}, ccipMessageSentEvent.Message.ExtraArgs, ccip.GenericExtraArgsV2Tag).GasLimit)
			require.False(t, testutils.MustDeserializeExtraArgs(t, &ccip_common.GenericExtraArgsV2{}, ccipMessageSentEvent.Message.ExtraArgs, ccip.GenericExtraArgsV2Tag).AllowOutOfOrderExecution)
			require.Equal(t, uint64(15), ccipMessageSentEvent.Message.Header.SourceChainSelector)
			require.Equal(t, uint64(21), ccipMessageSentEvent.Message.Header.DestChainSelector)
			hash, err := ccip.HashSVMToAnyMessage(ccipMessageSentEvent.Message)
			require.NoError(t, err)
			require.Equal(t, hash, ccipMessageSentEvent.Message.Header.MessageId[:])
		})

		t.Run("Deriving accounts with a token pool that also requires derivation", func(t *testing.T) {
			message := ccip_router.Svm2AnyMessage{
				Receiver: env.ValidEvmAbiReceiverAddress[:],
				Data:     []byte{},
				TokenAmounts: []ccip_router.SvmTokenAmount{
					{Token: env.UsdcPool.Mint, Amount: 7},
				},
				FeeToken:  solana.PublicKey{},
				ExtraArgs: []byte{},
			}

			// Must be read at the same commitment the derivation simulates against
			// (BuildReadOnlySimulationTx pins CommitmentConfirmed). Reading at Finalized instead
			// races the sends in the sibling subtests: finalization lags confirmation, so the nonce
			// here would be stale — or the account not yet visible at all — while the on-chain
			// derivation below already sees the newer value, yielding a different
			// ccip_cctp_message_sent_event PDA.
			var nonces ccip_router.Nonce
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, env.Client, env.NonceEvmPDA, config.DefaultCommitment, &nonces))

			messageSendEvent, _, _ := solana.FindProgramAddress([][]byte{
				[]byte("ccip_cctp_message_sent_event"),
				testutils.User.PublicKey().Bytes(),
				common.Uint64ToLE(config.EvmChainSelector),
				common.Uint64ToLE(nonces.TotalNonce + 1),
			},
				config.CctpTokenPoolProgram)

			expectedAccounts := []*solana.AccountMeta{
				solana.Meta(config.RouterConfigPDA),
				solana.Meta(config.EvmDestChainStatePDA).WRITE(),
				solana.Meta(env.NonceEvmPDA).WRITE(),
				solana.Meta(testutils.User.PublicKey()).WRITE().SIGNER(),
				solana.Meta(solana.SystemProgramID),
				solana.Meta(env.WSOL.Program),
				solana.Meta(env.WSOL.Mint),
				solana.Meta(common.ZeroAddress),
				solana.Meta(env.WSOL.BillingATA).WRITE(),
				solana.Meta(config.BillingSignerPDA),
				solana.Meta(config.FeeQuoterProgram),
				solana.Meta(config.FqConfigPDA),
				solana.Meta(config.FqEvmDestChainPDA),
				solana.Meta(env.WSOL.FqBillingCfgPDA),
				solana.Meta(env.Link22.FqBillingCfgPDA),
				solana.Meta(config.RMNRemoteProgram),
				solana.Meta(config.RMNRemoteCursesPDA),
				solana.Meta(config.RMNRemoteConfigPDA),
				// token pool (static accounts):
				solana.Meta(env.UsdcPool.User[testutils.User.PublicKey()]).WRITE(),
				solana.Meta(env.UsdcPool.Billing[config.EvmChainSelector]),
				solana.Meta(env.UsdcPool.Chain[config.EvmChainSelector]).WRITE(),
				solana.Meta(env.UsdcPool.PoolLookupTable),
				solana.Meta(env.UsdcPool.AdminRegistryPDA),
				solana.Meta(env.UsdcPool.PoolProgram),
				solana.Meta(env.UsdcPool.PoolConfig),
				solana.Meta(env.UsdcPool.PoolTokenAccount).WRITE(),
				solana.Meta(env.UsdcPool.PoolSigner).WRITE(),
				solana.Meta(env.UsdcPool.Program),
				solana.Meta(env.UsdcPool.Mint).WRITE(),
				solana.Meta(env.UsdcPool.FeeTokenConfig),
				solana.Meta(env.UsdcPool.RouterSigner),
				// token pool (derived additional accounts):
				solana.Meta(env.MessageTransmitter.MessageTransmitter).WRITE(),
				solana.Meta(config.CctpTokenMessengerMinter),
				solana.Meta(solana.SystemProgramID),
				solana.Meta(config.CctpMessageTransmitter),
				solana.Meta(env.TokenMessengerMinter.TokenMessenger),
				solana.Meta(env.TokenMessengerMinter.TokenMinter),
				solana.Meta(env.TokenMessengerMinter.LocalToken).WRITE(),
				solana.Meta(env.TokenMessengerMinter.EventAuthority),
				solana.Meta(env.TokenMessengerMinter.AuthorityPda),
				solana.Meta(env.TokenMessengerMinter.RemoteTokenMessenger),
				solana.Meta(messageSendEvent).WRITE(),
			}

			derivedAccounts, _, tokenIndices := testutils.DeriveSendAccounts(
				ctx,
				t,
				testutils.User,
				message,
				config.EvmChainSelector,
				env.Client,
				config.CcipRouterProgram,
			)

			require.Equal(t, []uint8{0}, tokenIndices)

			// Keep strict checks for deterministic accounts and use invariant checks
			// for the dynamic nested token-derivation tail.
			const deterministicPrefixLen = 39
			require.GreaterOrEqual(t, len(derivedAccounts), deterministicPrefixLen+3, "Derived accounts list too short")

			for i, expected := range expectedAccounts[:deterministicPrefixLen] {
				require.Equal(t, expected, derivedAccounts[i], "Deterministic account at index %d does not match expected", i)
			}

			equalMeta := func(a, b *solana.AccountMeta) bool {
				return a.PublicKey == b.PublicKey && a.IsWritable == b.IsWritable && a.IsSigner == b.IsSigner
			}

			expectedDynamic := []*solana.AccountMeta{
				solana.Meta(env.TokenMessengerMinter.AuthorityPda),
				solana.Meta(env.TokenMessengerMinter.RemoteTokenMessenger),
				solana.Meta(messageSendEvent).WRITE(),
			}

			dynamicTail := derivedAccounts[deterministicPrefixLen:]
			matched := 0
			for _, got := range dynamicTail {
				if matched < len(expectedDynamic) && equalMeta(got, expectedDynamic[matched]) {
					matched++
				}
			}

			if matched != len(expectedDynamic) {
				t.Logf("Expected dynamic accounts (ordered subsequence):")
				for i, exp := range expectedDynamic {
					t.Logf("  expected_dynamic[%d] = %+v", i, exp)
				}
				t.Logf("Actual dynamic tail (%d accounts):", len(dynamicTail))
				for i, got := range dynamicTail {
					t.Logf("  dynamic_tail[%d] = %+v", i, got)
				}
				require.Failf(t, "Dynamic account invariant mismatch", "Did not find expected dynamic nested accounts in order")
			}
		})

		t.Run("When sending a valid CCIP message with a pre-funded nonce PDA, it succeeds", func(t *testing.T) {
			// This test verifies that ccip_send works even when the nonce PDA has been
			// pre-funded with lamports before initialization.
			prefundedUser, err := solana.NewRandomPrivateKey()
			require.NoError(t, err)
			err = testutils.FundAccounts(ctx, []solana.PrivateKey{prefundedUser}, env.Client)
			require.NoError(t, err)

			zeroPubkey := solana.PublicKeyFromBytes(make([]byte, 32))

			// Derive the nonce PDA for this user
			noncePDA, err := state.FindNoncePDA(config.EvmChainSelector, prefundedUser.PublicKey(), config.CcipRouterProgram)
			require.NoError(t, err)

			// Pre-fund the nonce PDA. The transfer must meet rent-exemption
			// for a 0-byte account, otherwise Solana rejects it.
			rentExemptMin := uint64(890_880) // rent-exempt minimum for 0 data bytes
			ixPrefund, err := system.NewTransferInstruction(rentExemptMin, prefundedUser.PublicKey(), noncePDA).ValidateAndBuild()
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixPrefund}, prefundedUser, config.DefaultCommitment)

			// Send a CCIP message paying with native SOL — this should succeed despite the pre-funded nonce PDA
			message := ccip_router.Svm2AnyMessage{
				FeeToken:  zeroPubkey, // will pay with native SOL
				Receiver:  env.ValidEvmAbiReceiverAddress[:],
				Data:      []byte{4, 5, 6},
				ExtraArgs: []byte{},
			}
			instruction, err := ccip_router.NewCcipSendInstruction(
				config.EvmChainSelector,
				message,
				[]byte{},
				config.RouterConfigPDA,
				config.EvmDestChainStatePDA,
				noncePDA,
				prefundedUser.PublicKey(),
				solana.SystemProgramID,
				env.WSOL.Program,
				env.WSOL.Mint,
				zeroPubkey, // no user token account, because paying with native SOL
				env.WSOL.BillingATA,
				config.BillingSignerPDA,
				config.FeeQuoterProgram,
				config.FqConfigPDA,
				config.FqEvmDestChainPDA,
				env.WSOL.FqBillingCfgPDA,
				env.Link22.FqBillingCfgPDA,
				config.RMNRemoteProgram,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)

			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{instruction}, prefundedUser, config.DefaultCommitment)
			require.NotNil(t, result)

			// Verify the nonce was initialized correctly
			var nonceCounterAccount ccip_router.Nonce
			err = common.GetAccountDataBorshIntoAnchor(ctx, env.Client, noncePDA, config.DefaultCommitment, &nonceCounterAccount)
			require.NoError(t, err, "failed to get nonce account info")
			require.Equal(t, uint64(1), nonceCounterAccount.OrderedNonce)
			require.Equal(t, uint64(1), nonceCounterAccount.TotalNonce)
		})
	})

	///////////////////////////
	// CCIP Sender Contract //
	//////////////////////////

	t.Run("Ccip Sender Contract", func(t *testing.T) {
		senderState, _, err := solana.FindProgramAddress([][]byte{[]byte("state")}, config.CcipBaseSender)
		require.NoError(t, err)
		senderPDA, _, err := solana.FindProgramAddress([][]byte{[]byte("ccip_sender")}, config.CcipBaseSender)
		require.NoError(t, err)
		senderEvmDestChainConfigPDA, _, err := solana.FindProgramAddress([][]byte{[]byte("remote_chain_config"), common.Uint64ToLE(config.EvmChainSelector)}, config.CcipBaseSender)
		require.NoError(t, err)
		senderSvmDestChainConfigPDA, _, err := solana.FindProgramAddress([][]byte{[]byte("remote_chain_config"), common.Uint64ToLE(config.SvmChainSelector)}, config.CcipBaseSender)
		require.NoError(t, err)
		wsolATAIx, wsolSenderATA, err := tokens.CreateAssociatedTokenAccount(config.SPLTokenProgram, solana.WrappedSol, senderPDA, testutils.User.PublicKey())
		require.NoError(t, err)
		link22ATAIx, link22SenderATA, err := tokens.CreateAssociatedTokenAccount(config.Token2022Program, env.Link22.Mint, senderPDA, testutils.User.PublicKey())
		require.NoError(t, err)
		senderEvmNoncePDA, err := state.FindNoncePDA(config.EvmChainSelector, senderPDA, config.CcipRouterProgram)
		require.NoError(t, err)
		senderSvmNoncePDA, err := state.FindNoncePDA(config.SvmChainSelector, senderPDA, config.CcipRouterProgram)
		require.NoError(t, err)

		token0ATAIx, token0SenderATA, err := tokens.CreateAssociatedTokenAccount(env.Token0.Program, env.Token0.Mint, senderPDA, testutils.User.PublicKey())
		require.NoError(t, err)
		token1ATAIx, _, err := tokens.CreateAssociatedTokenAccount(env.Token1.Program, env.Token1.Mint, senderPDA, testutils.User.PublicKey())
		require.NoError(t, err)

		// requireSentBySender checks the message the router's 1.6 ccip_send recorded for a send through
		// the sender program, which is all a legacy send can be checked for without 1.6 offchain.
		requireSentBySender := func(t *testing.T, result *rpc.GetTransactionResult, destChainSelector uint64, receiver []byte, tokenCount int) {
			t.Helper()
			require.NotNil(t, result)
			event, err := common.ParseEventFromLogs(result.Meta.LogMessages, ccip_router.ParseEvent_CcipMessageSent)
			require.NoError(t, err)
			require.Equal(t, destChainSelector, event.DestChainSelector)
			require.Equal(t, destChainSelector, event.Message.Header.DestChainSelector)
			require.NotZero(t, event.SequenceNumber)
			require.NotEqual(t, [32]uint8{}, event.Message.Header.MessageId)
			require.Equal(t, senderPDA, event.Message.Sender, "the CCIP sender is the sender program's PDA, not the payer")
			require.Equal(t, receiver, event.Message.Receiver)
			require.Equal(t, []byte{1, 2, 3}, event.Message.Data)
			require.Len(t, event.Message.TokenAmounts, tokenCount)

			senderEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, example_ccip_sender.ParseEvent_MessageSent)
			require.NoError(t, err)
			require.Equal(t, event.Message.Header.MessageId, senderEvent.MessageId)
		}

		lamportBalance := func(account solana.PublicKey) uint64 {
			balance, bErr := env.Client.GetBalance(ctx, account, config.DefaultCommitment)
			require.NoError(t, bErr)
			return balance.Value
		}

		t.Run("setup", func(t *testing.T) {
			programData := testutils.GetProgramDataAddress(ctx, t, env.Client, config.CcipBaseSender)

			// Only the program's upgrade authority may initialize it.
			userInitIx, err := example_ccip_sender.NewInitializeInstruction(config.CcipRouterProgram, senderState, testutils.User.PublicKey(), solana.SystemProgramID, config.CcipBaseSender, programData)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, env.Client, []solana.Instruction{userInitIx}, testutils.User, config.DefaultCommitment, []string{"Error Code: Unauthorized"})

			// The upgrade authority initializes it and hands ownership to the user, who drives it below.
			initIx, err := example_ccip_sender.NewInitializeInstruction(config.CcipRouterProgram, senderState, testutils.Admin.PublicKey(), solana.SystemProgramID, config.CcipBaseSender, programData)
			require.NoError(t, err)
			transferOwnershipIx, err := example_ccip_sender.NewTransferOwnershipInstruction(testutils.User.PublicKey(), senderState, testutils.Admin.PublicKey())
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{initIx, transferOwnershipIx}, testutils.Admin, config.DefaultCommitment)
			acceptOwnershipIx, err := example_ccip_sender.NewAcceptOwnershipInstruction(senderState, testutils.User.PublicKey())
			require.NoError(t, err)

			evmDestChainIx, err := example_ccip_sender.NewInitChainConfigInstruction(config.EvmChainSelector, env.ValidEvmAbiReceiverAddress[:], []byte{}, senderState, senderEvmDestChainConfigPDA, testutils.User.PublicKey(), solana.SystemProgramID)
			require.NoError(t, err)
			svmDestChainIx, err := example_ccip_sender.NewInitChainConfigInstruction(config.SvmChainSelector, env.ValidSvmReceiverAddress[:], testutils.MustSerializeExtraArgs(t, ccip_common.SvmExtraArgsV1{
				AllowOutOfOrderExecution: true,
			}, ccip.SVMExtraArgsV1Tag), senderState, senderSvmDestChainConfigPDA, testutils.User.PublicKey(), solana.SystemProgramID)
			require.NoError(t, err)

			transferSolIx, err := system.NewTransferInstruction(1_000_000_000, testutils.User.PublicKey(), senderPDA).ValidateAndBuild()
			require.NoError(t, err)

			approveLinkIx, err := tokens.TokenApproveChecked(1e9, 9, env.Link22.Program, env.Link22.UserATA, env.Link22.Mint, senderPDA, testutils.User.PublicKey(), nil)
			require.NoError(t, err)

			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{transferSolIx, wsolATAIx, approveLinkIx, link22ATAIx, acceptOwnershipIx, evmDestChainIx, svmDestChainIx, token0ATAIx, token1ATAIx}, testutils.User, rpc.CommitmentConfirmed)
		})

		feeConfig := []struct {
			name                                                                                          string
			feeToken, userATA, feeProgram, feeMint, feeSenderATA, feeBillingATA, feeTokenBillingConfigPDA solana.PublicKey
		}{
			{
				name:                     "sol",
				feeToken:                 solana.PublicKey{},
				userATA:                  env.WSOL.UserATA,
				feeProgram:               env.WSOL.Program,
				feeMint:                  env.WSOL.Mint,
				feeSenderATA:             wsolSenderATA,
				feeBillingATA:            env.WSOL.BillingATA,
				feeTokenBillingConfigPDA: env.WSOL.FqBillingCfgPDA,
			},
			{
				name:                     "link",
				feeToken:                 env.Link22.Mint,
				userATA:                  env.Link22.UserATA,
				feeProgram:               env.Link22.Program,
				feeMint:                  env.Link22.Mint,
				feeSenderATA:             link22SenderATA,
				feeBillingATA:            env.Link22.BillingATA,
				feeTokenBillingConfigPDA: env.Link22.FqBillingCfgPDA,
			},
		}

		chainConfig := []struct {
			destName                                               string
			chainSelector                                          uint64
			receiver                                               []byte
			senderChainConfig, senderNonce, destChainPDA, fqConfig solana.PublicKey
		}{
			{
				destName:          "EVM",
				chainSelector:     config.EvmChainSelector,
				receiver:          env.ValidEvmAbiReceiverAddress[:],
				senderChainConfig: senderEvmDestChainConfigPDA,
				senderNonce:       senderEvmNoncePDA,
				destChainPDA:      config.EvmDestChainStatePDA,
				fqConfig:          config.FqEvmDestChainPDA,
			},
			{
				destName:          "SVM",
				chainSelector:     config.SvmChainSelector,
				receiver:          env.ValidSvmReceiverAddress[:],
				senderChainConfig: senderSvmDestChainConfigPDA,
				senderNonce:       senderSvmNoncePDA,
				destChainPDA:      config.SvmDestChainStatePDA,
				fqConfig:          config.FqSvmDestChainPDA,
			},
		}

		for _, cc := range chainConfig {
			t.Run("SVM->"+cc.destName, func(t *testing.T) {
				for _, fc := range feeConfig {
					t.Run(fmt.Sprintf("billing-%s/message_only", fc.name), func(t *testing.T) {
						ix, err := example_ccip_sender.NewCcipSendInstruction(
							cc.chainSelector,
							[]example_ccip_sender.SvmTokenAmount{},
							[]byte{1, 2, 3},
							fc.feeToken,
							[]uint8{},
							senderState,
							cc.senderChainConfig,
							senderPDA,
							fc.userATA,
							testutils.User.PublicKey(),
							solana.SystemProgramID,
							config.CcipRouterProgram,
							config.RouterConfigPDA,
							cc.destChainPDA,
							cc.senderNonce,
							fc.feeProgram,
							fc.feeMint,
							fc.feeSenderATA,
							fc.feeBillingATA,
							config.BillingSignerPDA,
							config.FeeQuoterProgram,
							config.FqConfigPDA,
							cc.fqConfig,
							fc.feeTokenBillingConfigPDA,
							env.Link22.FqBillingCfgPDA,
							config.RMNRemoteProgram,
							config.RMNRemoteCursesPDA,
							config.RMNRemoteConfigPDA,
						)
						require.NoError(t, err)
						senderBalanceBefore := lamportBalance(senderPDA)
						callerBalanceBefore := lamportBalance(testutils.User.PublicKey())

						result := testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.User, config.DefaultCommitment, common.AddComputeUnitLimit(computebudget.MAX_COMPUTE_UNIT_LIMIT))
						requireSentBySender(t, result, cc.chainSelector, cc.receiver, 0)

						// The sender PDA only fronts the native fee and its nonce account's rent;
						// the caller pays both back within the send.
						require.Equal(t, senderBalanceBefore, lamportBalance(senderPDA))
						if fc.feeToken.IsZero() {
							require.Greater(t, callerBalanceBefore-lamportBalance(testutils.User.PublicKey()), result.Meta.Fee)
						}
					})
				}

				// update config before running with tokens
				if cc.destName == "SVM" {
					svmDestChainWithTokensIx, err := example_ccip_sender.NewUpdateChainConfigInstruction(config.SvmChainSelector, env.ValidSvmReceiverAddress[:], testutils.MustSerializeExtraArgs(t, ccip_common.SvmExtraArgsV1{
						AllowOutOfOrderExecution: true,
						TokenReceiver:            env.ValidSvmReceiverAddress,
					}, ccip.SVMExtraArgsV1Tag), senderState, senderSvmDestChainConfigPDA, testutils.User.PublicKey(), solana.SystemProgramID)
					require.NoError(t, err)
					testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{svmDestChainWithTokensIx}, testutils.User, config.DefaultCommitment)
				}

				for _, fc := range feeConfig {
					t.Run(fmt.Sprintf("billing-%s/with_tokens", fc.name), func(t *testing.T) {
						t.Run("Happy path", func(t *testing.T) {
							instruction, err := example_ccip_sender.NewCcipSendInstruction(
								cc.chainSelector,
								[]example_ccip_sender.SvmTokenAmount{
									{Token: env.Token0.Mint, Amount: 1},
								},
								[]byte{1, 2, 3},
								fc.feeToken,
								[]uint8{1},
								senderState,
								cc.senderChainConfig,
								senderPDA,
								fc.userATA,
								testutils.User.PublicKey(),
								solana.SystemProgramID,
								config.CcipRouterProgram,
								config.RouterConfigPDA,
								cc.destChainPDA,
								cc.senderNonce,
								fc.feeProgram,
								fc.feeMint,
								fc.feeSenderATA,
								fc.feeBillingATA,
								config.BillingSignerPDA,
								config.FeeQuoterProgram,
								config.FqConfigPDA,
								cc.fqConfig,
								fc.feeTokenBillingConfigPDA,
								env.Link22.FqBillingCfgPDA,
								config.RMNRemoteProgram,
								config.RMNRemoteCursesPDA,
								config.RMNRemoteConfigPDA,
							)
							require.NoError(t, err)
							genericInstruction, ok := instruction.(*solana.GenericInstruction)
							require.True(t, ok, "instruction must be *solana.GenericInstruction")
							genericInstruction.AccountValues = append(
								genericInstruction.AccountValues,
								solana.Meta(env.Token0.User[testutils.User.PublicKey()]).WRITE(),
							)

							tokenMetas0, addressTables, err := tokens.ParseTokenLookupTableWithChain(ctx, env.Client, env.Token0, token0SenderATA, cc.chainSelector)
							require.NoError(t, err)
							genericInstruction.AccountValues = append(genericInstruction.AccountValues, tokenMetas0...)
							maps.Copy(addressTables, env.CcipSendLookupTable)

							ixApprove0, err := tokens.TokenApproveChecked(1, token0Decimals, env.Token0.Program, env.Token0.User[testutils.User.PublicKey()], env.Token0.Mint, senderPDA, testutils.User.PublicKey(), nil)
							require.NoError(t, err)

							result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{ixApprove0, instruction}, testutils.User, config.DefaultCommitment, addressTables, common.AddComputeUnitLimit(computebudget.MAX_COMPUTE_UNIT_LIMIT))
							requireSentBySender(t, result, cc.chainSelector, cc.receiver, 1)
						})

						t.Run("When paying with "+fc.name+" and transferring link, it works", func(t *testing.T) {
							instruction, err := example_ccip_sender.NewCcipSendInstruction(
								cc.chainSelector,
								[]example_ccip_sender.SvmTokenAmount{
									{Token: env.Token0.Mint, Amount: 1},
								},
								[]byte{1, 2, 3},
								fc.feeToken,
								[]uint8{1},
								senderState,
								cc.senderChainConfig,
								senderPDA,
								fc.userATA,
								testutils.User.PublicKey(),
								solana.SystemProgramID,
								config.CcipRouterProgram,
								config.RouterConfigPDA,
								cc.destChainPDA,
								cc.senderNonce,
								fc.feeProgram,
								fc.feeMint,
								fc.feeSenderATA,
								fc.feeBillingATA,
								config.BillingSignerPDA,
								config.FeeQuoterProgram,
								config.FqConfigPDA,
								cc.fqConfig,
								fc.feeTokenBillingConfigPDA,
								env.Link22.FqBillingCfgPDA,
								config.RMNRemoteProgram,
								config.RMNRemoteCursesPDA,
								config.RMNRemoteConfigPDA,
							)
							require.NoError(t, err)
							genericInstruction, ok := instruction.(*solana.GenericInstruction)
							require.True(t, ok, "instruction must be *solana.GenericInstruction")
							genericInstruction.AccountValues = append(
								genericInstruction.AccountValues,
								solana.Meta(env.Token0.User[testutils.User.PublicKey()]).WRITE(),
							)

							tokenMetas0, addressTables, err := tokens.ParseTokenLookupTableWithChain(ctx, env.Client, env.Token0, token0SenderATA, cc.chainSelector)
							require.NoError(t, err)
							genericInstruction.AccountValues = append(genericInstruction.AccountValues, tokenMetas0...)
							maps.Copy(addressTables, env.CcipSendLookupTable)

							ixApprove0, err := tokens.TokenApproveChecked(1, token0Decimals, env.Token0.Program, env.Token0.User[testutils.User.PublicKey()], env.Token0.Mint, senderPDA, testutils.User.PublicKey(), nil)
							require.NoError(t, err)

							result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{ixApprove0, instruction}, testutils.User, config.DefaultCommitment, addressTables, common.AddComputeUnitLimit(computebudget.MAX_COMPUTE_UNIT_LIMIT))
							requireSentBySender(t, result, cc.chainSelector, cc.receiver, 1)
						})
					})
				}
			})
		}

		t.Run("When transferring the same token twice, it fails", func(t *testing.T) {
			instruction, err := example_ccip_sender.NewCcipSendInstruction(
				config.EvmChainSelector,
				[]example_ccip_sender.SvmTokenAmount{
					{Token: env.Token0.Mint, Amount: 1},
					{Token: env.Token0.Mint, Amount: 2},
				},
				[]byte{1, 2, 3},
				solana.PublicKey{},
				[]uint8{2, 16},
				senderState,
				senderEvmDestChainConfigPDA,
				senderPDA,
				env.WSOL.UserATA,
				testutils.User.PublicKey(),
				solana.SystemProgramID,
				config.CcipRouterProgram,
				config.RouterConfigPDA,
				config.EvmDestChainStatePDA,
				senderEvmNoncePDA,
				config.SPLTokenProgram,
				env.WSOL.Mint,
				wsolSenderATA,
				env.WSOL.BillingATA,
				config.BillingSignerPDA,
				config.FeeQuoterProgram,
				config.FqConfigPDA,
				config.FqEvmDestChainPDA,
				env.WSOL.FqBillingCfgPDA,
				env.Link22.FqBillingCfgPDA,
				config.RMNRemoteProgram,
				config.RMNRemoteCursesPDA,
				config.RMNRemoteConfigPDA,
			)
			require.NoError(t, err)
			genericInstruction, ok := instruction.(*solana.GenericInstruction)
			require.True(t, ok, "instruction must be *solana.GenericInstruction")
			genericInstruction.AccountValues = append(
				genericInstruction.AccountValues,
				solana.Meta(env.Token0.User[testutils.User.PublicKey()]).WRITE(),
				solana.Meta(env.Token0.User[testutils.User.PublicKey()]).WRITE(),
			)

			tokenMetas0, addressTables, err := tokens.ParseTokenLookupTableWithChain(ctx, env.Client, env.Token0, token0SenderATA, config.EvmChainSelector)
			require.NoError(t, err)
			genericInstruction.AccountValues = append(genericInstruction.AccountValues, tokenMetas0...)
			genericInstruction.AccountValues = append(genericInstruction.AccountValues, tokenMetas0...)
			maps.Copy(addressTables, env.CcipSendLookupTable)

			ixApprove0, err := tokens.TokenApproveChecked(3, token0Decimals, env.Token0.Program, env.Token0.User[testutils.User.PublicKey()], env.Token0.Mint, senderPDA, testutils.User.PublicKey(), nil)
			require.NoError(t, err)

			testutils.SendAndFailWithLookupTables(ctx, t, env.Client, []solana.Instruction{ixApprove0, instruction}, testutils.User, config.DefaultCommitment, addressTables, []string{"TransferTokenDuplicated"})
		})

		// usdCentsToLamports mirrors the router's receipt conversion (as EVM's OnRamp._getReceipts):
		// cents times one multiplier, premium% * 1e32 / price, with the WSOL price expressed as
		// 18-decimals USD for 1e18 lamports and WSOL's 100% premium. All ccip_send_v2 tests here
		// bill in WSOL.
		feeMultiplier := new(big.Int).Mul(big.NewInt(100), new(big.Int).Exp(big.NewInt(10), big.NewInt(32), nil))
		feeMultiplier.Div(feeMultiplier, testutils.WsolUsdPrice)
		usdCentsToLamports := func(cents uint16) uint64 {
			return new(big.Int).Mul(big.NewInt(int64(cents)), feeMultiplier).Uint64()
		}

		// execGasCostLamports is the execution cost the fee quoter quotes for every 2.0 message in
		// this suite (quote_gas_for_exec_v2: the lane's gas price times the corrected total gas,
		// rounded up to the cent — a flat V2LaneExecutionGasCostUsdCents at the baseline price, see
		// testutils.LaneUsdPerUnitGas). The router converts it without the premium (cents raised to
		// 36 decimals over the price) and adds it to the executor's receipt on top of the
		// executor's own fee, so it is part of both the quote and the amount the sender is billed.
		execGasCostLamports := new(big.Int).Div(
			new(big.Int).Mul(big.NewInt(int64(testutils.V2LaneExecutionGasCostUsdCents)), new(big.Int).Exp(big.NewInt(10), big.NewInt(34), nil)),
			testutils.WsolUsdPrice,
		).Uint64()

		// feeBalances snapshots the balances of the given fee token accounts, in order. Used for
		// before/after assertions on flat fee distribution: ccip_send_v2 pulls the total quoted
		// fee from the sender and pays each receipt issuer's share into that issuer's fee vault
		// ATA, with the network fee (and any legacy-pool fee) remaining in the onramp fee vault.
		feeBalances := func(accounts ...solana.PublicKey) []uint64 {
			balances := make([]uint64, len(accounts))
			for i, account := range accounts {
				balances[i] = env.GetBalance(account)
			}
			return balances
		}

		// sendLookupTables fetches the address lookup tables the resolver asked for: the router's
		// onramp table (router / fee quoter / RMN / executor / CCV statics plus the issuer
		// fee-vault ATAs) and any token pool's. Every ccip_send_v2 and get_fee_v2 send needs
		// these — the resolved account list is well past what fits in a legacy (LUT-less)
		// transaction.
		sendLookupTables := func(resolved ccip.ResolveAccountsResult) map[solana.PublicKey]solana.PublicKeySlice {
			lookupTables := make(map[solana.PublicKey]solana.PublicKeySlice, len(resolved.LookupTables))
			for _, table := range resolved.LookupTables {
				entries, lutErr := common.GetAddressLookupTable(ctx, env.Client, table)
				require.NoError(t, lutErr)
				lookupTables[table] = entries
			}
			return lookupTables
		}

		// receiptsTotal sums the fee amounts of all receipts in a CCIPMessageSentV2 event, which
		// is the exact amount the sender is billed.
		receiptsTotal := func(receipts []ccip_router.Receipt) uint64 {
			var sum uint64
			for _, r := range receipts {
				sum += r.FeeTokenAmount
			}
			return sum
		}

		// The router returns its onramp lookup table from ccip_send_v2 / get_fee_v2 account
		// resolution, so senders never attach it themselves. The ReferenceAddresses PDA is optional: until the
		// owner creates it, resolution still works and just returns no such table. Every v2 send
		// below relies on it being set here, since sendLookupTables only fetches resolved tables.
		t.Run("onramp lookup table", func(t *testing.T) {
			message := ccip_router.Svm2AnyMessage{
				FeeToken: env.WSOL.Mint,
				Receiver: env.ValidEvmAbiReceiverAddress[:],
				Data:     []byte{4, 5, 6},
				ExtraArgs: testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV3{
					GasLimit: ccip_common.CrossChainGas{V0: 5000},
					Ccvs:     []solana.PublicKey{config.CommitteeVerifierProgram},
					CcvArgs:  [][]byte{{}},
				}, ccip.GenericExtraArgsV3Tag),
			}
			resolvedTables := func(t *testing.T) (send, getFee []solana.PublicKey) {
				resolvedSend, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
				require.NoError(t, err)
				resolvedGetFee, err := ccip.ResolveGetFeeAccounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
				require.NoError(t, err)
				return resolvedSend.LookupTables, resolvedGetFee.LookupTables
			}

			t.Run("resolution returns no onramp table before one is set", func(t *testing.T) {
				_, err := env.Client.GetAccountInfo(ctx, config.RouterReferenceAddressesPDA)
				require.ErrorIs(t, err, rpc.ErrNotFound)

				send, getFee := resolvedTables(t)
				require.Empty(t, send)
				require.Empty(t, getFee)
			})

			t.Run("only the owner can set it", func(t *testing.T) {
				// Admin is the router's upgrade authority but no longer its owner (see setup).
				ix := testutils.UpdateRouterReferenceAddressesInstruction(t, env.CcipSendLookupTableAddr, testutils.Admin.PublicKey())
				testutils.SendAndFailWith(ctx, t, env.Client, []solana.Instruction{ix}, testutils.Admin, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
			})

			t.Run("the owner sets it and resolution returns it", func(t *testing.T) {
				ix := testutils.UpdateRouterReferenceAddressesInstruction(t, env.CcipSendLookupTableAddr, testutils.CcipAdmin.PublicKey())
				result := testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.CcipAdmin, config.DefaultCommitment)
				tx, err := result.Transaction.GetTransaction()
				require.NoError(t, err)
				event, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_ReferenceAddressesSet)
				require.NoError(t, err)
				require.Equal(t, env.CcipSendLookupTableAddr, event.OnrampLookupTable)

				var referenceAddresses ccip_router.ReferenceAddresses
				require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, env.Client, config.RouterReferenceAddressesPDA, config.DefaultCommitment, &referenceAddresses))
				require.Equal(t, uint8(1), referenceAddresses.Version)
				require.Equal(t, env.CcipSendLookupTableAddr, referenceAddresses.OnrampLookupTable)

				send, getFee := resolvedTables(t)
				require.Equal(t, []solana.PublicKey{env.CcipSendLookupTableAddr}, send)
				require.Equal(t, []solana.PublicKey{env.CcipSendLookupTableAddr}, getFee)
			})

			t.Run("the zero key clears it, and it can be set again", func(t *testing.T) {
				clearIx := testutils.UpdateRouterReferenceAddressesInstruction(t, solana.PublicKey{}, testutils.CcipAdmin.PublicKey())
				testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{clearIx}, testutils.CcipAdmin, config.DefaultCommitment)
				send, getFee := resolvedTables(t)
				require.Empty(t, send)
				require.Empty(t, getFee)

				env.SetOnrampLookupTable()
				send, getFee = resolvedTables(t)
				require.Equal(t, []solana.PublicKey{env.CcipSendLookupTableAddr}, send)
				require.Equal(t, []solana.PublicKey{env.CcipSendLookupTableAddr}, getFee)
			})
		})

		t.Run("ccip_send_v2 with resolved accounts emits CCIPMessageSentV2", func(t *testing.T) {
			// The committee verifier is registered as a CCV for the v2 lane in setupCV. The CCV is
			// requested through the message's V3 extra args (no longer a mock input); receipts are
			// produced for real by the router calling the CCV's get_fee.
			ccvAddr := config.CommitteeVerifierProgram

			message := ccip_router.Svm2AnyMessage{
				FeeToken: env.WSOL.Mint,
				Receiver: env.ValidEvmAbiReceiverAddress[:], // this has additional leading zeros (32 bytes instead of 20) to test the router's receiver normalization
				Data:     []byte{4, 5, 6},
				ExtraArgs: testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV3{
					GasLimit: ccip_common.CrossChainGas{V0: 5000},
					Ccvs:     []solana.PublicKey{ccvAddr},
					CcvArgs:  [][]byte{{}},
				}, ccip.GenericExtraArgsV3Tag),
			}

			resolved, err := ccip.ResolveCcipSendV2Accounts(
				ctx,
				env.Client,
				config.CcipRouterProgram,
				config.EvmChainSelector,
				message,
				testutils.User.PublicKey(),
				logger.Test(t),
			)
			require.NoError(t, err)
			require.GreaterOrEqual(t, len(resolved.AccountsToSave), 4)
			ccvAccounts := resolved.AccountsToSave[len(resolved.AccountsToSave)-4:]
			require.Equal(t, ccvAddr, ccvAccounts[0].PublicKey)
			require.Equal(t, config.CVConfigPDA, ccvAccounts[1].PublicKey)
			require.Equal(t, config.CCVSignerPDA, ccvAccounts[2].PublicKey)
			require.False(t, ccvAccounts[2].IsSigner)
			require.Equal(t, config.CVEvmRemoteChainConfigPDA, ccvAccounts[3].PublicKey)

			result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved))
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)

			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
			require.NoError(t, err)
			require.Equal(t, config.EvmChainSelector, event.DestChainSelector)
			require.Equal(t, testutils.User.PublicKey(), event.Sender)
			require.Equal(t, message.FeeToken, event.FeeToken)
			require.NotEqual(t, [32]uint8{}, event.MessageId)

			// Receipts follow the canonical [CCV.., executor, network fee] order. The CCV receipt
			// comes from the CCV's get_fee (the committee verifier currently quotes zero), the
			// executor receipt comes from the executor's get_fee (setupExecutor configures a zero
			// fee for the lane; the non-zero path is covered by a dedicated fee subtest below) plus
			// the fee quoter's execution cost, and the network-fee receipt carries the lane's
			// configured message fee, converted to lamports.
			require.Len(t, event.Receipts, 3)
			require.Equal(t, ccvAddr, event.Receipts[0].Issuer)
			// setupCV configures the committee verifier's EVM lane with a zero fee, so its receipt
			// quotes zero here (the non-zero path is covered by the dedicated fee subtest below).
			require.Equal(t, uint64(0), event.Receipts[0].FeeTokenAmount)

			executorReceipt := event.Receipts[1]
			// The executor's own fee is zero, so its receipt carries just the execution cost.
			require.Equal(t, execGasCostLamports, executorReceipt.FeeTokenAmount)
			// Gas accounting is real: the lane's base execution gas plus the requested gas limit.
			require.Equal(t, ccip_router.CrossChainGas{V0: testutils.V2LaneBaseExecutionGasCost + 5000}, executorReceipt.DestGasLimit)

			networkFeeReceipt := event.Receipts[2]
			require.Equal(t, config.CcipRouterProgram, networkFeeReceipt.Issuer)
			require.Equal(t, usdCentsToLamports(testutils.V2LaneMessageNetworkFeeUsdCents), networkFeeReceipt.FeeTokenAmount)

			require.Len(t, event.VerifierBlobs, 1)
			require.Equal(t, []byte{0xe9, 0xa0, 0x5a, 0x20}, event.VerifierBlobs[0])

			decoded, err := protocol.DecodeMessage(event.EncodedMessage)
			require.NoError(t, err)
			require.Equal(t, uint32(5000), decoded.CcipReceiveGasLimit)
			require.Equal(t, uint8(20), decoded.ReceiverLength)
			require.Equal(t, env.ValidEvmRawReceiverAddress[:], decoded.Receiver.Bytes()) // this has just 20 bytes (raw, not abi-encoded)

			// The encoded gas limit is the *corrected* total the fee quoter returns: every receipt's
			// gas limit plus the lane's per-payload-byte gas applied to their total byte overhead.
			var wantExecutionGasLimit, wantBytesOverhead uint32
			for _, r := range event.Receipts {
				wantExecutionGasLimit += r.DestGasLimit.V0
				wantBytesOverhead += r.DestBytesOverhead
			}
			wantExecutionGasLimit += wantBytesOverhead * env.ValidFqDestChainConfigV2.DestGasPerPayloadByteBase
			require.Equal(t, wantExecutionGasLimit, decoded.ExecutionGasLimit)
		})

		// The token stages are only reached through resolution for messages carrying a token, but
		// each one is independently callable, so a caller can hand any of them arbitrary inputs.
		// They must reject malformed ones with an error rather than panic.
		t.Run("token resolution stages reject malformed inputs", func(t *testing.T) {
			resolveParams := func(t *testing.T, tokenAmounts []ccip_router.SvmTokenAmount, metadata []byte) ccip_router.ResolveAccountsParams {
				ixData := append([]byte{}, ccip_router.Instruction_CcipSendV2[:]...)
				sendParams, err := bin.MarshalBorsh(ccip_router.CcipSendV2Params{
					DestChainSelector: config.EvmChainSelector,
					Message: ccip_router.Svm2AnyMessage{
						FeeToken:     env.WSOL.Mint,
						Receiver:     env.ValidEvmAbiReceiverAddress[:],
						TokenAmounts: tokenAmounts,
					},
					ResolutionMetadata: []byte{},
				})
				require.NoError(t, err)
				return ccip_router.ResolveAccountsParams{
					Caller:   testutils.User.PublicKey(),
					IxData:   append(ixData, sendParams...),
					Metadata: metadata,
				}
			}
			// Borsh encoding of a CcipSendTokenStaticStageMetadata: three zero list lengths, an
			// empty ccvs vector, then the lookup table cursor.
			staticStageMetadata := func(cursor uint8) []byte {
				return []byte{0, 0, 0, 0, 0, 0, 0, cursor}
			}
			unused := solana.NewWallet().PublicKey()
			token0Amounts := []ccip_router.SvmTokenAmount{{Token: env.Token0.Mint, Amount: 1}}
			lutError := []string{"Error Code: " + ccip.InvalidInputsLookupTableAccounts_CcipRouterError.String()}

			t.Run("without token amounts", func(t *testing.T) {
				// Borsh encoding of an empty CcipSendTokenStagesMetadata: three zero list lengths
				// followed by two empty vectors.
				emptyTokenStagesMetadata := make([]byte, 11)
				params := resolveParams(t, nil, emptyTokenStagesMetadata)
				staticParams := resolveParams(t, nil, staticStageMetadata(0))

				lutIx, err := ccip_router.NewResolveAccountsTokenTransferRetrieveLutStageInstruction(
					params, config.RouterConfigPDA, unused,
				)
				require.NoError(t, err)
				// The lookup table must be a genuine one to get past the account constraints.
				staticIx, err := ccip_router.NewResolveAccountsTokenTransferStaticAccountsStageInstruction(
					staticParams, config.RouterConfigPDA, unused, env.Token0.PoolLookupTable, unused,
				)
				require.NoError(t, err)
				nestedIx, err := ccip_router.NewResolveAccountsNestedTokenStageInstruction(
					params, config.RouterConfigPDA, unused, unused, config.EvmDestChainStatePDAV2,
				)
				require.NoError(t, err)

				for name, ix := range map[string]solana.Instruction{"retrieve lut": lutIx, "static accounts": staticIx, "nested token": nestedIx} {
					t.Run(name, func(t *testing.T) {
						testutils.SendAndFailWith(ctx, t, env.Client, []solana.Instruction{ix}, testutils.User, config.DefaultCommitment, []string{"Error Code: " + ccip.InvalidInstructionData_CcipRouterError.String()})
					})
				}
			})

			for _, tc := range []struct {
				name     string
				tokenLut solana.PublicKey
				cursor   uint8
			}{
				// Not owned by the lookup table program, so its contents mean nothing.
				{name: "with an account that is not a lookup table", tokenLut: unused},
				// A genuine table, but registered for a different token.
				{name: "with another token's lookup table", tokenLut: env.Token1.PoolLookupTable},
				// The cursor is caller-supplied, and this one points past the end of the table.
				{name: "with a cursor past the end of the lookup table", tokenLut: env.Token0.PoolLookupTable, cursor: 200},
			} {
				t.Run("static accounts "+tc.name, func(t *testing.T) {
					ix, err := ccip_router.NewResolveAccountsTokenTransferStaticAccountsStageInstruction(
						resolveParams(t, token0Amounts, staticStageMetadata(tc.cursor)),
						config.RouterConfigPDA, env.Token0.AdminRegistryPDA, tc.tokenLut, env.WSOL.Mint,
					)
					require.NoError(t, err)
					testutils.SendAndFailWith(ctx, t, env.Client, []solana.Instruction{ix}, testutils.User, config.DefaultCommitment, lutError)
				})
			}
		})

		// The v2 lane carries its own sender allowlist, enforced by ccip_send_v2 exactly like the
		// legacy path enforces the v1 lane's (see "When sending with an empty but enabled allowlist,
		// it fails" above). The CCVs' own allowlists are separate and do not stand in for it.
		t.Run("ccip_send_v2 enforces the lane sender allowlist", func(t *testing.T) {
			updateLaneConfig := func(t *testing.T, cfg ccip_router.DestChainConfigCcipV2) {
				ix, err := ccip_router.NewUpdateDestChainConfigV2Instruction(
					ccip_router.UpdateChainSelectorV2Params{
						DestChainSelector: config.EvmChainSelector,
						DestChainConfig:   cfg,
					},
					config.EvmDestChainStatePDAV2, config.RouterConfigPDA,
					testutils.CcipAdmin.PublicKey(), solana.SystemProgramID,
					config.RouterEventAuthorityPDA, config.CcipRouterProgram,
				)
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.CcipAdmin, config.DefaultCommitment)
			}

			message := ccip_router.Svm2AnyMessage{
				FeeToken:  env.WSOL.Mint,
				Receiver:  env.ValidEvmRawReceiverAddress[:],
				Data:      []byte{4, 5, 6},
				ExtraArgs: []byte{},
			}

			sendIx := func(t *testing.T) ccip.ResolveAccountsResult {
				resolved, err := ccip.ResolveCcipSendV2Accounts(
					ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector,
					message, testutils.User.PublicKey(), logger.Test(t),
				)
				require.NoError(t, err)
				return resolved
			}

			// Restore the baseline lane config so the rest of the suite is unaffected.
			defer updateLaneConfig(t, testutils.DefaultV2DestChainConfig())

			t.Run("an empty but enabled allowlist rejects everyone", func(t *testing.T) {
				cfg := testutils.DefaultV2DestChainConfig()
				cfg.AllowListEnabled = true
				updateLaneConfig(t, cfg)

				resolved := sendIx(t)
				testutils.SendAndFailWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved), []string{"Error Code: SenderNotAllowed"})
			})

			t.Run("a sender missing from a non-empty allowlist is rejected", func(t *testing.T) {
				cfg := testutils.DefaultV2DestChainConfig()
				cfg.AllowListEnabled = true
				cfg.AllowedSenders = []solana.PublicKey{solana.NewWallet().PublicKey()}
				updateLaneConfig(t, cfg)

				resolved := sendIx(t)
				testutils.SendAndFailWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved), []string{"Error Code: SenderNotAllowed"})
			})

			t.Run("an allowlisted sender succeeds", func(t *testing.T) {
				cfg := testutils.DefaultV2DestChainConfig()
				cfg.AllowListEnabled = true
				cfg.AllowedSenders = []solana.PublicKey{testutils.User.PublicKey()}
				updateLaneConfig(t, cfg)

				resolved := sendIx(t)
				result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved))
				require.NotNil(t, result)
			})
		})

		t.Run("ccip_send_v2 with empty extra args falls back to the lane default CCV", func(t *testing.T) {
			// Empty extra args => the merged CCV list is exactly the baseline lane's single default
			// CCV (the committee verifier). A lane with zero CCVs is rejected at config time by the
			// router (mirroring EVM `_validateDefaultAndMandatedCCVs`), so the zero-CCV path is no
			// longer reachable.
			message := ccip_router.Svm2AnyMessage{
				FeeToken:  env.WSOL.Mint,
				Receiver:  env.ValidEvmRawReceiverAddress[:],
				Data:      []byte{7, 8, 9},
				ExtraArgs: []byte{},
			}

			resolved, err := ccip.ResolveCcipSendV2Accounts(
				ctx,
				env.Client,
				config.CcipRouterProgram,
				config.EvmChainSelector,
				message,
				testutils.User.PublicKey(),
				logger.Test(t),
			)
			require.NoError(t, err)

			result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved))
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)

			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
			require.NoError(t, err)
			require.Equal(t, config.EvmChainSelector, event.DestChainSelector)
			require.Equal(t, testutils.User.PublicKey(), event.Sender)
			// The lane default CCV (committee verifier) is applied and produces its verifier blob.
			require.Len(t, event.VerifierBlobs, 1)
			require.Equal(t, []byte{0xe9, 0xa0, 0x5a, 0x20}, event.VerifierBlobs[0])
			// Canonical [CCV, executor, network fee] receipts.
			require.Len(t, event.Receipts, 3)
			require.Equal(t, config.CommitteeVerifierProgram, event.Receipts[0].Issuer)
			require.Equal(t, config.CcipRouterProgram, event.Receipts[2].Issuer)
		})

		setToken1Interface := func(t *testing.T, version uint8, resolutionType ccip_router.ResolutionType) {
			t.Helper()
			ixInterface, ixErr := ccip_router.NewSetPoolInterfaceImplementationInstruction(
				ccip_router.SetPoolInterfaceImplementationParams{
					Mint: env.Token1.Mint, InterfaceVersion: version, AccountResolution: resolutionType,
				},
				config.RouterConfigPDA, env.Token1.AdminRegistryPDA, testutils.Token1Admin.PublicKey(),
				config.RouterEventAuthorityPDA, config.CcipRouterProgram,
			)
			require.NoError(t, ixErr)
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixInterface}, testutils.Token1Admin, config.DefaultCommitment)
		}

		t.Run("ccip_send_v2 with 2.0 pool token transfer that does not customize fees", func(t *testing.T) {
			_, initSupply, err := tokens.TokenSupply(ctx, env.Client, env.Token1.Mint, config.DefaultCommitment)
			require.NoError(t, err)
			_, initBal, err := tokens.TokenBalance(ctx, env.Client, env.Token1.User[testutils.User.PublicKey()], config.DefaultCommitment)
			require.NoError(t, err)
			feesBefore := feeBalances(env.WSOL.UserATA, env.WSOL.BillingATA, env.WSOL.BurnMintPoolVaultATA)

			message := ccip_router.Svm2AnyMessage{
				FeeToken:  env.WSOL.Mint,
				Receiver:  env.ValidEvmRawReceiverAddress[:],
				Data:      []byte{7, 8, 9},
				ExtraArgs: []byte{},
				TokenAmounts: []ccip_router.SvmTokenAmount{
					{Token: env.Token1.Mint, Amount: 1},
				},
			}

			resolved, err := ccip.ResolveCcipSendV2Accounts(
				ctx,
				env.Client,
				config.CcipRouterProgram,
				config.EvmChainSelector,
				message,
				testutils.User.PublicKey(),
				logger.Test(t),
			)
			require.NoError(t, err)

			lookupTables := sendLookupTables(resolved)
			userTokenAccount, ok := env.Token1.User[testutils.User.PublicKey()]
			require.True(t, ok)

			ixApprove, err := tokens.TokenApproveChecked(1, token1Decimals, env.Token1.Program, userTokenAccount, env.Token1.Mint, config.BillingSignerPDA, testutils.User.PublicKey(), nil)
			require.NoError(t, err)

			// The fee CPIs (token price, pool get_fee fallback) put the legacy transfer just above
			// the 300k mark.
			result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{ixApprove, resolved.Ix}, testutils.User, config.DefaultCommitment, lookupTables, common.AddComputeUnitLimit(400_000))
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)

			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
			require.NoError(t, err)
			require.Equal(t, config.EvmChainSelector, event.DestChainSelector)
			require.Equal(t, testutils.User.PublicKey(), event.Sender)
			// The lane's default CCV (committee verifier) is merged in even for a legacy token
			// transfer, so its verifier blob is present.
			require.Len(t, event.VerifierBlobs, 1)
			require.Equal(t, []byte{0xe9, 0xa0, 0x5a, 0x20}, event.VerifierBlobs[0])

			// Receipts: [CCV, token, executor, network fee]. Token1's fee-quoter transfer fee
			// config is disabled, so the token receipt comes from the fee quoter fallback built
			// on the dest chain's token defaults (default flat fee and gas overhead, plus the
			// lock-or-burn return size as byte overhead), converted to lamports.
			require.Len(t, event.Receipts, 4)
			tokenReceipt := event.Receipts[1]
			require.Equal(t, config.CcipBurnMintTokenPoolProgram, tokenReceipt.Issuer)
			require.Equal(t, usdCentsToLamports(env.ValidFqDestChainConfig.DefaultTokenFeeUsdcents), tokenReceipt.FeeTokenAmount)
			require.Equal(t, ccip_router.CrossChainGas{V0: env.ValidFqDestChainConfig.DefaultTokenDestGasOverhead.V0}, tokenReceipt.DestGasLimit)
			require.Equal(t, uint32(32), tokenReceipt.DestBytesOverhead) // CCIP_LOCK_OR_BURN_V1_RET_BYTES
			// Token transfers pay the (higher) token-transfer network fee.
			networkFeeReceipt := event.Receipts[3]
			require.Equal(t, config.CcipRouterProgram, networkFeeReceipt.Issuer)
			require.Equal(t, usdCentsToLamports(testutils.V2LaneTokenTransferNetworkFeeUsdCents), networkFeeReceipt.FeeTokenAmount)

			// The message carries no extra args, so the token receiver falls back to the message
			// receiver, normalized to the lane's address length (the dedicated token receiver
			// subtests below cover the extra args override).
			decoded, err := protocol.DecodeMessage(event.EncodedMessage)
			require.NoError(t, err)
			require.NotNil(t, decoded.TokenTransfer)
			require.Equal(t, uint8(20), decoded.TokenTransfer.TokenReceiverLength)
			require.Equal(t, env.ValidEvmRawReceiverAddress[:], []byte(decoded.TokenTransfer.TokenReceiver))

			// check pool event (2.0)
			poolEvent, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipBurnMintTokenPoolProgram, base_token_pool.ParseEvent_LockedOrBurned)
			require.NoError(t, err)
			require.Equal(t, env.Token1.RouterSigner, poolEvent.Sender)
			require.Equal(t, uint64(1), poolEvent.Amount)
			require.Equal(t, env.Token1.Mint, poolEvent.Mint)

			// check that the fee came from FQ fallback
			// it's the 2nd call into FQ, after the one to fetch the fee token's price
			fqCpi, err := common.ExtractCPI(tx, result.Meta, config.FeeQuoterProgram, 1)
			require.NoError(t, err)
			require.Equal(t, fee_quoter.Instruction_GetFallbackTokenTransferFeeV2, [8]byte(fqCpi.Data[:8]))

			// check balances
			_, currSupply, err := tokens.TokenSupply(ctx, env.Client, env.Token1.Mint, config.DefaultCommitment)
			require.NoError(t, err)
			require.Equal(t, 1, initSupply-currSupply)
			_, currBal, err := tokens.TokenBalance(ctx, env.Client, env.Token1.User[testutils.User.PublicKey()], config.DefaultCommitment)
			require.NoError(t, err)
			require.Equal(t, 1, initBal-currBal)
			_, poolBal, err := tokens.TokenBalance(ctx, env.Client, env.Token1.PoolTokenAccount, config.DefaultCommitment)
			require.NoError(t, err)
			require.Equal(t, 0, poolBal)

			// check fee distribution: the sender is billed the sum of all receipts. Token1's
			// registry declares interface version 2 (see setupTokenAdminRegistries), so despite
			// the pool billing through the fee quoter fallback it is a v2 issuer and receives
			// its fee share in its vault ATA, exactly like on EVM where any IPoolV2 pool is paid
			// directly. The network fee remains in the onramp fee vault.
			feesAfter := feeBalances(env.WSOL.UserATA, env.WSOL.BillingATA, env.WSOL.BurnMintPoolVaultATA)
			require.Equal(t, receiptsTotal(event.Receipts), feesBefore[0]-feesAfter[0])
			require.Equal(t, networkFeeReceipt.FeeTokenAmount, feesAfter[1]-feesBefore[1])
			require.Equal(t, tokenReceipt.FeeTokenAmount, feesAfter[2]-feesBefore[2])
		})

		t.Run("ccip_send_v2 keeps a legacy (1.6 interface) pool's fee in the onramp", func(t *testing.T) {
			// Flip Token1's registry to the true 1.6 interface for the duration of this subtest:
			// the fee for a legacy pool is not paid out but accrues in the onramp fee vault,
			// mirroring EVM where non-IPoolV2 pools leave their share on the OnRamp for later
			// withdrawal. (Account resolution for v1 pools is not supported on the v2 lane, so
			// it is disabled together with the version downgrade.)
			setToken1Interface(t, 1, ccip_router.ResolutionType_NoResolution)
			defer setToken1Interface(t, 2, ccip_router.ResolutionType_Standard)

			feesBefore := feeBalances(env.WSOL.UserATA, env.WSOL.BillingATA, env.WSOL.BurnMintPoolVaultATA)

			message := ccip_router.Svm2AnyMessage{
				FeeToken:  env.WSOL.Mint,
				Receiver:  env.ValidEvmRawReceiverAddress[:],
				Data:      []byte{7, 8, 9},
				ExtraArgs: []byte{},
				TokenAmounts: []ccip_router.SvmTokenAmount{
					{Token: env.Token1.Mint, Amount: 1},
				},
			}

			resolved, err := ccip.ResolveCcipSendV2Accounts(
				ctx,
				env.Client,
				config.CcipRouterProgram,
				config.EvmChainSelector,
				message,
				testutils.User.PublicKey(),
				logger.Test(t),
			)
			require.NoError(t, err)

			lookupTables := sendLookupTables(resolved)

			userTokenAccount, ok := env.Token1.User[testutils.User.PublicKey()]
			require.True(t, ok)
			ixApprove, err := tokens.TokenApproveChecked(1, token1Decimals, env.Token1.Program, userTokenAccount, env.Token1.Mint, config.BillingSignerPDA, testutils.User.PublicKey(), nil)
			require.NoError(t, err)

			result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{ixApprove, resolved.Ix}, testutils.User, config.DefaultCommitment, lookupTables, common.AddComputeUnitLimit(400_000))
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
			require.NoError(t, err)

			// Receipts: [CCV, token, executor, network fee], with the token receipt built from
			// the fee quoter fallback and issued by the pool program.
			require.Len(t, event.Receipts, 4)
			tokenReceipt := event.Receipts[1]
			require.Equal(t, config.CcipBurnMintTokenPoolProgram, tokenReceipt.Issuer)
			require.Equal(t, usdCentsToLamports(env.ValidFqDestChainConfig.DefaultTokenFeeUsdcents), tokenReceipt.FeeTokenAmount)
			networkFeeReceipt := event.Receipts[3]
			require.Equal(t, config.CcipRouterProgram, networkFeeReceipt.Issuer)

			// The sender is still billed the full receipt total, but the legacy pool's share is
			// not paid out: it remains in the onramp fee vault together with the network fee, and
			// the pool's vault ATA is untouched.
			feesAfter := feeBalances(env.WSOL.UserATA, env.WSOL.BillingATA, env.WSOL.BurnMintPoolVaultATA)
			require.Equal(t, receiptsTotal(event.Receipts), feesBefore[0]-feesAfter[0])
			require.Equal(t, networkFeeReceipt.FeeTokenAmount+tokenReceipt.FeeTokenAmount, feesAfter[1]-feesBefore[1])
			require.Equal(t, feesBefore[2], feesAfter[2])
		})

		t.Run("ccip_send_v2 with a pure 1.6 pool token transfer enforces the lane default CCV", func(t *testing.T) {
			// Flip Token1's registry to the true 1.6 interface for the duration of this subtest:
			// the fee for a legacy pool is not paid out but accrues in the onramp fee vault,
			// mirroring EVM where non-IPoolV2 pools leave their share on the OnRamp for later
			// withdrawal. (Account resolution for v1 pools is not supported on the v2 lane, so
			// it is disabled together with the version downgrade.)
			setToken1Interface(t, 1, ccip_router.ResolutionType_NoResolution)
			defer setToken1Interface(t, 2, ccip_router.ResolutionType_Standard)

			// A pure token transfer (no data, zero receiver) skips the user/default CCV fallback
			// entirely, so the pool is the only source that can pull in the lane defaults. A
			// legacy (1.6) pool has no get_required_ccvs interface: it contributes the
			// include-defaults sentinel (EVM _getCCVsForPool parity), so the transfer is still
			// verified by the lane default CCV instead of requiring zero CCVs.
			message := ccip_router.Svm2AnyMessage{
				FeeToken:  env.WSOL.Mint,
				Receiver:  make([]byte, 20), // zero receiver + no data → pure token transfer
				ExtraArgs: []byte{},
				TokenAmounts: []ccip_router.SvmTokenAmount{
					{Token: env.Token1.Mint, Amount: 1},
				},
			}

			resolved, err := ccip.ResolveCcipSendV2Accounts(
				ctx,
				env.Client,
				config.CcipRouterProgram,
				config.EvmChainSelector,
				message,
				testutils.User.PublicKey(),
				logger.Test(t),
			)
			require.NoError(t, err)

			lookupTables := sendLookupTables(resolved)

			userTokenAccount, ok := env.Token1.User[testutils.User.PublicKey()]
			require.True(t, ok)
			ixApprove, err := tokens.TokenApproveChecked(1, token1Decimals, env.Token1.Program, userTokenAccount, env.Token1.Mint, config.BillingSignerPDA, testutils.User.PublicKey(), nil)
			require.NoError(t, err)

			result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{ixApprove, resolved.Ix}, testutils.User, config.DefaultCommitment, lookupTables, common.AddComputeUnitLimit(400_000))
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)

			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
			require.NoError(t, err)
			// The lane default CCV (committee verifier) is enforced purely through the pool
			// sentinel: it produced its verifier blob and its receipt leads the list.
			require.Len(t, event.VerifierBlobs, 1)
			require.Equal(t, []byte{0xe9, 0xa0, 0x5a, 0x20}, event.VerifierBlobs[0])
			require.Len(t, event.Receipts, 4)
			require.Equal(t, config.CommitteeVerifierProgram, event.Receipts[0].Issuer)

			// Check that the pool interface called was v1
			poolEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, base_token_pool.ParseEvent_Burned) // v1 event
			require.NoError(t, err)
			require.Equal(t, message.TokenAmounts[0].Amount, poolEvent.Amount)
			require.Equal(t, env.Token1.Mint, poolEvent.Mint)
			require.Equal(t, env.Token1.RouterSigner, poolEvent.Sender)

			// Quote/send parity: get_fee_v2 runs the same CCV merge (including the pool
			// sentinel), so its quote equals the sum of the receipts the send just emitted.
			resolvedFee, err := ccip.ResolveGetFeeAccounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			feeLookupTables := sendLookupTables(resolvedFee)
			feeResult := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolvedFee.Ix}, testutils.User, config.DefaultCommitment, feeLookupTables, common.AddComputeUnitLimit(400_000))
			require.NotNil(t, feeResult)
			quote, err := common.ExtractAnchorTypedReturnValue[ccip_router.GetFeeResultV2](ctx, feeResult.Meta.LogMessages, config.CcipRouterProgram.String())
			require.NoError(t, err)
			require.Equal(t, env.WSOL.Mint, quote.Token)
			var receiptSum uint64
			for _, r := range event.Receipts {
				receiptSum += r.FeeTokenAmount
			}
			require.Equal(t, receiptSum, quote.Amount)
		})

		// The token receiver is the address the transferred tokens land on at the destination. It
		// comes from the message's V3 extra args, and only when those leave it empty does it fall
		// back to the message receiver (covered by the transfer subtests above and below, which send
		// no extra args). Either source is normalized to the lane's configured address length.
		//
		// It is only observable in the encoded message: the router passes it to the pool's
		// lock_or_burn as an informational argument, and the pool neither validates it nor includes
		// it in its LockedOrBurned event, so these cases assert on the encoded TokenTransferV1.
		t.Run("ccip_send_v2 token receiver", func(t *testing.T) {
			// A destination address distinct from env.ValidEvmRawReceiverAddress (which stays the
			// *message* receiver below), in both its raw 20-byte and abi-encoded (left-zero-padded
			// to 32 bytes) forms.
			rawTokenReceiver := [20]byte{}
			rawTokenReceiver[0] = 2
			abiTokenReceiver := make([]byte, 32)
			copy(abiTokenReceiver[12:], rawTokenReceiver[:])

			tokenTransferMessage := func(receiver, data, tokenReceiver []byte) ccip_router.Svm2AnyMessage {
				return ccip_router.Svm2AnyMessage{
					FeeToken: env.WSOL.Mint,
					Receiver: receiver,
					Data:     data,
					ExtraArgs: testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV3{
						GasLimit:      ccip_common.CrossChainGas{V0: 5000},
						TokenReceiver: tokenReceiver,
					}, ccip.GenericExtraArgsV3Tag),
					TokenAmounts: []ccip_router.SvmTokenAmount{
						{Token: env.Token1.Mint, Amount: 1},
					},
				}
			}

			// Resolves the accounts for a Token1 transfer and returns the delegation plus send
			// instructions, ready to be submitted with the lookup tables resolution produced.
			transferIxs := func(t *testing.T, message ccip_router.Svm2AnyMessage) ([]solana.Instruction, map[solana.PublicKey]solana.PublicKeySlice) {
				resolved, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
				require.NoError(t, err)

				userTokenAccount, ok := env.Token1.User[testutils.User.PublicKey()]
				require.True(t, ok)
				ixApprove, err := tokens.TokenApproveChecked(1, token1Decimals, env.Token1.Program, userTokenAccount, env.Token1.Mint, config.BillingSignerPDA, testutils.User.PublicKey(), nil)
				require.NoError(t, err)

				return []solana.Instruction{ixApprove, resolved.Ix}, sendLookupTables(resolved)
			}

			// Sends the transfer and returns the decoded message the router emitted.
			send := func(t *testing.T, message ccip_router.Svm2AnyMessage) *protocol.Message {
				ixs, lookupTables := transferIxs(t, message)
				result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, ixs, testutils.User, config.DefaultCommitment, lookupTables, common.AddComputeUnitLimit(400_000))
				require.NotNil(t, result)

				tx, err := result.Transaction.GetTransaction()
				require.NoError(t, err)
				event, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
				require.NoError(t, err)
				decoded, err := protocol.DecodeMessage(event.EncodedMessage)
				require.NoError(t, err)
				require.NotNil(t, decoded.TokenTransfer)
				return decoded
			}

			t.Run("from extra args overrides the message receiver", func(t *testing.T) {
				decoded := send(t, tokenTransferMessage(env.ValidEvmRawReceiverAddress[:], []byte{7, 8, 9}, rawTokenReceiver[:]))
				require.Equal(t, rawTokenReceiver[:], []byte(decoded.TokenTransfer.TokenReceiver))
				// Only the tokens are redirected: the message receiver (which gets the data) is
				// untouched by the token receiver.
				require.Equal(t, env.ValidEvmRawReceiverAddress[:], decoded.Receiver.Bytes())
			})

			t.Run("from extra args is normalized to the lane's address length", func(t *testing.T) {
				// Both addresses are abi-encoded (32 bytes with 12 leading zeros) and are normalized
				// down to the lane's 20-byte address length independently of each other.
				decoded := send(t, tokenTransferMessage(env.ValidEvmAbiReceiverAddress[:], []byte{7, 8, 9}, abiTokenReceiver))
				require.Equal(t, uint8(20), decoded.TokenTransfer.TokenReceiverLength)
				require.Equal(t, rawTokenReceiver[:], []byte(decoded.TokenTransfer.TokenReceiver))
				require.Equal(t, uint8(20), decoded.ReceiverLength)
				require.Equal(t, env.ValidEvmRawReceiverAddress[:], decoded.Receiver.Bytes())
			})

			t.Run("from extra args is used by a pure token transfer", func(t *testing.T) {
				// The main use case for a separate token receiver: a pure token transfer carries a
				// zero message receiver (no data to deliver), so the extra args are the only place
				// that can say where the tokens should land.
				decoded := send(t, tokenTransferMessage(make([]byte, 20), nil, rawTokenReceiver[:]))
				require.Equal(t, rawTokenReceiver[:], []byte(decoded.TokenTransfer.TokenReceiver))
				require.Equal(t, make([]byte, 20), decoded.Receiver.Bytes())
			})

			// Normalization rejects anything that is not the lane's address length after stripping
			// leading zeros, exactly as it does for the message receiver. The token receiver is
			// normalized during account resolution (the nested token stage hands it to the pool as
			// the lock_or_burn receiver), so an invalid one is rejected there, before a send is ever
			// built: the sender learns about it without paying for a full transaction.
			//
			// The resolution helper only surfaces the numeric error code, so the assertion is on the
			// error name in the simulation logs the resolution rounds emit.
			resolveAndFail := func(t *testing.T, message ccip_router.Svm2AnyMessage, expectedError string) {
				// The resolver logs the simulation output at Debug, so observe at that level and
				// assert over the captured entries rather than the returned error.
				lggr, observed := logger.TestObserved(t, zapcore.DebugLevel)
				_, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), lggr)
				require.Error(t, err)

				var logs strings.Builder
				for _, entry := range observed.All() {
					logs.WriteString(entry.Message)
					logs.WriteByte('\n')
				}
				require.Contains(t, logs.String(), expectedError)
			}

			t.Run("shorter than the lane's address length is rejected", func(t *testing.T) {
				resolveAndFail(t, tokenTransferMessage(env.ValidEvmRawReceiverAddress[:], []byte{7, 8, 9}, rawTokenReceiver[:19]), "InvalidDestTokenReceiverAddress")
			})

			t.Run("falling back to an invalid message receiver is reported as a receiver error", func(t *testing.T) {
				// The sender specified no token receiver, so the failure is about their message
				// receiver: it must not be reported as a token receiver error for an address they
				// never provided.
				resolveAndFail(t, tokenTransferMessage(env.ValidEvmRawReceiverAddress[:19], []byte{7, 8, 9}, nil), "InvalidReceiverAddress")
			})

			t.Run("longer than the lane's address length with non-zero padding is rejected", func(t *testing.T) {
				dirtyPadding := make([]byte, len(abiTokenReceiver))
				copy(dirtyPadding, abiTokenReceiver)
				dirtyPadding[0] = 1

				resolveAndFail(t, tokenTransferMessage(env.ValidEvmRawReceiverAddress[:], []byte{7, 8, 9}, dirtyPadding), "InvalidDestTokenReceiverAddress")
			})
		})

		t.Run("ccip_send_v2 with 2.0 pool token transfer", func(t *testing.T) {
			v2TokenMint := solana.NewWallet().PrivateKey
			v2Token, err := tokens.NewTokenPool(config.SPLTokenProgram, config.CcipBurnMintTokenPoolProgram, v2TokenMint.PublicKey())
			require.NoError(t, err)

			ixV2, err := tokens.CreateTokenFrom(ctx, v2Token, testutils.Token0Admin.PublicKey(), v2TokenDecimals, env.Client, config.DefaultCommitment)
			require.NoError(t, err)
			ixAtaV2, addrV2, err := tokens.CreateAssociatedTokenAccount(v2Token.Program, v2Token.Mint, testutils.User.PublicKey(), testutils.Token0Admin.PublicKey())
			require.NoError(t, err)
			ixMintToV2, err := tokens.MintTo(10000000, v2Token.Program, v2Token.Mint, addrV2, testutils.Token0Admin.PublicKey())
			require.NoError(t, err)
			v2Token.User[testutils.User.PublicKey()] = addrV2
			ixV2 = append(ixV2, ixAtaV2, ixMintToV2)
			testutils.SendAndConfirm(ctx, t, env.Client, ixV2, testutils.Token0Admin, config.DefaultCommitment, common.AddSigners(v2TokenMint))

			programDataAddress := testutils.GetProgramDataAddress(ctx, t, env.Client, config.CcipBurnMintTokenPoolProgram)
			globalConfigPDA, err := tokens.TokenPoolGlobalConfigPDA(config.CcipBurnMintTokenPoolProgram)
			require.NoError(t, err)
			burnmintEventAuthority, _, _ := state.FindEventAuthorityPDA(config.CcipBurnMintTokenPoolProgram)

			ixInitV2, err := burnmint_tokenpool.NewInitializeInstruction(
				v2Token.PoolConfig,
				v2Token.Mint,
				testutils.Admin.PublicKey(),
				solana.SystemProgramID,
				config.CcipBurnMintTokenPoolProgram,
				programDataAddress,
				globalConfigPDA,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixInitV2}, testutils.Admin, config.DefaultCommitment)

			ixTransferV2, err := burnmint_tokenpool.NewTransferOwnershipInstruction(
				testutils.Token0Admin.PublicKey(),
				v2Token.PoolConfig,
				v2Token.Mint,
				testutils.Admin.PublicKey(),
			)
			require.NoError(t, err)
			ixAcceptV2, err := burnmint_tokenpool.NewAcceptOwnershipInstruction(
				v2Token.PoolConfig,
				v2Token.Mint,
				testutils.Token0Admin.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixTransferV2, ixAcceptV2}, testutils.Admin, config.DefaultCommitment, common.AddSigners(testutils.Token0Admin))

			ixAtaV2Pool, addrV2Pool, err := tokens.CreateAssociatedTokenAccount(v2Token.Program, v2Token.Mint, v2Token.PoolSigner, testutils.Token0Admin.PublicKey())
			require.NoError(t, err)
			v2Token.PoolTokenAccount = addrV2Pool
			v2Token.User[v2Token.PoolSigner] = addrV2Pool
			ixAuthV2, err := tokens.SetTokenMintAuthority(v2Token.Program, v2Token.PoolSigner, v2Token.Mint, testutils.Token0Admin.PublicKey())
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixAtaV2Pool, ixAuthV2}, testutils.Token0Admin, config.DefaultCommitment)

			require.NoError(t, v2Token.SetupLookupTable(ctx, env.Client, testutils.Token0Admin))

			// The pool program's fee vault is qualified by the managed token's mint, so this
			// fresh mint gets its own wSOL fee vault ATA, separate from the Token1 one created
			// in the shared setup. ccip_send_v2 pays the pool's receipt into it and fails with
			// MissingAtaForReceiptIssuer if it doesn't exist.
			v2PoolFeeVaultPDA, _, err := state.FindFeeVaultPDA(config.CcipBurnMintTokenPoolProgram, v2Token.Mint.Bytes())
			require.NoError(t, err)
			ixV2PoolVaultAta, v2PoolVaultATA, err := tokens.CreateAssociatedTokenAccount(config.SPLTokenProgram, solana.WrappedSol, v2PoolFeeVaultPDA, testutils.Admin.PublicKey())
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixV2PoolVaultAta}, testutils.Admin, config.DefaultCommitment)

			// The pool's in-token BPS fee accrues in that same vault's ATA for the pool's own
			// mint, which set_chain_config_v2 creates, so it only needs deriving here.
			v2PoolTokenFeeVaultATA, err := tokens.FindFeeVaultATA(v2Token.Program, v2Token.Mint, config.CcipBurnMintTokenPoolProgram, v2Token.Mint.Bytes())
			require.NoError(t, err)

			ixProposeAdmin, err := ccip_router.NewCcipAdminProposeAdministratorInstruction(
				testutils.Token0Admin.PublicKey(),
				config.RouterConfigPDA,
				v2Token.AdminRegistryPDA,
				v2Token.Mint,
				testutils.CcipAdmin.PublicKey(),
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixProposeAdmin}, testutils.CcipAdmin, config.DefaultCommitment)

			ixAcceptAdmin, err := ccip_router.NewAcceptAdminRoleTokenAdminRegistryInstruction(
				config.RouterConfigPDA,
				v2Token.AdminRegistryPDA,
				v2Token.Mint,
				testutils.Token0Admin.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixAcceptAdmin}, testutils.Token0Admin, config.DefaultCommitment)

			ixSetPool, err := ccip_router.NewSetPoolInstruction(
				v2Token.WritableIndexes,
				config.RouterConfigPDA,
				v2Token.AdminRegistryPDA,
				v2Token.Mint,
				v2Token.PoolLookupTable,
				testutils.Token0Admin.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixSetPool}, testutils.Token0Admin, config.DefaultCommitment)

			ixSetInterface, err := ccip_router.NewSetPoolInterfaceImplementationInstruction(
				ccip_router.SetPoolInterfaceImplementationParams{
					Mint:              v2Token.Mint,
					InterfaceVersion:  2,
					AccountResolution: ccip_router.ResolutionType_Standard,
				},
				config.RouterConfigPDA,
				v2Token.AdminRegistryPDA,
				testutils.Token0Admin.PublicKey(),
				config.RouterEventAuthorityPDA,
				config.CcipRouterProgram,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixSetInterface}, testutils.Token0Admin, config.DefaultCommitment)

			ixRemoteConfigV2, err := burnmint_tokenpool.NewInitChainRemoteConfigInstruction(
				config.EvmChainSelector,
				v2Token.Mint,
				burnmint_tokenpool.RemoteConfig{
					TokenAddress: burnmint_tokenpool.RemoteAddress{Address: config.EVMToken0AddressBytes},
					Decimals:     18,
				},
				v2Token.PoolConfig,
				v2Token.Chain[config.EvmChainSelector],
				testutils.Token0Admin.PublicKey(),
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			ixRateLimitV2, err := burnmint_tokenpool.NewSetChainRateLimitInstruction(
				config.EvmChainSelector,
				v2Token.Mint,
				burnmint_tokenpool.RateLimitConfig{},
				burnmint_tokenpool.RateLimitConfig{},
				v2Token.PoolConfig,
				v2Token.Chain[config.EvmChainSelector],
				testutils.Token0Admin.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixRemoteConfigV2, ixRateLimitV2}, testutils.Token0Admin, config.DefaultCommitment)

			defaultFeeConfig := fee_quoter.TokenTransferFeeConfig{DestBytesOverhead: 32, MinFee: fee_quoter.UsdCents{V0: 0}, MaxFee: fee_quoter.UsdCents{V0: 1}}
			ixBillingV2, err := fee_quoter.NewSetTokenTransferFeeConfigInstruction(
				config.EvmChainSelector,
				v2Token.Mint,
				defaultFeeConfig,
				config.FqConfigPDA,
				v2Token.Billing[config.EvmChainSelector],
				testutils.CcipAdmin.PublicKey(),
				solana.SystemProgramID,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixBillingV2}, testutils.CcipAdmin, config.DefaultCommitment)

			_, initSupply, err := tokens.TokenSupply(ctx, env.Client, v2Token.Mint, config.DefaultCommitment)
			require.NoError(t, err)
			_, initBal, err := tokens.TokenBalance(ctx, env.Client, v2Token.User[testutils.User.PublicKey()], config.DefaultCommitment)
			require.NoError(t, err)

			message := ccip_router.Svm2AnyMessage{
				FeeToken:  env.WSOL.Mint,
				Receiver:  env.ValidEvmRawReceiverAddress[:],
				Data:      []byte{7, 8, 9},
				ExtraArgs: []byte{},
				TokenAmounts: []ccip_router.SvmTokenAmount{
					{Token: v2Token.Mint, Amount: 1},
				},
			}

			resolved, err := ccip.ResolveCcipSendV2Accounts(
				ctx,
				env.Client,
				config.CcipRouterProgram,
				config.EvmChainSelector,
				message,
				testutils.User.PublicKey(),
				logger.Test(t),
			)
			require.NoError(t, err)

			lookupTables := sendLookupTables(resolved)

			userTokenAccount := v2Token.User[testutils.User.PublicKey()]
			ixApprove, err := tokens.TokenApproveChecked(1, v2TokenDecimals, v2Token.Program, userTokenAccount, v2Token.Mint, config.BillingSignerPDA, testutils.User.PublicKey(), nil)
			require.NoError(t, err)

			// A 2.0 pool send is the most CPI-heavy path in the suite (pool get_fee,
			// get_required_ccvs and lock_or_burn, the fee quoter views, the fee payouts), landing
			// just under 400k compute units — close enough to need headroom above it.
			result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{ixApprove, resolved.Ix}, testutils.User, config.DefaultCommitment, lookupTables, common.AddComputeUnitLimit(500_000))
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)

			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
			require.NoError(t, err)
			require.Equal(t, config.EvmChainSelector, event.DestChainSelector)
			require.Equal(t, testutils.User.PublicKey(), event.Sender)
			// The lane's default CCV (committee verifier) is merged in for the v2 token transfer too.
			require.Len(t, event.VerifierBlobs, 1)
			require.Equal(t, []byte{0xe9, 0xa0, 0x5a, 0x20}, event.VerifierBlobs[0])

			// Receipts: [CCV, token, executor, network fee]. The router asks the V2 pool for its
			// fee via the get_fee CPI, but the burnmint pool currently returns a disabled receipt,
			// so the router falls back to the fee quoter. This token's transfer fee config is
			// disabled as well, leaving the dest chain's token defaults.
			require.Len(t, event.Receipts, 4)
			tokenReceipt := event.Receipts[1]
			require.Equal(t, config.CcipBurnMintTokenPoolProgram, tokenReceipt.Issuer)
			require.Equal(t, usdCentsToLamports(env.ValidFqDestChainConfig.DefaultTokenFeeUsdcents), tokenReceipt.FeeTokenAmount)
			require.Equal(t, ccip_router.CrossChainGas{V0: env.ValidFqDestChainConfig.DefaultTokenDestGasOverhead.V0}, tokenReceipt.DestGasLimit)
			require.Equal(t, uint32(32), tokenReceipt.DestBytesOverhead) // CCIP_LOCK_OR_BURN_V1_RET_BYTES

			poolEvent, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipBurnMintTokenPoolProgram, base_token_pool.ParseEvent_LockedOrBurned)
			require.NoError(t, err)
			require.Equal(t, v2Token.Mint, poolEvent.Mint)
			require.Equal(t, config.EvmChainSelector, poolEvent.RemoteChainSelector)
			require.Equal(t, v2Token.RouterSigner, poolEvent.Sender)
			require.Equal(t, uint64(1), poolEvent.Amount)

			_, currSupply, err := tokens.TokenSupply(ctx, env.Client, v2Token.Mint, config.DefaultCommitment)
			require.NoError(t, err)
			require.Equal(t, 1, initSupply-currSupply)
			_, currBal, err := tokens.TokenBalance(ctx, env.Client, v2Token.User[testutils.User.PublicKey()], config.DefaultCommitment)
			require.NoError(t, err)
			require.Equal(t, 1, initBal-currBal)
			_, poolBal, err := tokens.TokenBalance(ctx, env.Client, v2Token.PoolTokenAccount, config.DefaultCommitment)
			require.NoError(t, err)
			require.Equal(t, 0, poolBal)

			// Resolves, sends and confirms a pure (no data, zero receiver) transfer of the v2
			// token, returning the emitted CCIPMessageSentV2 event. Pure transfers skip the
			// user/default CCV fallback, so the CCVs in the event come exclusively from the
			// pool contribution merge under test.
			sendPureV2TokenTransfer := func(t *testing.T) *ccip_router.CcipMessageSentV2 {
				message := ccip_router.Svm2AnyMessage{
					FeeToken:  env.WSOL.Mint,
					Receiver:  make([]byte, 32), // zero receiver + no data → pure token transfer
					ExtraArgs: []byte{},
					TokenAmounts: []ccip_router.SvmTokenAmount{
						{Token: v2Token.Mint, Amount: 1},
					},
				}

				resolved, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
				require.NoError(t, err)

				lookupTables := sendLookupTables(resolved)

				userTokenAccount := v2Token.User[testutils.User.PublicKey()]
				ixApprove, err := tokens.TokenApproveChecked(1, v2TokenDecimals, v2Token.Program, userTokenAccount, v2Token.Mint, config.BillingSignerPDA, testutils.User.PublicKey(), nil)
				require.NoError(t, err)

				result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{ixApprove, resolved.Ix}, testutils.User, config.DefaultCommitment, lookupTables, common.AddComputeUnitLimit(500_000))
				require.NotNil(t, result)

				tx, err := result.Transaction.GetTransaction()
				require.NoError(t, err)
				event, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
				require.NoError(t, err)
				return event
			}

			t.Run("pure token transfer with no pool CCV config falls back to the lane default CCV", func(t *testing.T) {
				// The pool has no chain_config_v2 for this lane yet, so get_required_ccvs answers
				// with an empty list, which collapses to the include-defaults sentinel — the same
				// outcome as the legacy pool case, but through the v2 CPI path.
				event := sendPureV2TokenTransfer(t)
				require.Len(t, event.VerifierBlobs, 1)
				require.Equal(t, []byte{0xe9, 0xa0, 0x5a, 0x20}, event.VerifierBlobs[0])
				require.Len(t, event.Receipts, 4)
				require.Equal(t, config.CommitteeVerifierProgram, event.Receipts[0].Issuer)
			})

			t.Run("pure token transfer uses the pool-required CCVs without pulling in lane defaults", func(t *testing.T) {
				// Configure the pool to require the committee verifier for outbound transfers, and
				// point the lane defaults at a program that does not exist: if the pool's
				// non-empty CCV response wrongly pulled in the lane defaults, resolution/send
				// would try to CPI the bogus program and fail. Success proves a pool that
				// specifies its own CCVs is taken verbatim.
				laneCfg := testutils.DefaultV2DestChainConfig()
				laneCfg.DefaultCcvs = []solana.PublicKey{solana.NewWallet().PublicKey()}
				updateLaneConfig := func(cfg ccip_router.DestChainConfigCcipV2) {
					ix, err := ccip_router.NewUpdateDestChainConfigV2Instruction(
						ccip_router.UpdateChainSelectorV2Params{
							DestChainSelector: config.EvmChainSelector,
							DestChainConfig:   cfg,
						},
						config.EvmDestChainStatePDAV2, config.RouterConfigPDA,
						testutils.CcipAdmin.PublicKey(), solana.SystemProgramID,
						config.RouterEventAuthorityPDA, config.CcipRouterProgram,
					)
					require.NoError(t, err)
					testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.CcipAdmin, config.DefaultCommitment)
				}
				updateLaneConfig(laneCfg)
				defer updateLaneConfig(testutils.DefaultV2DestChainConfig())

				// First-time ccv config: creates chain_config_v2 for this lane (owner-only), leaving
				// the allowed finality at WAIT_FOR_FINALITY (zero). Later subtests reconfigure it
				// with edit_chain_config_v2.
				ixSetCcv, err := burnmint_tokenpool.NewInitChainConfigV2Instruction(
					burnmint_tokenpool.SetChainConfigV2Params{
						Mint:                   v2Token.Mint,
						RemoteChainSelector:    config.EvmChainSelector,
						TokenTransferFeeConfig: burnmint_tokenpool.TokenTransferFeeConfig{},
						CcvConfig: burnmint_tokenpool.CcvConfig{
							OutboundCcvs: []solana.PublicKey{config.CommitteeVerifierProgram},
						},
					},
					v2Token.PoolConfig,
					v2Token.ChainConfigV2[config.EvmChainSelector],
					testutils.Token0Admin.PublicKey(),
					solana.SystemProgramID,
					v2Token.Mint,
					v2PoolFeeVaultPDA,
					v2PoolTokenFeeVaultATA,
					v2Token.Program,
					solana.SPLAssociatedTokenAccountProgramID,
					burnmintEventAuthority,
					config.CcipBurnMintTokenPoolProgram,
				)
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixSetCcv}, testutils.Token0Admin, config.DefaultCommitment)

				event := sendPureV2TokenTransfer(t)
				require.Len(t, event.VerifierBlobs, 1)
				require.Equal(t, []byte{0xe9, 0xa0, 0x5a, 0x20}, event.VerifierBlobs[0])
				require.Len(t, event.Receipts, 4)
				require.Equal(t, config.CommitteeVerifierProgram, event.Receipts[0].Issuer)
			})

			// The pool's allowed finality for this lane is WAIT_FOR_FINALITY (full finality only):
			// the subtest above created chain_config_v2 with a zero allowed_finality_config, which
			// matches what an unconfigured lane defaults to. A token transfer that requests a
			// *valid* fast finality (a finality flag, no block depth — passes the router's
			// FinalityConfig::is_valid, unlike the 7046 case above) must therefore be rejected by
			// the pool's lock_or_burn finality-policy gate, mirroring EVM
			// TokenPool._ensureRequestedFinalityAllowed.
			t.Run("rejects a fast-finality token transfer the pool does not allow", func(t *testing.T) {
				message := ccip_router.Svm2AnyMessage{
					FeeToken: env.WSOL.Mint,
					Receiver: env.ValidEvmRawReceiverAddress[:],
					Data:     []byte{7, 8, 9},
					ExtraArgs: testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV3{
						GasLimit: ccip_common.CrossChainGas{V0: 5000},
						// Flag-only finality: valid on the wire but not permitted by the unconfigured pool.
						Finality: ccip_common.FinalityConfig{Flags: 1},
					}, ccip.GenericExtraArgsV3Tag),
					TokenAmounts: []ccip_router.SvmTokenAmount{
						{Token: v2Token.Mint, Amount: 1},
					},
				}

				resolved, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
				require.NoError(t, err)

				lookupTables := sendLookupTables(resolved)

				userTokenAccount := v2Token.User[testutils.User.PublicKey()]
				ixApprove, err := tokens.TokenApproveChecked(1, v2TokenDecimals, v2Token.Program, userTokenAccount, v2Token.Mint, config.BillingSignerPDA, testutils.User.PublicKey(), nil)
				require.NoError(t, err)

				// The pool CPI rejects with CcipTokenPoolError::InvalidRequestedFinality.
				testutils.SendAndFailWithLookupTables(ctx, t, env.Client, []solana.Instruction{ixApprove, resolved.Ix}, testutils.User, config.DefaultCommitment, lookupTables, []string{"InvalidRequestedFinality"}, common.AddComputeUnitLimit(400_000))
			})

			// Counterpart to the rejection above: once the lane's pool is configured with a more relaxed
			// allowed finality that matches the requested flag, the same fast-finality token transfer is
			// accepted and burns successfully. This proves the gate is driven by the pool's configured
			// allowed_finality_config, not a blanket reject.
			t.Run("accepts a fast-finality token transfer the pool allows", func(t *testing.T) {
				// Relax the pool's allowed finality for this lane to permit the flag-only finality
				// (Flags:1). edit_chain_config_v2 updates chain_config_v2 (owner-only); the
				// committee verifier is the lane default CCV anyway, so keeping it required by the
				// pool changes nothing for this subtest.
				ixSetCcv, err := burnmint_tokenpool.NewEditChainConfigV2Instruction(
					burnmint_tokenpool.SetChainConfigV2Params{
						Mint:                   v2Token.Mint,
						RemoteChainSelector:    config.EvmChainSelector,
						AllowedFinalityConfig:  burnmint_tokenpool.FinalityConfig{Flags: 1},
						TokenTransferFeeConfig: burnmint_tokenpool.TokenTransferFeeConfig{},
						CcvConfig: burnmint_tokenpool.CcvConfig{
							OutboundCcvs: []solana.PublicKey{config.CommitteeVerifierProgram},
						},
					},
					v2Token.PoolConfig,
					v2Token.ChainConfigV2[config.EvmChainSelector],
					testutils.Token0Admin.PublicKey(),
					solana.SystemProgramID,
					burnmintEventAuthority,
					config.CcipBurnMintTokenPoolProgram,
				)
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixSetCcv}, testutils.Token0Admin, config.DefaultCommitment)

				_, beforeSupply, err := tokens.TokenSupply(ctx, env.Client, v2Token.Mint, config.DefaultCommitment)
				require.NoError(t, err)

				message := ccip_router.Svm2AnyMessage{
					FeeToken: env.WSOL.Mint,
					Receiver: env.ValidEvmRawReceiverAddress[:],
					Data:     []byte{7, 8, 9},
					ExtraArgs: testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV3{
						GasLimit: ccip_common.CrossChainGas{V0: 5000},
						Finality: ccip_common.FinalityConfig{Flags: 1},
					}, ccip.GenericExtraArgsV3Tag),
					TokenAmounts: []ccip_router.SvmTokenAmount{
						{Token: v2Token.Mint, Amount: 1},
					},
				}

				resolved, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
				require.NoError(t, err)

				lookupTables := sendLookupTables(resolved)

				userTokenAccount := v2Token.User[testutils.User.PublicKey()]
				ixApprove, err := tokens.TokenApproveChecked(1, v2TokenDecimals, v2Token.Program, userTokenAccount, v2Token.Mint, config.BillingSignerPDA, testutils.User.PublicKey(), nil)
				require.NoError(t, err)

				result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{ixApprove, resolved.Ix}, testutils.User, config.DefaultCommitment, lookupTables, common.AddComputeUnitLimit(450_000))
				require.NotNil(t, result)

				tx, err := result.Transaction.GetTransaction()
				require.NoError(t, err)
				poolEvent, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipBurnMintTokenPoolProgram, base_token_pool.ParseEvent_LockedOrBurned)
				require.NoError(t, err)
				require.Equal(t, v2Token.Mint, poolEvent.Mint)
				require.Equal(t, config.EvmChainSelector, poolEvent.RemoteChainSelector)
				require.Equal(t, uint64(1), poolEvent.Amount)

				_, afterSupply, err := tokens.TokenSupply(ctx, env.Client, v2Token.Mint, config.DefaultCommitment)
				require.NoError(t, err)
				require.Equal(t, 1, beforeSupply-afterSupply)
			})

			// sendV2TokenTransfer resolves and submits a token transfer of `amount` through the v2
			// pool, requesting `finality` (the zero value means WAIT_FOR_FINALITY, sent as empty
			// extra args). An empty wantErr asserts the send succeeds; otherwise it must fail with
			// that error. The pool-side rejection cases below all go through this so they exercise
			// the same real ccip_send_v2 path as the happy path above.
			sendV2TokenTransfer := func(t *testing.T, amount uint64, finality ccip_common.FinalityConfig, wantErr string) {
				t.Helper()

				extraArgs := []byte{}
				if finality != (ccip_common.FinalityConfig{}) {
					extraArgs = testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV3{
						GasLimit: ccip_common.CrossChainGas{V0: 5000},
						Finality: finality,
					}, ccip.GenericExtraArgsV3Tag)
				}

				message := ccip_router.Svm2AnyMessage{
					FeeToken:  env.WSOL.Mint,
					Receiver:  env.ValidEvmRawReceiverAddress[:],
					Data:      []byte{7, 8, 9},
					ExtraArgs: extraArgs,
					TokenAmounts: []ccip_router.SvmTokenAmount{
						{Token: v2Token.Mint, Amount: amount},
					},
				}

				resolved, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
				require.NoError(t, err)

				lookupTables := sendLookupTables(resolved)

				ixApprove, err := tokens.TokenApproveChecked(amount, v2TokenDecimals, v2Token.Program, v2Token.User[testutils.User.PublicKey()], v2Token.Mint, config.BillingSignerPDA, testutils.User.PublicKey(), nil)
				require.NoError(t, err)

				ixs := []solana.Instruction{ixApprove, resolved.Ix}
				if wantErr == "" {
					require.NotNil(t, testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, ixs, testutils.User, config.DefaultCommitment, lookupTables, common.AddComputeUnitLimit(500_000)))
					return
				}
				testutils.SendAndFailWithLookupTables(ctx, t, env.Client, ixs, testutils.User, config.DefaultCommitment, lookupTables, []string{wantErr}, common.AddComputeUnitLimit(500_000))
			}

			// setV2TokenRateLimits installs default (non-override) rate limit buckets for the lane
			// and leaves them FULL and frozen: the first write uses a refill rate equal to the
			// capacity so the elapsed second tops the bucket up, the second pins the rate to 0 so
			// nothing refills mid-test. Without the freeze the assertions below would race the
			// wall clock, since refill is driven by the on-chain unix timestamp in whole seconds.
			setV2TokenRateLimits := func(t *testing.T, capacity uint64) {
				t.Helper()
				set := func(rate uint64) {
					ix, err := burnmint_tokenpool.NewSetChainRateLimitInstruction(
						config.EvmChainSelector,
						v2Token.Mint,
						burnmint_tokenpool.RateLimitConfig{Enabled: true, Capacity: capacity, Rate: rate},
						burnmint_tokenpool.RateLimitConfig{Enabled: true, Capacity: capacity, Rate: rate},
						v2Token.PoolConfig,
						v2Token.Chain[config.EvmChainSelector],
						testutils.Token0Admin.PublicKey(),
					)
					require.NoError(t, err)
					testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.Token0Admin, config.DefaultCommitment)
				}
				time.Sleep(time.Second)
				set(capacity)
				set(0)
			}

			// clearV2TokenRateLimits restores the disabled buckets the pool was set up with, so a
			// rate limit subtest cannot leak capacity constraints into the ones that follow.
			clearV2TokenRateLimits := func(t *testing.T) {
				t.Helper()
				ix, err := burnmint_tokenpool.NewSetChainRateLimitInstruction(
					config.EvmChainSelector,
					v2Token.Mint,
					burnmint_tokenpool.RateLimitConfig{},
					burnmint_tokenpool.RateLimitConfig{},
					v2Token.PoolConfig,
					v2Token.Chain[config.EvmChainSelector],
					testutils.Token0Admin.PublicKey(),
				)
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.Token0Admin, config.DefaultCommitment)
			}

			// setFtfOverrideRateLimits initialises (or updates) the pool's FTF override bucket for
			// the lane. Like the default buckets it is left full and frozen at rate 0.
			setFtfOverrideRateLimits := func(t *testing.T, capacity uint64, enabled bool) {
				t.Helper()
				cfg := func(rate uint64) burnmint_tokenpool.RateLimitConfig {
					if !enabled {
						return burnmint_tokenpool.RateLimitConfig{}
					}
					return burnmint_tokenpool.RateLimitConfig{Enabled: true, Capacity: capacity, Rate: rate}
				}
				overridePDA := v2Token.ChainOverride[config.EvmChainSelector]

				var exists burnmint_tokenpool.ChainConfigOverride
				if err := common.GetAccountDataBorshIntoAnchor(ctx, env.Client, overridePDA, config.DefaultCommitment, &exists); err != nil {
					ixInit, iErr := burnmint_tokenpool.NewInitChainConfigOverrideInstruction(
						burnmint_tokenpool.InitChainConfigOverrideParams{
							RemoteChainSelector: config.EvmChainSelector,
							Mint:                v2Token.Mint,
							OverrideType:        burnmint_tokenpool.PoolChainConfigOverrideType_Ftf,
							Inbound:             cfg(capacity),
							Outbound:            cfg(capacity),
						},
						v2Token.PoolConfig,
						overridePDA,
						testutils.Token0Admin.PublicKey(),
						solana.SystemProgramID,
						burnmintEventAuthority,
						config.CcipBurnMintTokenPoolProgram,
					)
					require.NoError(t, iErr)
					testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixInit}, testutils.Token0Admin, config.DefaultCommitment)
				}

				set := func(rate uint64) {
					ix, err := burnmint_tokenpool.NewSetChainConfigOverrideRateLimitInstruction(
						burnmint_tokenpool.SetChainConfigOverrideRateLimitParams{
							RemoteChainSelector: config.EvmChainSelector,
							Mint:                v2Token.Mint,
							OverrideType:        burnmint_tokenpool.PoolChainConfigOverrideType_Ftf,
							Inbound:             cfg(rate),
							Outbound:            cfg(rate),
						},
						v2Token.PoolConfig,
						overridePDA,
						testutils.Token0Admin.PublicKey(),
						burnmintEventAuthority,
						config.CcipBurnMintTokenPoolProgram,
					)
					require.NoError(t, err)
					testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.Token0Admin, config.DefaultCommitment)
				}
				time.Sleep(time.Second)
				set(capacity)
				set(0)
			}

			// deleteFtfOverride closes the FTF override PDA so the pool falls back to its default
			// buckets, and so a following subtest can init it afresh.
			deleteFtfOverride := func(t *testing.T) {
				t.Helper()
				ix, err := burnmint_tokenpool.NewDeleteChainConfigOverrideInstruction(
					burnmint_tokenpool.DeleteChainConfigOverrideParams{
						RemoteChainSelector: config.EvmChainSelector,
						Mint:                v2Token.Mint,
						OverrideType:        burnmint_tokenpool.PoolChainConfigOverrideType_Ftf,
					},
					v2Token.PoolConfig,
					v2Token.ChainOverride[config.EvmChainSelector],
					testutils.Token0Admin.PublicKey(),
					burnmintEventAuthority,
					config.CcipBurnMintTokenPoolProgram,
				)
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.Token0Admin, config.DefaultCommitment)
			}

			// The pool's outbound rate limiter has two distinct rejections: a request larger than
			// the bucket's capacity can never succeed (RLMaxCapacityExceeded, checked before any
			// refill), while a request that merely outruns the tokens currently in the bucket is
			// RLRateLimitReached. Both are asserted through a real ccip_send_v2.
			t.Run("the outbound rate limit rejects a transfer larger than the bucket capacity", func(t *testing.T) {
				setV2TokenRateLimits(t, 10)
				defer clearV2TokenRateLimits(t)

				sendV2TokenTransfer(t, 11, ccip_common.FinalityConfig{}, "max capacity exceeded")
			})

			t.Run("the outbound rate limit rejects a second transfer that drains the bucket", func(t *testing.T) {
				setV2TokenRateLimits(t, 10)
				defer clearV2TokenRateLimits(t)

				// The bucket is frozen at 10 tokens, so 6 passes and the next 6 cannot.
				sendV2TokenTransfer(t, 6, ccip_common.FinalityConfig{}, "")
				sendV2TokenTransfer(t, 6, ccip_common.FinalityConfig{}, "rate limit reached")
			})

			// The pool's allow list gates lock_or_burn on the ORIGINAL SENDER, which under
			// ccip_send_v2 is the signing user. Enabling the list with a different address is
			// therefore enough to make a real send fail.
			t.Run("the pool allow list rejects a sender that is not on it", func(t *testing.T) {
				configureAllowList := func(t *testing.T, add []solana.PublicKey, enabled bool) {
					ix, err := burnmint_tokenpool.NewConfigureAllowListInstruction(
						add, enabled,
						v2Token.PoolConfig,
						v2Token.Mint,
						testutils.Token0Admin.PublicKey(),
						solana.SystemProgramID,
					)
					require.NoError(t, err)
					testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.Token0Admin, config.DefaultCommitment)
				}

				configureAllowList(t, []solana.PublicKey{testutils.Token0Admin.PublicKey()}, true)
				defer configureAllowList(t, []solana.PublicKey{}, false)

				sendV2TokenTransfer(t, 1, ccip_common.FinalityConfig{}, "InvalidSender")
			})

			// Bucket SELECTION for faster-than-finality transfers: when the FTF override exists and
			// is enabled it replaces the default outbound bucket entirely, so a pair of transfers
			// that would blow the (deliberately tight) default limit still goes through.
			t.Run("a fast-finality transfer consumes the override bucket instead of the default", func(t *testing.T) {
				setV2TokenRateLimits(t, 10)
				defer clearV2TokenRateLimits(t)
				setFtfOverrideRateLimits(t, 1000, true)
				defer deleteFtfOverride(t)

				// 2 x 6 = 12 > the default capacity of 10, but well inside the override's 1000.
				sendV2TokenTransfer(t, 6, ccip_common.FinalityConfig{Flags: 1}, "")
				sendV2TokenTransfer(t, 6, ccip_common.FinalityConfig{Flags: 1}, "")
			})

			t.Run("a fast-finality transfer falls back to the default bucket when the override is disabled", func(t *testing.T) {
				setV2TokenRateLimits(t, 10)
				defer clearV2TokenRateLimits(t)
				setFtfOverrideRateLimits(t, 0, false)
				defer deleteFtfOverride(t)

				// Same pair as above, but the disabled override sends both through the default
				// bucket, which only has room for the first.
				sendV2TokenTransfer(t, 6, ccip_common.FinalityConfig{Flags: 1}, "")
				sendV2TokenTransfer(t, 6, ccip_common.FinalityConfig{Flags: 1}, "rate limit reached")
			})

			t.Run("a fast-finality transfer uses the default bucket when no override exists", func(t *testing.T) {
				setV2TokenRateLimits(t, 10)
				defer clearV2TokenRateLimits(t)

				// No override PDA at all (the subtests above each delete theirs): the pool must
				// treat the uninitialized account as absent rather than failing to load it.
				sendV2TokenTransfer(t, 6, ccip_common.FinalityConfig{Flags: 1}, "")
				sendV2TokenTransfer(t, 6, ccip_common.FinalityConfig{Flags: 1}, "rate limit reached")
			})

			t.Run("pool with an enabled fee config deducts its BPS fee in-token", func(t *testing.T) {
				// Enable the pool's own token transfer fee config for the lane: the router then
				// takes the token receipt from the pool's get_fee quote (flat fee and overheads,
				// no BPS component), and the pool keeps amount * bps / 10_000 of the transferred
				// token for itself, burning only the rest — mirroring EVM's TokenPool.lockOrBurn.
				const (
					transferAmount   = uint64(10_000)
					bpsFee           = uint64(250) // transferAmount * 250 bps
					poolFlatFeeCents = uint16(10)
				)

				ixFeeCfg, err := burnmint_tokenpool.NewEditChainConfigV2Instruction(
					burnmint_tokenpool.SetChainConfigV2Params{
						Mint:                v2Token.Mint,
						RemoteChainSelector: config.EvmChainSelector,
						CcvConfig: burnmint_tokenpool.CcvConfig{
							OutboundCcvs: []solana.PublicKey{config.CommitteeVerifierProgram},
						},
						TokenTransferFeeConfig: burnmint_tokenpool.TokenTransferFeeConfig{
							DestGasOverhead:   burnmint_tokenpool.CrossChainGas{V0: 90_000},
							DestBytesOverhead: 32,
							FinalityFee:       burnmint_tokenpool.UsdCents{V0: uint32(poolFlatFeeCents)},
							FinalityBpsFee:    250,
							IsEnabled:         true,
						},
					},
					v2Token.PoolConfig,
					v2Token.ChainConfigV2[config.EvmChainSelector],
					testutils.Token0Admin.PublicKey(),
					solana.SystemProgramID,
					burnmintEventAuthority,
					config.CcipBurnMintTokenPoolProgram,
				)
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixFeeCfg}, testutils.Token0Admin, config.DefaultCommitment)

				_, beforeSupply, err := tokens.TokenSupply(ctx, env.Client, v2Token.Mint, config.DefaultCommitment)
				require.NoError(t, err)
				_, beforeUserBal, err := tokens.TokenBalance(ctx, env.Client, v2Token.User[testutils.User.PublicKey()], config.DefaultCommitment)
				require.NoError(t, err)
				_, beforePoolBal, err := tokens.TokenBalance(ctx, env.Client, v2Token.PoolTokenAccount, config.DefaultCommitment)
				require.NoError(t, err)
				_, beforeFeeVaultBal, err := tokens.TokenBalance(ctx, env.Client, v2PoolTokenFeeVaultATA, config.DefaultCommitment)
				require.NoError(t, err)
				feesBefore := feeBalances(env.WSOL.UserATA, env.WSOL.BillingATA, v2PoolVaultATA)

				message := ccip_router.Svm2AnyMessage{
					FeeToken:  env.WSOL.Mint,
					Receiver:  env.ValidEvmRawReceiverAddress[:],
					Data:      []byte{7, 8, 9},
					ExtraArgs: []byte{},
					TokenAmounts: []ccip_router.SvmTokenAmount{
						{Token: v2Token.Mint, Amount: transferAmount},
					},
				}

				resolved, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
				require.NoError(t, err)

				lookupTables := sendLookupTables(resolved)

				userTokenAccount := v2Token.User[testutils.User.PublicKey()]
				ixApprove, err := tokens.TokenApproveChecked(transferAmount, v2TokenDecimals, v2Token.Program, userTokenAccount, v2Token.Mint, config.BillingSignerPDA, testutils.User.PublicKey(), nil)
				require.NoError(t, err)

				result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{ixApprove, resolved.Ix}, testutils.User, config.DefaultCommitment, lookupTables, common.AddComputeUnitLimit(500_000))
				require.NotNil(t, result)

				tx, err := result.Transaction.GetTransaction()
				require.NoError(t, err)
				event, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
				require.NoError(t, err)

				// The token receipt comes from the pool's quote instead of the fee quoter
				// fallback, and carries only the flat fee: the BPS fee is charged in the
				// transferred token, never in the fee token.
				require.Len(t, event.Receipts, 4)
				tokenReceipt := event.Receipts[1]
				require.Equal(t, config.CcipBurnMintTokenPoolProgram, tokenReceipt.Issuer)
				require.Equal(t, usdCentsToLamports(poolFlatFeeCents), tokenReceipt.FeeTokenAmount)
				require.Equal(t, ccip_router.CrossChainGas{V0: 90_000}, tokenReceipt.DestGasLimit)
				require.Equal(t, uint32(32), tokenReceipt.DestBytesOverhead)

				// The event reports the original amount, while the encoded message carries the
				// post-fee amount that actually crosses chains.
				require.Equal(t, transferAmount, new(big.Int).SetBytes(event.TokenAmountBeforeTokenPoolFees.BeBytes[:]).Uint64())
				decoded, err := protocol.DecodeMessage(event.EncodedMessage)
				require.NoError(t, err)
				require.NotNil(t, decoded.TokenTransfer)
				require.Equal(t, transferAmount-bpsFee, decoded.TokenTransfer.Amount.Uint64())

				poolEvent, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipBurnMintTokenPoolProgram, base_token_pool.ParseEvent_LockedOrBurned)
				require.NoError(t, err)
				require.Equal(t, transferAmount-bpsFee, poolEvent.Amount)

				// The user paid the full amount; the pool burned the post-fee amount and moved the
				// BPS fee to the fee vault, leaving its own token account untouched. Keeping the
				// fee out of `pool_token_account` is what makes it withdrawable: only the vault
				// ATA is reachable by `withdraw_fee_tokens`.
				_, afterSupply, err := tokens.TokenSupply(ctx, env.Client, v2Token.Mint, config.DefaultCommitment)
				require.NoError(t, err)
				require.Equal(t, int(transferAmount-bpsFee), beforeSupply-afterSupply)
				_, afterUserBal, err := tokens.TokenBalance(ctx, env.Client, v2Token.User[testutils.User.PublicKey()], config.DefaultCommitment)
				require.NoError(t, err)
				require.Equal(t, int(transferAmount), beforeUserBal-afterUserBal)
				_, afterPoolBal, err := tokens.TokenBalance(ctx, env.Client, v2Token.PoolTokenAccount, config.DefaultCommitment)
				require.NoError(t, err)
				require.Equal(t, beforePoolBal, afterPoolBal)
				_, afterFeeVaultBal, err := tokens.TokenBalance(ctx, env.Client, v2PoolTokenFeeVaultATA, config.DefaultCommitment)
				require.NoError(t, err)
				require.Equal(t, int(bpsFee), afterFeeVaultBal-beforeFeeVaultBal)

				// And the owner can actually take it out: the same instruction that withdraws the
				// pool's fee-token receipts withdraws the in-token BPS fee, as the fee vault holds
				// it in an ATA of the pool's own mint.
				recipientATA := v2Token.User[testutils.User.PublicKey()]
				ixWithdraw, err := burnmint_tokenpool.NewWithdrawFeeTokensInstruction(
					nil, // sweep the whole vault
					v2Token.PoolConfig,
					v2Token.Mint,
					v2PoolFeeVaultPDA,
					v2PoolTokenFeeVaultATA,
					recipientATA,
					v2Token.Mint,
					v2Token.Program,
					testutils.Token0Admin.PublicKey(),
				)
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixWithdraw}, testutils.Token0Admin, config.DefaultCommitment)

				_, sweptFeeVaultBal, err := tokens.TokenBalance(ctx, env.Client, v2PoolTokenFeeVaultATA, config.DefaultCommitment)
				require.NoError(t, err)
				require.Zero(t, sweptFeeVaultBal)
				_, recipientBal, err := tokens.TokenBalance(ctx, env.Client, recipientATA, config.DefaultCommitment)
				require.NoError(t, err)
				require.Equal(t, int(bpsFee), recipientBal-afterUserBal)

				// Fee distribution: the sender is billed the receipt total in wSOL, the pool's
				// flat fee lands in the fee vault ATA qualified by this token's mint (each token
				// managed by the pool program is billed separately), and the network fee stays
				// in the onramp vault.
				feesAfter := feeBalances(env.WSOL.UserATA, env.WSOL.BillingATA, v2PoolVaultATA)
				require.Equal(t, receiptsTotal(event.Receipts), feesBefore[0]-feesAfter[0])
				require.Equal(t, usdCentsToLamports(testutils.V2LaneTokenTransferNetworkFeeUsdCents), feesAfter[1]-feesBefore[1])
				require.Equal(t, usdCentsToLamports(poolFlatFeeCents), feesAfter[2]-feesBefore[2])
			})
		})

		t.Run("ccip_send_v2 with CCTPv2 token", func(t *testing.T) {
			messageAmount := uint64(123)

			_, usdcBalance, err := tokens.TokenBalance(ctx, env.Client, env.UsdcPool.User[testutils.User.PublicKey()], config.DefaultCommitment)
			require.NoError(t, err)
			require.Greater(t, uint64(usdcBalance), messageAmount, "test user must have enough USDC to send") //nolint:gosec // safe for the test values

			receiver := [20]byte{}
			for i := range 20 {
				receiver[i] = byte(i + 1)
			}

			sendUsdcMessage := func(t *testing.T, amount uint64, message ccip_router.Svm2AnyMessage) *ccip_router.CcipMessageSentV2 {
				t.Helper()

				message.TokenAmounts = []ccip_router.SvmTokenAmount{
					{Token: env.UsdcPool.Mint, Amount: amount},
				}

				// Get Fee
				resolvedFee, ferr := ccip.ResolveGetFeeAccounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
				require.NoError(t, ferr)
				resultFee := testutils.SimulateTransactionWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolvedFee.Ix}, testutils.User.PublicKey(), sendLookupTables(resolvedFee))
				require.NotNil(t, resultFee)

				quote, ferr := common.ExtractAnchorTypedReturnValue[ccip_router.GetFeeResultV2](ctx, resultFee.Value.Logs, config.CcipRouterProgram.String())
				require.NoError(t, ferr)
				require.Equal(t, env.WSOL.Mint, quote.Token)

				// Actually send message
				resolved, nerr := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
				require.NoError(t, nerr)

				lookupTables := sendLookupTables(resolved)

				userTokenAccount := env.UsdcPool.User[testutils.User.PublicKey()]
				ixApprove, nerr := tokens.TokenApproveChecked(amount, usdcTokenDecimals, env.UsdcPool.Program, userTokenAccount, env.UsdcPool.Mint, config.BillingSignerPDA, testutils.User.PublicKey(), nil)
				require.NoError(t, nerr)

				result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{ixApprove, resolved.Ix}, testutils.User, config.DefaultCommitment, lookupTables, common.AddComputeUnitLimit(550_000))
				require.NotNil(t, result)

				for _, log := range result.Meta.LogMessages {
					t.Log(log)
				}

				tx, nerr := result.Transaction.GetTransaction()
				require.NoError(t, nerr)

				event, nerr := common.ExtractCPIEvent(tx, result.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
				require.NoError(t, nerr)

				// Check quoted fee against real receipts
				billedFee := uint64(0)
				for _, receipt := range event.Receipts {
					billedFee += receipt.FeeTokenAmount
				}
				require.Equal(t, quote.Amount, billedFee)

				return event
			}

			noncePda, _, err := state.FindCctpVerifierNoncePDA(config.EvmChainSelector, testutils.User.PublicKey(), env.UsdcPool.Mint, config.CctpTokenPoolProgram)
			require.NoError(t, err)

			t.Run("first message creates the Nonce PDA and Event PDA", func(t *testing.T) {
				require.True(t, common.IsClosedAccount(ctx, env.Client, noncePda, config.DefaultCommitment), "the Nonce PDA should not exist yet, before the first cctp message")

				event := sendUsdcMessage(t, messageAmount, ccip_router.Svm2AnyMessage{
					// pure token transfer, fees paid in SOL
					Receiver: make([]byte, 20),
					Data:     []byte{},
					ExtraArgs: testutils.MustSerializeExtraArgs(t,
						ccip_common.GenericExtraArgsV3{TokenReceiver: receiver[:]}, ccip.GenericExtraArgsV3Tag),
				})

				addr, err := state.FindDestChainStatePDAV2(config.EvmChainSelector, config.CcipRouterProgram)
				require.NoError(t, err)
				var chainConf ccip_router.DestChainCcipV2
				require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, env.Client, addr, config.DefaultCommitment, &chainConf))
				require.Len(t, chainConf.Config.DefaultCcvs, 1)
				require.Equal(t, config.CommitteeVerifierProgram, chainConf.Config.DefaultCcvs[0], "the default CCV for the lane should be the committee verifier")
				require.Empty(t, chainConf.Config.LaneMandatedCcvs, "there should be no lane mandated CCVs")

				// Check receipts
				require.Len(t, event.Receipts, 4)
				require.Equal(t, config.CctpTokenPoolProgram, event.Receipts[0].Issuer) // as the CCTP verifier (only verifier in pure transfer)
				require.Equal(t, config.CctpTokenPoolProgram, event.Receipts[1].Issuer) // as the pool
				require.Equal(t, config.ExecutorProgram, event.Receipts[2].Issuer)
				require.Equal(t, config.CcipRouterProgram, event.Receipts[3].Issuer)

				// Check nonce PDA
				var nonce cctp_token_pool.Nonce
				require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, env.Client, noncePda, config.DefaultCommitment, &nonce))
				require.Equal(t, env.UsdcPool.Mint, nonce.Mint)
				require.Equal(t, config.EvmChainSelector, nonce.RemoteChainSelector)
				require.Equal(t, testutils.User.PublicKey(), nonce.Sender)
				require.Equal(t, uint64(1), nonce.Nonce)

				// Check CCTP Event PDA
				eventPda, _, err := state.FindCctpVerifierEventPDA(config.EvmChainSelector, testutils.User.PublicKey(), env.UsdcPool.Mint, nonce.Nonce, config.CctpTokenPoolProgram)
				require.NoError(t, err)
				require.False(t, common.IsClosedAccount(ctx, env.Client, eventPda, config.DefaultCommitment), "the Event PDA should exist after the cctp message")
				cctpMsg, err := common.GetParsedAccountData[cctp.Message](ctx, env.Client, eventPda, config.DefaultCommitment, cctp.ParseMessage)

				require.NoError(t, err)
				require.Equal(t, env.CctpEvmDomain, cctpMsg.DestinationDomain)
				require.Equal(t, common.ToLeftPadded32Bytes(receiver[:]), cctpMsg.Body.MintRecipient)
				require.Len(t, cctpMsg.Body.HookData, 36,
					"hook data should just contain the VersionTag (4bytes) + CCIP MsgID (32 bytes)")
				require.Equal(t, [4]byte{0x91, 0xb3, 0x33, 0x8e}, [4]byte(cctpMsg.Body.HookData[:4]),
					"the hook data should start with the VersionTag")
				require.Equal(t, event.MessageId, [32]byte(cctpMsg.Body.HookData[4:]),
					"the hook data payload must contain the CCIP MsgID to correlate with the CCTP msg")
				require.Equal(t, [32]byte{0}, cctpMsg.Body.MaxFee, "the fee for standard transfers (no ftf) is 0")
				require.Equal(t, uint32(2000), cctpMsg.MinFinalityThreshold) // wait for finality
				require.Equal(t, [32]byte(config.EVMCctpMessageTransmitterProxy), cctpMsg.DestinationCaller)

				t.Run("fees have been accrued and can be withdrawn", func(t *testing.T) {
					t.Run("pool", func(t *testing.T) {
						// using the "testutils.User" as the recipient, just because the ATA is already created
						recipientATA := env.WSOL.UserATA

						startingRecipientBalance := env.GetBalance(recipientATA)

						poolVault, _, err := state.FindFeeVaultPDA(config.CctpTokenPoolProgram, env.UsdcPool.Mint.Bytes())
						require.NoError(t, err)

						wsolATA, err := tokens.FindFeeVaultATA(env.WSOL.Program, env.WSOL.Mint, config.CctpTokenPoolProgram, env.UsdcPool.Mint.Bytes())
						require.NoError(t, err)

						ix, err := cctp_token_pool.NewWithdrawPoolFeeTokensInstruction(
							nil,
							env.UsdcPool.PoolConfig,
							env.UsdcPool.Mint,
							poolVault,
							wsolATA,
							recipientATA,
							env.WSOL.Mint,
							env.WSOL.Program,
							testutils.UsdcPoolAdmin.PublicKey(),
						)
						require.NoError(t, err)

						testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.UsdcPoolAdmin, config.DefaultCommitment)

						finalRecipientBalance := env.GetBalance(recipientATA)
						require.Greater(t, finalRecipientBalance, startingRecipientBalance)
					})

					t.Run("verifier", func(t *testing.T) {
						verifierVault, _, err := state.FindFeeVaultPDA(config.CctpTokenPoolProgram, []byte{})
						require.NoError(t, err)
						vaultATA, err := tokens.FindFeeVaultATA(env.WSOL.Program, env.WSOL.Mint, config.CctpTokenPoolProgram, []byte{ /*the verifier has no qualifier*/ })
						require.NoError(t, err)

						startingRecipientBalance := env.GetBalance(env.WSOL.FeeAggregatorATA)

						ix, err := cctp_token_pool.NewWithdrawVerifierFeeTokensInstruction(
							config.CctpVerifierConfigPDA,
							vaultATA,
							verifierVault,
							env.WSOL.FeeAggregatorATA,
							env.WSOL.Mint,
							env.WSOL.Program,
						)
						require.NoError(t, err)

						testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.User, config.DefaultCommitment) // permissionless

						finalRecipientBalance := env.GetBalance(env.WSOL.FeeAggregatorATA)
						require.Greater(t, finalRecipientBalance, startingRecipientBalance)
					})
				})
			})

			t.Run("second message increments the Nonce PDA and creates another Event PDA", func(t *testing.T) {
				event := sendUsdcMessage(t, messageAmount, ccip_router.Svm2AnyMessage{
					// PTT
					Receiver:  receiver[:],
					Data:      make([]byte, 100), // set something not too small to test the heap management as a side-effect
					ExtraArgs: []byte{},
				})

				// Check receipts
				require.Len(t, event.Receipts, 5)
				require.Equal(t, config.CommitteeVerifierProgram, event.Receipts[0].Issuer) // it is PTT, so it includes default CCV
				require.Equal(t, config.CctpTokenPoolProgram, event.Receipts[1].Issuer)     // as the CCTP verifier
				require.Equal(t, config.CctpTokenPoolProgram, event.Receipts[2].Issuer)     // as the pool
				require.Equal(t, config.ExecutorProgram, event.Receipts[3].Issuer)
				require.Equal(t, config.CcipRouterProgram, event.Receipts[4].Issuer)

				// Check nonce PDA
				var nonce cctp_token_pool.Nonce
				require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, env.Client, noncePda, config.DefaultCommitment, &nonce))
				require.Equal(t, env.UsdcPool.Mint, nonce.Mint)
				require.Equal(t, config.EvmChainSelector, nonce.RemoteChainSelector)
				require.Equal(t, testutils.User.PublicKey(), nonce.Sender)
				require.Equal(t, uint64(2), nonce.Nonce, "the nonce should be incremented in each message")

				// Check CCTP Event PDA
				eventPda, _, err := state.FindCctpVerifierEventPDA(config.EvmChainSelector, testutils.User.PublicKey(), env.UsdcPool.Mint, nonce.Nonce, config.CctpTokenPoolProgram)
				require.NoError(t, err)
				require.False(t, common.IsClosedAccount(ctx, env.Client, eventPda, config.DefaultCommitment), "the Event PDA should exist after the cctp message")
				cctpMsg, err := common.GetParsedAccountData[cctp.Message](ctx, env.Client, eventPda, config.DefaultCommitment, cctp.ParseMessage)

				require.NoError(t, err)
				require.Equal(t, env.CctpEvmDomain, cctpMsg.DestinationDomain)
				require.Equal(t, common.ToLeftPadded32Bytes(receiver[:]), cctpMsg.Body.MintRecipient)
				require.Len(t, cctpMsg.Body.HookData, 36,
					"hook data should just contain the VersionTag (4bytes) + CCIP MsgID (32 bytes)")
				require.Equal(t, [4]byte{0x91, 0xb3, 0x33, 0x8e}, [4]byte(cctpMsg.Body.HookData[:4]),
					"the hook data should start with the VersionTag")
				require.Equal(t, event.MessageId, [32]byte(cctpMsg.Body.HookData[4:]),
					"the hook data payload must contain the CCIP MsgID to correlate with the CCTP msg")
				require.Equal(t, [32]byte{0}, cctpMsg.Body.MaxFee, "the fee for standard transfers (no ftf) is 0")
				require.Equal(t, uint32(2000), cctpMsg.MinFinalityThreshold) // wait for finality

				t.Run("attempt to reclaim event account CPIs into CCTP but fails on cooldown check", func(t *testing.T) {
					ix, err := cctp_token_pool.NewReclaimVerifierEventAccountV2Instruction(
						cctp_token_pool.ReclaimVerifierEventAccountParams{
							Mint:                env.UsdcPool.Mint,
							OriginalSender:      testutils.User.PublicKey(),
							RemoteChainSelector: config.EvmChainSelector,
							CcipCctpNonce:       2,
							Attestation:         []byte{}, // test placeholder, would fail in CCTP after cooldown check
							DestinationMessage:  []byte{}, // test placeholder, would fail in CCTP after cooldown check
						},
						config.CctpVerifierConfigPDA,
						env.UsdcPool.PoolSigner,
						eventPda,
						env.MessageTransmitterV2.MessageTransmitter,
						env.MessageTransmitterV2.Program,
						testutils.Admin.PublicKey(), // used as verifier admin too
						solana.SystemProgramID,
						env.UsdcPool.EventAuthority,
						env.UsdcPool.PoolProgram,
					)
					require.NoError(t, err)

					// this error is thrown inside the CCTP Message Transmitter contract. It means that the CPI
					// was correctly parsed, but the 5-day cooldown period (hardcoded into CCTP's contracts) has not yet elapsed
					testutils.SendAndFailWith(ctx, t, env.Client, []solana.Instruction{ix}, testutils.Admin, config.DefaultCommitment, []string{"EventAccountWindowNotExpired"})
				})
			})

			// Regression: the CCTP config's supported tokens grow the verifier's remote chain config
			// past its initial size, which used to make every later chain config update fail with
			// ConstraintSpace. Re-applying the current admin config keeps sibling subtests unaffected.
			t.Run("verifier chain config can be edited once CCTP config is stored", func(t *testing.T) {
				var before cctp_token_pool.RemoteChainConfig
				require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, env.Client, config.CctpVerifierEvmRemoteChainConfigPDA, config.DefaultCommitment, &before))
				require.NotEmpty(t, before.Cctp.SupportedTokens)

				ix, err := cctp_token_pool.NewEditVerifierChainConfigInstruction(
					cctp_token_pool.SetVerifierChainConfigParams{
						RemoteChainSelector: config.EvmChainSelector,
						ChainConfig:         before.AdminConfig,
					},
					config.CctpVerifierConfigPDA,
					testutils.Admin.PublicKey(),
					config.CctpVerifierEvmRemoteChainConfigPDA,
					config.CctpEventAuthorityPDA,
					config.CctpTokenPoolProgram,
				)
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.Admin, config.DefaultCommitment)

				var after cctp_token_pool.RemoteChainConfig
				require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, env.Client, config.CctpVerifierEvmRemoteChainConfigPDA, config.DefaultCommitment, &after))
				require.Equal(t, before, after)
			})

			t.Run("verifier allowlist can be set by the owner or the allowlist admin", func(t *testing.T) {
				setAdmin := func(t *testing.T, allowlistAdmin solana.PublicKey) {
					ix, err := cctp_token_pool.NewSetVerifierAllowlistAdminInstruction(allowlistAdmin, config.CctpVerifierConfigPDA, testutils.Admin.PublicKey(), config.CctpEventAuthorityPDA, config.CctpTokenPoolProgram)
					require.NoError(t, err)
					testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.Admin, config.DefaultCommitment)
				}
				setAllowlist := func(signer solana.PrivateKey, allowlist cctp_token_pool.RemoteChainAllowlistConfig) solana.Instruction {
					ix, err := cctp_token_pool.NewSetVerifierAllowlistInstruction(
						cctp_token_pool.SetVerifierAllowlistParams{DestChainSelector: config.EvmChainSelector, Enabled: allowlist.AllowlistEnabled, AllowedSenders: allowlist.AllowedSenders},
						config.CctpVerifierConfigPDA,
						signer.PublicKey(),
						config.CctpVerifierEvmRemoteChainConfigPDA,
						solana.SystemProgramID,
						config.CctpEventAuthorityPDA,
						config.CctpTokenPoolProgram,
					)
					require.NoError(t, err)
					return ix
				}
				readAllowlist := func(t *testing.T) cctp_token_pool.RemoteChainAllowlistConfig {
					var cfg cctp_token_pool.RemoteChainConfig
					require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, env.Client, config.CctpVerifierEvmRemoteChainConfigPDA, config.DefaultCommitment, &cfg))
					return cfg.AllowlistConfig
				}

				// Give the allowlist admin role to another key, so the owner and the admin differ.
				original := readAllowlist(t)
				setAdmin(t, testutils.User.PublicKey())
				defer func() {
					testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{setAllowlist(testutils.Admin, original)}, testutils.Admin, config.DefaultCommitment)
					setAdmin(t, testutils.Admin.PublicKey())
				}()

				byAdmin := cctp_token_pool.RemoteChainAllowlistConfig{AllowedSenders: []solana.PublicKey{solana.NewWallet().PublicKey()}}
				testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{setAllowlist(testutils.User, byAdmin)}, testutils.User, config.DefaultCommitment)
				require.Equal(t, byAdmin, readAllowlist(t))

				byOwner := cctp_token_pool.RemoteChainAllowlistConfig{AllowedSenders: []solana.PublicKey{solana.NewWallet().PublicKey()}}
				testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{setAllowlist(testutils.Admin, byOwner)}, testutils.Admin, config.DefaultCommitment)
				require.Equal(t, byOwner, readAllowlist(t))

				outsider := solana.NewWallet().PrivateKey
				require.NoError(t, testutils.FundAccounts(ctx, []solana.PrivateKey{outsider}, env.Client))
				testutils.SendAndFailWith(ctx, t, env.Client, []solana.Instruction{setAllowlist(outsider, byAdmin)}, outsider, config.DefaultCommitment, []string{"Error Code: Unauthorized"})
			})
		})

		// The following cases exercise how the final CCV list is assembled from its different
		// sources. The committee verifier is reused as the CCV (it is the only CCV program available
		// in tests), so multiple simultaneous distinct CCVs are out of scope here. The pool-required
		// source is covered by the token transfer subtests above.
		setV2LaneMandatedCCVs := func(t *testing.T, mandated []solana.PublicKey) {
			// Start from the baseline lane so the off-ramp/fee config stays intact. When setting a
			// mandated list, drop the default CCVs so the mandated list is the only CCV source
			// under test; when clearing it, the baseline's default CCV keeps the lane valid (the
			// router requires every lane to carry at least one CCV, as EVM
			// `_validateDefaultAndMandatedCCVs` does).
			cfg := testutils.DefaultV2DestChainConfig()
			cfg.LaneMandatedCcvs = mandated
			if len(mandated) > 0 {
				cfg.DefaultCcvs = nil
			}
			ix, err := ccip_router.NewUpdateDestChainConfigV2Instruction(
				ccip_router.UpdateChainSelectorV2Params{
					DestChainSelector: config.EvmChainSelector,
					DestChainConfig:   cfg,
				},

				config.EvmDestChainStatePDAV2,
				config.RouterConfigPDA,
				testutils.CcipAdmin.PublicKey(),
				solana.SystemProgramID,
				config.RouterEventAuthorityPDA,
				config.CcipRouterProgram,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.CcipAdmin, config.DefaultCommitment)
		}

		// setV2DestConfig replaces the entire V2 dest chain config for the EVM lane. Callers should
		// build overrides on top of testutils.DefaultV2DestChainConfig() (so the lane keeps a CCV,
		// off-ramp address and fee config) and defer a restore to that default.
		setV2DestConfig := func(t *testing.T, cfg ccip_router.DestChainConfigCcipV2) {
			ix, err := ccip_router.NewUpdateDestChainConfigV2Instruction(
				ccip_router.UpdateChainSelectorV2Params{
					DestChainSelector: config.EvmChainSelector,
					DestChainConfig:   cfg,
				},
				config.EvmDestChainStatePDAV2,
				config.RouterConfigPDA,
				testutils.CcipAdmin.PublicKey(),
				solana.SystemProgramID,
				config.RouterEventAuthorityPDA,
				config.CcipRouterProgram,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.CcipAdmin, config.DefaultCommitment)
		}

		// sendFailingV2Update attempts a V2 dest chain config update that is expected to revert, so
		// the lane config is left unchanged.
		sendFailingV2Update := func(t *testing.T, cfg ccip_router.DestChainConfigCcipV2, wantErr string) {
			ix, err := ccip_router.NewUpdateDestChainConfigV2Instruction(
				ccip_router.UpdateChainSelectorV2Params{
					DestChainSelector: config.EvmChainSelector,
					DestChainConfig:   cfg,
				},
				config.EvmDestChainStatePDAV2, config.RouterConfigPDA,
				testutils.CcipAdmin.PublicKey(), solana.SystemProgramID,
				config.RouterEventAuthorityPDA, config.CcipRouterProgram,
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, env.Client, []solana.Instruction{ix}, testutils.CcipAdmin, config.DefaultCommitment, []string{wantErr})
		}

		// Mirrors EVM CCVConfigValidation._validateDefaultAndMandatedCCVs: a dest chain config must
		// carry >=1 CCV, no zero-address, and no duplicates within or across the default/mandated
		// lists. Every config below is otherwise valid, so only the CCV lists are under test.
		t.Run("update_dest_chain_config_v2 rejects invalid CCV lists", func(t *testing.T) {
			dummy := solana.NewWallet().PublicKey()
			executor := solana.NewWallet().PublicKey()

			// (1) Address byte length set to 0.
			sendFailingV2Update(t, ccip_router.DestChainConfigCcipV2{}, "InvalidInputsAddressBytesLength")
			// (2) No default or lane-mandated CCVs at all.
			sendFailingV2Update(t, ccip_router.DestChainConfigCcipV2{
				AddressBytesLength: 1,
				DefaultExecutor:    executor,
			}, "MustSpecifyDefaultOrMandatedCcvs")
			// (3) The zero/default pubkey is not a valid CCV.
			sendFailingV2Update(t, ccip_router.DestChainConfigCcipV2{
				AddressBytesLength: 1,
				DefaultExecutor:    executor,
				DefaultCcvs:        []solana.PublicKey{{}},
			}, "CcvZeroAddressNotAllowed")
			// (4) Duplicate CCV across the default + lane-mandated lists.
			sendFailingV2Update(t, ccip_router.DestChainConfigCcipV2{
				AddressBytesLength: 1,
				DefaultExecutor:    executor,
				DefaultCcvs:        []solana.PublicKey{dummy},
				LaneMandatedCcvs:   []solana.PublicKey{dummy},
			}, "DuplicateCcvNotAllowed")
		})

		// The offramp is stored as raw destination-chain-sized bytes, so the router requires its
		// width to match the configured address width (executor pricing relies on that equality).
		t.Run("update_dest_chain_config_v2 rejects an offramp of the wrong width", func(t *testing.T) {
			cfg := testutils.DefaultV2DestChainConfig()
			cfg.AddressBytesLength = 32 // the default offramp is 20 bytes wide
			sendFailingV2Update(t, cfg, "InvalidInputsAddressBytesLength")

			cfg = testutils.DefaultV2DestChainConfig()
			cfg.Offramp = append([]byte{}, append(cfg.Offramp, 0x01)...) // 21 bytes
			sendFailingV2Update(t, cfg, "InvalidInputsAddressBytesLength")
		})

		// INV-LCFG-5: a lane must name a default executor, since ccip_send_v2 falls back to it
		// whenever the message's extra args don't request one.
		t.Run("update_dest_chain_config_v2 rejects a zero default executor", func(t *testing.T) {
			cfg := testutils.DefaultV2DestChainConfig()
			cfg.DefaultExecutor = solana.PublicKey{}
			sendFailingV2Update(t, cfg, "InvalidExecutor")
		})

		t.Run("ccip_send_v2 falls back to the lane default executor when none is requested", func(t *testing.T) {
			ccvAddr := config.CommitteeVerifierProgram
			// The fallback executor gets its get_fee CPI'd for real, so it must be the actual
			// executor program (the only one deployed in the test env).
			defaultExecutor := config.ExecutorProgram
			laneConfig := testutils.DefaultV2DestChainConfig()
			laneConfig.DefaultExecutor = defaultExecutor
			setV2DestConfig(t, laneConfig)
			defer setV2DestConfig(t, testutils.DefaultV2DestChainConfig()) // restore default lane

			// No executor in the extra args (zero on the wire): the executor receipt must be issued
			// by the lane's configured default executor.
			message := ccip_router.Svm2AnyMessage{
				FeeToken: env.WSOL.Mint,
				Receiver: env.ValidEvmRawReceiverAddress[:],
				Data:     []byte{1, 2, 3},
				ExtraArgs: testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV3{
					GasLimit: ccip_common.CrossChainGas{V0: 5000},
					Ccvs:     []solana.PublicKey{ccvAddr},
					CcvArgs:  [][]byte{{}},
				}, ccip.GenericExtraArgsV3Tag),
			}

			resolved, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved))
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
			require.NoError(t, err)
			// [CCV, executor, network fee]: the executor receipt is second-to-last.
			require.Len(t, event.Receipts, 3)
			require.Equal(t, defaultExecutor, event.Receipts[len(event.Receipts)-2].Issuer)
		})

		t.Run("ccip_send_v2 prefers the executor from extra args over the lane default", func(t *testing.T) {
			ccvAddr := config.CommitteeVerifierProgram
			// The winning (user-requested) executor gets its get_fee CPI'd for real, so it must be
			// the actual executor program; the losing lane default is never invoked, so a random
			// key suffices to prove the override.
			defaultExecutor := solana.NewWallet().PublicKey()
			userExecutor := config.ExecutorProgram
			laneConfig := testutils.DefaultV2DestChainConfig()
			laneConfig.DefaultExecutor = defaultExecutor
			setV2DestConfig(t, laneConfig)
			defer setV2DestConfig(t, testutils.DefaultV2DestChainConfig()) // restore default lane

			// A non-zero executor in the extra args overrides the lane default.
			message := ccip_router.Svm2AnyMessage{
				FeeToken: env.WSOL.Mint,
				Receiver: env.ValidEvmRawReceiverAddress[:],
				Data:     []byte{1, 2, 3},
				ExtraArgs: testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV3{
					GasLimit: ccip_common.CrossChainGas{V0: 5000},
					Ccvs:     []solana.PublicKey{ccvAddr},
					CcvArgs:  [][]byte{{}},
					Executor: userExecutor,
				}, ccip.GenericExtraArgsV3Tag),
			}

			resolved, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved))
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
			require.NoError(t, err)
			require.Len(t, event.Receipts, 3)
			require.Equal(t, userExecutor, event.Receipts[len(event.Receipts)-2].Issuer)
		})

		t.Run("ccip_send_v2 sources the off-ramp address from the dest chain config", func(t *testing.T) {
			ccvAddr := config.CommitteeVerifierProgram
			// An EVM-style 20-byte off-ramp address configured on the lane.
			offramp := []byte{
				0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a,
				0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14,
			}
			laneConfig := testutils.DefaultV2DestChainConfig()
			laneConfig.Offramp = offramp
			setV2DestConfig(t, laneConfig)
			defer setV2DestConfig(t, testutils.DefaultV2DestChainConfig()) // restore default lane

			message := ccip_router.Svm2AnyMessage{
				FeeToken: env.WSOL.Mint,
				Receiver: env.ValidEvmRawReceiverAddress[:],
				Data:     []byte{1, 2, 3},
				ExtraArgs: testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV3{
					GasLimit: ccip_common.CrossChainGas{V0: 5000},
					Ccvs:     []solana.PublicKey{ccvAddr},
					CcvArgs:  [][]byte{{}},
				}, ccip.GenericExtraArgsV3Tag),
			}

			resolved, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved))
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
			require.NoError(t, err)

			decoded, err := protocol.DecodeMessage(event.EncodedMessage)
			require.NoError(t, err)
			require.Equal(t, offramp, []byte(decoded.OffRampAddress))
		})

		t.Run("ccip_send_v2 sources a CCV from lane-mandated config", func(t *testing.T) {
			ccvAddr := config.CommitteeVerifierProgram
			setV2LaneMandatedCCVs(t, []solana.PublicKey{ccvAddr})
			defer setV2LaneMandatedCCVs(t, nil) // restore the lane to no mandated CCVs

			// No user-requested CCVs in the extra args: the only CCV comes from the lane's mandated
			// list.
			message := ccip_router.Svm2AnyMessage{
				FeeToken:  env.WSOL.Mint,
				Receiver:  env.ValidEvmRawReceiverAddress[:],
				Data:      []byte{1, 2, 3},
				ExtraArgs: []byte{},
			}

			resolved, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved))
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
			require.NoError(t, err)
			// [CCV, executor, network fee]: lane-mandated CCV first, router-issued network fee last.
			require.Len(t, event.Receipts, 3)
			require.Equal(t, ccvAddr, event.Receipts[0].Issuer)
			require.Equal(t, config.CcipRouterProgram, event.Receipts[2].Issuer)
			require.Len(t, event.VerifierBlobs, 1)
		})

		t.Run("ccip_send_v2 dedups a CCV requested via both extra args and lane-mandated", func(t *testing.T) {
			ccvAddr := config.CommitteeVerifierProgram
			setV2LaneMandatedCCVs(t, []solana.PublicKey{ccvAddr})
			defer setV2LaneMandatedCCVs(t, nil) // restore the lane to no mandated CCVs

			// The same CCV is both requested by the user (extra args) and mandated by the lane; the
			// merge must include it exactly once.
			message := ccip_router.Svm2AnyMessage{
				FeeToken: env.WSOL.Mint,
				Receiver: env.ValidEvmRawReceiverAddress[:],
				Data:     []byte{1, 2, 3},
				ExtraArgs: testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV3{
					GasLimit: ccip_common.CrossChainGas{V0: 5000},
					Ccvs:     []solana.PublicKey{ccvAddr},
					CcvArgs:  [][]byte{{}},
				}, ccip.GenericExtraArgsV3Tag),
			}

			resolved, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved))
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
			require.NoError(t, err)
			// Deduped to a single CCV receipt, followed by the executor and network-fee receipts.
			require.Len(t, event.Receipts, 3)
			require.Equal(t, ccvAddr, event.Receipts[0].Issuer)
			require.Equal(t, config.CcipRouterProgram, event.Receipts[2].Issuer)
		})

		t.Run("get_fee_v2 quotes a plain message and matches the send receipts", func(t *testing.T) {
			message := ccip_router.Svm2AnyMessage{
				FeeToken: env.WSOL.Mint,
				Receiver: env.ValidEvmRawReceiverAddress[:],
				Data:     []byte{4, 5, 6},
				ExtraArgs: testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV3{
					GasLimit: ccip_common.CrossChainGas{V0: 5000},
					Ccvs:     []solana.PublicKey{config.CommitteeVerifierProgram},
					CcvArgs:  [][]byte{{}},
				}, ccip.GenericExtraArgsV3Tag),
			}

			resolved, err := ccip.ResolveGetFeeAccounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved))
			require.NotNil(t, result)

			quote, err := common.ExtractAnchorTypedReturnValue[ccip_router.GetFeeResultV2](ctx, result.Meta.LogMessages, config.CcipRouterProgram.String())
			require.NoError(t, err)
			require.Equal(t, env.WSOL.Mint, quote.Token)
			// The CCV (committee verifier) and executor fees are configured to zero in the shared
			// env, so the quote is the lane's message network fee plus the execution cost, in
			// lamports.
			require.Equal(t, usdCentsToLamports(testutils.V2LaneMessageNetworkFeeUsdCents)+execGasCostLamports, quote.Amount)

			// Quoting and sending must agree: the quote equals the sum of the fee amounts in the
			// receipts an identical ccip_send_v2 emits.
			resolvedSend, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			sendResult := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolvedSend.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolvedSend))
			require.NotNil(t, sendResult)
			sendTx, err := sendResult.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(sendTx, sendResult.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
			require.NoError(t, err)
			var receiptSum uint64
			for _, r := range event.Receipts {
				receiptSum += r.FeeTokenAmount
			}
			require.Equal(t, quote.Amount, receiptSum)
		})

		// plainV2Message is the minimal 2.0 message the gas-price tests below quote. It matches
		// the plain message quoted above, so its baseline quote is the lane's network fee plus
		// the flat execution cost.
		plainV2Message := ccip_router.Svm2AnyMessage{
			FeeToken: env.WSOL.Mint,
			Receiver: env.ValidEvmRawReceiverAddress[:],
			Data:     []byte{4, 5, 6},
			ExtraArgs: testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV3{
				GasLimit: ccip_common.CrossChainGas{V0: 5000},
				Ccvs:     []solana.PublicKey{config.CommitteeVerifierProgram},
				CcvArgs:  [][]byte{{}},
			}, ccip.GenericExtraArgsV3Tag),
		}

		t.Run("get_fee_v2 prices execution from the 1.6 lane gas price", func(t *testing.T) {
			// quote_gas_for_exec_v2 reads usd_per_unit_gas from the 1.6 dest chain state, not
			// from the 2.0 one, because the 1.6 account is the only one the offchain price
			// updater writes. Nothing writes the 2.0 chain state's price, so moving the 1.6
			// price is the only thing that can move the quoted execution cost.
			quote := func() uint64 {
				resolved, err := ccip.ResolveGetFeeAccounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, plainV2Message, testutils.User.PublicKey(), logger.Test(t))
				require.NoError(t, err)
				result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved))
				value, err := common.ExtractAnchorTypedReturnValue[ccip_router.GetFeeResultV2](ctx, result.Meta.LogMessages, config.CcipRouterProgram.String())
				require.NoError(t, err)
				return value.Amount
			}

			baseline := quote()
			require.Equal(t, usdCentsToLamports(testutils.V2LaneMessageNetworkFeeUsdCents)+execGasCostLamports, baseline)

			// Restore the baseline price even if an assertion below fails, so later subtests
			// keep seeing the lane the rest of the suite is calibrated against.
			t.Cleanup(func() {
				env.SetLaneGasPrice(ctx, testutils.Admin, config.EvmChainSelector, testutils.LaneUsdPerUnitGas)
			})

			raised := new(big.Int).Mul(testutils.LaneUsdPerUnitGas, big.NewInt(100))
			env.SetLaneGasPrice(ctx, testutils.Admin, config.EvmChainSelector, raised)
			require.Greater(t, quote(), baseline)

			env.SetLaneGasPrice(ctx, testutils.Admin, config.EvmChainSelector, testutils.LaneUsdPerUnitGas)
			require.Equal(t, baseline, quote())
		})

		t.Run("get_fee_v2 ignores whether the 1.6 lane is enabled", func(t *testing.T) {
			// The 1.6 dest chain is read only as the source of the gas price, so its own
			// enablement flag is not a gate on the 2.0 lane: a 1.6 lane that has been closed to
			// traffic still quotes 2.0 messages, at the price it holds. Only the 2.0 lane's own
			// is_enabled controls 2.0 traffic.
			var legacyDestChain fee_quoter.DestChain
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, env.Client, config.FqEvmDestChainPDA, config.DefaultCommitment, &legacyDestChain))

			setLegacyLaneConfig := func(cfg fee_quoter.DestChainConfig) {
				ix, err := fee_quoter.NewUpdateDestChainConfigInstruction(
					config.EvmChainSelector, cfg, config.FqConfigPDA, config.FqEvmDestChainPDA, testutils.CcipAdmin.PublicKey(),
				)
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.CcipAdmin, config.DefaultCommitment)
			}
			t.Cleanup(func() { setLegacyLaneConfig(legacyDestChain.Config) })

			disabled := legacyDestChain.Config
			disabled.IsEnabled = false
			setLegacyLaneConfig(disabled)

			resolved, err := ccip.ResolveGetFeeAccounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, plainV2Message, testutils.User.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved))
			quote, err := common.ExtractAnchorTypedReturnValue[ccip_router.GetFeeResultV2](ctx, result.Meta.LogMessages, config.CcipRouterProgram.String())
			require.NoError(t, err)
			// Same quote as the enabled baseline: the flag was never an input to the price.
			require.Equal(t, usdCentsToLamports(testutils.V2LaneMessageNetworkFeeUsdCents)+execGasCostLamports, quote.Amount)
		})

		t.Run("get_fee_v2 defaults the fee token to wrapped SOL", func(t *testing.T) {
			message := ccip_router.Svm2AnyMessage{
				FeeToken:  solana.PublicKey{}, // native SOL, quoted as wrapped SOL
				Receiver:  env.ValidEvmRawReceiverAddress[:],
				Data:      []byte{4, 5, 6},
				ExtraArgs: []byte{},
			}

			resolved, err := ccip.ResolveGetFeeAccounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved))
			require.NotNil(t, result)

			quote, err := common.ExtractAnchorTypedReturnValue[ccip_router.GetFeeResultV2](ctx, result.Meta.LogMessages, config.CcipRouterProgram.String())
			require.NoError(t, err)
			require.Equal(t, solana.WrappedSol, quote.Token)
			require.Equal(t, usdCentsToLamports(testutils.V2LaneMessageNetworkFeeUsdCents)+execGasCostLamports, quote.Amount)
		})

		t.Run("get_fee_v2 and ccip_send_v2 reflect the committee verifier's configured fee", func(t *testing.T) {
			// non-zero verifier fee/gas/payload for the EVM lane, confirm both the router's get_fee_v2
			// quote and a matching ccip_send_v2's CCV receipt reflect it, then restore the zero-fee
			// config so sibling subtests keep their zero-CCV-fee assumptions.
			const (
				verifierFeeCents = uint16(42)
				verifierGas      = uint32(7000)
				verifierBytes    = uint32(256)
			)

			setCVConfig := func(fee committee_verifier.UsdCents, gas committee_verifier.CrossChainGas, payloadBytes uint32) {
				resolved, err := ccip.ResolveCVEditChainConfigAccounts(ctx, env.Client, committee_verifier.ProgramID,
					committee_verifier.SetChainConfigParams{
						RemoteChainSelector: config.EvmChainSelector,
						ChainConfig: committee_verifier.RemoteChainAdminConfig{
							RouterSigner:       config.CCVSignerPDA,
							Fee:                fee,
							GasForVerification: gas,
							PayloadSizeBytes:   payloadBytes,
						},
					}, testutils.Admin.PublicKey(), logger.Test(t))
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.Admin, config.DefaultCommitment)
			}

			setCVConfig(committee_verifier.UsdCents{V0: uint32(verifierFeeCents)}, committee_verifier.CrossChainGas{V0: verifierGas}, verifierBytes)
			// Restore the setupCV default (RouterSigner only, zero fee/gas/payload).
			defer setCVConfig(committee_verifier.UsdCents{}, committee_verifier.CrossChainGas{}, 0)

			message := ccip_router.Svm2AnyMessage{
				FeeToken: env.WSOL.Mint,
				Receiver: env.ValidEvmRawReceiverAddress[:],
				Data:     []byte{4, 5, 6},
				ExtraArgs: testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV3{
					GasLimit: ccip_common.CrossChainGas{V0: 5000},
					Ccvs:     []solana.PublicKey{config.CommitteeVerifierProgram},
					CcvArgs:  [][]byte{{}},
				}, ccip.GenericExtraArgsV3Tag),
			}

			// The get_fee_v2 quote includes the verifier fee on top of the network fee.
			resolved, err := ccip.ResolveGetFeeAccounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved))
			require.NotNil(t, result)
			quote, err := common.ExtractAnchorTypedReturnValue[ccip_router.GetFeeResultV2](ctx, result.Meta.LogMessages, config.CcipRouterProgram.String())
			require.NoError(t, err)
			require.Equal(t,
				usdCentsToLamports(testutils.V2LaneMessageNetworkFeeUsdCents)+usdCentsToLamports(verifierFeeCents)+execGasCostLamports,
				quote.Amount)

			// A matching ccip_send_v2's CCV receipt carries the configured fee/gas/payload. A payout
			// to a second issuer (the verifier's vault) on top of the fee CPIs puts this send just
			// above the default 200k compute units.
			feesBefore := feeBalances(env.WSOL.UserATA, env.WSOL.BillingATA, env.WSOL.CVVaultATA)
			resolvedSend, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			sendResult := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolvedSend.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolvedSend), common.AddComputeUnitLimit(300_000))
			require.NotNil(t, sendResult)
			sendTx, err := sendResult.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(sendTx, sendResult.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
			require.NoError(t, err)

			ccvReceipt := event.Receipts[0]
			require.Equal(t, config.CommitteeVerifierProgram, ccvReceipt.Issuer)
			require.Equal(t, usdCentsToLamports(verifierFeeCents), ccvReceipt.FeeTokenAmount)
			require.Equal(t, ccip_router.CrossChainGas{V0: verifierGas}, ccvReceipt.DestGasLimit)
			require.Equal(t, verifierBytes, ccvReceipt.DestBytesOverhead)

			// Quoting and sending must agree.
			require.Equal(t, quote.Amount, receiptsTotal(event.Receipts))

			// Fee distribution: the sender is billed the quoted amount, the verifier's fee lands
			// in the committee verifier's fee vault ATA, and the network fee stays in the onramp
			// fee vault.
			feesAfter := feeBalances(env.WSOL.UserATA, env.WSOL.BillingATA, env.WSOL.CVVaultATA)
			require.Equal(t, quote.Amount, feesBefore[0]-feesAfter[0])
			require.Equal(t, usdCentsToLamports(testutils.V2LaneMessageNetworkFeeUsdCents), feesAfter[1]-feesBefore[1])
			require.Equal(t, usdCentsToLamports(verifierFeeCents), feesAfter[2]-feesBefore[2])
		})

		t.Run("get_fee_v2 and ccip_send_v2 reflect the executor's configured fee", func(t *testing.T) {
			// Set a non-zero executor fee for the EVM lane, confirm both the router's get_fee_v2
			// quote and a matching ccip_send_v2's executor receipt reflect it, then restore the
			// zero-fee config so sibling subtests keep their zero-executor-fee assumptions.
			const executorFeeCents = uint16(33)

			setExecutorFee := func(fee executor.UsdCents) {
				resolved, err := ccip.ResolveExecSetChainConfigAccounts(ctx, env.Client, config.ExecutorProgram,
					executor.SetChainConfigParams{
						RemoteChainSelector: config.EvmChainSelector,
						UsdCentsFee:         fee,
						Enabled:             true,
					}, testutils.Admin.PublicKey(), logger.Test(t))
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.Admin, config.DefaultCommitment)
			}

			setExecutorFee(executor.UsdCents{V0: uint32(executorFeeCents)})
			// Restore the setupExecutor default (enabled, zero fee).
			defer setExecutorFee(executor.UsdCents{})

			message := ccip_router.Svm2AnyMessage{
				FeeToken: env.WSOL.Mint,
				Receiver: env.ValidEvmRawReceiverAddress[:],
				Data:     []byte{4, 5, 6},
				ExtraArgs: testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV3{
					GasLimit: ccip_common.CrossChainGas{V0: 5000},
					Ccvs:     []solana.PublicKey{config.CommitteeVerifierProgram},
					CcvArgs:  [][]byte{{}},
				}, ccip.GenericExtraArgsV3Tag),
			}

			// The get_fee_v2 quote includes the executor fee on top of the network fee.
			resolved, err := ccip.ResolveGetFeeAccounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved))
			require.NotNil(t, result)
			quote, err := common.ExtractAnchorTypedReturnValue[ccip_router.GetFeeResultV2](ctx, result.Meta.LogMessages, config.CcipRouterProgram.String())
			require.NoError(t, err)
			require.Equal(t,
				usdCentsToLamports(testutils.V2LaneMessageNetworkFeeUsdCents)+usdCentsToLamports(executorFeeCents)+execGasCostLamports,
				quote.Amount)

			// A matching ccip_send_v2's executor receipt carries the configured fee.
			feesBefore := feeBalances(env.WSOL.UserATA, env.WSOL.BillingATA, env.WSOL.ExecutorVaultATA)
			resolvedSend, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			sendResult := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolvedSend.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolvedSend))
			require.NotNil(t, sendResult)
			sendTx, err := sendResult.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(sendTx, sendResult.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
			require.NoError(t, err)

			executorReceipt := event.Receipts[1]
			require.Equal(t, config.ExecutorProgram, executorReceipt.Issuer)
			// The execution cost rides along with the executor's own fee on its receipt.
			require.Equal(t, usdCentsToLamports(executorFeeCents)+execGasCostLamports, executorReceipt.FeeTokenAmount)

			// Quoting and sending must agree.
			require.Equal(t, quote.Amount, receiptsTotal(event.Receipts))

			// Fee distribution: the sender is billed the quoted amount, the executor's fee lands
			// in the executor's fee vault ATA, and the network fee stays in the onramp fee vault.
			feesAfter := feeBalances(env.WSOL.UserATA, env.WSOL.BillingATA, env.WSOL.ExecutorVaultATA)
			require.Equal(t, quote.Amount, feesBefore[0]-feesAfter[0])
			require.Equal(t, usdCentsToLamports(testutils.V2LaneMessageNetworkFeeUsdCents), feesAfter[1]-feesBefore[1])
			require.Equal(t, usdCentsToLamports(executorFeeCents)+execGasCostLamports, feesAfter[2]-feesBefore[2])
		})

		t.Run("ccip_send_v2 paying in native SOL wraps the fee and distributes it", func(t *testing.T) {
			// Paying with the zero-pubkey fee token pulls native lamports from the sender's
			// wallet (no user ATA involved), wraps them into the onramp's wSOL fee vault, and
			// distributes issuer shares from there like any wSOL payment. A non-zero verifier fee
			// exercises an actual issuer payout through the wrapped path.
			const verifierFeeCents = uint16(21)

			setCVConfig := func(fee committee_verifier.UsdCents) {
				resolvedCV, cvErr := ccip.ResolveCVEditChainConfigAccounts(ctx, env.Client, committee_verifier.ProgramID,
					committee_verifier.SetChainConfigParams{
						RemoteChainSelector: config.EvmChainSelector,
						ChainConfig: committee_verifier.RemoteChainAdminConfig{
							RouterSigner: config.CCVSignerPDA,
							Fee:          fee,
						},
					}, testutils.Admin.PublicKey(), logger.Test(t))
				require.NoError(t, cvErr)
				testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{resolvedCV.Ix}, testutils.Admin, config.DefaultCommitment)
			}
			setCVConfig(committee_verifier.UsdCents{V0: uint32(verifierFeeCents)})
			// Restore the setupCV default (RouterSigner only, zero fee).
			defer setCVConfig(committee_verifier.UsdCents{})

			getLamports := func(account solana.PublicKey) uint64 {
				out, lamportsErr := env.Client.GetBalance(ctx, account, rpc.CommitmentConfirmed)
				require.NoError(t, lamportsErr)
				return out.Value
			}

			message := ccip_router.Svm2AnyMessage{
				FeeToken:  solana.PublicKey{}, // native SOL
				Receiver:  env.ValidEvmRawReceiverAddress[:],
				Data:      []byte{4, 5, 6},
				ExtraArgs: []byte{},
			}

			// Quote first: native SOL is billed as wrapped SOL.
			resolvedQuote, err := ccip.ResolveGetFeeAccounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			quoteResult := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolvedQuote.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolvedQuote))
			require.NotNil(t, quoteResult)
			quote, err := common.ExtractAnchorTypedReturnValue[ccip_router.GetFeeResultV2](ctx, quoteResult.Meta.LogMessages, config.CcipRouterProgram.String())
			require.NoError(t, err)
			require.Equal(t, solana.WrappedSol, quote.Token)
			require.Equal(t,
				usdCentsToLamports(testutils.V2LaneMessageNetworkFeeUsdCents)+usdCentsToLamports(verifierFeeCents)+execGasCostLamports,
				quote.Amount)

			lamportsBefore := getLamports(testutils.User.PublicKey())
			feesBefore := feeBalances(env.WSOL.BillingATA, env.WSOL.CVVaultATA)

			resolved, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved))
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			event, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
			require.NoError(t, err)
			// The event reports the normalized fee token.
			require.Equal(t, solana.WrappedSol, event.FeeToken)
			require.Equal(t, quote.Amount, receiptsTotal(event.Receipts))

			// The sender paid the quoted fee in native lamports (plus the transaction fee); the
			// total was wrapped into the onramp's wSOL fee vault, which keeps the network fee and
			// pays the verifier's share out to its vault ATA.
			lamportsAfter := getLamports(testutils.User.PublicKey())
			require.Equal(t, quote.Amount+result.Meta.Fee, lamportsBefore-lamportsAfter)
			feesAfter := feeBalances(env.WSOL.BillingATA, env.WSOL.CVVaultATA)
			require.Equal(t, usdCentsToLamports(testutils.V2LaneMessageNetworkFeeUsdCents), feesAfter[0]-feesBefore[0])
			require.Equal(t, usdCentsToLamports(verifierFeeCents), feesAfter[1]-feesBefore[1])
		})

		t.Run("get_fee_v2 rejects a finality the committee verifier disallows", func(t *testing.T) {
			// The shared env's verifier allows flag-only fast finality (Flags: 1) besides the
			// always-permitted wait-for-finality (see setupCV). A block-depth request is valid at
			// the router level (flags OR depth, never both), but the verifier's allowed config has
			// no block depth, so its get_fee CPI rejects the quote with InvalidFinality.
			message := ccip_router.Svm2AnyMessage{
				FeeToken: env.WSOL.Mint,
				Receiver: env.ValidEvmRawReceiverAddress[:],
				Data:     []byte{4, 5, 6},
				ExtraArgs: testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV3{
					GasLimit: ccip_common.CrossChainGas{V0: 5000},
					Finality: ccip_common.FinalityConfig{BlockDepth: 5},
					Ccvs:     []solana.PublicKey{config.CommitteeVerifierProgram},
					CcvArgs:  [][]byte{{}},
				}, ccip.GenericExtraArgsV3Tag),
			}

			resolved, err := ccip.ResolveGetFeeAccounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndFailWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved),
				[]string{"Error Code: " + "InvalidFinality"})
		})

		t.Run("get_fee_v2 includes the fallback token fee for a legacy pool transfer", func(t *testing.T) {
			message := ccip_router.Svm2AnyMessage{
				FeeToken:  env.WSOL.Mint,
				Receiver:  env.ValidEvmRawReceiverAddress[:],
				Data:      []byte{7, 8, 9},
				ExtraArgs: []byte{},
				TokenAmounts: []ccip_router.SvmTokenAmount{
					{Token: env.Token1.Mint, Amount: 1},
				},
			}

			resolved, err := ccip.ResolveGetFeeAccounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			lookupTables := sendLookupTables(resolved)

			result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, lookupTables, common.AddComputeUnitLimit(300_000))
			require.NotNil(t, result)

			quote, err := common.ExtractAnchorTypedReturnValue[ccip_router.GetFeeResultV2](ctx, result.Meta.LogMessages, config.CcipRouterProgram.String())
			require.NoError(t, err)
			require.Equal(t, env.WSOL.Mint, quote.Token)
			// Token1 is a legacy (1.6) pool and its transfer fee config is disabled, so the token
			// fee is the fee quoter fallback (the dest chain's default token fee), on top of the
			// token-transfer network fee and the execution cost. CCV and executor fees are zero.
			// Note the quote does NOT include the transferred amount itself, only the fees.
			expected := usdCentsToLamports(env.ValidFqDestChainConfig.DefaultTokenFeeUsdcents) +
				usdCentsToLamports(testutils.V2LaneTokenTransferNetworkFeeUsdCents) +
				execGasCostLamports
			require.Equal(t, expected, quote.Amount)
		})

		// The lane caps the total fee of a single message (the equivalent of EVM's
		// maxFeeJuelsPerMsg, configured per dest chain here rather than globally). The cap lives in
		// the V2 dest chain config and is compared, in fee token amounts, against the *sum* of every
		// receipt's fee — so both get_fee_v2 and ccip_send_v2 reject an over-cap message, since both
		// go through collect_receipts.
		t.Run("max fee per message", func(t *testing.T) {
			setLaneMaxFee := func(t *testing.T, cents uint16) {
				cfg := testutils.DefaultV2DestChainConfig()
				cfg.MaxFeePerMessage = ccip_router.UsdCents{V0: uint32(cents)}
				setV2DestConfig(t, cfg)
			}

			// The full quote of the plain message below, in cents: the CCV and executor fees are zero
			// in the shared env, so it is the lane's message network fee plus the execution cost.
			plainMessageFeeCents := testutils.V2LaneMessageNetworkFeeUsdCents + testutils.V2LaneExecutionGasCostUsdCents

			// A plain message quoting exactly plainMessageFeeCents (see the get_fee_v2 subtests).
			plainMessage := func(t *testing.T) ccip_router.Svm2AnyMessage {
				return ccip_router.Svm2AnyMessage{
					FeeToken: env.WSOL.Mint,
					Receiver: env.ValidEvmRawReceiverAddress[:],
					Data:     []byte{4, 5, 6},
					ExtraArgs: testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV3{
						GasLimit: ccip_common.CrossChainGas{V0: 5000},
						Ccvs:     []solana.PublicKey{config.CommitteeVerifierProgram},
						CcvArgs:  [][]byte{{}},
					}, ccip.GenericExtraArgsV3Tag),
				}
			}

			// feeExceededError is the router's rejection, surfaced from the CPI-free tail of
			// collect_receipts once every receipt has been priced.
			feeExceededError := []string{"Error Code: " + "FeeExceedsMaxAllowed"}

			t.Run("a cap below the quoted fee rejects both get_fee_v2 and ccip_send_v2", func(t *testing.T) {
				// One cent under the quoted fee: enough to exceed the cap after the conversion to
				// lamports (a cent is ~500k lamports at the test WSOL price, so truncation cannot
				// collapse the two values).
				setLaneMaxFee(t, plainMessageFeeCents-1)
				defer setV2DestConfig(t, testutils.DefaultV2DestChainConfig()) // restore default lane

				message := plainMessage(t)

				// Quoting fails: the cap is enforced while collecting receipts, which get_fee_v2
				// shares with the send path.
				resolvedQuote, err := ccip.ResolveGetFeeAccounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
				require.NoError(t, err)
				testutils.SendAndFailWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolvedQuote.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolvedQuote), feeExceededError)

				// Sending fails too, and bills nothing: the sender's fee token balance and the
				// onramp's fee vault are untouched.
				balancesBefore := feeBalances(env.WSOL.UserATA, env.WSOL.BillingATA)
				resolvedSend, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
				require.NoError(t, err)
				testutils.SendAndFailWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolvedSend.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolvedSend), feeExceededError)
				require.Equal(t, balancesBefore, feeBalances(env.WSOL.UserATA, env.WSOL.BillingATA))
			})

			t.Run("a zero cap rejects every message", func(t *testing.T) {
				// Worth pinning down because it is the value a lane gets by omission (e.g. a config
				// migrated from a schema without the field): a zero cap is not "unlimited", it
				// closes the lane to every paying message.
				setLaneMaxFee(t, 0)
				defer setV2DestConfig(t, testutils.DefaultV2DestChainConfig()) // restore default lane

				resolved, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, plainMessage(t), testutils.User.PublicKey(), logger.Test(t))
				require.NoError(t, err)
				testutils.SendAndFailWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved), feeExceededError)
			})

			t.Run("a cap exactly equal to the quoted fee is accepted", func(t *testing.T) {
				// The check is an inclusive >=, and both sides go through the same USD -> fee token
				// conversion, so a cap set to the exact fee must still let the message through.
				setLaneMaxFee(t, plainMessageFeeCents)
				defer setV2DestConfig(t, testutils.DefaultV2DestChainConfig()) // restore default lane

				resolved, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, plainMessage(t), testutils.User.PublicKey(), logger.Test(t))
				require.NoError(t, err)
				result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved))
				require.NotNil(t, result)

				tx, err := result.Transaction.GetTransaction()
				require.NoError(t, err)
				event, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
				require.NoError(t, err)
				require.Equal(t, usdCentsToLamports(testutils.V2LaneMessageNetworkFeeUsdCents)+execGasCostLamports, receiptsTotal(event.Receipts))
			})

			t.Run("the cap applies to the receipt total, not to individual receipts", func(t *testing.T) {
				// With the cap at the plain message's fee, adding a verifier fee pushes the *total*
				// over it while each individual receipt ($0.50 network, $0.10 CCV, $0.01 execution)
				// stays under.
				const verifierFeeCents = uint16(10)

				setCVConfig := func(fee committee_verifier.UsdCents) {
					resolvedCV, cvErr := ccip.ResolveCVEditChainConfigAccounts(ctx, env.Client, committee_verifier.ProgramID,
						committee_verifier.SetChainConfigParams{
							RemoteChainSelector: config.EvmChainSelector,
							ChainConfig: committee_verifier.RemoteChainAdminConfig{
								RouterSigner: config.CCVSignerPDA,
								Fee:          fee,
							},
						}, testutils.Admin.PublicKey(), logger.Test(t))
					require.NoError(t, cvErr)
					testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{resolvedCV.Ix}, testutils.Admin, config.DefaultCommitment)
				}

				setLaneMaxFee(t, plainMessageFeeCents)
				defer setV2DestConfig(t, testutils.DefaultV2DestChainConfig()) // restore default lane
				setCVConfig(committee_verifier.UsdCents{V0: uint32(verifierFeeCents)})
				// Restore the setupCV default (RouterSigner only, zero fee/gas/payload) so sibling
				// subtests keep their zero-CCV-fee assumptions.
				defer setCVConfig(committee_verifier.UsdCents{})

				resolved, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, plainMessage(t), testutils.User.PublicKey(), logger.Test(t))
				require.NoError(t, err)
				testutils.SendAndFailWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved), feeExceededError)

				// Raising the cap to cover the sum lets the same message through, confirming the
				// verifier fee (and nothing else about the lane) was what tipped it over. The extra
				// compute is for the verifier's fee payout, as in the verifier-fee subtest above.
				setLaneMaxFee(t, plainMessageFeeCents+verifierFeeCents)
				resolved, err = ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, plainMessage(t), testutils.User.PublicKey(), logger.Test(t))
				require.NoError(t, err)
				result := testutils.SendAndConfirmWithLookupTables(ctx, t, env.Client, []solana.Instruction{resolved.Ix}, testutils.User, config.DefaultCommitment, sendLookupTables(resolved), common.AddComputeUnitLimit(300_000))
				require.NotNil(t, result)

				tx, err := result.Transaction.GetTransaction()
				require.NoError(t, err)
				event, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipRouterProgram, ccip_router.ParseEvent_CcipMessageSentV2)
				require.NoError(t, err)
				require.Equal(t,
					usdCentsToLamports(testutils.V2LaneMessageNetworkFeeUsdCents)+usdCentsToLamports(verifierFeeCents)+execGasCostLamports,
					receiptsTotal(event.Receipts))
			})
		})

		t.Run("ccip_send_v2 with invalid finality is rejected", func(t *testing.T) {
			// FinalityConfig::is_valid() rejects setting both a finality flag and a block depth:
			// the requested finality may carry flags OR a block depth, never both. The router
			// validates this while parsing the V3 extra args, which happens during account
			// resolution (the construct-final-ccv-list stage parses extra args for every message),
			// so the failure surfaces from ResolveCcipSendV2Accounts rather than from the send.
			message := ccip_router.Svm2AnyMessage{
				FeeToken: env.WSOL.Mint,
				Receiver: env.ValidEvmRawReceiverAddress[:],
				Data:     []byte{7, 8, 9},
				ExtraArgs: testutils.MustSerializeExtraArgs(t, ccip_common.GenericExtraArgsV3{
					Finality: ccip_common.FinalityConfig{Flags: 1, BlockDepth: 5},
				}, ccip.GenericExtraArgsV3Tag),
			}

			_, err := ccip.ResolveCcipSendV2Accounts(ctx, env.Client, config.CcipRouterProgram, config.EvmChainSelector, message, testutils.User.PublicKey(), logger.Test(t))
			require.Error(t, err)
			// The resolver helper returns the raw simulation error (custom program error code),
			// not the program logs, so we match on the numeric code rather than the error name.
			// 7046 == CcipRouterError::InvalidFinality (see ccip_router IDL errors).
			require.ErrorContains(t, err, "7046")
		})
	})

	///////////////////////////
	// Withdraw billed funds //
	///////////////////////////

	t.Run("Withdraw billed funds", func(t *testing.T) {
		t.Run("Preconditions", func(t *testing.T) {
			require.Positive(t, env.GetBalance(env.WSOL.BillingATA))
			require.Positive(t, env.GetBalance(env.Link22.BillingATA))
		})

		t.Run("When an non-admin user tries to withdraw funds from a billing token account, it fails", func(t *testing.T) {
			ix, err := ccip_router.NewWithdrawBilledFundsInstruction(
				true, uint64(0),
				env.WSOL.Mint, env.WSOL.BillingATA, env.WSOL.FeeAggregatorATA,
				env.WSOL.Program, config.BillingSignerPDA, config.RouterConfigPDA,
				testutils.User.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, env.Client, []solana.Instruction{ix}, testutils.User, config.DefaultCommitment, []string{ccip.Unauthorized_CcipRouterError.String()})
		})

		t.Run("When withdrawing funds but sending them to the token account for a wrong token, it fails", func(t *testing.T) {
			ix, err := ccip_router.NewWithdrawBilledFundsInstruction(
				true, uint64(0),
				env.WSOL.Mint, env.WSOL.BillingATA, env.Link22.FeeAggregatorATA,
				env.WSOL.Program, config.BillingSignerPDA, config.RouterConfigPDA,
				testutils.CcipAdmin.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, env.Client, []solana.Instruction{ix}, testutils.CcipAdmin, config.DefaultCommitment, []string{ccip.InvalidInputsAtaAddress_CcipRouterError.String()})
		})

		t.Run("When withdrawing funds from a token account that does not belong to billing, it fails", func(t *testing.T) {
			ix, err := ccip_router.NewWithdrawBilledFundsInstruction(
				true, uint64(0),
				env.WSOL.Mint, env.WSOL.UserATA, env.WSOL.FeeAggregatorATA,
				env.WSOL.Program, config.BillingSignerPDA, config.RouterConfigPDA,
				testutils.CcipAdmin.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, env.Client, []solana.Instruction{ix}, testutils.CcipAdmin, config.DefaultCommitment, []string{"Error Code: " + common.ConstraintTokenOwner_AnchorError.String()})
		})

		t.Run("When withdrawing funds but sending them to a non-whitelisted token account, it fails", func(t *testing.T) {
			ix, err := ccip_router.NewWithdrawBilledFundsInstruction(
				true, uint64(0),
				env.WSOL.Mint, env.WSOL.BillingATA, env.WSOL.UserATA,
				env.WSOL.Program, config.BillingSignerPDA, config.RouterConfigPDA,
				testutils.CcipAdmin.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, env.Client, []solana.Instruction{ix}, testutils.CcipAdmin, config.DefaultCommitment, []string{ccip.InvalidInputsAtaAddress_CcipRouterError.String()})
		})

		t.Run("When trying to withdraw more funds than what's available, it fails", func(t *testing.T) {
			ix, err := ccip_router.NewWithdrawBilledFundsInstruction(
				false, env.GetBalance(env.WSOL.BillingATA)+1,
				env.WSOL.Mint, env.WSOL.BillingATA, env.WSOL.FeeAggregatorATA,
				env.WSOL.Program, config.BillingSignerPDA, config.RouterConfigPDA,
				testutils.CcipAdmin.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, env.Client, []solana.Instruction{ix}, testutils.CcipAdmin, config.DefaultCommitment, []string{ccip.InsufficientFunds_CcipRouterError.String()})
		})

		t.Run("When withdrawing a specific amount of funds, it succeeds", func(t *testing.T) {
			funds := env.GetBalance(env.Link22.BillingATA)
			require.Positive(t, funds)

			initialAggrBalance := env.GetBalance(env.Link22.FeeAggregatorATA)
			amount := uint64(2)

			ix, err := ccip_router.NewWithdrawBilledFundsInstruction(
				false, amount,
				env.Link22.Mint, env.Link22.BillingATA, env.Link22.FeeAggregatorATA,
				env.Link22.Program, config.BillingSignerPDA, config.RouterConfigPDA,
				testutils.CcipAdmin.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.CcipAdmin, config.DefaultCommitment)

			require.Equal(t, funds-amount, env.GetBalance(env.Link22.BillingATA))
			require.Equal(t, amount, env.GetBalance(env.Link22.FeeAggregatorATA)-initialAggrBalance)
		})

		t.Run("When withdrawing all funds, it succeeds", func(t *testing.T) {
			funds := env.GetBalance(env.WSOL.BillingATA)
			require.Positive(t, funds)

			initialAggrBalance := env.GetBalance(env.WSOL.FeeAggregatorATA)

			ix, err := ccip_router.NewWithdrawBilledFundsInstruction(
				true, uint64(0),
				env.WSOL.Mint, env.WSOL.BillingATA, env.WSOL.FeeAggregatorATA,
				env.WSOL.Program, config.BillingSignerPDA, config.RouterConfigPDA,
				testutils.CcipAdmin.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.CcipAdmin, config.DefaultCommitment)

			require.Equal(t, uint64(0), env.GetBalance(env.WSOL.BillingATA))
			require.Equal(t, funds, env.GetBalance(env.WSOL.FeeAggregatorATA)-initialAggrBalance)
		})

		t.Run("When withdrawing all funds but the accumulator account is already empty (no balance), it fails", func(t *testing.T) {
			funds := env.GetBalance(env.WSOL.BillingATA)
			require.Equal(t, uint64(0), funds)

			ix, err := ccip_router.NewWithdrawBilledFundsInstruction(
				true, uint64(0),
				env.WSOL.Mint, env.WSOL.BillingATA, env.WSOL.FeeAggregatorATA,
				env.WSOL.Program, config.BillingSignerPDA, config.RouterConfigPDA,
				testutils.CcipAdmin.PublicKey(),
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, env.Client, []solana.Instruction{ix}, testutils.CcipAdmin, config.DefaultCommitment, []string{ccip.InsufficientFunds_CcipRouterError.String()})
		})
	})

	// The counterpart of "Withdraw billed funds" for the *receipt issuers*: ccip_send_v2 paid each
	// of them into the ATA of their own ["fee_vault"(, qualifier)] PDA (see
	// ccip_common::seed::vault_ata), and each issuer program can only get those tokens out by
	// signing for that exact PDA. This runs against the fees the sends above really accrued, so a
	// withdrawal that derives its signer any other way cannot pass here.
	t.Run("Withdraw accrued receipt issuer fees", func(t *testing.T) {
		t.Run("Executor", func(t *testing.T) {
			accrued := env.GetBalance(env.WSOL.ExecutorVaultATA)
			require.Positive(t, accrued)
			aggregatorBefore := env.GetBalance(env.WSOL.FeeAggregatorATA)

			feeVault, _, err := state.FindFeeVaultPDA(config.ExecutorProgram, nil)
			require.NoError(t, err)

			ix, err := executor.NewWithdrawFeeTokensInstruction(
				config.ExecConfigPDA,
				env.WSOL.ExecutorVaultATA,
				feeVault,
				env.WSOL.FeeAggregatorATA,
				env.WSOL.Mint,
				env.WSOL.Program,
			)
			require.NoError(t, err)
			// Permissionless: any signer can trigger the sweep into the configured aggregator ATA.
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.User, config.DefaultCommitment)

			require.Equal(t, uint64(0), env.GetBalance(env.WSOL.ExecutorVaultATA))
			require.Equal(t, accrued, env.GetBalance(env.WSOL.FeeAggregatorATA)-aggregatorBefore)
		})

		t.Run("Committee verifier", func(t *testing.T) {
			accrued := env.GetBalance(env.WSOL.CVVaultATA)
			require.Positive(t, accrued)

			// Unlike the executor, the verifier's aggregator is not set at initialization.
			ixSetAggregator, err := committee_verifier.NewSetFeeAggregatorInstruction(
				testutils.FeeAggregator.PublicKey(),
				config.CVConfigPDA,
				testutils.Admin.PublicKey(),
				config.CVEventAuthorityPDA,
				config.CommitteeVerifierProgram,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ixSetAggregator}, testutils.Admin, config.DefaultCommitment)

			aggregatorBefore := env.GetBalance(env.WSOL.FeeAggregatorATA)

			feeVault, _, err := state.FindFeeVaultPDA(config.CommitteeVerifierProgram, nil)
			require.NoError(t, err)

			ix, err := committee_verifier.NewWithdrawFeeTokensInstruction(
				config.CVConfigPDA,
				env.WSOL.CVVaultATA,
				feeVault,
				env.WSOL.FeeAggregatorATA,
				env.WSOL.Mint,
				env.WSOL.Program,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{ix}, testutils.User, config.DefaultCommitment)

			require.Equal(t, uint64(0), env.GetBalance(env.WSOL.CVVaultATA))
			require.Equal(t, accrued, env.GetBalance(env.WSOL.FeeAggregatorATA)-aggregatorBefore)
		})

		t.Run("Token pool", func(t *testing.T) {
			// The pool's vault is qualified by the mint it serves (Token1 here), so this only
			// sweeps the fees that pool accrued for that one token.
			accrued := env.GetBalance(env.WSOL.BurnMintPoolVaultATA)
			require.Positive(t, accrued)
			recipientBefore := env.GetBalance(env.WSOL.FeeAggregatorATA)

			feeVault, _, err := state.FindFeeVaultPDA(config.CcipBurnMintTokenPoolProgram, env.Token1.Mint.Bytes())
			require.NoError(t, err)

			withdrawIx := func(authority solana.PublicKey) solana.Instruction {
				ixRaw, ixErr := burnmint_tokenpool.NewWithdrawFeeTokensInstruction(
					// Withdraw the entire balance
					nil,
					env.Token1.PoolConfig,
					env.Token1.Mint,
					feeVault,
					env.WSOL.BurnMintPoolVaultATA,
					env.WSOL.FeeAggregatorATA,
					env.WSOL.Mint,
					env.WSOL.Program,
					authority,
				)
				require.NoError(t, ixErr)
				return &tokens.TokenInstruction{Instruction: ixRaw, Program: config.CcipBurnMintTokenPoolProgram}
			}

			// Owner-gated, unlike the executor and the verifier: a pool has no configured
			// aggregator, so its owner both authorizes the sweep and names the destination.
			testutils.SendAndFailWith(ctx, t, env.Client, []solana.Instruction{withdrawIx(testutils.User.PublicKey())}, testutils.User, config.DefaultCommitment,
				[]string{"Error Code: " + "Unauthorized"})

			testutils.SendAndConfirm(ctx, t, env.Client, []solana.Instruction{withdrawIx(testutils.Token1Admin.PublicKey())}, testutils.Token1Admin, config.DefaultCommitment)

			require.Equal(t, uint64(0), env.GetBalance(env.WSOL.BurnMintPoolVaultATA))
			require.Equal(t, accrued, env.GetBalance(env.WSOL.FeeAggregatorATA)-recipientBefore)
		})
	})
}
