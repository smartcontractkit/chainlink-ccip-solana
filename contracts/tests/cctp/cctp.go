package cctp

import (
	"encoding/binary"
	"testing"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/tests/config"
	utilscctp "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/cctp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
)

//////////////////////////
// CCTP testing helpers //
//////////////////////////
//
// The Circle PDA derivations live in contracts/utils/cctp so the deployment tooling can share
// them; the wrappers here just adapt the (T, error) signatures to require.NoError.

type (
	MessageTransmitterPDAs     = utilscctp.MessageTransmitterPDAs
	MessageTransmitterV2PDAs   = utilscctp.MessageTransmitterPDAs
	TokenMessengerMinterPDAs   = utilscctp.TokenMessengerMinterPDAs
	TokenMessengerMinterV2PDAs = utilscctp.TokenMessengerMinterPDAs
)

func GetMessageTransmitterPDAs(t *testing.T) MessageTransmitterPDAs {
	t.Helper()
	pdas, err := utilscctp.GetMessageTransmitterPDAs()
	require.NoError(t, err)
	return pdas
}

func GetTokenMessengerMinterPDAs(t *testing.T, domain uint32, usdcMint solana.PublicKey, remoteTokenAddr []byte) TokenMessengerMinterPDAs {
	t.Helper()
	pdas, err := utilscctp.GetTokenMessengerMinterPDAs(domain, usdcMint, remoteTokenAddr)
	require.NoError(t, err)
	return pdas
}

type perChainConfigPDAs struct {
	ChainConfig,
	ChainConfigV2,
	ChainConfigOverrides solana.PublicKey
}

type TokenPoolPDAs struct {
	Program,
	GlobalConfig,
	State,
	Signer,
	TokenAccount,
	EventAuthority solana.PublicKey
	Evm,
	Svm perChainConfigPDAs
}

func GetTokenPoolPDAs(t *testing.T, usdcMint solana.PublicKey) TokenPoolPDAs {
	t.Helper()

	statePDA, err := tokens.TokenPoolConfigAddress(usdcMint, config.CctpTokenPoolProgram)
	require.NoError(t, err)
	globalConfigPDA, err := tokens.TokenPoolGlobalConfigPDA(config.CctpTokenPoolProgram)
	require.NoError(t, err)
	signerPDA, err := tokens.TokenPoolSignerAddress(usdcMint, config.CctpTokenPoolProgram)
	require.NoError(t, err)
	poolTokenAccount, _, err := tokens.FindAssociatedTokenAddress(solana.TokenProgramID, usdcMint, signerPDA)
	require.NoError(t, err)
	eventAuthority, _, err := state.FindEventAuthorityPDA(config.CctpTokenPoolProgram)
	require.NoError(t, err)

	return TokenPoolPDAs{
		Program:        config.CctpTokenPoolProgram,
		GlobalConfig:   globalConfigPDA,
		State:          statePDA,
		Signer:         signerPDA,
		TokenAccount:   poolTokenAccount,
		EventAuthority: eventAuthority,
		Evm:            getChainConfigPDAs(t, config.EvmChainSelector, usdcMint),
		Svm:            getChainConfigPDAs(t, config.SvmChainSelector, usdcMint),
	}
}

func getChainConfigPDAs(t *testing.T, chainSelector uint64, usdcMint solana.PublicKey) perChainConfigPDAs {
	t.Helper()

	chainConfigPDA, _, err := tokens.TokenPoolChainConfigPDA(chainSelector, usdcMint, config.CctpTokenPoolProgram)
	require.NoError(t, err)

	chainConfigV2PDA, _, err := tokens.TokenPoolChainConfigV2PDA(chainSelector, usdcMint, config.CctpTokenPoolProgram)
	require.NoError(t, err)

	chainConfigOverridesPDA, _, err := tokens.TokenPoolChainConfigOverridePDA(chainSelector, usdcMint, config.CctpTokenPoolProgram, []byte("ftf"))
	require.NoError(t, err)

	return perChainConfigPDAs{
		ChainConfig:          chainConfigPDA,
		ChainConfigV2:        chainConfigV2PDA,
		ChainConfigOverrides: chainConfigOverridesPDA,
	}
}

func GetNonce(messageBytes []byte) uint64 {
	nonceBytes := messageBytes[12:20] // extract nonce from message, which is the 12th to 20th byte of the message
	nonce := binary.BigEndian.Uint64(nonceBytes)
	return nonce
}

func SetNonce(messageBytes *[]byte, nonce uint64) {
	binary.BigEndian.PutUint64((*messageBytes)[12:20], nonce) // set nonce in message, which is the 12th to 20th byte of the message
}

func GetSrcDomain(cctpMessage []byte) uint32 {
	srcDomainBytes := cctpMessage[4:8] // extract source domain from message, which is the 4th to 8th byte of the message
	srcDomain := binary.BigEndian.Uint32(srcDomainBytes)
	return srcDomain
}

func SetSrcDomain(cctpMessage *[]byte, srcDomain uint32) {
	binary.BigEndian.PutUint32((*cctpMessage)[4:8], srcDomain) // set source domain in message, which is the 4th to 8th byte of the message
}

func GetUsedNoncesPDA(t *testing.T, cctpMessage []byte) solana.PublicKey {
	t.Helper()
	nonce := GetNonce(cctpMessage)
	t.Log("Nonce for receiving message:", nonce)
	firstNonce := (nonce-1)/6400*6400 + 1 // round down to the first nonce that is a multiple of 6400
	t.Log("First nonce:", firstNonce)
	t.Log("Domain:", GetSrcDomain(cctpMessage))
	firstNoncePda, _, err := solana.FindProgramAddress(
		[][]byte{
			[]byte("used_nonces"),
			[]byte(common.NumToStr(GetSrcDomain(cctpMessage))),
			[]byte(common.NumToStr(firstNonce)),
		},
		config.CctpMessageTransmitter,
	)
	require.NoError(t, err)
	return firstNoncePda
}

/////////////////////////////
// CCTP v2 testing helpers //
/////////////////////////////

func GetMessageTransmitterV2PDAs(t *testing.T) MessageTransmitterV2PDAs {
	t.Helper()
	pdas, err := utilscctp.GetMessageTransmitterV2PDAs()
	require.NoError(t, err)
	return pdas
}

func GetTokenMessengerMinterV2PDAs(t *testing.T, remoteDomain uint32, usdcMint solana.PublicKey, remoteTokenAddr []byte) TokenMessengerMinterV2PDAs {
	t.Helper()
	pdas, err := utilscctp.GetTokenMessengerMinterV2PDAs(remoteDomain, usdcMint, remoteTokenAddr)
	require.NoError(t, err)
	return pdas
}

// GetUsedNonceV2PDAFromMsg derives the MessageTransmitterV2 PDA marking a message's nonce
// as used. Unlike v1's 6400-nonce buckets, v2 stores each nonce in its own PDA,
// seeded by the 32-byte nonce at bytes [12:44] of the message header.
func GetUsedNonceV2PDAFromMsg(t *testing.T, cctpMessage []byte) solana.PublicKey {
	t.Helper()

	require.GreaterOrEqual(t, len(cctpMessage), 44, "CCTP v2 message too short to contain a nonce")

	return GetUsedNonceV2PDA(t, cctpMessage[12:44])
}

func GetUsedNonceV2PDA(t *testing.T, nonce []byte) solana.PublicKey {
	t.Helper()

	require.Len(t, nonce, 32)

	pda, _, err := solana.FindProgramAddress([][]byte{[]byte("used_nonce"), nonce}, config.CctpMessageTransmitterV2)
	require.NoError(t, err)
	return pda
}

// GetDenylistV2PDA derives the TokenMessengerMinterV2 denylist PDA for an account.
// The account is denylisted only if an account exists at this address, so for a
// clean wallet the derived (non-existent) PDA is passed as-is.
func GetDenylistV2PDA(t *testing.T, account solana.PublicKey) solana.PublicKey {
	t.Helper()

	pda, err := utilscctp.GetDenylistV2PDA(account)
	require.NoError(t, err)
	return pda
}

type Message struct {
	Version                   uint32
	SourceDomain              uint32
	DestinationDomain         uint32
	Nonce                     [32]byte
	Sender                    [32]byte
	Recipient                 [32]byte
	DestinationCaller         [32]byte
	MinFinalityThreshold      uint32
	FinalityThresholdExecuted uint32

	Body MessageBody
}

type MessageBody struct {
	Version         uint32
	BurnToken       [32]byte
	MintRecipient   [32]byte
	Amount          [32]byte
	MessageSender   [32]byte
	MaxFee          [32]byte
	FeeExecuted     [32]byte
	ExpirationBlock [32]byte
	HookData        []byte
}

type iterator struct {
	bytes []byte
	pos   int
}

func newIterator(bytes []byte) *iterator {
	return &iterator{bytes: bytes, pos: 0}
}

func (it *iterator) next(n int) []byte {
	if it.pos+n > len(it.bytes) {
		return nil // in these tests, we don't return an error - if used in production this should be made safer
	}
	b := it.bytes[it.pos : it.pos+n]
	it.pos += n
	return b
}

func (it *iterator) remaining() []byte {
	return it.bytes[it.pos:]
}

func ParseMessage(bytes []byte) (*Message, error) {
	const start = 8 + 32 + 8 + 4 // Anchor discriminator + RentPayer(Pubkey) + CreatedAt(i64) + VecPrefix

	it := newIterator(bytes)
	it.next(start) // skip the bytes before the actual message contents

	return &Message{
		Version:                   bin.BE.Uint32(it.next(4)),
		SourceDomain:              bin.BE.Uint32(it.next(4)),
		DestinationDomain:         bin.BE.Uint32(it.next(4)),
		Nonce:                     [32]byte(it.next(32)),
		Sender:                    [32]byte(it.next(32)),
		Recipient:                 [32]byte(it.next(32)),
		DestinationCaller:         [32]byte(it.next(32)),
		MinFinalityThreshold:      bin.BE.Uint32(it.next(4)),
		FinalityThresholdExecuted: bin.BE.Uint32(it.next(4)),

		Body: MessageBody{
			Version:         bin.BE.Uint32(it.next(4)),
			BurnToken:       [32]byte(it.next(32)),
			MintRecipient:   [32]byte(it.next(32)),
			Amount:          [32]byte(it.next(32)),
			MessageSender:   [32]byte(it.next(32)),
			MaxFee:          [32]byte(it.next(32)),
			FeeExecuted:     [32]byte(it.next(32)),
			ExpirationBlock: [32]byte(it.next(32)),
			HookData:        it.remaining(),
		},
	}, nil
}

func (m *Message) Serialize() []byte {
	// See https://developers.circle.com/cctp/references/technical-guide#message-body to understand the slice capacity
	body := make([]byte, 0, 228+len(m.Body.HookData))

	body = append(body, uint32Be(m.Body.Version)...)
	body = append(body, m.Body.BurnToken[:]...)
	body = append(body, m.Body.MintRecipient[:]...)
	body = append(body, m.Body.Amount[:]...)
	body = append(body, m.Body.MessageSender[:]...)
	body = append(body, m.Body.MaxFee[:]...)
	body = append(body, m.Body.FeeExecuted[:]...)
	body = append(body, m.Body.ExpirationBlock[:]...)
	body = append(body, m.Body.HookData...)

	// See https://developers.circle.com/cctp/references/technical-guide#message-header to understand the slice capacity
	message := make([]byte, 0, 148+len(body))
	message = append(message, uint32Be(m.Version)...)
	message = append(message, uint32Be(m.SourceDomain)...)
	message = append(message, uint32Be(m.DestinationDomain)...)
	message = append(message, m.Nonce[:]...)
	message = append(message, m.Sender[:]...)
	message = append(message, m.Recipient[:]...)
	message = append(message, m.DestinationCaller[:]...)
	message = append(message, uint32Be(m.MinFinalityThreshold)...)
	message = append(message, uint32Be(m.FinalityThresholdExecuted)...)
	message = append(message, body...)

	return message
}

func uint32Be(value uint32) []byte {
	b := make([]byte, 4)
	bin.BE.PutUint32(b, value)
	return b
}
