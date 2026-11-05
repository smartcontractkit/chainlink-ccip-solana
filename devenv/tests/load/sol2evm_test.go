package loadtest

import (
	"testing"

	"github.com/stretchr/testify/require"

	soldevenv "github.com/smartcontractkit/chainlink-ccip-solana/devenv"
)

func runSolanaLoadTest(t *testing.T, cfg loadTestConfig, poolSize int) {
	t.Helper()

	ctx, env := setupCrossFamilyLoadEnv(t)

	evmChain := env.impls[env.evmSelector]
	evmReceiver, err := evmChain.GetEOAReceiverAddress()
	require.NoError(t, err)

	solChain, ok := env.impls[env.solSelector].(*soldevenv.Chain)
	require.True(t, ok, "Solana chain should be *soldevenv.Chain")
	require.NoError(t, solChain.InitSignerPool(ctx, poolSize))
	t.Cleanup(func() {
		if drainErr := solChain.DrainSignerPool(ctx); drainErr != nil {
			t.Logf("DrainSignerPool failed: %v", drainErr)
		}
	})

	gun, err := NewSolanaTXGun(env.logger, env.impls, []Lane{{
		Src: env.solSelector, Dst: env.evmSelector, Receiver: evmReceiver,
	}})
	require.NoError(t, err)

	runLoadProfile(ctx, t, env.impls, gun, cfg, withPostVerifyDelay(postTestVerificationDelay))
}

// TestSolana2EVMLoad fires sustained Sol→EVM CCIP messages and verifies execution on EVM.
// Expects the devenv to already be running (make start-devenv; env-out.toml).
//

func TestSolana2EVMLoad(t *testing.T) {
	runSolanaLoadTest(t, loadTestConfigFromEnv(), loadTestPoolSize())
}
