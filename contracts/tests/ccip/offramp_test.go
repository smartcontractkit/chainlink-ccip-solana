// offramp_test.go tests the inbound message execution flow of the Offramp program.
// This includes commit report processing, message execution, token transfers on receive,
// and end-to-end inbound CCIP message lifecycle.
package contracts

import (
	"fmt"
	"maps"
	"math/big"
	"strings"
	"testing"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/token"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/base_token_pool"
	burnmint_tokenpool "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/burnmint_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/cctp_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/committee_verifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/example_ccip_receiver_v2"
	lockrelease_tokenpool "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/lockrelease_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/lombard_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/test_ccip_receiver"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/cctp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/testutils/eth"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
)

func TestOffRamp(t *testing.T) {
	t.Parallel()

	env := testutils.NewTestEnv(t).
		WithTrafficConfig().
		WithCCTPv2().
		Build()

	ctx := env.Ctx()
	solanaGoClient := env.Client
	user := testutils.User
	ccipAdmin := testutils.CcipAdmin
	anotherUser := testutils.AnotherUser
	signers := env.Signers
	validSourceChainConfig := env.ValidSourceChainConfig
	legacyAdmin := testutils.Admin

	// remoteTokenDecimals is the decimal count every remote (EVM) chain config in this test
	// registers for its token, and therefore the value token transfer messages must advertise in
	// their extra_data. The local mints use 9 decimals, so 18->9 scaling applies on delivery.
	const remoteTokenDecimals = 18

	encodedMsg := testutils.EncodeMinimalMessageV1(config.EvmChainSelector)

	execInputs := ccip_offramp.ExecutionInputsV2{
		EncodedMessage:  encodedMsg,
		VerifierResults: [][]byte{},
		Ccvs:            []solana.PublicKey{},
	}

	getCCVsForMsg := func(t *testing.T) ccip_offramp.GetCcvsForMsgResponse {
		resolved, err := ccip.ResolveGetCCVsForMsgAccounts(ctx, solanaGoClient, config.CcipOfframpProgram, encodedMsg, user.PublicKey(), logger.Test(t))

		require.NoError(t, err)

		result := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, user, config.DefaultCommitment)
		require.NotNil(t, result)

		returnValue, err := common.ExtractAnchorTypedReturnValue[ccip_offramp.GetCcvsForMsgResponse](ctx, result.Meta.LogMessages, ccip_offramp.ProgramID.String())
		require.NoError(t, err)
		return *returnValue
	}

	updateEvmSourceChainConfig := func(t *testing.T, sourceChainConfig ccip_offramp.SourceChainConfig) {
		ix, err := ccip_offramp.NewUpdateSourceChainConfigInstruction(
			ccip_offramp.UpdateSourceChainParams{
				SourceChainSelector: config.EvmChainSelector,
				SourceChainConfig:   sourceChainConfig,
			},
			config.OfframpEvmSourceChainPDA,
			config.OfframpConfigPDA,
			ccipAdmin.PublicKey(),
			solana.SystemProgramID,
			config.OfframpEventAuthorityPDA,
			config.CcipOfframpProgram,
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, ccipAdmin, config.DefaultCommitment)
	}

	// resolveLutMap loads the lookup-table entries for the given LUT keys.
	resolveLutMap := func(t *testing.T, luts []solana.PublicKey) map[solana.PublicKey]solana.PublicKeySlice {
		lutMap := make(map[solana.PublicKey]solana.PublicKeySlice, len(luts))
		for _, lut := range luts {
			entries, gErr := common.GetAddressLookupTable(ctx, solanaGoClient, lut)
			require.NoError(t, gErr)
			require.NotNil(t, entries)
			lutMap[lut] = entries
		}
		return lutMap
	}

	t.Run("GetCCVsForMessage", func(t *testing.T) {
		// validSourceChainConfig has DefaultCcvs=[CommitteeVerifierProgram], LaneMandatedCcvs=[].
		// Receiver/pool CCV resolution is not wired yet, so the receiver path returns the
		// Pubkey::default() placeholder (EVM non-V2 receiver behaviour) and the lane defaults are
		// always enforced.

		t.Run("returns default CCVs when nothing else configured", func(t *testing.T) {
			returnValue := getCCVsForMsg(t)
			require.Equal(t, []solana.PublicKey{config.CommitteeVerifierProgram}, returnValue.RequiredCcvs)
			require.Empty(t, returnValue.OptionalCcvs)
			require.Equal(t, uint8(0), returnValue.OptionalThreshold)
		})

		t.Run("returns lane-mandated CCVs followed by defaults", func(t *testing.T) {
			laneMandatedCCV := solana.NewWallet().PublicKey()
			updated := validSourceChainConfig
			updated.LaneMandatedCcvs = []solana.PublicKey{laneMandatedCCV}
			updateEvmSourceChainConfig(t, updated)
			defer updateEvmSourceChainConfig(t, validSourceChainConfig)

			returnValue := getCCVsForMsg(t)
			// lane-mandated first, then the lane default appended via placeholder expansion.
			require.Equal(t, []solana.PublicKey{laneMandatedCCV, config.CommitteeVerifierProgram}, returnValue.RequiredCcvs)
			require.Empty(t, returnValue.OptionalCcvs)
			require.Equal(t, uint8(0), returnValue.OptionalThreshold)
		})

		t.Run("returns multiple lane-mandated CCVs in order followed by defaults", func(t *testing.T) {
			// A CCV cannot be both lane-mandated and a default (config validation rejects the
			// cross-set duplicate), so use distinct mandated CCVs and assert order is preserved and
			// the lane default is appended after them via placeholder expansion.
			laneMandatedCCV1 := solana.NewWallet().PublicKey()
			laneMandatedCCV2 := solana.NewWallet().PublicKey()
			updated := validSourceChainConfig
			updated.LaneMandatedCcvs = []solana.PublicKey{laneMandatedCCV1, laneMandatedCCV2}
			updateEvmSourceChainConfig(t, updated)
			defer updateEvmSourceChainConfig(t, validSourceChainConfig)

			returnValue := getCCVsForMsg(t)
			require.Equal(t,
				[]solana.PublicKey{laneMandatedCCV1, laneMandatedCCV2, config.CommitteeVerifierProgram},
				returnValue.RequiredCcvs,
			)
			require.Empty(t, returnValue.OptionalCcvs)
			require.Equal(t, uint8(0), returnValue.OptionalThreshold)
		})
	})

	t.Run("ExecuteV2", func(t *testing.T) {
		// No CCVs supplied, so the lane-default committee verifier is missing from the input set.
		resolved, err := ccip.ResolveExecAccounts(ctx, solanaGoClient, config.CcipOfframpProgram, execInputs, user.PublicKey(), logger.Test(t))
		require.NoError(t, err)

		result := testutils.SendAndFailWithLookupTables(
			ctx,
			t,
			solanaGoClient,
			[]solana.Instruction{resolved.Ix},
			user,
			config.DefaultCommitment,
			resolveLutMap(t, resolved.LookupTables),
			[]string{"Required CCV missing from execute input"},
		)
		require.NotNil(t, result)
	})

	t.Run("ExecuteV2 fails when required lane-mandated CCV is omitted", func(t *testing.T) {
		laneMandatedCCV := solana.NewWallet().PublicKey()
		updated := validSourceChainConfig
		updated.LaneMandatedCcvs = []solana.PublicKey{laneMandatedCCV}
		updateEvmSourceChainConfig(t, updated)
		defer updateEvmSourceChainConfig(t, validSourceChainConfig)

		execInputsWithDefaultOnly := ccip_offramp.ExecutionInputsV2{
			EncodedMessage:  encodedMsg,
			Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
			VerifierResults: [][]byte{{}},
		}

		resolved, err := ccip.ResolveExecAccounts(ctx, solanaGoClient, config.CcipOfframpProgram, execInputsWithDefaultOnly, user.PublicKey(), logger.Test(t))
		require.NoError(t, err)

		result := testutils.SendAndFailWithLookupTables(
			ctx,
			t,
			solanaGoClient,
			[]solana.Instruction{resolved.Ix},
			user,
			config.DefaultCommitment,
			resolveLutMap(t, resolved.LookupTables),
			[]string{"Required CCV missing from execute input"},
		)
		require.NotNil(t, result)
	})

	// The `source_chain` account's PDA seed is read out of the account itself, so the Anchor
	// constraint alone only proves it is *a* canonical SourceChain — not the one for the message
	// being executed. `execute_v2` closes that gap with an explicit selector check; without it a
	// caller could point execution at any other lane and have that lane's policy (is_enabled,
	// on_ramp, default and lane-mandated CCVs) applied to this message.
	t.Run("ExecuteV2 rejects a source chain account for a different lane", func(t *testing.T) {
		// Make the SVM lane an exact copy of the EVM one and give it offramp permission, so the
		// swapped account is indistinguishable from the correct one apart from its chain selector.
		// This is what makes the test a regression test rather than a restatement of the on_ramp
		// check: every other lane check passes on the swapped account.
		ixOfframpPermission, err := ccip_router.NewAddOfframpInstruction(
			config.SvmChainSelector, config.CcipOfframpProgram, config.AllowedOfframpSvmPDA,
			config.RouterConfigPDA, ccipAdmin.PublicKey(), solana.SystemProgramID,
		)
		require.NoError(t, err)
		ixSvmLane, err := ccip_offramp.NewUpdateSourceChainConfigInstruction(
			ccip_offramp.UpdateSourceChainParams{
				SourceChainSelector: config.SvmChainSelector,
				SourceChainConfig:   validSourceChainConfig,
			},
			config.OfframpSvmSourceChainPDA,
			config.OfframpConfigPDA,
			ccipAdmin.PublicKey(),
			solana.SystemProgramID,
			config.OfframpEventAuthorityPDA,
			config.CcipOfframpProgram,
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixOfframpPermission, ixSvmLane}, ccipAdmin, config.DefaultCommitment)

		resolved, err := ccip.ResolveExecAccounts(ctx, solanaGoClient, config.CcipOfframpProgram, execInputs, user.PublicKey(), logger.Test(t))
		require.NoError(t, err)

		// `allowed_offramp` is seeded from `source_chain.chain_selector`, so it has to move with the
		// source chain or Anchor rejects on seeds before the handler ever runs.
		swapForSvmLane := map[solana.PublicKey]solana.PublicKey{
			config.OfframpEvmSourceChainPDA: config.OfframpSvmSourceChainPDA,
			config.AllowedOfframpEvmPDA:     config.AllowedOfframpSvmPDA,
		}
		swapped := 0
		for _, acc := range resolved.Ix.Accounts() {
			if replacement, ok := swapForSvmLane[acc.PublicKey]; ok {
				acc.PublicKey = replacement
				swapped++
			}
		}
		require.Equal(t, len(swapForSvmLane), swapped, "expected to swap both the source chain and its allowed offramp account")

		result := testutils.SendAndFailWithLookupTables(
			ctx,
			t,
			solanaGoClient,
			[]solana.Instruction{resolved.Ix},
			user,
			config.DefaultCommitment,
			resolveLutMap(t, resolved.LookupTables),
			[]string{ccip.InvalidMessageSourceChainSelector_CcipOfframpError.String()},
		)
		require.NotNil(t, result)
	})

	sendChunkV2 := func(
		t *testing.T,
		i int,
		chunkSize int,
		numChunks int,
		rawInputs []byte,
		buffer_id [32]byte,
		bufferPDA solana.PublicKey,
		transmitter solana.PrivateKey,
	) solana.Instruction {
		start := i * chunkSize
		end := min(((i + 1) * chunkSize), len(rawInputs))

		chunk := rawInputs[start:end]

		appendIx, appendErr := ccip_offramp.NewBufferExecutionInputsInstruction(
			ccip_offramp.BufferExecutionInputsParams{
				BufferId:    buffer_id,
				TotalLength: uint32(len(rawInputs)), //nolint:gosec // G115
				Chunk:       chunk,
				ChunkIndex:  uint8(i),         //nolint:gosec // G115
				NumChunks:   uint8(numChunks), //nolint:gosec // G115

			},
			bufferPDA,
			config.OfframpConfigPDA,
			transmitter.PublicKey(),
			solana.SystemProgramID,
			config.OfframpEventAuthorityPDA,
			config.CcipOfframpProgram)

		require.NoError(t, appendErr)

		return appendIx
	}

	// bufferIDForSigner derives the buffer ID (and its PDA) for a set of execution inputs from the
	// encoded message. The contract does not tie buffer_id to the message, so any deterministic id
	// works; deriving it from the message lets the resolve and close helpers agree on the same PDA
	// without threading extra state through the tests.
	bufferIDForSigner := func(execInputs ccip_offramp.ExecutionInputsV2, signer solana.PrivateKey) ([32]byte, solana.PublicKey) {
		var bufferID [32]byte
		copy(bufferID[:], eth.Keccak256(execInputs.EncodedMessage))
		bufferPDA, _, _ := solana.FindProgramAddress([][]byte{[]byte("execution_inputs_buffer"), bufferID[:], signer.PublicKey().Bytes()}, config.CcipOfframpProgram)
		return bufferID, bufferPDA
	}

	// resolveExecAccountsWithAutoBufferingInChunks buffers the serialized execution inputs across
	// `numChunks` on-chain writes, then resolves the execute accounts against the resulting buffer.
	// Splitting into more than one chunk exercises the contract's chunk-assembly path end-to-end.
	// The buffer PDA is derived from the message alone, so a caller that reuses a message must close
	// the buffer (see closeExecInputsBuffer) before buffering it again — execute does not close it.
	resolveExecAccountsWithAutoBufferingInChunks := func(
		t *testing.T,
		execInputs ccip_offramp.ExecutionInputsV2,
		signer solana.PrivateKey,
		numChunks int,
	) ccip.ResolveAccountsResult {
		require.Positive(t, numChunks)
		execInputsData, err := execInputs.Marshal()
		require.NoError(t, err)

		bufferID, bufferPDA := bufferIDForSigner(execInputs, signer)

		// Equal-sized chunks with a possibly-smaller terminator (the only chunk the buffer contract
		// permits to differ in size). ceil so every byte lands in a chunk; the guards make the two
		// contract invariants explicit: exactly `numChunks` chunks, and a non-empty terminator.
		chunkSize := (len(execInputsData) + numChunks - 1) / numChunks
		require.Equal(t, numChunks, (len(execInputsData)+chunkSize-1)/chunkSize, "chunkSize does not yield numChunks chunks")
		require.Less(t, (numChunks-1)*chunkSize, len(execInputsData), "chunking would produce an empty terminator chunk")

		for i := range numChunks {
			appendIx := sendChunkV2(t, i, chunkSize, numChunks, execInputsData, bufferID, bufferPDA, signer)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{appendIx}, signer, config.DefaultCommitment)
		}

		resolved, err := ccip.ResolveExecAccountsWithBuffer(ctx, solanaGoClient, config.CcipOfframpProgram, signer.PublicKey(), bufferPDA, logger.Test(t))
		require.NoError(t, err)
		return resolved
	}

	resolveExecAccountsWithAutoBuffering := func(
		t *testing.T,
		execInputs ccip_offramp.ExecutionInputsV2,
		signer solana.PrivateKey,
	) ccip.ResolveAccountsResult {
		return resolveExecAccountsWithAutoBufferingInChunks(t, execInputs, signer, 1)
	}

	t.Run("resolution paginates a message with many messaging accounts", func(t *testing.T) {
		// A dest blob carrying more messaging accounts than fit in one resolution response.
		// Solana caps instruction return data at 1kB, so the resolver emits the messaging segment
		// across several rounds and the off-chain driver reassembles it. Without pagination the
		// resolution simply fails, which strands the message: it can be executed with up to 64
		// messaging accounts but could not be resolved with them.
		const extraAccounts = 20

		msg := testutils.MinimalMessageV1(config.EvmChainSelector)
		msg.SequenceNumber = 9500

		// The three real receiver accounts (as MinimalMessageV1 uses), then enough filler to
		// overflow a single response. Only resolution is exercised here, so the filler accounts
		// never have to exist on chain.
		messagingAccounts := make([]solana.PublicKey, 0, 3+extraAccounts)
		messagingAccounts = append(messagingAccounts,
			config.ReceiverExternalExecutionConfigPDA,
			config.ReceiverTargetAccountPDA,
			solana.SystemProgramID,
		)
		for range extraAccounts {
			messagingAccounts = append(messagingAccounts, solana.NewWallet().PublicKey())
		}

		executorArgs := ccip.SVMExecutorArgs{
			SvmDestBlob: ccip_common.SvmDestBlob{
				UseAta:                  ccip_common.SvmTokenReceiverUsage_DeriveAtaDontCreate,
				AccountIsWritableBitmap: 0b011,
				Accounts:                messagingAccounts,
			},
		}
		executorArgsBytes, err := executorArgs.AbiEncodePacked()
		require.NoError(t, err)
		msg.DestBlob = executorArgsBytes
		msg.DestBlobLength = uint16(len(executorArgsBytes)) //nolint:gosec // G115 bounded

		encodedMessage, err := msg.Encode()
		require.NoError(t, err)

		// No CCVs: this test is about the messaging segment, so end resolution right after it.
		execInputs := ccip_offramp.ExecutionInputsV2{
			EncodedMessage:  encodedMessage,
			Ccvs:            []solana.PublicKey{},
			VerifierResults: [][]byte{},
		}

		// A message this size fits in neither the resolution transaction nor a single buffer
		// write, so it takes the chunked-buffer path a real large message would take.
		resolved := resolveExecAccountsWithAutoBufferingInChunks(t, execInputs, user, 4)

		resolvedKeys := make([]solana.PublicKey, 0, len(resolved.AccountsToSave))
		for _, acc := range resolved.AccountsToSave {
			resolvedKeys = append(resolvedKeys, acc.PublicKey)
		}

		// Anchor the run on the first filler account (a fresh key, so it appears exactly once) and
		// assert the whole tail follows it contiguously and in order: pagination must not drop,
		// duplicate or reorder anything across rounds.
		filler := messagingAccounts[3:]
		start := -1
		for i, key := range resolvedKeys {
			if key.Equals(filler[0]) {
				start = i
				break
			}
		}
		require.NotEqual(t, -1, start, "first paginated messaging account missing from resolution")
		require.LessOrEqual(t, start+len(filler), len(resolvedKeys),
			"resolution truncated the messaging segment")
		require.Equal(t, filler, resolvedKeys[start:start+len(filler)],
			"messaging accounts must survive pagination in order")

		// The three real receiver accounts precede the filler, so the whole segment is contiguous:
		// the page boundary must be invisible in the reassembled list.
		require.GreaterOrEqual(t, start, 3)
		require.Equal(t, messagingAccounts, resolvedKeys[start-3:start-3+len(messagingAccounts)],
			"the messaging segment must be contiguous across pages")
	})

	// closeExecInputsBuffer closes the execution-inputs buffer PDA created by the auto-buffering
	// helpers. Execute never closes the buffer, so a negative case that leaves a failed execution
	// behind must close it explicitly: this frees the PDA for a later case that reuses the same
	// message and exercises the CloseExecutionInputsBuffer flow end-to-end.
	closeExecInputsBuffer := func(
		t *testing.T,
		execInputs ccip_offramp.ExecutionInputsV2,
		signer solana.PrivateKey,
	) {
		bufferID, bufferPDA := bufferIDForSigner(execInputs, signer)

		closeIx, err := ccip_offramp.NewCloseExecutionInputsBufferInstruction(
			ccip_offramp.CloseExecutionInputsBufferParams{BufferId: bufferID},
			bufferPDA,
			config.OfframpConfigPDA,
			signer.PublicKey(),
			solana.SystemProgramID,
			config.OfframpEventAuthorityPDA,
			config.CcipOfframpProgram,
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{closeIx}, signer, config.DefaultCommitment)
	}

	t.Run("ExecuteV2 with CCV verification", func(t *testing.T) {
		const threshold = 3

		privateKeys, err := eth.GenerateEthPrivateKeys(5)
		require.NoError(t, err)
		ccvSigners, err := eth.GetEvmSigners(privateKeys)
		require.NoError(t, err)
		signerAddresses := make([][20]uint8, len(ccvSigners))
		for i, s := range ccvSigners {
			signerAddresses[i] = s.Address
		}

		// The traffic config env already created the RemoteChainConfig PDA; point it at the router.
		resolvedChainConfig, err := ccip.ResolveCVEditChainConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetChainConfigParams{
			RemoteChainSelector: config.EvmChainSelector,
			ChainConfig: committee_verifier.RemoteChainAdminConfig{
				RouterSigner: config.CCVSignerPDA,
			},
		}, legacyAdmin.PublicKey(), logger.Test(t))
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolvedChainConfig.Ix}, legacyAdmin, config.DefaultCommitment)

		resolvedSigners, err := ccip.ResolveCVSetSignerConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetSignerConfigParams{
			SourceChainSelector: config.EvmChainSelector,
			Threshold:           threshold,
			Signers:             signerAddresses,
		}, legacyAdmin.PublicKey(), logger.Test(t))
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolvedSigners.Ix}, legacyAdmin, config.DefaultCommitment)

		remoteChainConfigPda, _, _ := solana.FindProgramAddress([][]byte{[]byte("remote_chain"), common.Uint64ToLE(config.EvmChainSelector)}, committee_verifier.ProgramID)

		var remoteChainCfg committee_verifier.RemoteChainConfig
		err = common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, remoteChainConfigPda, config.DefaultCommitment, &remoteChainCfg)
		require.NoError(t, err)
		require.Len(t, remoteChainCfg.SignerConfig.Signers, 5)

		ccvEncodedMsg := testutils.EncodeMinimalMessageV1(config.EvmChainSelector)
		var ccvMessageID [32]byte
		copy(ccvMessageID[:], eth.Keccak256(ccvEncodedMsg))

		verifierResults, err := testutils.SignCommitteeVerifierMessage(ccvMessageID, ccvSigners, threshold)
		require.NoError(t, err)

		ccvAddr := config.CommitteeVerifierProgram
		ccvExecInputs := ccip_offramp.ExecutionInputsV2{
			EncodedMessage:  ccvEncodedMsg,
			Ccvs:            []solana.PublicKey{ccvAddr},
			VerifierResults: [][]byte{verifierResults},
		}

		resolved, err := ccip.ResolveExecAccounts(ctx, solanaGoClient, config.CcipOfframpProgram, ccvExecInputs, user.PublicKey(), logger.Test(t))
		require.NoError(t, err)

		// The committee verifier is now part of the required set (lane default), so verification
		// actually runs here; raise the CU budget to fit the secp256k1 recoveries.
		result := testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, user, config.DefaultCommitment, resolveLutMap(t, resolved.LookupTables), common.AddComputeUnitLimit(400_000))
		require.NotNil(t, result)

		t.Run("ignores duplicate extra CCV verifier data", func(t *testing.T) {
			msg := testutils.MinimalMessageV1(config.EvmChainSelector)
			msg.SequenceNumber = 2
			encodedMsgWithDuplicateExtra, err := msg.Encode()
			require.NoError(t, err)

			var messageID [32]byte
			copy(messageID[:], eth.Keccak256(encodedMsgWithDuplicateExtra))

			validVerifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, ccvSigners, threshold)
			require.NoError(t, err)

			execInputsWithDuplicateExtra := ccip_offramp.ExecutionInputsV2{
				EncodedMessage: encodedMsgWithDuplicateExtra,
				Ccvs: []solana.PublicKey{
					ccvAddr,
					ccvAddr,
				},
				VerifierResults: [][]byte{
					validVerifierResults,
					[]byte("invalid-extra-verifier-result"),
				},
			}

			// Buffer the execution inputs: with a real receiver the message now carries the
			// receiver CPI accounts, and two CCVs (duplicate) with two verifier results would push
			// an inline execute transaction over the size limit. Buffering moves the inputs out of
			// the transaction, which is exactly the path a real executor uses for large payloads.
			resolved := resolveExecAccountsWithAutoBuffering(t, execInputsWithDuplicateExtra, user)

			result := testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, user, config.DefaultCommitment, resolveLutMap(t, resolved.LookupTables), common.AddComputeUnitLimit(400_000))
			require.NotNil(t, result)
		})

		t.Run("rejects an invalid verifier result", func(t *testing.T) {
			// The committee verifier is the lane default, so it IS part of the required set and its
			// verify CPI runs. Supplying structurally-valid signatures from keys that are NOT in the
			// configured signer set must be rejected as an unauthorized signer (proves verification
			// actually happens, not just that valid results are accepted).
			msg := testutils.MinimalMessageV1(config.EvmChainSelector)
			msg.SequenceNumber = 3
			encodedMessage, err := msg.Encode()
			require.NoError(t, err)

			var messageID [32]byte
			copy(messageID[:], eth.Keccak256(encodedMessage))

			unauthorizedPrivateKeys, err := eth.GenerateEthPrivateKeys(threshold)
			require.NoError(t, err)
			unauthorizedSigners, err := eth.GetEvmSigners(unauthorizedPrivateKeys)
			require.NoError(t, err)
			invalidVerifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, unauthorizedSigners, threshold)
			require.NoError(t, err)

			execInputsWithInvalidResult := ccip_offramp.ExecutionInputsV2{
				EncodedMessage:  encodedMessage,
				Ccvs:            []solana.PublicKey{ccvAddr},
				VerifierResults: [][]byte{invalidVerifierResults},
			}

			resolved, err := ccip.ResolveExecAccounts(ctx, solanaGoClient, config.CcipOfframpProgram, execInputsWithInvalidResult, user.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndFailWithLookupTables(
				ctx,
				t,
				solanaGoClient,
				[]solana.Instruction{resolved.Ix},
				user,
				config.DefaultCommitment,
				resolveLutMap(t, resolved.LookupTables),
				[]string{"Unauthorized signer"},
				common.AddComputeUnitLimit(400_000),
			)
			require.NotNil(t, result)
		})
	})

	// ---------------------------------------------------------------------------------------------
	// TOKEN-TRANSFER tests.
	//
	// Coverage is split into seven clearly-named cases:
	//   1. token v1 bnm                          - legacy burn/mint pool transfer (mints to receiver).
	//   2. token v1 lnr                          - legacy lock/release pool transfer (releases; supply unchanged).
	//   3. token v2 bnm (all ATA options)        - receiver-usage matrix + negative bad-input cases.
	//   4. token v2 bnm (lane default ccvs)      - get_required_ccvs VIEW: lane default + mandated append.
	//   5. token v2 bnm (ccvs required per pool) - get_required_ccvs VIEW: pool drives, mandated appended.
	//   6. token v2 lnr (lane default ccvs)      - as (4) for the lock-release pool.
	//   7. token v2 lnr (ccvs required per pool) - as (5) for the lock-release pool.
	//
	// VIEW tests (4-7) carry ALL the CCV-set assertions and never execute_v2, so a dummy lane-mandated
	// CCV that cannot actually verify is fine. TRANSFER tests (1-3) only assert the transfer succeeds
	// (plus the negative bad-input cases) and use the committee verifier as the sole required CCV (the
	// only program that can actually verify in tests).
	// ---------------------------------------------------------------------------------------------

	// ensureCommitteeVerifierLane makes sure the committee-verifier EVM-lane RemoteChainConfig exists
	// (creating it the first time) and (re)sets its signer config to the env signers at the given
	// threshold. Idempotent across the token-transfer subtests.
	ensureCommitteeVerifierLane := func(t *testing.T, threshold uint8) {
		remoteChainConfigPda, _, _ := solana.FindProgramAddress(
			[][]byte{[]byte("remote_chain"), common.Uint64ToLE(config.EvmChainSelector)},
			committee_verifier.ProgramID,
		)
		var existingRemoteChainCfg committee_verifier.RemoteChainConfig
		if getErr := common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, remoteChainConfigPda, config.DefaultCommitment, &existingRemoteChainCfg); getErr != nil {
			resolvedChainConfig, ccErr := ccip.ResolveCVInitChainConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetChainConfigParams{
				RemoteChainSelector: config.EvmChainSelector,
				ChainConfig:         committee_verifier.RemoteChainAdminConfig{RouterSigner: config.CCVSignerPDA},
			}, legacyAdmin.PublicKey(), logger.Test(t))
			require.NoError(t, ccErr)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolvedChainConfig.Ix}, legacyAdmin, config.DefaultCommitment)
		}

		signerAddresses := make([][20]uint8, len(signers))
		for i, s := range signers {
			signerAddresses[i] = s.Address
		}
		resolvedSigners, err := ccip.ResolveCVSetSignerConfigAccounts(ctx, solanaGoClient, config.CommitteeVerifierProgram, committee_verifier.SetSignerConfigParams{
			SourceChainSelector: config.EvmChainSelector,
			Threshold:           threshold,
			Signers:             signerAddresses,
		}, legacyAdmin.PublicKey(), logger.Test(t))
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolvedSigners.Ix}, legacyAdmin, config.DefaultCommitment)
	}

	// registerPoolWithRouter wires a freshly-initialized pool into the router: propose+accept the admin
	// role, set the pool's lookup table, and (when interfaceVersion > 0) mark the interface version. v1
	// legacy pools pass interfaceVersion 1 with implementsAccountResolution=false; v2 pools pass 2/true.
	registerPoolWithRouter := func(t *testing.T, pool tokens.TokenPool, interfaceVersion uint8, resolutionType ccip_router.ResolutionType) {
		ixProposeAdmin, err := ccip_router.NewCcipAdminProposeAdministratorInstruction(
			testutils.Token0Admin.PublicKey(),
			config.RouterConfigPDA,
			pool.AdminRegistryPDA,
			pool.Mint,
			testutils.CcipAdmin.PublicKey(),
			solana.SystemProgramID,
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixProposeAdmin}, testutils.CcipAdmin, config.DefaultCommitment)

		ixAcceptAdmin, err := ccip_router.NewAcceptAdminRoleTokenAdminRegistryInstruction(
			config.RouterConfigPDA,
			pool.AdminRegistryPDA,
			pool.Mint,
			testutils.Token0Admin.PublicKey(),
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixAcceptAdmin}, testutils.Token0Admin, config.DefaultCommitment)

		ixSetPool, err := ccip_router.NewSetPoolInstruction(
			pool.WritableIndexes,
			config.RouterConfigPDA,
			pool.AdminRegistryPDA,
			pool.Mint,
			pool.PoolLookupTable,
			testutils.Token0Admin.PublicKey(),
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixSetPool}, testutils.Token0Admin, config.DefaultCommitment)

		if interfaceVersion > 0 {
			ixSetInterface, err := ccip_router.NewSetPoolInterfaceImplementationInstruction(
				ccip_router.SetPoolInterfaceImplementationParams{
					Mint:              pool.Mint,
					InterfaceVersion:  interfaceVersion,
					AccountResolution: resolutionType,
				},
				config.RouterConfigPDA,
				pool.AdminRegistryPDA,
				testutils.Token0Admin.PublicKey(),
				config.RouterEventAuthorityPDA,
				config.CcipRouterProgram,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixSetInterface}, testutils.Token0Admin, config.DefaultCommitment)
		}
	}

	// buildTokenTransferMsgCore assembles a single-token-transfer EVM->SVM message carrying
	// crossChainAmount in 18-decimal cross-chain units. When messageReceiver is the zero pubkey it
	// is a pure token transfer (no receiver CPI); otherwise the message also carries a receiver,
	// and messagingAccounts/messagingBitmap describe the receiver-specific accounts the offramp
	// forwards to its `ccip_receive` CPI (after the offramp-prepended
	// [external_execution_signer, offramp, allowed_offramp]).
	buildTokenTransferMsgCore := func(t *testing.T, destMint solana.PublicKey, sourcePoolAddress protocol.ByteSlice, tokenReceiver solana.PublicKey, usage ccip_common.SvmTokenReceiverUsage, messageReceiver solana.PublicKey, messagingAccounts []solana.PublicKey, messagingBitmap uint64, crossChainAmount uint64) ([]byte, [32]byte) {
		sourceTokenAddress := protocol.ByteSlice{0x04, 0x05}
		destTokenAddress := protocol.ByteSlice(destMint.Bytes())
		// The source pool advertises its token decimals as a 32-byte big-endian value in extra_data.
		// v2 pools reject a transfer whose extra_data disagrees with their remote chain config, which
		// every InitChainRemoteConfig below registers as remoteTokenDecimals.
		sourcePoolDecimals := tokens.ToBigEndianU256(remoteTokenDecimals)
		extraData := protocol.ByteSlice(sourcePoolDecimals[:])

		amount := ccip_offramp.ProtocolAmount{BeBytes: tokens.ToBigEndianU256(crossChainAmount)}
		tokenTransfer := protocol.TokenTransfer{
			Amount:                   new(big.Int).SetBytes(amount.BeBytes[:]),
			SourcePoolAddress:        sourcePoolAddress,
			SourceTokenAddress:       sourceTokenAddress,
			DestTokenAddress:         destTokenAddress,
			TokenReceiver:            tokenReceiver.Bytes(),
			ExtraData:                extraData,
			Version:                  1,
			SourcePoolAddressLength:  uint8(len(sourcePoolAddress)),  //nolint:gosec // G115 always < 256
			SourceTokenAddressLength: uint8(len(sourceTokenAddress)), //nolint:gosec // G115 always < 256
			DestTokenAddressLength:   uint8(len(destTokenAddress)),   //nolint:gosec // G115 always < 256
			TokenReceiverLength:      32,
			ExtraDataLength:          uint16(len(extraData)), //nolint:gosec // G115 always < 256
		}
		tokenTransferLength := uint16(len(sourcePoolAddress)) + uint16(len(sourceTokenAddress)) + uint16(len(destTokenAddress)) + 32 + uint16(len(extraData)) //nolint:gosec // G115

		destBlob := ccip.SVMExecutorArgs{SvmDestBlob: ccip_common.SvmDestBlob{
			UseAta:                  usage,
			AccountIsWritableBitmap: messagingBitmap,
			Accounts:                messagingAccounts,
		}}
		destBlobBytes, err := destBlob.BorshEncode()
		require.NoError(t, err)

		msg := testutils.MinimalMessageV1(config.EvmChainSelector)
		msg.Receiver = messageReceiver.Bytes()
		msg.TokenTransfer = &tokenTransfer
		msg.TokenTransferLength = tokenTransferLength
		msg.DestBlob = destBlobBytes
		msg.DestBlobLength = uint16(len(destBlobBytes)) //nolint:gosec // G115

		encodedMessage, err := msg.Encode()
		require.NoError(t, err)
		var messageID [32]byte
		copy(messageID[:], eth.Keccak256(encodedMessage))
		return encodedMessage, messageID
	}

	// buildTokenTransferMsg assembles a pure single-token-transfer EVM->SVM message (no receiver
	// CPI). The amount is fixed at 1e9 cross-chain units which, with 18->9 decimal scaling, yields 1
	// dest base unit. Returns the encoded message and its keccak message id.
	buildTokenTransferMsg := func(t *testing.T, destMint solana.PublicKey, sourcePoolAddress protocol.ByteSlice, tokenReceiver solana.PublicKey, usage ccip_common.SvmTokenReceiverUsage) ([]byte, [32]byte) {
		return buildTokenTransferMsgCore(t, destMint, sourcePoolAddress, tokenReceiver, usage, solana.PublicKey{}, nil, 0, 1000000000)
	}

	// buildTokenTransferMsgOfSize is buildTokenTransferMsg with a caller-chosen dest amount. The
	// 18->9 decimal scaling means one dest base unit costs 1e9 cross-chain units. Rate limit tests
	// need this: the pool's buckets are denominated in dest base units.
	buildTokenTransferMsgOfSize := func(t *testing.T, destMint solana.PublicKey, sourcePoolAddress protocol.ByteSlice, tokenReceiver solana.PublicKey, usage ccip_common.SvmTokenReceiverUsage, destAmount uint64) ([]byte, [32]byte) {
		return buildTokenTransferMsgCore(t, destMint, sourcePoolAddress, tokenReceiver, usage, solana.PublicKey{}, nil, 0, destAmount*1000000000)
	}

	// receiverCPIMessagingAccounts are the test_ccip_receiver `ccip_receive` accounts that follow the
	// offramp-prepended [external_execution_signer, offramp, allowed_offramp]: its
	// external_execution_config and counter (both writable, bits 0 and 1) plus the system program.
	receiverCPIMessagingAccounts := []solana.PublicKey{
		config.ReceiverExternalExecutionConfigPDA,
		config.ReceiverTargetAccountPDA,
		solana.SystemProgramID,
	}
	const receiverCPIMessagingBitmap uint64 = 0b011

	// getRequiredCcvsView calls the get_required_ccvs view (never executes) and returns its decoded
	// response. VIEW tests use this to assert the resolved RequiredCcvs set.
	getRequiredCcvsView := func(t *testing.T, encodedMessage []byte) ccip_offramp.GetCcvsForMsgResponse {
		viewResolved, err := ccip.ResolveGetCCVsForMsgAccounts(ctx, solanaGoClient, config.CcipOfframpProgram, encodedMessage, user.PublicKey(), logger.Test(t))
		require.NoError(t, err)
		viewResult := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{viewResolved.Ix}, user, config.DefaultCommitment)
		require.NotNil(t, viewResult)
		viewReturn, err := common.ExtractAnchorTypedReturnValue[ccip_offramp.GetCcvsForMsgResponse](ctx, viewResult.Meta.LogMessages, ccip_offramp.ProgramID.String())
		require.NoError(t, err)
		return *viewReturn
	}

	// The token stages are only reached through resolution for messages carrying a token, but
	// each one is independently callable, so a caller can hand any of them arbitrary inputs.
	// They must reject malformed ones with an error rather than panic.
	t.Run("token resolution stages reject malformed inputs", func(t *testing.T) {
		resolveParams := func(t *testing.T, encodedMessage []byte, metadata []byte) ccip_offramp.ResolveAccountsParams {
			execParams, err := bin.MarshalBorsh(ccip_offramp.ExecuteParams{
				ExecInputs: &ccip_offramp.ExecutionInputsV2{
					EncodedMessage:  encodedMessage,
					VerifierResults: [][]byte{},
					Ccvs:            []solana.PublicKey{},
				},
				ResolutionMetadata: []byte{},
			})
			require.NoError(t, err)
			return ccip_offramp.ResolveAccountsParams{
				Caller:   user.PublicKey(),
				IxData:   append(append([]byte{}, ccip_offramp.Instruction_ExecuteV2[:]...), execParams...),
				Metadata: metadata,
			}
		}
		// Borsh encoding of an ExecuteTokenStaticStageMetadata: five zero list lengths, an empty
		// ccvs vector, then the lookup table cursor.
		staticStageMetadata := func(cursor uint8) []byte {
			return []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, cursor}
		}
		staticIx := func(t *testing.T, encodedMessage []byte, cursor uint8, adminRegistry, tokenLut solana.PublicKey) solana.Instruction {
			ix, err := ccip_offramp.NewResolveAccountsExecuteTokenTransferStaticAccountsStageInstruction(
				resolveParams(t, encodedMessage, staticStageMetadata(cursor)),
				config.OfframpConfigPDA, config.OfframpReferenceAddressesPDA, adminRegistry, tokenLut,
			)
			require.NoError(t, err)
			return ix
		}
		unused := solana.NewWallet().PublicKey()
		token0Msg, _ := buildTokenTransferMsg(t, env.Token0.Mint, protocol.ByteSlice{0x01, 0x02}, user.PublicKey(), ccip_common.SvmTokenReceiverUsage_DeriveAtaDontCreate)
		instructionDataError := []string{"Error Code: " + ccip.InvalidInstructionData_CcipOfframpError.String()}
		lutError := []string{"Error Code: " + ccip.InvalidInputsLookupTableAccounts_CcipCommonError.String()}

		t.Run("without a token transfer", func(t *testing.T) {
			lutIx, err := ccip_offramp.NewResolveAccountsExecuteTokenTransferRetrieveLutStageInstruction(
				resolveParams(t, encodedMsg, []byte{}), config.OfframpConfigPDA, unused,
			)
			require.NoError(t, err)
			// The lookup table must be a genuine one to get past the account constraints.
			static := staticIx(t, encodedMsg, 0, unused, env.Token0.PoolLookupTable)

			for name, ix := range map[string]solana.Instruction{"retrieve lut": lutIx, "static accounts": static} {
				t.Run(name, func(t *testing.T) {
					testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment, instructionDataError)
				})
			}
		})

		// Tables the lookup table program owns, but that were never registered for any token: one
		// too short to hold the required entries, and one full-sized and naming Token0's mint at
		// the offset the stage reads it from.
		shortLut, err := common.SetupLookupTable(ctx, solanaGoClient, legacyAdmin, []solana.PublicKey{env.Token0.Mint, env.Token0.Mint, env.Token0.Mint})
		require.NoError(t, err)
		impostorEntries := make([]solana.PublicKey, 10)
		for i := range impostorEntries {
			impostorEntries[i] = env.Token0.Mint
		}
		impostorLut, err := common.SetupLookupTable(ctx, solanaGoClient, legacyAdmin, impostorEntries)
		require.NoError(t, err)

		// The nested stage takes `source_chain` without a seeds constraint and reads the lane's
		// legacy address length off it, so only the selector check ties it to the message. Hand it
		// the SVM lane for an EVM message: it must be rejected before the pool is ever invoked.
		t.Run("nested token stage with a source chain account for a different lane", func(t *testing.T) {
			// Borsh encoding of an ExecuteNestedTokenStageMetadata: five zero list lengths, an empty
			// ccvs vector and empty inner resolution inputs (i.e. the first round).
			nestedStageMetadata := make([]byte, 5+4+4)
			ix, err := ccip_offramp.NewResolveAccountsExecuteNestedTokenStageInstruction(
				resolveParams(t, token0Msg, nestedStageMetadata),
				config.OfframpConfigPDA,
				config.OfframpSvmSourceChainPDA,
				env.Token0.PoolProgram,
				unused,
			)
			require.NoError(t, err)
			testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment,
				[]string{"Error Code: " + ccip.InvalidMessageSourceChainSelector_CcipOfframpError.String()})
		})

		for _, tc := range []struct {
			name     string
			tokenLut solana.PublicKey
			cursor   uint8
		}{
			// Not owned by the lookup table program, so its contents mean nothing.
			{name: "with an account that is not a lookup table", tokenLut: unused},
			{name: "with a lookup table missing the required entries", tokenLut: shortLut},
			{name: "with a lookup table not registered for the token", tokenLut: impostorLut},
			// The cursor is caller-supplied, and this one points past the end of the table.
			{name: "with a cursor past the end of the lookup table", tokenLut: env.Token0.PoolLookupTable, cursor: 200},
		} {
			t.Run("static accounts "+tc.name, func(t *testing.T) {
				ix := staticIx(t, token0Msg, tc.cursor, env.Token0.AdminRegistryPDA, tc.tokenLut)
				testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment, lutError)
			})
		}
	})

	t.Run("ExecuteV2 with buffering (message-only) succeeds", func(t *testing.T) {
		// Asserts that buffered inputs drive a full SUCCESSFUL execution of a message-only payload (no
		// token transfer): buffer -> resolve -> execute, with the lane-default committee verifier
		// actually verifying. The multi-chunk variant below covers the chunk-assembly path.
		ensureCommitteeVerifierLane(t, 1)

		// Unique sequence number so this message has not been executed before: execute short-circuits
		// (returns Ok without running) on an already-successful message_id, which would make the
		// assertion vacuous.
		msg := testutils.MinimalMessageV1(config.EvmChainSelector)
		msg.SequenceNumber = 9001
		encodedMessage, err := msg.Encode()
		require.NoError(t, err)

		var messageID [32]byte
		copy(messageID[:], eth.Keccak256(encodedMessage))
		verifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
		require.NoError(t, err)

		execInputs := ccip_offramp.ExecutionInputsV2{
			EncodedMessage:  encodedMessage,
			Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
			VerifierResults: [][]byte{verifierResults},
		}

		resolved := resolveExecAccountsWithAutoBuffering(t, execInputs, user)

		// balance after having paid for buffer but before exec'ing
		balanceBeforeExec, err := solanaGoClient.GetBalance(ctx, user.PublicKey(), config.DefaultCommitment)
		require.NoError(t, err)

		result := testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, user, config.DefaultCommitment, resolveLutMap(t, resolved.LookupTables), common.AddComputeUnitLimit(400_000))
		require.NotNil(t, result)

		tx, err := result.Transaction.GetTransaction()
		require.NoError(t, err)
		event, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipOfframpProgram, ccip_offramp.ParseEvent_BufferClosed)
		require.NoError(t, err)
		require.Equal(t, user.PublicKey(), event.Authority)
		require.Equal(t, messageID, event.BufferId)

		balanceAfter, err := solanaGoClient.GetBalance(ctx, user.PublicKey(), config.DefaultCommitment)
		require.NoError(t, err)

		require.Greater(t, balanceAfter.Value, balanceBeforeExec.Value, "user should have recovered buffer rent")
	})

	t.Run("ExecuteV2 rejects a faster-than-finality message addressed to a v1 receiver", func(t *testing.T) {
		// INV-FIN-DST-2 / INV-FIN-OPTIN-2: a receiver that does not implement the v2 CCV/finality
		// interface is treated as allowing only WAIT_FOR_FINALITY, so an FTF request must be
		// rejected — a v1 receiver has no way to opt in. `MinimalMessageV1` targets the v1
		// `test_ccip_receiver`, which is the case this covers. Mirrors EVM's
		// `_getCCVsFromReceiver`, which runs the finality check even when the ERC165 probe fails.
		ensureCommitteeVerifierLane(t, 1)

		msg := testutils.MinimalMessageV1(config.EvmChainSelector)
		msg.SequenceNumber = 9042
		msg.Finality = 1 // block_depth == 1: faster-than-finality
		encodedMessage, err := msg.Encode()
		require.NoError(t, err)

		var messageID [32]byte
		copy(messageID[:], eth.Keccak256(encodedMessage))
		verifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
		require.NoError(t, err)

		execInputs := ccip_offramp.ExecutionInputsV2{
			EncodedMessage:  encodedMessage,
			Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
			VerifierResults: [][]byte{verifierResults},
		}

		resolved := resolveExecAccountsWithAutoBuffering(t, execInputs, user)
		testutils.SendAndFailWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, user,
			config.DefaultCommitment, resolveLutMap(t, resolved.LookupTables),
			[]string{ccip.RequestedFinalityNotAllowed_CcipOfframpError.String()}, common.AddComputeUnitLimit(400_000))

		// The non-authoritative view shares `resolve_receiver_required_ccvs`, so it must predict the
		// same rejection rather than reporting a satisfiable CCV set for a message that cannot execute.
		viewResolved, err := ccip.ResolveGetCCVsForMsgAccounts(ctx, solanaGoClient, config.CcipOfframpProgram, encodedMessage, user.PublicKey(), logger.Test(t))
		require.NoError(t, err)
		testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{viewResolved.Ix}, user,
			config.DefaultCommitment, []string{ccip.RequestedFinalityNotAllowed_CcipOfframpError.String()})
	})

	t.Run("ExecuteV2 with buffering fails when execution is attempted by a different transmitter", func(t *testing.T) {
		// Asserts that buffered inputs drive a full SUCCESSFUL execution of a message-only payload (no
		// token transfer): buffer -> resolve -> execute, with the lane-default committee verifier
		// actually verifying. The multi-chunk variant below covers the chunk-assembly path.
		ensureCommitteeVerifierLane(t, 1)

		// Unique sequence number so this message has not been executed before: execute short-circuits
		// (returns Ok without running) on an already-successful message_id, which would make the
		// assertion vacuous.
		msg := testutils.MinimalMessageV1(config.EvmChainSelector)
		msg.SequenceNumber = 9001
		encodedMessage, err := msg.Encode()
		require.NoError(t, err)

		var messageID [32]byte
		copy(messageID[:], eth.Keccak256(encodedMessage))
		verifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
		require.NoError(t, err)

		execInputs := ccip_offramp.ExecutionInputsV2{
			EncodedMessage:  encodedMessage,
			Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
			VerifierResults: [][]byte{verifierResults},
		}

		resolved := resolveExecAccountsWithAutoBuffering(t, execInputs, user) // this sets the user as the authority
		// replace the authority with a different signer trying to poach the buffer
		for i, acc := range resolved.Ix.Accounts() {
			if acc.PublicKey.Equals(user.PublicKey()) && acc.IsSigner {
				resolved.Ix.Accounts()[i] = &solana.AccountMeta{
					PublicKey:  anotherUser.PublicKey(),
					IsSigner:   true,
					IsWritable: acc.IsWritable,
				}
			}
		}

		testutils.SendAndFailWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, anotherUser, config.DefaultCommitment, resolveLutMap(t, resolved.LookupTables), []string{ccip.WrongBufferAuthority_CcipOfframpError.String()}, common.AddComputeUnitLimit(400_000))
	})

	t.Run("ExecuteV2 with multi-chunk buffering succeeds", func(t *testing.T) {
		// The auto-buffering helper otherwise sends a single chunk; forcing several chunks exercises
		// the on-chain chunk-assembly path (ordering, offsets, smaller terminator) end-to-end, which
		// is otherwise only covered by the buffering.rs unit tests.
		ensureCommitteeVerifierLane(t, 1)

		msg := testutils.MinimalMessageV1(config.EvmChainSelector)
		msg.SequenceNumber = 9002
		encodedMessage, err := msg.Encode()
		require.NoError(t, err)

		var messageID [32]byte
		copy(messageID[:], eth.Keccak256(encodedMessage))
		verifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
		require.NoError(t, err)

		execInputs := ccip_offramp.ExecutionInputsV2{
			EncodedMessage:  encodedMessage,
			Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
			VerifierResults: [][]byte{verifierResults},
		}

		const numChunks = 3
		resolved := resolveExecAccountsWithAutoBufferingInChunks(t, execInputs, user, numChunks)

		result := testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, user, config.DefaultCommitment, resolveLutMap(t, resolved.LookupTables), common.AddComputeUnitLimit(400_000))
		require.NotNil(t, result)
	})

	t.Run("ExecuteV2 with a large payload succeeds on the custom heap", func(t *testing.T) {
		// The heap-exhaustion regression test. A buffered execution of a multi-KB arbitrary payload
		// keeps several copies of it live at once on the program heap: the buffered
		// `ExecutionInputsV2.encoded_message`, the parsed `MessageV1.data`, the per-CCV
		// `encoded_message.clone()` in the verify loop, and the `ccip_receive` instruction data built
		// for the receiver CPI. Against Anchor's default bump allocator — fixed at 32KB and never
		// freeing within an instruction — that overflows and the program aborts with
		// `memory allocation failed, out of memory`.
		//
		// Two things make this pass: the offramp installs `probing-bump-allocator` as its
		// `#[global_allocator]`, which is configured for the 256KB maximum and so can use whatever
		// frame the transaction requests (it never reclaims, so the gain is capacity alone), and the
		// transaction requests the 256KB frame with AddHeapFrame — the same size
		// `pkg/transmitter/buffer.go` requests for real buffered executions. Keep the two in sync.
		ensureCommitteeVerifierLane(t, 1)

		// 4KB: well past the ~1KB point at which the default 32KB heap gives out once the copies
		// above are counted, and still inside the 1.4M compute-unit ceiling. Compute cost grows with
		// the payload (roughly 160 CU/byte across the whole execution, not attributed to any single
		// step such as the receiver's deserialization), which is what caps this test's payload
		// rather than the heap.
		const payloadBytes = 4 * 1024

		msg := testutils.MinimalMessageV1(config.EvmChainSelector)
		msg.SequenceNumber = 9003
		msg.Data = make([]byte, payloadBytes)
		for i := range msg.Data {
			msg.Data[i] = byte(i % 251) // non-trivial, deterministic; 251 is prime so it does not align to word boundaries
		}
		msg.DataLength = payloadBytes // Encode() rejects a DataLength that disagrees with len(Data)
		encodedMessage, err := msg.Encode()
		require.NoError(t, err)

		var messageID [32]byte
		copy(messageID[:], eth.Keccak256(encodedMessage))
		verifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
		require.NoError(t, err)

		execInputs := ccip_offramp.ExecutionInputsV2{
			EncodedMessage:  encodedMessage,
			Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
			VerifierResults: [][]byte{verifierResults},
		}

		var counterBefore test_ccip_receiver.Counter
		require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.ReceiverTargetAccountPDA, config.DefaultCommitment, &counterBefore))

		// The payload does not fit in a single buffering transaction, so spread it over enough chunks
		// to stay under the transaction size limit.
		const numChunks = 10
		resolved := resolveExecAccountsWithAutoBufferingInChunks(t, execInputs, user, numChunks)

		result := testutils.SendAndConfirmWithLookupTables(
			ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, user, config.DefaultCommitment,
			resolveLutMap(t, resolved.LookupTables),
			common.AddComputeUnitLimit(1_400_000), common.AddHeapFrame(256*1024),
		)
		require.NotNil(t, result)

		// The receiver's counter is only committed if the whole instruction succeeded, so together
		// with the confirmed transaction a bumped counter proves the large payload was executed
		// end to end, not just buffered.
		var counterAfter test_ccip_receiver.Counter
		require.NoError(t, common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.ReceiverTargetAccountPDA, config.DefaultCommitment, &counterAfter))
		require.Equal(t, counterBefore.Value+1, counterAfter.Value, "receiver ccip_receive must have run with the large payload")
	})

	t.Run("ExecuteV2 during an OnRamp upgrade window", func(t *testing.T) {
		// The state the lane sits in while the source chain upgrades its OnRamp: [old, new] configured
		// at once, with the old OnRamp still draining its inflight messages. Both must execute, which
		// is the whole point of allowing several OnRamps per source chain.
		ensureCommitteeVerifierLane(t, 1)

		// env.OnRampAddress is the OnRamp MinimalMessageV1 stamps into its messages and the only one
		// the lane is configured with; the "upgraded" OnRamp is a second, distinct address.
		oldOnRamp := env.OnRampAddress[:32]
		var upgradedOnRamp [64]byte
		for i := range 32 {
			upgradedOnRamp[i] = 0x5a
		}

		upgradeWindowConfig := validSourceChainConfig
		upgradeWindowConfig.OnRamps = []ccip_offramp.OnRampAddress{
			{Bytes: env.OnRampAddress, Len: 32},
			{Bytes: upgradedOnRamp, Len: 32},
		}
		updateEvmSourceChainConfig(t, upgradeWindowConfig)
		defer updateEvmSourceChainConfig(t, validSourceChainConfig)

		// Each case needs a sequence number of its own: execute short-circuits on a message id that
		// already succeeded, which would make the assertion vacuous.
		execInputsFrom := func(t *testing.T, onRamp []byte, sequenceNumber protocol.SequenceNumber) ccip_offramp.ExecutionInputsV2 {
			msg := testutils.MinimalMessageV1(config.EvmChainSelector)
			msg.SequenceNumber = sequenceNumber
			msg.OnRampAddress = onRamp
			encodedMessage, err := msg.Encode()
			require.NoError(t, err)

			var messageID [32]byte
			copy(messageID[:], eth.Keccak256(encodedMessage))
			verifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
			require.NoError(t, err)

			return ccip_offramp.ExecutionInputsV2{
				EncodedMessage:  encodedMessage,
				Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
				VerifierResults: [][]byte{verifierResults},
			}
		}

		execute := func(t *testing.T, execInputs ccip_offramp.ExecutionInputsV2) {
			resolved, err := ccip.ResolveExecAccounts(ctx, solanaGoClient, config.CcipOfframpProgram, execInputs, user.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			result := testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, user, config.DefaultCommitment, resolveLutMap(t, resolved.LookupTables), common.AddComputeUnitLimit(400_000))
			require.NotNil(t, result)
		}

		t.Run("a message from the old OnRamp executes", func(t *testing.T) {
			execute(t, execInputsFrom(t, oldOnRamp, 9101))
		})

		t.Run("a message from the upgraded OnRamp executes", func(t *testing.T) {
			execute(t, execInputsFrom(t, upgradedOnRamp[:32], 9102))
		})

		t.Run("dropping an OnRamp with inflight messages is recoverable", func(t *testing.T) {
			// Narrowing the set to the upgraded OnRamp alone — as an operator would once the old one
			// has drained — while a message from the old OnRamp is still inflight.
			onlyUpgraded := validSourceChainConfig
			onlyUpgraded.OnRamps = []ccip_offramp.OnRampAddress{{Bytes: upgradedOnRamp, Len: 32}}
			updateEvmSourceChainConfig(t, onlyUpgraded)

			strandedInputs := execInputsFrom(t, oldOnRamp, 9103)
			resolved, err := ccip.ResolveExecAccounts(ctx, solanaGoClient, config.CcipOfframpProgram, strandedInputs, user.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			testutils.SendAndFailWithLookupTables(
				ctx,
				t,
				solanaGoClient,
				[]solana.Instruction{resolved.Ix},
				user,
				config.DefaultCommitment,
				resolveLutMap(t, resolved.LookupTables),
				[]string{ccip.InvalidOnrampAddress_CcipOfframpError.String()},
				common.AddComputeUnitLimit(400_000),
			)

			// Re-adding the dropped OnRamp makes that very same message executable again: the check
			// runs at execution time against the current config, and the rejected attempt above wrote
			// no state to undo.
			updateEvmSourceChainConfig(t, upgradeWindowConfig)
			execute(t, strandedInputs)
		})
	})

	// useMultisigMintAuthority hands the pool's mint authority to a 1-of-2 SPL multisig that has the
	// pool signer as a member, and appends the multisig to the pool lookup table as an additional
	// entry: burn/mint pools find a multisig mint authority among the extra accounts they are
	// forwarded. Call it while Token0Admin is still the mint authority and before the lookup table
	// is set up.
	useMultisigMintAuthority := func(t *testing.T, pool *tokens.TokenPool) {
		multisig := solana.NewWallet().PrivateKey
		ixMsig, err := tokens.CreateMultisig(ctx, testutils.Token0Admin.PublicKey(), pool.Program, multisig.PublicKey(), 1, []solana.PublicKey{testutils.Token0Admin.PublicKey(), pool.PoolSigner}, solanaGoClient, config.DefaultCommitment)
		require.NoError(t, err)
		ixAuth, err := tokens.SetTokenMintAuthority(pool.Program, multisig.PublicKey(), pool.Mint, testutils.Token0Admin.PublicKey())
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, append(ixMsig, ixAuth), testutils.Token0Admin, config.DefaultCommitment, common.AddSigners(multisig))
		pool.AdditionalAccounts = append(pool.AdditionalAccounts, multisig.PublicKey())
	}

	// setupV1BurnmintPool sets up a legacy burn/mint pool (no IPoolV2 interface, NoResolution) that
	// accepts transfers from v1SourcePoolAddress. With multisigMintAuthority the pool mints through a
	// multisig carried as a lookup table extra (see useMultisigMintAuthority); otherwise the pool
	// signer is the mint authority.
	v1SourcePoolAddressTrim := protocol.ByteSlice(config.EVMRemotePoolAddressBytesTrim)
	v1SourcePoolAddressFull := protocol.ByteSlice(config.EVMRemotePoolAddressBytes)
	setupV1BurnmintPool := func(t *testing.T, multisigMintAuthority bool) tokens.TokenPool {
		v1TokenMint := solana.NewWallet().PrivateKey
		v1Token, err := tokens.NewTokenPool(config.SPLTokenProgram, config.CcipBurnMintTokenPoolProgram, v1TokenMint.PublicKey())
		require.NoError(t, err)

		// Create the pool token account before building the LUT so the legacy pool account list is complete.
		ixV1Token, err := tokens.CreateTokenFrom(ctx, v1Token, testutils.Token0Admin.PublicKey(), 9, solanaGoClient, config.DefaultCommitment)
		require.NoError(t, err)
		ixAtaPoolV1, addrPoolV1, err := tokens.CreateAssociatedTokenAccount(v1Token.Program, v1Token.Mint, v1Token.PoolSigner, testutils.Token0Admin.PublicKey())
		require.NoError(t, err)
		v1Token.PoolTokenAccount = addrPoolV1
		v1Token.User[v1Token.PoolSigner] = addrPoolV1
		ixV1Token = append(ixV1Token, ixAtaPoolV1)

		if !multisigMintAuthority {
			// Burn/mint: transfer mint authority to the pool signer so the pool can mint on release.
			ixSetMintAuthorityV1, authErr := tokens.SetTokenMintAuthority(v1Token.Program, v1Token.PoolSigner, v1Token.Mint, testutils.Token0Admin.PublicKey())
			require.NoError(t, authErr)
			ixV1Token = append(ixV1Token, ixSetMintAuthorityV1)
		}
		testutils.SendAndConfirm(ctx, t, solanaGoClient, ixV1Token, testutils.Token0Admin, config.DefaultCommitment, common.AddSigners(v1TokenMint))
		if multisigMintAuthority {
			useMultisigMintAuthority(t, &v1Token)
		}

		programDataAddress := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, config.CcipBurnMintTokenPoolProgram)
		globalConfigPDA, err := tokens.TokenPoolGlobalConfigPDA(config.CcipBurnMintTokenPoolProgram)
		require.NoError(t, err)

		ixInitV1, err := burnmint_tokenpool.NewInitializeInstruction(
			v1Token.PoolConfig,
			v1Token.Mint,
			testutils.Admin.PublicKey(),
			solana.SystemProgramID,
			config.CcipBurnMintTokenPoolProgram,
			programDataAddress,
			globalConfigPDA,
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixInitV1}, testutils.Admin, config.DefaultCommitment)

		ixTransferV1, err := burnmint_tokenpool.NewTransferOwnershipInstruction(
			testutils.Token0Admin.PublicKey(),
			v1Token.PoolConfig,
			v1Token.Mint,
			testutils.Admin.PublicKey(),
		)
		require.NoError(t, err)
		ixAcceptV1, err := burnmint_tokenpool.NewAcceptOwnershipInstruction(
			v1Token.PoolConfig,
			v1Token.Mint,
			testutils.Token0Admin.PublicKey(),
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixTransferV1, ixAcceptV1}, testutils.Admin, config.DefaultCommitment, common.AddSigners(testutils.Token0Admin))

		// Setup lookup table for v1Token after its pool token account is known.
		require.NoError(t, v1Token.SetupLookupTable(ctx, solanaGoClient, testutils.Token0Admin))

		registerPoolWithRouter(t, v1Token, 0, ccip_router.ResolutionType_NoResolution)

		ixRemoteConfigV1, err := burnmint_tokenpool.NewInitChainRemoteConfigInstruction(
			config.EvmChainSelector,
			v1Token.Mint,
			burnmint_tokenpool.RemoteConfig{
				TokenAddress: burnmint_tokenpool.RemoteAddress{Address: config.EVMToken0AddressBytes},
				Decimals:     remoteTokenDecimals,
			},
			v1Token.PoolConfig,
			v1Token.Chain[config.EvmChainSelector],
			testutils.Token0Admin.PublicKey(),
			solana.SystemProgramID,
		)
		require.NoError(t, err)
		ixAppendRemoteV1, err := burnmint_tokenpool.NewAppendRemotePoolAddressesInstruction(
			config.EvmChainSelector,
			v1Token.Mint,
			[]burnmint_tokenpool.RemoteAddress{{Address: v1SourcePoolAddressTrim}},
			v1Token.PoolConfig,
			v1Token.Chain[config.EvmChainSelector],
			testutils.Token0Admin.PublicKey(),
			solana.SystemProgramID,
		)
		require.NoError(t, err)
		ixRateLimitV1, err := burnmint_tokenpool.NewSetChainRateLimitInstruction(
			config.EvmChainSelector,
			v1Token.Mint,
			burnmint_tokenpool.RateLimitConfig{},
			burnmint_tokenpool.RateLimitConfig{},
			v1Token.PoolConfig,
			v1Token.Chain[config.EvmChainSelector],
			testutils.Token0Admin.PublicKey(),
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixRemoteConfigV1, ixAppendRemoteV1, ixRateLimitV1}, testutils.Token0Admin, config.DefaultCommitment)

		ensureCommitteeVerifierLane(t, 1)

		return v1Token
	}

	t.Run("token v1 bnm", func(t *testing.T) {
		// Legacy burn/mint pool transfer: the pool holds mint authority and mints the received amount to
		// the receiver ATA. No IPoolV2 interface, so the lane-default committee verifier is enforced.
		v1Token := setupV1BurnmintPool(t, false)

		ixAtaReceiverV1, addrReceiverV1, err := tokens.CreateAssociatedTokenAccount(v1Token.Program, v1Token.Mint, config.ReceiverExternalExecutionConfigPDA, testutils.Token0Admin.PublicKey())
		require.NoError(t, err)
		v1Token.User[config.ReceiverExternalExecutionConfigPDA] = addrReceiverV1
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixAtaReceiverV1}, testutils.Token0Admin, config.DefaultCommitment)

		encodedMessage, messageID := buildTokenTransferMsg(t, v1Token.Mint, v1SourcePoolAddressFull, config.ReceiverExternalExecutionConfigPDA, ccip_common.SvmTokenReceiverUsage_DeriveAtaDontCreate)
		verifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
		require.NoError(t, err)

		execInputs := ccip_offramp.ExecutionInputsV2{
			EncodedMessage:  encodedMessage,
			Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
			VerifierResults: [][]byte{verifierResults},
		}

		resolved, err := ccip.ResolveExecAccounts(ctx, solanaGoClient, config.CcipOfframpProgram, execInputs, user.PublicKey(), logger.Test(t))
		require.NoError(t, err)

		result := testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, user, config.DefaultCommitment, resolveLutMap(t, resolved.LookupTables), common.AddComputeUnitLimit(400_000))
		require.NotNil(t, result)

		mintEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, base_token_pool.ParseEvent_Minted)
		require.NoError(t, err)
		require.Equal(t, config.ReceiverExternalExecutionConfigPDA, mintEvent.Recipient)
		require.Equal(t, uint64(1), mintEvent.Amount)
		require.Equal(t, v1Token.PoolSigner, mintEvent.Sender)
		require.Equal(t, v1Token.Mint, mintEvent.Mint)

		// get_required_ccvs view: a legacy (non-IPoolV2) pool is never queried, so the OffRamp applies
		// the lane defaults (EVM's address(0) placeholder branch). The view must report the lane-default
		// committee verifier as the required CCV for a v1 token transfer.
		viewReturn := getRequiredCcvsView(t, encodedMessage)
		require.Equal(t, []solana.PublicKey{config.CommitteeVerifierProgram}, viewReturn.RequiredCcvs)

		// The OffRamp narrows the source pool address to the lane's legacy width (20 bytes for EVM)
		// before CPI'ing into the pool, which stores the 20-byte form. These run through the legacy
		// (v1 interface) path, where the narrowing happens at execution time.
		execV1Transfer := func(t *testing.T, sourcePool protocol.ByteSlice, wantErr string) *rpc.GetTransactionResult {
			t.Helper()
			encodedMessage, messageID := buildTokenTransferMsg(t, v1Token.Mint, sourcePool, config.ReceiverExternalExecutionConfigPDA, ccip_common.SvmTokenReceiverUsage_DeriveAtaDontCreate)
			verifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
			require.NoError(t, err)
			execInputs := ccip_offramp.ExecutionInputsV2{
				EncodedMessage:  encodedMessage,
				Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
				VerifierResults: [][]byte{verifierResults},
			}
			resolved, err := ccip.ResolveExecAccounts(ctx, solanaGoClient, config.CcipOfframpProgram, execInputs, user.PublicKey(), logger.Test(t))
			require.NoError(t, err)
			ixs := []solana.Instruction{resolved.Ix}
			lutMap := resolveLutMap(t, resolved.LookupTables)
			if wantErr == "" {
				return testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient, ixs, user, config.DefaultCommitment, lutMap, common.AddComputeUnitLimit(400_000))
			}
			return testutils.SendAndFailWithLookupTables(ctx, t, solanaGoClient, ixs, user, config.DefaultCommitment, lutMap, []string{wantErr}, common.AddComputeUnitLimit(400_000))
		}

		t.Run("a padded source pool address with a non-zero prefix is rejected by the OffRamp", func(t *testing.T) {
			// Same 20 trailing bytes as the registered pool, so only the padding check can reject it.
			dirty := append(protocol.ByteSlice{}, v1SourcePoolAddressFull...)
			dirty[0] = 0x01
			// The pool has an error with the same name, so match the OffRamp's error number too:
			// Anchor's 6000 base plus the OffRamp's 3000 offset (see ccip-offramp/src/error.rs).
			const offrampErrorCodeOffset = 9000
			offrampErr := fmt.Sprintf("Error Code: %s. Error Number: %d",
				ccip.InvalidSourcePoolAddress_CcipOfframpError.String(),
				offrampErrorCodeOffset+int(ccip.InvalidSourcePoolAddress_CcipOfframpError))
			execV1Transfer(t, dirty, offrampErr)
		})

		t.Run("a source pool address already at the legacy width passes through unchanged", func(t *testing.T) {
			require.Len(t, v1SourcePoolAddressTrim, 20)
			result := execV1Transfer(t, v1SourcePoolAddressTrim, "")
			mintEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, base_token_pool.ParseEvent_Minted)
			require.NoError(t, err)
			require.Equal(t, v1Token.Mint, mintEvent.Mint)
		})
	})

	t.Run("token v1 lnr", func(t *testing.T) {
		// Legacy lock/release pool transfer: mirrors the v1 bnm setup but the release model differs.
		// The pool does NOT hold mint authority; instead it is pre-funded with liquidity (mint into a
		// rebalancer ATA, set_rebalancer, approve the pool signer as delegate, provide_liquidity). A
		// release moves custody from the pool to the receiver, leaving total supply UNCHANGED.
		v1LnrMint := solana.NewWallet().PrivateKey
		v1Lnr, err := tokens.NewTokenPool(config.SPLTokenProgram, config.CcipBasePoolLockRelease, v1LnrMint.PublicKey())
		require.NoError(t, err)

		// Create token accounts before building the LUT (Token0Admin keeps mint authority; lock/release
		// never mints). The receiver here is the shared external-execution receiver PDA, matching v1 bnm.
		ixCreate, err := tokens.CreateTokenFrom(ctx, v1Lnr, testutils.Token0Admin.PublicKey(), 9, solanaGoClient, config.DefaultCommitment)
		require.NoError(t, err)
		ixAtaUser, addrUser, err := tokens.CreateAssociatedTokenAccount(v1Lnr.Program, v1Lnr.Mint, user.PublicKey(), testutils.Token0Admin.PublicKey())
		require.NoError(t, err)
		v1Lnr.User[user.PublicKey()] = addrUser
		ixAtaReceiver, addrReceiver, err := tokens.CreateAssociatedTokenAccount(v1Lnr.Program, v1Lnr.Mint, config.ReceiverExternalExecutionConfigPDA, testutils.Token0Admin.PublicKey())
		require.NoError(t, err)
		v1Lnr.User[config.ReceiverExternalExecutionConfigPDA] = addrReceiver
		ixAtaPool, addrPool, err := tokens.CreateAssociatedTokenAccount(v1Lnr.Program, v1Lnr.Mint, v1Lnr.PoolSigner, testutils.Token0Admin.PublicKey())
		require.NoError(t, err)
		v1Lnr.PoolTokenAccount = addrPool
		v1Lnr.User[v1Lnr.PoolSigner] = addrPool
		// Rebalancer (Token0Admin) liquidity ATA, funded via Token0Admin's retained mint authority.
		ixRebalancerAta, rebalancerAta, err := tokens.CreateAssociatedTokenAccount(v1Lnr.Program, v1Lnr.Mint, testutils.Token0Admin.PublicKey(), testutils.Token0Admin.PublicKey())
		require.NoError(t, err)
		ixMintLiquidity, err := tokens.MintTo(1000000, v1Lnr.Program, v1Lnr.Mint, rebalancerAta, testutils.Token0Admin.PublicKey())
		require.NoError(t, err)
		testutils.SendAndConfirm(
			ctx,
			t,
			solanaGoClient,
			append(ixCreate, ixAtaUser, ixAtaReceiver, ixAtaPool, ixRebalancerAta, ixMintLiquidity),
			testutils.Token0Admin,
			config.DefaultCommitment,
			common.AddSigners(v1LnrMint),
		)

		programDataAddress := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, config.CcipBasePoolLockRelease)
		globalConfigPDA, err := tokens.TokenPoolGlobalConfigPDA(config.CcipBasePoolLockRelease)
		require.NoError(t, err)

		// The shared env only initializes the burnmint global config; lockrelease's must be initialized
		// here because Initialize requires it (else AccountNotInitialized on config). Init is idempotent
		// across lnr subtests only if not already present, so guard it.
		var existingGlobalCfg lockrelease_tokenpool.PoolConfig
		if getErr := common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, globalConfigPDA, config.DefaultCommitment, &existingGlobalCfg); getErr != nil {
			ixGlobalConfig, gErr := lockrelease_tokenpool.NewInitGlobalConfigInstruction(
				config.CcipRouterProgram,
				config.RMNRemoteProgram,
				globalConfigPDA,
				testutils.Admin.PublicKey(),
				solana.SystemProgramID,
				config.CcipBasePoolLockRelease,
				programDataAddress,
			)
			require.NoError(t, gErr)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixGlobalConfig}, testutils.Admin, config.DefaultCommitment)
		}

		ixInit, err := lockrelease_tokenpool.NewInitializeInstruction(
			v1Lnr.PoolConfig, v1Lnr.Mint, testutils.Admin.PublicKey(), solana.SystemProgramID,
			config.CcipBasePoolLockRelease, programDataAddress, globalConfigPDA,
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixInit}, testutils.Admin, config.DefaultCommitment)

		ixTransfer, err := lockrelease_tokenpool.NewTransferOwnershipInstruction(
			testutils.Token0Admin.PublicKey(), v1Lnr.PoolConfig, v1Lnr.Mint,
			testutils.Admin.PublicKey(),
		)
		require.NoError(t, err)
		ixAccept, err := lockrelease_tokenpool.NewAcceptOwnershipInstruction(
			v1Lnr.PoolConfig, v1Lnr.Mint, testutils.Token0Admin.PublicKey(),
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixTransfer, ixAccept}, testutils.Admin, config.DefaultCommitment, common.AddSigners(testutils.Token0Admin))

		// Provide liquidity the idiomatic lock-release way: set the rebalancer, approve the pool signer
		// as delegate over the rebalancer's ATA, then pull the liquidity into the pool token account.
		const liquidityAmount = 100000
		ixSetReb, err := lockrelease_tokenpool.NewSetRebalancerInstruction(
			testutils.Token0Admin.PublicKey(), v1Lnr.PoolConfig, v1Lnr.Mint,
			testutils.Token0Admin.PublicKey(),
		)
		require.NoError(t, err)
		ixApprove, err := tokens.TokenApproveChecked(liquidityAmount, 9, v1Lnr.Program, rebalancerAta, v1Lnr.Mint, v1Lnr.PoolSigner, testutils.Token0Admin.PublicKey(), nil)
		require.NoError(t, err)
		ixProvide, err := lockrelease_tokenpool.NewProvideLiquidityInstruction(
			liquidityAmount, v1Lnr.PoolConfig, v1Lnr.Program, v1Lnr.Mint, v1Lnr.PoolSigner,
			v1Lnr.PoolTokenAccount, rebalancerAta, testutils.Token0Admin.PublicKey(),
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixSetReb, ixApprove, ixProvide}, testutils.Token0Admin, config.DefaultCommitment)

		require.NoError(t, v1Lnr.SetupLookupTable(ctx, solanaGoClient, testutils.Token0Admin))

		// Legacy v1 pool: register with interface version 1 (no account resolution).
		registerPoolWithRouter(t, v1Lnr, 1, ccip_router.ResolutionType_NoResolution)

		happyPathSourcePoolAddress := protocol.ByteSlice(config.EVMRemotePoolAddressBytesTrim) // this has to match the length configured in the offramp's source chain
		ixRemoteConfig, err := lockrelease_tokenpool.NewInitChainRemoteConfigInstruction(
			config.EvmChainSelector,
			v1Lnr.Mint,
			lockrelease_tokenpool.RemoteConfig{
				TokenAddress: lockrelease_tokenpool.RemoteAddress{Address: config.EVMToken0AddressBytes},
				Decimals:     remoteTokenDecimals,
			},
			v1Lnr.PoolConfig,
			v1Lnr.Chain[config.EvmChainSelector],
			testutils.Token0Admin.PublicKey(),
			solana.SystemProgramID,
		)
		require.NoError(t, err)
		ixAppendRemote, err := lockrelease_tokenpool.NewAppendRemotePoolAddressesInstruction(
			config.EvmChainSelector,
			v1Lnr.Mint,
			[]lockrelease_tokenpool.RemoteAddress{{Address: happyPathSourcePoolAddress}},
			v1Lnr.PoolConfig,
			v1Lnr.Chain[config.EvmChainSelector],
			testutils.Token0Admin.PublicKey(),
			solana.SystemProgramID,
		)
		require.NoError(t, err)
		ixRateLimit, err := lockrelease_tokenpool.NewSetChainRateLimitInstruction(
			config.EvmChainSelector,
			v1Lnr.Mint,
			lockrelease_tokenpool.RateLimitConfig{},
			lockrelease_tokenpool.RateLimitConfig{},
			v1Lnr.PoolConfig,
			v1Lnr.Chain[config.EvmChainSelector],
			testutils.Token0Admin.PublicKey(),
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixRemoteConfig, ixAppendRemote, ixRateLimit}, testutils.Token0Admin, config.DefaultCommitment)

		ensureCommitteeVerifierLane(t, 1)

		_, initSupply, err := tokens.TokenSupply(ctx, solanaGoClient, v1Lnr.Mint, config.DefaultCommitment)
		require.NoError(t, err)
		_, initRecvBal, err := tokens.TokenBalance(ctx, solanaGoClient, addrReceiver, config.DefaultCommitment)
		require.NoError(t, err)
		_, initPoolBal, err := tokens.TokenBalance(ctx, solanaGoClient, v1Lnr.PoolTokenAccount, config.DefaultCommitment)
		require.NoError(t, err)

		encodedMessage, messageID := buildTokenTransferMsg(t, v1Lnr.Mint, happyPathSourcePoolAddress, config.ReceiverExternalExecutionConfigPDA, ccip_common.SvmTokenReceiverUsage_DeriveAtaDontCreate)
		verifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
		require.NoError(t, err)

		execInputs := ccip_offramp.ExecutionInputsV2{
			EncodedMessage:  encodedMessage,
			Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
			VerifierResults: [][]byte{verifierResults},
		}
		resolved, err := ccip.ResolveExecAccounts(ctx, solanaGoClient, config.CcipOfframpProgram, execInputs, user.PublicKey(), logger.Test(t))
		require.NoError(t, err)
		result := testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, user, config.DefaultCommitment, resolveLutMap(t, resolved.LookupTables), common.AddComputeUnitLimit(400_000))
		require.NotNil(t, result)

		// A legacy (interface_version <= 1) lock-release pool takes the V1 release path, which emits the
		// `Released` event via `emit!` (a program-log event) — not the V2 `emit_cpi!` ReleasedOrMinted.
		// So it is read from the program logs, not from inner-instruction CPI events (matching how the
		// v1 bnm case reads `Minted`).
		relEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, base_token_pool.ParseEvent_Released)
		require.NoError(t, err)
		require.Equal(t, config.ReceiverExternalExecutionConfigPDA, relEvent.Recipient)
		require.Equal(t, v1Lnr.PoolSigner, relEvent.Sender)
		require.Equal(t, uint64(1), relEvent.Amount)
		require.Equal(t, v1Lnr.Mint, relEvent.Mint)

		// Release model: total supply UNCHANGED; tokens move from the pool to the receiver.
		_, finalSupply, err := tokens.TokenSupply(ctx, solanaGoClient, v1Lnr.Mint, config.DefaultCommitment)
		require.NoError(t, err)
		require.Equal(t, initSupply, finalSupply)
		_, finalRecvBal, err := tokens.TokenBalance(ctx, solanaGoClient, addrReceiver, config.DefaultCommitment)
		require.NoError(t, err)
		require.Equal(t, 1, finalRecvBal-initRecvBal)
		_, finalPoolBal, err := tokens.TokenBalance(ctx, solanaGoClient, v1Lnr.PoolTokenAccount, config.DefaultCommitment)
		require.NoError(t, err)
		require.Equal(t, 1, initPoolBal-finalPoolBal)

		t.Run("pool ATA as token receiver", func(t *testing.T) {
			encodedMessage, messageID := buildTokenTransferMsg(t, v1Lnr.Mint, happyPathSourcePoolAddress, v1Lnr.PoolSigner, ccip_common.SvmTokenReceiverUsage_DeriveAtaDontCreate)
			verifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
			require.NoError(t, err)
			execInputs := ccip_offramp.ExecutionInputsV2{
				EncodedMessage:  encodedMessage,
				Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
				VerifierResults: [][]byte{verifierResults},
			}
			resolved := resolveExecAccountsWithAutoBuffering(t, execInputs, user)

			balanceBefore := env.GetBalance(v1Lnr.PoolTokenAccount)
			require.Positive(t, balanceBefore)
			_, supplyBefore, err := tokens.TokenSupply(ctx, solanaGoClient, v1Lnr.Mint, config.DefaultCommitment)
			require.NoError(t, err)

			result := testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, user, config.DefaultCommitment, resolveLutMap(t, resolved.LookupTables), common.AddComputeUnitLimit(400_000))
			require.NotNil(t, result)

			relEvent, err := common.ParseEventFromLogs(result.Meta.LogMessages, base_token_pool.ParseEvent_Released)
			require.NoError(t, err)
			require.Equal(t, v1Lnr.PoolSigner, relEvent.Recipient)
			require.Equal(t, v1Lnr.PoolSigner, relEvent.Sender)
			require.Equal(t, uint64(1), relEvent.Amount)
			require.Equal(t, v1Lnr.Mint, relEvent.Mint)

			require.Equal(t, balanceBefore, env.GetBalance(v1Lnr.PoolTokenAccount), "releasing to the pool itself must not change its balance")
			_, supplyAfter, err := tokens.TokenSupply(ctx, solanaGoClient, v1Lnr.Mint, config.DefaultCommitment)
			require.NoError(t, err)
			require.Equal(t, supplyBefore, supplyAfter)
		})
	})

	// setupV2BurnmintPool creates, funds, registers (interface v2) and lane-configures a v2 burn/mint
	// pool, returning it ready for transfers and view queries.
	// setupV2BurnmintPoolWith sets up a v2 burn/mint pool with Standard resolution. With
	// multisigMintAuthority the pool mints through a multisig carried as a lookup table extra (see
	// useMultisigMintAuthority); otherwise the pool signer is the mint authority.
	v2SourcePoolAddressFull := protocol.ByteSlice(config.EVMRemotePoolAddressBytes)
	v2SourcePoolAddressTrim := protocol.ByteSlice(config.EVMRemotePoolAddressBytesTrim)
	setupV2BurnmintPoolWith := func(t *testing.T, multisigMintAuthority bool) tokens.TokenPool {
		v2TokenMint := solana.NewWallet().PrivateKey
		v2Token, err := tokens.NewTokenPool(config.SPLTokenProgram, config.CcipBurnMintTokenPoolProgram, v2TokenMint.PublicKey())
		require.NoError(t, err)

		ixV2, err := tokens.CreateTokenFrom(ctx, v2Token, testutils.Token0Admin.PublicKey(), 9, solanaGoClient, config.DefaultCommitment)
		require.NoError(t, err)
		ixAtaV2, addrV2, err := tokens.CreateAssociatedTokenAccount(v2Token.Program, v2Token.Mint, user.PublicKey(), testutils.Token0Admin.PublicKey())
		require.NoError(t, err)
		ixMintToV2, err := tokens.MintTo(10000000, v2Token.Program, v2Token.Mint, addrV2, testutils.Token0Admin.PublicKey())
		require.NoError(t, err)
		v2Token.User[user.PublicKey()] = addrV2
		ixV2 = append(ixV2, ixAtaV2, ixMintToV2)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, ixV2, testutils.Token0Admin, config.DefaultCommitment, common.AddSigners(v2TokenMint))

		programDataAddress := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, config.CcipBurnMintTokenPoolProgram)
		globalConfigPDA, err := tokens.TokenPoolGlobalConfigPDA(config.CcipBurnMintTokenPoolProgram)
		require.NoError(t, err)

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
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixInitV2}, testutils.Admin, config.DefaultCommitment)

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
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixTransferV2, ixAcceptV2}, testutils.Admin, config.DefaultCommitment, common.AddSigners(testutils.Token0Admin))

		ixAtaV2Pool, addrV2Pool, err := tokens.CreateAssociatedTokenAccount(v2Token.Program, v2Token.Mint, v2Token.PoolSigner, testutils.Token0Admin.PublicKey())
		require.NoError(t, err)
		v2Token.PoolTokenAccount = addrV2Pool
		v2Token.User[v2Token.PoolSigner] = addrV2Pool
		if multisigMintAuthority {
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixAtaV2Pool}, testutils.Token0Admin, config.DefaultCommitment)
			// Lookup table extras reach a 2.0 pool ahead of the accounts its nested resolution
			// supplies, so they must lead with the pool's own `#[event_cpi]` accounts or they would
			// shift its account list (see example_sender_v2_test.go).
			eventAuthority, _, findErr := state.FindEventAuthorityPDA(v2Token.PoolProgram)
			require.NoError(t, findErr)
			v2Token.AdditionalAccounts = append(v2Token.AdditionalAccounts, eventAuthority, v2Token.PoolProgram)
			useMultisigMintAuthority(t, &v2Token)
		} else {
			ixAuthV2, authErr := tokens.SetTokenMintAuthority(v2Token.Program, v2Token.PoolSigner, v2Token.Mint, testutils.Token0Admin.PublicKey())
			require.NoError(t, authErr)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixAtaV2Pool, ixAuthV2}, testutils.Token0Admin, config.DefaultCommitment)
		}

		require.NoError(t, v2Token.SetupLookupTable(ctx, solanaGoClient, testutils.Token0Admin))

		ensureCommitteeVerifierLane(t, 1)
		registerPoolWithRouter(t, v2Token, 2, ccip_router.ResolutionType_Standard)

		poolAddresses := []burnmint_tokenpool.RemoteAddress{{Address: v2SourcePoolAddressTrim}}
		ixRemoteConfigV2, err := burnmint_tokenpool.NewInitChainRemoteConfigInstruction(
			config.EvmChainSelector,
			v2Token.Mint,
			burnmint_tokenpool.RemoteConfig{
				TokenAddress: burnmint_tokenpool.RemoteAddress{Address: config.EVMToken0AddressBytes},
				Decimals:     remoteTokenDecimals,
			},
			v2Token.PoolConfig,
			v2Token.Chain[config.EvmChainSelector],
			testutils.Token0Admin.PublicKey(),
			solana.SystemProgramID,
		)
		require.NoError(t, err)
		ixAppendRemoteV2, err := burnmint_tokenpool.NewAppendRemotePoolAddressesInstruction(
			config.EvmChainSelector,
			v2Token.Mint,
			poolAddresses,
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
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixRemoteConfigV2, ixAppendRemoteV2, ixRateLimitV2}, testutils.Token0Admin, config.DefaultCommitment)

		return v2Token
	}

	setupV2BurnmintPool := func(t *testing.T) tokens.TokenPool {
		return setupV2BurnmintPoolWith(t, false)
	}

	t.Run("token v2 bnm (all ATA options)", func(t *testing.T) {
		v2Token := setupV2BurnmintPool(t)

		t.Run("Positive cases", func(t *testing.T) {
			// Each case uses its own receiver so the subtests are INDEPENDENT — no case relies on
			// another having created an ATA. The "ATA does not exist" case uses a wallet whose ATA is
			// NOT pre-created (the execute must create it); the idempotent and use-as-is cases use
			// wallets whose ATAs are pre-created here in setup.
			createUser := solana.NewWallet()
			createUserAta, _, err := solana.FindAssociatedTokenAddress(createUser.PublicKey(), v2Token.Mint)
			require.NoError(t, err)

			idempotentUser := solana.NewWallet()
			ixCreateIdempotentAta, idempotentUserAta, err := tokens.CreateAssociatedTokenAccount(v2Token.Program, v2Token.Mint, idempotentUser.PublicKey(), testutils.Token0Admin.PublicKey())
			require.NoError(t, err)

			useAsIsUser := solana.NewWallet()
			ixCreateUseAsIsAta, useAsIsUserAta, err := tokens.CreateAssociatedTokenAccount(v2Token.Program, v2Token.Mint, useAsIsUser.PublicKey(), testutils.Token0Admin.PublicKey())
			require.NoError(t, err)

			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixCreateIdempotentAta, ixCreateUseAsIsAta}, testutils.Token0Admin, config.DefaultCommitment)

			type testModifiers struct {
				callAtaCreation bool
				ataExistBefore  bool
			}

			type testCase struct {
				name                 string
				usage                ccip_common.SvmTokenReceiverUsage
				tokenReceiverWallet  solana.PublicKey
				receiverTokenAccount solana.PublicKey
				should               testModifiers
			}

			testcases := []testCase{
				{
					name:                 "DeriveAtaDontCreate",
					usage:                ccip_common.SvmTokenReceiverUsage_DeriveAtaDontCreate,
					tokenReceiverWallet:  user.PublicKey(),
					receiverTokenAccount: v2Token.User[user.PublicKey()],
					should: testModifiers{
						callAtaCreation: false,
						ataExistBefore:  true,
					},
				},
				{
					// ATA does not exist yet -> the execute must create it.
					name:                 "DeriveAtaAndCreate (ATA does not exist)",
					usage:                ccip_common.SvmTokenReceiverUsage_DeriveAtaAndCreate,
					tokenReceiverWallet:  createUser.PublicKey(),
					receiverTokenAccount: createUserAta,
					should: testModifiers{
						callAtaCreation: true,
						ataExistBefore:  false,
					},
				},
				{
					// ATA pre-created in setup -> the execute calls create_idempotent_ata even though
					// it already exists (idempotent). Independent of the case above.
					name:                 "DeriveAtaAndCreate (ATA already exists, idempotent)",
					usage:                ccip_common.SvmTokenReceiverUsage_DeriveAtaAndCreate,
					tokenReceiverWallet:  idempotentUser.PublicKey(),
					receiverTokenAccount: idempotentUserAta,
					should: testModifiers{
						callAtaCreation: true,
						ataExistBefore:  true,
					},
				},
				{
					// Receiver is the (pre-created) token account itself, used as-is.
					name:                 "UseAsIs",
					usage:                ccip_common.SvmTokenReceiverUsage_UseAsIs,
					tokenReceiverWallet:  useAsIsUserAta,
					receiverTokenAccount: useAsIsUserAta,
					should: testModifiers{
						callAtaCreation: false,
						ataExistBefore:  true,
					},
				},
			}

			for _, tc := range testcases {
				t.Run("Token transfer with receiver usage "+tc.name, func(t *testing.T) {
					_, initSupply, err := tokens.TokenSupply(ctx, solanaGoClient, v2Token.Mint, config.DefaultCommitment)
					require.NoError(t, err)

					initBal := 0
					if tc.should.ataExistBefore {
						_, initBal, err = tokens.TokenBalance(ctx, solanaGoClient, tc.receiverTokenAccount, config.DefaultCommitment)
						require.NoError(t, err)
					}

					encodedMessage, messageID := buildTokenTransferMsg(t, v2Token.Mint, v2SourcePoolAddressFull, tc.tokenReceiverWallet, tc.usage)
					verifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
					require.NoError(t, err)

					execInputs := ccip_offramp.ExecutionInputsV2{
						EncodedMessage:  encodedMessage,
						Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
						VerifierResults: [][]byte{verifierResults},
					}

					resolved := resolveExecAccountsWithAutoBuffering(t, execInputs, user)

					result := testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, user, config.DefaultCommitment, resolveLutMap(t, resolved.LookupTables), common.AddComputeUnitLimit(400_000))
					require.NotNil(t, result)

					tx, err := result.Transaction.GetTransaction()
					require.NoError(t, err)

					mintEvent, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipBurnMintTokenPoolProgram, base_token_pool.ParseEvent_ReleasedOrMinted)
					require.NoError(t, err)
					require.Equal(t, tc.tokenReceiverWallet, mintEvent.Recipient)
					require.Equal(t, v2Token.PoolSigner, mintEvent.Sender)
					require.Equal(t, uint64(1), mintEvent.Amount)
					require.Equal(t, v2Token.Mint, mintEvent.Mint)
					require.Equal(t, config.EvmChainSelector, mintEvent.RemoteChainSelector)

					_, ataCpiErr := common.ExtractCPI(tx, result.Meta, tokens.AssociatedTokenProgramID, 0)
					if tc.should.callAtaCreation {
						require.NoError(t, ataCpiErr)
					} else {
						require.Error(t, ataCpiErr)
					}

					_, finalSupply, err := tokens.TokenSupply(ctx, solanaGoClient, v2Token.Mint, config.DefaultCommitment)
					require.NoError(t, err)
					_, finalBal, err := tokens.TokenBalance(ctx, solanaGoClient, tc.receiverTokenAccount, config.DefaultCommitment)
					require.NoError(t, err)
					require.Equal(t, 1, finalSupply-initSupply)
					require.Equal(t, 1, finalBal-initBal)
				})
			}
		})

		t.Run("Negative cases", func(t *testing.T) {
			newUser := solana.MustPrivateKeyFromBase58("5cKYF5zHyFsdTS729hdRhZXpmvW2dPBEpkhs8HhT4JtsYgRkQgoJvzRZiwyC9DpV2ykxFgJcsgtH2mLGPJ44NdyT")
			newUserAta, _, err := solana.FindAssociatedTokenAddress(newUser.PublicKey(), v2Token.Mint)
			require.NoError(t, err)
			newUserAtaOtherToken, _, err := solana.FindAssociatedTokenAddress(newUser.PublicKey(), env.Token0.Mint)
			require.NoError(t, err)

			type testCase struct {
				name                     string
				usage                    ccip_common.SvmTokenReceiverUsage
				tokenReceiverWallet      solana.PublicKey
				overrideTokenAccountWith solana.PublicKey
				errorType                string
			}

			testcases := []testCase{
				{
					name:                "DeriveAtaDontCreate with uninitialized ATA",
					usage:               ccip_common.SvmTokenReceiverUsage_DeriveAtaDontCreate,
					tokenReceiverWallet: newUser.PublicKey(),
					errorType:           ccip.InvalidInputsTokenAccounts_CcipCommonError.String(),
				},
				{
					name:                "UseAsIs with wallet that is not a token account",
					usage:               ccip_common.SvmTokenReceiverUsage_UseAsIs,
					tokenReceiverWallet: newUser.PublicKey(),
					errorType:           ccip.InvalidInputsTokenAccounts_CcipCommonError.String(),
				},
				{
					name:                     "DeriveAtaAndCreate with another user ATA (bad input)",
					usage:                    ccip_common.SvmTokenReceiverUsage_DeriveAtaAndCreate,
					tokenReceiverWallet:      newUser.PublicKey(),
					overrideTokenAccountWith: v2Token.User[user.PublicKey()],
					errorType:                "ConstraintAddress",
				},
				{
					name:                     "DeriveAtaAndCreate with another token (bad input)",
					usage:                    ccip_common.SvmTokenReceiverUsage_DeriveAtaAndCreate,
					tokenReceiverWallet:      newUser.PublicKey(),
					overrideTokenAccountWith: newUserAtaOtherToken,
					errorType:                "ConstraintAddress",
				},
			}

			for _, tc := range testcases {
				t.Run("Token transfer with "+tc.name, func(t *testing.T) {
					encodedMessage, messageID := buildTokenTransferMsg(t, v2Token.Mint, v2SourcePoolAddressFull, tc.tokenReceiverWallet, tc.usage)
					verifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
					require.NoError(t, err)

					execInputs := ccip_offramp.ExecutionInputsV2{
						EncodedMessage:  encodedMessage,
						Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
						VerifierResults: [][]byte{verifierResults},
					}

					resolved := resolveExecAccountsWithAutoBuffering(t, execInputs, user)

					if tc.overrideTokenAccountWith != (solana.PublicKey{}) {
						for i, acct := range resolved.Ix.Accounts() {
							if acct.PublicKey == newUserAta {
								resolved.Ix.Accounts()[i].PublicKey = tc.overrideTokenAccountWith
								break
							}
						}
					}

					testutils.SendAndFailWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, user, config.DefaultCommitment, resolveLutMap(t, resolved.LookupTables), []string{tc.errorType}, common.AddComputeUnitLimit(400_000))

					// Execute failed, so the buffer PDA is still around. Close it so cases that reuse the
					// same message (the DeriveAtaAndCreate pair only differs in a post-resolution account
					// override) don't collide on the PDA, and to cover the close flow.
					closeExecInputsBuffer(t, execInputs, user)
				})
			}
		})
	})

	// A pool lookup table may carry additional entries after the required ones, which the OffRamp
	// forwards to the pool. When the message asks for the receiver ATA to be created, the parsers
	// expect those entries after the receiver wallet and the Associated Token Program, so resolution
	// must emit them there. A pool minting through a multisig is the canonical case: the multisig is
	// such an entry, and the pool cannot mint without it.
	t.Run("token transfer creating the receiver ATA with lookup table extras", func(t *testing.T) {
		transferCreatingAta := func(t *testing.T, pool tokens.TokenPool, sourcePoolAddress protocol.ByteSlice) {
			require.NotEmpty(t, pool.AdditionalAccounts, "the pool lookup table must carry an additional entry")

			receiver := solana.NewWallet().PublicKey()
			receiverAta, _, err := solana.FindAssociatedTokenAddress(receiver, pool.Mint)
			require.NoError(t, err)

			encodedMessage, messageID := buildTokenTransferMsg(t, pool.Mint, sourcePoolAddress, receiver, ccip_common.SvmTokenReceiverUsage_DeriveAtaAndCreate)
			verifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
			require.NoError(t, err)

			execInputs := ccip_offramp.ExecutionInputsV2{
				EncodedMessage:  encodedMessage,
				Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
				VerifierResults: [][]byte{verifierResults},
			}

			resolved := resolveExecAccountsWithAutoBuffering(t, execInputs, user)

			result := testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, user, config.DefaultCommitment, resolveLutMap(t, resolved.LookupTables), common.AddComputeUnitLimit(400_000))
			require.NotNil(t, result)

			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			_, err = common.ExtractCPI(tx, result.Meta, tokens.AssociatedTokenProgramID, 0)
			require.NoError(t, err, "the execution must create the receiver ATA")

			_, balance, err := tokens.TokenBalance(ctx, solanaGoClient, receiverAta, config.DefaultCommitment)
			require.NoError(t, err)
			require.Equal(t, 1, balance)
		}

		t.Run("v1 pool", func(t *testing.T) {
			// The prefix, the required entries and the ATA pair leave room for the extra in the first
			// resolution round.
			transferCreatingAta(t, setupV1BurnmintPool(t, true), v1SourcePoolAddressFull)
		})

		t.Run("v2 pool", func(t *testing.T) {
			// The prefix, the required entries and the ATA pair fill the first resolution round
			// exactly, so the extras are emitted in a second one.
			transferCreatingAta(t, setupV2BurnmintPoolWith(t, true), v2SourcePoolAddressFull)
		})
	})

	// A burn/mint token whose mint authority is an SPL multisig (with the pool signer as a member)
	// has to work on 2.0 lanes in both directions. On those lanes the router and the OffRamp pass
	// the pool its named accounts, then the lookup table's additional entries, then the accounts
	// the pool's own resolver returned. lock_or_burn_tokens_v2 and release_or_mint_tokens_v2 both
	// expect their resolver's accounts right after the named ones, so any additional entry shifts
	// them. One table serves both directions, so on 2.0 the table cannot be what delivers the
	// multisig: the release_or_mint resolver has to return it.
	//
	// Each case below is a configuration that should work both ways. Today the sends succeed and
	// the executions fail with InvalidMultisig, because nothing passes the pool its multisig.
	t.Run("token v2 bnm with a multisig mint authority", func(t *testing.T) {
		programDataAddress := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, config.CcipBurnMintTokenPoolProgram)

		// setupMultisigPool starts from a pool that mints with its own signer and hands the mint
		// authority to a 1-of-2 multisig of [Token0Admin, pool signer] through the pool's own
		// instruction, as the v2_0_0 token sequence does. It then appends lookupTableExtras to the
		// pool's lookup table and registers the pool with the given resolution type.
		setupMultisigPool := func(t *testing.T, lookupTableExtras func(multisig solana.PublicKey) []solana.PublicKey, resolution ccip_router.ResolutionType) tokens.TokenPool {
			pool := setupV2BurnmintPool(t)

			multisig := solana.NewWallet().PrivateKey
			ixMsig, err := tokens.CreateMultisig(ctx, testutils.Token0Admin.PublicKey(), pool.Program, multisig.PublicKey(), 1, []solana.PublicKey{testutils.Token0Admin.PublicKey(), pool.PoolSigner}, solanaGoClient, config.DefaultCommitment)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, ixMsig, testutils.Token0Admin, config.DefaultCommitment, common.AddSigners(multisig))

			ixAuth, err := burnmint_tokenpool.NewTransferMintAuthorityToMultisigInstruction(
				pool.PoolConfig,
				pool.Mint,
				pool.Program,
				pool.PoolSigner,
				legacyAdmin.PublicKey(),
				multisig.PublicKey(),
				config.CcipBurnMintTokenPoolProgram,
				programDataAddress,
			)
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixAuth}, legacyAdmin, config.DefaultCommitment)
			pool.MintAuthorityMultisig = multisig.PublicKey()

			// Guard against a vacuous pass: the pool must not be able to mint with its signer alone.
			var mintAccount token.Mint
			require.NoError(t, common.GetAccountDataBorshInto(ctx, solanaGoClient, pool.Mint, config.DefaultCommitment, &mintAccount))
			require.NotNil(t, mintAccount.MintAuthority)
			require.Equal(t, multisig.PublicKey(), *mintAccount.MintAuthority)

			if lookupTableExtras != nil {
				extras := lookupTableExtras(multisig.PublicKey())
				require.NoError(t, common.ExtendLookupTable(ctx, solanaGoClient, pool.PoolLookupTable, testutils.Token0Admin, extras))
				require.NoError(t, common.AwaitSlotChange(ctx, solanaGoClient))
				pool.AdditionalAccounts = append(pool.AdditionalAccounts, extras...)
			}

			// setupV2BurnmintPool registers the pool with Standard resolution.
			if resolution != ccip_router.ResolutionType_Standard {
				ixInterface, ixErr := ccip_router.NewSetPoolInterfaceImplementationInstruction(
					ccip_router.SetPoolInterfaceImplementationParams{
						Mint:              pool.Mint,
						InterfaceVersion:  2,
						AccountResolution: resolution,
					},
					config.RouterConfigPDA,
					pool.AdminRegistryPDA,
					testutils.Token0Admin.PublicKey(),
					config.RouterEventAuthorityPDA,
					config.CcipRouterProgram,
				)
				require.NoError(t, ixErr)
				testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixInterface}, testutils.Token0Admin, config.DefaultCommitment)
			}

			// ccip_send_v2 pays the pool's fee receipt into the pool's fee vault ATA for the fee
			// token. The vault is qualified by the managed mint, so this fresh mint needs its own.
			feeVault, _, err := state.FindFeeVaultPDA(config.CcipBurnMintTokenPoolProgram, pool.Mint.Bytes())
			require.NoError(t, err)
			ixVaultAta, _, err := tokens.CreateAssociatedTokenAccount(env.WSOL.Program, env.WSOL.Mint, feeVault, legacyAdmin.PublicKey())
			require.NoError(t, err)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixVaultAta}, legacyAdmin, config.DefaultCommitment)

			return pool
		}

		testcases := []struct {
			name string
			// lookupTableExtras returns the entries the pool's lookup table carries after the
			// required ones. Nil means none.
			lookupTableExtras func(multisig solana.PublicKey) []solana.PublicKey
			resolution        ccip_router.ResolutionType
		}{
			{
				// A token onboarded on 2.0 has nothing after the required entries, so nothing
				// shifts the pool's resolved accounts in either direction.
				name:       "onboarded on 2.0 with no lookup table extras",
				resolution: ccip_router.ResolutionType_Standard,
			},
			{
				// A token migrated from 1.6 keeps the multisig in its lookup table, because its 1.6
				// lanes find it nowhere else, and its registry has 2.0 lanes skip the extras.
				name:              "migrated from 1.6 with the multisig extra skipped on 2.0",
				lookupTableExtras: func(multisig solana.PublicKey) []solana.PublicKey { return []solana.PublicKey{multisig} },
				resolution:        ccip_router.ResolutionType_WithoutExtendedLookupTable,
			},
		}

		for _, tc := range testcases {
			t.Run(tc.name, func(t *testing.T) {
				pool := setupMultisigPool(t, tc.lookupTableExtras, tc.resolution)

				t.Run("ccip_send_v2 burns the tokens", func(t *testing.T) {
					userTokenAccount := pool.User[user.PublicKey()]
					_, initBal, err := tokens.TokenBalance(ctx, solanaGoClient, userTokenAccount, config.DefaultCommitment)
					require.NoError(t, err)

					message := ccip_router.Svm2AnyMessage{
						FeeToken:     env.WSOL.Mint,
						Receiver:     env.ValidEvmRawReceiverAddress[:],
						Data:         []byte{7, 8, 9},
						ExtraArgs:    []byte{},
						TokenAmounts: []ccip_router.SvmTokenAmount{{Token: pool.Mint, Amount: 1}},
					}
					resolved, err := ccip.ResolveCcipSendV2Accounts(ctx, solanaGoClient, config.CcipRouterProgram, config.EvmChainSelector, message, user.PublicKey(), logger.Test(t))
					require.NoError(t, err)

					// This suite never sets the router's onramp lookup table, so resolution doesn't
					// return it. Attach it here so the send fits in a transaction.
					lookupTables := resolveLutMap(t, resolved.LookupTables)
					maps.Copy(lookupTables, env.CcipSendLookupTable)

					ixApprove, err := tokens.TokenApproveChecked(1, 9, pool.Program, userTokenAccount, pool.Mint, config.BillingSignerPDA, user.PublicKey(), nil)
					require.NoError(t, err)

					result := testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{ixApprove, resolved.Ix}, user, config.DefaultCommitment, lookupTables, common.AddComputeUnitLimit(500_000))
					require.NotNil(t, result)

					tx, err := result.Transaction.GetTransaction()
					require.NoError(t, err)
					burnEvent, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipBurnMintTokenPoolProgram, base_token_pool.ParseEvent_LockedOrBurned)
					require.NoError(t, err)
					require.Equal(t, pool.Mint, burnEvent.Mint)
					require.Equal(t, uint64(1), burnEvent.Amount)

					_, finalBal, err := tokens.TokenBalance(ctx, solanaGoClient, userTokenAccount, config.DefaultCommitment)
					require.NoError(t, err)
					require.Equal(t, 1, initBal-finalBal)
				})

				t.Run("execute_v2 mints through the multisig", func(t *testing.T) {
					receiver := solana.NewWallet().PublicKey()
					receiverAta, _, err := solana.FindAssociatedTokenAddress(receiver, pool.Mint)
					require.NoError(t, err)

					encodedMessage, messageID := buildTokenTransferMsg(t, pool.Mint, v2SourcePoolAddressFull, receiver, ccip_common.SvmTokenReceiverUsage_DeriveAtaAndCreate)
					verifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
					require.NoError(t, err)

					execInputs := ccip_offramp.ExecutionInputsV2{
						EncodedMessage:  encodedMessage,
						Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
						VerifierResults: [][]byte{verifierResults},
					}
					resolved := resolveExecAccountsWithAutoBuffering(t, execInputs, user)

					// Fails today with InvalidMultisig: the pool's resolver returns only its own
					// #[event_cpi] accounts, so the multisig never reaches release_or_mint.
					result := testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, user, config.DefaultCommitment, resolveLutMap(t, resolved.LookupTables), common.AddComputeUnitLimit(400_000))
					require.NotNil(t, result)

					tx, err := result.Transaction.GetTransaction()
					require.NoError(t, err)
					mintEvent, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipBurnMintTokenPoolProgram, base_token_pool.ParseEvent_ReleasedOrMinted)
					require.NoError(t, err)
					require.Equal(t, receiver, mintEvent.Recipient)
					require.Equal(t, uint64(1), mintEvent.Amount)

					_, balance, err := tokens.TokenBalance(ctx, solanaGoClient, receiverAta, config.DefaultCommitment)
					require.NoError(t, err)
					require.Equal(t, 1, balance)
				})
			})
		}
	})

	// Pool-side rejections on the inbound path. These live here rather than in tokenpool_test.go
	// because they are only meaningful when the pool is driven by the real OffRamp CPI: a mock ramp
	// can reach the same error codes, but it proves nothing about what a real execution does.
	t.Run("token v2 bnm (pool rejections)", func(t *testing.T) {
		v2Token := setupV2BurnmintPool(t)

		// setInboundRateLimit installs the pool's default inbound bucket for the lane and leaves it
		// FULL and frozen: the first write uses a refill rate equal to the capacity so the elapsed
		// second tops the bucket up, the second pins the rate to 0 so nothing refills mid-test.
		// Refill is driven by the on-chain unix timestamp in whole seconds, so without the freeze
		// the assertions below would race the wall clock.
		setInboundRateLimit := func(t *testing.T, capacity uint64) {
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
				testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, testutils.Token0Admin, config.DefaultCommitment)
			}
			time.Sleep(time.Second)
			set(capacity)
			set(0)
		}

		// execTokenTransfer runs a full ExecuteV2 for a pure token transfer of destAmount base
		// units from sourcePool to a fresh wallet (fresh so every call has a distinct message id and
		// its own ATA). An empty wantErr asserts the execution succeeds.
		execTokenTransfer := func(t *testing.T, sourcePool protocol.ByteSlice, destAmount uint64, wantErr string) {
			t.Helper()

			encodedMessage, messageID := buildTokenTransferMsgOfSize(
				t, v2Token.Mint, sourcePool, solana.NewWallet().PublicKey(),
				ccip_common.SvmTokenReceiverUsage_DeriveAtaAndCreate, destAmount,
			)
			verifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
			require.NoError(t, err)

			execInputs := ccip_offramp.ExecutionInputsV2{
				EncodedMessage:  encodedMessage,
				Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
				VerifierResults: [][]byte{verifierResults},
			}

			resolved := resolveExecAccountsWithAutoBuffering(t, execInputs, user)
			lutMap := resolveLutMap(t, resolved.LookupTables)
			ixs := []solana.Instruction{resolved.Ix}

			if wantErr == "" {
				require.NotNil(t, testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient, ixs, user, config.DefaultCommitment, lutMap, common.AddComputeUnitLimit(400_000)))
				return
			}
			testutils.SendAndFailWithLookupTables(ctx, t, solanaGoClient, ixs, user, config.DefaultCommitment, lutMap, []string{wantErr}, common.AddComputeUnitLimit(400_000))
			// The execute reverted, so its buffer PDA is still around. Close it to keep the
			// account state clean for the subtests that follow.
			closeExecInputsBuffer(t, execInputs, user)
		}

		// The pool's inbound rate limiter has two distinct rejections: a request larger than the
		// bucket's capacity can never succeed (RLMaxCapacityExceeded, checked before any refill),
		// while a request that merely outruns the tokens currently in the bucket is
		// RLRateLimitReached.
		t.Run("the inbound rate limit rejects a transfer larger than the bucket capacity", func(t *testing.T) {
			setInboundRateLimit(t, 10)
			execTokenTransfer(t, v2SourcePoolAddressFull, 11, "max capacity exceeded")
		})

		t.Run("the inbound rate limit rejects a second transfer that drains the bucket", func(t *testing.T) {
			setInboundRateLimit(t, 10)
			// The bucket is frozen at 10 tokens, so 6 lands and the next 6 cannot.
			execTokenTransfer(t, v2SourcePoolAddressFull, 6, "")
			execTokenTransfer(t, v2SourcePoolAddressFull, 6, "rate limit reached")
		})

		// The source pool address travels in the message, so an execution claiming to come from a
		// pool the local pool has not registered for the lane must be rejected. This is the pool's
		// own check (`InvalidSourcePoolAddress`), reached here through a real execution.
		t.Run("a transfer from an unregistered source pool is rejected", func(t *testing.T) {
			setInboundRateLimit(t, 1000)
			// It has to be long enough to match the expected length of a full source pool address.
			// The test uses the tokenV2, which has a different remote pool
			execTokenTransfer(t, protocol.ByteSlice(config.EVMUsdcPoolAddressBytes), 1, "InvalidSourcePoolAddress")
		})
	})

	// Lombard is registered with WithoutExtendedLookupTable in deployment (deploy_lombard_chain.go):
	// it resolves its own accounts, so the OffRamp must leave the lookup table's additional entries
	// out of a 2.0 execution while still running the pool's nested resolution. Resolution reads only
	// the registry, the table and the pool's State, so this sets up nothing the pool would need to
	// actually release tokens.
	t.Run("token v2 lombard resolves without the lookup table extras", func(t *testing.T) {
		mintKey := solana.NewWallet().PrivateKey
		lombardPool, err := tokens.NewTokenPool(config.SPLTokenProgram, config.LombardTokenPoolProgram, mintKey.PublicKey())
		require.NoError(t, err)
		ixToken, err := tokens.CreateTokenFrom(ctx, lombardPool, testutils.Token0Admin.PublicKey(), 9, solanaGoClient, config.DefaultCommitment)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, ixToken, testutils.Token0Admin, config.DefaultCommitment, common.AddSigners(mintKey))

		programDataAddress := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, config.LombardTokenPoolProgram)
		ixInitGlobal, err := lombard_token_pool.NewInitGlobalConfigInstruction(
			config.LombardConfigPDA, testutils.Admin.PublicKey(), solana.SystemProgramID, config.LombardTokenPoolProgram, programDataAddress,
		)
		require.NoError(t, err)
		ixInitPool, err := lombard_token_pool.NewInitializeInstruction(
			config.CcipRouterProgram, config.RMNRemoteProgram, config.MockLombardBridgeProgram,
			lombardPool.PoolConfig, lombardPool.Mint, testutils.Admin.PublicKey(), config.LombardTokenPoolProgram,
			programDataAddress, config.LombardConfigPDA, solana.SystemProgramID,
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixInitGlobal, ixInitPool}, testutils.Admin, config.DefaultCommitment)

		// SetPool checks the table's pool token account entry against the pool signer's ATA.
		ixPoolAta, poolAta, err := tokens.CreateAssociatedTokenAccount(lombardPool.Program, lombardPool.Mint, lombardPool.PoolSigner, testutils.Token0Admin.PublicKey())
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixPoolAta}, testutils.Token0Admin, config.DefaultCommitment)
		lombardPool.PoolTokenAccount = poolAta

		// An extra the table carries past its required entries, which only Standard resolution forwards.
		extra := solana.NewWallet().PublicKey()
		lombardPool.AdditionalAccounts = append(lombardPool.AdditionalAccounts, extra)
		require.NoError(t, lombardPool.SetupLookupTable(ctx, solanaGoClient, testutils.Token0Admin))
		registerPoolWithRouter(t, lombardPool, 2, ccip_router.ResolutionType_WithoutExtendedLookupTable)

		encodedMessage, _ := buildTokenTransferMsg(t, lombardPool.Mint, v2SourcePoolAddressFull, solana.NewWallet().PublicKey(), ccip_common.SvmTokenReceiverUsage_DeriveAtaDontCreate)
		// No CCVs, so resolution ends right after the token stages.
		execInputs := ccip_offramp.ExecutionInputsV2{
			EncodedMessage:  encodedMessage,
			Ccvs:            []solana.PublicKey{},
			VerifierResults: [][]byte{},
		}
		lombardEventAuthority, _, err := state.FindEventAuthorityPDA(config.LombardTokenPoolProgram)
		require.NoError(t, err)

		resolvedKeys := func(t *testing.T) []solana.PublicKey {
			resolved := resolveExecAccountsWithAutoBuffering(t, execInputs, user)
			closeExecInputsBuffer(t, execInputs, user)
			keys := make([]solana.PublicKey, 0, len(resolved.AccountsToSave))
			for _, acc := range resolved.AccountsToSave {
				keys = append(keys, acc.PublicKey)
			}
			return keys
		}

		keys := resolvedKeys(t)
		require.NotContains(t, keys, extra, "WithoutExtendedLookupTable must skip the table's additional entries")
		// The pool's nested resolution still ran: these come from its release_or_mint_start.
		require.Contains(t, keys, lombardEventAuthority)
		require.Contains(t, keys, config.LombardTokenPoolProgram)

		// Flipping the same registry to Standard brings the extra back, so the absence above is down
		// to the resolution mode rather than to how the table was built.
		ixStandard, err := ccip_router.NewSetPoolInterfaceImplementationInstruction(
			ccip_router.SetPoolInterfaceImplementationParams{
				Mint:              lombardPool.Mint,
				InterfaceVersion:  2,
				AccountResolution: ccip_router.ResolutionType_Standard,
			},
			config.RouterConfigPDA,
			lombardPool.AdminRegistryPDA,
			testutils.Token0Admin.PublicKey(),
			config.RouterEventAuthorityPDA,
			config.CcipRouterProgram,
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixStandard}, testutils.Token0Admin, config.DefaultCommitment)

		keys = resolvedKeys(t)
		require.Contains(t, keys, extra)
		require.Contains(t, keys, lombardEventAuthority)
	})

	t.Run("token v2 cctp", func(t *testing.T) {
		verifierVersionTag := [4]byte{0x91, 0xb3, 0x33, 0x8e} // bytes4(keccak256("CCTPVerifier 2.1.0"))
		poolVersionTag := [4]byte{0x30, 0x47, 0x58, 0x7c}     // bytes4(keccak256("CCTP_V2_CCV"))

		transferAmount := uint64(321)

		receiverATA := env.UsdcPool.User[user.PublicKey()]

		// A zero receive caller would let anyone consume the CCTP message on the destination.
		t.Run("the verifier rejects zero CCTP allowed callers", func(t *testing.T) {
			for name, cctp := range map[string]cctp_token_pool.RemoteCctpConfig{
				"receive": {Enabled: true, DomainIdentifier: env.CctpEvmDomain, AllowedCallerDeposit: solana.PublicKey(config.EVMUsdcPoolAddressBytes)},
				"deposit": {Enabled: true, DomainIdentifier: env.CctpEvmDomain, AllowedCallerReceive: solana.PublicKey(config.EVMCctpMessageTransmitterProxy)},
			} {
				t.Run(name, func(t *testing.T) {
					ix, err := cctp_token_pool.NewSetVerifierChainConfigCctpInstruction(
						cctp_token_pool.SetVerifierChainConfigCctpParams{RemoteChainSelector: config.EvmChainSelector, Cctp: cctp},
						config.CctpVerifierConfigPDA,
						testutils.Admin.PublicKey(),
						config.CctpVerifierEvmRemoteChainConfigPDA,
						solana.SystemProgramID,
						config.CctpEventAuthorityPDA,
						config.CctpTokenPoolProgram,
					)
					require.NoError(t, err)
					testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{ix}, testutils.Admin, config.DefaultCommitment, []string{"Error Code: CctpAllowedCallersInvalidZero"})
				})
			}
		})

		type execOptions struct {
			FTF bool
		}

		cctpCurrentNonce := uint8(1)

		makeExecInputs := func(options execOptions) (ccip_offramp.ExecutionInputsV2, cctp.Message) {
			receiver := user.PublicKey()
			sourceTokenAddress := config.EVMUsdcAddressBytesTrim
			destTokenAddress := protocol.ByteSlice(env.UsdcPool.Mint.Bytes())
			// The source pool advertises its token decimals as a 32-byte big-endian value in extra_data.
			// v2 pools reject a transfer whose extra_data disagrees with their remote chain config, which
			// every InitChainRemoteConfig below registers as remoteTokenDecimals.
			extraData := protocol.ByteSlice(poolVersionTag[:])

			amount := ccip_offramp.ProtocolAmount{BeBytes: tokens.ToBigEndianU256(transferAmount)}
			tokenTransfer := protocol.TokenTransfer{
				Amount:                   new(big.Int).SetBytes(amount.BeBytes[:]),
				SourcePoolAddress:        config.EVMUsdcPoolAddressBytes, // 32-bytes here - the offramp normalizes to 20 for EVM chains later
				SourceTokenAddress:       sourceTokenAddress,
				DestTokenAddress:         destTokenAddress,
				TokenReceiver:            protocol.ByteSlice(receiver[:]),
				ExtraData:                extraData,
				Version:                  1,
				SourcePoolAddressLength:  uint8(len(config.EVMUsdcPoolAddressBytes)), //nolint:gosec // G115 always < 256
				SourceTokenAddressLength: uint8(len(sourceTokenAddress)),             //nolint:gosec // G115 always < 256
				DestTokenAddressLength:   32,
				TokenReceiverLength:      32,
				ExtraDataLength:          uint16(len(extraData)), //nolint:gosec // G115 always < 256
			}
			tokenTransferLength := uint16(len(tokenTransfer.SourcePoolAddress)) + uint16(len(sourceTokenAddress)) + uint16(len(destTokenAddress)) + 32 + uint16(len(extraData)) //nolint:gosec // G115

			destBlob := ccip.SVMExecutorArgs{SvmDestBlob: ccip_common.SvmDestBlob{
				UseAta: ccip_common.SvmTokenReceiverUsage_DeriveAtaDontCreate,
			}}
			destBlobBytes, err := destBlob.BorshEncode()
			require.NoError(t, err)

			msg := testutils.MinimalMessageV1(config.EvmChainSelector)
			msg.Receiver = solana.SystemProgramID[:] // no receiver, pure token transfer
			msg.TokenTransfer = &tokenTransfer
			msg.TokenTransferLength = tokenTransferLength
			msg.DestBlob = destBlobBytes
			msg.DestBlobLength = uint16(len(destBlobBytes)) //nolint:gosec // G115

			if options.FTF {
				// Modify tokenTransfer for FTF scenario
				msg.Finality = protocol.NewFinality().WithBlockDepth(2)
			}

			encodedMessage, err := msg.Encode()
			require.NoError(t, err)
			var messageID [32]byte
			copy(messageID[:], eth.Keccak256(encodedMessage))

			hookData := make([]byte, len(verifierVersionTag)+len(messageID))
			copy(hookData[:4], verifierVersionTag[:])
			copy(hookData[4:], messageID[:])

			cctpMessage := cctp.Message{
				Version:                   1,
				SourceDomain:              env.CctpEvmDomain,
				DestinationDomain:         uint32(5), // Solana is always 5
				Nonce:                     [32]byte{cctpCurrentNonce},
				Sender:                    [32]byte(config.EVMCctpTokenMessenger),
				Recipient:                 config.CctpTokenMessengerMinterV2,
				DestinationCaller:         env.UsdcPool.PoolSigner,
				MinFinalityThreshold:      2000, // standard transfer
				FinalityThresholdExecuted: 2001,
				Body: cctp.MessageBody{
					Version:         1,
					BurnToken:       [32]byte(config.EVMUsdcAddressBytes),
					MintRecipient:   env.UsdcPool.PoolTokenAccount,
					Amount:          tokens.ToBigEndianU256(transferAmount),
					MessageSender:   [32]byte(config.EVMUsdcPoolAddressBytes),
					MaxFee:          [32]byte{}, // 0, no ftf
					FeeExecuted:     [32]byte{}, // 0, no ftf
					ExpirationBlock: [32]byte{}, // 0, no expiration
					HookData:        hookData,
				},
			}
			cctpCurrentNonce++

			if options.FTF {
				// See https://developers.circle.com/cctp/references/technical-guide#defined-finality-thresholds
				// and https://developers.circle.com/cctp/concepts/fees
				cctpMessage.Body.MaxFee = tokens.ToBigEndianU256(100)
				cctpMessage.Body.FeeExecuted = tokens.ToBigEndianU256(100)
				cctpMessage.FinalityThresholdExecuted = 1000
				cctpMessage.MinFinalityThreshold = 1000
			}
			cctpMessageBytes := cctpMessage.Serialize()
			attestation, err := testutils.AttestCCTP(cctpMessageBytes, env.CctpAttesters)
			require.NoError(t, err)

			verifierResult := make([]byte, len(verifierVersionTag)+len(cctpMessageBytes)+len(attestation))
			copy(verifierResult[:len(verifierVersionTag)], verifierVersionTag[:])
			copy(verifierResult[len(verifierVersionTag):len(verifierVersionTag)+len(cctpMessageBytes)], cctpMessageBytes)
			copy(verifierResult[len(verifierVersionTag)+len(cctpMessageBytes):], attestation)

			return ccip_offramp.ExecutionInputsV2{
				EncodedMessage:  encodedMessage,
				Ccvs:            []solana.PublicKey{config.CctpTokenPoolProgram},
				VerifierResults: [][]byte{verifierResult},
			}, cctpMessage
		}

		t.Run("standard transfer", func(t *testing.T) {
			_, initBal, err := tokens.TokenBalance(ctx, solanaGoClient, receiverATA, config.DefaultCommitment)
			require.NoError(t, err)

			execInputs, _ := makeExecInputs(execOptions{FTF: false})
			resolved := resolveExecAccountsWithAutoBufferingInChunks(t, execInputs, user, 2)
			result := testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, user, config.DefaultCommitment, resolveLutMap(t, resolved.LookupTables), common.AddComputeUnitLimit(500_000))
			require.NotNil(t, result)

			_, finalBal, err := tokens.TokenBalance(ctx, solanaGoClient, receiverATA, config.DefaultCommitment)
			require.NoError(t, err)
			require.Equal(t, transferAmount, uint64(finalBal-initBal)) //nolint:gosec // the math here is safe with the values in this test
		})

		t.Run("ftf transfer", func(t *testing.T) {
			_, initBal, err := tokens.TokenBalance(ctx, solanaGoClient, receiverATA, config.DefaultCommitment)
			require.NoError(t, err)

			execInputs, cctpMsg := makeExecInputs(execOptions{FTF: true})
			resolved := resolveExecAccountsWithAutoBufferingInChunks(t, execInputs, user, 2)
			result := testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, user, config.DefaultCommitment, resolveLutMap(t, resolved.LookupTables), common.AddComputeUnitLimit(500_000))
			require.NotNil(t, result)

			_, finalBal, err := tokens.TokenBalance(ctx, solanaGoClient, receiverATA, config.DefaultCommitment)
			require.NoError(t, err)
			require.Equal(t, transferAmount-bin.BE.Uint64(cctpMsg.Body.FeeExecuted[24:]), uint64(finalBal-initBal)) //nolint:gosec // the math here is safe with the values in this test
		})
	})

	t.Run("token v2 bnm (lane default ccvs)", func(t *testing.T) {
		// get_required_ccvs VIEW test (does NOT execute): lane default = [committee verifier],
		// lane-mandated = [a distinct dummy pubkey]. The view's RequiredCcvs must be both: the lane
		// default (committee verifier) plus the lane-mandated dummy appended. Order-independent.
		v2Token := setupV2BurnmintPool(t)

		laneMandatedCCV := solana.NewWallet().PublicKey()
		laneCfg := validSourceChainConfig
		laneCfg.DefaultCcvs = []solana.PublicKey{config.CommitteeVerifierProgram}
		laneCfg.LaneMandatedCcvs = []solana.PublicKey{laneMandatedCCV}
		updateEvmSourceChainConfig(t, laneCfg)
		defer updateEvmSourceChainConfig(t, validSourceChainConfig)

		encodedMessage, _ := buildTokenTransferMsg(t, v2Token.Mint, v2SourcePoolAddressFull, user.PublicKey(), ccip_common.SvmTokenReceiverUsage_DeriveAtaDontCreate)
		viewReturn := getRequiredCcvsView(t, encodedMessage)
		require.ElementsMatch(t, []solana.PublicKey{config.CommitteeVerifierProgram, laneMandatedCCV}, viewReturn.RequiredCcvs)
	})

	t.Run("token v2 bnm (ccvs required per pool)", func(t *testing.T) {
		// get_required_ccvs VIEW test (does NOT execute): the pool's set_ccv_config inbound is set to
		// [committee verifier], the lane DEFAULT is a distinct dummy (to prove the pool — not the
		// default — drives the set) and the lane-mandated is yet another dummy. The view's RequiredCcvs
		// must be [committee verifier (pool), dummy-mandated]: the pool-driven CCV plus the always-
		// appended lane-mandated; the dummy DEFAULT must NOT appear. Order-independent.
		v2Token := setupV2BurnmintPool(t)

		dummyDefault := solana.NewWallet().PublicKey()
		laneMandatedCCV := solana.NewWallet().PublicKey()
		laneCfg := validSourceChainConfig
		laneCfg.DefaultCcvs = []solana.PublicKey{dummyDefault}
		laneCfg.LaneMandatedCcvs = []solana.PublicKey{laneMandatedCCV}
		updateEvmSourceChainConfig(t, laneCfg)
		defer updateEvmSourceChainConfig(t, validSourceChainConfig)

		burnmintEventAuthority, _, _ := state.FindEventAuthorityPDA(config.CcipBurnMintTokenPoolProgram)
		// init_chain_config_v2 creates (or reuses) the fee vault ATA for the pool's own mint, the
		// account the lane's in-token BPS fee accrues into.
		burnmintFeeVault, _, err := state.FindFeeVaultPDA(config.CcipBurnMintTokenPoolProgram, v2Token.Mint.Bytes())
		require.NoError(t, err)
		burnmintFeeVaultATA, err := tokens.FindFeeVaultATA(v2Token.Program, v2Token.Mint, config.CcipBurnMintTokenPoolProgram, v2Token.Mint.Bytes())
		require.NoError(t, err)
		ixSetCcv, err := burnmint_tokenpool.NewInitChainConfigV2Instruction(
			burnmint_tokenpool.SetChainConfigV2Params{
				Mint:                   v2Token.Mint,
				RemoteChainSelector:    config.EvmChainSelector,
				TokenTransferFeeConfig: burnmint_tokenpool.TokenTransferFeeConfig{},
				CcvConfig: burnmint_tokenpool.CcvConfig{
					ThresholdAmount:      0,
					InboundCcvs:          []solana.PublicKey{config.CommitteeVerifierProgram},
					ThresholdInboundCcvs: []solana.PublicKey{},
				},
			},
			v2Token.PoolConfig,
			v2Token.ChainConfigV2[config.EvmChainSelector],
			testutils.Token0Admin.PublicKey(),
			solana.SystemProgramID,
			v2Token.Mint,
			burnmintFeeVault,
			burnmintFeeVaultATA,
			v2Token.Program,
			solana.SPLAssociatedTokenAccountProgramID,
			burnmintEventAuthority,
			config.CcipBurnMintTokenPoolProgram,
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixSetCcv}, testutils.Token0Admin, config.DefaultCommitment)

		encodedMessage, _ := buildTokenTransferMsg(t, v2Token.Mint, v2SourcePoolAddressFull, user.PublicKey(), ccip_common.SvmTokenReceiverUsage_DeriveAtaDontCreate)
		viewReturn := getRequiredCcvsView(t, encodedMessage)
		require.ElementsMatch(t, []solana.PublicKey{config.CommitteeVerifierProgram, laneMandatedCCV}, viewReturn.RequiredCcvs)
		require.NotContains(t, viewReturn.RequiredCcvs, dummyDefault)
	})

	// setupV2LockReleasePool creates, funds (lock-release way), registers (interface v2) and lane-
	// configures a v2 lock-release pool, returning it ready for view queries.
	setupV2LockReleasePool := func(t *testing.T) tokens.TokenPool {
		lnrMintPriv := solana.NewWallet().PrivateKey
		lnrToken, err := tokens.NewTokenPool(config.SPLTokenProgram, config.CcipBasePoolLockRelease, lnrMintPriv.PublicKey())
		require.NoError(t, err)

		// Token0Admin keeps mint authority (lock/release never mints). Liquidity is added the idiomatic
		// lock-release way below: mint into the rebalancer's ATA, then provide_liquidity pulls it into
		// the pool token account.
		ixCreate, err := tokens.CreateTokenFrom(ctx, lnrToken, testutils.Token0Admin.PublicKey(), 9, solanaGoClient, config.DefaultCommitment)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, ixCreate, testutils.Token0Admin, config.DefaultCommitment, common.AddSigners(lnrMintPriv))

		programDataAddress := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, config.CcipBasePoolLockRelease)
		globalConfigPDA, err := tokens.TokenPoolGlobalConfigPDA(config.CcipBasePoolLockRelease)
		require.NoError(t, err)

		// The shared env only initializes the burnmint global config; lockrelease's must be initialized
		// in-test because Initialize requires it (else AccountNotInitialized on config). Guarded so it
		// is set only once across the lnr subtests.
		var existingGlobalCfg lockrelease_tokenpool.PoolConfig
		if getErr := common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, globalConfigPDA, config.DefaultCommitment, &existingGlobalCfg); getErr != nil {
			ixGlobalConfig, gErr := lockrelease_tokenpool.NewInitGlobalConfigInstruction(
				config.CcipRouterProgram,
				config.RMNRemoteProgram,
				globalConfigPDA,
				testutils.Admin.PublicKey(),
				solana.SystemProgramID,
				config.CcipBasePoolLockRelease,
				programDataAddress,
			)
			require.NoError(t, gErr)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixGlobalConfig}, testutils.Admin, config.DefaultCommitment)
		}

		ixInit, err := lockrelease_tokenpool.NewInitializeInstruction(
			lnrToken.PoolConfig, lnrToken.Mint, testutils.Admin.PublicKey(), solana.SystemProgramID,
			config.CcipBasePoolLockRelease, programDataAddress, globalConfigPDA,
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixInit}, testutils.Admin, config.DefaultCommitment)

		ixTransfer, err := lockrelease_tokenpool.NewTransferOwnershipInstruction(
			testutils.Token0Admin.PublicKey(), lnrToken.PoolConfig, lnrToken.Mint,
			testutils.Admin.PublicKey(),
		)
		require.NoError(t, err)
		ixAccept, err := lockrelease_tokenpool.NewAcceptOwnershipInstruction(
			lnrToken.PoolConfig, lnrToken.Mint, testutils.Token0Admin.PublicKey(),
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixTransfer, ixAccept}, testutils.Admin, config.DefaultCommitment, common.AddSigners(testutils.Token0Admin))

		ixAtaPool, addrPool, err := tokens.CreateAssociatedTokenAccount(lnrToken.Program, lnrToken.Mint, lnrToken.PoolSigner, testutils.Token0Admin.PublicKey())
		require.NoError(t, err)
		lnrToken.PoolTokenAccount = addrPool
		lnrToken.User[lnrToken.PoolSigner] = addrPool
		// Rebalancer (Token0Admin) liquidity ATA, funded via Token0Admin's retained mint authority.
		ixRebalancerAta, rebalancerAta, err := tokens.CreateAssociatedTokenAccount(lnrToken.Program, lnrToken.Mint, testutils.Token0Admin.PublicKey(), testutils.Token0Admin.PublicKey())
		require.NoError(t, err)
		ixMintLiquidity, err := tokens.MintTo(1000000, lnrToken.Program, lnrToken.Mint, rebalancerAta, testutils.Token0Admin.PublicKey())
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixAtaPool, ixRebalancerAta, ixMintLiquidity}, testutils.Token0Admin, config.DefaultCommitment)

		// Provide liquidity the idiomatic lock-release way: set the rebalancer, approve the pool signer
		// as delegate over the rebalancer's ATA (provide_liquidity moves the tokens with the pool signer
		// as the SPL authority), then pull the liquidity into the pool token account.
		const liquidityAmount = 100000
		ixSetReb, err := lockrelease_tokenpool.NewSetRebalancerInstruction(
			testutils.Token0Admin.PublicKey(), lnrToken.PoolConfig, lnrToken.Mint,
			testutils.Token0Admin.PublicKey(),
		)
		require.NoError(t, err)
		ixApprove, err := tokens.TokenApproveChecked(liquidityAmount, 9, lnrToken.Program, rebalancerAta, lnrToken.Mint, lnrToken.PoolSigner, testutils.Token0Admin.PublicKey(), nil)
		require.NoError(t, err)
		ixProvide, err := lockrelease_tokenpool.NewProvideLiquidityInstruction(
			liquidityAmount, lnrToken.PoolConfig, lnrToken.Program, lnrToken.Mint, lnrToken.PoolSigner,
			lnrToken.PoolTokenAccount, rebalancerAta, testutils.Token0Admin.PublicKey(),
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixSetReb, ixApprove, ixProvide}, testutils.Token0Admin, config.DefaultCommitment)

		require.NoError(t, lnrToken.SetupLookupTable(ctx, solanaGoClient, testutils.Token0Admin))

		ensureCommitteeVerifierLane(t, 1)
		registerPoolWithRouter(t, lnrToken, 2, ccip_router.ResolutionType_Standard)

		// Remote config + allowed source pool address + rate limit.
		poolAddresses := []lockrelease_tokenpool.RemoteAddress{{Address: config.EVMRemotePoolAddressBytesTrim}}
		ixRemoteConfig, err := lockrelease_tokenpool.NewInitChainRemoteConfigInstruction(config.EvmChainSelector, lnrToken.Mint, lockrelease_tokenpool.RemoteConfig{TokenAddress: lockrelease_tokenpool.RemoteAddress{Address: config.EVMToken0AddressBytes}, Decimals: remoteTokenDecimals}, lnrToken.PoolConfig, lnrToken.Chain[config.EvmChainSelector], testutils.Token0Admin.PublicKey(), solana.SystemProgramID)
		require.NoError(t, err)
		ixAppendRemote, err := lockrelease_tokenpool.NewAppendRemotePoolAddressesInstruction(config.EvmChainSelector, lnrToken.Mint, poolAddresses, lnrToken.PoolConfig, lnrToken.Chain[config.EvmChainSelector], testutils.Token0Admin.PublicKey(), solana.SystemProgramID)
		require.NoError(t, err)
		ixRateLimit, err := lockrelease_tokenpool.NewSetChainRateLimitInstruction(config.EvmChainSelector, lnrToken.Mint, lockrelease_tokenpool.RateLimitConfig{}, lockrelease_tokenpool.RateLimitConfig{}, lnrToken.PoolConfig, lnrToken.Chain[config.EvmChainSelector], testutils.Token0Admin.PublicKey())
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixRemoteConfig, ixAppendRemote, ixRateLimit}, testutils.Token0Admin, config.DefaultCommitment)

		return lnrToken
	}

	t.Run("token v2 lnr (lane default ccvs)", func(t *testing.T) {
		// get_required_ccvs VIEW test (does NOT execute) for the lock-release pool: lane default =
		// [committee verifier], lane-mandated = [a distinct dummy pubkey]. RequiredCcvs == both
		// (order-independent).
		lnrToken := setupV2LockReleasePool(t)

		laneMandatedCCV := solana.NewWallet().PublicKey()
		laneCfg := validSourceChainConfig
		laneCfg.DefaultCcvs = []solana.PublicKey{config.CommitteeVerifierProgram}
		laneCfg.LaneMandatedCcvs = []solana.PublicKey{laneMandatedCCV}
		updateEvmSourceChainConfig(t, laneCfg)
		defer updateEvmSourceChainConfig(t, validSourceChainConfig)

		encodedMessage, _ := buildTokenTransferMsg(t, lnrToken.Mint, v2SourcePoolAddressFull, user.PublicKey(), ccip_common.SvmTokenReceiverUsage_DeriveAtaDontCreate)
		viewReturn := getRequiredCcvsView(t, encodedMessage)
		require.ElementsMatch(t, []solana.PublicKey{config.CommitteeVerifierProgram, laneMandatedCCV}, viewReturn.RequiredCcvs)
	})

	t.Run("token v2 lnr (ccvs required per pool)", func(t *testing.T) {
		// get_required_ccvs VIEW test (does NOT execute) for the lock-release pool: lockrelease's
		// set_ccv_config inbound = [committee verifier], lane DEFAULT = distinct dummy (proves the pool
		// drives), lane-mandated = another dummy. RequiredCcvs == [committee verifier (pool),
		// dummy-mandated]; the dummy DEFAULT must NOT appear. Order-independent.
		lnrToken := setupV2LockReleasePool(t)
		lnrEventAuthority, _, _ := state.FindEventAuthorityPDA(config.CcipBasePoolLockRelease)

		dummyDefault := solana.NewWallet().PublicKey()
		laneMandatedCCV := solana.NewWallet().PublicKey()
		laneCfg := validSourceChainConfig
		laneCfg.DefaultCcvs = []solana.PublicKey{dummyDefault}
		laneCfg.LaneMandatedCcvs = []solana.PublicKey{laneMandatedCCV}
		updateEvmSourceChainConfig(t, laneCfg)
		defer updateEvmSourceChainConfig(t, validSourceChainConfig)

		// As above: the fee vault ATA for the pool's own mint is where the in-token BPS fee lands,
		// kept apart from the locked liquidity in pool_token_account.
		lnrFeeVault, _, err := state.FindFeeVaultPDA(config.CcipBasePoolLockRelease, lnrToken.Mint.Bytes())
		require.NoError(t, err)
		lnrFeeVaultATA, err := tokens.FindFeeVaultATA(lnrToken.Program, lnrToken.Mint, config.CcipBasePoolLockRelease, lnrToken.Mint.Bytes())
		require.NoError(t, err)
		ixSetCcv, err := lockrelease_tokenpool.NewInitChainConfigV2Instruction(
			lockrelease_tokenpool.SetChainConfigV2Params{
				Mint:                   lnrToken.Mint,
				RemoteChainSelector:    config.EvmChainSelector,
				TokenTransferFeeConfig: lockrelease_tokenpool.TokenTransferFeeConfig{},
				CcvConfig: lockrelease_tokenpool.CcvConfig{
					InboundCcvs: []solana.PublicKey{config.CommitteeVerifierProgram},
				},
			},
			lnrToken.PoolConfig, lnrToken.ChainConfigV2[config.EvmChainSelector],
			testutils.Token0Admin.PublicKey(), solana.SystemProgramID,
			lnrToken.Mint, lnrFeeVault, lnrFeeVaultATA, lnrToken.Program, solana.SPLAssociatedTokenAccountProgramID,
			lnrEventAuthority, config.CcipBasePoolLockRelease,
		)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ixSetCcv}, testutils.Token0Admin, config.DefaultCommitment)

		encodedMessage, _ := buildTokenTransferMsg(t, lnrToken.Mint, v2SourcePoolAddressFull, user.PublicKey(), ccip_common.SvmTokenReceiverUsage_DeriveAtaDontCreate)
		viewReturn := getRequiredCcvsView(t, encodedMessage)
		require.ElementsMatch(t, []solana.PublicKey{config.CommitteeVerifierProgram, laneMandatedCCV}, viewReturn.RequiredCcvs)
		require.NotContains(t, viewReturn.RequiredCcvs, dummyDefault)
	})

	t.Run("token v2 lnr (pool ATA as token receiver)", func(t *testing.T) {
		lnrToken := setupV2LockReleasePool(t)

		encodedMessage, messageID := buildTokenTransferMsg(t, lnrToken.Mint, v2SourcePoolAddressFull, lnrToken.PoolTokenAccount, ccip_common.SvmTokenReceiverUsage_UseAsIs)
		verifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
		require.NoError(t, err)
		execInputs := ccip_offramp.ExecutionInputsV2{
			EncodedMessage:  encodedMessage,
			Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
			VerifierResults: [][]byte{verifierResults},
		}
		// Buffer the inputs: with a real receiver the message carries the receiver CPI accounts,
		// and buffering keeps the execute transaction under the size limit (the path a real
		// executor uses for large payloads).
		resolved := resolveExecAccountsWithAutoBuffering(t, execInputs, user)

		balanceBefore := env.GetBalance(lnrToken.PoolTokenAccount)
		testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, user, config.DefaultCommitment, resolveLutMap(t, resolved.LookupTables), common.AddComputeUnitLimit(400_000))
		balanceAfter := env.GetBalance(lnrToken.PoolTokenAccount)

		require.Equal(t, balanceBefore, balanceAfter, "when transferring to the lnr pool directly, there should be no balance change")
	})

	// ---------------------------------------------------------------------------------------------
	// RECEIVER-CPI tests. These exercise the offramp -> receiver `ccip_receive` CPI (the real
	// deployed receiver is test_ccip_receiver). Its counter PDA is shared across the suite, so each
	// case asserts a delta rather than an absolute value.
	t.Run("ExecuteV2 receiver CPI", func(t *testing.T) {
		ensureCommitteeVerifierLane(t, 1)

		readReceiverCounter := func(t *testing.T) uint64 {
			var counter test_ccip_receiver.Counter
			err := common.GetAccountDataBorshIntoAnchor(ctx, solanaGoClient, config.ReceiverTargetAccountPDA, config.DefaultCommitment, &counter)
			require.NoError(t, err)
			return counter.Value
		}

		execMessage := func(t *testing.T, encodedMessage []byte, messageID [32]byte) *rpc.GetTransactionResult {
			verifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
			require.NoError(t, err)
			execInputs := ccip_offramp.ExecutionInputsV2{
				EncodedMessage:  encodedMessage,
				Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
				VerifierResults: [][]byte{verifierResults},
			}
			// Buffer the inputs: with a real receiver the message carries the receiver CPI accounts,
			// and buffering keeps the execute transaction under the size limit (the path a real
			// executor uses for large payloads).
			resolved := resolveExecAccountsWithAutoBuffering(t, execInputs, user)
			return testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, user, config.DefaultCommitment, resolveLutMap(t, resolved.LookupTables), common.AddComputeUnitLimit(400_000))
		}

		extractTestReceiverLogs := func(t *testing.T, result *rpc.GetTransactionResult) []string {
			t.Helper()

			foundReceiverCPI := false
			var logs []string
			for _, msg := range result.Meta.LogMessages {
				if msg == "Program log: Instruction: CcipReceive" {
					foundReceiverCPI = true
				}
				if foundReceiverCPI {
					logs = append(logs, msg)
				}
			}
			return logs
		}

		t.Run("message-only invokes receiver with empty token amounts", func(t *testing.T) {
			before := readReceiverCounter(t)

			// MinimalMessageV1 already targets the real receiver with its CPI accounts.
			msg := testutils.MinimalMessageV1(config.EvmChainSelector)
			msg.SequenceNumber = 7001
			encodedMessage, err := msg.Encode()
			require.NoError(t, err)
			var messageID [32]byte
			copy(messageID[:], eth.Keccak256(encodedMessage))

			result := execMessage(t, encodedMessage, messageID)
			require.NotNil(t, result)

			require.Equal(t, before+1, readReceiverCounter(t), "receiver ccip_receive must have incremented the counter")
			require.Contains(t, strings.Join(extractTestReceiverLogs(t, result), "\n"),
				"token_amounts: []", "a message-only CPI must forward empty token_amounts")
		})

		t.Run("message with token transfer forwards the delivered token amount", func(t *testing.T) {
			v2Token := setupV2BurnmintPool(t)
			before := readReceiverCounter(t)

			// DeriveAtaAndCreate to a fresh wallet: the execute both delivers the token and CPIs the
			// receiver, so the receiver sees token_amounts=[{mint, 1}].
			tokenReceiver := solana.NewWallet().PublicKey()

			encodedMessage, messageID := buildTokenTransferMsgCore(
				t, v2Token.Mint, v2SourcePoolAddressFull, tokenReceiver,
				ccip_common.SvmTokenReceiverUsage_DeriveAtaAndCreate,
				config.CcipLogicReceiver, receiverCPIMessagingAccounts, receiverCPIMessagingBitmap, 1000000000,
			)

			result := execMessage(t, encodedMessage, messageID)
			require.NotNil(t, result)

			// Token was delivered to the wallet's ATA.
			tx, err := result.Transaction.GetTransaction()
			require.NoError(t, err)
			mintEvent, err := common.ExtractCPIEvent(tx, result.Meta, config.CcipBurnMintTokenPoolProgram, base_token_pool.ParseEvent_ReleasedOrMinted)
			require.NoError(t, err)
			require.Equal(t, tokenReceiver, mintEvent.Recipient)
			require.Equal(t, uint64(1), mintEvent.Amount)
			require.Equal(t, v2Token.Mint, mintEvent.Mint)

			// Receiver was invoked and received the delivered token amount in its Any2SVMMessage.
			require.Equal(t, before+1, readReceiverCounter(t))
			require.Contains(t, strings.Join(extractTestReceiverLogs(t, result), "\n"), v2Token.Mint.String(),
				"receiver message must include the delivered token mint in token_amounts")
		})

		t.Run("reverts when the receiver rejects", func(t *testing.T) {
			// Flip the receiver into reject-all (owner-only), then restore it so later cases and reruns
			// still pass. The counter/reject_all PDA is shared, so the reset is mandatory.
			setReject := func(t *testing.T, reject bool) {
				ix, err := test_ccip_receiver.NewSetRejectAllInstruction(reject, config.ReceiverTargetAccountPDA, user.PublicKey())
				require.NoError(t, err)
				testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment)
			}
			setReject(t, true)
			defer setReject(t, false)

			msg := testutils.MinimalMessageV1(config.EvmChainSelector)
			msg.SequenceNumber = 7003
			encodedMessage, err := msg.Encode()
			require.NoError(t, err)
			var messageID [32]byte
			copy(messageID[:], eth.Keccak256(encodedMessage))

			verifierResults, err := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
			require.NoError(t, err)
			execInputs := ccip_offramp.ExecutionInputsV2{
				EncodedMessage:  encodedMessage,
				Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
				VerifierResults: [][]byte{verifierResults},
			}
			resolved := resolveExecAccountsWithAutoBuffering(t, execInputs, user)

			// The receiver's RejectAll error propagates through the CPI and reverts the whole execute.
			result := testutils.SendAndFailWithLookupTables(
				ctx, t, solanaGoClient, []solana.Instruction{resolved.Ix}, user, config.DefaultCommitment,
				resolveLutMap(t, resolved.LookupTables), []string{"Rejecting all messages"}, common.AddComputeUnitLimit(400_000),
			)
			require.NotNil(t, result)
		})
	})

	// Exercise the OffRamp -> v2-receiver integration: the OffRamp queries the v2 receiver's `get_ccvs_and_finality_config` via CPI,
	// folds the result into the required-CCV set + quorum, and dispatches delivery to `ccip_receive_v2`.
	// The v1 path is unchanged and covered by the "ExecuteV2 receiver CPI" tests above (an
	// unregistered receiver is treated as v1, so those tests need no registration).
	//
	// The OffRamp selects the v1/v2 flow from the router-owned ReceiverRegistry PDA, so
	// this block registers the receiver as v2 (via its `register_with_router` CPI, authorized by the
	// receiver program's upgrade authority) before exercising the v2 path.
	t.Run("ExecuteV2 receiver v2", func(t *testing.T) {
		ensureCommitteeVerifierLane(t, 1)

		v2 := config.CcipBaseReceiverV2
		receiverConfigPDA, _, _ := solana.FindProgramAddress([][]byte{[]byte("config")}, v2)
		eventAuthorityPDA, _, _ := state.FindEventAuthorityPDA(v2)
		ccvConfigPDA, _, _ := solana.FindProgramAddress(
			[][]byte{[]byte("ccv_config"), common.Uint64ToLE(config.EvmChainSelector)}, v2)

		senderBytes := []byte(testutils.MinimalMessageV1(config.EvmChainSelector).Sender)
		approvedSenderPDA, findErr := state.FindApprovedSenderV2(config.EvmChainSelector, senderBytes, v2)
		require.NoError(t, findErr)

		// One-time v2-receiver setup: initialize (router=CcipRouter) and approve the message's source
		// sender so `ccip_receive_v2`'s approved_sender gate passes. Only the program's upgrade
		// authority may initialize it, so Admin does and hands ownership to the user.
		programData := testutils.GetProgramDataAddress(ctx, t, solanaGoClient, v2)
		userInitIx, err := example_ccip_receiver_v2.NewInitializeInstruction(
			config.CcipRouterProgram, receiverConfigPDA, user.PublicKey(), solana.SystemProgramID, v2, programData)
		require.NoError(t, err)
		testutils.SendAndFailWith(ctx, t, solanaGoClient, []solana.Instruction{userInitIx}, user, config.DefaultCommitment, []string{"Error Code: Unauthorized"})

		initIx, err := example_ccip_receiver_v2.NewInitializeInstruction(
			config.CcipRouterProgram, receiverConfigPDA, testutils.Admin.PublicKey(), solana.SystemProgramID, v2, programData)
		require.NoError(t, err)
		transferOwnershipIx, err := example_ccip_receiver_v2.NewTransferOwnershipInstruction(user.PublicKey(), receiverConfigPDA, testutils.Admin.PublicKey())
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{initIx, transferOwnershipIx}, testutils.Admin, config.DefaultCommitment)
		acceptOwnershipIx, err := example_ccip_receiver_v2.NewAcceptOwnershipInstruction(receiverConfigPDA, user.PublicKey())
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{acceptOwnershipIx}, user, config.DefaultCommitment)

		approveIx, err := example_ccip_receiver_v2.NewApproveSenderInstruction(
			example_ccip_receiver_v2.SenderParams{ChainSelector: config.EvmChainSelector, RemoteSender: senderBytes},
			receiverConfigPDA, approvedSenderPDA, user.PublicKey(), solana.SystemProgramID)
		require.NoError(t, err)
		testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{approveIx}, user, config.DefaultCommitment)

		// ReceiverRegistry wiring. Admin is the upgrade authority of the deployed
		// receiver program, so it may drive `register_with_router` (which CPIs the router's
		// `register_receiver`, signing with the receiver's registration PDA).
		receiverRegistryPDA, _, regErr := state.FindReceiverRegistryPDA(v2, config.CcipRouterProgram)
		require.NoError(t, regErr)
		registrationSignerPDA, _, regErr := state.FindReceiverRegistrationSignerPDA(v2, config.CcipRouterProgram)
		require.NoError(t, regErr)
		routerEventAuthorityPDA, _, regErr := state.FindEventAuthorityPDA(config.CcipRouterProgram)
		require.NoError(t, regErr)

		registerReceiverV2 := func(t *testing.T) *rpc.GetTransactionResult {
			t.Helper()
			ix, ixErr := example_ccip_receiver_v2.NewRegisterWithRouterInstruction(
				receiverConfigPDA,
				user.PublicKey(),
				registrationSignerPDA,
				config.CcipRouterProgram,
				receiverRegistryPDA,
				config.RouterConfigPDA,
				routerEventAuthorityPDA,
				solana.SystemProgramID,
			)
			require.NoError(t, ixErr)
			return testutils.SendAndConfirm(ctx, t, solanaGoClient,
				[]solana.Instruction{ix}, user, config.DefaultCommitment)
		}

		// Register as v2 up front so the v2-flow subtests below see the registry. The dedicated
		// "register_with_router" subtest re-runs it (idempotent) to assert the emitted event + state.
		registerReceiverV2(t)

		t.Run("register_with_router (upgrade authority) emits event and writes v2 registry", func(t *testing.T) {
			result := registerReceiverV2(t)

			tx, txErr := result.Transaction.GetTransaction()
			require.NoError(t, txErr)
			evt, evtErr := common.ExtractCPIEvent(tx, result.Meta, config.CcipRouterProgram,
				ccip_router.ParseEvent_ReceiverRegistered)
			require.NoError(t, evtErr)
			require.Equal(t, v2, evt.ReceiverProgram)
			require.Equal(t, uint8(2), evt.ReceiverVersion)

			var registry ccip_common.ReceiverRegistry
			require.NoError(t, common.GetAccountDataBorshIntoAnchor(
				ctx, solanaGoClient, receiverRegistryPDA, config.DefaultCommitment, &registry))
			require.Equal(t, uint8(1), registry.Version)
			require.Equal(t, v2, registry.ReceiverProgram)
			require.Equal(t, uint8(2), registry.ReceiverVersion)
		})

		setCcvConfigWith := func(t *testing.T, required, optional []solana.PublicKey, optionalThreshold uint8) {
			ix, cfgErr := example_ccip_receiver_v2.NewSetCcvConfigInstruction(
				example_ccip_receiver_v2.SetCcvConfigParams{
					SourceChainSelector:   config.EvmChainSelector,
					RequiredCcvs:          required,
					OptionalCcvs:          optional,
					OptionalThreshold:     optionalThreshold,
					AllowedFinalityConfig: example_ccip_receiver_v2.FinalityConfig{}, // WAIT_FOR_FINALITY
				},
				receiverConfigPDA, ccvConfigPDA, user.PublicKey(), solana.SystemProgramID, eventAuthorityPDA, v2)
			require.NoError(t, cfgErr)
			testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{ix}, user, config.DefaultCommitment)
		}

		setCcvConfig := func(t *testing.T, required []solana.PublicKey) {
			setCcvConfigWith(t, required, []solana.PublicKey{}, 0)
		}

		// The `ccip_receive_v2` CPI accounts that follow the offramp-prepended [external_execution_
		// signer, offramp, allowed_offramp]: the receiver's approved_sender + state, then its
		// #[event_cpi] accounts (event_authority + program). All read-only, so the bitmap is 0.
		v2MessagingAccounts := []solana.PublicKey{approvedSenderPDA, receiverConfigPDA, eventAuthorityPDA, v2}

		buildV2MsgWith := func(t *testing.T, seq uint64, data []byte, ccipReceiveGasLimit uint32) ([]byte, [32]byte) {
			destBlob := ccip.SVMExecutorArgs{SvmDestBlob: ccip_common.SvmDestBlob{
				UseAta:                  ccip_common.SvmTokenReceiverUsage_DeriveAtaDontCreate,
				AccountIsWritableBitmap: 0,
				Accounts:                v2MessagingAccounts,
			}}
			destBlobBytes, encErr := destBlob.AbiEncodePacked()
			require.NoError(t, encErr)

			msg := testutils.MinimalMessageV1(config.EvmChainSelector)
			msg.SequenceNumber = protocol.SequenceNumber(seq)
			msg.Receiver = v2.Bytes()
			msg.DestBlob = destBlobBytes
			msg.DestBlobLength = uint16(len(destBlobBytes)) //nolint:gosec // G115 bounded
			msg.Data = data
			msg.DataLength = uint16(len(data)) //nolint:gosec // G115 bounded
			msg.CcipReceiveGasLimit = ccipReceiveGasLimit
			encoded, encodeErr := msg.Encode()
			require.NoError(t, encodeErr)
			var messageID [32]byte
			copy(messageID[:], eth.Keccak256(encoded))
			return encoded, messageID
		}

		buildV2Msg := func(t *testing.T, seq uint64) ([]byte, [32]byte) {
			return buildV2MsgWith(t, seq, []byte{0xde, 0xad, 0xbe, 0xef}, 300_000)
		}

		t.Run("v2 CCV query + dispatch succeeds and invokes ccip_receive_v2", func(t *testing.T) {
			setCcvConfig(t, []solana.PublicKey{config.CommitteeVerifierProgram})

			encoded, messageID := buildV2Msg(t, 7201)
			verifierResults, sErr := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
			require.NoError(t, sErr)
			execInputs := ccip_offramp.ExecutionInputsV2{
				EncodedMessage:  encoded,
				Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
				VerifierResults: [][]byte{verifierResults},
			}
			resolved := resolveExecAccountsWithAutoBuffering(t, execInputs, user)

			// Account resolution must include the dedicated receiver_ccv segment in the expected
			// order: [receiver_program, source_chain_ccv_config]. ccvConfigPDA appears only in that
			// segment, so assert the account immediately before it is the receiver program.
			accs := resolved.Ix.Accounts()
			ccvIdx := -1
			for i, a := range accs {
				if a.PublicKey == ccvConfigPDA {
					ccvIdx = i
					break
				}
			}
			require.GreaterOrEqual(t, ccvIdx, 1, "receiver ccv_config account must be present in resolution")
			require.Equal(t, v2, accs[ccvIdx-1].PublicKey,
				"receiver_ccv segment must be [receiver_program, source_chain_ccv_config] in order")

			result := testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient,
				[]solana.Instruction{resolved.Ix}, user, config.DefaultCommitment,
				resolveLutMap(t, resolved.LookupTables), common.AddComputeUnitLimit(400_000))
			require.NotNil(t, result)

			// The v2 receiver's ccip_receive_v2 emitted MessageReceived for this message.
			tx, txErr := result.Transaction.GetTransaction()
			require.NoError(t, txErr)
			evt, evtErr := common.ExtractCPIEvent(tx, result.Meta, v2, example_ccip_receiver_v2.ParseEvent_MessageReceived)
			require.NoError(t, evtErr)
			require.Equal(t, messageID, evt.MessageId)
		})

		t.Run("receiver-required CCV missing fails quorum", func(t *testing.T) {
			// Configure the receiver to require a CCV the executor does NOT supply. The receiver's
			// requirement must be folded into quorum, so execution fails before any verification.
			requiredCCV := solana.NewWallet().PublicKey()
			setCcvConfig(t, []solana.PublicKey{requiredCCV})
			defer setCcvConfig(t, []solana.PublicKey{config.CommitteeVerifierProgram}) // restore

			encoded, messageID := buildV2Msg(t, 7202)
			verifierResults, sErr := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
			require.NoError(t, sErr)
			execInputs := ccip_offramp.ExecutionInputsV2{
				EncodedMessage:  encoded,
				Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram}, // not the receiver-required CCV
				VerifierResults: [][]byte{verifierResults},
			}
			resolved := resolveExecAccountsWithAutoBuffering(t, execInputs, user)
			result := testutils.SendAndFailWithLookupTables(ctx, t, solanaGoClient,
				[]solana.Instruction{resolved.Ix}, user, config.DefaultCommitment,
				resolveLutMap(t, resolved.LookupTables),
				[]string{"Required CCV missing from execute input"}, common.AddComputeUnitLimit(400_000))
			require.NotNil(t, result)
		})

		t.Run("a receiver requiring only optional CCVs is not forced to take the lane defaults", func(t *testing.T) {
			// EVM parity: with no lane-mandated CCVs and no token, a receiver that requires no CCVs
			// but sets an optional threshold is satisfied by any one optional CCV. The lane default is
			// a CCV the executor does not supply, so pulling it in would fail quorum.
			updated := validSourceChainConfig
			updated.DefaultCcvs = []solana.PublicKey{solana.NewWallet().PublicKey()}
			updated.LaneMandatedCcvs = []solana.PublicKey{}
			updateEvmSourceChainConfig(t, updated)
			defer updateEvmSourceChainConfig(t, validSourceChainConfig)

			optional := []solana.PublicKey{config.CommitteeVerifierProgram, solana.NewWallet().PublicKey()}
			setCcvConfigWith(t, []solana.PublicKey{}, optional, 1)
			defer setCcvConfig(t, []solana.PublicKey{config.CommitteeVerifierProgram})

			encoded, messageID := buildV2Msg(t, 7203)

			// The view the executor consults must agree: nothing required, so a single optional
			// result is enough and the quorum is not classified as impossible.
			resolvedView, vErr := ccip.ResolveGetCCVsForMsgAccounts(ctx, solanaGoClient, config.CcipOfframpProgram, encoded, user.PublicKey(), logger.Test(t))
			require.NoError(t, vErr)
			viewResult := testutils.SendAndConfirm(ctx, t, solanaGoClient, []solana.Instruction{resolvedView.Ix}, user, config.DefaultCommitment)
			ccvs, rErr := common.ExtractAnchorTypedReturnValue[ccip_offramp.GetCcvsForMsgResponse](ctx, viewResult.Meta.LogMessages, ccip_offramp.ProgramID.String())
			require.NoError(t, rErr)
			require.Empty(t, ccvs.RequiredCcvs)
			require.ElementsMatch(t, optional, ccvs.OptionalCcvs)
			require.Equal(t, uint8(1), ccvs.OptionalThreshold)

			verifierResults, sErr := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
			require.NoError(t, sErr)
			execInputs := ccip_offramp.ExecutionInputsV2{
				EncodedMessage:  encoded,
				Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
				VerifierResults: [][]byte{verifierResults},
			}
			resolved := resolveExecAccountsWithAutoBuffering(t, execInputs, user)
			result := testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient,
				[]solana.Instruction{resolved.Ix}, user, config.DefaultCommitment,
				resolveLutMap(t, resolved.LookupTables), common.AddComputeUnitLimit(400_000))
			require.NotNil(t, result)

			tx, txErr := result.Transaction.GetTransaction()
			require.NoError(t, txErr)
			evt, evtErr := common.ExtractCPIEvent(tx, result.Meta, v2, example_ccip_receiver_v2.ParseEvent_MessageReceived)
			require.NoError(t, evtErr)
			require.Equal(t, messageID, evt.MessageId)
		})

		t.Run("a dataless message with zero callback gas skips the receiver entirely", func(t *testing.T) {
			// A receiver requiring a CCV nobody supplies: if the receiver were consulted, quorum
			// would fail. It is not, so the lane defaults alone govern and no callback is made.
			unsuppliedCCV := solana.NewWallet().PublicKey()
			setCcvConfig(t, []solana.PublicKey{unsuppliedCCV})
			defer setCcvConfig(t, []solana.PublicKey{config.CommitteeVerifierProgram})

			encoded, messageID := buildV2MsgWith(t, 7205, nil, 0)
			verifierResults, sErr := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
			require.NoError(t, sErr)
			execInputs := ccip_offramp.ExecutionInputsV2{
				EncodedMessage:  encoded,
				Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
				VerifierResults: [][]byte{verifierResults},
			}
			resolved := resolveExecAccountsWithAutoBuffering(t, execInputs, user)

			// The receiver_ccv segment is not resolved for a message the receiver never sees.
			for _, a := range resolved.Ix.Accounts() {
				require.NotEqual(t, ccvConfigPDA, a.PublicKey,
					"receiver ccv_config must not be resolved for a token-only message")
			}

			result := testutils.SendAndConfirmWithLookupTables(ctx, t, solanaGoClient,
				[]solana.Instruction{resolved.Ix}, user, config.DefaultCommitment,
				resolveLutMap(t, resolved.LookupTables), common.AddComputeUnitLimit(400_000))
			require.NotNil(t, result)

			tx, txErr := result.Transaction.GetTransaction()
			require.NoError(t, txErr)
			_, evtErr := common.ExtractCPIEvent(tx, result.Meta, v2, example_ccip_receiver_v2.ParseEvent_MessageReceived)
			require.Error(t, evtErr, "receiver must not be invoked without data or callback gas")
		})

		t.Run("a dataless message with callback gas still consults the receiver", func(t *testing.T) {
			unsuppliedCCV := solana.NewWallet().PublicKey()
			setCcvConfig(t, []solana.PublicKey{unsuppliedCCV})
			defer setCcvConfig(t, []solana.PublicKey{config.CommitteeVerifierProgram})

			encoded, messageID := buildV2MsgWith(t, 7206, nil, 300_000)
			verifierResults, sErr := testutils.SignCommitteeVerifierMessage(messageID, signers, 1)
			require.NoError(t, sErr)
			execInputs := ccip_offramp.ExecutionInputsV2{
				EncodedMessage:  encoded,
				Ccvs:            []solana.PublicKey{config.CommitteeVerifierProgram},
				VerifierResults: [][]byte{verifierResults},
			}
			resolved := resolveExecAccountsWithAutoBuffering(t, execInputs, user)
			result := testutils.SendAndFailWithLookupTables(ctx, t, solanaGoClient,
				[]solana.Instruction{resolved.Ix}, user, config.DefaultCommitment,
				resolveLutMap(t, resolved.LookupTables),
				[]string{"Required CCV missing from execute input"}, common.AddComputeUnitLimit(400_000))
			require.NotNil(t, result)
		})

		t.Run("get_ccvs_for_msg view skips the receiver for a token-only message", func(t *testing.T) {
			viewRequiredCCV := solana.NewWallet().PublicKey()
			setCcvConfig(t, []solana.PublicKey{viewRequiredCCV})
			defer setCcvConfig(t, []solana.PublicKey{config.CommitteeVerifierProgram})

			encoded, _ := buildV2MsgWith(t, 7207, nil, 0)
			resolved, err := ccip.ResolveGetCCVsForMsgAccounts(
				ctx, solanaGoClient, config.CcipOfframpProgram, encoded, user.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient,
				[]solana.Instruction{resolved.Ix}, user, config.DefaultCommitment)
			require.NotNil(t, result)

			returnValue, err := common.ExtractAnchorTypedReturnValue[ccip_offramp.GetCcvsForMsgResponse](
				ctx, result.Meta.LogMessages, ccip_offramp.ProgramID.String())
			require.NoError(t, err)
			require.Equal(t, []solana.PublicKey{config.CommitteeVerifierProgram}, returnValue.RequiredCcvs)
		})

		t.Run("get_ccvs_for_msg view predicts the v2 receiver's CCVs", func(t *testing.T) {
			// The non-authoritative view must fold in the SAME receiver-required CCVs that execute_v2
			// enforces. Configure a distinct required CCV so the receiver's contribution is visible in
			// the predicted set (it is not a lane default), then query the view with receiverIsV2=true.
			viewRequiredCCV := solana.NewWallet().PublicKey()
			setCcvConfig(t, []solana.PublicKey{viewRequiredCCV})
			defer setCcvConfig(t, []solana.PublicKey{config.CommitteeVerifierProgram}) // restore

			encoded, _ := buildV2Msg(t, 7204)
			resolved, err := ccip.ResolveGetCCVsForMsgAccounts(
				ctx, solanaGoClient, config.CcipOfframpProgram, encoded, user.PublicKey(), logger.Test(t))
			require.NoError(t, err)

			result := testutils.SendAndConfirm(ctx, t, solanaGoClient,
				[]solana.Instruction{resolved.Ix}, user, config.DefaultCommitment)
			require.NotNil(t, result)

			returnValue, err := common.ExtractAnchorTypedReturnValue[ccip_offramp.GetCcvsForMsgResponse](
				ctx, result.Meta.LogMessages, ccip_offramp.ProgramID.String())
			require.NoError(t, err)
			// Receiver required a single, non-default CCV → it is the whole required set (no lane
			// defaults are pulled in, since no `Pubkey::default()` marker is present).
			require.Equal(t, []solana.PublicKey{viewRequiredCCV}, returnValue.RequiredCcvs)
		})
	})
}
