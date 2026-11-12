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

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

var (
	v100 = semver.MustParse("1.0.0")
	v160 = semver.MustParse("1.6.0")
	v200 = semver.MustParse("2.0.0")
	v201 = semver.MustParse("2.0.1")
)

func TestUpgradedProgramRefForInputIsIdempotent(t *testing.T) {
	selector := chainsel.SOLANA_DEVNET.Selector
	address := solana.NewWallet().PublicKey().String()
	refs := []datastore.AddressRef{
		{Address: address, ChainSelector: selector, Type: datastore.ContractType(sequences.UpgradeTypeRouter), Version: v160},
		{Address: address, ChainSelector: selector, Type: datastore.ContractType(sequences.UpgradeTypeRouter), Version: v200},
	}
	change, err := upgradedProgramRefForInput(refs, UpgradeProgramChangesetInput{
		ChainSelector: selector,
		ProgramType:   sequences.UpgradeTypeRouter,
	}, v200)
	require.NoError(t, err)
	require.Equal(t, refChange{}, change)
	require.Len(t, refs, 2, "an existing 2.0.0 reference must not be duplicated")

	refs[1].Address = solana.NewWallet().PublicKey().String()
	_, err = upgradedProgramRefForInput(refs, UpgradeProgramChangesetInput{
		ChainSelector: selector,
		ProgramType:   sequences.UpgradeTypeRouter,
	}, v200)
	require.ErrorContains(t, err, "differs from the 1.6.0 ref")
}

func TestUpgradedProgramRefUpdatesTheVersionOfTheExistingRef(t *testing.T) {
	selector := chainsel.SOLANA_DEVNET.Selector
	address := solana.NewWallet().PublicKey().String()
	old := datastore.AddressRef{
		Address:       address,
		ChainSelector: selector,
		Type:          datastore.ContractType(sequences.UpgradeTypeCCTPTokenPool),
		Version:       v160,
		Qualifier:     "USDC",
		Labels:        datastore.NewLabelSet("legacy"),
	}
	input := UpgradeProgramChangesetInput{ChainSelector: selector, ProgramType: sequences.UpgradeTypeCCTPTokenPool, Qualifier: "USDC"}

	change, err := upgradedProgramRefForInput([]datastore.AddressRef{old}, input, v200)
	require.NoError(t, err)
	require.NotNil(t, change.add)
	require.Equal(t, address, change.add.Address)
	require.Equal(t, "USDC", change.add.Qualifier)
	require.True(t, v200.Equal(change.add.Version))
	require.Equal(t, old.Labels, change.add.Labels)
	require.Empty(t, change.superseded, "the 1.x ref stays: other integrations still resolve it")

	t.Run("to the version of the release being deployed", func(t *testing.T) {
		change, err := upgradedProgramRefForInput([]datastore.AddressRef{old}, input, v201)
		require.NoError(t, err)
		require.NotNil(t, change.add)
		require.True(t, v201.Equal(change.add.Version))
		require.Empty(t, change.superseded)
	})

	t.Run("a deployed ref of the same major is replaced", func(t *testing.T) {
		deployed := old
		deployed.Version = v200
		change, err := upgradedProgramRefForInput([]datastore.AddressRef{old, deployed}, input, v201)
		require.NoError(t, err)
		require.NotNil(t, change.add)
		require.True(t, v201.Equal(change.add.Version))
		require.Equal(t, []datastore.AddressRef{deployed}, change.superseded, "only 2.0.1 remains next to the 1.6.0 ref")
	})

	t.Run("a stale same-major ref is removed once the target version exists", func(t *testing.T) {
		stale, target := old, old
		stale.Version, target.Version = v200, v201
		change, err := upgradedProgramRefForInput([]datastore.AddressRef{old, stale, target}, input, v201)
		require.NoError(t, err)
		require.Nil(t, change.add)
		require.Equal(t, []datastore.AddressRef{stale}, change.superseded)
	})

	t.Run("the same-major ref moves to the new version for a program per version", func(t *testing.T) {
		offramp := datastore.AddressRef{Address: address, ChainSelector: selector, Type: datastore.ContractType(sequences.UpgradeTypeOffRamp), Version: v200}
		change, err := upgradedProgramRefForInput([]datastore.AddressRef{offramp}, UpgradeProgramChangesetInput{ChainSelector: selector, ProgramType: sequences.UpgradeTypeOffRamp}, v201)
		require.NoError(t, err)
		require.True(t, v201.Equal(change.add.Version))
		require.Equal(t, []datastore.AddressRef{offramp}, change.superseded)
	})

	t.Run("refs of other qualifiers are not touched", func(t *testing.T) {
		other := old
		other.Qualifier = "OTHER"
		_, err := upgradedProgramRefForInput([]datastore.AddressRef{other}, input, v200)
		require.ErrorContains(t, err, "set programAddress")
	})

	t.Run("the requested address has to be the deployed one", func(t *testing.T) {
		in := input
		in.ProgramAddress = solana.NewWallet().PublicKey().String()
		_, err := upgradedProgramRefForInput([]datastore.AddressRef{old}, in, v200)
		require.ErrorContains(t, err, "differs from requested programAddress")
	})

	t.Run("two refs of other versions cannot be told apart", func(t *testing.T) {
		second := old
		second.Version = semver.MustParse("1.5.0")
		second.Address = solana.NewWallet().PublicKey().String()
		_, err := upgradedProgramRefForInput([]datastore.AddressRef{old, second}, input, v200)
		require.ErrorContains(t, err, "cannot tell which one is deployed")
	})
}

func TestUpgradedProgramRefRegistersAProgramWithoutARef(t *testing.T) {
	selector := chainsel.SOLANA_DEVNET.Selector
	address := solana.NewWallet().PublicKey().String()
	input := UpgradeProgramChangesetInput{ChainSelector: selector, ProgramType: sequences.UpgradeTypeFeeQuoter}

	_, err := upgradedProgramRefForInput(nil, input, v200)
	require.ErrorContains(t, err, "set programAddress")

	input.ProgramAddress = address
	change, err := upgradedProgramRefForInput(nil, input, v200)
	require.NoError(t, err)
	require.Nil(t, change.superseded)
	require.NotNil(t, change.add)
	require.Equal(t, address, change.add.Address)
	require.True(t, v200.Equal(change.add.Version))
}

func TestUpgradedProgramRefAddsARefPerVersionForTheOffRamp(t *testing.T) {
	selector := chainsel.SOLANA_DEVNET.Selector
	v16Address := solana.NewWallet().PublicKey().String()
	v2Address := solana.NewWallet().PublicKey().String()
	v16 := datastore.AddressRef{Address: v16Address, ChainSelector: selector, Type: datastore.ContractType(sequences.UpgradeTypeOffRamp), Version: v160}
	input := UpgradeProgramChangesetInput{ChainSelector: selector, ProgramType: sequences.UpgradeTypeOffRamp}

	_, err := upgradedProgramRefForInput([]datastore.AddressRef{v16}, input, v200)
	require.ErrorContains(t, err, "set programAddress", "the new version is another program, whose address has to be given")

	input.ProgramAddress = v2Address
	change, err := upgradedProgramRefForInput([]datastore.AddressRef{v16}, input, v200)
	require.NoError(t, err)
	require.NotNil(t, change.add)
	require.Nil(t, change.superseded, "the 1.6.0 OffRamp is still deployed")
	require.Equal(t, v2Address, change.add.Address)

	// A rerun finds the 2.0.0 ref; the 1.6.0 ref at another address is fine.
	change, err = upgradedProgramRefForInput([]datastore.AddressRef{v16, *change.add}, input, v200)
	require.NoError(t, err)
	require.Nil(t, change.add)
}

func TestProgramRefVersion(t *testing.T) {
	t.Setenv(solcommon.ContractsReleaseEnvVar, "")

	for name, tc := range map[string]struct {
		programType sequences.ProgramUpgradeType
		version     string
		want        *semver.Version
	}{
		"local build: 2.0.0":                         {sequences.UpgradeTypeRouter, "local", v200},
		"local build: the v1 test sender is 1.0.0":   {sequences.UpgradeTypeTestSenderLegacy, "local", v100},
		"local build: the v1 test receiver is 1.0.0": {sequences.UpgradeTypeTestReceiverLegacy, "local", v100},
		"local build: the v2 test sender is 2.0.0":   {sequences.UpgradeTypeTestSenderV2, "local", v200},
		"a release version":                          {sequences.UpgradeTypeCCTPTokenPool, "v2.0.1", v201},
		"a contracts release tag":                    {sequences.UpgradeTypeOffRamp, "contracts/v2.0.1", v201},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := programRefVersion(tc.programType, tc.version, "")
			require.NoError(t, err)
			require.True(t, tc.want.Equal(got), "got %s, want %s", got, tc.want)
		})
	}

	t.Run("the pinned release of the environment when the pipeline gives none", func(t *testing.T) {
		t.Setenv(solcommon.ContractsReleaseEnvVar, "v2.0.1")
		got, err := programRefVersion(sequences.UpgradeTypeRouter, "", "")
		require.NoError(t, err)
		require.True(t, v201.Equal(got))
	})

	t.Run("no version at all", func(t *testing.T) {
		_, err := programRefVersion(sequences.UpgradeTypeRouter, "", "")
		require.Error(t, err)
	})
}

func TestUpgradedProgramRefForFirstDeploymentUsesExplicitAddress(t *testing.T) {
	selector := chainsel.SOLANA_DEVNET.Selector
	address := solana.NewWallet().PublicKey().String()
	input := UpgradeProgramChangesetInput{
		ChainSelector:  selector,
		ProgramType:    sequences.UpgradeTypeTestSenderLegacy,
		ProgramAddress: address,
	}
	change, err := upgradedProgramRefForInput(nil, input, v100)
	require.NoError(t, err)
	require.NotNil(t, change.add)
	ref := *change.add
	require.Equal(t, address, ref.Address)
	require.Equal(t, "TestSender", ref.Type.String())
	require.Equal(t, "default", ref.Qualifier)
	require.Equal(t, semver.MustParse("1.0.0"), ref.Version)

	change, err = upgradedProgramRefForInput([]datastore.AddressRef{ref}, input, v100)
	require.NoError(t, err)
	require.Nil(t, change.add)

	input.ProgramAddress = solana.NewWallet().PublicKey().String()
	_, err = upgradedProgramRefForInput([]datastore.AddressRef{{
		Address:       address,
		ChainSelector: selector,
		Type:          "TestSender",
		Version:       semver.MustParse("1.0.0"),
		Qualifier:     "default",
	}}, input, v100)
	require.ErrorContains(t, err, "differs from requested programAddress")

	// The v2 test sender is a separate program next to the v1 one.
	v2Address := solana.NewWallet().PublicKey().String()
	v2 := UpgradeProgramChangesetInput{ChainSelector: selector, ProgramType: sequences.UpgradeTypeTestSenderV2, ProgramAddress: v2Address}
	v1Ref := datastore.AddressRef{Address: address, ChainSelector: selector, Type: "TestSender", Version: v100, Qualifier: "default"}
	change, err = upgradedProgramRefForInput([]datastore.AddressRef{v1Ref}, v2, v200)
	require.NoError(t, err)
	require.Nil(t, change.superseded)
	require.NotNil(t, change.add)
	require.Equal(t, v2Address, change.add.Address)
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

	withAddress := input
	withAddress.ProgramAddress = solana.NewWallet().PublicKey().String()
	require.NoError(t, upgradeProgramVerify(env, withAddress))

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

func TestSetPoolInterfaceImplementationVerify(t *testing.T) {
	sel := chainsel.SOLANA_DEVNET.Selector
	ds := datastore.NewMemoryDataStore()
	require.NoError(t, ds.Addresses().Add(datastore.AddressRef{
		ChainSelector: sel,
		Type:          "Router",
		Version:       semver.MustParse("2.0.0"),
		Address:       solana.NewWallet().PublicKey().String(),
	}))
	env := cldf.Environment{
		BlockChains: cldf_chain.NewBlockChains(map[uint64]cldf_chain.BlockChain{
			sel: cldf_solana.Chain{Selector: sel},
		}),
		DataStore: ds.Seal(),
	}
	mint := solana.NewWallet().PublicKey().String()
	input := func(token string, version uint8, resolution string) SetPoolInterfaceImplementationChangesetInput {
		return SetPoolInterfaceImplementationChangesetInput{ChainSelector: sel, TokenMint: token, InterfaceVersion: version, AccountResolution: resolution}
	}

	require.NoError(t, setPoolInterfaceImplementationVerify(env, input(mint, 2, AccountResolutionWithoutExtendedLookupTable)))
	require.NoError(t, setPoolInterfaceImplementationVerify(env, input(mint, 1, AccountResolutionNoResolution)))
	require.ErrorContains(t, setPoolInterfaceImplementationVerify(env, input("not-a-key", 2, AccountResolutionWithoutExtendedLookupTable)), "invalid token mint")
	require.ErrorContains(t, setPoolInterfaceImplementationVerify(env, input(mint, 3, AccountResolutionWithoutExtendedLookupTable)), "interfaceVersion")
	require.ErrorContains(t, setPoolInterfaceImplementationVerify(env, input(mint, 2, "2")), "invalid accountResolution")
	// Each interface version takes a single resolution mode, as the Router enforces.
	require.ErrorContains(t, setPoolInterfaceImplementationVerify(env, input(mint, 2, AccountResolutionStandard)), "requires account resolution WithoutExtendedLookupTable")
	require.ErrorContains(t, setPoolInterfaceImplementationVerify(env, input(mint, 2, AccountResolutionNoResolution)), "requires account resolution WithoutExtendedLookupTable")
	require.ErrorContains(t, setPoolInterfaceImplementationVerify(env, input(mint, 1, AccountResolutionWithoutExtendedLookupTable)), "requires account resolution NoResolution")
	envNoRouter := cldf.Environment{BlockChains: env.BlockChains, DataStore: datastore.NewMemoryDataStore().Seal()}
	require.ErrorContains(t, setPoolInterfaceImplementationVerify(envNoRouter, input(mint, 2, AccountResolutionWithoutExtendedLookupTable)), "find router")
}
