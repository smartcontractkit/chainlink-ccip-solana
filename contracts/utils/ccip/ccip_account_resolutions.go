package ccip

import (
	"bytes"
	"context"
	"fmt"

	binary "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	anchorerrors "github.com/smartcontractkit/anchor-go/errors"
	"github.com/smartcontractkit/chainlink-ccv/protocol"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/committee_verifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/example_ccip_sender_v2"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/executor"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
)

const maxResolveAccountsRounds = 64

type ResolveAccountsResult struct {
	AccountsToSave     []*solana.AccountMeta
	LookupTables       []solana.PublicKey
	ResolutionMetadata []byte
	Ix                 solana.Instruction
}

// AccountResolutionRPC is the read-only RPC surface account resolution needs. Both *rpc.Client and
// the MultiNode adapter satisfy it, so callers can resolve against either.
type AccountResolutionRPC interface {
	SimulateTransactionWithOpts(ctx context.Context, tx *solana.Transaction, opts *rpc.SimulateTransactionOpts) (*rpc.SimulateTransactionResponse, error)
}

// Offramp-specific resolution

func ResolveExecAccounts(
	ctx context.Context,
	arRPC AccountResolutionRPC,
	offrampProgramID solana.PublicKey,
	execInputs ccip_offramp.ExecutionInputsV2,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return resolveAccounts(
		ctx,
		arRPC,
		offrampProgramID,
		ccip_offramp.Instruction_ExecuteV2,
		ccip_offramp.ExecuteParams{
			ExecInputs:         &execInputs,
			ResolutionMetadata: []byte{},
		},
		func(params *ccip_offramp.ExecuteParams, metadata []byte) error {
			params.ResolutionMetadata = metadata
			return nil
		},
		nil,
		payer,
		payer,
		lggr,
	)
}

func ResolveExecAccountsWithBuffer(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	offrampProgramID solana.PublicKey,
	payer solana.PublicKey,
	bufferPDA solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return resolveAccounts(
		ctx,
		solanaGoClient,
		offrampProgramID,
		ccip_offramp.Instruction_ExecuteV2,
		ccip_offramp.ExecuteParams{
			ExecInputs:         nil,
			ResolutionMetadata: []byte{},
		},
		func(params *ccip_offramp.ExecuteParams, metadata []byte) error {
			params.ResolutionMetadata = metadata
			return nil
		},
		[]*solana.AccountMeta{{PublicKey: bufferPDA, IsWritable: false, IsSigner: false}},
		payer,
		payer,
		lggr,
	)
}

func getCCVsForMsgParams(encodedMsg []byte) (ccip_offramp.GetCcvsForMsgParams, error) {
	msg, err := protocol.DecodeMessage(encodedMsg)
	if err != nil {
		return ccip_offramp.GetCcvsForMsgParams{}, fmt.Errorf("decode message: %w", err)
	}

	// Wire-format finality is 4 bytes big-endian: high u16 = flags, low u16 = block_depth.
	finality := uint32(msg.Finality)
	params := ccip_offramp.GetCcvsForMsgParams{
		ResolutionMetadata:  []byte{},
		RemoteChainSelector: uint64(msg.SourceChainSelector),
		RequestedFinality: ccip_offramp.FinalityConfig{
			Flags:      uint16(finality >> 16),
			BlockDepth: uint16(finality), //nolint:gosec // G115 lower 16 bits
		},
		Sender: msg.Sender,
	}

	params.DataLen = uint32(len(msg.Data)) //nolint:gosec // G115 message data is length-prefixed by a uint16
	params.CcipReceiveGasLimit = msg.CcipReceiveGasLimit
	if len(msg.Receiver) == solana.PublicKeyLength {
		params.MessageReceiver = solana.PublicKeyFromBytes(msg.Receiver)
	}

	if msg.TokenTransfer != nil {
		var beBytes [32]byte
		if msg.TokenTransfer.Amount != nil {
			msg.TokenTransfer.Amount.FillBytes(beBytes[:])
		}
		params.TokenTransfer = &ccip_offramp.TokenTransferV1{
			Version:            msg.TokenTransfer.Version,
			Amount:             ccip_offramp.ProtocolAmount{BeBytes: beBytes},
			SourcePoolAddress:  msg.TokenTransfer.SourcePoolAddress,
			SourceTokenAddress: msg.TokenTransfer.SourceTokenAddress,
			DestTokenAddress:   msg.TokenTransfer.DestTokenAddress,
			TokenReceiver:      msg.TokenTransfer.TokenReceiver,
			ExtraData:          msg.TokenTransfer.ExtraData,
		}
	}

	return params, nil
}

func ResolveGetCCVsForMsgAccounts(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	offrampProgramID solana.PublicKey,
	encodedMsg []byte,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	params, err := getCCVsForMsgParams(encodedMsg)
	if err != nil {
		return ResolveAccountsResult{}, err
	}

	// The on-chain resolver returns every segment the processor reads back
	// ([receiver_registry, receiver_ccv, pool]), so this is a plain resolveAccounts like
	// ResolveExecAccounts.
	return resolveAccounts(
		ctx,
		solanaGoClient,
		offrampProgramID,
		ccip_offramp.Instruction_GetCcvsForMsg,
		params,
		func(params *ccip_offramp.GetCcvsForMsgParams, metadata []byte) error {
			params.ResolutionMetadata = metadata
			return nil
		},
		nil,
		payer,
		payer,
		lggr,
	)
}

// Router-specific resolution

func ResolveGetFeeAccounts(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	routerProgramID solana.PublicKey,
	destChainSelector uint64,
	message ccip_router.Svm2AnyMessage,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return resolveAccounts(
		ctx,
		solanaGoClient,
		routerProgramID,
		ccip_router.Instruction_GetFeeV2,
		ccip_router.GetFeeParams{
			DestChainSelector:  destChainSelector,
			Message:            message,
			ResolutionMetadata: []byte{},
		},
		func(params *ccip_router.GetFeeParams, metadata []byte) error {
			params.ResolutionMetadata = metadata
			return nil
		},
		nil,
		payer,
		payer,
		lggr,
	)
}

func ResolveCcipSendV2Accounts(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	routerProgramID solana.PublicKey,
	destChainSelector uint64,
	message ccip_router.Svm2AnyMessage,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return ResolveCcipSendV2AccountsForCaller(
		ctx, solanaGoClient, routerProgramID, destChainSelector, message, payer, payer, lggr)
}

// ResolveCcipSendV2AccountsForCaller resolves `ccip_send_v2` accounts for a sender that is not the
// payer. Programs that wrap the onramp sign the CPI with a PDA of their own, so the resolved list
// (the authority slot, its fee ATA, and the sender's token account) must be derived for that PDA -
// which cannot itself pay for the resolution simulations.
func ResolveCcipSendV2AccountsForCaller(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	routerProgramID solana.PublicKey,
	destChainSelector uint64,
	message ccip_router.Svm2AnyMessage,
	caller solana.PublicKey,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return resolveAccounts(
		ctx,
		solanaGoClient,
		routerProgramID,
		ccip_router.Instruction_CcipSendV2,
		ccip_router.CcipSendV2Params{
			DestChainSelector:  destChainSelector,
			Message:            message,
			ResolutionMetadata: []byte{},
		},
		func(params *ccip_router.CcipSendV2Params, metadata []byte) error {
			params.ResolutionMetadata = metadata
			return nil
		},
		nil,
		caller,
		payer,
		lggr,
	)
}

// Example sender v2 resolution
//
// The example sender wraps `ccip_send_v2`, so its resolver wraps the router's: each of its rounds
// CPIs into the corresponding router stage. That is invisible from here - resolution is driven
// exactly as for any other program - except for the accounts its start stage needs on top of the
// interface's config account, which is what this helper assembles.

// ResolveExampleSenderV2SendAccounts resolves the accounts for the example sender v2's `ccip_send`,
// including the router's own list, which the sender's resolver forwards.
//
// `transferTokenMint` must be set when `params.TokenAmount` is, and is ignored otherwise: the
// resolver reads it to derive the caller's source token account, which is not part of the router's
// list. `feeTokenMint` is the mint fees are quoted in, i.e. wrapped SOL when paying in native SOL.
//
// `params.ResolutionMetadata` and `params.MaxFee` are ignored by the resolver (the first is what
// resolution produces, the second does not affect the account list); the returned instruction
// carries the resolved metadata, so only `MaxFee` needs filling in afterwards.
func ResolveExampleSenderV2SendAccounts(
	ctx context.Context,
	solanaGoClient *rpc.Client,
	senderProgramID solana.PublicKey,
	routerProgramID solana.PublicKey,
	params example_ccip_sender_v2.CcipSendParams,
	feeTokenMint solana.PublicKey,
	transferTokenMint solana.PublicKey,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	statePDA, _, err := solana.FindProgramAddress([][]byte{[]byte("state")}, senderProgramID)
	if err != nil {
		return ResolveAccountsResult{}, fmt.Errorf("failed to find sender state PDA: %w", err)
	}
	chainConfigPDA, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("remote_chain_config"), common.Uint64ToLE(params.DestChainSelector)}, senderProgramID)
	if err != nil {
		return ResolveAccountsResult{}, fmt.Errorf("failed to find sender chain config PDA: %w", err)
	}
	routerConfigPDA, _, err := state.FindConfigPDA(routerProgramID)
	if err != nil {
		return ResolveAccountsResult{}, fmt.Errorf("failed to find router config PDA: %w", err)
	}

	// Order is what the sender's resolve_accounts_start context declares, after its own state
	// account: see the resolve_accounts module docs in the program.
	additionalAccounts := []*solana.AccountMeta{
		solana.Meta(chainConfigPDA),
		solana.Meta(feeTokenMint),
		solana.Meta(routerProgramID),
		solana.Meta(routerConfigPDA),
	}
	if params.TokenAmount != nil {
		additionalAccounts = append(additionalAccounts, solana.Meta(transferTokenMint))
	}

	return resolveAccountsWithConfig(
		ctx,
		solanaGoClient,
		senderProgramID,
		statePDA,
		example_ccip_sender_v2.Instruction_CcipSend,
		params,
		func(p *example_ccip_sender_v2.CcipSendParams, metadata []byte) error {
			p.ResolutionMetadata = metadata
			return nil
		},
		additionalAccounts,
		// The user signs `ccip_send` itself; the sender substitutes its own PDA when it resolves
		// the router.
		payer,
		payer,
		lggr,
	)
}

// Committee Verifier resolution
//
// Note there are no resolution helpers here for the verifier's CPI-only instructions
// (`verify`, `get_fee`, `start_verification_at_source`): their accounts are only
// resolvable through `nested_resolve_accounts_start`, which is driven by the calling
// program's resolver (offramp/router), never directly from off-chain code.

func ResolveTransferOwnershipAccounts(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	cvProgramID solana.PublicKey,
	proposedOwner solana.PublicKey,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return resolveAccounts(
		ctx,
		solanaGoClient,
		cvProgramID,
		committee_verifier.Instruction_TransferOwnership,
		proposedOwner,
		func(_ *solana.PublicKey, _ []uint8) error { return nil },
		nil,
		payer,
		payer,
		lggr,
	)
}

func ResolveCVAcceptOwnershipAccounts(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	cvProgramID solana.PublicKey,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return resolveAccounts(
		ctx,
		solanaGoClient,
		cvProgramID,
		committee_verifier.Instruction_AcceptOwnership,
		struct{}{},
		func(_ *struct{}, _ []uint8) error { return nil },
		nil,
		payer,
		payer,
		lggr,
	)
}

func ResolveCVTransferStorageLocationAdminAccounts(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	cvProgramID solana.PublicKey,
	proposedStorageLocationAdmin solana.PublicKey,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return resolveAccounts(
		ctx,
		solanaGoClient,
		cvProgramID,
		committee_verifier.Instruction_TransferStorageLocationAdmin,
		proposedStorageLocationAdmin,
		func(_ *solana.PublicKey, _ []uint8) error { return nil },
		nil,
		payer,
		payer,
		lggr,
	)
}

func ResolveCVAcceptStorageLocationAdminAccounts(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	cvProgramID solana.PublicKey,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return resolveAccounts(
		ctx,
		solanaGoClient,
		cvProgramID,
		committee_verifier.Instruction_AcceptStorageLocationAdmin,
		struct{}{},
		func(_ *struct{}, _ []uint8) error { return nil },
		nil,
		payer,
		payer,
		lggr,
	)
}

func ResolveCCVSetAllowlistAdminAccounts(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	cvProgramID solana.PublicKey,
	allowlistAdmin solana.PublicKey,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return resolveAccounts(
		ctx,
		solanaGoClient,
		cvProgramID,
		committee_verifier.Instruction_SetAllowlistAdmin,
		allowlistAdmin,
		func(_ *solana.PublicKey, _ []uint8) error { return nil },
		nil,
		payer,
		payer,
		lggr,
	)
}

func ResolveCVSetFeeAggregatorAccounts(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	cvProgramID solana.PublicKey,
	feeAggregator solana.PublicKey,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return resolveAccounts(
		ctx,
		solanaGoClient,
		cvProgramID,
		committee_verifier.Instruction_SetFeeAggregator,
		feeAggregator,
		func(_ *solana.PublicKey, _ []uint8) error { return nil },
		nil,
		payer,
		payer,
		lggr,
	)
}

func ResolveSetDefaultCodeVersionAccounts(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	cvProgramID solana.PublicKey,
	codeVersion committee_verifier.CodeVersion,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return resolveAccounts(
		ctx,
		solanaGoClient,
		cvProgramID,
		committee_verifier.Instruction_SetDefaultCodeVersion,
		codeVersion,
		func(_ *committee_verifier.CodeVersion, _ []uint8) error { return nil },
		nil,
		payer,
		payer,
		lggr,
	)
}

func ResolveCVGetStorageLocationsAccounts(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	cvProgramID solana.PublicKey,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return resolveAccounts(
		ctx,
		solanaGoClient,
		cvProgramID,
		committee_verifier.Instruction_GetStorageLocations,
		struct{}{},
		func(_ *struct{}, _ []uint8) error { return nil },
		nil,
		payer,
		payer,
		lggr,
	)
}

// ResolveCVInitChainConfigAccounts resolves `init_chain_config`, which creates a remote chain's
// config. Use ResolveCVEditChainConfigAccounts once the remote chain is configured.
func ResolveCVInitChainConfigAccounts(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	cvProgramID solana.PublicKey,
	params committee_verifier.SetChainConfigParams,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return resolveAccounts(
		ctx,
		solanaGoClient,
		cvProgramID,
		committee_verifier.Instruction_InitChainConfig,
		params,
		func(_ *committee_verifier.SetChainConfigParams, _ []uint8) error { return nil },
		nil,
		payer,
		payer,
		lggr,
	)
}

// ResolveCVEditChainConfigAccounts resolves `edit_chain_config`, which overwrites the admin
// config of an already configured remote chain.
func ResolveCVEditChainConfigAccounts(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	cvProgramID solana.PublicKey,
	params committee_verifier.SetChainConfigParams,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return resolveAccounts(
		ctx,
		solanaGoClient,
		cvProgramID,
		committee_verifier.Instruction_EditChainConfig,
		params,
		func(_ *committee_verifier.SetChainConfigParams, _ []uint8) error { return nil },
		nil,
		payer,
		payer,
		lggr,
	)
}

func ResolveCCVSetAllowlistAccounts(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	cvProgramID solana.PublicKey,
	params committee_verifier.SetAllowlistParams,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return resolveAccounts(
		ctx,
		solanaGoClient,
		cvProgramID,
		committee_verifier.Instruction_SetAllowlist,
		params,
		func(_ *committee_verifier.SetAllowlistParams, _ []uint8) error { return nil },
		nil,
		payer,
		payer,
		lggr,
	)
}

func ResolveCVSetSignerConfigAccounts(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	cvProgramID solana.PublicKey,
	params committee_verifier.SetSignerConfigParams,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return resolveAccounts(
		ctx,
		solanaGoClient,
		cvProgramID,
		committee_verifier.Instruction_SetSignerConfig,
		params,
		func(_ *committee_verifier.SetSignerConfigParams, _ []uint8) error { return nil },
		nil,
		payer,
		payer,
		lggr,
	)
}

func ResolveCVRemoveSignerConfigAccounts(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	cvProgramID solana.PublicKey,
	params committee_verifier.RemoveSignerConfigParams,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return resolveAccounts(
		ctx,
		solanaGoClient,
		cvProgramID,
		committee_verifier.Instruction_RemoveSignerConfig,
		params,
		func(_ *committee_verifier.RemoveSignerConfigParams, _ []uint8) error { return nil },
		nil,
		payer,
		payer,
		lggr,
	)
}

func ResolveCVSetStorageLocationsAccounts(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	cvProgramID solana.PublicKey,
	params committee_verifier.SetStorageLocationsParams,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return resolveAccounts(
		ctx,
		solanaGoClient,
		cvProgramID,
		committee_verifier.Instruction_SetStorageLocations,
		params,
		func(_ *committee_verifier.SetStorageLocationsParams, _ []uint8) error { return nil },
		nil,
		payer,
		payer,
		lggr,
	)
}

// Executor resolution

func ResolveExecTransferOwnershipAccounts(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	executorProgramID solana.PublicKey,
	proposedOwner solana.PublicKey,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return resolveAccounts(
		ctx,
		solanaGoClient,
		executorProgramID,
		executor.Instruction_TransferOwnership,
		proposedOwner,
		func(_ *solana.PublicKey, _ []uint8) error { return nil },
		[]*solana.AccountMeta{},
		payer,
		payer,
		lggr,
	)
}

func ResolveExecAcceptOwnershipAccounts(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	executorProgramID solana.PublicKey,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return resolveAccounts(
		ctx,
		solanaGoClient,
		executorProgramID,
		executor.Instruction_AcceptOwnership,
		struct{}{},
		func(_ *struct{}, _ []uint8) error { return nil },
		[]*solana.AccountMeta{},
		payer,
		payer,
		lggr,
	)
}

func ResolveExecSetChainConfigAccounts(
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	executorProgramID solana.PublicKey,
	params executor.SetChainConfigParams,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	return resolveAccounts(
		ctx,
		solanaGoClient,
		executorProgramID,
		executor.Instruction_SetChainConfig,
		params,
		func(_ *executor.SetChainConfigParams, _ []uint8) error { return nil },
		[]*solana.AccountMeta{},
		payer,
		payer,
		lggr,
	)
}

////////////////////////////////////////
// Shared resolution across contracts //
////////////////////////////////////////

func buildIxToResolve(programID solana.PublicKey, discriminator [8]byte, params any) (solana.Instruction, error) {
	ixData, err := encodeIxData(discriminator, params)
	if err != nil {
		return nil, fmt.Errorf("failed to encode instruction data: %w", err)
	}

	ix := solana.NewInstruction(
		programID,
		solana.AccountMetaSlice{},
		ixData,
	)
	return ix, nil
}

// based on Anchor-go's implementation of instruction data encoding, but adapted to our needs for account resolution
// (mainly, we need to be able to do this without having a full instruction with all the accounts)
func encodeIxData(discriminator [8]byte, params any) ([]byte, error) {
	buf := new(bytes.Buffer)
	enc := binary.NewBorshEncoder(buf)

	// Encode the instruction discriminator.
	err := enc.WriteBytes(discriminator[:], false)
	if err != nil {
		return nil, fmt.Errorf("failed to write instruction discriminator: %w", err)
	}
	{
		err = enc.Encode(params)
		if err != nil {
			return nil, anchorerrors.NewField("paramsParam", err)
		}
	}

	return buf.Bytes(), nil
}

// This is reusable for all instructions that support account resolution, across programs that implement the
// shared resolution interface (defined in ccip-common), which involves:
// - shared types for ix data & responses
// - shared discriminator for the first instruction in the resolution flow
// - equivalent context for the first instruction, i.e. just the config account of the contract (which uses the common config seed)
func resolveAccounts[T any](
	ctx context.Context,
	rpc AccountResolutionRPC,
	programID solana.PublicKey,
	discriminator [8]byte,
	ixParams T,
	setResolutionMetadata func(*T, []byte) error, // to set the resolver-produced metadata in the final ixData
	// Extra accounts to append to the first resolve_accounts_start call as remaining accounts.
	// Instruction-specific resolver docs define when this is required.
	additionalAccounts []*solana.AccountMeta,
	// The account that will sign the resolved instruction. Usually the payer, but not when the
	// instruction is invoked via CPI: there the signer is the calling program's PDA, which cannot
	// pay for the resolution simulations.
	caller solana.PublicKey,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	configPDA, _, err := state.FindConfigPDA(programID)
	if err != nil {
		return ResolveAccountsResult{}, fmt.Errorf("failed to find config PDA: %w", err)
	}
	return resolveAccountsWithConfig(
		ctx, rpc, programID, configPDA, discriminator, ixParams,
		setResolutionMetadata, additionalAccounts, caller, payer, lggr)
}

// resolveAccountsWithConfig is resolveAccounts for a program whose config account does not sit at
// the conventional `[b"config"]` seed. The interface only requires that the account be first in the
// start stage's context, not what it is called or how it is derived - the example sender v2, for
// instance, plays that role with its `[b"state"]` PDA.
func resolveAccountsWithConfig[T any](
	ctx context.Context,
	solanaGoClient AccountResolutionRPC,
	programID solana.PublicKey,
	configPDA solana.PublicKey,
	discriminator [8]byte,
	ixParams T,
	setResolutionMetadata func(*T, []byte) error,
	additionalAccounts []*solana.AccountMeta,
	caller solana.PublicKey,
	payer solana.PublicKey,
	lggr logger.Logger,
) (ResolveAccountsResult, error) {
	result := ResolveAccountsResult{
		AccountsToSave:     make([]*solana.AccountMeta, 0),
		LookupTables:       make([]solana.PublicKey, 0),
		ResolutionMetadata: make([]byte, 0),
		Ix:                 nil,
	}

	ix, err := buildIxToResolve(programID, discriminator, ixParams)
	if err != nil {
		return ResolveAccountsResult{}, fmt.Errorf("failed to build instruction to resolve accounts: %w", err)
	}

	ixData, err := ix.Data()
	if err != nil {
		return ResolveAccountsResult{}, fmt.Errorf("failed to get instruction data: %w", err)
	}

	nextDiscrim := ccip_common.Instruction_ResolveAccountsStart // the start discrim is shared across contracts
	accountsForNextRound := make([]*solana.AccountMeta, 0, 1+len(additionalAccounts))
	accountsForNextRound = append(accountsForNextRound, solana.Meta(configPDA))
	accountsForNextRound = append(accountsForNextRound, additionalAccounts...)
	params := ccip_common.ResolveAccountsParams{
		Caller:   caller,
		IxData:   ixData,
		Metadata: []byte{}, // this value can be modified in each round
	}

	for round := 0; ; round++ {
		if err := checkResolveAccountsRoundLimit(round); err != nil {
			return ResolveAccountsResult{}, err
		}

		nextResolutionIxStart, err := ccip_common.NewResolveAccountsStartInstruction(
			params,
			solana.PublicKey{}, // this is just a placeholder, it gets overwritten in the actual ix
		)
		if err != nil {
			return ResolveAccountsResult{}, fmt.Errorf("failed to create resolve accounts instruction: %w", err)
		}

		nextResolutionIxData, err := nextResolutionIxStart.Data()
		if err != nil {
			return ResolveAccountsResult{}, fmt.Errorf("failed to get next instruction data: %w", err)
		}

		copy(nextResolutionIxData[0:8], nextDiscrim[:])

		nextResolutionIx := solana.GenericInstruction{
			AccountValues: accountsForNextRound,
			ProgID:        ix.ProgramID(), // take the program ID from the original instruction we want to resolve accounts for, as that's the one that needs to be called for resolution
			DataBytes:     nextResolutionIxData,
		}

		res, err := simulateResolveAccountsRound(ctx, solanaGoClient, &nextResolutionIx, payer)
		if err != nil {
			return ResolveAccountsResult{}, fmt.Errorf("failed to simulate transaction: %w", err)
		}
		lggr.Debug("Simulated transaction logs:")
		for _, line := range res.Value.Logs {
			lggr.Debug(line)
		}
		if res.Value.Err != nil {
			// Include the program logs: for failures like AccountNotFound they name the
			// offending account, which res.Value.Err alone does not.
			return ResolveAccountsResult{}, fmt.Errorf("simulation error on resolution round %d: %v, logs: %q", round, res.Value.Err, res.Value.Logs)
		}
		response, err := common.ExtractAnchorTypedReturnValue[ccip_common.ResolveAccountsResponse](
			ctx, res.Value.Logs, ix.ProgramID().String())
		if err != nil {
			return ResolveAccountsResult{}, fmt.Errorf("failed to extract resolve accounts response: %w", err)
		}
		if response == nil {
			return ResolveAccountsResult{}, fmt.Errorf("no resolve accounts response found in simulation logs: %q", res.Value.Logs)
		}
		lggr.Debug("Response from resolve accounts instruction", "response", response)

		// Store the response from this round into the final result
		for _, acc := range response.AccountsToSave {
			result.AccountsToSave = append(result.AccountsToSave, &solana.AccountMeta{
				PublicKey:  acc.Pubkey,
				IsWritable: acc.IsWritable,
				IsSigner:   acc.IsSigner,
			})
		}

		result.LookupTables = append(result.LookupTables, response.LookupTablesToSave...)

		// Return if there are no more rounds
		if response.NextIxDiscriminator == nil {
			result.ResolutionMetadata = response.Metadata
			if setErr := setResolutionMetadata(&ixParams, result.ResolutionMetadata); setErr != nil {
				return ResolveAccountsResult{}, fmt.Errorf("failed to set resolution metadata: %w", setErr)
			}

			ix, err = buildIxToResolve(programID, discriminator, ixParams)
			if err != nil {
				return ResolveAccountsResult{}, fmt.Errorf("failed to build final instruction data: %w", err)
			}
			ixData, err = ix.Data()
			if err != nil {
				return ResolveAccountsResult{}, fmt.Errorf("failed to get final instruction data: %w", err)
			}
			result.Ix = &solana.GenericInstruction{
				AccountValues: result.AccountsToSave,
				ProgID:        programID,
				DataBytes:     ixData,
			}
			return result, nil
		}

		// Prepare for next round
		nextDiscrim = *response.NextIxDiscriminator
		params.Metadata = response.Metadata

		accountsForNextRound = make([]*solana.AccountMeta, len(response.AskAgainWith))
		for i, key := range response.AskAgainWith {
			accountsForNextRound[i] = &solana.AccountMeta{
				PublicKey:  key,
				IsWritable: false, // the account resolution process is always read-only
				IsSigner:   false,
			}
		}
	}
}

func checkResolveAccountsRoundLimit(round int) error {
	if round >= maxResolveAccountsRounds {
		return fmt.Errorf("account resolution exceeded maximum rounds (round=%d max=%d)", round, maxResolveAccountsRounds)
	}
	return nil
}

func simulateResolveAccountsRound(
	ctx context.Context,
	rpcClient AccountResolutionRPC,
	resolveIx solana.Instruction,
	payer solana.PublicKey,
) (*rpc.SimulateTransactionResponse, error) {
	tx, opts, err := common.BuildReadOnlySimulationTx(payer, resolveIx)
	if err != nil {
		return nil, err
	}
	return rpcClient.SimulateTransactionWithOpts(ctx, tx, opts)
}

// BuildExecutionInputsV2 assembles execute_v2 inputs: CCV program IDs and
// verifier-result blobs, positionally aligned with each other. Shared by the
// transmitter (real submissions) and the destination reader (simulations) so the
// two can never diverge on alignment.
func BuildExecutionInputsV2(encodedMessage []byte, ccvs []protocol.UnknownAddress, ccvData [][]byte) (ccip_offramp.ExecutionInputsV2, error) {
	// Same check as the on-chain execute_v2.
	if len(ccvs) != len(ccvData) {
		return ccip_offramp.ExecutionInputsV2{}, fmt.Errorf("%d CCVs but %d verifier results", len(ccvs), len(ccvData))
	}
	ccvKeys := make([]solana.PublicKey, len(ccvs))
	for i, addr := range ccvs {
		if len(addr) != solana.PublicKeyLength {
			return ccip_offramp.ExecutionInputsV2{}, fmt.Errorf("invalid CCV address length at index %d: %d", i, len(addr))
		}
		copy(ccvKeys[i][:], addr)
	}
	results := make([][]byte, len(ccvData))
	for i, blob := range ccvData {
		results[i] = append([]byte(nil), blob...)
	}
	return ccip_offramp.ExecutionInputsV2{
		EncodedMessage:  encodedMessage,
		VerifierResults: results,
		Ccvs:            ccvKeys,
	}, nil
}
