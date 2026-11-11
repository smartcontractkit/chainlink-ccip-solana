package cctpverifier

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"

	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/cctp_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/allowlist"
)

// readAllowlist returns the sender allowlist stored for destChainSelector.
func readAllowlist(ctx context.Context, client *rpc.Client, program solana.PublicKey, destChainSelector uint64) (allowlist.Current, error) {
	pda, _, err := state.FindCctpVerifierRemoteChainConfigPDA(destChainSelector, program)
	if err != nil {
		return allowlist.Current{}, fmt.Errorf("derive verifier remote chain config PDA: %w", err)
	}
	cfg, err := common.GetInitializedAccount(ctx, client, pda, program, cldf_solana.SolDefaultCommitment, cctp_token_pool.ParseAccount_RemoteChainConfig)
	if err != nil || cfg == nil {
		return allowlist.Current{}, err
	}
	return allowlist.Current{Initialized: true, Enabled: cfg.AllowlistConfig.AllowlistEnabled, Senders: cfg.AllowlistConfig.AllowedSenders}, nil
}
