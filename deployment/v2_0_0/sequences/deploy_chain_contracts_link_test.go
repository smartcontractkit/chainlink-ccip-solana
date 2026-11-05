package sequences

import (
	"strings"
	"testing"

	ccvadapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations/optest"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

// LINK is provided by the environment (devenv deploys a mock in its pre-deploy
// hook), so the sequence fails fast when the ref is missing instead of deploying
// a fresh mint. The contract params carry valid versions, so the failure comes
// from LINK resolution and not the earlier version check.
func TestDeployChainContractsRequiresLinkRef(t *testing.T) {
	_, err := cldf_ops.ExecuteSequence(
		optest.NewBundle(t), DeployChainContracts, cldf_chain.NewBlockChains(nil),
		ccvadapters.DeployChainContractsInput{
			ChainSelector:    1,
			DeployerContract: "devenv",
			ContractParams: ccvadapters.DeployContractParams{
				OffRamp:   ccvadapters.OffRampDeployParams{Version: solcommon.Version2_0_0},
				OnRamp:    ccvadapters.OnRampDeployParams{Version: solcommon.Version2_0_0},
				FeeQuoter: ccvadapters.FeeQuoterDeployParams{Version: solcommon.Version2_0_0},
			},
		})
	if err == nil || !strings.Contains(err.Error(), "failed to resolve LINK token") {
		t.Fatalf("err = %v, want LINK resolution failure", err)
	}
}
