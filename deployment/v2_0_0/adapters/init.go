package adapters

import (
	"github.com/Masterminds/semver/v3"

	chainsel "github.com/smartcontractkit/chain-selectors"
	deployapi "github.com/smartcontractkit/chainlink-ccip/deployment/deploy"
	"github.com/smartcontractkit/chainlink-ccip/deployment/fastcurse"
	"github.com/smartcontractkit/chainlink-ccip/deployment/fees"
	tokensapi "github.com/smartcontractkit/chainlink-ccip/deployment/tokens"
	cciputils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	mcmsreaderapi "github.com/smartcontractkit/chainlink-ccip/deployment/utils/changesets"
	ccipdeploymentv2adapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"
	ccipoffchainshared "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/offchain/shared"
	ccvdeploymentadapters "github.com/smartcontractkit/chainlink-ccv/deployment/adapters"
	ccvdeploymentshared "github.com/smartcontractkit/chainlink-ccv/deployment/shared"
	nodev1 "github.com/smartcontractkit/chainlink-protos/job-distributor/v1/node"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	solseq "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

func init() {
	v := semver.MustParse("2.0.0")
	ccvdeploymentshared.RegisterChainTypeFamily(nodev1.ChainType_CHAIN_TYPE_SOLANA, chainsel.FamilySolana)
	// Solana is address-class (same 20-byte secp256k1→keccak256 as EVM), so it uses
	// the default EVMSigningIdentityReader. Registering it ensures fetch_signing_keys
	// indexes the address under the solana family key.
	ccvdeploymentshared.RegisterSigningIdentityReader(chainsel.FamilySolana, ccvdeploymentshared.EVMSigningIdentityReader{})
	ccipoffchainshared.RegisterSigningIdentityReader(chainsel.FamilySolana, ccipoffchainshared.EVMSigningIdentityReader{})

	// --- Tier 1: 2.0-specific registrations ---

	// Deploy chain contracts
	ccipdeploymentv2adapters.GetDeployChainContractsRegistry().Register(
		chainsel.FamilySolana, &SolanaDeployChainContractsAdapter{},
	)

	// Chain family (replaces LaneAdapter for 2.0)
	ccipdeploymentv2adapters.GetChainFamilyRegistry().RegisterChainFamily(
		chainsel.FamilySolana, &solseq.SolanaAdapter{},
	)

	// Committee verifier contract resolution for PopulateCommitteeConfig (still lives in chainlink-ccip).
	ccipdeploymentv2adapters.GetCommitteeVerifierContractRegistry().Register(
		chainsel.FamilySolana, &SolanaCommitteeVerifierContractAdapter{},
	)

	// CCV consolidated registry (per-type singletons, post-https://github.com/smartcontractkit/chainlink-ccv/pull/1127).
	// Each family registers once per adapter type; the legacy GetRegistry().Register(ChainAdapters{}) shim is intentionally avoided.
	ccvdeploymentadapters.GetAggregatorRegistry().Register(chainsel.FamilySolana, &SolanaCCVAggregatorConfigAdapter{})
	ccvdeploymentadapters.GetExecutorRegistry().Register(chainsel.FamilySolana, &SolanaExecutorConfigAdapter{})
	ccvdeploymentadapters.GetVerifierRegistry().Register(chainsel.FamilySolana, &SolanaVerifierConfigAdapter{})
	ccvdeploymentadapters.GetIndexerRegistry().Register(chainsel.FamilySolana, &SolanaCCVIndexerConfigAdapter{})
	ccvdeploymentadapters.GetTokenVerifierRegistry().Register(chainsel.FamilySolana, &SolanaTokenVerifierConfigAdapter{})
	ccvdeploymentadapters.GetCommitteeVerifierOnchainRegistry().Register(chainsel.FamilySolana, &SolanaCommitteeVerifierOnchainAdapter{})

	// Topology-free deploy adapters (post-#1127). Protocol deploy excludes the committee
	// verifier; the verifier owns its own phase-3 deploy (2026-06-17 split).
	ccvdeploymentadapters.GetProtocolContractsDeployRegistry().Register(chainsel.FamilySolana, &SolanaProtocolContractsDeployAdapter{})
	ccvdeploymentadapters.GetLaneConfigRegistry().Register(chainsel.FamilySolana, &SolanaLaneConfigAdapter{})
	ccvdeploymentadapters.GetCommitteeVerifierDeployRegistry().Register(chainsel.FamilySolana, &SolanaCommitteeVerifierDeployAdapter{})

	// Curse adapter
	fastcurse.GetCurseRegistry().RegisterNewCurse(fastcurse.CurseRegistryInput{
		CursingFamily:       chainsel.FamilySolana,
		CursingVersion:      v,
		CurseAdapter:        NewCurseAdapter(),
		CurseSubjectAdapter: NewCurseAdapter(),
	})

	// Fee adapter
	fees.GetRegistry().RegisterFeeAdapter(chainsel.FamilySolana, v, NewFeesAdapter(&solseq.SolanaAdapter{}))

	// Fee aggregator adapter
	fees.GetFeeAggregatorRegistry().RegisterFeeAggregatorAdapter(chainsel.FamilySolana, v, NewFeeAggregatorAdapter(&solseq.SolanaAdapter{}))

	// --- Tier 2: Shared infrastructure registrations ---

	// Chain family selector
	cciputils.RegisterChainFamilySelector(chainsel.FamilySolana, solcommon.SVMChainFamilySelector)

	// MCMS Reader
	mcmsreaderapi.GetRegistry().RegisterMCMSReader(chainsel.FamilySolana, &solseq.SolanaAdapter{})

	// Transfer ownership. deploy.TransferToTimelock (used by the v2 DeployChainContracts
	// changeset when DeployerKeyOwned is false) resolves the adapter at MCMSVersion;
	// the 2.0.0 registration serves the standalone transfer-ownership changesets.
	deployapi.GetTransferOwnershipRegistry().RegisterAdapter(chainsel.FamilySolana, deployapi.MCMSVersion, &solseq.SolanaAdapter{})
	deployapi.GetTransferOwnershipRegistry().RegisterAdapter(chainsel.FamilySolana, v, &solseq.SolanaAdapter{})

	// Token adapter
	tokensapi.GetTokenAdapterRegistry().RegisterTokenAdapter(chainsel.FamilySolana, v, &solseq.SolanaAdapter{})
	tokensapi.GetTokenAdapterRegistry().RegisterTokenAdapter(chainsel.FamilySolana, semver.MustParse("1.6.1"), &solseq.SolanaAdapter{})

	// --- Tier 3: Family-keyed helpers (versionless) ---

	fees.GetRegistry().RegisterFeeResolver(chainsel.FamilySolana, &SolanaFeeResolver{})
	deployapi.GetAddressNormalizerRegistry().RegisterAddressNormalizer(chainsel.FamilySolana, &SolanaAddressNormalizer{})
	tokensapi.GetTokenAdapterRegistry().RegisterTokenAdminRegistryManager(chainsel.FamilySolana, &SolanaAdminRegistryManager{})
	tokensapi.GetTokenAdapterRegistry().RegisterTokenRefResolver(chainsel.FamilySolana, &solseq.SolanaAdapter{})
}
