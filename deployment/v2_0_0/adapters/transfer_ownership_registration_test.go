package adapters

import (
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"
	deployapi "github.com/smartcontractkit/chainlink-ccip/deployment/deploy"
)

// deploy.TransferToTimelock (v2 DeployChainContracts with DeployerKeyOwned=false) resolves
// the transfer-ownership adapter at deploy.MCMSVersion, so the Solana adapter must be
// registered there - not only at the contracts' own version.
func TestTransferOwnershipAdapterRegisteredAtMCMSVersion(t *testing.T) {
	t.Parallel()
	sel := chainsel.SOLANA_DEVNET.Selector

	adapter, err := deployapi.GetTransferOwnershipRegistry().GetAdapterByChainSelector(sel, deployapi.MCMSVersion)
	require.NoError(t, err, "the framework looks the adapter up at MCMSVersion")
	require.NotNil(t, adapter)

	// the standalone transfer-ownership changesets address the adapter by contract version
	adapter, err = deployapi.GetTransferOwnershipRegistry().GetAdapterByChainSelector(sel, semver.MustParse("2.0.0"))
	require.NoError(t, err)
	require.NotNil(t, adapter)
}
