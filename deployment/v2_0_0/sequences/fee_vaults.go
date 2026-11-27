package sequences

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_ops "github.com/smartcontractkit/chainlink-deployments-framework/operations"

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

// ensureFeeVaults creates the issuer's fee vault token account for each fee token, skipping
// those that already exist, and returns the accounts it created. qualifier is nil for an
// unqualified vault.
func ensureFeeVaults(ctx context.Context, chain cldf_solana.Chain, issuer solana.PublicKey, qualifier []byte, fees []feeToken) ([]solana.PublicKey, error) {
	vault, _, err := state.FindFeeVaultPDA(issuer, qualifier)
	if err != nil {
		return nil, fmt.Errorf("failed to find fee vault of %s: %w", issuer, err)
	}
	var created []solana.PublicKey
	for _, ft := range fees {
		ata, _, err := tokens.FindAssociatedTokenAddress(ft.program, ft.mint, vault)
		if err != nil {
			return created, fmt.Errorf("failed to find fee vault account of %s for %s: %w", issuer, ft.mint, err)
		}
		exists, err := accountExists(ctx, chain, ata)
		if err != nil {
			return created, fmt.Errorf("failed to fetch fee vault account %s of %s for %s: %w", ata, issuer, ft.mint, err)
		}
		if exists {
			continue
		}
		ixn, _, err := tokens.CreateAssociatedTokenAccount(ft.program, ft.mint, vault, chain.Deployer().PublicKey())
		if err != nil {
			return created, fmt.Errorf("failed to build fee vault account of %s for %s: %w", issuer, ft.mint, err)
		}
		if err = chain.Confirm([]solana.Instruction{ixn}); err != nil {
			return created, fmt.Errorf("failed to create fee vault account %s of %s for %s: %w", ata, issuer, ft.mint, err)
		}
		created = append(created, ata)
	}
	return created, nil
}

// EnsurePoolTokenAccountsInput selects a 2.0 pool instance (pool program + mint).
type EnsurePoolTokenAccountsInput struct {
	Pool         solana.PublicKey `json:"pool"`
	Mint         solana.PublicKey `json:"mint"`
	TokenProgram solana.PublicKey `json:"tokenProgram"`
	// LinkMint is the chain's LINK mint, or zero when it has none.
	LinkMint solana.PublicKey `json:"linkMint"`
}

// EnsurePoolTokenAccountsOutput lists the token accounts the operation created.
type EnsurePoolTokenAccountsOutput struct {
	Created []solana.PublicKey `json:"created"`
}

// EnsurePoolTokenAccounts creates the token accounts a 2.0 pool instance needs and lacks: the pool
// signer's, and a fee vault account per fee token (where ccip_send pays the pool's fee share) and for
// the pool's own mint (where burn/lock_v2 move the BPS fee). ccip_send fails with
// MissingAtaForReceiptIssuer once the pool charges a fee into a missing vault. Creating an associated
// token account needs no authority, so the deployer sends them directly, also when the rest of the
// changeset is an MCMS proposal; the output lists them.
var EnsurePoolTokenAccounts = cldf_ops.NewOperation(
	"token-pool:ensure-token-accounts",
	solcommon.Version2_0_0,
	"Creates the pool signer and fee vault token accounts of a 2.0 token pool instance",
	func(b cldf_ops.Bundle, chain cldf_solana.Chain, input EnsurePoolTokenAccountsInput) (EnsurePoolTokenAccountsOutput, error) {
		var out EnsurePoolTokenAccountsOutput
		signer, err := tokens.TokenPoolSignerAddress(input.Mint, input.Pool)
		if err != nil {
			return out, fmt.Errorf("derive pool signer address: %w", err)
		}
		signerATA, _, err := tokens.FindAssociatedTokenAddress(input.TokenProgram, input.Mint, signer)
		if err != nil {
			return out, fmt.Errorf("derive pool signer token account: %w", err)
		}
		exists, err := accountExists(b.GetContext(), chain, signerATA)
		if err != nil {
			return out, fmt.Errorf("fetch pool signer token account %s: %w", signerATA, err)
		}
		if !exists {
			if err = ensureAssociatedTokenAccount(b, chain, input.TokenProgram, input.Mint, signer); err != nil {
				return out, fmt.Errorf("create pool signer token account: %w", err)
			}
			out.Created = append(out.Created, signerATA)
		}

		fees, err := feeTokens(b.GetContext(), chain, input.LinkMint)
		if err != nil {
			return out, err
		}
		if !slices.ContainsFunc(fees, func(ft feeToken) bool { return ft.mint.Equals(input.Mint) }) {
			fees = append(fees, feeToken{mint: input.Mint, program: input.TokenProgram})
		}
		created, err := ensureFeeVaults(b.GetContext(), chain, input.Pool, input.Mint.Bytes(), fees)
		out.Created = append(out.Created, created...)
		if err != nil {
			return out, fmt.Errorf("create fee vaults of pool %s for mint %s: %w", input.Pool, input.Mint, err)
		}
		return out, nil
	},
)

// ensurePoolTokenAccounts runs EnsurePoolTokenAccounts for a pool instance, taking the LINK mint from refs.
func ensurePoolTokenAccounts(b cldf_ops.Bundle, chain cldf_solana.Chain, refs []datastore.AddressRef, pool, mint, tokenProgram solana.PublicKey) error {
	linkMint, err := findLinkMint(refs, chain.Selector)
	if err != nil {
		return err
	}
	_, err = cldf_ops.ExecuteOperation(b, EnsurePoolTokenAccounts, chain, EnsurePoolTokenAccountsInput{
		Pool:         pool,
		Mint:         mint,
		TokenProgram: tokenProgram,
		LinkMint:     linkMint,
	})
	return err
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
	_, err = ensureFeeVaults(ctx, chain, issuer, nil, fees)
	return err
}
