package solcommon

import (
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/state"
	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	cldf_datastore "github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_deployment "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
)

const governanceTestSelector = uint64(12463857294658392847)

var timelockTestProgram = solana.MustPublicKeyFromBase58("Ccip842gzYHhvdDkSyi2YVCoAWPbYJoApMFzSxQroE9C")

func timelockRef(version string, address string) cldf_datastore.AddressRef {
	return cldf_datastore.AddressRef{
		ChainSelector: governanceTestSelector,
		Type:          cldf_datastore.ContractType(common_utils.RBACTimelock),
		Version:       semver.MustParse(version),
		Qualifier:     common_utils.CLLQualifier,
		Address:       address,
	}
}

// The timelock ref is read at 1.6.0 only: there is no v2 MCMS deployment, v2 reuses the
// shared 1.6 MCMS. A datastore carrying the ref at any other version reports not found.
func TestFindTimelockSignerPDAReadsThe16Ref(t *testing.T) {
	var seed state.PDASeed
	copy(seed[:], "timelock")
	want := state.GetTimelockSignerPDA(timelockTestProgram, seed)
	address := timelockTestProgram.String() + ".timelock"

	signer, found, err := FindTimelockSignerPDA(
		[]cldf_datastore.AddressRef{timelockRef("1.6.0", address)},
		governanceTestSelector,
		common_utils.CLLQualifier,
	)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, want, signer)

	_, found, err = FindTimelockSignerPDA(
		[]cldf_datastore.AddressRef{timelockRef("2.0.0", address)},
		governanceTestSelector,
		common_utils.CLLQualifier,
	)
	require.NoError(t, err)
	require.False(t, found, "a 2.0.0 MCMS deployment does not exist, so its ref must not resolve")
}

// An environment deployed without MCMS (the devenv) has no timelock ref at all. Callers that can
// work without one must be able to tell that apart from a broken ref, so the lookup reports
// found=false instead of failing.
func TestFindTimelockSignerPDAReportsMissingRefWithoutError(t *testing.T) {
	signer, found, err := FindTimelockSignerPDA(nil, governanceTestSelector, common_utils.CLLQualifier)
	require.NoError(t, err)
	require.False(t, found)
	require.True(t, signer.IsZero())

	_, err = GetTimelockSignerPDA(nil, governanceTestSelector, common_utils.CLLQualifier)
	require.ErrorContains(t, err, "no RBACTimelock address")
}

// A ref that exists but does not parse is a broken environment, not an absent one: both lookups
// fail rather than deriving a PDA from a malformed address.
func TestTimelockSignerPDARejectsMalformedRef(t *testing.T) {
	refs := []cldf_datastore.AddressRef{timelockRef("1.6.0", "not-a-timelock-address")}

	_, _, err := FindTimelockSignerPDA(refs, governanceTestSelector, common_utils.CLLQualifier)
	require.ErrorContains(t, err, "parse timelock address")

	_, err = GetTimelockSignerPDA(refs, governanceTestSelector, common_utils.CLLQualifier)
	require.ErrorContains(t, err, "parse timelock address")
}

func mcmsInfraRef(contractType cldf_deployment.ContractType, version, qualifier, address string) cldf_datastore.AddressRef {
	return cldf_datastore.AddressRef{
		ChainSelector: governanceTestSelector,
		Type:          cldf_datastore.ContractType(contractType),
		Version:       semver.MustParse(version),
		Qualifier:     qualifier,
		Address:       address,
	}
}

func TestMCMSQualifierOrDefault(t *testing.T) {
	t.Parallel()
	require.Equal(t, common_utils.CLLQualifier, MCMSQualifierOrDefault(""))
	require.Equal(t, "OTHER", MCMSQualifierOrDefault("OTHER"))
}

func TestFindMCMSRefReadsThe16Ref(t *testing.T) {
	t.Parallel()
	addr := solana.NewWallet().PublicKey().String()

	got, err := FindMCMSRef(
		[]cldf_datastore.AddressRef{mcmsInfraRef(common_utils.RBACTimelock, "1.6.0", common_utils.CLLQualifier, addr)},
		governanceTestSelector, common_utils.RBACTimelock, common_utils.CLLQualifier)
	require.NoError(t, err)
	require.Equal(t, addr, got.Address)

	_, err = FindMCMSRef(
		[]cldf_datastore.AddressRef{mcmsInfraRef(common_utils.RBACTimelock, "2.0.0", common_utils.CLLQualifier, addr)},
		governanceTestSelector, common_utils.RBACTimelock, common_utils.CLLQualifier)
	require.ErrorContains(t, err, "no RBACTimelock address", "a 2.0.0 MCMS deployment does not exist, so its ref must not resolve")
}

// A missing ref is an error naming it, so proposal building fails loudly instead of
// panicking on a zero address downstream.
func TestFindMCMSRefNamesMissingRef(t *testing.T) {
	t.Parallel()
	_, err := FindMCMSRef(nil, governanceTestSelector, common_utils.RBACTimelock, common_utils.CLLQualifier)
	require.ErrorContains(t, err, "no RBACTimelock address for chain")
	require.ErrorContains(t, err, `qualifier "CLLCCIP"`)

	// a ref stored under another MCMS instance's qualifier must not match
	refs := []cldf_datastore.AddressRef{
		mcmsInfraRef(common_utils.RBACTimelock, "1.6.0", "OTHER", solana.NewWallet().PublicKey().String()),
	}
	_, err = FindMCMSRef(refs, governanceTestSelector, common_utils.RBACTimelock, common_utils.CLLQualifier)
	require.ErrorContains(t, err, "no RBACTimelock address for chain")
}

func TestParseOwnerAddress(t *testing.T) {
	t.Parallel()

	// plain base58 keys pass through
	key := solana.NewWallet().PublicKey()
	got, err := ParseOwnerAddress(key.String())
	require.NoError(t, err)
	require.Equal(t, key, got)

	// the timelock's datastore address ("program.seed"), as handed over by
	// deploy.TransferToTimelock, resolves to its signer PDA
	var seed state.PDASeed
	copy(seed[:], "timelock")
	want := state.GetTimelockSignerPDA(timelockTestProgram, seed)
	got, err = ParseOwnerAddress(timelockTestProgram.String() + ".timelock")
	require.NoError(t, err)
	require.Equal(t, want, got)

	_, err = ParseOwnerAddress("not-an-owner")
	require.ErrorContains(t, err, "neither a base58 public key nor an MCMS contract address")
}
