package adapters

import (
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	ccvadapters "github.com/smartcontractkit/chainlink-ccv/deployment/adapters"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"
	"github.com/smartcontractkit/chainlink-deployments-framework/pkg/logger"
)

const handoverTestSelector uint64 = 16423721717087811551

func handoverTestBundle(t *testing.T) operations.Bundle {
	t.Helper()
	return operations.NewBundle(t.Context, logger.Test(t), operations.NewMemoryReporter())
}

func TestVerifierRefsForHandover_RequiresTimelock(t *testing.T) {
	_, err := verifierRefsForHandover(handoverTestBundle(t), cldf_solana.Chain{},
		ccvadapters.CommitteeVerifierDeployInput{ChainSelector: handoverTestSelector}, nil)
	require.ErrorContains(t, err, "deploy MCMS first, or set DeployerKeyOwned")
}

func TestVerifierRefsForHandover_SkipsOtherContracts(t *testing.T) {
	timelock := datastore.AddressRef{
		ChainSelector: handoverTestSelector,
		Type:          datastore.ContractType(common_utils.RBACTimelock),
		Version:       semver.MustParse("1.6.0"),
		Qualifier:     common_utils.CLLQualifier,
		Address:       solana.NewWallet().PublicKey().String() + ".timelock",
	}
	other := datastore.AddressRef{ChainSelector: handoverTestSelector, Type: "Router", Address: solana.NewWallet().PublicKey().String()}

	refs, err := verifierRefsForHandover(handoverTestBundle(t), cldf_solana.Chain{},
		ccvadapters.CommitteeVerifierDeployInput{ChainSelector: handoverTestSelector, ExistingAddresses: []datastore.AddressRef{timelock}},
		[]datastore.AddressRef{other})
	require.NoError(t, err)
	require.Empty(t, refs, "only committee verifier programs are handed over here")
}
