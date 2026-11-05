package changesets

import (
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	chainsel "github.com/smartcontractkit/chain-selectors"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/mcms"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	mcms_types "github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

func TestUpgradedProgramRefForInputIsIdempotent(t *testing.T) {
	selector := chainsel.SOLANA_DEVNET.Selector
	address := solana.NewWallet().PublicKey().String()
	refs := []datastore.AddressRef{
		{Address: address, ChainSelector: selector, Type: datastore.ContractType(sequences.UpgradeTypeRouter), Version: semver.MustParse("1.6.0")},
		{Address: address, ChainSelector: selector, Type: datastore.ContractType(sequences.UpgradeTypeRouter), Version: semver.MustParse("2.0.0")},
	}
	got, add, err := upgradedProgramRefForInput(refs, UpgradeProgramChangesetInput{
		ChainSelector: selector,
		ProgramType:   sequences.UpgradeTypeRouter,
	})
	require.NoError(t, err)
	require.False(t, add)
	require.Equal(t, datastore.AddressRef{}, got)
	require.Len(t, refs, 2, "an existing 2.0.0 reference must not be duplicated")

	refs[1].Address = solana.NewWallet().PublicKey().String()
	_, _, err = upgradedProgramRefForInput(refs, UpgradeProgramChangesetInput{
		ChainSelector: selector,
		ProgramType:   sequences.UpgradeTypeRouter,
	})
	require.ErrorContains(t, err, "differs from legacy ref")
}

func TestUpgradedProgramRefPreservesExplicitCCTPQualifier(t *testing.T) {
	selector := chainsel.SOLANA_DEVNET.Selector
	address := solana.NewWallet().PublicKey().String()
	refs := []datastore.AddressRef{{
		Address:       address,
		ChainSelector: selector,
		Type:          datastore.ContractType(sequences.UpgradeTypeCCTPTokenPool),
		Version:       semver.MustParse("1.6.0"),
		Qualifier:     "USDC",
	}}
	ref, add, err := upgradedProgramRefForInput(refs, UpgradeProgramChangesetInput{
		ChainSelector: selector,
		ProgramType:   sequences.UpgradeTypeCCTPTokenPool,
		Qualifier:     "USDC",
	})
	require.NoError(t, err)
	require.True(t, add)
	require.Equal(t, "USDC", ref.Qualifier)
}

func TestUpgradedProgramRefForFirstDeploymentUsesExplicitAddress(t *testing.T) {
	selector := chainsel.SOLANA_DEVNET.Selector
	address := solana.NewWallet().PublicKey().String()
	input := UpgradeProgramChangesetInput{
		ChainSelector:  selector,
		ProgramType:    sequences.UpgradeTypeTestSenderLegacy,
		ProgramAddress: address,
	}
	ref, add, err := upgradedProgramRefForInput(nil, input)
	require.NoError(t, err)
	require.True(t, add)
	require.Equal(t, address, ref.Address)
	require.Equal(t, "TestSender", ref.Type.String())
	require.Equal(t, "default", ref.Qualifier)
	require.Equal(t, semver.MustParse("1.0.0"), ref.Version)

	ref, add, err = upgradedProgramRefForInput([]datastore.AddressRef{ref}, input)
	require.NoError(t, err)
	require.False(t, add)

	input.ProgramAddress = solana.NewWallet().PublicKey().String()
	_, _, err = upgradedProgramRefForInput([]datastore.AddressRef{{
		Address:       address,
		ChainSelector: selector,
		Type:          "TestSender",
		Version:       semver.MustParse("1.0.0"),
		Qualifier:     "default",
	}}, input)
	require.ErrorContains(t, err, "differs from requested programAddress")
}

func TestUpgradeProgramVerifyValidatesVersionProgramTypeAndMCMSDefaults(t *testing.T) {
	selector := chainsel.SOLANA_DEVNET.Selector
	env := cldf.Environment{BlockChains: cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{
		selector: cldf_solana.Chain{Selector: selector},
	})}
	input := UpgradeProgramChangesetInput{
		ChainSelector: selector,
		ProgramType:   sequences.UpgradeTypeRouter,
		Version:       "v2.0.0",
		MCMS: mcms.Input{
			TimelockAction: mcms_types.TimelockActionSchedule,
		},
	}
	require.NoError(t, upgradeProgramVerify(env, input), "ValidUntil should be defaulted when omitted")

	badAddress := input
	badAddress.ProgramAddress = "not-a-public-key"
	require.ErrorContains(t, upgradeProgramVerify(env, badAddress), "invalid programAddress")

	inPlaceAddress := input
	inPlaceAddress.ProgramAddress = solana.NewWallet().PublicKey().String()
	require.ErrorContains(t, upgradeProgramVerify(env, inPlaceAddress), "must be omitted for in-place upgrades")

	badVersion := input
	badVersion.Version = "2.0.0"
	require.ErrorContains(t, upgradeProgramVerify(env, badVersion), "invalid version")

	badProgram := input
	badProgram.ProgramType = "UnsupportedProgram"
	require.ErrorContains(t, upgradeProgramVerify(env, badProgram), "unsupported programType")
}

func TestValidateMintAuthorityUpdates(t *testing.T) {
	pool := datastore.AddressRef{Address: solana.NewWallet().PublicKey().String()}
	mint := solana.NewWallet().PublicKey()
	msig := solana.NewWallet().PublicKey()
	valid := TokenMintAuthorityUpdate{TokenPoolRef: pool, TokenMint: mint, NewMintAuthority: msig}

	require.NoError(t, validateMintAuthorityUpdates([]TokenMintAuthorityUpdate{valid}))
	require.ErrorContains(t, validateMintAuthorityUpdates(nil), "at least one update")

	for wantErr, u := range map[string]TokenMintAuthorityUpdate{
		"tokenPoolRef must not be empty":    {TokenMint: mint, NewMintAuthority: msig},
		"tokenMint must not be zero":        {TokenPoolRef: pool, NewMintAuthority: msig},
		"newMintAuthority must not be zero": {TokenPoolRef: pool, TokenMint: mint},
	} {
		require.ErrorContains(t, validateMintAuthorityUpdates([]TokenMintAuthorityUpdate{u}), wantErr)
	}
	require.ErrorContains(t, validateMintAuthorityUpdates([]TokenMintAuthorityUpdate{valid, valid}), "duplicate tokenMint")
}

func TestRMNSetCurserVerify(t *testing.T) {
	sel := chainsel.SOLANA_DEVNET.Selector
	env := cldf.Environment{BlockChains: cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{
		sel: cldf_solana.Chain{Selector: sel},
	})}
	input := func(selector uint64, curser solana.PublicKey) RMNRemoteSetCurserChangesetInput {
		return RMNRemoteSetCurserChangesetInput{
			ChainSelector: selector,
			Curser:        curser,
			MCMS: mcms.Input{
				TimelockAction: mcms_types.TimelockActionSchedule,
				ValidUntil:     uint32(time.Now().Add(time.Hour).Unix()), //nolint:gosec // fits in uint32 until 2106
			},
		}
	}

	require.NoError(t, rmnSetCurserVerify(env, input(sel, solana.NewWallet().PublicKey())))
	require.ErrorContains(t, rmnSetCurserVerify(env, input(sel, solana.PublicKey{})), "curser public key must not be zero")
	require.ErrorContains(t, rmnSetCurserVerify(env, input(chainsel.SOLANA_MAINNET.Selector, solana.NewWallet().PublicKey())), "does not exist")
}

func TestLanesV2Verify(t *testing.T) {
	sel := chainsel.SOLANA_DEVNET.Selector
	remote := chainsel.ETHEREUM_TESTNET_SEPOLIA.Selector
	env := cldf.Environment{BlockChains: cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{
		sel: cldf_solana.Chain{Selector: sel},
	})}
	mcmsInput := mcms.Input{
		TimelockAction: mcms_types.TimelockActionSchedule,
		ValidUntil:     uint32(time.Now().Add(time.Hour).Unix()), //nolint:gosec // fits in uint32 until 2106
	}
	wallet := solana.NewWallet().PublicKey().String()
	enabled := true
	input := func(remotes []uint64, enabled bool, senders ...string) SetSolanaLanesV2AllowlistChangesetInput {
		return SetSolanaLanesV2AllowlistChangesetInput{
			ChainSelector:        sel,
			RemoteChainSelectors: remotes,
			Enabled:              &enabled,
			AllowedSendersToAdd:  senders,
			MCMS:                 mcmsInput,
		}
	}

	require.NoError(t, setSolanaLanesV2AllowlistVerify(env, input([]uint64{remote}, true, wallet)))
	require.NoError(t, setSolanaLanesV2AllowlistVerify(env, input([]uint64{remote}, false)))
	require.NoError(t, setSolanaLanesV2AllowlistVerify(env, input([]uint64{remote}, true)), "enabling with the lane's kept list is allowed")
	require.ErrorContains(t, setSolanaLanesV2AllowlistVerify(env, input([]uint64{remote}, true, wallet, wallet)), "duplicate allowed sender")
	require.ErrorContains(t, setSolanaLanesV2AllowlistVerify(env, input([]uint64{remote}, true, "not-a-key")), "invalid allowed allowlist sender")
	require.ErrorContains(t, setSolanaLanesV2AllowlistVerify(env, input([]uint64{remote}, true, solana.PublicKey{}.String())), "zero key")
	require.ErrorContains(t, setSolanaLanesV2AllowlistVerify(env, SetSolanaLanesV2AllowlistChangesetInput{
		ChainSelector:          sel,
		RemoteChainSelectors:   []uint64{remote},
		Enabled:                &enabled,
		AllowedSendersToAdd:    []string{wallet},
		AllowedSendersToRemove: []string{wallet},
		MCMS:                   mcmsInput,
	}), "is in both the senders to add and the senders to remove")
	require.ErrorContains(t, setSolanaLanesV2AllowlistVerify(env, input(nil, true, wallet)), "at least one remote chain selector")
	require.ErrorContains(t, setSolanaLanesV2AllowlistVerify(env, input([]uint64{remote, remote}, true, wallet)), "duplicate remote chain selector")
	require.ErrorContains(t, setSolanaLanesV2AllowlistVerify(env, input([]uint64{sel}, true, wallet)), "is the chain itself")

	require.NoError(t, lanesV2Verify(env, LanesV2ChangesetInput{ChainSelector: sel, RemoteChainSelectors: []uint64{remote}, MCMS: mcmsInput}))
	require.ErrorContains(t, lanesV2Verify(env, LanesV2ChangesetInput{ChainSelector: chainsel.SOLANA_MAINNET.Selector, RemoteChainSelectors: []uint64{remote}, MCMS: mcmsInput}), "does not exist")
	require.NoError(t, lanesV2Verify(env, LanesV2ChangesetInput{ChainSelector: sel, RemoteChainSelectors: []uint64{remote}}), "MCMS is only needed when the timelock owns the Router")
	require.NoError(t, lanesV2Verify(env, LanesV2ChangesetInput{ChainSelector: sel, RemoteChainSelectors: []uint64{remote}, MCMS: mcms.Input{Qualifier: "CLLCCIP"}}), "valid until and timelock action are defaulted")
	require.ErrorContains(t, lanesV2Verify(env, LanesV2ChangesetInput{ChainSelector: sel, RemoteChainSelectors: []uint64{remote}, MCMS: mcms.Input{TimelockAction: "nope"}}), "invalid MCMS configuration")
}

func TestMigrateCctpTokenPoolToV2Verify(t *testing.T) {
	sel := chainsel.SOLANA_DEVNET.Selector
	ds := datastore.NewMemoryDataStore()
	require.NoError(t, ds.Addresses().Add(datastore.AddressRef{
		ChainSelector: sel,
		Type:          "CCTPTokenPool",
		Version:       semver.MustParse("1.6.0"),
		Qualifier:     "USDC",
		Address:       solana.NewWallet().PublicKey().String(),
	}))
	env := cldf.Environment{
		BlockChains: cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{
			sel: cldf_solana.Chain{Selector: sel},
		}),
		DataStore: ds.Seal(),
	}
	mcmsInput := mcms.Input{
		TimelockAction: mcms_types.TimelockActionSchedule,
		ValidUntil:     uint32(time.Now().Add(time.Hour).Unix()), //nolint:gosec // fits in uint32 until 2106
	}
	mint := solana.NewWallet().PublicKey().String()
	input := func(selector uint64, token string, m mcms.Input) MigrateCctpTokenPoolToV2ChangesetInput {
		return MigrateCctpTokenPoolToV2ChangesetInput{ChainSelector: selector, USDCToken: token, MCMS: m}
	}

	require.NoError(t, migrateCctpTokenPoolToV2Verify(env, input(sel, mint, mcmsInput)))
	require.NoError(t, migrateCctpTokenPoolToV2Verify(env, input(sel, mint, mcms.Input{})), "MCMS is optional")
	require.ErrorContains(t, migrateCctpTokenPoolToV2Verify(env, input(chainsel.SOLANA_MAINNET.Selector, mint, mcmsInput)), "does not exist")
	require.ErrorContains(t, migrateCctpTokenPoolToV2Verify(env, input(sel, "not-a-key", mcmsInput)), "invalid USDC token")
	require.ErrorContains(t, migrateCctpTokenPoolToV2Verify(env, input(sel, mint, mcms.Input{TimelockAction: mcms_types.TimelockActionSchedule, ValidUntil: 1})),
		"ValidUntil", "a past ValidUntil is rejected")

	emptyDS := datastore.NewMemoryDataStore()
	envNoPool := cldf.Environment{BlockChains: env.BlockChains, DataStore: emptyDS.Seal()}
	require.ErrorContains(t, migrateCctpTokenPoolToV2Verify(envNoPool, input(sel, mint, mcms.Input{})), "no 1.x CCTPTokenPool ref")
}

func TestUpgradedProgramRefs(t *testing.T) {
	sel := chainsel.SOLANA_DEVNET.Selector
	v160 := semver.MustParse("1.6.0")
	v200 := semver.MustParse("2.0.0")
	ref := func(contractType string, version *semver.Version, address string) datastore.AddressRef {
		return datastore.AddressRef{ChainSelector: sel, Type: datastore.ContractType(contractType), Version: version, Address: address}
	}
	legacy := []datastore.AddressRef{
		ref("Router", v160, "router"),
		ref("FeeQuoter", v160, "fq"),
		ref("RMNRemote", v160, "rmn"),
		// Another chain's refs are ignored.
		{ChainSelector: chainsel.SOLANA_MAINNET.Selector, Type: "Router", Version: v200, Address: "other"},
	}

	toAdd, err := upgradedProgramRefs(legacy, sel)
	require.NoError(t, err)
	require.Equal(t, []datastore.AddressRef{
		ref("Router", v200, "router"),
		ref("FeeQuoter", v200, "fq"),
		ref("RMNRemote", v200, "rmn"),
	}, toAdd, "each program gets a 2.0.0 ref at its 1.6.0 address")

	toAdd, err = upgradedProgramRefs(append(legacy, ref("Router", v200, "router")), sel)
	require.NoError(t, err)
	require.Len(t, toAdd, 2, "an existing 2.0.0 ref at the same address is skipped")

	_, err = upgradedProgramRefs(append(legacy, ref("Router", v200, "elsewhere")), sel)
	require.ErrorContains(t, err, "Router 2.0.0 is registered at elsewhere")

	_, err = upgradedProgramRefs(legacy[1:], sel)
	require.ErrorContains(t, err, "no Router 1.6.0 ref")
}
