// Package allowlist holds the sender allowlist logic shared by the Router and the verifier
// operations.
package allowlist

import (
	"errors"
	"fmt"
	"slices"

	"github.com/gagliardetto/solana-go"
)

// ApplyUpdates returns the allowlist that results from removing `toRemove` from `current` and then
// adding `toAdd`. It keeps the order of `current` and appends new senders, so repeated runs
// with the same input produce the same list. Like EVM `_applyAllowlistUpdates`, it rejects the
// zero key and additions to a disabled allowlist. It also rejects a sender listed in both toAdd
// and toRemove, because the intent is ambiguous.
func ApplyUpdates(current, toAdd, toRemove []solana.PublicKey, enabled bool) ([]solana.PublicKey, error) {
	if len(toAdd) > 0 && !enabled {
		return nil, fmt.Errorf("cannot add %d allowlisted senders while the allowlist is disabled", len(toAdd))
	}
	removeSet := make(map[solana.PublicKey]struct{}, len(toRemove))
	for _, k := range toRemove {
		removeSet[k] = struct{}{}
	}
	for _, k := range toAdd {
		if k.IsZero() {
			return nil, errors.New("cannot add the zero key as an allowlisted sender")
		}
		if _, ok := removeSet[k]; ok {
			return nil, fmt.Errorf("sender %s is in both the senders to add and the senders to remove", k)
		}
	}
	out := make([]solana.PublicKey, 0, len(current)+len(toAdd))
	seen := make(map[solana.PublicKey]struct{}, len(current)+len(toAdd))
	for _, k := range append(append([]solana.PublicKey{}, current...), toAdd...) {
		if _, drop := removeSet[k]; drop {
			continue
		}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out, nil
}

// ParseSenders parses base58 Solana sender addresses. label names the list in errors.
func ParseSenders(senders []string, label string) ([]solana.PublicKey, error) {
	out := make([]solana.PublicKey, 0, len(senders))
	for _, s := range senders {
		pk, err := solana.PublicKeyFromBase58(s)
		if err != nil {
			return nil, fmt.Errorf("invalid %s allowlist sender %q: %w", label, s, err)
		}
		out = append(out, pk)
	}
	return out, nil
}

// Current is a lane's allowlist as stored on chain. Initialized is false when the lane's remote
// chain config does not exist yet.
type Current struct {
	Initialized bool
	Enabled     bool
	Senders     []solana.PublicKey
}

// Update applies the senders to add and to remove to the current allowlist. changed is false
// when the result matches the current allowlist, so nothing needs to be sent.
func Update(current Current, enabled bool, toAdd, toRemove []solana.PublicKey) (senders []solana.PublicKey, changed bool, err error) {
	senders, err = ApplyUpdates(current.Senders, toAdd, toRemove, enabled)
	if err != nil {
		return nil, false, err
	}
	return senders, current.Enabled != enabled || !slices.Equal(current.Senders, senders), nil
}

// ErrLaneNotInitialized is returned by Signer when the deployer would send set_allowlist for a
// lane that does not exist.
var ErrLaneNotInitialized = errors.New("lane is not initialized; set its chain config first")

// Signer returns who signs a verifier's set_allowlist, which accepts the owner or the allowlist
// admin. An initialized lane uses the deployer when it holds either role, and the allowlist admin
// otherwise. A lane that is not initialized yet has its chain config still pending under the
// owner (in an MCMS proposal when the owner is MCMS), so the owner signs to keep both in the same
// proposal, in order. When that owner is the deployer, nothing is pending and the lane must be
// set up first.
func Signer(current Current, owner, allowlistAdmin, deployer solana.PublicKey) (solana.PublicKey, error) {
	if current.Initialized {
		if deployer == owner || deployer == allowlistAdmin {
			return deployer, nil
		}
		return allowlistAdmin, nil
	}
	if owner == deployer {
		return solana.PublicKey{}, ErrLaneNotInitialized
	}
	return owner, nil
}
