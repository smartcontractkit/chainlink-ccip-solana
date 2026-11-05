package sequences

import (
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	tokensops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokens"
)

func TestFindLinkMint(t *testing.T) {
	const sel, other = uint64(16423721717087811551), uint64(1)
	linkRef := func(chain uint64, mint solana.PublicKey, version *semver.Version) datastore.AddressRef {
		return datastore.AddressRef{
			ChainSelector: chain,
			Address:       mint.String(),
			Type:          datastore.ContractType(tokensops.LinkContractType),
			Version:       version,
		}
	}
	store := func(refs ...datastore.AddressRef) []datastore.AddressRef { return refs }

	got, err := findLinkMint(store(), sel)
	require.NoError(t, err)
	require.True(t, got.IsZero(), "no LINK on the chain means no LINK vaults")

	link, otherLink := solana.NewWallet().PublicKey(), solana.NewWallet().PublicKey()
	got, err = findLinkMint(store(linkRef(sel, link, tokensops.Version), linkRef(other, otherLink, tokensops.Version)), sel)
	require.NoError(t, err)
	require.Equal(t, link, got, "only the requested chain is considered")

	legacy := solana.NewWallet().PublicKey()
	got, err = findLinkMint(store(linkRef(sel, legacy, semver.MustParse("1.6.0")), linkRef(sel, link, tokensops.Version)), sel)
	require.NoError(t, err)
	require.Equal(t, link, got, "with several versions, the 2.0.0 ref wins")
}
