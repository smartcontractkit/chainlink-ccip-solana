package utils

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	soldevenv "github.com/smartcontractkit/chainlink-ccip-solana/devenv"
	"github.com/smartcontractkit/chainlink-ccip-solana/pkg/solutil"
)

// MaxComputeUnits is the maximum number of compute units to request for Solana transactions.
const MaxComputeUnits = 500_000 // this value is very high and should be reduced once onchain removes debug statements

// SolSendOptions reads the Solana private key from the environment and returns a SolanaSendOptions struct.
func SolSendOptions() (soldevenv.SolanaSendOptions, error) {
	opts := soldevenv.SolanaSendOptions{}
	raw := strings.TrimSpace(os.Getenv(solutil.SolanaPrivateKeyEnv))
	if raw == "" {
		return opts, nil
	}
	kp, err := solutil.PrivateKeyFromRaw(raw)
	if err != nil {
		return opts, fmt.Errorf("parse %s: %w", solutil.SolanaPrivateKeyEnv, err)
	}
	opts.Signer = &kp
	return opts, nil
}

// MustSolSendOptions is the test-call-site variant of SolSendOptions.
func MustSolSendOptions(t *testing.T) soldevenv.SolanaSendOptions {
	t.Helper()
	opts, err := SolSendOptions()
	require.NoError(t, err)
	return opts
}
