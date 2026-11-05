package devenv

import (
	"fmt"
	"sync/atomic"

	"github.com/gagliardetto/solana-go"
)

// SignerPool holds a pre-generated set of Solana keypairs and provides
// lock-free round-robin access for concurrent transaction signing.
type SignerPool struct {
	keys  []solana.PrivateKey
	index atomic.Uint32
}

// MaxSignerPoolSize is the upper bound for signer pool size. Prevents accidental misuse.
const MaxSignerPoolSize = 20

// NewSignerPool generates n random Solana keypairs. Returns an error if n < 1 or n > MaxSignerPoolSize.
func NewSignerPool(n int) (*SignerPool, error) {
	if n < 1 || n > MaxSignerPoolSize {
		return nil, fmt.Errorf("signer pool size must be between 1 and %d, got %d", MaxSignerPoolSize, n)
	}
	keys := make([]solana.PrivateKey, n)
	for i := range keys {
		key, err := solana.NewRandomPrivateKey()
		if err != nil {
			return nil, fmt.Errorf("generate keypair %d: %w", i, err)
		}
		keys[i] = key
	}
	return &SignerPool{keys: keys}, nil
}

// NextUser returns the next keypair in round-robin order. Safe for concurrent use.
func (p *SignerPool) NextUser() solana.PrivateKey {
	idx := p.index.Add(1) - 1
	return p.keys[idx%uint32(len(p.keys))] //nolint:gosec // len bounded by MaxSignerPoolSize
}

// PrivateKeys returns a shallow copy of all private keys in the pool.
func (p *SignerPool) PrivateKeys() []solana.PrivateKey {
	out := make([]solana.PrivateKey, len(p.keys))
	copy(out, p.keys)
	return out
}

// PublicKeys returns all public keys in the pool.
func (p *SignerPool) PublicKeys() []solana.PublicKey {
	pubs := make([]solana.PublicKey, len(p.keys))
	for i, k := range p.keys {
		pubs[i] = k.PublicKey()
	}
	return pubs
}

// FindKey looks up a private key by its public key. Returns the key and true
// if found, or nil and false otherwise.
func (p *SignerPool) FindKey(pubkey solana.PublicKey) (solana.PrivateKey, bool) {
	for _, k := range p.keys {
		if k.PublicKey() == pubkey {
			return k, true
		}
	}
	return nil, false
}
