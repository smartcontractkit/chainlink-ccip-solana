// Package eth provides EVM-style signing helpers for tests: deriving EVM addresses from secp256k1
// keys, generating throwaway key sets, and keccak hashing.
//
// This lives under contracts/tests/ rather than contracts/utils/ because nothing in production uses
// it. The production path that configures committee signers (`set_signer_config`) takes hex
// addresses straight from config and converts them with go-ethereum's `ethcommon.HexToAddress` — see
// deployment/v2_0_0/adapters. It never derives an address from a private key and never generates
// keys, so these helpers only ever serve tests.
package eth

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"golang.org/x/crypto/sha3"
)

// Secp256k1N is the order of the secp256k1 curve. Exposed so signature helpers can normalize S to
// the lower half of the range (S -> N-S), which is what the on-chain `compute_signed_hash` and
// chainlink-ccv's committee hashing expect.
var Secp256k1N = secp256k1.Params().N

type Signer struct {
	PrivateKey []byte
	PubKey     []byte
	Address    [20]byte
}

func (s Signer) String() string {
	return "0x" + hex.EncodeToString(s.Address[:])
}

// GetEvmSigners derives signers from hex private keys, sorted by ascending address.
//
// The ordering is not cosmetic: the committee verifier's signer configuration requires addresses in
// ascending order, so callers feeding these into `set_signer_config` depend on it.
func GetEvmSigners(privateKeys []string) ([]Signer, error) {
	signers := make([]Signer, 0, len(privateKeys))
	for _, key := range privateKeys {
		signer, err := GetSignerFromPk(key)
		if err != nil {
			return nil, fmt.Errorf("failed to get signer from private key: %w", err)
		}
		signers = append(signers, signer)
	}

	sort.Slice(signers, func(i, j int) bool {
		return string(signers[i].Address[:]) < string(signers[j].Address[:])
	})
	return signers, nil
}

// GetSignerFromPk derives the EVM address for a hex-encoded (no "0x") secp256k1 private key, as
// keccak256(uncompressed_pubkey[1:])[12:].
func GetSignerFromPk(key string) (Signer, error) {
	pkBytes, err := hex.DecodeString(key)
	if err != nil {
		return Signer{}, fmt.Errorf("failed to decode private key: %w", err)
	}

	privateKey := secp256k1.PrivKeyFromBytes(pkBytes)
	publicKey := privateKey.PubKey()

	pubKeyBytes := publicKey.SerializeUncompressed()
	hash := Keccak256(pubKeyBytes[1:]) // skip the leading 0x04 byte
	var address [20]byte
	copy(address[:], hash[12:])

	return Signer{
		PrivateKey: pkBytes,
		PubKey:     pubKeyBytes,
		Address:    address,
	}, nil
}

// GenerateEthPrivateKeys returns n private keys as raw hexadecimal strings without a "0x" prefix.
func GenerateEthPrivateKeys(n int) ([]string, error) {
	if n <= 0 {
		return nil, errors.New("number of keys must be positive")
	}

	privateKeys := make([]string, n)
	for i := range n {
		// GeneratePrivateKey rejection-samples until the scalar lands in [1, N), which is exactly the
		// validity condition, and Serialize always returns a zero-padded 32 bytes.
		pk, err := secp256k1.GeneratePrivateKey()
		if err != nil {
			return nil, fmt.Errorf("failed to generate private key: %w", err)
		}
		privateKeys[i] = hex.EncodeToString(pk.Serialize())
	}

	return privateKeys, nil
}

func Keccak256(data []byte) []byte {
	hash := sha3.NewLegacyKeccak256()
	hash.Write(data)
	return hash.Sum(nil)
}
