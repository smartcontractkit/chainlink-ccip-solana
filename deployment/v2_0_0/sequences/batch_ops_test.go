package sequences

import (
	"errors"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	seq_core "github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations/optest"
	mcms_types "github.com/smartcontractkit/mcms/types"

	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
)

func TestRunOpCollectsBatchOps(t *testing.T) {
	type in struct{ Fail bool }
	op := cldf_ops.NewOperation(
		"test:batch-op",
		semver.MustParse("1.0.0"),
		"returns a batch when the deployer is not the authority",
		func(_ cldf_ops.Bundle, chain cldf_solana.Chain, input in) (seq_core.OnChainOutput, error) {
			if input.Fail {
				return seq_core.OnChainOutput{}, errors.New("boom")
			}
			return seq_core.OnChainOutput{BatchOps: []mcms_types.BatchOperation{{ChainSelector: mcms_types.ChainSelector(chain.Selector)}}}, nil
		},
	)
	chain := cldf_solana.Chain{Selector: 1}
	existing := mcms_types.BatchOperation{ChainSelector: 7}
	batches := []mcms_types.BatchOperation{existing}

	require.NoError(t, runOp(optest.NewBundle(t), op, chain, in{}, &batches))
	require.Equal(t, []mcms_types.BatchOperation{existing, {ChainSelector: 1}}, batches, "batches are appended in order")

	require.ErrorContains(t, runOp(optest.NewBundle(t), op, chain, in{Fail: true}, &batches), "boom")
	require.Len(t, batches, 2, "a failed operation adds nothing")
}

func TestLaneTestSendersAllowlist(t *testing.T) {
	wallet := solana.NewWallet().PublicKey()
	enabled := true
	staged := &routerops.DestChainAllowlist{Enabled: &enabled, AllowedSendersToAdd: []solana.PublicKey{solana.NewWallet().PublicKey()}}
	walletStaged := &routerops.DestChainAllowlist{Enabled: &enabled, AllowedSendersToAdd: []solana.PublicKey{wallet}}

	got, err := laneTestSendersAllowlist(nil, nil, true)
	require.NoError(t, err)
	require.Nil(t, got, "a lane without test senders keeps its on-chain allowlist")

	got, err = laneTestSendersAllowlist(nil, staged, true)
	require.NoError(t, err)
	require.Same(t, staged, got, "a staged run's allowlist applies to lanes without their own")

	got, err = laneTestSendersAllowlist([]string{wallet.String()}, staged, true)
	require.NoError(t, err)
	require.Equal(t, walletStaged, got, "in a staged run a lane's test senders stage it, even if it exists")

	got, err = laneTestSendersAllowlist([]string{wallet.String()}, nil, false)
	require.NoError(t, err)
	require.Equal(t, walletStaged, got, "a new lane with test senders starts closed")

	got, err = laneTestSendersAllowlist([]string{wallet.String()}, nil, true)
	require.NoError(t, err)
	require.Nil(t, got, "ordinary configuration must not close a lane that already exists")

	_, err = laneTestSendersAllowlist([]string{"not-a-key"}, nil, false)
	require.ErrorContains(t, err, "parse test sender")
}
