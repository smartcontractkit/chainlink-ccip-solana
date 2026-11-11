package adapters

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-ccip/deployment/fees"
	datastore_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils/datastore"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	cldf "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	solseq "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/sequences"
)

var _ fees.FeeAggregatorAdapter = (*FeeAggregatorAdapter)(nil)

type FeeAggregatorAdapter struct {
	sol *solseq.SolanaAdapter
}

func NewFeeAggregatorAdapter(solAdapter *solseq.SolanaAdapter) *FeeAggregatorAdapter {
	return &FeeAggregatorAdapter{
		sol: solAdapter,
	}
}

func (a *FeeAggregatorAdapter) GetFeeAggregator(e cldf.Environment, chainSelector uint64) (string, error) {
	chain, ok := e.BlockChains.SolanaChains()[chainSelector]
	if !ok {
		return "", fmt.Errorf("solana chain not found for selector %d", chainSelector)
	}

	routerAddr, err := a.sol.GetRouterAddress(e.DataStore, chainSelector)
	if err != nil {
		return "", fmt.Errorf("failed to get Router address for chain selector %d: %w", chainSelector, err)
	}
	routerPubkey := solana.PublicKeyFromBytes(routerAddr)

	configPDA, _, err := state.FindConfigPDA(routerPubkey)
	if err != nil {
		return "", fmt.Errorf("failed to derive config PDA for router %s on chain %d: %w", routerPubkey, chainSelector, err)
	}

	var cfg ccip_router.Config
	if err := common.GetAccountDataBorshIntoAnchor(context.TODO(), chain.Client, configPDA, cldf_solana.SolDefaultCommitment, &cfg); err != nil {
		return "", fmt.Errorf("failed to read router config on chain %d: %w", chainSelector, err)
	}

	return cfg.FeeAggregator.String(), nil
}

func (a *FeeAggregatorAdapter) resolveRouterPubkey(e cldf.Environment, input fees.FeeAggregatorForChain) (solana.PublicKey, error) {
	if len(input.Contracts) > 0 {
		if len(input.Contracts) != 1 {
			return solana.PublicKey{}, fmt.Errorf("solana 2.0 adapter supports exactly one contract ref, got %d", len(input.Contracts))
		}
		ref := input.Contracts[0]
		if string(ref.Type) != routerops.ContractType.String() {
			return solana.PublicKey{}, fmt.Errorf("solana 2.0 adapter only supports contract type %q, got %q", routerops.ContractType, ref.Type)
		}
		resolved, err := solcommon.FindAndFormatCompatibleRef(e.DataStore, ref, input.ChainSelector, datastore_utils.FullRef)
		if err != nil {
			return solana.PublicKey{}, fmt.Errorf("failed to resolve Router ref on chain %d: %w", input.ChainSelector, err)
		}
		routerPubkey, err := solana.PublicKeyFromBase58(resolved.Address)
		if err != nil {
			return solana.PublicKey{}, fmt.Errorf("failed to parse resolved Router address %q on chain %d: %w", resolved.Address, input.ChainSelector, err)
		}
		return routerPubkey, nil
	}
	routerAddr, err := a.sol.GetRouterAddress(e.DataStore, input.ChainSelector)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to get Router address for chain selector %d: %w", input.ChainSelector, err)
	}
	return solana.PublicKeyFromBytes(routerAddr), nil
}

func (a *FeeAggregatorAdapter) SetFeeAggregator(e cldf.Environment) *operations.Sequence[fees.FeeAggregatorForChain, sequences.OnChainOutput, cldf_chain.BlockChains] {
	return operations.NewSequence(
		"SetFeeAggregator",
		semver.MustParse("2.0.0"),
		"Sets the fee aggregator address on Solana Router",
		func(b operations.Bundle, chains cldf_chain.BlockChains, input fees.FeeAggregatorForChain) (sequences.OnChainOutput, error) {
			solChain, ok := chains.SolanaChains()[input.ChainSelector]
			if !ok {
				return sequences.OnChainOutput{}, fmt.Errorf("solana chain not found for selector %d", input.ChainSelector)
			}

			feeAggregatorPubkey, err := solana.PublicKeyFromBase58(input.FeeAggregator)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("invalid fee aggregator base58 address %q: %w", input.FeeAggregator, err)
			}

			routerPubkey, err := a.resolveRouterPubkey(e, input)
			if err != nil {
				return sequences.OnChainOutput{}, err
			}

			authority, err := routerops.GetAuthority(b.GetContext(), solChain, routerPubkey)
			if err != nil {
				return sequences.OnChainOutput{}, err
			}

			routerConfigPDA, _, err := state.FindConfigPDA(routerPubkey)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to derive config PDA for router %s: %w", routerPubkey, err)
			}

			ixn, err := ccip_router.NewUpdateFeeAggregatorInstruction(
				feeAggregatorPubkey,
				routerConfigPDA,
				authority,
				solana.SystemProgramID,
			)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to build UpdateFeeAggregator instruction: %w", err)
			}
			// Target this chain's router explicitly instead of rebinding the binding's
			// package-level ProgramID, which is process-global.
			ixn, err = solcommon.WithProgramID(ixn, routerPubkey)
			if err != nil {
				return sequences.OnChainOutput{}, err
			}

			return solcommon.ConfirmOrBatch(solChain, authority, []solana.Instruction{ixn}, routerPubkey, routerops.ContractType.String())
		},
	)
}

func (a *FeeAggregatorAdapter) WithdrawFeeTokens(e cldf.Environment) *operations.Sequence[fees.WithdrawFeeTokensForChain, sequences.OnChainOutput, cldf_chain.BlockChains] {
	return operations.NewSequence(
		"WithdrawFeeTokens",
		semver.MustParse("2.0.0"),
		"Withdraws accumulated fee token balances to the fee aggregator on the Solana Router",
		func(b operations.Bundle, chains cldf_chain.BlockChains, input fees.WithdrawFeeTokensForChain) (sequences.OnChainOutput, error) {
			solChain, ok := chains.SolanaChains()[input.ChainSelector]
			if !ok {
				return sequences.OnChainOutput{}, fmt.Errorf("solana chain not found for selector %d", input.ChainSelector)
			}
			if len(input.FeeTokens) == 0 {
				return sequences.OnChainOutput{}, fmt.Errorf("no fee tokens specified for chain %d", input.ChainSelector)
			}

			routerPubkey, err := a.resolveRouterPubkey(e, fees.FeeAggregatorForChain{
				ChainSelector: input.ChainSelector,
				Contracts:     input.Contracts,
			})
			if err != nil {
				return sequences.OnChainOutput{}, err
			}

			authority, err := routerops.GetAuthority(b.GetContext(), solChain, routerPubkey)
			if err != nil {
				return sequences.OnChainOutput{}, err
			}

			routerConfigPDA, _, err := state.FindConfigPDA(routerPubkey)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to derive config PDA for router %s: %w", routerPubkey, err)
			}
			billingSignerPDA, _, err := state.FindFeeBillingSignerPDA(routerPubkey)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to derive fee billing signer PDA for router %s: %w", routerPubkey, err)
			}

			// The router transfers to the fee aggregator recorded in its own config, so read it
			// rather than taking it from the input -- that keeps EVM and Solana semantics aligned.
			var cfg ccip_router.Config
			if err := common.GetAccountDataBorshIntoAnchor(e.GetContext(), solChain.Client, routerConfigPDA, cldf_solana.SolDefaultCommitment, &cfg); err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to read router config on chain %d: %w", input.ChainSelector, err)
			}
			feeAggregator := cfg.FeeAggregator
			if feeAggregator.IsZero() {
				return sequences.OnChainOutput{}, fmt.Errorf("fee aggregator is not set on router %s (chain %d); set it before withdrawing",
					routerPubkey, input.ChainSelector)
			}

			// WithdrawBilledFunds handles one mint per instruction, so build one per fee token and
			// submit them together.
			ixns := make([]solana.Instruction, 0, len(input.FeeTokens))
			for i, tok := range input.FeeTokens {
				mint, err := solana.PublicKeyFromBase58(tok.Token)
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("invalid fee token mint %q at feeTokens[%d] on chain %d: %w",
						tok.Token, i, input.ChainSelector, err)
				}

				transferAll, desiredAmount, err := withdrawAmountFor(tok)
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("invalid amount at feeTokens[%d] on chain %d: %w", i, input.ChainSelector, err)
				}

				tokenProgram, err := resolveTokenProgram(e, solChain, mint)
				if err != nil {
					return sequences.OnChainOutput{}, err
				}

				feeTokenAccum, _, err := tokens.FindAssociatedTokenAddress(tokenProgram, mint, billingSignerPDA)
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to derive billing ATA for mint %s on chain %d: %w", mint, input.ChainSelector, err)
				}
				recipient, _, err := tokens.FindAssociatedTokenAddress(tokenProgram, mint, feeAggregator)
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to derive fee aggregator ATA for mint %s on chain %d: %w", mint, input.ChainSelector, err)
				}

				// The router rejects a missing recipient account, so make sure it exists first.
				if err = ensureFeeAggregatorATA(e, solChain, tokenProgram, mint, feeAggregator, recipient); err != nil {
					return sequences.OnChainOutput{}, err
				}

				ixn, err := ccip_router.NewWithdrawBilledFundsInstruction(
					transferAll,
					desiredAmount,
					mint,
					feeTokenAccum,
					recipient,
					tokenProgram,
					billingSignerPDA,
					routerConfigPDA,
					authority,
				)
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to build WithdrawBilledFunds instruction for mint %s on chain %d: %w",
						mint, input.ChainSelector, err)
				}
				ixn, err = solcommon.WithProgramID(ixn, routerPubkey)
				if err != nil {
					return sequences.OnChainOutput{}, err
				}
				ixns = append(ixns, ixn)
			}

			return solcommon.ConfirmOrBatch(solChain, authority, ixns, routerPubkey, routerops.ContractType.String())
		},
	)
}

// resolveTokenProgram determines which SPL token program owns a mint. Fee tokens may be either
// classic SPL or Token-2022, and the withdraw instruction has to be handed the matching program.
// The account must be a mint: a token account or multisig would give a wrong ATA derivation.
func resolveTokenProgram(e cldf.Environment, chain cldf_solana.Chain, mint solana.PublicKey) (solana.PublicKey, error) {
	program, err := solcommon.FetchTokenProgramID(e.GetContext(), chain, mint)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to resolve token program for fee mint %s on chain %d: %w", mint, chain.Selector, err)
	}
	return program, nil
}

// ensureFeeAggregatorATA makes sure the fee aggregator holds a token account for the mint, since
// the router will not create the recipient itself.
//
// Creating an ATA is permissionless, so this runs immediately with the deployer key instead of
// joining the MCMS proposal: the account is in place by the time the withdrawal executes, and the
// proposal stays limited to the privileged instruction.
func ensureFeeAggregatorATA(
	e cldf.Environment,
	chain cldf_solana.Chain,
	tokenProgram, mint, feeAggregator, recipient solana.PublicKey,
) error {
	_, err := chain.Client.GetAccountInfoWithOpts(e.GetContext(), recipient, &rpc.GetAccountInfoOpts{
		Commitment: cldf_solana.SolDefaultCommitment,
	})
	switch {
	case err == nil:
		return nil
	case !errors.Is(err, rpc.ErrNotFound):
		return fmt.Errorf("failed to check fee aggregator ATA %s for mint %s on chain %d: %w",
			recipient, mint, chain.Selector, err)
	}

	createIx, created, err := tokens.CreateAssociatedTokenAccount(
		tokenProgram, mint, feeAggregator, chain.DeployerKey.PublicKey())
	if err != nil {
		return fmt.Errorf("failed to build ATA creation for fee aggregator %s and mint %s on chain %d: %w",
			feeAggregator, mint, chain.Selector, err)
	}
	// Guards against the derivation here drifting from the one used to build the instruction.
	if created != recipient {
		return fmt.Errorf("derived fee aggregator ATA %s does not match expected %s for mint %s on chain %d",
			created, recipient, mint, chain.Selector)
	}
	if err := chain.Confirm([]solana.Instruction{createIx}); err != nil {
		return fmt.Errorf("failed to create fee aggregator ATA %s for mint %s on chain %d: %w",
			recipient, mint, chain.Selector, err)
	}

	return nil
}

// withdrawAmountFor translates the family-agnostic amount into the (transferAll, desiredAmount)
// pair the router instruction expects. A nil amount withdraws the whole accumulated balance.
func withdrawAmountFor(tok fees.FeeTokenWithdrawal) (transferAll bool, desiredAmount uint64, err error) {
	if tok.Amount == nil {
		return true, 0, nil
	}
	if tok.Amount.Sign() <= 0 {
		return false, 0, fmt.Errorf("amount %s for fee token %s must be positive", tok.Amount, tok.Token)
	}
	if !tok.Amount.IsUint64() {
		return false, 0, fmt.Errorf("amount %s for fee token %s does not fit in uint64", tok.Amount, tok.Token)
	}
	return false, tok.Amount.Uint64(), nil
}
