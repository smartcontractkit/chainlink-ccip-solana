package devenv

import "github.com/gagliardetto/solana-go"

// SolanaOptions exposes Solana-specific capabilities beyond cciptestinterfaces.Chain.
type SolanaOptions interface {
	// GetRoundRobinUser returns a concurrency-safe closure that rotates through
	// the chain's signers. Call once at setup, then invoke the closure on each
	// send to get the next key. Falls back to the deployer key when no pool is configured.
	GetRoundRobinUser() func() solana.PrivateKey
}

func (c *Chain) GetRoundRobinUser() func() solana.PrivateKey {
	if c.signerPool == nil {
		return func() solana.PrivateKey {
			return *c.chain.DeployerKey
		}
	}
	return c.signerPool.NextUser
}
