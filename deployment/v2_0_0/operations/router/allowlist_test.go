package router

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
)

func TestResolveAllowlist(t *testing.T) {
	wallet := solana.NewWallet().PublicKey()
	other := solana.NewWallet().PublicKey()
	onChain := &ccip_router.DestChainConfigCcipV2{AllowListEnabled: true, AllowedSenders: []solana.PublicKey{wallet}}
	enabled, disabled := true, false

	// Re-running lane config without allowlist changes keeps a staged lane's allowlist.
	gotEnabled, gotSenders, err := resolveAllowlist(nil, onChain)
	require.NoError(t, err)
	require.True(t, gotEnabled)
	require.Equal(t, []solana.PublicKey{wallet}, gotSenders)

	// A new lane without an allowlist starts open.
	gotEnabled, gotSenders, err = resolveAllowlist(nil, nil)
	require.NoError(t, err)
	require.False(t, gotEnabled)
	require.Empty(t, gotSenders)

	// A nil Enabled keeps the lane's setting while the senders change.
	gotEnabled, gotSenders, err = resolveAllowlist(&DestChainAllowlist{
		AllowedSendersToAdd:    []solana.PublicKey{other},
		AllowedSendersToRemove: []solana.PublicKey{wallet},
	}, onChain)
	require.NoError(t, err)
	require.True(t, gotEnabled)
	require.Equal(t, []solana.PublicKey{other}, gotSenders)

	// Disabling keeps the stored senders, so a rollback can enable them again.
	gotEnabled, gotSenders, err = resolveAllowlist(&DestChainAllowlist{Enabled: &disabled}, onChain)
	require.NoError(t, err)
	require.False(t, gotEnabled)
	require.Equal(t, []solana.PublicKey{wallet}, gotSenders)

	// Adding senders while the allowlist is disabled is rejected.
	_, _, err = resolveAllowlist(&DestChainAllowlist{Enabled: &disabled, AllowedSendersToAdd: []solana.PublicKey{other}}, onChain)
	require.ErrorContains(t, err, "while the allowlist is disabled")

	// Enabling a lane with no stored senders would reject everyone, the test wallet included.
	_, _, err = resolveAllowlist(&DestChainAllowlist{Enabled: &enabled}, &ccip_router.DestChainConfigCcipV2{})
	require.ErrorContains(t, err, "at least one allowed sender")
}

func TestTargetAllowlist(t *testing.T) {
	wallet := solana.NewWallet().PublicKey()
	other := solana.NewWallet().PublicKey()
	staged := ccip_router.DestChainConfigCcipV2{AllowListEnabled: true, AllowedSenders: []solana.PublicKey{wallet}}
	promoted := ccip_router.DestChainConfigCcipV2{AllowListEnabled: false, AllowedSenders: []solana.PublicKey{wallet}}
	enabled, disabled := true, false

	// Promoting keeps the list, so a rollback can enable it again without restating it.
	gotEnabled, gotSenders, changed, err := targetAllowlist(SetDestChainAllowlistParams{Enabled: &disabled}, staged)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gotEnabled)
	require.Equal(t, []solana.PublicKey{wallet}, gotSenders)

	_, _, changed, err = targetAllowlist(SetDestChainAllowlistParams{Enabled: &disabled}, promoted)
	require.NoError(t, err)
	require.False(t, changed, "a lane already in the requested state is skipped")

	// A rollback enables the kept list again.
	gotEnabled, gotSenders, changed, err = targetAllowlist(SetDestChainAllowlistParams{Enabled: &enabled}, promoted)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, gotEnabled)
	require.Equal(t, []solana.PublicKey{wallet}, gotSenders)

	// Senders to add and to remove apply to the current list.
	gotEnabled, gotSenders, changed, err = targetAllowlist(SetDestChainAllowlistParams{
		AllowedSendersToAdd:    []solana.PublicKey{other},
		AllowedSendersToRemove: []solana.PublicKey{wallet},
	}, staged)
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, gotEnabled)
	require.Equal(t, []solana.PublicKey{other}, gotSenders)

	// Rolling back a lane with no stored senders would reject everyone, the test wallet included.
	_, _, _, err = targetAllowlist(SetDestChainAllowlistParams{Enabled: &enabled}, ccip_router.DestChainConfigCcipV2{})
	require.ErrorContains(t, err, "at least one allowed sender")

	// Adding senders while the allowlist is disabled is rejected.
	_, _, _, err = targetAllowlist(SetDestChainAllowlistParams{
		Enabled:             &disabled,
		AllowedSendersToAdd: []solana.PublicKey{other},
	}, staged)
	require.ErrorContains(t, err, "while the allowlist is disabled")
}
