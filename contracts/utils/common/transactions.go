//revive:disable:var-naming // legacy package name
package common

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/fees"
)

// MaxSolanaTxSize is the Solana transaction size limit enforced by RPC nodes.
const MaxSolanaTxSize = 1232

func SendAndConfirm(ctx context.Context, rpcClient *rpc.Client, instructions []solana.Instruction,
	signer solana.PrivateKey, commitment rpc.CommitmentType, opts ...TxModifier,
) (*rpc.GetTransactionResult, error) {
	emptyLookupTables := map[solana.PublicKey]solana.PublicKeySlice{}
	return SendAndConfirmWithLookupTables(ctx, rpcClient, instructions, signer, commitment, emptyLookupTables, opts...)
}

func SendAndConfirmWithLookupTables(ctx context.Context, rpcClient *rpc.Client, instructions []solana.Instruction,
	signer solana.PrivateKey, commitment rpc.CommitmentType, lookupTables map[solana.PublicKey]solana.PublicKeySlice, opts ...TxModifier,
) (*rpc.GetTransactionResult, error) {
	txres, err := sendTransactionWithLookupTables(ctx, rpcClient, instructions, signer, commitment, TransactionConfigs{SkipPreflight: false, Retries: 1, RetryDelay: 50 * time.Millisecond}, lookupTables, opts...) // do not skipPreflight when expected to pass, preflight can help debug
	if err != nil {
		return nil, err
	}

	if txres.Meta == nil {
		return nil, errors.New("txres.Meta == nil")
	}

	if txres.Meta.Err != nil {
		return nil, fmt.Errorf("tx failed with: %+v", txres.Meta) // tx should not err, print meta if it does (contains logs)
	}
	return txres, nil
}

func SendAndConfirmWithLookupTablesAndRetries(ctx context.Context, rpcClient *rpc.Client, instructions []solana.Instruction,
	signer solana.PrivateKey, commitment rpc.CommitmentType, lookupTables map[solana.PublicKey]solana.PublicKeySlice, opts ...TxModifier,
) (*rpc.GetTransactionResult, error) {
	txres, err := sendTransactionWithLookupTables(ctx, rpcClient, instructions, signer, commitment, TransactionConfigs{SkipPreflight: false, Retries: 500, RetryDelay: 50 * time.Millisecond}, lookupTables, opts...) // do not skipPreflight when expected to pass, preflight can help debug
	if err != nil {
		return nil, err
	}

	if txres.Meta == nil {
		return nil, errors.New("txres.Meta == nil")
	}

	if txres.Meta.Err != nil {
		return nil, fmt.Errorf("tx failed with: %+v", txres.Meta) // tx should not err, print meta if it does (contains logs)
	}
	return txres, nil
}

func SendAndFailWith(ctx context.Context, rpcClient *rpc.Client, instructions []solana.Instruction,
	signer solana.PrivateKey, commitment rpc.CommitmentType, expectedErrors []string, opts ...TxModifier,
) (*rpc.GetTransactionResult, error) {
	emptyLookupTables := map[solana.PublicKey]solana.PublicKeySlice{}
	return SendAndFailWithLookupTables(ctx, rpcClient, instructions, signer, commitment, emptyLookupTables, expectedErrors, opts...)
}

func SendAndFailWithLookupTables(ctx context.Context, rpcClient *rpc.Client, instructions []solana.Instruction,
	signer solana.PrivateKey, commitment rpc.CommitmentType, lookupTables map[solana.PublicKey]solana.PublicKeySlice, expectedErrors []string, opts ...TxModifier,
) (*rpc.GetTransactionResult, error) {
	txres, err := sendTransactionWithLookupTables(ctx, rpcClient, instructions, signer, commitment, TransactionConfigs{SkipPreflight: true, Retries: 1, RetryDelay: 50 * time.Millisecond}, lookupTables, opts...) // skipPreflight when expected to fail so revert captured onchain
	if err != nil {
		return nil, err
	}

	if txres.Meta == nil {
		return nil, errors.New("txres.Meta == nil")
	}
	if txres.Meta.Err == nil {
		for _, l := range txres.Meta.LogMessages {
			fmt.Println(l)
		}
		return nil, errors.New("expected transaction to fail, but txres.Meta.Err == nil")
	}
	logs := strings.Join(txres.Meta.LogMessages, " ")
	for _, expectedError := range expectedErrors {
		if !strings.Contains(logs, expectedError) {
			return nil, fmt.Errorf("the logs did not contain '%s'. The logs were: %s", expectedError, logs)
		}
	}
	return txres, nil
}

func SendAndFailWithRPCError(ctx context.Context, rpcClient *rpc.Client, instructions []solana.Instruction,
	signer solana.PrivateKey, _ rpc.CommitmentType, expectedErrors []string, opts ...TxModifier,
) error {
	hashRes, err := rpcClient.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return err
	}

	tx, err := solana.NewTransaction(
		instructions,
		hashRes.Value.Blockhash,
		solana.TransactionPayer(signer.PublicKey()),
	)
	if err != nil {
		return err
	}

	// build signers map
	signers := map[solana.PublicKey]solana.PrivateKey{}
	signers[signer.PublicKey()] = signer

	// set options before signing transaction
	for _, o := range opts {
		if err = o(tx, signers); err != nil {
			return err
		}
	}

	if _, err = tx.Sign(func(_ solana.PublicKey) *solana.PrivateKey {
		return &signer
	}); err != nil {
		return err
	}

	_, err = rpcClient.SendTransactionWithOpts(ctx, tx, rpc.TransactionOpts{SkipPreflight: false, PreflightCommitment: rpc.CommitmentProcessed})
	if err == nil {
		return errors.New("expected RPC error - none found")
	}

	errStr := err.Error()
	for _, expectedError := range expectedErrors {
		if !strings.Contains(errStr, expectedError) {
			return fmt.Errorf("the error did not contain '%s'. The error was: %s", expectedError, errStr)
		}
	}
	return nil
}

// TxModifier is a dynamic function used to flexibly add components to a transaction such as additional signers, and compute budget parameters
type TxModifier func(tx *solana.Transaction, signers map[solana.PublicKey]solana.PrivateKey) error

func AddSigners(additionalSigners ...solana.PrivateKey) TxModifier {
	return func(_ *solana.Transaction, s map[solana.PublicKey]solana.PrivateKey) error {
		for _, v := range additionalSigners {
			s[v.PublicKey()] = v
		}
		return nil
	}
}

// AddComputeUnitLimit allows for configuring the total compute unit limit for a transaction - solana network default is 200K, maximum is 1.4M
// signature verification compute units can vary depending on searching for signatures
func AddComputeUnitLimit(v fees.ComputeUnitLimit) TxModifier {
	return func(tx *solana.Transaction, _ map[solana.PublicKey]solana.PrivateKey) error {
		return fees.SetComputeUnitLimit(tx, v)
	}
}

// AddHeapFrame requests a larger program heap frame for the transaction - the network default is
// 32KB, the maximum is 256KB. This only has an effect on programs that install their own
// `#[global_allocator]` (the `custom-heap` feature); Anchor's default bump allocator is hardcoded to
// 32KB and ignores the requested frame.
func AddHeapFrame(v fees.HeapFrame) TxModifier {
	return func(tx *solana.Transaction, _ map[solana.PublicKey]solana.PrivateKey) error {
		return fees.SetHeapFrame(tx, v)
	}
}

func AddComputeUnitPrice(v fees.ComputeUnitPrice) TxModifier {
	return func(tx *solana.Transaction, _ map[solana.PublicKey]solana.PrivateKey) error {
		return fees.SetComputeUnitPrice(tx, v)
	}
}

type TransactionConfigs struct {
	SkipPreflight bool
	Retries       int
	RetryDelay    time.Duration
}

func sendTransactionWithLookupTables(ctx context.Context, rpcClient *rpc.Client, instructions []solana.Instruction,
	signerAndPayer solana.PrivateKey, commitment rpc.CommitmentType, transactionConfigs TransactionConfigs, lookupTables map[solana.PublicKey]solana.PublicKeySlice, opts ...TxModifier,
) (*rpc.GetTransactionResult, error) {
	var errBlockHash error
	var hashRes *rpc.GetLatestBlockhashResult
	for range transactionConfigs.Retries {
		hashRes, errBlockHash = rpcClient.GetLatestBlockhash(ctx, commitment)
		if errBlockHash != nil {
			fmt.Println("GetLatestBlockhash error:", errBlockHash)
			time.Sleep(transactionConfigs.RetryDelay)
			continue
		}
		break
	}
	if errBlockHash != nil {
		fmt.Println("GetLatestBlockhash error after retries:", errBlockHash)
		return nil, errBlockHash
	}

	tx, err := solana.NewTransaction(
		instructions,
		hashRes.Value.Blockhash,
		solana.TransactionAddressTables(lookupTables),
		solana.TransactionPayer(signerAndPayer.PublicKey()),
	)
	if err != nil {
		return nil, err
	}

	// build signers map
	signers := map[solana.PublicKey]solana.PrivateKey{}
	signers[signerAndPayer.PublicKey()] = signerAndPayer

	// set options before signing transaction
	for _, o := range opts {
		if err = o(tx, signers); err != nil {
			return nil, err
		}
	}

	if _, err = tx.Sign(func(pub solana.PublicKey) *solana.PrivateKey {
		priv, ok := signers[pub]
		if !ok {
			fmt.Printf("ERROR: Missing signer private key for %s\n", pub)
		}
		return &priv
	}); err != nil {
		return nil, err
	}

	var txsig solana.Signature
	for range transactionConfigs.Retries {
		txsig, err = rpcClient.SendTransactionWithOpts(ctx, tx, rpc.TransactionOpts{SkipPreflight: transactionConfigs.SkipPreflight, PreflightCommitment: commitment})
		if err != nil {
			var rpcErr *jsonrpc.RPCError
			if errors.As(err, &rpcErr) {
				fmt.Println("RPC Error:", rpcErr.Message)
				fmt.Println("RPC Error Code:", rpcErr.Code)
				if strings.Contains(rpcErr.Message, "Blockhash not found") {
					// this can happen when the blockhash we retrieved above is not yet visible to the rpc
					// given we get the blockhash from the same rpc, this should not happen, but we see it in practice
					// retrying fixes this
					fmt.Println("Blockhash not found, retrying...")
					time.Sleep(50 * time.Millisecond)
					continue
				}
				fmt.Printf("Failed Tx: %+v\n", tx)
				fmt.Println("Unexpected error (most likely contract related), no point in retrying:", err)
				return nil, err
			}
			// Not an RPC error — should only happen when we fail to hit the rpc service
			fmt.Println("Unexpected error (could not hit rpc service), retrying:", err)
			time.Sleep(transactionConfigs.RetryDelay)
			continue
		}
		// no error = success
		break
	}

	var txStatus rpc.ConfirmationStatusType
	count := 0
	for txStatus != rpc.ConfirmationStatusConfirmed && txStatus != rpc.ConfirmationStatusFinalized {
		if count > 500 {
			return nil, fmt.Errorf("unable to find transaction within timeout (sig: %v)", txsig)
		}
		count++
		statusRes, sigErr := rpcClient.GetSignatureStatuses(ctx, true, txsig)
		if sigErr != nil {
			fmt.Println(sigErr) // debugging if tx errors; mainnet can be flakey
			time.Sleep(transactionConfigs.RetryDelay)
			continue
		}
		if statusRes != nil && len(statusRes.Value) > 0 && statusRes.Value[0] != nil {
			txStatus = statusRes.Value[0].ConfirmationStatus
		}
		time.Sleep(transactionConfigs.RetryDelay)
	}

	v := uint64(0)
	var errGetTx error
	var transactionRes *rpc.GetTransactionResult
	for range transactionConfigs.Retries {
		transactionRes, errGetTx = rpcClient.GetTransaction(ctx, txsig, &rpc.GetTransactionOpts{
			Commitment:                     commitment,
			MaxSupportedTransactionVersion: &v,
		})
		if errGetTx != nil {
			fmt.Println("Error getting transaction:", errGetTx)
			time.Sleep(transactionConfigs.RetryDelay)
			continue
		}
		break
	}
	return transactionRes, errGetTx
}

// InstructionWithProgramID returns ix with programID as the instruction target. Use it with gobinding
// constructors, which target the package-level ProgramID.
func InstructionWithProgramID(ix solana.Instruction, programID solana.PublicKey) (solana.Instruction, error) {
	data, err := ix.Data()
	if err != nil {
		return nil, err
	}
	return solana.NewInstruction(programID, ix.Accounts(), data), nil
}

// BuildReadOnlySimulationTx builds an unsigned transaction suitable for read-only
// simulation of one or more instructions (account resolution, onchain getters, etc.).
//
// As the underlying call is read-only, simulation is enough, no need to actually execute
// things onchain. Use ReplaceRecentBlockhash + CommitmentConfirmed so we simulate against
// the latest confirmed state rather than finalized, which may lag behind on local validators.
// Callers that compare simulation output against separately-read account state must read that
// state at the same commitment, or they will compare against a different bank.
// Include compute budget instructions directly in the instruction array to ensure they are
// always present (tx modifiers can silently fail on certain transaction shapes). The CU limit
// and heap frame are set to the network maximums since simulation does not pay for them.
func BuildReadOnlySimulationTx(
	payer solana.PublicKey,
	instructions ...solana.Instruction,
) (*solana.Transaction, *rpc.SimulateTransactionOpts, error) {
	return BuildReadOnlySimulationTxWithLookupTables(payer, nil, instructions...)
}

// BuildReadOnlySimulationTxWithLookupTables is BuildReadOnlySimulationTx plus optional address
// lookup tables, for when the instruction account list is compressed.
func BuildReadOnlySimulationTxWithLookupTables(
	payer solana.PublicKey,
	lookupTables map[solana.PublicKey]solana.PublicKeySlice,
	instructions ...solana.Instruction,
) (*solana.Transaction, *rpc.SimulateTransactionOpts, error) {
	// A tx carrying only the compute-budget preamble simulates successfully but exercises nothing,
	// so an empty list is a caller bug rather than a degenerate-but-valid request.
	if len(instructions) == 0 {
		return nil, nil, errors.New("no instructions to simulate")
	}

	cuLimitIx, err := computebudget.NewSetComputeUnitLimitInstruction(computebudget.MAX_COMPUTE_UNIT_LIMIT).ValidateAndBuild()
	if err != nil {
		return nil, nil, fmt.Errorf("build compute unit limit instruction: %w", err)
	}
	// Programs that install their own allocator (the `custom-heap` feature — currently ccip_router,
	// ccip_offramp and test_ccip_receiver) grow into this frame; the rest keep Anchor's fixed 32KB
	// allocator and ignore it. Account resolution runs the same code paths as execution, so it needs
	// the same frame.
	heapFrameIx, err := computebudget.NewRequestHeapFrameInstruction(computebudget.MAX_HEAP_FRAME_BYTES).ValidateAndBuild()
	if err != nil {
		return nil, nil, fmt.Errorf("build heap frame instruction: %w", err)
	}

	txOpts := []solana.TransactionOption{solana.TransactionPayer(payer)}
	if len(lookupTables) > 0 {
		txOpts = append(txOpts, solana.TransactionAddressTables(lookupTables))
	}

	tx, err := solana.NewTransaction(
		append([]solana.Instruction{cuLimitIx, heapFrameIx}, instructions...),
		solana.Hash{}, // replaced by RPC because ReplaceRecentBlockhash is true
		txOpts...,
	)
	if err != nil {
		return nil, nil, err
	}
	tx.Signatures = []solana.Signature{{}} // placeholder, SigVerify is false for read-only simulation

	return tx, &rpc.SimulateTransactionOpts{
		ReplaceRecentBlockhash: true,
		SigVerify:              false,
		Commitment:             rpc.CommitmentConfirmed,
	}, nil
}

func ExtractReturnValue(_ context.Context, logs []string, programID string) ([]byte, error) {
	if logs == nil {
		return []byte{}, nil
	}

	for _, log := range logs {
		if strings.HasPrefix(log, "Program return: "+programID) {
			parts := strings.Split(log, " ")
			encoded := parts[len(parts)-1]
			return base64.StdEncoding.DecodeString(encoded)
		}
	}
	return []byte{}, nil
}

func ExtractReturnedError(_ context.Context, logs []string, _ string) *string {
	if logs == nil {
		return nil
	}

	for _, log := range logs {
		if strings.Contains(log, "Error Code: ") {
			parts := strings.Split(log, "Error Code: ")
			if len(parts) > 1 {
				codePart := strings.Split(parts[1], ".")[0]
				return &codePart
			}
		}
	}
	return nil
}

type anchorType[T any] interface {
	*T
	UnmarshalWithDecoder(decoder *bin.Decoder) (err error)
}

func ExtractAnchorTypedReturnValue[T any, PT anchorType[T]](ctx context.Context, logs []string, programID string) (*T, error) {
	var result T
	bytes, err := ExtractReturnValue(ctx, logs, programID)
	if err != nil {
		return nil, err
	}
	err = PT(&result).UnmarshalWithDecoder(bin.NewBorshDecoder(bytes))
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func ExtractTypedReturnValue[T any](ctx context.Context, logs []string, programID string, decoderFn func([]byte) T) (T, error) {
	bytes, err := ExtractReturnValue(ctx, logs, programID)
	return decoderFn(bytes), err
}

// hard-coded method discriminator for the emit_cpi! handler
// https://github.com/solana-foundation/anchor/blob/7b108db7987afedd1e72edd783d7fc00416e213f/lang/src/event.rs#L2
const (
	anchorCPIEventHandlerDiscrim uint64 = 0x1d9acb512ea545e4
	dl                           int    = 8 // Anchor CPI event discriminator length
)

// ExtractCPIEvent extracts a CPI event emitted via Anchor's emit_cpi! macro from a
// transaction's inner instructions. It scans inner instructions for those targeting
// the given programID, strips the 8-byte Anchor CPI event header, and returns the
// first event that parseFn successfully parses.
func ExtractCPIEvent[T any](
	tx *solana.Transaction,
	meta *rpc.TransactionMeta,
	programID solana.PublicKey,
	parseEvent func([]byte) (*T, error),
) (*T, error) {
	if meta == nil || meta.InnerInstructions == nil {
		return nil, errors.New("transaction meta with innerInstructions is required to extract CPI events")
	}
	allKeys := GetAllKeys(tx, meta)
	for _, innerIxSet := range meta.InnerInstructions {
		for _, ix := range innerIxSet.Instructions {
			if int(ix.ProgramIDIndex) >= len(allKeys) {
				continue
			}
			if allKeys[ix.ProgramIDIndex] != programID {
				continue
			}
			if len(ix.Data) <= dl {
				continue
			}
			if anchorCPIEventHandlerDiscrim != bin.LE.Uint64(ix.Data[:dl]) {
				continue
			}
			event, err := parseEvent(ix.Data[dl:])
			if err == nil {
				return event, nil
			}
		}
	}
	return nil, errors.New("CPI event not found in inner instructions")
}

func ExtractCPI(
	tx *solana.Transaction,
	meta *rpc.TransactionMeta,
	programID solana.PublicKey,
	idx int, // if there are many CPIs to the same program, look for the idx-th one (0-based index)
) (*rpc.CompiledInstruction, error) {
	if meta == nil || meta.InnerInstructions == nil {
		return nil, errors.New("transaction meta with innerInstructions is required to extract CPI")
	}
	allKeys := GetAllKeys(tx, meta)
	seen := 0
	for _, innerIxSet := range meta.InnerInstructions {
		for _, ix := range innerIxSet.Instructions {
			if int(ix.ProgramIDIndex) >= len(allKeys) {
				continue
			}
			if allKeys[ix.ProgramIDIndex] != programID {
				continue
			}
			if seen == idx {
				return &ix, nil
			}
			seen++
		}
	}
	return nil, errors.New("CPI not found in inner instructions")
}

func GetAllKeys(tx *solana.Transaction, meta *rpc.TransactionMeta) []solana.PublicKey {
	allKeys := make([]solana.PublicKey, 0, len(tx.Message.AccountKeys))
	allKeys = append(allKeys, tx.Message.AccountKeys...)
	if meta != nil && meta.LoadedAddresses.Writable != nil {
		allKeys = append(allKeys, meta.LoadedAddresses.Writable...)
	}
	if meta != nil && meta.LoadedAddresses.ReadOnly != nil {
		allKeys = append(allKeys, meta.LoadedAddresses.ReadOnly...)
	}
	return allKeys
}

func GetAccountDataBorshInto(ctx context.Context, solanaGoClient *rpc.Client, account solana.PublicKey, commitment rpc.CommitmentType, data any) error {
	resp, err := solanaGoClient.GetAccountInfoWithOpts(
		ctx,
		account,
		&rpc.GetAccountInfoOpts{
			Commitment: commitment,
			DataSlice:  nil,
		},
	)
	if err != nil {
		return err
	}
	return bin.NewBorshDecoder(resp.Value.Data.GetBinary()).Decode(data)
}

// GetAccountDataBorshIntoAnchor fetches account data, strips the 8-byte Anchor discriminator,
// and deserializes the remaining bytes into the provided data structure.
// Use this for types generated by anchor-go v1.0.0 (latest bindings), where
// UnmarshalWithDecoder no longer reads the discriminator internally.
func GetAccountDataBorshIntoAnchor(ctx context.Context, solanaGoClient *rpc.Client, account solana.PublicKey, commitment rpc.CommitmentType, data any) error {
	resp, err := solanaGoClient.GetAccountInfoWithOpts(
		ctx,
		account,
		&rpc.GetAccountInfoOpts{
			Commitment: commitment,
			DataSlice:  nil,
		},
	)
	if err != nil {
		return err
	}
	raw := resp.Value.Data.GetBinary()
	if len(raw) < 8 {
		return fmt.Errorf("account data too short to contain anchor discriminator: %d bytes", len(raw))
	}
	return bin.NewBorshDecoder(raw[8:]).Decode(data)
}

// GetParsedAccountData fetches on-chain account data and parses it using the provided
// parser function (typically a generated ParseAccount_* function from the latest bindings).
// This provides type-safe deserialization with discriminator validation.
func GetParsedAccountData[T any](ctx context.Context, solanaGoClient *rpc.Client, account solana.PublicKey, commitment rpc.CommitmentType, parser func([]byte) (*T, error)) (*T, error) {
	resp, err := solanaGoClient.GetAccountInfoWithOpts(
		ctx,
		account,
		&rpc.GetAccountInfoOpts{
			Commitment: commitment,
			DataSlice:  nil,
		},
	)
	if err != nil {
		return nil, err
	}
	return parser(resp.Value.Data.GetBinary())
}

func IsClosedAccount(ctx context.Context, solanaGoClient *rpc.Client, accountKey solana.PublicKey, commitment rpc.CommitmentType) bool {
	_, err := solanaGoClient.GetAccountInfoWithOpts(ctx, accountKey, &rpc.GetAccountInfoOpts{
		Commitment: commitment,
	})
	return errors.Is(err, rpc.ErrNotFound)
}
