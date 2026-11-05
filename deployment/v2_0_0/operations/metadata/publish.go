package metadata

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
)

// MaxChunkSize is the document bytes carried by one Write.
//
// A transaction is capped at 1232 bytes. A write costs two signatures (the fee
// payer and the buffer keypair), four account keys, the blockhash and the
// 5-byte instruction prefix -- a little over 300 bytes. The rest could hold
// ~890, but the margin is deliberate: a compute budget instruction or an extra
// account added later should not silently push existing documents over.
const MaxChunkSize = 700

// MaxInlineDocument is the compressed size that still fits in an Initialize or
// SetData instruction. Below it a document needs no buffer at all, which is the
// common case for a security.txt.
const MaxInlineDocument = 700

// MaxDataIncrease is how far the runtime lets an account grow in one
// instruction. A document that needs more arrives in steps.
const MaxDataIncrease = 10 * 1024

// MaxTransactionDataIncrease is how far accounts may grow across all the
// instructions of one transaction. Inner instructions share this budget with
// the one that invoked them, so everything an MCMS batch runs through a single
// timelock ExecuteBatch has to fit in it together.
const MaxTransactionDataIncrease = MaxDataIncrease

// BufferSpace is the account size a buffer needs to hold payload.
func BufferSpace(payloadLen int) int {
	return HeaderLen + payloadLen
}

// Compress deflates a document. Program Metadata records the algorithm in the
// header, so readers decompress without being told.
func Compress(document []byte) ([]byte, error) {
	var buf bytes.Buffer

	w := zlib.NewWriter(&buf)
	if _, err := w.Write(document); err != nil {
		return nil, fmt.Errorf("compress document: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("finish compressing document: %w", err)
	}

	return buf.Bytes(), nil
}

// Decompress inflates a document read back from an account.
func Decompress(payload []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("open compressed document: %w", err)
	}
	defer r.Close()

	document, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("decompress document: %w", err)
	}

	return document, nil
}

// LoadJSON reads a JSON document and returns it compacted, so that formatting
// differences between builds do not change what is published.
func LoadJSON(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return nil, fmt.Errorf("%s is not valid json: %w", path, err)
	}

	return compact.Bytes(), nil
}

// Chunk splits a payload into Write-sized pieces.
func Chunk(payload []byte) [][]byte {
	if len(payload) == 0 {
		return nil
	}

	chunks := make([][]byte, 0, (len(payload)+MaxChunkSize-1)/MaxChunkSize)
	for start := 0; start < len(payload); start += MaxChunkSize {
		end := min(start+MaxChunkSize, len(payload))
		chunks = append(chunks, payload[start:end])
	}

	return chunks
}

// PublishConfig describes one document to attach to one program.
type PublishConfig struct {
	// Program is the deployed program the document describes.
	Program solana.PublicKey
	// ProgramData is the program's ProgramData account. The metadata program
	// reads the upgrade authority from it to decide whether the write is
	// canonical, so it is required for the canonical path.
	ProgramData solana.PublicKey
	// Authority signs Initialize and SetData. For a canonical document this is
	// the program's upgrade authority -- under governance, the timelock signer
	// PDA -- which is why those two instructions become MCMS operations.
	Authority solana.PublicKey
	// Payer funds the buffer and signs the chunked upload.
	Payer solana.PublicKey
	// Buffer is an ephemeral keypair that acts as both the scratch account and
	// its own authority. It must sign Allocate and every Write.
	Buffer solana.PublicKey
	// RentExemptFor reports the lamports needed to keep an account of the given
	// size rent exempt, normally the RPC call of the same name. The buffer size
	// is only known after compression, so the plan asks rather than being told.
	RentExemptFor func(space int) (uint64, error)
	// Seed selects the document, e.g. SeedIDL or SeedSecurity.
	Seed string
	// Format is the document type, e.g. FormatJSON.
	Format Format
	// Document is the uncompressed bytes to publish.
	Document []byte
	// Exists skips Initialize, for republishing over an existing account.
	Exists bool
	// CurrentLamports is the existing account's balance, used to fund only the
	// shortfall to rent exemption. Zero is safe: the plan then funds the full
	// amount, as it must for an account that does not exist yet.
	CurrentLamports uint64
	// CurrentSize is the existing account's size, used to work out how far it
	// has to grow. Zero is safe: the plan then assumes the smallest account and
	// may step up more than strictly necessary.
	CurrentSize int
}

// PublishPlan is a publish split by who has to sign it.
//
// The division is the point: everything the deployer key can do is separated
// from the instructions that need the upgrade authority, so a governed publish
// puts two instructions through MCMS rather than every chunk of a large IDL.
type PublishPlan struct {
	// Metadata is the canonical account the document lands in.
	Metadata solana.PublicKey
	// Buffer is the scratch account the upload streams through. Zero when the
	// document was small enough to travel inside the instruction.
	Buffer solana.PublicKey
	// BufferSpace is the size the buffer must be rent exempt at.
	BufferSpace int
	// PayloadLen is the compressed size actually published.
	PayloadLen int

	// Prepare creates and funds the accounts. It is signed by the payer and,
	// when there is one, the buffer, and can run directly.
	Prepare []solana.Instruction
	// Writes fill the buffer, one transaction each, in order. Signed by the
	// buffer.
	Writes []solana.Instruction

	// Initialize creates the metadata account. Nil when it already exists.
	Initialize solana.Instruction
	// Extends grow the account when the document cannot be installed in a
	// single reallocation.
	Extends []solana.Instruction
	// SetData installs the document. Nil when Initialize already carried it.
	SetData solana.Instruction

	// InitializeGrowth and SetDataGrowth are how many bytes those instructions
	// add to the metadata account. Extends each add MaxDataIncrease. They let
	// GovernedGroups keep every transaction inside the runtime's growth budget.
	InitializeGrowth int
	SetDataGrowth    int

	// CloseBuffer reclaims the buffer rent to the payer once the document has
	// landed. Nil when no buffer was used.
	CloseBuffer solana.Instruction
}

// Governed returns the instructions that the upgrade authority must sign, in
// order. These are the ones to wrap as MCMS operations.
func (p PublishPlan) Governed() []solana.Instruction {
	ixs := make([]solana.Instruction, 0, len(p.Extends)+2)
	if p.Initialize != nil {
		ixs = append(ixs, p.Initialize)
	}
	ixs = append(ixs, p.Extends...)
	if p.SetData != nil {
		ixs = append(ixs, p.SetData)
	}

	return ixs
}

// GovernedGroups splits Governed into runs that can each share a transaction.
// The runtime bounds how much accounts may grow within one transaction, and an
// MCMS batch executes as a single one, so a run closes as soon as adding the
// next instruction would pass MaxTransactionDataIncrease. A small document
// stays in one run; a large one gets a run per Extend. Wrap each run as its own
// MCMS operation.
func (p PublishPlan) GovernedGroups() [][]solana.Instruction {
	type step struct {
		ix     solana.Instruction
		growth int
	}
	steps := make([]step, 0, len(p.Extends)+2)
	if p.Initialize != nil {
		steps = append(steps, step{p.Initialize, p.InitializeGrowth})
	}
	for _, ix := range p.Extends {
		steps = append(steps, step{ix, MaxDataIncrease})
	}
	if p.SetData != nil {
		steps = append(steps, step{p.SetData, p.SetDataGrowth})
	}

	var groups [][]solana.Instruction
	current := make([]solana.Instruction, 0, len(steps))
	used := 0
	for _, st := range steps {
		if len(current) > 0 && used+st.growth > MaxTransactionDataIncrease {
			groups = append(groups, current)
			current, used = make([]solana.Instruction, 0, len(steps)), 0
		}
		current = append(current, st.ix)
		used += st.growth
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}

	return groups
}

// PlanPublish compresses the document and lays out the instructions to publish
// it. Nothing is sent; the caller decides which halves to execute directly and
// which to route through governance.
func PlanPublish(cfg PublishConfig) (PublishPlan, error) {
	if len(cfg.Document) == 0 {
		return PublishPlan{}, fmt.Errorf("no document to publish for seed %q", cfg.Seed)
	}
	if cfg.Program.IsZero() {
		return PublishPlan{}, fmt.Errorf("program is required to publish seed %q", cfg.Seed)
	}
	if cfg.RentExemptFor == nil {
		return PublishPlan{}, errors.New("RentExemptFor is required to fund the accounts")
	}

	metadataAccount, err := CanonicalPDA(cfg.Program, cfg.Seed)
	if err != nil {
		return PublishPlan{}, err
	}

	payload, err := Compress(cfg.Document)
	if err != nil {
		return PublishPlan{}, err
	}

	plan := PublishPlan{Metadata: metadataAccount, PayloadLen: len(payload)}

	// A document that fits in one transaction skips the buffer entirely and
	// costs a single governed instruction.
	if len(payload) <= MaxInlineDocument {
		// Neither creating the account nor growing it draws lamports from a
		// funder, so it has to be rent exempt at its final size before the
		// authority's instruction runs. That applies to a republish too: a
		// document larger than the current account reallocates it, and without
		// the top-up the metadata instruction fails.
		if fErr := fundAccount(&plan, cfg, metadataAccount, HeaderLen+len(payload)); fErr != nil {
			return PublishPlan{}, fErr
		}

		if cfg.Exists {
			plan.SetDataGrowth = max(0, HeaderLen+len(payload)-cfg.CurrentSize)
			plan.SetData, err = SetDataInline(
				metadataAccount, cfg.Authority, cfg.Program, cfg.ProgramData,
				EncodingUTF8, CompressionZlib, cfg.Format, DataSourceDirect, payload)

			return plan, err
		}

		plan.InitializeGrowth = HeaderLen + len(payload)
		plan.Initialize, err = InitializeInstruction(
			metadataAccount, cfg.Authority, cfg.Program, cfg.ProgramData,
			cfg.Seed, EncodingUTF8, CompressionZlib, cfg.Format, DataSourceDirect, payload)

		return plan, err
	}

	if cfg.Buffer.IsZero() {
		return PublishPlan{}, fmt.Errorf(
			"a %d byte document needs a buffer keypair to publish seed %q", len(payload), cfg.Seed)
	}

	plan.Buffer = cfg.Buffer
	plan.BufferSpace = BufferSpace(len(payload))

	bufferRent, err := cfg.RentExemptFor(plan.BufferSpace)
	if err != nil {
		return PublishPlan{}, fmt.Errorf("rent for a %d byte buffer: %w", plan.BufferSpace, err)
	}

	// The buffer is created at its full size and already owned by the metadata
	// program, so AllocateBufferInstruction only has to stamp the header. See
	// its comment for why it is not left to create the account itself.
	plan.Prepare = []solana.Instruction{
		system.NewCreateAccountInstruction(
			//nolint:gosec // BufferSpace is HeaderLen plus a payload length, never negative
			bufferRent, uint64(plan.BufferSpace), ProgramID, cfg.Payer, cfg.Buffer).Build(),
		AllocateBufferInstruction(cfg.Buffer),
	}

	var offset uint32
	for _, chunk := range Chunk(payload) {
		plan.Writes = append(plan.Writes, WriteInstruction(cfg.Buffer, cfg.Buffer, offset, chunk))
		offset += uint32(len(chunk)) //nolint:gosec // chunks sum to len(payload), an int
	}

	// The metadata account has to be rent exempt at its final size before the
	// authority's instructions run, because neither creating nor growing it
	// draws lamports from a funder.
	if err = fundAccount(&plan, cfg, metadataAccount, HeaderLen+len(payload)); err != nil {
		return PublishPlan{}, err
	}

	size := HeaderLen
	if !cfg.Exists {
		// Initialize refuses to create an empty account, so it carries a one
		// byte placeholder that SetData immediately replaces.
		plan.Initialize, err = InitializeInstruction(
			metadataAccount, cfg.Authority, cfg.Program, cfg.ProgramData,
			cfg.Seed, EncodingUTF8, CompressionZlib, cfg.Format, DataSourceDirect, []byte{0})
		if err != nil {
			return PublishPlan{}, err
		}
		size = HeaderLen + 1
		plan.InitializeGrowth = size
	} else if cfg.CurrentSize > size {
		size = cfg.CurrentSize
	}

	// An account can only grow so far in one instruction, so anything beyond
	// that is stepped up first and SetData just installs the bytes.
	target := HeaderLen + len(payload)
	for target-size > MaxDataIncrease {
		plan.Extends = append(plan.Extends,
			ExtendInstruction(metadataAccount, cfg.Authority, cfg.Program, cfg.ProgramData, MaxDataIncrease))
		size += MaxDataIncrease
	}

	plan.SetDataGrowth = max(0, target-size)
	plan.SetData = SetDataFromBuffer(
		metadataAccount, cfg.Authority, cfg.Buffer, cfg.Program, cfg.ProgramData,
		EncodingUTF8, CompressionZlib, cfg.Format)

	plan.CloseBuffer = CloseInstruction(
		cfg.Buffer, cfg.Buffer, solana.PublicKey{}, solana.PublicKey{}, cfg.Payer)

	return plan, nil
}

// fundAccount tops the account up to rent exemption at space. Creating and
// growing a metadata account both take the lamports from the account itself
// rather than from a funder, so the payer has to put them there first.
func fundAccount(plan *PublishPlan, cfg PublishConfig, account solana.PublicKey, space int) error {
	rent, err := cfg.RentExemptFor(space)
	if err != nil {
		return fmt.Errorf("rent for a %d byte metadata account: %w", space, err)
	}
	// Only the shortfall is sent. An account that already exists holds rent for
	// its current size, and neither SetData nor Extend refunds an overpayment,
	// so transferring the full amount on every republish would strand lamports
	// in the PDA for good.
	if rent <= cfg.CurrentLamports {
		return nil
	}
	plan.Prepare = append(plan.Prepare,
		system.NewTransferInstruction(rent-cfg.CurrentLamports, cfg.Payer, account).Build())

	return nil
}
