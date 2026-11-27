package solcommon

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
)

func chainWithDeployer(t *testing.T, url string, deployer solana.PrivateKey) cldf_solana.Chain {
	t.Helper()

	return cldf_solana.Chain{URL: url, DeployerKey: &deployer}
}

func TestPublishingIsMandatoryOnlyOffLocalValidators(t *testing.T) {
	local := PolicyForChain(cldf_solana.Chain{URL: "http://localhost:8899"})
	require.True(t, local.RequireIDL, "the artifact must still carry an IDL locally")
	require.False(t, local.RequirePublishedIDL,
		"a bare local validator has no metadata program, so publishing cannot be mandatory")

	network := PolicyForChain(cldf_solana.Chain{URL: "https://api.devnet.solana.com"})
	require.True(t, network.RequirePublishedIDL, "publishing is mandatory on a real network")
	require.True(t, network.RequireVerifiedBuild)
	require.True(t, network.RequireSecurityTxt)
}

// chainWithRemoteDeployer returns a chain whose deployer only exposes a signer,
// as a KMS-backed deployer does.
func chainWithRemoteDeployer(t *testing.T, url string, deployer solana.PrivateKey) cldf_solana.Chain {
	t.Helper()

	return cldf_solana.Chain{URL: url, DeployerSigner: cldf_solana.NewPrivateKeySigner(deployer)}
}

func TestIDLBufferKeyIsStableAndPerDeployerPerProgram(t *testing.T) {
	for name, newChain := range map[string]func(*testing.T, string, solana.PrivateKey) cldf_solana.Chain{
		"local deployer key": chainWithDeployer,
		"remote deployer":    chainWithRemoteDeployer,
	} {
		t.Run(name, func(t *testing.T) {
			deployer, err := solana.NewRandomPrivateKey()
			require.NoError(t, err)
			other, err := solana.NewRandomPrivateKey()
			require.NoError(t, err)

			programA := solana.NewWallet().PublicKey()
			programB := solana.NewWallet().PublicKey()

			chain := newChain(t, "https://api.devnet.solana.com", deployer)
			bufferKey := func(chain cldf_solana.Chain, program solana.PublicKey) solana.PrivateKey {
				key, err := idlBufferKey(t.Context(), chain, program)
				require.NoError(t, err)
				return key
			}

			// Stable across runs: this is what makes a leftover buffer reclaimable
			// instead of a permanent loss of rent.
			first := bufferKey(chain, programA)
			require.Equal(t, first.PublicKey(), bufferKey(chain, programA).PublicKey())

			// Distinct per program, so two publishes cannot fight over one buffer.
			require.NotEqual(t, first.PublicKey(), bufferKey(chain, programB).PublicKey())

			// Distinct per deployer, so two deployers cannot collide.
			require.NotEqual(t, first.PublicKey(),
				bufferKey(newChain(t, "https://api.devnet.solana.com", other), programA).PublicKey())

			// It must be a usable signing key, not just 32 bytes.
			require.Len(t, first, 64)
			require.True(t, first.PublicKey().IsOnCurve())
		})
	}
}
