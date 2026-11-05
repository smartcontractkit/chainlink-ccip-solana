// Package solutil provides shared utilities for the Solana CCV chain
// implementation. These helpers sit at the boundary between CCV's
// chain-agnostic hex-encoded addresses and Solana's native public keys.
package solutil

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/gagliardetto/solana-go"
	mrtronbase58 "github.com/mr-tron/base58"
)

// SolanaPrivateKeyEnv is the standard env var name for a funded Solana private key
// (base58 or JSON array format) in stead of using the default solana signer key.
// Used by the CLDF provider factory and test helpers to select a deployer/signer
// key for deployed environments (e.g. staging).
const SolanaPrivateKeyEnv = "SOLANA_PRIVATE_KEY"

// PrivateKeyFromEnvOrDefault reads SolanaPrivateKeyEnv and parses it, falling back
// to defaultKey if the env var is unset or empty. Pass "" for defaultKey to get an
// error when the env var is not set.
func PrivateKeyFromEnvOrDefault(defaultKey string) (solana.PrivateKey, error) {
	raw := strings.TrimSpace(os.Getenv(SolanaPrivateKeyEnv))
	if raw == "" {
		raw = defaultKey
	}
	if raw == "" {
		return nil, fmt.Errorf("%s not set and no default provided", SolanaPrivateKeyEnv)
	}
	return PrivateKeyFromRaw(raw)
}

// PrivateKeyFromRaw parses a Solana private key or keypair from common encodings:
//   - Base58-encoded 32-byte seed or 64-byte keypair
//   - JSON byte array (Anchor / solana-keygen format), 32 or 64 bytes
func PrivateKeyFromRaw(raw string) (solana.PrivateKey, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("empty private key")
	}
	if strings.HasPrefix(raw, "[") {
		var keyBytes []byte
		if err := json.Unmarshal([]byte(raw), &keyBytes); err != nil {
			return nil, fmt.Errorf("decode json keypair: %w", err)
		}
		return privateKeyFromBytes(keyBytes)
	}

	decoded, err := mrtronbase58.Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("decode base58 key: %w", err)
	}
	return privateKeyFromBytes(decoded)
}

func privateKeyFromBytes(b []byte) (solana.PrivateKey, error) {
	switch len(b) {
	case ed25519.SeedSize:
		return solana.PrivateKey(ed25519.NewKeyFromSeed(b)), nil
	case ed25519.PrivateKeySize:
		pk := solana.PrivateKey(b)
		if _, err := solana.ValidatePrivateKey(pk); err != nil {
			return nil, fmt.Errorf("invalid keypair: %w", err)
		}
		return pk, nil
	default:
		return nil, fmt.Errorf("invalid key length: got %d bytes, want %d (seed) or %d (keypair)",
			len(b), ed25519.SeedSize, ed25519.PrivateKeySize)
	}
}

// PublicKeyFromHex converts a hex-encoded 32-byte string to a Solana public key.
// Accepts optional "0x" prefix (CCV convention for chain-agnostic address encoding).
func PublicKeyFromHex(hexStr string) (solana.PublicKey, error) {
	hexStr = strings.TrimPrefix(hexStr, "0x")

	b, err := hex.DecodeString(hexStr)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("invalid hex string: %w", err)
	}

	if len(b) != 32 {
		return solana.PublicKey{}, fmt.Errorf("invalid public key length: got %d bytes, want 32", len(b))
	}

	return solana.PublicKeyFromBytes(b), nil
}

// MustPublicKeyFromHex is like PublicKeyFromHex but panics on error.
// Use only for compile-time constants and test fixtures.
func MustPublicKeyFromHex(hexStr string) solana.PublicKey {
	pk, err := PublicKeyFromHex(hexStr)
	if err != nil {
		panic(fmt.Sprintf("MustPublicKeyFromHex: %s", err))
	}
	return pk
}

// PublicKeyToHex converts a Solana public key to a hex-encoded string
// (no "0x" prefix).
func PublicKeyToHex(pk solana.PublicKey) string {
	return hex.EncodeToString(pk.Bytes())
}

// SetProgramID updates a gobinding package's ProgramID var and re-registers the
// instruction decoder under the new address.
func SetProgramID[T any](programIDVar *solana.PublicKey, parser func([]byte) (T, error), newID solana.PublicKey) {
	*programIDVar = newID
	_ = solana.RegisterInstructionDecoder(newID, func(_ []*solana.AccountMeta, data []byte) (any, error) {
		return parser(data)
	})
}
