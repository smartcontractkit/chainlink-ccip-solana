package adapters

import (
	"testing"

	chainsel "github.com/smartcontractkit/chain-selectors"
	ccvadapters "github.com/smartcontractkit/chainlink-ccv/deployment/adapters"
	ccvshared "github.com/smartcontractkit/chainlink-ccv/deployment/shared"
	nodev1 "github.com/smartcontractkit/chainlink-protos/job-distributor/v1/node"
)

// TestRegistersCCVAdapters verifies that Solana is registered in each of the consolidated
// per-type CCV registries (post-#1127 / #2084 migration). Resolution is by chain selector.
func TestRegistersCCVAdapters(t *testing.T) {
	sel := chainsel.SOLANA_DEVNET.Selector

	if _, err := ccvadapters.GetAggregatorRegistry().Get(sel); err != nil {
		t.Fatalf("expected solana ccv aggregator config adapter to be registered: %v", err)
	}
	if _, err := ccvadapters.GetExecutorRegistry().Get(sel); err != nil {
		t.Fatalf("expected solana ccv executor config adapter to be registered: %v", err)
	}
	if _, err := ccvadapters.GetVerifierRegistry().Get(sel); err != nil {
		t.Fatalf("expected solana ccv verifier config adapter to be registered: %v", err)
	}
	if _, err := ccvadapters.GetIndexerRegistry().Get(sel); err != nil {
		t.Fatalf("expected solana ccv indexer config adapter to be registered: %v", err)
	}
	if _, err := ccvadapters.GetTokenVerifierRegistry().Get(sel); err != nil {
		t.Fatalf("expected solana ccv token verifier config adapter to be registered: %v", err)
	}
	if _, err := ccvadapters.GetCommitteeVerifierOnchainRegistry().Get(sel); err != nil {
		t.Fatalf("expected solana ccv CommitteeVerifierOnchain adapter to be registered: %v", err)
	}

	// Topology-free deploy adapters (R4).
	if _, err := ccvadapters.GetProtocolContractsDeployRegistry().Get(sel); err != nil {
		t.Fatalf("expected solana protocol contracts deploy adapter to be registered: %v", err)
	}
	if _, err := ccvadapters.GetLaneConfigRegistry().Get(sel); err != nil {
		t.Fatalf("expected solana lane config adapter to be registered: %v", err)
	}
	if _, err := ccvadapters.GetCommitteeVerifierDeployRegistry().Get(sel); err != nil {
		t.Fatalf("expected solana committee verifier deploy adapter to be registered: %v", err)
	}
}

func TestRegistersCCVSolanaChainType(t *testing.T) {
	family, ok := ccvshared.GetChainTypeFamily(nodev1.ChainType_CHAIN_TYPE_SOLANA)
	if !ok {
		t.Fatal("expected solana JD chain type to be registered")
	}
	if family != chainsel.FamilySolana {
		t.Fatalf("expected solana JD chain type family %q, got %q", chainsel.FamilySolana, family)
	}
}
