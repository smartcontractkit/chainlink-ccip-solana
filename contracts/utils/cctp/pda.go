// Package cctp derives the Circle CCTP program addresses that the cctp_token_pool program
// CPIs into. The pool hardcodes Circle's program IDs rather than reading them from config
// (see contracts/programs/cctp-token-pool/src/verifier/cctp_constants.rs and
// src/pool/instructions/mod.rs), so these derivations only need the seeds - the programs
// themselves are assumed to already be deployed at their canonical addresses.
//
// The pool half (CCIP 1.5 lock_or_burn/release_or_mint) talks to CCTP v1; the embedded
// verifier half (CCIP 2.0 start_verification_at_source/verify) talks to CCTP v2. Both sets
// live here.
package cctp

import (
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	message_transmitter "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/cctp_message_transmitter"
	message_transmitter_v2 "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/cctp_message_transmitter_v2"
	token_messenger_minter "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/cctp_token_messenger_minter"
	token_messenger_minter_v2 "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/cctp_token_messenger_minter_v2"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
)

// Seeds used by Circle's programs. Domains are encoded as decimal ASCII (via common.NumToStr),
// not little-endian bytes - getting this wrong yields a valid-looking but wrong PDA.
var (
	messageTransmitterSeed          = []byte("message_transmitter")
	messageTransmitterAuthoritySeed = []byte("message_transmitter_authority")
	senderAuthoritySeed             = []byte("sender_authority")
	tokenMessengerSeed              = []byte("token_messenger")
	remoteTokenMessengerSeed        = []byte("remote_token_messenger")
	tokenMinterSeed                 = []byte("token_minter")
	localTokenSeed                  = []byte("local_token")
	tokenPairSeed                   = []byte("token_pair")
	custodySeed                     = []byte("custody")
	denylistAccountSeed             = []byte("denylist_account")
)

// MessageTransmitterPDAs are the MessageTransmitter accounts the pool/verifier must pass on
// every burn and mint. They are domain- and mint-independent.
type MessageTransmitterPDAs struct {
	Program            solana.PublicKey
	AuthorityPda       solana.PublicKey
	MessageTransmitter solana.PublicKey
	EventAuthority     solana.PublicKey
}

// TokenMessengerMinterPDAs are the TokenMessengerMinter accounts for one (remote domain, local
// mint, remote token) triple. RemoteTokenMessenger varies by domain; LocalToken, TokenPair and
// CustodyTokenAccount vary by mint and remote token, so a distinct lane or mint needs its own set.
type TokenMessengerMinterPDAs struct {
	Program              solana.PublicKey
	AuthorityPda         solana.PublicKey
	TokenMessenger       solana.PublicKey
	RemoteTokenMessenger solana.PublicKey
	TokenMinter          solana.PublicKey
	LocalToken           solana.PublicKey
	TokenPair            solana.PublicKey
	CustodyTokenAccount  solana.PublicKey
	EventAuthority       solana.PublicKey
}

// MessageTransmitterProgramID returns Circle's CCTP v1 MessageTransmitter program address.
func MessageTransmitterProgramID() solana.PublicKey { return message_transmitter.ProgramID }

// MessageTransmitterV2ProgramID returns Circle's CCTP v2 MessageTransmitter program address.
func MessageTransmitterV2ProgramID() solana.PublicKey { return message_transmitter_v2.ProgramID }

// TokenMessengerMinterProgramID returns Circle's CCTP v1 TokenMessengerMinter program address.
func TokenMessengerMinterProgramID() solana.PublicKey { return token_messenger_minter.ProgramID }

// TokenMessengerMinterV2ProgramID returns Circle's CCTP v2 TokenMessengerMinter program address.
func TokenMessengerMinterV2ProgramID() solana.PublicKey {
	return token_messenger_minter_v2.ProgramID
}

// GetMessageTransmitterPDAs derives the CCTP v1 MessageTransmitter accounts.
func GetMessageTransmitterPDAs() (MessageTransmitterPDAs, error) {
	return messageTransmitterPDAs(message_transmitter.ProgramID, token_messenger_minter.ProgramID)
}

// GetMessageTransmitterV2PDAs derives the CCTP v2 MessageTransmitter accounts.
func GetMessageTransmitterV2PDAs() (MessageTransmitterPDAs, error) {
	return messageTransmitterPDAs(message_transmitter_v2.ProgramID, token_messenger_minter_v2.ProgramID)
}

func messageTransmitterPDAs(transmitter, minter solana.PublicKey) (MessageTransmitterPDAs, error) {
	messageTransmitterPDA, _, err := solana.FindProgramAddress([][]byte{messageTransmitterSeed}, transmitter)
	if err != nil {
		return MessageTransmitterPDAs{}, fmt.Errorf("derive message transmitter PDA: %w", err)
	}
	authorityPDA, _, err := solana.FindProgramAddress([][]byte{messageTransmitterAuthoritySeed, minter.Bytes()}, transmitter)
	if err != nil {
		return MessageTransmitterPDAs{}, fmt.Errorf("derive message transmitter authority PDA: %w", err)
	}
	eventAuthority, _, err := state.FindEventAuthorityPDA(transmitter)
	if err != nil {
		return MessageTransmitterPDAs{}, fmt.Errorf("derive message transmitter event authority PDA: %w", err)
	}
	return MessageTransmitterPDAs{
		Program:            transmitter,
		AuthorityPda:       authorityPDA,
		MessageTransmitter: messageTransmitterPDA,
		EventAuthority:     eventAuthority,
	}, nil
}

// GetTokenMessengerMinterPDAs derives the CCTP v1 TokenMessengerMinter accounts for a lane.
// remoteTokenAddr may be shorter than 32 bytes (e.g. a 20-byte EVM address); it is left-padded,
// matching how Circle stores remote token addresses.
func GetTokenMessengerMinterPDAs(remoteDomain uint32, localMint solana.PublicKey, remoteTokenAddr []byte) (TokenMessengerMinterPDAs, error) {
	return tokenMessengerMinterPDAs(token_messenger_minter.ProgramID, remoteDomain, localMint, remoteTokenAddr)
}

// GetTokenMessengerMinterV2PDAs derives the CCTP v2 TokenMessengerMinter accounts for a lane.
func GetTokenMessengerMinterV2PDAs(remoteDomain uint32, localMint solana.PublicKey, remoteTokenAddr []byte) (TokenMessengerMinterPDAs, error) {
	return tokenMessengerMinterPDAs(token_messenger_minter_v2.ProgramID, remoteDomain, localMint, remoteTokenAddr)
}

func tokenMessengerMinterPDAs(minter solana.PublicKey, remoteDomain uint32, localMint solana.PublicKey, remoteTokenAddr []byte) (TokenMessengerMinterPDAs, error) {
	remoteToken32, err := LeftPadTo32Bytes(remoteTokenAddr)
	if err != nil {
		return TokenMessengerMinterPDAs{}, fmt.Errorf("pad remote token address: %w", err)
	}
	domain := []byte(common.NumToStr(remoteDomain))

	find := func(name string, seeds ...[]byte) (solana.PublicKey, error) {
		pda, _, ferr := solana.FindProgramAddress(seeds, minter)
		if ferr != nil {
			return solana.PublicKey{}, fmt.Errorf("derive %s PDA: %w", name, ferr)
		}
		return pda, nil
	}

	out := TokenMessengerMinterPDAs{Program: minter}
	for _, f := range []struct {
		dst   *solana.PublicKey
		name  string
		seeds [][]byte
	}{
		{&out.AuthorityPda, "sender authority", [][]byte{senderAuthoritySeed}},
		{&out.TokenMessenger, "token messenger", [][]byte{tokenMessengerSeed}},
		{&out.RemoteTokenMessenger, "remote token messenger", [][]byte{remoteTokenMessengerSeed, domain}},
		{&out.TokenMinter, "token minter", [][]byte{tokenMinterSeed}},
		{&out.LocalToken, "local token", [][]byte{localTokenSeed, localMint.Bytes()}},
		{&out.TokenPair, "token pair", [][]byte{tokenPairSeed, domain, remoteToken32[:]}},
		{&out.CustodyTokenAccount, "custody token account", [][]byte{custodySeed, localMint.Bytes()}},
	} {
		pda, ferr := find(f.name, f.seeds...)
		if ferr != nil {
			return TokenMessengerMinterPDAs{}, ferr
		}
		*f.dst = pda
	}

	out.EventAuthority, _, err = state.FindEventAuthorityPDA(minter)
	if err != nil {
		return TokenMessengerMinterPDAs{}, fmt.Errorf("derive token messenger minter event authority PDA: %w", err)
	}
	return out, nil
}

// GetDenylistV2PDA derives the TokenMessengerMinterV2 denylist PDA for an account. An account is
// denylisted only if an account exists at this address, so for a clean wallet the derived
// (non-existent) PDA is passed through as-is.
func GetDenylistV2PDA(account solana.PublicKey) (solana.PublicKey, error) {
	pda, _, err := solana.FindProgramAddress([][]byte{denylistAccountSeed, account.Bytes()}, token_messenger_minter_v2.ProgramID)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("derive denylist PDA for %s: %w", account, err)
	}
	return pda, nil
}

// LeftPadTo32Bytes left-pads an address to 32 bytes, the width Circle and CCIP both use on the
// wire. Inputs already 32 bytes wide are returned unchanged.
func LeftPadTo32Bytes(addr []byte) ([32]byte, error) {
	var out [32]byte
	if len(addr) == 0 {
		return out, errors.New("address is empty")
	}
	if len(addr) > 32 {
		return out, fmt.Errorf("address is %d bytes, want at most 32", len(addr))
	}
	copy(out[32-len(addr):], addr)
	return out, nil
}
