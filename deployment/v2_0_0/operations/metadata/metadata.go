// Package metadata builds instructions for the SPL Program Metadata program,
// which attaches documents -- an IDL, a security.txt -- to an already deployed
// program.
//
// Anchor used to expose IDL management as instructions on the target program
// itself. anchor-lang 1.x removed them: they survive only behind
// #[program(legacy_idl)], which upstream deprecates as "the source of security
// issues in the past" and points here instead. Our programs are on 1.1.2 and do
// not opt in, so they answer InstructionFallbackNotFound to the old tag.
//
// Program Metadata is a separate deployed program, so nothing has to be
// compiled into ours and no redeploy is needed to publish or update a document.
// It is live on mainnet-beta and devnet at ProgramID below.
//
// Layouts follow program-metadata v1.0.0, and are hand-encoded because there
// are no gobindings for it:
// https://github.com/solana-program/program-metadata
package metadata

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
)

// ProgramID is the Program Metadata program, deployed at the same address on
// every cluster.
var ProgramID = solana.MustPublicKeyFromBase58("ProgM6JCCvbYkfKqJYHePx4xxSUSqJp7rh8Lyv7nk7S")

const (
	// HeaderLen is the fixed prefix on metadata and buffer accounts alike --
	// both headers are exactly 96 bytes -- after which the document starts.
	HeaderLen = 96

	// SeedLen is the width of the seed field. Shorter seeds are zero padded,
	// and the padded form is what the PDA is derived from.
	SeedLen = 16
)

// Standard seeds. The seed is what distinguishes one document from another on
// the same program, so these spellings are load-bearing: explorers look for
// exactly these.
const (
	SeedIDL      = "idl"
	SeedSecurity = "security"
)

// Instruction discriminators, a single leading byte each.
const (
	ixWrite uint8 = iota
	ixInitialize
	ixSetAuthority
	ixSetData
	ixSetImmutable
	ixTrim
	ixClose
	ixAllocate
	ixExtend
)

// Encoding describes how the stored bytes decode to text.
type Encoding uint8

const (
	EncodingNone Encoding = iota
	EncodingUTF8
	EncodingBase58
	EncodingBase64
)

// Compression is applied to the document before it is written.
type Compression uint8

const (
	CompressionNone Compression = iota
	CompressionGzip
	CompressionZlib
)

// Format is the document's type once decoded and decompressed.
type Format uint8

const (
	FormatNone Format = iota
	FormatJSON
	FormatYAML
	FormatTOML
)

// DataSource says whether the account holds the document or only points at it.
type DataSource uint8

const (
	DataSourceDirect DataSource = iota
	DataSourceURL
	DataSourceExternal
)

// EncodeSeed zero pads a seed to the fixed width used in both the instruction
// data and the PDA derivation.
func EncodeSeed(seed string) ([SeedLen]byte, error) {
	var out [SeedLen]byte
	if seed == "" {
		return out, errors.New("metadata seed must not be empty")
	}
	if len(seed) > SeedLen {
		return out, fmt.Errorf("metadata seed %q is %d bytes, limit is %d", seed, len(seed), SeedLen)
	}
	copy(out[:], seed)

	return out, nil
}

// CanonicalPDA derives the metadata account that only the program's upgrade
// authority can create. There is at most one per (program, seed), which is what
// makes it trustworthy: anyone else's document lands on a NonCanonicalPDA.
func CanonicalPDA(program solana.PublicKey, seed string) (solana.PublicKey, error) {
	padded, err := EncodeSeed(seed)
	if err != nil {
		return solana.PublicKey{}, err
	}
	addr, _, err := solana.FindProgramAddress([][]byte{program.Bytes(), padded[:]}, ProgramID)

	return addr, err
}

// NonCanonicalPDA derives the metadata account for a third party authority.
func NonCanonicalPDA(program, authority solana.PublicKey, seed string) (solana.PublicKey, error) {
	padded, err := EncodeSeed(seed)
	if err != nil {
		return solana.PublicKey{}, err
	}
	addr, _, err := solana.FindProgramAddress(
		[][]byte{program.Bytes(), authority.Bytes(), padded[:]}, ProgramID)

	return addr, err
}

// optional resolves an absent optional account. The program keeps every
// instruction's account list a fixed length and uses its own ID to mean "not
// provided", so a zero key is translated rather than dropped.
func optional(key solana.PublicKey) solana.PublicKey {
	if key.IsZero() {
		return ProgramID
	}

	return key
}

func meta(key solana.PublicKey, writable, signer bool) *solana.AccountMeta {
	return &solana.AccountMeta{PublicKey: key, IsWritable: writable, IsSigner: signer}
}

// AllocateBufferInstruction creates a scratch buffer owned by an ephemeral
// keypair. The program recognises a buffer as a keypair buffer -- rather than a
// PDA, which would have to belong to the upgrade authority -- when the buffer
// and the authority are the same account, and then it takes no seed.
//
// Using a keypair buffer is what keeps the chunked upload off MCMS: the
// deployer key funds and fills it, and only the final SetData is signed by the
// upgrade authority. That mirrors how program upgrades already work.
//
// The caller must create the buffer account first, at its full size and
// already owned by the metadata program, so that this instruction only stamps
// the header. Left to create the account itself the program would CPI into
// system instruction 13, CreateAccountAllowPrefund, which Agave only gained
// recently -- a validator without it rejects the whole publish with "invalid
// instruction data". Creating the account up front takes the Assign branch
// instead, which works everywhere.
func AllocateBufferInstruction(buffer solana.PublicKey) *solana.GenericInstruction {
	accounts := solana.AccountMetaSlice{
		meta(buffer, true, true),
		meta(buffer, false, true), // authority == buffer marks it a keypair buffer
		meta(ProgramID, false, false),
		meta(ProgramID, false, false),
		meta(system.ProgramID, false, false),
	}

	return solana.NewInstruction(ProgramID, accounts, []byte{ixAllocate})
}

// WriteInstruction appends a chunk at offset, relative to the start of the
// document rather than the account.
func WriteInstruction(buffer, authority solana.PublicKey, offset uint32, chunk []byte) *solana.GenericInstruction {
	data := []byte{ixWrite}
	data = binary.LittleEndian.AppendUint32(data, offset)
	data = append(data, chunk...)

	accounts := solana.AccountMetaSlice{
		meta(buffer, true, false),
		meta(authority, false, true),
		meta(ProgramID, false, false), // no source buffer: the bytes are inline
	}

	return solana.NewInstruction(ProgramID, accounts, data)
}

// InitializeInstruction creates the metadata account. The account is canonical
// when authority is the program's upgrade authority, which is the only case we
// use; programData is what the program checks that against.
//
// Initialize takes no buffer account, so a document too large to fit in one
// transaction is created empty here and filled by SetDataFromBuffer.
func InitializeInstruction(
	metadataAccount, authority, program, programData solana.PublicKey,
	seed string,
	encoding Encoding, compression Compression, format Format, source DataSource,
	inline []byte,
) (*solana.GenericInstruction, error) {
	padded, err := EncodeSeed(seed)
	if err != nil {
		return nil, err
	}

	data := make([]byte, 0, 1+len(padded)+4+len(inline))
	data = append(data, ixInitialize)
	data = append(data, padded[:]...)
	data = append(data, byte(encoding), byte(compression), byte(format), byte(source))
	data = append(data, inline...)

	accounts := solana.AccountMetaSlice{
		meta(metadataAccount, true, false),
		meta(authority, false, true),
		meta(program, false, false),
		meta(optional(programData), false, false),
		meta(system.ProgramID, false, false),
	}

	return solana.NewInstruction(ProgramID, accounts, data), nil
}

// SetDataFromBuffer replaces the document with the contents of buffer. This is
// the only instruction in a publish that the upgrade authority has to sign, so
// it is the one that becomes an MCMS operation.
func SetDataFromBuffer(
	metadataAccount, authority, buffer, program, programData solana.PublicKey,
	encoding Encoding, compression Compression, format Format,
) *solana.GenericInstruction {
	// The trailing data_source byte with no bytes after it is what tells the
	// program to read from the buffer account instead of the instruction.
	data := []byte{ixSetData, byte(encoding), byte(compression), byte(format), byte(DataSourceDirect)}

	accounts := solana.AccountMetaSlice{
		meta(metadataAccount, true, false),
		meta(authority, false, true),
		meta(buffer, true, false),
		meta(program, false, false),
		meta(optional(programData), false, false),
	}

	return solana.NewInstruction(ProgramID, accounts, data)
}

// SetDataInline replaces the document with bytes carried in the instruction,
// for documents small enough to avoid a buffer round trip.
func SetDataInline(
	metadataAccount, authority, program, programData solana.PublicKey,
	encoding Encoding, compression Compression, format Format, source DataSource,
	document []byte,
) (*solana.GenericInstruction, error) {
	if len(document) == 0 {
		return nil, errors.New("inline set data requires a document")
	}

	data := make([]byte, 0, 5+len(document))
	data = append(data, ixSetData, byte(encoding), byte(compression), byte(format), byte(source))
	data = append(data, document...)

	accounts := solana.AccountMetaSlice{
		meta(metadataAccount, true, false),
		meta(authority, false, true),
		meta(ProgramID, false, false), // no buffer
		meta(program, false, false),
		meta(optional(programData), false, false),
	}

	return solana.NewInstruction(ProgramID, accounts, data), nil
}

// CloseInstruction deletes a metadata or buffer account and refunds its rent to
// destination.
func CloseInstruction(account, authority, program, programData, destination solana.PublicKey) *solana.GenericInstruction {
	accounts := solana.AccountMetaSlice{
		meta(account, true, false),
		meta(authority, false, true),
		meta(optional(program), false, false),
		meta(optional(programData), false, false),
		meta(destination, true, false),
	}

	return solana.NewInstruction(ProgramID, accounts, []byte{ixClose})
}

// SetAuthorityInstruction changes who may update the account. A nil authority
// clears it, which for a canonical account hands control back to whoever holds
// the program's upgrade authority.
func SetAuthorityInstruction(
	account, authority, program, programData solana.PublicKey,
	newAuthority *solana.PublicKey,
) *solana.GenericInstruction {
	data := []byte{ixSetAuthority}
	if newAuthority == nil {
		data = append(data, 0)
	} else {
		data = append(data, 1)
		data = append(data, newAuthority.Bytes()...)
	}

	accounts := solana.AccountMetaSlice{
		meta(account, true, false),
		meta(authority, false, true),
		meta(optional(program), false, false),
		meta(optional(programData), false, false),
	}

	return solana.NewInstruction(ProgramID, accounts, data)
}

// SetImmutableInstruction permanently freezes the document.
func SetImmutableInstruction(metadataAccount, authority, program, programData solana.PublicKey) *solana.GenericInstruction {
	accounts := solana.AccountMetaSlice{
		meta(metadataAccount, true, false),
		meta(authority, false, true),
		meta(optional(program), false, false),
		meta(optional(programData), false, false),
	}

	return solana.NewInstruction(ProgramID, accounts, []byte{ixSetImmutable})
}

// ExtendInstruction grows an account by length bytes. The runtime caps a single
// reallocation, so a document larger than that is grown in steps before it is
// installed.
func ExtendInstruction(account, authority, program, programData solana.PublicKey, length uint16) *solana.GenericInstruction {
	data := []byte{ixExtend}
	data = binary.LittleEndian.AppendUint16(data, length)

	accounts := solana.AccountMetaSlice{
		meta(account, true, false),
		meta(authority, false, true),
		meta(optional(program), false, false),
		meta(optional(programData), false, false),
	}

	return solana.NewInstruction(ProgramID, accounts, data)
}

// Account is the decoded header of a metadata account plus its document.
type Account struct {
	Program     solana.PublicKey
	Authority   *solana.PublicKey // nil once cleared
	Mutable     bool
	Canonical   bool
	Seed        string
	Encoding    Encoding
	Compression Compression
	Format      Format
	DataSource  DataSource
	Data        []byte
}

// Offsets within the 96-byte metadata header.
const (
	offProgram     = 1
	offAuthority   = 33
	offMutable     = 65
	offCanonical   = 66
	offSeed        = 67
	offEncoding    = 83
	offCompression = 84
	offFormat      = 85
	offDataSource  = 86
	offDataLength  = 87 // u32, then 5 bytes of padding to reach HeaderLen
)

const discriminatorMetadata = 2

// ParseAccount decodes a metadata account. Data is returned exactly as stored,
// still compressed; use Decompress for the document itself.
func ParseAccount(raw []byte) (Account, error) {
	var acct Account
	if len(raw) < HeaderLen {
		return acct, fmt.Errorf("metadata account is %d bytes, need at least %d", len(raw), HeaderLen)
	}
	if raw[0] != discriminatorMetadata {
		return acct, fmt.Errorf("account discriminator is %d, want %d for metadata", raw[0], discriminatorMetadata)
	}

	acct.Program = solana.PublicKeyFromBytes(raw[offProgram:offAuthority])

	// An all-zero authority means "none" rather than the zero address.
	if authority := solana.PublicKeyFromBytes(raw[offAuthority:offMutable]); !authority.IsZero() {
		acct.Authority = &authority
	}

	acct.Mutable = raw[offMutable] != 0
	acct.Canonical = raw[offCanonical] != 0

	seed := raw[offSeed:offEncoding]
	if end := indexZero(seed); end >= 0 {
		seed = seed[:end]
	}
	acct.Seed = string(seed)

	acct.Encoding = Encoding(raw[offEncoding])
	acct.Compression = Compression(raw[offCompression])
	acct.Format = Format(raw[offFormat])
	acct.DataSource = DataSource(raw[offDataSource])

	length := binary.LittleEndian.Uint32(raw[offDataLength : offDataLength+4])
	body := raw[HeaderLen:]
	if int(length) > len(body) {
		return acct, fmt.Errorf("metadata header declares %d bytes but account holds %d", length, len(body))
	}
	acct.Data = body[:length]

	return acct, nil
}

func indexZero(b []byte) int {
	for i, c := range b {
		if c == 0 {
			return i
		}
	}

	return -1
}
