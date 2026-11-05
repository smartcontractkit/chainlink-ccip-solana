package tokens

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/Masterminds/semver/v3"
	token_metadata "github.com/gagliardetto/metaplex-go/clients/token-metadata"
	"github.com/gagliardetto/solana-go"
	addresslookuptable "github.com/gagliardetto/solana-go/programs/address-lookup-table"
	"github.com/gagliardetto/solana-go/rpc"

	solCommonUtil "github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/common"
	tokenapi "github.com/smartcontractkit/chainlink-ccip/deployment/tokens"
	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_deployment "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	soltokens "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
)

var (
	LinkContractType cldf_deployment.ContractType = "LinkToken"
	// TokenMultisigType is the mint-authority multisig created for a token pool.
	TokenMultisigType cldf_deployment.ContractType = "TokenMultisig"
	// This is not the LinkToken Version, but the version of this deployment package.
	Version *semver.Version = solcommon.Version2_0_0
)

type Params struct {
	ExistingAddresses      []datastore.AddressRef
	TokenProgramName       cldf_deployment.ContractType
	TokenPrivKey           solana.PrivateKey
	TokenDecimals          uint8
	TokenSymbol            string
	ATAList                []solana.PublicKey
	PreMint                uint64
	DisableFreezeAuthority bool
}

var DeployLINK = operations.NewOperation(
	"link:deploy",
	Version,
	"Deploys the LINK token contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (datastore.AddressRef, error) {
		instructions, err := soltokens.CreateToken(
			b.GetContext(),
			solana.TokenProgramID,
			input.TokenPrivKey.PublicKey(),
			chain.DeployerKey.PublicKey(),
			input.TokenDecimals,
			chain.Client,
			cldf_solana.SolDefaultCommitment,
		)
		if err != nil {
			return datastore.AddressRef{}, err
		}
		err = chain.Confirm(instructions, solCommonUtil.AddSigners(input.TokenPrivKey))
		if err != nil {
			return datastore.AddressRef{}, err
		}
		return datastore.AddressRef{
			ChainSelector: chain.Selector,
			Address:       input.TokenPrivKey.PublicKey().String(),
			Version:       Version,
			Type:          datastore.ContractType(LinkContractType),
		}, nil
	},
)

var DeploySolanaToken = operations.NewOperation(
	"solana-token:deploy",
	Version,
	"Deploys and configures an SPL token contract",
	func(b operations.Bundle, chain cldf_solana.Chain, input Params) (datastore.AddressRef, error) {
		// CREATE TOKEN
		tokenProgramID, err := solcommon.GetTokenProgramID(input.TokenProgramName)
		if err != nil {
			return datastore.AddressRef{}, err
		}
		tokenAdminPubKey := chain.DeployerKey.PublicKey()
		// if we're disabling the freeze authority, we first set it to the deployer key so it can
		// immediately revoke it - and the timelock is not looked up at all, so a chain without
		// MCMS (the devenv) can deploy a token by opting out of a freeze authority explicitly.
		freezeAuthority := chain.DeployerKey.PublicKey()
		if !input.DisableFreezeAuthority {
			freezeAuthority, err = solcommon.GetTimelockSignerPDA(input.ExistingAddresses, chain.Selector, common_utils.CLLQualifier)
			if err != nil {
				return datastore.AddressRef{}, err
			}
		}
		var mint solana.PublicKey
		privKey := input.TokenPrivKey
		if privKey.IsValid() {
			mint = privKey.PublicKey()
		} else {
			privKey, err = solana.NewRandomPrivateKey()
			if err != nil {
				return datastore.AddressRef{}, err
			}
			mint = privKey.PublicKey()
		}
		instructions, err := soltokens.CreateTokenWith(
			b.GetContext(),
			tokenProgramID,
			mint,
			tokenAdminPubKey,
			freezeAuthority,
			input.TokenDecimals,
			chain.Client,
			cldf_solana.SolDefaultCommitment,
			false,
		)
		if err != nil {
			return datastore.AddressRef{}, err
		}
		err = chain.Confirm(instructions, solCommonUtil.AddSigners(privKey))
		if err != nil {
			return datastore.AddressRef{}, err
		}
		// CREATE ATAs
		for _, sender := range input.ATAList {
			var createATAIx solana.Instruction
			createATAIx, _, err = soltokens.CreateAssociatedTokenAccount(
				tokenProgramID,
				mint,
				sender,
				chain.DeployerKey.PublicKey(),
			)
			if err != nil {
				return datastore.AddressRef{}, err
			}
			if err1 := chain.Confirm([]solana.Instruction{createATAIx}); err1 != nil {
				return datastore.AddressRef{}, err1
			}
			if input.PreMint > 0 {
				var ata solana.PublicKey
				ata, _, err = soltokens.FindAssociatedTokenAddress(tokenProgramID, mint, sender)
				if err != nil {
					return datastore.AddressRef{}, fmt.Errorf("failed to derive associated token address: %w", err)
				}
				mintToI, err1 := soltokens.MintTo(input.PreMint, tokenProgramID, mint, ata, chain.DeployerKey.PublicKey())
				if err1 != nil {
					return datastore.AddressRef{}, err1
				}
				if err2 := chain.Confirm([]solana.Instruction{mintToI}); err2 != nil {
					return datastore.AddressRef{}, err2
				}
			}
		}
		// DISABLE FREEZE AUTHORITY
		if input.DisableFreezeAuthority {
			err = solcommon.DisableFreezeAuthority(b.GetContext(), chain, []solana.PublicKey{mint})
			if err != nil {
				return datastore.AddressRef{}, err
			}
		}
		return datastore.AddressRef{
			ChainSelector: chain.Selector,
			Address:       mint.String(),
			Type:          datastore.ContractType(input.TokenProgramName),
			Version:       Version,
			Qualifier:     input.TokenSymbol,
		}, nil
	},
)

type TokenMultisigParams struct {
	Signers      []solana.PublicKey
	TokenProgram solana.PublicKey
	TokenMint    solana.PublicKey
	TokenSymbol  string
}

const (
	// MaxMultisigSigners is the SPL token multisig signer limit.
	MaxMultisigSigners = 11
	// MultisigThreshold is the number of signatures (m) required by the multisigs that
	// CreateTokenMultisig creates.
	MultisigThreshold uint8 = 1
)

var CreateTokenMultisig = operations.NewOperation(
	"solana-token:create-multisig",
	Version,
	"Creates a Token Multisig account for the given token program",
	func(b operations.Bundle, chain cldf_solana.Chain, input TokenMultisigParams) (sequences.OnChainOutput, error) {
		ctx := b.GetContext()

		m := MultisigThreshold

		// --- Validate inputs ---
		if input.TokenProgram.IsZero() {
			return sequences.OnChainOutput{}, errors.New("token program is zero")
		}
		if !input.TokenProgram.Equals(solana.Token2022ProgramID) && !input.TokenProgram.Equals(solana.TokenProgramID) {
			return sequences.OnChainOutput{}, fmt.Errorf("unsupported token program: %s", input.TokenProgram.String())
		}
		if len(input.Signers) < 2 {
			return sequences.OnChainOutput{}, fmt.Errorf("signers length must be at least 2, got %d", len(input.Signers))
		}
		if len(input.Signers) > 11 {
			return sequences.OnChainOutput{}, fmt.Errorf("too many signers: %d > %d", len(input.Signers), 11)
		}

		// --- Create multisig keypair (must sign CreateAccount) ---
		multisig, err := solana.NewRandomPrivateKey()
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to generate multisig keypair: %w", err)
		}

		// --- Instructions ---
		// get stake amount for init
		lamports, err := chain.Client.GetMinimumBalanceForRentExemption(ctx, soltokens.MultisigSize, rpc.CommitmentConfirmed)
		if err != nil {
			return sequences.OnChainOutput{}, err
		}

		ixs, err := soltokens.IxsInitTokenMultisig(input.TokenProgram, lamports, chain.DeployerKey.PublicKey(), multisig.PublicKey(), m, input.Signers)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to create multisig instructions: %w", err)
		}
		// The new multisig account must sign its own CreateAccount.
		err = chain.Confirm(ixs, solCommonUtil.AddSigners(multisig))
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to confirm create multisig transaction: %w", err)
		}
		return sequences.OnChainOutput{
			Addresses: []datastore.AddressRef{
				{
					ChainSelector: chain.Selector,
					Address:       multisig.PublicKey().String(),
					Type:          datastore.ContractType(TokenMultisigType),
					Version:       Version,
					Qualifier:     input.TokenSymbol,
					Labels:        datastore.NewLabelSet(input.TokenMint.String()),
				},
			},
		}, nil
	})

type ExtendTokenPoolLookupTableParams struct {
	TokenMint solana.PublicKey
	Router    solana.PublicKey
	// Accounts to add; ones already in the table, and repeats, are skipped.
	Accounts []solana.PublicKey
}

// ExtendTokenPoolLookupTable adds accounts to the lookup table registered in the token's
// TokenAdminRegistry. The table must be active and owned by the deployer key.
var ExtendTokenPoolLookupTable = operations.NewOperation(
	"solana-token:extend-token-pool-lookup-table",
	Version,
	"Extends the token pool address lookup table with additional accounts",
	func(b operations.Bundle, chain cldf_solana.Chain, input ExtendTokenPoolLookupTableParams) (sequences.OnChainOutput, error) {
		ctx := b.GetContext()
		if input.TokenMint.IsZero() {
			return sequences.OnChainOutput{}, errors.New("token mint is zero")
		}
		if input.Router.IsZero() {
			return sequences.OnChainOutput{}, errors.New("router is zero")
		}
		if len(input.Accounts) == 0 {
			b.Logger.Info("no accounts provided - skipping token pool lookup table extension")
			return sequences.OnChainOutput{}, nil
		}
		if chain.DeployerKey == nil {
			return sequences.OnChainOutput{}, fmt.Errorf("solana deployer key is nil for chain selector %d", chain.Selector)
		}

		tarPDA, _, err := state.FindTokenAdminRegistryPDA(input.TokenMint, input.Router)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to find token admin registry PDA: %w", err)
		}
		tar, _, err := routerops.ReadTokenAdminRegistry(ctx, chain.Client, tarPDA)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to read token admin registry %s: %w", tarPDA, err)
		}
		lookupTable := tar.LookupTable
		if lookupTable.IsZero() {
			return sequences.OnChainOutput{}, fmt.Errorf("token pool lookup table is not set for token %s on router %s; set the pool first", input.TokenMint, input.Router)
		}

		lut, err := addresslookuptable.GetAddressLookupTableStateWithOpts(ctx, chain.Client, lookupTable, &rpc.GetAccountInfoOpts{Commitment: cldf_solana.SolDefaultCommitment})
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to read lookup table %s: %w", lookupTable, err)
		}
		if !lut.IsActive() {
			return sequences.OnChainOutput{}, fmt.Errorf("token pool lookup table %s is not active", lookupTable)
		}
		if lut.Authority == nil || !lut.Authority.Equals(chain.DeployerKey.PublicKey()) {
			return sequences.OnChainOutput{}, fmt.Errorf("token pool lookup table %s is not owned by the deployer key %s", lookupTable, chain.DeployerKey.PublicKey())
		}

		seen := make(map[solana.PublicKey]bool, len(lut.Addresses)+len(input.Accounts))
		for _, entry := range lut.Addresses {
			seen[entry] = true
		}
		toAdd := make([]solana.PublicKey, 0, len(input.Accounts))
		for i, acct := range input.Accounts {
			if acct.IsZero() {
				return sequences.OnChainOutput{}, fmt.Errorf("account at index %d is zero", i)
			}
			if !seen[acct] {
				toAdd = append(toAdd, acct)
				seen[acct] = true
			}
		}
		if len(toAdd) == 0 {
			b.Logger.Infof("all %d account(s) already in token pool lookup table %s", len(input.Accounts), lookupTable)
			return sequences.OnChainOutput{}, nil
		}

		if err = common.ExtendLookupTable(ctx, chain.Client, lookupTable, *chain.DeployerKey, toAdd); err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to extend token pool lookup table %s: %w", lookupTable, err)
		}
		if err = common.AwaitSlotChange(ctx, chain.Client); err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to await slot change after extending lookup table %s: %w", lookupTable, err)
		}
		return sequences.OnChainOutput{}, nil
	})

var UpsertTokenMetadata = operations.NewOperation(
	"solana-token:upsert-metadata",
	Version,
	"Upserts metadata for an SPL token",
	func(b operations.Bundle, chain cldf_solana.Chain, input TokenMetadataInput) (sequences.OnChainOutput, error) {
		ctx := b.GetContext()

		if input.Metadata.TokenPubkey == "" {
			return sequences.OnChainOutput{}, errors.New("TokenPubkey is required")
		}
		mint, err := solana.PublicKeyFromBase58(input.Metadata.TokenPubkey)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("invalid TokenPubkey: %w", err)
		}

		// Determine the update authority: default to deployer, override if specified.
		updateAuthority := chain.DeployerKey.PublicKey()
		if input.Metadata.UpdateAuthority != "" {
			updateAuthority, err = solana.PublicKeyFromBase58(input.Metadata.UpdateAuthority)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("invalid UpdateAuthority: %w", err)
			}
		}

		metadataPDA, err := soltokens.FindMplTokenMetadataPDA(mint)
		if err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to find metadata PDA: %w", err)
		}

		var ixn solana.Instruction
		existingMetadata, _, getErr := soltokens.GetTokenMetadata(ctx, chain.Client, mint)
		if getErr != nil {
			// Metadata account does not yet exist — create it.
			if input.Metadata.MetadataJSONPath == "" {
				return sequences.OnChainOutput{}, errors.New("MetadataJSONPath is required for initial metadata creation")
			}
			data, readErr := readMetadataJSON(input.Metadata.MetadataJSONPath)
			if readErr != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to read metadata JSON: %w", readErr)
			}
			createIxn := token_metadata.NewCreateMetadataAccountV2Instruction(
				token_metadata.CreateMetadataAccountArgsV2{
					Data: token_metadata.DataV2{
						Name:   data.Name,
						Symbol: data.Symbol,
						Uri:    data.URI,
					},
					IsMutable: true,
				},
				metadataPDA,
				mint,
				chain.DeployerKey.PublicKey(), // mint authority must sign
				chain.DeployerKey.PublicKey(), // payer
				updateAuthority,
				solana.SystemProgramID,
				solana.SysVarRentPubkey,
			)
			ixn, err = createIxn.ValidateAndBuild()
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to build create metadata instruction: %w", err)
			}
		} else {
			// Metadata account already exists — update only the non-empty fields.
			currentData := soltokens.GetTokenDataV2(existingMetadata)
			if input.Metadata.UpdateName != "" {
				currentData.Name = input.Metadata.UpdateName
			}
			if input.Metadata.UpdateSymbol != "" {
				currentData.Symbol = input.Metadata.UpdateSymbol
			}
			if input.Metadata.UpdateURI != "" {
				currentData.Uri = input.Metadata.UpdateURI
			}

			// Only change the update authority if explicitly requested.
			var newUpdateAuthority *solana.PublicKey
			if input.Metadata.UpdateAuthority != "" {
				newUpdateAuthority = &updateAuthority
			}

			updateIxn := token_metadata.NewUpdateMetadataAccountV2Instruction(
				token_metadata.UpdateMetadataAccountArgsV2{
					Data:            &currentData,
					UpdateAuthority: newUpdateAuthority,
				},
				metadataPDA,
				updateAuthority,
			)
			ixn, err = updateIxn.ValidateAndBuild()
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to build update metadata instruction: %w", err)
			}
		}

		if err = chain.Confirm([]solana.Instruction{ixn}); err != nil {
			return sequences.OnChainOutput{}, fmt.Errorf("failed to confirm metadata instruction: %w", err)
		}
		return sequences.OnChainOutput{}, nil
	},
)

// metadataJSON is the schema for the metadata JSON file referenced by TokenMetadata.MetadataJSONPath.
type metadataJSON struct {
	Name   string `json:"name"`
	Symbol string `json:"symbol"`
	URI    string `json:"uri"`
}

func readMetadataJSON(path string) (metadataJSON, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return metadataJSON{}, fmt.Errorf("open %s: %w", path, err)
	}
	var data metadataJSON
	if err = json.Unmarshal(raw, &data); err != nil {
		return metadataJSON{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if data.Name == "" || data.Symbol == "" {
		return metadataJSON{}, fmt.Errorf("metadata JSON at %s must contain non-empty name and symbol", path)
	}
	return data, nil
}

type TokenMetadataInput struct {
	ExistingAddresses []datastore.AddressRef
	Metadata          tokenapi.TokenMetadata
}

// discriminator for update_metadata_account_v2 ix
const UpdateMetadataAccountV2Ix = 15

// discriminator for create_metadata_account
const CreateMetadataAccountV2Ix = 16
