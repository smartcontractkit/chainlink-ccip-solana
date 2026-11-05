package sequences

import (
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	tokenapi "github.com/smartcontractkit/chainlink-ccip/deployment/tokens"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	tokensops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokens"
)

func TestValidateManualRegistrationInput(t *testing.T) {
	owner := solana.NewWallet().PublicKey()
	customer := solana.NewWallet().PublicKey()
	input := func(proposedOwner string, args *tokenapi.SVMExtraArgs) tokenapi.ManualRegistrationSequenceInput {
		return tokenapi.ManualRegistrationSequenceInput{RegisterTokenConfig: tokenapi.RegisterTokenConfig{
			ProposedOwner: proposedOwner,
			SVMExtraArgs:  args,
		}}
	}

	got, err := validateManualRegistrationInput(input(owner.String(), nil))
	require.NoError(t, err)
	require.Equal(t, owner, got)

	got, err = validateManualRegistrationInput(input("", &tokenapi.SVMExtraArgs{SkipTokenPoolInit: true}))
	require.NoError(t, err, "no owner is needed when the pool is not initialized")
	require.True(t, got.IsZero())

	_, err = validateManualRegistrationInput(input("", nil))
	require.ErrorContains(t, err, "proposed owner is required")

	_, err = validateManualRegistrationInput(input("not-base58!", nil))
	require.ErrorContains(t, err, "failed to parse proposed owner")

	_, err = validateManualRegistrationInput(input(owner.String(), &tokenapi.SVMExtraArgs{TransferMintAuthority: true}))
	require.ErrorContains(t, err, "no non-zero customer mint authorities")

	_, err = validateManualRegistrationInput(input(owner.String(), &tokenapi.SVMExtraArgs{
		TransferMintAuthority:   true,
		CustomerMintAuthorities: []solana.PublicKey{{}},
	}))
	require.ErrorContains(t, err, "no non-zero customer mint authorities", "zero keys are dropped from the multisig")

	_, err = validateManualRegistrationInput(input(owner.String(), &tokenapi.SVMExtraArgs{
		CustomerMintAuthorities: []solana.PublicKey{{}, {}},
	}))
	require.ErrorContains(t, err, "at least one non-zero key")

	tooMany := make([]solana.PublicKey, maxCustomerMintAuthorities+1)
	for i := range tooMany {
		tooMany[i] = solana.NewWallet().PublicKey()
	}
	_, err = validateManualRegistrationInput(input(owner.String(), &tokenapi.SVMExtraArgs{CustomerMintAuthorities: tooMany}))
	require.ErrorContains(t, err, "too many non-zero customer mint authorities")

	// Zero keys don't count towards the limit
	_, err = validateManualRegistrationInput(input(owner.String(), &tokenapi.SVMExtraArgs{
		CustomerMintAuthorities: append(tooMany[:maxCustomerMintAuthorities:maxCustomerMintAuthorities], solana.PublicKey{}),
	}))
	require.NoError(t, err)

	_, err = validateManualRegistrationInput(input(owner.String(), &tokenapi.SVMExtraArgs{
		TransferMintAuthority:   true,
		CustomerMintAuthorities: []solana.PublicKey{customer},
	}))
	require.NoError(t, err)
}

func TestMultisigSigners(t *testing.T) {
	poolSigner := solana.NewWallet().PublicKey()
	a, b := solana.NewWallet().PublicKey(), solana.NewWallet().PublicKey()
	require.Equal(t,
		[]solana.PublicKey{poolSigner, a, b},
		multisigSigners(poolSigner, []solana.PublicKey{a, {}, b}),
		"pool signer comes first and zero keys are dropped")
}

func TestNewSolanaTokenRef(t *testing.T) {
	const selector = uint64(16423721717087811551)
	mint := solana.NewWallet().PublicKey()

	ref, err := newSolanaTokenRef(selector, mint, solana.Token2022ProgramID, "BTC.b")
	require.NoError(t, err)
	require.Equal(t, datastore.AddressRef{
		ChainSelector: selector,
		Qualifier:     "BTC.b",
		Address:       mint.String(),
		Version:       tokensops.Version,
		Labels:        datastore.NewLabelSet(),
		Type:          datastore.ContractType(solcommon.SPL2022Tokens),
	}, ref)

	ref, err = newSolanaTokenRef(selector, mint, solana.TokenProgramID, "LBTC")
	require.NoError(t, err)
	require.Equal(t, datastore.ContractType(solcommon.SPLTokens), ref.Type)

	_, err = newSolanaTokenRef(selector, mint, solana.SystemProgramID, "X")
	require.Error(t, err, "only SPL and Token-2022 mints are supported")
}
