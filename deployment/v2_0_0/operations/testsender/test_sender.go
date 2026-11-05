package testsender

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_deployment "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/example_ccip_sender"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/example_ccip_sender_v2"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

var ContractType cldf_deployment.ContractType = "TestSender"
var Version *semver.Version = solcommon.Version2_0_0
var ProgramName = "example_ccip_sender_v2"

// Legacy (v1) sender identity. Same contract type, distinguished by version, so both senders
// can be registered for the same chain and qualifier.
var LegacyVersion *semver.Version = semver.MustParse("1.0.0")
var LegacyProgramName = "example_ccip_sender"

// PDA seeds. Both sender versions share them.
var (
	stateSeed       = []byte("state")
	chainConfigSeed = []byte("remote_chain_config")
	ccipSenderSeed  = []byte("ccip_sender")
)

// FindStatePDA derives the sender's singleton state PDA.
func FindStatePDA(sender solana.PublicKey) (solana.PublicKey, error) {
	pda, _, err := solana.FindProgramAddress([][]byte{stateSeed}, sender)
	return pda, err
}

// FindChainConfigPDA derives the sender's per-destination-chain config PDA.
func FindChainConfigPDA(sender solana.PublicKey, destChainSelector uint64) (solana.PublicKey, error) {
	pda, _, err := solana.FindProgramAddress([][]byte{chainConfigSeed, common.Uint64ToLE(destChainSelector)}, sender)
	return pda, err
}

// FindCcipSenderPDA derives the PDA that signs the CPI into the router and custodies the tokens
// and lamports a send pays with.
func FindCcipSenderPDA(sender solana.PublicKey) (solana.PublicKey, error) {
	pda, _, err := solana.FindProgramAddress([][]byte{ccipSenderSeed}, sender)
	return pda, err
}

type DeployParams struct {
	ExistingAddresses []datastore.AddressRef
	Qualifier         string
}

var Deploy = operations.NewOperation(
	"sender:deploy",
	Version,
	"Deploys the Sender program",
	func(b operations.Bundle, chain cldf_solana.Chain, input DeployParams) (datastore.AddressRef, error) {
		return solcommon.DeployContractIfNotInDatastore(
			b,
			chain,
			input.ExistingAddresses,
			ContractType,
			Version,
			input.Qualifier,
			ProgramName)
	},
)

var DeployLegacy = operations.NewOperation(
	"sender:deploy-legacy",
	LegacyVersion,
	"Deploys the legacy (v1) Sender program",
	func(b operations.Bundle, chain cldf_solana.Chain, input DeployParams) (datastore.AddressRef, error) {
		return solcommon.DeployContractIfNotInDatastore(
			b,
			chain,
			input.ExistingAddresses,
			ContractType,
			LegacyVersion,
			input.Qualifier,
			LegacyProgramName)
	},
)

var Upgrade = operations.NewOperation(
	"sender:upgrade",
	Version,
	"Upgrades the default Sender program",
	func(b operations.Bundle, chain cldf_solana.Chain, programID solana.PublicKey) ([]solana.Instruction, error) {
		return solcommon.UpgradeProgram(
			b,
			chain,
			ContractType,
			programID,
			ProgramName,
		)
	},
)

var UpgradeLegacy = operations.NewOperation(
	"sender:upgrade-legacy",
	LegacyVersion,
	"Upgrades the legacy (v1) Sender program",
	func(b operations.Bundle, chain cldf_solana.Chain, programID solana.PublicKey) ([]solana.Instruction, error) {
		return solcommon.UpgradeProgram(
			b,
			chain,
			ContractType,
			programID,
			LegacyProgramName,
		)
	},
)

type Params struct {
	Router solana.PublicKey
	Sender solana.PublicKey
	// SenderVersion selects the interface to call, and matches the version the sender is
	// registered under in the datastore. It cannot be inferred from Sender, because a deployed v1
	// sender does not sit at the gobinding's declare_id! address outside devenv. Nil means the
	// current version.
	SenderVersion *semver.Version
}

// IsLegacy reports whether a sender version uses the v1 interface. Nil means the current version.
// Only the major component is compared, so patch releases of either line resolve correctly.
func IsLegacy(senderVersion *semver.Version) (bool, error) {
	if senderVersion == nil {
		return false, nil
	}
	switch senderVersion.Major() {
	case LegacyVersion.Major():
		return true, nil
	case Version.Major():
		return false, nil
	default:
		return false, fmt.Errorf("unsupported sender version %s, expected %s or %s", senderVersion, LegacyVersion, Version)
	}
}

func baseStateDiscriminator(legacy bool) [8]byte {
	if legacy {
		return example_ccip_sender.Account_BaseState
	}
	return example_ccip_sender_v2.Account_BaseState
}

func remoteChainConfigDiscriminator(legacy bool) [8]byte {
	if legacy {
		return example_ccip_sender.Account_RemoteChainConfig
	}
	return example_ccip_sender_v2.Account_RemoteChainConfig
}

// Initialize creates the sender's state PDA, pointing it at Router and making the program's upgrade
// authority its owner. Both versions take the router as their only argument; v2 additionally stores the bump of
// the ccip_sender PDA. Skipped when the state PDA already exists, so the sequence can be re-run.
var Initialize = operations.NewOperation(
	"sender:initialize",
	Version,
	"Initializes the Sender contract (v2, or v1 when SenderVersion is legacy)",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (sequences.OnChainOutput, error) {
		legacy, err := IsLegacy(input.SenderVersion)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		statePDA, err := FindStatePDA(input.Sender)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive sender state PDA: %w", err)
		}
		initialized, err := common.IsAccountInitialized(b.GetContext(), chain.Client, statePDA, input.Sender, cldf_solana.SolDefaultCommitment, baseStateDiscriminator(legacy))
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		if initialized {
			b.Logger.Infof("Sender %s already initialized (state %s), skipping", input.Sender, statePDA)
			return sequences.OnChainOutput{}, nil
		}

		// Only the program's upgrade authority may initialize it.
		programData, err := solcommon.GetSolProgramData(chain.Client, input.Sender)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to get program data: %w", err)
		}
		authority, err := solcommon.InitializerAuthority(chain, input.Sender)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}

		var initIx solana.Instruction
		if legacy {
			solcommon.SetProgramID(&example_ccip_sender.ProgramID, example_ccip_sender.ParseInstructionWithoutAccounts, input.Sender)
			initIx, err = example_ccip_sender.NewInitializeInstruction(
				input.Router,
				statePDA,
				authority,
				solana.SystemProgramID,
				input.Sender,
				programData.Address,
			)
		} else {
			solcommon.SetProgramID(&example_ccip_sender_v2.ProgramID, example_ccip_sender_v2.ParseInstructionWithoutAccounts, input.Sender)
			ccipSenderPDA, pdaErr := FindCcipSenderPDA(input.Sender)
			if pdaErr != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("derive ccip_sender PDA: %w", pdaErr)
			}
			initIx, err = example_ccip_sender_v2.NewInitializeInstruction(
				input.Router,
				statePDA,
				ccipSenderPDA,
				authority,
				solana.SystemProgramID,
				input.Sender,
				programData.Address,
			)
		}
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to build initialize instruction: %w", err)
		}
		return solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{initIx}, input.Sender, ContractType.String())
	},
)

type SetChainConfigParams struct {
	Sender            solana.PublicKey
	DestChainSelector uint64
	// Recipient is the hex address messages are sent to on the destination chain, with or
	// without the 0x prefix. It is left-padded to 32 bytes, the form both EVM (ABI-encoded) and
	// SVM receivers are addressed with. Kept as a string so the report shows the address.
	Recipient string
	// ExtraArgs is the tag-prefixed extra args blob, hex-encoded, that the sender attaches to
	// every message for this chain. For the v2 sender this is a GenericExtraArgsV3 blob.
	ExtraArgs string
	// SenderVersion selects the interface to call. See Params.SenderVersion.
	SenderVersion *semver.Version
}

// SetChainConfig enables a destination chain on the sender, storing the recipient and extra args
// it attaches to every outgoing message for that chain. It calls init_chain_config when the chain
// config PDA does not exist yet and update_chain_config otherwise, so it can be re-run to change
// either value. The state owner signs: when that is not the deployer the instruction is returned
// as an MCMS batch operation.
var SetChainConfig = operations.NewOperation(
	"sender:set-chain-config",
	Version,
	"Creates or updates a destination chain config on the Sender contract (v2, or v1 when SenderVersion is legacy)",
	func(b operations.Bundle, chain cldf_solana.Chain, input SetChainConfigParams) (sequences.OnChainOutput, error) {
		legacy, err := IsLegacy(input.SenderVersion)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		recipient, err := solcommon.DecodeRemoteSender(input.Recipient)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("decode recipient: %w", err)
		}
		extraArgs, err := hex.DecodeString(strings.TrimPrefix(input.ExtraArgs, "0x"))
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("decode extra args %q: %w", input.ExtraArgs, err)
		}
		statePDA, err := FindStatePDA(input.Sender)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive sender state PDA: %w", err)
		}
		chainConfigPDA, err := FindChainConfigPDA(input.Sender, input.DestChainSelector)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive sender chain config PDA: %w", err)
		}
		isUpdate, err := common.IsAccountInitialized(b.GetContext(), chain.Client, chainConfigPDA, input.Sender, cldf_solana.SolDefaultCommitment, remoteChainConfigDiscriminator(legacy))
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		authority, err := stateOwner(b.GetContext(), chain, input.Sender, legacy)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}

		var ix solana.Instruction
		switch {
		case legacy && !isUpdate:
			solcommon.SetProgramID(&example_ccip_sender.ProgramID, example_ccip_sender.ParseInstructionWithoutAccounts, input.Sender)
			ix, err = example_ccip_sender.NewInitChainConfigInstruction(
				input.DestChainSelector, recipient, extraArgs,
				statePDA, chainConfigPDA, authority, solana.SystemProgramID)
		case legacy && isUpdate:
			solcommon.SetProgramID(&example_ccip_sender.ProgramID, example_ccip_sender.ParseInstructionWithoutAccounts, input.Sender)
			ix, err = example_ccip_sender.NewUpdateChainConfigInstruction(
				input.DestChainSelector, recipient, extraArgs,
				statePDA, chainConfigPDA, authority, solana.SystemProgramID)
		case !isUpdate:
			solcommon.SetProgramID(&example_ccip_sender_v2.ProgramID, example_ccip_sender_v2.ParseInstructionWithoutAccounts, input.Sender)
			ix, err = example_ccip_sender_v2.NewInitChainConfigInstruction(
				example_ccip_sender_v2.ChainConfigParams{
					ChainSelector:  input.DestChainSelector,
					Recipient:      recipient,
					ExtraArgsBytes: extraArgs,
				},
				statePDA, chainConfigPDA, authority, solana.SystemProgramID)
		default:
			solcommon.SetProgramID(&example_ccip_sender_v2.ProgramID, example_ccip_sender_v2.ParseInstructionWithoutAccounts, input.Sender)
			ix, err = example_ccip_sender_v2.NewUpdateChainConfigInstruction(
				example_ccip_sender_v2.ChainConfigParams{
					ChainSelector:  input.DestChainSelector,
					Recipient:      recipient,
					ExtraArgsBytes: extraArgs,
				},
				statePDA, chainConfigPDA, authority, solana.SystemProgramID)
		}
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build chain config instruction for chain %d: %w", input.DestChainSelector, err)
		}
		out, err := solcommon.ConfirmOrBatch(chain, authority, []solana.Instruction{ix}, input.Sender, ContractType.String())
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("chain config instruction for chain %d: %w", input.DestChainSelector, err)
		}
		return out, nil
	},
)

// IsInitialized reports whether the sender's state account exists.
func IsInitialized(ctx context.Context, chain cldf_solana.Chain, sender solana.PublicKey, version *semver.Version) (bool, error) {
	legacy, err := IsLegacy(version)
	if err != nil {
		return false, err
	}
	statePDA, err := FindStatePDA(sender)
	if err != nil {
		return false, fmt.Errorf("derive sender state PDA: %w", err)
	}
	return common.IsAccountInitialized(ctx, chain.Client, statePDA, sender, cldf_solana.SolDefaultCommitment, baseStateDiscriminator(legacy))
}

// stateOwner returns the owner of the sender's state, the only key that may change its config. It
// is the deployer until the state exists.
func stateOwner(ctx context.Context, chain cldf_solana.Chain, sender solana.PublicKey, legacy bool) (solana.PublicKey, error) {
	statePDA, err := FindStatePDA(sender)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("derive sender state PDA: %w", err)
	}
	if legacy {
		return solcommon.ConfigAuthority(ctx, chain, sender, statePDA, example_ccip_sender.ParseAccount_BaseState,
			func(s *example_ccip_sender.BaseState) solana.PublicKey { return s.Owner })
	}
	return solcommon.ConfigAuthority(ctx, chain, sender, statePDA, example_ccip_sender_v2.ParseAccount_BaseState,
		func(s *example_ccip_sender_v2.BaseState) solana.PublicKey { return s.Owner })
}

type InitCcipSenderATAParams struct {
	Sender solana.PublicKey
	Mint   solana.PublicKey
	// SenderVersion selects the interface to call. See Params.SenderVersion.
	SenderVersion *semver.Version
}

// TokenProgramForMint returns the token program that owns mint, and fails unless mint is an
// existing account owned by the SPL Token or Token-2022 program. Sequences call it before any
// on-chain side effects, so a bad mint fails before the sender is deployed.
func TokenProgramForMint(ctx context.Context, chain cldf_solana.Chain, mint solana.PublicKey) (solana.PublicKey, error) {
	info, err := chain.Client.GetAccountInfoWithOpts(ctx, mint, &rpc.GetAccountInfoOpts{
		Commitment: cldf_solana.SolDefaultCommitment,
	})
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("fetch mint %s: %w", mint, err)
	}
	owner := info.Value.Owner
	if !owner.Equals(solana.TokenProgramID) && !owner.Equals(solana.Token2022ProgramID) {
		return solana.PublicKey{}, fmt.Errorf("mint %s is owned by %s, not a token program", mint, owner)
	}
	return owner, nil
}

// InitCcipSenderATA creates the sender's ccip_sender PDA token account for a mint. The router
// validates that account (as the message sender's and as the fee payer's) before doing any work,
// so it must exist for every fee token (wrapped SOL for native fees included) and transferred token
// before the first send that uses it. The token program is read from the mint's owner.
//
// v2 has its own idempotent init_ccip_sender_ata instruction. The legacy sender has none, and its
// ccip_send only checks the account, so for it the account is created directly through the
// associated token program, after checking it does not exist yet.
var InitCcipSenderATA = operations.NewOperation(
	"sender:init-ccip-sender-ata",
	Version,
	"Creates the Sender ccip_sender PDA token account for a mint (v2, or v1 when SenderVersion is legacy)",
	func(b operations.Bundle, chain cldf_solana.Chain, input InitCcipSenderATAParams) (sequences.OnChainOutput, error) {
		legacy, err := IsLegacy(input.SenderVersion)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		tokenProgram, err := TokenProgramForMint(b.GetContext(), chain, input.Mint)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}
		ccipSenderPDA, err := FindCcipSenderPDA(input.Sender)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive ccip_sender PDA: %w", err)
		}
		ata, _, err := tokens.FindAssociatedTokenAddress(tokenProgram, input.Mint, ccipSenderPDA)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive ccip_sender ATA for mint %s: %w", input.Mint, err)
		}

		var ix solana.Instruction
		if legacy {
			_, getErr := chain.Client.GetAccountInfoWithOpts(b.GetContext(), ata, &rpc.GetAccountInfoOpts{
				Commitment: cldf_solana.SolDefaultCommitment,
			})
			switch {
			case getErr == nil:
				b.Logger.Infof("Legacy sender ccip_sender ATA %s for mint %s already exists, skipping", ata, input.Mint)
				return sequences.OnChainOutput{}, nil
			case !errors.Is(getErr, rpc.ErrNotFound):
				return sequences.OnChainOutput{}, fmt.Errorf("fetch ccip_sender ATA %s: %w", ata, getErr)
			}
			ix, _, err = tokens.CreateAssociatedTokenAccount(tokenProgram, input.Mint, ccipSenderPDA, chain.DeployerKey.PublicKey())
		} else {
			solcommon.SetProgramID(&example_ccip_sender_v2.ProgramID, example_ccip_sender_v2.ParseInstructionWithoutAccounts, input.Sender)
			statePDA, pdaErr := FindStatePDA(input.Sender)
			if pdaErr != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("derive sender state PDA: %w", pdaErr)
			}
			ix, err = example_ccip_sender_v2.NewInitCcipSenderAtaInstruction(
				statePDA,
				ata,
				input.Mint,
				ccipSenderPDA,
				chain.DeployerKey.PublicKey(),
				tokenProgram,
				solana.SPLAssociatedTokenAccountProgramID,
				solana.SystemProgramID,
			)
		}
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build init ccip_sender ATA instruction: %w", err)
		}
		if err := chain.Confirm([]solana.Instruction{ix}); err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("confirm init ccip_sender ATA instruction for mint %s: %w", input.Mint, err)
		}
		return sequences.OnChainOutput{}, nil
	},
)

type FundCcipSenderParams struct {
	Sender solana.PublicKey
	// Lamports is the balance to top the ccip_sender PDA up to. Nothing is sent when the PDA
	// already holds at least this much.
	Lamports uint64
}

// FundCcipSender tops the sender's ccip_sender PDA up to a lamport balance, paid by the deployer.
// The v1 sender's PDA fronts the router's native SOL fee and nonce rent, which the caller pays back.
var FundCcipSender = operations.NewOperation(
	"sender:fund-ccip-sender",
	Version,
	"Tops up the Sender ccip_sender PDA, which pays native SOL fees",
	func(b operations.Bundle, chain cldf_solana.Chain, input FundCcipSenderParams) (sequences.OnChainOutput, error) {
		ccipSenderPDA, err := FindCcipSenderPDA(input.Sender)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("derive ccip_sender PDA: %w", err)
		}
		balance, err := chain.Client.GetBalance(b.GetContext(), ccipSenderPDA, cldf_solana.SolDefaultCommitment)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("get ccip_sender PDA %s balance: %w", ccipSenderPDA, err)
		}
		if balance.Value >= input.Lamports {
			return sequences.OnChainOutput{}, nil
		}
		ix, err := system.NewTransferInstruction(input.Lamports-balance.Value, chain.DeployerKey.PublicKey(), ccipSenderPDA).ValidateAndBuild()
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("build ccip_sender PDA funding instruction: %w", err)
		}
		if err := chain.Confirm([]solana.Instruction{ix}); err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("fund ccip_sender PDA %s: %w", ccipSenderPDA, err)
		}
		return sequences.OnChainOutput{}, nil
	},
)
