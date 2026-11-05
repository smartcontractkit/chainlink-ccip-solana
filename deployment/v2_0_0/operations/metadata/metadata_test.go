package metadata

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"
)

var (
	testProgram     = solana.MustPublicKeyFromBase58("RmnXLft1mSEwDgMKu2okYuHkiazxntFFcZFrrcXxYg7")
	testProgramData = solana.MustPublicKeyFromBase58("9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM")
)

// stubRent stands in for the rent RPC. The guard is not ceremony: it is what
// makes the conversion provably safe rather than merely obviously safe.
func stubRent(space int) (uint64, error) {
	if space < 0 {
		return 0, fmt.Errorf("negative account space %d", space)
	}

	return uint64(space), nil
}

// mustData unwraps the instruction data, which the solana-go interface returns
// alongside an encoding error that hand-built instructions never produce.
func mustData(t *testing.T, ix solana.Instruction) []byte {
	t.Helper()
	data, err := ix.Data()
	require.NoError(t, err)

	return data
}

func TestEncodeSeedPadsAndRejects(t *testing.T) {
	padded, err := EncodeSeed(SeedIDL)
	require.NoError(t, err)
	require.Len(t, padded[:], SeedLen)
	require.Equal(t, []byte("idl"), padded[:3])
	require.Equal(t, make([]byte, SeedLen-3), padded[3:], "the tail must be zero padding")

	_, err = EncodeSeed("")
	require.Error(t, err, "an empty seed would collide across documents")

	_, err = EncodeSeed("this-seed-is-far-too-long")
	require.Error(t, err)
}

func TestCanonicalAndNonCanonicalPDAsDiffer(t *testing.T) {
	authority := solana.NewWallet().PublicKey()

	canonical, err := CanonicalPDA(testProgram, SeedIDL)
	require.NoError(t, err)

	again, err := CanonicalPDA(testProgram, SeedIDL)
	require.NoError(t, err)
	require.Equal(t, canonical, again, "derivation must be deterministic")

	// Different seeds are different documents on the same program.
	security, err := CanonicalPDA(testProgram, SeedSecurity)
	require.NoError(t, err)
	require.NotEqual(t, canonical, security)

	// The whole point of the canonical account is that a third party cannot
	// land on it.
	nonCanonical, err := NonCanonicalPDA(testProgram, authority, SeedIDL)
	require.NoError(t, err)
	require.NotEqual(t, canonical, nonCanonical)
}

func TestAbsentOptionalAccountsUseProgramIDSentinel(t *testing.T) {
	// The program keeps a fixed account list and reads its own ID as "not
	// provided", so a zero key must be translated rather than passed through.
	ix := CloseInstruction(
		solana.NewWallet().PublicKey(), solana.NewWallet().PublicKey(),
		solana.PublicKey{}, solana.PublicKey{}, solana.NewWallet().PublicKey())

	accounts := ix.Accounts()
	require.Len(t, accounts, 5)
	require.Equal(t, ProgramID, accounts[2].PublicKey, "absent program must be the sentinel")
	require.Equal(t, ProgramID, accounts[3].PublicKey, "absent programData must be the sentinel")
	require.False(t, accounts[2].PublicKey.IsZero())
}

func TestAllocateMarksKeypairBufferAsItsOwnAuthority(t *testing.T) {
	buffer := solana.NewWallet().PublicKey()

	ix := AllocateBufferInstruction(buffer)
	accounts := ix.Accounts()

	require.Equal(t, []byte{ixAllocate}, mustData(t, ix), "a keypair buffer carries no seed")
	require.Len(t, accounts, 5)

	// This equality is what tells the program the buffer is a keypair buffer
	// rather than a PDA, which is what keeps the upload off MCMS.
	require.Equal(t, buffer, accounts[0].PublicKey)
	require.Equal(t, buffer, accounts[1].PublicKey)
	require.True(t, accounts[0].IsWritable)
	require.True(t, accounts[1].IsSigner)
}

func TestWriteEncodesOffsetLittleEndian(t *testing.T) {
	buffer := solana.NewWallet().PublicKey()
	chunk := []byte("payload")

	ix := WriteInstruction(buffer, buffer, 0x01020304, chunk)
	data := mustData(t, ix)

	require.Equal(t, ixWrite, data[0])
	require.Equal(t, uint32(0x01020304), binary.LittleEndian.Uint32(data[1:5]))
	require.Equal(t, chunk, data[5:])
}

func TestInitializeLayout(t *testing.T) {
	authority := solana.NewWallet().PublicKey()
	metadataAccount, err := CanonicalPDA(testProgram, SeedIDL)
	require.NoError(t, err)

	ix, err := InitializeInstruction(
		metadataAccount, authority, testProgram, testProgramData,
		SeedIDL, EncodingUTF8, CompressionZlib, FormatJSON, DataSourceDirect, nil)
	require.NoError(t, err)

	data := mustData(t, ix)
	require.Len(t, data, 21, "discriminator, 16 byte seed, and four enum bytes")
	require.Equal(t, ixInitialize, data[0])
	require.Equal(t, []byte("idl"), data[1:4])
	require.Equal(t, byte(EncodingUTF8), data[17])
	require.Equal(t, byte(CompressionZlib), data[18])
	require.Equal(t, byte(FormatJSON), data[19])
	require.Equal(t, byte(DataSourceDirect), data[20])

	accounts := ix.Accounts()
	require.Equal(t, metadataAccount, accounts[0].PublicKey)
	require.True(t, accounts[0].IsWritable)
	require.Equal(t, authority, accounts[1].PublicKey)
	require.True(t, accounts[1].IsSigner)
	require.Equal(t, testProgram, accounts[2].PublicKey)
	require.Equal(t, testProgramData, accounts[3].PublicKey)
}

func TestSetDataFromBufferOmitsInlineBytes(t *testing.T) {
	authority := solana.NewWallet().PublicKey()
	buffer := solana.NewWallet().PublicKey()

	ix := SetDataFromBuffer(
		solana.NewWallet().PublicKey(), authority, buffer, testProgram, testProgramData,
		EncodingUTF8, CompressionZlib, FormatJSON)

	// A trailing data_source byte with nothing after it is precisely what tells
	// the program to read the buffer; appending bytes here would switch it to
	// the inline path and ignore the buffer.
	require.Len(t, mustData(t, ix), 5)
	require.Equal(t, ixSetData, mustData(t, ix)[0])
	require.Equal(t, byte(DataSourceDirect), mustData(t, ix)[4])

	accounts := ix.Accounts()
	require.Equal(t, buffer, accounts[2].PublicKey)
	require.True(t, accounts[2].IsWritable)
}

func TestSetDataInlineCarriesDocument(t *testing.T) {
	doc := []byte(`{"a":1}`)

	ix, err := SetDataInline(
		solana.NewWallet().PublicKey(), solana.NewWallet().PublicKey(), testProgram, testProgramData,
		EncodingUTF8, CompressionNone, FormatJSON, DataSourceDirect, doc)
	require.NoError(t, err)
	require.Equal(t, doc, mustData(t, ix)[5:])
	require.Equal(t, ProgramID, ix.Accounts()[2].PublicKey, "inline data uses no buffer")

	_, err = SetDataInline(
		solana.NewWallet().PublicKey(), solana.NewWallet().PublicKey(), testProgram, testProgramData,
		EncodingUTF8, CompressionNone, FormatJSON, DataSourceDirect, nil)
	require.Error(t, err, "an empty inline document would silently mean read-from-buffer")
}

func TestSetAuthorityOptionPrefix(t *testing.T) {
	account := solana.NewWallet().PublicKey()
	authority := solana.NewWallet().PublicKey()
	next := solana.NewWallet().PublicKey()

	set := SetAuthorityInstruction(account, authority, testProgram, testProgramData, &next)
	require.Equal(t, []byte{ixSetAuthority, 1}, mustData(t, set)[:2])
	require.Equal(t, next.Bytes(), mustData(t, set)[2:])

	cleared := SetAuthorityInstruction(account, authority, testProgram, testProgramData, nil)
	require.Equal(t, []byte{ixSetAuthority, 0}, mustData(t, cleared))
}

func TestChunkCoversPayloadExactly(t *testing.T) {
	require.Nil(t, Chunk(nil))

	for _, size := range []int{1, MaxChunkSize - 1, MaxChunkSize, MaxChunkSize + 1, 5 * MaxChunkSize} {
		payload := make([]byte, size)
		for i := range payload {
			payload[i] = byte(i)
		}

		var rebuilt []byte
		for _, chunk := range Chunk(payload) {
			require.LessOrEqual(t, len(chunk), MaxChunkSize)
			require.NotEmpty(t, chunk)
			rebuilt = append(rebuilt, chunk...)
		}
		require.Equal(t, payload, rebuilt, "chunks must reassemble for size %d", size)
	}
}

func TestCompressRoundTrip(t *testing.T) {
	doc := []byte(`{"name":"rmn_remote","instructions":[{"name":"init"}]}`)

	payload, err := Compress(doc)
	require.NoError(t, err)

	back, err := Decompress(payload)
	require.NoError(t, err)
	require.Equal(t, doc, back)
}

func TestParseAccountReadsHeader(t *testing.T) {
	authority := solana.NewWallet().PublicKey()

	const payloadLen = 16
	payload := []byte("compressed-bytes")
	require.Len(t, payload, payloadLen)

	raw := make([]byte, HeaderLen, HeaderLen+payloadLen)
	raw[0] = discriminatorMetadata
	copy(raw[offProgram:], testProgram.Bytes())
	copy(raw[offAuthority:], authority.Bytes())
	raw[offMutable] = 1
	raw[offCanonical] = 1
	copy(raw[offSeed:], SeedIDL)
	raw[offEncoding] = byte(EncodingUTF8)
	raw[offCompression] = byte(CompressionZlib)
	raw[offFormat] = byte(FormatJSON)
	raw[offDataSource] = byte(DataSourceDirect)
	binary.LittleEndian.PutUint32(raw[offDataLength:], payloadLen)
	raw = append(raw, payload...)

	acct, err := ParseAccount(raw)
	require.NoError(t, err)
	require.Equal(t, testProgram, acct.Program)
	require.NotNil(t, acct.Authority)
	require.Equal(t, authority, *acct.Authority)
	require.True(t, acct.Mutable)
	require.True(t, acct.Canonical)
	require.Equal(t, SeedIDL, acct.Seed, "the seed must come back without its padding")
	require.Equal(t, CompressionZlib, acct.Compression)
	require.Equal(t, payload, acct.Data)
}

func TestParseAccountRejectsBadInput(t *testing.T) {
	_, err := ParseAccount(make([]byte, HeaderLen-1))
	require.Error(t, err, "a short account must not be read past its end")

	buffer := make([]byte, HeaderLen)
	buffer[0] = 1 // buffer discriminator, not metadata
	_, err = ParseAccount(buffer)
	require.Error(t, err, "a buffer account must not parse as metadata")

	overrun := make([]byte, HeaderLen)
	overrun[0] = discriminatorMetadata
	binary.LittleEndian.PutUint32(overrun[offDataLength:], 4096)
	_, err = ParseAccount(overrun)
	require.Error(t, err, "a declared length beyond the account must be rejected")
}

func TestClearedAuthorityParsesAsNone(t *testing.T) {
	raw := make([]byte, HeaderLen)
	raw[0] = discriminatorMetadata
	// authority left as zeros

	acct, err := ParseAccount(raw)
	require.NoError(t, err)
	require.Nil(t, acct.Authority, "an all-zero authority means none, not the zero address")
}

// largeDocument builds a document that will not compress below the inline
// limit, so the plan is forced down the buffer path.
func largeDocument(t *testing.T, size int) []byte {
	t.Helper()

	noise := make([]byte, size)
	_, err := rand.Read(noise)
	require.NoError(t, err)

	doc, err := json.Marshal(map[string]string{"name": "rmn_remote", "blob": hex.EncodeToString(noise)})
	require.NoError(t, err)

	return doc
}

func TestPlanPublishSplitsBySigner(t *testing.T) {
	payer := solana.NewWallet().PublicKey()
	buffer := solana.NewWallet().PublicKey()
	authority := solana.NewWallet().PublicKey()

	plan, err := PlanPublish(PublishConfig{
		Program:       testProgram,
		ProgramData:   testProgramData,
		Authority:     authority,
		Payer:         payer,
		Buffer:        buffer,
		RentExemptFor: stubRent,
		Seed:          SeedIDL,
		Format:        FormatJSON,
		Document:      largeDocument(t, 8*1024),
	})
	require.NoError(t, err)

	require.Greater(t, plan.PayloadLen, MaxInlineDocument, "the document must not fit inline")
	require.Equal(t, BufferSpace(plan.PayloadLen), plan.BufferSpace)
	require.Len(t, plan.Prepare, 3, "create the buffer, stamp its header, fund the metadata account")
	require.NotEmpty(t, plan.Writes)
	require.NotNil(t, plan.CloseBuffer)

	// Only these need the upgrade authority; everything else is the deployer
	// key. That split is what keeps a large IDL out of MCMS.
	require.Equal(t, []solana.Instruction{plan.Initialize, plan.SetData}, plan.Governed())

	// Writes must tile the payload with no gap or overlap.
	covered := 0
	for _, w := range plan.Writes {
		data := mustData(t, w)
		require.Equal(t, covered, int(binary.LittleEndian.Uint32(data[1:5])))
		covered += len(data) - 5
	}
	require.Equal(t, plan.PayloadLen, covered)
}

func TestPlanPublishSendsSmallDocumentsInline(t *testing.T) {
	plan, err := PlanPublish(PublishConfig{
		Program:       testProgram,
		ProgramData:   testProgramData,
		Authority:     solana.NewWallet().PublicKey(),
		Payer:         solana.NewWallet().PublicKey(),
		RentExemptFor: stubRent,
		Seed:          SeedSecurity,
		Format:        FormatJSON,
		Document:      []byte(`{"name":"CCIP Router","policy":"https://example.invalid"}`),
	})
	require.NoError(t, err)

	// No buffer keypair was supplied and none is needed.
	require.True(t, plan.Buffer.IsZero())
	require.Empty(t, plan.Writes)
	require.Nil(t, plan.SetData)
	require.Nil(t, plan.CloseBuffer)
	require.Len(t, plan.Governed(), 1, "a small document costs one governed instruction")
	require.Len(t, plan.Prepare, 1, "only the rent for the metadata account")
}

func TestPlanPublishStepsUpOversizedDocuments(t *testing.T) {
	plan, err := PlanPublish(PublishConfig{
		Program:       testProgram,
		ProgramData:   testProgramData,
		Authority:     solana.NewWallet().PublicKey(),
		Payer:         solana.NewWallet().PublicKey(),
		Buffer:        solana.NewWallet().PublicKey(),
		RentExemptFor: stubRent,
		Seed:          SeedIDL,
		Format:        FormatJSON,
		Document:      largeDocument(t, 64*1024),
	})
	require.NoError(t, err)

	// A single reallocation cannot cover this, so the account is grown first.
	require.Greater(t, plan.PayloadLen, MaxDataIncrease)
	require.NotEmpty(t, plan.Extends, "an account beyond the growth limit must be stepped up")

	grown := HeaderLen + 1
	for _, ix := range plan.Extends {
		grown += MaxDataIncrease
		require.Equal(t, ixExtend, mustData(t, ix)[0])
	}
	require.LessOrEqual(t, HeaderLen+plan.PayloadLen-grown, MaxDataIncrease,
		"the extends must leave a gap SetData can close in one reallocation")

	// Order matters: create, grow, then install.
	governed := plan.Governed()
	require.Equal(t, plan.Initialize, governed[0])
	require.Equal(t, plan.SetData, governed[len(governed)-1])
}

func TestPlanPublishSkipsInitializeWhenAccountExists(t *testing.T) {
	plan, err := PlanPublish(PublishConfig{
		Program:       testProgram,
		Buffer:        solana.NewWallet().PublicKey(),
		RentExemptFor: stubRent,
		Seed:          SeedIDL,
		Format:        FormatJSON,
		Document:      largeDocument(t, 8*1024),
		Exists:        true,
	})
	require.NoError(t, err)
	require.Nil(t, plan.Initialize)
	require.Equal(t, []solana.Instruction{plan.SetData}, plan.Governed(),
		"republishing only needs SetData")
}

func TestPlanPublishValidates(t *testing.T) {
	base := PublishConfig{
		Program: testProgram, Buffer: solana.NewWallet().PublicKey(),
		RentExemptFor: stubRent, Seed: SeedIDL, Document: []byte("x"),
	}

	empty := base
	empty.Document = nil
	_, err := PlanPublish(empty)
	require.Error(t, err)

	noProgram := base
	noProgram.Program = solana.PublicKey{}
	_, err = PlanPublish(noProgram)
	require.Error(t, err)

	noRent := base
	noRent.RentExemptFor = nil
	_, err = PlanPublish(noRent)
	require.Error(t, err)

	// A buffer is only required once the document outgrows the inline limit.
	noBuffer := base
	noBuffer.Buffer = solana.PublicKey{}
	noBuffer.Document = largeDocument(t, 8*1024)
	_, err = PlanPublish(noBuffer)
	require.Error(t, err)
}

func TestGovernedGroupsStayWithinTransactionGrowth(t *testing.T) {
	t.Parallel()

	for name, size := range map[string]int{"inline": 300, "one extend": 12 * 1024, "several extends": 40 * 1024} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			doc := make([]byte, size)
			_, err := rand.Read(doc)
			require.NoError(t, err)

			plan, err := PlanPublish(PublishConfig{
				Program:       solana.NewWallet().PublicKey(),
				ProgramData:   solana.NewWallet().PublicKey(),
				Authority:     solana.NewWallet().PublicKey(),
				Payer:         solana.NewWallet().PublicKey(),
				Buffer:        solana.NewWallet().PublicKey(),
				RentExemptFor: func(int) (uint64, error) { return 1, nil },
				Seed:          SeedIDL,
				Format:        FormatJSON,
				Document:      doc,
			})
			require.NoError(t, err)

			groups := plan.GovernedGroups()
			var total int
			for _, g := range groups {
				total += len(g)
			}
			require.Equal(t, len(plan.Governed()), total, "no instruction may be dropped or duplicated")
			if len(plan.Extends) > 0 {
				require.Greater(t, len(groups), len(plan.Extends), "an Extend must not share a transaction with the growth around it")
			} else {
				require.Len(t, groups, 1)
			}
		})
	}
}
