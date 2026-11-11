package config

import (
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/lombard_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/mock_lombard_bridge"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/mock_lombard_mailbox"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
)

var (
	// program ids. Lombard's verifier lives in the same program as the pool (lombard_token_pool),
	// unlike committee_verifier which is a separate program.
	LombardTokenPoolProgram   = GetProgramID("lombard_token_pool")
	MockLombardBridgeProgram  = GetProgramID("mock_lombard_bridge")
	MockLombardMailboxProgram = GetProgramID("mock_lombard_mailbox")

	// Lombard pool PDAs
	LombardConfigPDA, _, _ = state.FindLombardPoolConfigPDA(LombardTokenPoolProgram)

	// Lombard verifier PDAs. LombardVerifierConfigPDA deliberately reuses committee_verifier's
	// generic "config" seed (the offramp's generic CCV account resolution hardcodes it for any
	// CCV's own config PDA); the pool's own global config was moved off "config" to make room,
	// see contracts/utils/state/pda.go.
	LombardVerifierConfigPDA, _, _               = state.FindLVConfigPDA(LombardTokenPoolProgram)
	LombardVerifierStorageLocationsPDA, _, _     = state.FindLVStorageLocationsPDA(LombardTokenPoolProgram)
	LombardVerifierEvmRemoteChainConfigPDA, _, _ = state.FindLVRemoteChainConfigPDA(EvmChainSelector, LombardTokenPoolProgram)
)

func init() {
	type entry struct {
		programID solana.PublicKey
		decoder   func(instructionAccounts []*solana.AccountMeta, data []byte) (any, error)
		name      string
	}
	entries := []entry{
		{LombardTokenPoolProgram, decoder(lombard_token_pool.ParseInstructionWithAccounts), "lombard_token_pool"},
		{MockLombardBridgeProgram, decoder(mock_lombard_bridge.ParseInstructionWithAccounts), "mock_lombard_bridge"},
		{MockLombardMailboxProgram, decoder(mock_lombard_mailbox.ParseInstructionWithAccounts), "mock_lombard_mailbox"},
	}
	for _, e := range entries {
		if err := solana.RegisterInstructionDecoder(e.programID, e.decoder); err != nil {
			fmt.Printf("warn: failed to register instruction decoder for %s: %s. Depending on the context this may be safe to ignore.\n", e.name, err.Error())
		}
	}
}
