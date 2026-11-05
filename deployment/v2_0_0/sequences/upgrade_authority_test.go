package sequences

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
)

func TestCheckUpgradeAuthority(t *testing.T) {
	deployer := solana.NewWallet().PublicKey()
	timelock := solana.NewWallet().PublicKey()
	other := solana.NewWallet().PublicKey()

	tests := []struct {
		name             string
		authority        solana.PublicKey
		timelockFound    bool
		chainHasTimelock bool
		wantErr          string
	}{
		{name: "deployer in an environment without MCMS", authority: deployer},
		{name: "deployer with the timelock resolved", authority: deployer, timelockFound: true, chainHasTimelock: true},
		{name: "timelock with the timelock resolved", authority: timelock, timelockFound: true, chainHasTimelock: true},
		{
			name:             "deployer, but the qualifier does not resolve the environment's timelock",
			authority:        deployer,
			chainHasTimelock: true,
			wantErr:          "pass mcms.qualifier so the upgrade authority can be handed to it",
		},
		{
			name:             "authority is not the deployer and no qualifier resolved the timelock",
			authority:        timelock,
			chainHasTimelock: true,
			wantErr:          "pass mcms.qualifier if the timelock owns the program",
		},
		{name: "authority is not the deployer in an environment without MCMS", authority: other, wantErr: "this environment has no MCMS"},
		{
			name:             "authority is neither the deployer nor the resolved timelock",
			authority:        other,
			timelockFound:    true,
			chainHasTimelock: true,
			wantErr:          "is not the deployer or the timelock",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkUpgradeAuthority(tt.authority, deployer, timelock, tt.timelockFound, tt.chainHasTimelock, "CLLCCIP")
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestChainHasTimelock(t *testing.T) {
	const sel = uint64(16423721717087811551)
	timelock := datastore.AddressRef{ChainSelector: sel, Type: datastore.ContractType(common_utils.RBACTimelock), Qualifier: "CLLCCIP"}
	otherChainTimelock := datastore.AddressRef{ChainSelector: 1, Type: datastore.ContractType(common_utils.RBACTimelock)}
	router := datastore.AddressRef{ChainSelector: sel, Type: "Router"}

	require.False(t, chainHasTimelock([]datastore.AddressRef{otherChainTimelock, router}, sel), "a timelock on another chain does not count")
	require.True(t, chainHasTimelock([]datastore.AddressRef{otherChainTimelock, router, timelock}, sel))
}
