package sequences

import (
	"context"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/fee_quoter"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	tokensops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokens"
)

// feeToken is a token CCIP fees can be paid in, with the token program that owns its mint.
type feeToken struct {
	mint    solana.PublicKey
	program solana.PublicKey
}

// feeTokens lists the tokens fees can be paid in: WSOL, and LINK when link is set. On ccip_send
// the router forwards each receipt issuer's share into the issuer's vault for the fee token, so
// every fee token needs a vault per issuer.
func feeTokens(ctx context.Context, chain cldf_solana.Chain, link solana.PublicKey) ([]feeToken, error) {
	out := make([]feeToken, 0, 2)
	out = append(out, feeToken{mint: solana.WrappedSol, program: solana.TokenProgramID})
	if link.IsZero() {
		return out, nil
	}
	program, err := solcommon.FetchTokenProgramID(ctx, chain, link)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve token program for LINK %s: %w", link, err)
	}
	return append(out, feeToken{mint: link, program: program}), nil
}

// findLinkMint returns the chain's LINK mint from the address refs, or zero when it has none. LINK is the
// same for every version, so it takes the ref at the latest version whichever versions are registered.
func findLinkMint(addresses []datastore.AddressRef, chainSelector uint64) (solana.PublicKey, error) {
	ref, err := solcommon.FindLatestRefInRefs(addresses, chainSelector, datastore.ContractType(tokensops.LinkContractType), "")
	if errors.Is(err, solcommon.ErrAddressRefNotFound) {
		return solana.PublicKey{}, nil
	}
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to resolve LINK mint on chain %d: %w", chainSelector, err)
	}
	mint, err := solana.PublicKeyFromBase58(ref.Address)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to parse LINK mint %q: %w", ref.Address, err)
	}
	return mint, nil
}

// ensureFeeVaults creates the issuer's fee vault token account for each fee token, skipping
// those that already exist. qualifier is nil for an unqualified vault.
// accountExists reports whether an account exists. An RPC error other than not found is returned.
func accountExists(ctx context.Context, chain cldf_solana.Chain, account solana.PublicKey) (bool, error) {
	_, err := chain.Client.GetAccountInfoWithOpts(ctx, account, &rpc.GetAccountInfoOpts{
		Commitment: cldf_solana.SolDefaultCommitment,
	})
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, rpc.ErrNotFound):
		return false, nil
	default:
		return false, err
	}
}

func ensureFeeVaults(ctx context.Context, chain cldf_solana.Chain, issuer solana.PublicKey, qualifier []byte, fees []feeToken) error {
	vault, _, err := state.FindFeeVaultPDA(issuer, qualifier)
	if err != nil {
		return fmt.Errorf("failed to find fee vault of %s: %w", issuer, err)
	}
	for _, ft := range fees {
		ata, _, err := tokens.FindAssociatedTokenAddress(ft.program, ft.mint, vault)
		if err != nil {
			return fmt.Errorf("failed to find fee vault account of %s for %s: %w", issuer, ft.mint, err)
		}
		exists, err := accountExists(ctx, chain, ata)
		if err != nil {
			return fmt.Errorf("failed to fetch fee vault account %s of %s for %s: %w", ata, issuer, ft.mint, err)
		}
		if exists {
			continue
		}
		ixn, _, err := tokens.CreateAssociatedTokenAccount(ft.program, ft.mint, vault, chain.Deployer().PublicKey())
		if err != nil {
			return fmt.Errorf("failed to build fee vault account of %s for %s: %w", issuer, ft.mint, err)
		}
		if err = chain.Confirm([]solana.Instruction{ixn}); err != nil {
			return fmt.Errorf("failed to create fee vault account %s of %s for %s: %w", ata, issuer, ft.mint, err)
		}
	}
	return nil
}

// EnsureIssuerFeeVaults creates, idempotently, the unqualified fee vault token accounts of a receipt
// issuer program such as the committee verifier or the executor. The router pays each issuer's share
// of the fee into these accounts on ccip_send, and the transfer fails with InvalidAccountData when
// one does not exist. Programs registered without going through their deploy sequence must call
// this to get them.
//
// The fee tokens are read on-chain: WSOL, and the LINK mint recorded in the FeeQuoter's config
// account when it has one. The accounts are not recorded anywhere, as they are derivable from the
// issuer and the mint.
func EnsureIssuerFeeVaults(ctx context.Context, chain cldf_solana.Chain, feeQuoter, issuer solana.PublicKey) error {
	configPDA, _, err := state.FindFqConfigPDA(feeQuoter)
	if err != nil {
		return fmt.Errorf("failed to find FeeQuoter config of %s: %w", feeQuoter, err)
	}
	var config fee_quoter.Config
	if err = common.GetAccountDataBorshIntoAnchor(ctx, chain.Client, configPDA, cldf_solana.SolDefaultCommitment, &config); err != nil {
		return fmt.Errorf("failed to read FeeQuoter config %s: %w", configPDA, err)
	}
	fees, err := feeTokens(ctx, chain, config.LinkTokenMint)
	if err != nil {
		return err
	}
	return ensureFeeVaults(ctx, chain, issuer, nil, fees)
}
