package utils

import (
	"fmt"

	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_common"
	ccipsolutils "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
)

// BuildV2ReceiverExecutorArgs derives the execution accounts required to invoke
// the Solana v2 CCIP receiver for a source chain and sender.
func BuildV2ReceiverExecutorArgs(sourceChainSelector uint64, sourceSender []byte, receiverProgramID solana.PublicKey) (ccipsolutils.SVMExecutorArgs, error) {
	if len(sourceSender) > solana.PublicKeyLength {
		return ccipsolutils.SVMExecutorArgs{}, fmt.Errorf("source sender is %d bytes, exceeds %d-byte receiver seed", len(sourceSender), solana.PublicKeyLength)
	}

	paddedSender := make([]byte, solana.PublicKeyLength)
	copy(paddedSender[solana.PublicKeyLength-len(sourceSender):], sourceSender)

	approvedSenderPDA, err := state.FindApprovedSenderV2(sourceChainSelector, paddedSender, receiverProgramID)
	if err != nil {
		return ccipsolutils.SVMExecutorArgs{}, fmt.Errorf("derive approved sender PDA: %w", err)
	}
	receiverConfigPDA, _, err := state.FindConfigReceiverPDA(receiverProgramID)
	if err != nil {
		return ccipsolutils.SVMExecutorArgs{}, fmt.Errorf("derive receiver config PDA: %w", err)
	}
	receiverEventAuthorityPDA, _, err := state.FindEventAuthorityPDA(receiverProgramID)
	if err != nil {
		return ccipsolutils.SVMExecutorArgs{}, fmt.Errorf("derive receiver event authority PDA: %w", err)
	}

	return ccipsolutils.SVMExecutorArgs{
		SvmDestBlob: ccip_common.SvmDestBlob{
			UseAta:                  0,
			AccountIsWritableBitmap: 0,
			Accounts: []solana.PublicKey{
				approvedSenderPDA,
				receiverConfigPDA,
				receiverEventAuthorityPDA,
				receiverProgramID,
			},
		},
	}, nil
}
