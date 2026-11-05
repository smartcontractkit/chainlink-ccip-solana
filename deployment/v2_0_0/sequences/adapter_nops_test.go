package sequences

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"
	ccvadapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"
	"github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/offchain"
)

func TestValidateNOPsTopologyPerEnvironment(t *testing.T) {
	mainnet := strconv.FormatUint(chainsel.SOLANA_MAINNET.Selector, 10)
	devnet := strconv.FormatUint(chainsel.SOLANA_DEVNET.Selector, 10)

	tests := []struct {
		name       string
		env        string
		selector   string
		verifiers  int
		executors  int
		wantErrSub string
	}{
		{"prod_mainnet 16 verifiers 16 executors", "prod_mainnet", mainnet, 16, 16, ""},
		{"prod_mainnet 15 verifiers", "prod_mainnet", mainnet, 15, 16, `committee "default"`},
		{"prod_mainnet 15 executors", "prod_mainnet", mainnet, 16, 15, `executor pool "default"`},
		{"prod_testnet 16 verifiers 4 executors", "prod_testnet", devnet, 16, 4, ""},
		{"prod_testnet 3 executors", "prod_testnet", devnet, 16, 3, `executor pool "default"`},
		{"prod_testnet 3 verifiers", "prod_testnet", devnet, 3, 4, `committee "default"`},
		{"staging_testnet is not checked", "staging_testnet", devnet, 2, 2, ""},
		{"staging_testnet_solana is not checked", "staging_testnet_solana", devnet, 5, 3, ""},
	}

	registry := ccvadapters.NewChainFamilyRegistry()
	registry.RegisterChainFamily(chainsel.FamilySolana, &SolanaAdapter{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := solanaTopology(tt.selector, tt.verifiers, tt.executors).ValidateForEnvironment(tt.env, registry)
			if tt.wantErrSub == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErrSub)
		})
	}
}

func TestValidateNOPsTopologyRejectsUnknownSelector(t *testing.T) {
	require.Error(t, (&SolanaAdapter{}).ValidateNOPsTopology("not-a-selector", 16))
}

func solanaTopology(selector string, verifiers, executors int) *offchain.EnvironmentTopology {
	aliases := func(prefix string, n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("%s-%d", prefix, i)
		}
		return out
	}
	verifierAliases := aliases("verifier", verifiers)
	executorAliases := aliases("executor", executors)
	nops := make([]offchain.NOPConfig, 0, len(verifierAliases)+len(executorAliases))
	for _, alias := range append(append([]string{}, verifierAliases...), executorAliases...) {
		nops = append(nops, offchain.NOPConfig{Alias: alias, Name: alias})
	}
	return &offchain.EnvironmentTopology{
		IndexerAddress: []string{"http://indexer:8080"},
		NOPTopology: &offchain.NOPTopology{
			NOPs: nops,
			Committees: map[string]offchain.CommitteeConfig{
				"default": {
					Qualifier:    "default",
					Aggregators:  []offchain.AggregatorConfig{{Name: "aggregator-1", Address: "http://aggregator:8080"}},
					ChainConfigs: map[string]offchain.ChainCommitteeConfig{selector: {NOPAliases: verifierAliases, Threshold: 1}},
				},
			},
		},
		ExecutorPools: map[string]offchain.ExecutorPoolConfig{
			"default": {ChainConfigs: map[string]offchain.ChainExecutorPoolConfig{selector: {NOPAliases: executorAliases}}},
		},
	}
}
