package allowlist

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"
)

func TestApplyUpdates(t *testing.T) {
	t.Parallel()
	a, b, c, d := solana.NewWallet().PublicKey(), solana.NewWallet().PublicKey(), solana.NewWallet().PublicKey(), solana.NewWallet().PublicKey()

	tests := []struct {
		name                     string
		current, toAdd, toRemove []solana.PublicKey
		enabled                  bool
		want                     []solana.PublicKey
		wantErr                  string
	}{
		{name: "empty", enabled: true, want: []solana.PublicKey{}},
		{name: "keeps current order and appends added", current: []solana.PublicKey{b, a}, toAdd: []solana.PublicKey{c}, enabled: true, want: []solana.PublicKey{b, a, c}},
		{name: "drops removed", current: []solana.PublicKey{a, b, c}, toRemove: []solana.PublicKey{b}, enabled: true, want: []solana.PublicKey{a, c}},
		{name: "skips duplicates", current: []solana.PublicKey{a}, toAdd: []solana.PublicKey{a, d, d}, enabled: true, want: []solana.PublicKey{a, d}},
		{name: "keeps senders when disabled", current: []solana.PublicKey{a, b}, toRemove: []solana.PublicKey{b}, want: []solana.PublicKey{a}},
		{name: "rejects additions while disabled", toAdd: []solana.PublicKey{a}, wantErr: "while the allowlist is disabled"},
		{name: "rejects zero key", toAdd: []solana.PublicKey{{}}, enabled: true, wantErr: "zero key"},
		{name: "rejects sender to add and remove", toAdd: []solana.PublicKey{b}, toRemove: []solana.PublicKey{b}, enabled: true, wantErr: "in both the senders to add and the senders to remove"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ApplyUpdates(tt.current, tt.toAdd, tt.toRemove, tt.enabled)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestParseSenders(t *testing.T) {
	t.Parallel()
	pk := solana.NewWallet().PublicKey()

	got, err := ParseSenders([]string{pk.String()}, "added")
	require.NoError(t, err)
	require.Equal(t, []solana.PublicKey{pk}, got)

	_, err = ParseSenders([]string{"not-a-pubkey"}, "added")
	require.ErrorContains(t, err, "invalid added allowlist sender")
}

func TestUpdate(t *testing.T) {
	t.Parallel()
	a, b := solana.NewWallet().PublicKey(), solana.NewWallet().PublicKey()
	current := Current{Initialized: true, Enabled: true, Senders: []solana.PublicKey{a}}

	senders, changed, err := Update(current, true, []solana.PublicKey{a}, nil)
	require.NoError(t, err)
	require.False(t, changed, "adding a sender already on the list changes nothing")
	require.Equal(t, []solana.PublicKey{a}, senders)

	senders, changed, err = Update(current, true, []solana.PublicKey{b}, []solana.PublicKey{a})
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []solana.PublicKey{b}, senders)

	_, changed, err = Update(current, false, nil, nil)
	require.NoError(t, err)
	require.True(t, changed, "disabling is a change even with the same senders")

	_, _, err = Update(current, false, []solana.PublicKey{b}, nil)
	require.ErrorContains(t, err, "while the allowlist is disabled")
}

func TestSigner(t *testing.T) {
	t.Parallel()
	owner, admin, deployer := solana.NewWallet().PublicKey(), solana.NewWallet().PublicKey(), solana.NewWallet().PublicKey()

	signer, err := Signer(Current{Initialized: true}, owner, admin, deployer)
	require.NoError(t, err)
	require.Equal(t, admin, signer, "an initialized lane uses the allowlist admin")

	signer, err = Signer(Current{Initialized: true}, deployer, admin, deployer)
	require.NoError(t, err)
	require.Equal(t, deployer, signer, "the deployer signs when it is the owner")

	signer, err = Signer(Current{Initialized: true}, owner, deployer, deployer)
	require.NoError(t, err)
	require.Equal(t, deployer, signer, "the deployer signs when it is the allowlist admin")

	signer, err = Signer(Current{}, owner, admin, deployer)
	require.NoError(t, err)
	require.Equal(t, owner, signer, "a pending lane is set by its owner, in the same proposal")

	_, err = Signer(Current{}, deployer, admin, deployer)
	require.ErrorIs(t, err, ErrLaneNotInitialized)
}
