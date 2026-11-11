package sequences

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"

	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	ccvadapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"
	mcms_types "github.com/smartcontractkit/mcms/types"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/arbitrary_cpi_proxy"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/burnmint_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_offramp"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/cctp_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/committee_verifier"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/example_ccip_receiver"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/example_ccip_sender"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/executor"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/fee_quoter"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/lockrelease_token_pool"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/rmn_remote"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/test_ccip_invalid_receiver"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/test_ccip_receiver"
	solcommonutils "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/utils"
	cvops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/committeeverifier"
	executorops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/executor"
	fqops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/feequoter"
	offrampops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/offramp"
	rmnremoteops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/rmnremote"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
	tokensops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokens"
)

// Program file helpers adapted from chainlink/deployment/helpers/build_solana.go

// Run a command in a specific directory.
func runCommand(ctx context.Context, command string, args []string, workDir string) (string, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = workDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return stderr.String(), err
	}
	return stdout.String(), nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	return destFile.Sync()
}

// Deployable programs, i.e. the ones that get a deploy keypair. Shared-library
// crates (ccip_common, base_token_pool) are not programs on chain, so they are
// absent here.
var ccipPrograms map[string]solana.PublicKey = map[string]solana.PublicKey{
	"arbitrary_cpi_proxy":        arbitrary_cpi_proxy.ProgramID,
	"burnmint_token_pool":        burnmint_token_pool.ProgramID,
	"ccip_offramp":               ccip_offramp.ProgramID,
	"ccip_router":                ccip_router.ProgramID,
	"cctp_token_pool":            cctp_token_pool.ProgramID,
	"committee_verifier":         committee_verifier.ProgramID,
	"example_ccip_receiver":      example_ccip_receiver.ProgramID,
	"example_ccip_sender":        example_ccip_sender.ProgramID,
	"executor":                   executor.ProgramID,
	"fee_quoter":                 fee_quoter.ProgramID,
	"lockrelease_token_pool":     lockrelease_token_pool.ProgramID,
	"rmn_remote":                 rmn_remote.ProgramID,
	"test_ccip_invalid_receiver": test_ccip_invalid_receiver.ProgramID,
	"test_ccip_receiver":         test_ccip_receiver.ProgramID,
}

var programToVanityKey = map[string]string{
	"ccip_router":        "Ccip",
	"ccip_offramp":       "off",
	"fee_quoter":         "FeeQ",
	"rmn_remote":         "Rmn",
	"committee_verifier": "CV",
	"executor":           "Exe",
}

func generateKey(ctx context.Context, program, programsPath string) (string, error) {
	// Construct command arguments, generate vanity keys for some programs
	var args []string
	if prefix, exists := programToVanityKey[program]; exists {
		args = []string{"grind", "--starts-with", prefix + ":1"}
	} else {
		args = []string{"new", "--no-passphrase", "-o", solcommon.KeypairFile(program)}
	}

	destination := solcommon.KeypairPath(programsPath, program)

	// Skip generation if the keypair already exists (idempotent re-runs).
	if _, err := os.Stat(destination); err == nil {
		return destination, nil
	}

	// solana-keygen writes the keypair JSON into its working directory, so use
	// a temp dir to avoid polluting the caller's CWD and to stay on the same
	// filesystem as programsPath (os.Rename requires same-device).
	tmpDir, err := os.MkdirTemp("", "solana-keygen-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir for keygen: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Run command using helper function
	output, err := runCommand(ctx, "solana-keygen", args, tmpDir)
	if err != nil {
		return "", fmt.Errorf("failed to generate key for program %s: %w", program, err)
	}

	// Parse output for JSON filename
	scanner := bufio.NewScanner(strings.NewReader(output))
	jsonFilePattern := regexp.MustCompile(`Wrote (new\s)?keypair to (.*\.json)`)
	var jsonFilePath string

	for scanner.Scan() {
		line := scanner.Text()
		matches := jsonFilePattern.FindStringSubmatch(line)
		if len(matches) > 2 {
			jsonFilePath = matches[2]
			break
		}
	}

	if jsonFilePath == "" {
		return "", fmt.Errorf("failed to parse output for JSON file path when generating key for program %s: %s", program, output)
	}

	// jsonFilePath is relative to tmpDir; make it absolute.
	if !filepath.IsAbs(jsonFilePath) {
		jsonFilePath = filepath.Join(tmpDir, jsonFilePath)
	}

	if err := os.Rename(jsonFilePath, destination); err != nil {
		return "", fmt.Errorf("failed to move generated key from %s to %s: %w", jsonFilePath, destination, err)
	}
	return destination, nil
}

func GenerateKeys(ctx context.Context, programsPath string) (map[string]solana.PublicKey, error) {
	keys := make(map[string]solana.PublicKey, len(ccipPrograms))
	for program := range ccipPrograms {
		programFile, err := generateKey(ctx, program, programsPath)
		if err != nil {
			return nil, err
		}
		programKey, err := solana.PrivateKeyFromSolanaKeygenFile(programFile)
		if err != nil {
			return nil, err
		}
		keys[program] = programKey.PublicKey()
	}
	return keys, nil
}

// stageReleaseArtifacts downloads the given release's tarball and stages its
// programs, IDLs and provenance manifest into programsPath.
func stageReleaseArtifacts(ctx context.Context, programsPath, release string) error {
	destDir, err := utils.DownloadArtifacts(ctx, utils.DownloadArtifactsOpts{
		Host:         "github.com",
		Organization: "smartcontractkit",
		Repository:   "chainlink-ccip-solana",
		Release:      release,
		Asset:        "artifacts.tar.gz",
		Verify:       verifyReleaseArtifacts(release),
	})
	if err != nil {
		return fmt.Errorf("failed to download artifacts: %w", err)
	}

	// Copy from destDir to programsPath
	matches, err := filepath.Glob(filepath.Join(destDir, "target/deploy/*.so"))
	if err != nil {
		return fmt.Errorf("failed to find programs: %w", err)
	}
	if len(matches) == 0 {
		return fmt.Errorf("release %s contains no programs under %s; the cached package at %s may be corrupt, delete it and retry",
			release, filepath.Join("target", "deploy"), destDir)
	}
	for _, program := range matches {
		name := filepath.Base(program)
		if copyErr := copyFile(program, filepath.Join(programsPath, name)); copyErr != nil {
			return fmt.Errorf("failed to copy program %s: %w", name, copyErr)
		}
	}

	// The release tarball also carries target/idl/*.json. Copy those
	// alongside the binaries so the IDL changesets can publish them
	// on chain; without this the files are downloaded and discarded.
	// Carry the release manifest across too. RequireDeployable refuses to
	// deploy to a real network without it.
	if copyErr := copyFile(
		filepath.Join(destDir, "target", solcommon.ProvenanceFile),
		filepath.Join(programsPath, solcommon.ProvenanceFile),
	); copyErr != nil {
		return fmt.Errorf("failed to copy provenance manifest: %w", copyErr)
	}

	idls, err := filepath.Glob(filepath.Join(destDir, "target/idl/*.json"))
	if err != nil {
		return fmt.Errorf("failed to find idls: %w", err)
	}
	for _, idlFile := range idls {
		name := filepath.Base(idlFile)
		if copyErr := copyFile(idlFile, filepath.Join(programsPath, name)); copyErr != nil {
			return fmt.Errorf("failed to copy idl %s: %w", name, copyErr)
		}
	}
	return nil
}

var DeployChainContracts = operations.NewSequence(
	"deploy-chain-contracts",
	solcommon.Version2_0_0,
	"Deploys all required protocol contracts for CCIP 2.0.0 to a Solana chain",
	func(b operations.Bundle, chains cldf_chain.BlockChains, input ccvadapters.DeployChainContractsInput) (output ccvadapters.DeployChainContractsOutput, err error) {
		if valErr := validateDeployContractParamVersions(input.ContractParams); valErr != nil {
			return ccvadapters.DeployChainContractsOutput{}, valErr
		}
		chain := chains.SolanaChains()[input.ChainSelector]
		addresses := make([]datastore.AddressRef, 0)
		// FeeQuoter price updater and billing tokens; they are batched once MCMS owns the FeeQuoter
		batchOps := make([]mcms_types.BatchOperation, 0, 3)

		// Artifact sourcing per environment: devenv preloads local builds; staging
		// deploys its local build; production downloads a pinned release tag
		// (canonical IDs shared by prod-testnet and mainnet).
		if input.DeployerContract != "devenv" {
			release, local, srcErr := solcommon.ResolveArtifactSource()
			if srcErr != nil {
				return ccvadapters.DeployChainContractsOutput{}, srcErr
			}
			if !local {
				if stageErr := stageReleaseArtifacts(b.GetContext(), chain.ProgramsPath, release); stageErr != nil {
					return ccvadapters.DeployChainContractsOutput{}, stageErr
				}
			}
		}

		// Fail before any program is deployed.
		if verErr := requireIDLVersions(chain.ProgramsPath, input.ExistingAddresses, input.ChainSelector, input.ContractParams); verErr != nil {
			return ccvadapters.DeployChainContractsOutput{}, verErr
		}

		// LINK is a shared token, not a CCIP program, so this flow does not deploy it:
		// the environment provides the mint — devenv deploys a mock in its pre-deploy
		// hook, production resolves the canonical mint. The ref key matches what the
		// other sequences (fee vaults, lookup tables) use.
		linkTokenAddress, err := findLinkMint(input.ExistingAddresses, input.ChainSelector)
		if err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to resolve LINK token: %w", err)
		}
		if linkTokenAddress.IsZero() {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to resolve LINK token: %w: chain %d type %s",
				solcommon.ErrAddressRefNotFound, input.ChainSelector, tokensops.LinkContractType)
		}

		// Deploy Router
		routerRef, err := operations.ExecuteOperation(b, routerops.Deploy, chain, input.ExistingAddresses)
		if err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to deploy Router: %w", err)
		}
		addresses = append(addresses, routerRef.Output)

		// Deploy FeeQuoter
		feeQuoterRef, err := operations.ExecuteOperation(b, fqops.Deploy, chain, input.ExistingAddresses)
		if err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to deploy FeeQuoter: %w", err)
		}
		addresses = append(addresses, feeQuoterRef.Output)

		// Deploy OffRamp
		offRampRef, err := operations.ExecuteOperation(b, offrampops.Deploy, chain, input.ExistingAddresses)
		if err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to deploy OffRamp: %w", err)
		}
		addresses = append(addresses, offRampRef.Output)

		// Deploy Executor
		executorRef, err := operations.ExecuteOperation(b, executorops.Deploy, chain, input.ExistingAddresses)
		if err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to deploy Executor: %w", err)
		}
		addresses = append(addresses, executorRef.Output)
		// NOTE: workaround EVM idiosyncracies, register as executor proxy too
		executorProxyRef := executorRef.Output
		executorProxyRef.Type = executorops.ProxyContractType
		addresses = append(addresses, executorProxyRef)

		// Deploy RMN Remote
		rmnRemoteRef, err := operations.ExecuteOperation(b, rmnremoteops.Deploy, chain, input.ExistingAddresses)
		if err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to deploy RMN Remote: %w", err)
		}
		addresses = append(addresses, rmnRemoteRef.Output)

		// Deploy BurnMint Token Pool
		// Pool binaries come from the same artifact source as every other program:
		// the local build (devenv/staging) or the pinned release tarball (production).
		burnMintTokenPoolRef, err := operations.ExecuteOperation(b, tokenpoolops.DeployBurnMint, chain, input.ExistingAddresses)
		if err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to deploy BurnMint Token Pool: %w", err)
		}
		addresses = append(addresses, burnMintTokenPoolRef.Output)
		// HACK: we deploy 2.0.0 but sequences/tokens.go still seems to expect 1.6.1 so we double register
		burnMint16Ref := burnMintTokenPoolRef.Output
		burnMint16Ref.Version = solcommon.LegacyTokenPoolVersion
		addresses = append(addresses, burnMint16Ref)

		// Deploy LockRelease Token Pool
		lockReleaseTokenPoolRef, err := operations.ExecuteOperation(b, tokenpoolops.DeployLockRelease, chain, input.ExistingAddresses)
		if err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to deploy LockRelease Token Pool: %w", err)
		}
		addresses = append(addresses, lockReleaseTokenPoolRef.Output)
		// HACK: we deploy 2.0.0 but sequences/tokens.go still seems to expect 1.6.1 so we double register
		lockRelease16Ref := lockReleaseTokenPoolRef.Output
		lockRelease16Ref.Version = solcommon.LegacyTokenPoolVersion
		addresses = append(addresses, lockRelease16Ref)

		feeQuoterAddress := solana.MustPublicKeyFromBase58(feeQuoterRef.Output.Address)
		offRampAddress := solana.MustPublicKeyFromBase58(offRampRef.Output.Address)
		executorAddress := solana.MustPublicKeyFromBase58(executorRef.Output.Address)
		rmnRemoteAddress := solana.MustPublicKeyFromBase58(rmnRemoteRef.Output.Address)
		ccipRouterProgram := solana.MustPublicKeyFromBase58(routerRef.Output.Address)
		burnMintTokenPoolAddress := solana.MustPublicKeyFromBase58(burnMintTokenPoolRef.Output.Address)
		lockReleaseTokenPoolAddress := solana.MustPublicKeyFromBase58(lockReleaseTokenPoolRef.Output.Address)

		// Initialize the program-level token-pool configs once the pool programs,
		// Router, and RMN Remote have been deployed. Per-token initialization also
		// checks these configs, but deployments with no token setup still need them.
		if err = runOp(b, tokenpoolops.InitGlobalConfigBurnMint, chain, tokenpoolops.Params{
			TokenPool: burnMintTokenPoolAddress,
			Router:    ccipRouterProgram,
			RMNRemote: rmnRemoteAddress,
		}, &batchOps); err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to initialize BurnMint Token Pool global config: %w", err)
		}
		if err = runOp(b, tokenpoolops.InitGlobalConfigLockRelease, chain, tokenpoolops.Params{
			TokenPool: lockReleaseTokenPoolAddress,
			Router:    ccipRouterProgram,
			RMNRemote: rmnRemoteAddress,
		}, &batchOps); err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to initialize LockRelease Token Pool global config: %w", err)
		}

		// Initialize FeeQuoter
		lowBits, highBits := GetHighLowBits(input.ContractParams.FeeQuoter.MaxFeeJuelsPerMsg)
		_, err = operations.ExecuteOperation(b, fqops.Initialize, chain, fqops.Params{
			MaxFeeJuelsPerMsg: bin.Uint128{
				Lo:         lowBits,
				Hi:         highBits,
				Endianness: nil,
			},
			FeeQuoter: feeQuoterAddress,
			Router:    ccipRouterProgram,
			OffRamp:   offRampAddress,
			LinkToken: linkTokenAddress,
		})
		if err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to initialize FeeQuoter: %w", err)
		}

		err = runOp(b, fqops.AddPriceUpdater, chain, fqops.Params{
			OffRamp:   offRampAddress,
			FeeQuoter: feeQuoterAddress,
		}, &batchOps)
		if err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to add price updater to FeeQuoter: %w", err)
		}

		// Register the fee (billing) tokens with initial prices. The 2.0 fee accounting CPIs
		// into the FeeQuoter for a validated fee token price on every ccip_send_v2/get_fee_v2,
		// so a message using an unregistered fee token cannot be sent at all.
		// Prices are 18-decimals USD for 1e18 smallest token denominations; both mints have
		// 9 decimals, so $/whole-token scales by 1e9.
		wsolUsdPrice := new(big.Int).Mul(big.NewInt(150e9), big.NewInt(1e18)) // $150 per SOL
		linkUsdPrice := new(big.Int).Mul(big.NewInt(20e9), big.NewInt(1e18))  // $20 per LINK
		for _, billingToken := range []fqops.AddBillingTokenConfigInput{
			{
				FeeQuoter:                  feeQuoterAddress,
				Router:                     ccipRouterProgram,
				Mint:                       solana.WrappedSol,
				TokenProgram:               solana.TokenProgramID,
				UsdPerToken:                wsolUsdPrice,
				PremiumMultiplierWeiPerEth: 9e6,
			},
			{
				FeeQuoter:                  feeQuoterAddress,
				Router:                     ccipRouterProgram,
				Mint:                       linkTokenAddress,
				TokenProgram:               solana.TokenProgramID,
				UsdPerToken:                linkUsdPrice,
				PremiumMultiplierWeiPerEth: 11e6,
			},
		} {
			if err = runOp(b, fqops.AddBillingTokenConfig, chain, billingToken, &batchOps); err != nil {
				return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to add billing token config for %s: %w", billingToken.Mint, err)
			}
		}

		// Initialize Router
		_, err = operations.ExecuteOperation(b, routerops.Initialize, chain, routerops.Params{
			FeeQuoter: feeQuoterAddress,
			Router:    ccipRouterProgram,
			LinkToken: linkTokenAddress,
			RMNRemote: rmnRemoteAddress,
		})
		if err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to initialize Router: %w", err)
		}

		// Initialize Executor
		defaultExecutorCfg := input.ContractParams.Executors[0]
		if _, err = operations.ExecuteOperation(b, executorops.Initialize, chain, executorops.Params{
			Executor:              executorAddress,
			MaxCcvsPerMsg:         defaultExecutorCfg.MaxCCVsPerMsg,
			FeeAggregator:         chain.DeployerKey.PublicKey(),                    // TODO: move all config to the defaultExecutorCfg
			AllowedFinalityConfig: executor.FinalityConfig{Flags: 0, BlockDepth: 1}, // TODO: move all config to the defaultExecutorCfg
			CcvAllowlistEnabled:   defaultExecutorCfg.DynamicConfig.CcvAllowlistEnabled,
		}); err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("initialize executor: %w", err)
		}

		// initialize fee vault ATAs
		type ATA struct {
			ata          solana.PublicKey
			tokenProgram solana.PublicKey
			mint         solana.PublicKey
			authority    solana.PublicKey
		}
		ataFrom := func(tokenProgram, mint, authority solana.PublicKey) ATA {
			ata, _, aerr := tokens.FindAssociatedTokenAddress(tokenProgram, mint, authority)
			if aerr != nil {
				b.Logger.Warnw("failed to find associated token address", "tokenProgram", tokenProgram.String(), "mint", mint.String(), "authority", authority.String(), "error", aerr)
			}
			return ATA{
				ata:          ata,
				tokenProgram: tokenProgram,
				mint:         mint,
				authority:    authority,
			}
		}

		fees, err := feeTokens(b.GetContext(), chain, linkTokenAddress)
		if err != nil {
			return ccvadapters.DeployChainContractsOutput{}, err
		}
		atas := make([]ATA, 0, len(fees))
		for _, ft := range fees {
			vaultPda, _, aerr := state.FindFeeVaultPDA(executorAddress, []byte{})
			if aerr != nil {
				return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to find executor fee vault: %w", aerr)
			}
			atas = append(atas, ataFrom(ft.program, ft.mint, vaultPda))
		}

		for _, ata := range atas {
			ixn, _, ataErr := tokens.CreateAssociatedTokenAccount(
				ata.tokenProgram,
				ata.mint,
				ata.authority,
				chain.DeployerKey.PublicKey(),
			)
			if ataErr != nil {
				return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to create the instruction to create associated token account for Executor vault ATA (mint: %s, ccv: %s, ata/vaultAta: %s): %w", ata.mint.String(), executorAddress.String(), ata.ata.String(), ataErr)
			}
			if confirmErr := chain.Confirm([]solana.Instruction{ixn}); confirmErr != nil {
				return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to create associated token account for Executor vault ATA (mint: %s, ccv: %s, ata/vaultAta: %s): %w", ata.mint.String(), executorAddress.String(), ata.ata.String(), confirmErr)
			}
		}

		// Initialize OffRamp
		_, err = operations.ExecuteOperation(b, offrampops.Initialize, chain, offrampops.Params{
			FeeQuoter: feeQuoterAddress,
			OffRamp:   offRampAddress,
			Router:    ccipRouterProgram,
			RMNRemote: rmnRemoteAddress,
		})
		if err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to initialize OffRamp: %w", err)
		}

		_, err = operations.ExecuteOperation(b, offrampops.InitializeConfig, chain, offrampops.Params{
			OffRamp: offRampAddress,
		})
		if err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to initialize OffRamp config: %w", err)
		}

		// Initialize RMN Remote
		_, err = operations.ExecuteOperation(b, rmnremoteops.Initialize, chain, rmnremoteops.Params{
			RMNRemote: solana.MustPublicKeyFromBase58(rmnRemoteRef.Output.Address),
		})
		if err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to initialize RMN Remote: %w", err)
		}

		offrampLUTEntries, err := OfframpLutEntries(OfframpLutEntriesInput{
			OffRamp:              offRampAddress,
			FeeQuoter:            feeQuoterAddress,
			Router:               ccipRouterProgram,
			RMNRemote:            rmnRemoteAddress,
			LinkToken:            linkTokenAddress,
			BurnMintTokenPool:    burnMintTokenPoolAddress,
			LockReleaseTokenPool: lockReleaseTokenPoolAddress,
		})
		if err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to create OffRamp LUT entries: %w", err)
		}
		err = solcommon.ExtendOfframpLookupTable(chain, offRampAddress, offrampLUTEntries)
		if err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to extend OffRamp lookup table: %w", err)
		}

		// Create a ccipSend address lookup table with static accounts used by the
		// ccip_send_v2 instruction so transactions stay within the 1232-byte limit.
		ccipSendLUTEntries, err := CcipSendLutEntries(CcipSendLutEntriesInput{
			LinkToken:            linkTokenAddress,
			Router:               ccipRouterProgram,
			FeeQuoter:            feeQuoterAddress,
			RMNRemote:            rmnRemoteAddress,
			BurnMintTokenPool:    burnMintTokenPoolAddress,
			LockReleaseTokenPool: lockReleaseTokenPoolAddress,
			Executor:             executorAddress,
		})
		if err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to create ccipSend LUT entries: %w", err)
		}

		ccipSendLUTAddr, err := solcommonutils.SetupLookupTable(b.GetContext(), chain.Client, *chain.DeployerKey, ccipSendLUTEntries)
		if err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("create ccipSend lookup table: %w", err)
		}
		addresses = append(addresses, datastore.AddressRef{
			Address:       ccipSendLUTAddr.String(),
			Type:          solcommon.CcipSendLookupTableType,
			Version:       solcommon.Version2_0_0,
			Qualifier:     "",
			ChainSelector: input.ChainSelector,
		})
		// Point the router's ccip_send_v2 / get_fee_v2 account resolution at the table, so senders
		// get it from resolution instead of from the datastore.
		setLUTReport, err := operations.ExecuteOperation(b, routerops.UpdateReferenceAddresses, chain, routerops.UpdateReferenceAddressesParams{
			Router:            ccipRouterProgram,
			OnrampLookupTable: ccipSendLUTAddr,
		})
		if err != nil {
			return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to update router reference addresses: %w", err)
		}

		output = ccvadapters.DeployChainContractsOutput{
			// The batch is only non-empty when the router is already owned by MCMS.
			OnChainOutput: sequences.OnChainOutput{Addresses: addresses, BatchOps: append(batchOps, setLUTReport.Output.BatchOps...)},
		}
		if !input.DeployerKeyOwned {
			// Hand the freshly deployed programs to the timelock: the changeset transfers
			// every ref listed here. Programs handed over by an earlier run are owned by
			// the timelock already, so they are filtered out to keep re-runs idempotent.
			ownable := []datastore.AddressRef{
				routerRef.Output,
				feeQuoterRef.Output,
				offRampRef.Output,
				executorRef.Output,
				rmnRemoteRef.Output,
			}
			timelock, found, terr := solcommon.FindTimelockSignerPDA(input.ExistingAddresses, input.ChainSelector, common_utils.CLLQualifier)
			if terr != nil {
				return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to resolve timelock for ownership handover: %w", terr)
			}
			if !found {
				// Without the timelock ref there is nobody to hand ownership to and the
				// idempotency filter below cannot run, so re-runs would re-propose
				// transfers for programs already owned by the timelock. The shared
				// adapter contract requires failing fast here; DeployerKeyOwned skips
				// the handover for flows that intentionally keep the deployer as owner.
				return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf(
					"timelock (qualifier %q) not found in existing addresses for chain %d: deploy MCMS first, or set DeployerKeyOwned to keep the deployer as owner",
					common_utils.CLLQualifier, input.ChainSelector)
			}
			ownable, err = refsNotOwnedBy(ownable, timelock, func(ref datastore.AddressRef) (solana.PublicKey, error) {
				return programAuthority(b.GetContext(), chain, ref)
			})
			if err != nil {
				return ccvadapters.DeployChainContractsOutput{}, fmt.Errorf("failed to check current owners: %w", err)
			}
			output.RefsToTransferOwnership = ownable
		}
		return output, nil
	},
)

// validateDeployContractParamVersions enforces the shared contract-version constraint from the
// upstream deploy interface. Solana maps the upstream OnRamp version to its Router program; the
// remaining Solana-only programs use solcommon.Version2_0_0 and are versioned locally.
func validateDeployContractParamVersions(params ccvadapters.DeployContractParams) error {
	type namedVersion struct {
		name    string
		version *semver.Version
	}
	versions := make([]namedVersion, 0, 3+len(params.CommitteeVerifiers)+len(params.Executors)+len(params.MockReceivers))
	versions = append(versions,
		namedVersion{name: "OffRamp", version: params.OffRamp.Version},
		namedVersion{name: "OnRamp", version: params.OnRamp.Version},
		namedVersion{name: "FeeQuoter", version: params.FeeQuoter.Version},
	)
	for i, verifier := range params.CommitteeVerifiers {
		versions = append(versions, namedVersion{name: fmt.Sprintf("CommitteeVerifiers[%d]", i), version: verifier.Version})
	}
	for i, executor := range params.Executors {
		versions = append(versions, namedVersion{name: fmt.Sprintf("Executors[%d]", i), version: executor.Version})
	}
	for i, receiver := range params.MockReceivers {
		versions = append(versions, namedVersion{name: fmt.Sprintf("MockReceivers[%d]", i), version: receiver.Version})
	}

	var expected *semver.Version
	for _, item := range versions {
		if item.version == nil {
			return fmt.Errorf("%s version is required", item.name)
		}
		if expected == nil {
			expected = item.version
			continue
		}
		if !expected.Equal(item.version) {
			return fmt.Errorf("all contract parameter versions must be equal: %s has %s, expected %s", item.name, item.version, expected)
		}
	}
	return nil
}

// refsNotOwnedBy drops refs whose program is already owned by owner, so redeploying over
// an environment where the ownership handover ran does not re-propose transfers.
func refsNotOwnedBy(
	refs []datastore.AddressRef,
	owner solana.PublicKey,
	authorityOf func(datastore.AddressRef) (solana.PublicKey, error),
) ([]datastore.AddressRef, error) {
	kept := make([]datastore.AddressRef, 0, len(refs))
	for _, ref := range refs {
		authority, err := authorityOf(ref)
		if err != nil {
			return nil, err
		}
		if authority == owner {
			continue
		}
		kept = append(kept, ref)
	}
	return kept, nil
}

// programAuthority reads the current on-chain owner of the program behind ref.
func programAuthority(ctx context.Context, chain cldf_solana.Chain, ref datastore.AddressRef) (solana.PublicKey, error) {
	program, err := solana.PublicKeyFromBase58(ref.Address)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("parse %s address %q: %w", ref.Type, ref.Address, err)
	}
	switch ref.Type {
	case datastore.ContractType(routerops.ContractType):
		return routerops.GetAuthority(ctx, chain, program)
	case datastore.ContractType(fqops.ContractType):
		return fqops.GetAuthority(ctx, chain, program)
	case datastore.ContractType(offrampops.ContractType):
		return offrampops.GetAuthority(ctx, chain, program)
	case datastore.ContractType(rmnremoteops.ContractType):
		return rmnremoteops.GetAuthority(ctx, chain, program)
	case datastore.ContractType(executorops.ContractType):
		return executorops.GetAuthority(ctx, chain, program)
	default:
		return solana.PublicKey{}, fmt.Errorf("unsupported contract type %s for ownership check", ref.Type)
	}
}

type CcipSendLutEntriesInput struct {
	LinkToken            solana.PublicKey
	Router               solana.PublicKey
	FeeQuoter            solana.PublicKey
	RMNRemote            solana.PublicKey
	BurnMintTokenPool    solana.PublicKey
	LockReleaseTokenPool solana.PublicKey
	Executor             solana.PublicKey
}

// CcipSendLutEntries returns the static accounts used by ccip_send_v2.
// Both fresh deployments and maintenance extensions use this function so older
// lookup tables can be brought to the current account set without replacement.
func CcipSendLutEntries(input CcipSendLutEntriesInput) ([]solana.PublicKey, error) {
	routerConfigPDA, _, err := state.FindConfigPDA(input.Router)
	if err != nil {
		return nil, err
	}
	feeQuoterConfigPDA, _, err := state.FindFqConfigPDA(input.FeeQuoter)
	if err != nil {
		return nil, err
	}
	feeBillingSignerPDA, _, err := state.FindFeeBillingSignerPDA(input.Router)
	if err != nil {
		return nil, err
	}
	rmnRemoteConfigPDA, _, err := state.FindRMNRemoteConfigPDA(input.RMNRemote)
	if err != nil {
		return nil, err
	}
	rmnRemoteCursePDA, _, err := state.FindRMNRemoteCursesPDA(input.RMNRemote)
	if err != nil {
		return nil, err
	}
	wsolFqBillingConfigPDA, _, err := state.FindFqBillingTokenConfigPDA(solana.WrappedSol, input.FeeQuoter)
	if err != nil {
		return nil, err
	}
	linkFqBillingConfigPDA, _, err := state.FindFqBillingTokenConfigPDA(input.LinkToken, input.FeeQuoter)
	if err != nil {
		return nil, err
	}
	executorConfigPDA, _, err := state.FindConfigPDA(input.Executor)
	if err != nil {
		return nil, err
	}

	return []solana.PublicKey{
		solana.SystemProgramID,
		solana.TokenProgramID,
		solana.Token2022ProgramID,
		solana.WrappedSol,
		input.LinkToken,
		input.Router,
		routerConfigPDA,
		input.FeeQuoter,
		feeQuoterConfigPDA,
		feeBillingSignerPDA,
		input.RMNRemote,
		rmnRemoteConfigPDA,
		rmnRemoteCursePDA,
		wsolFqBillingConfigPDA,
		linkFqBillingConfigPDA,
		input.BurnMintTokenPool,
		input.LockReleaseTokenPool,
		input.Executor,
		executorConfigPDA,
	}, nil
}

type OfframpLutEntriesInput struct {
	OffRamp              solana.PublicKey
	FeeQuoter            solana.PublicKey
	Router               solana.PublicKey
	RMNRemote            solana.PublicKey
	LinkToken            solana.PublicKey
	BurnMintTokenPool    solana.PublicKey
	LockReleaseTokenPool solana.PublicKey
}

func OfframpLutEntries(input OfframpLutEntriesInput) ([]solana.PublicKey, error) {
	// off ramp
	offRampReferenceAddressesPDA, _, _ := state.FindOfframpReferenceAddressesPDA(input.OffRamp)
	offRampBillingSignerPDA, _, _ := state.FindOfframpBillingSignerPDA(input.OffRamp)
	offRampConfigPDA, _, _ := state.FindOfframpConfigPDA(input.OffRamp)
	// fee quoter
	linkFqBillingConfigPDA, _, _ := state.FindFqBillingTokenConfigPDA(input.LinkToken, input.FeeQuoter)
	wsolFqBillingConfigPDA, _, _ := state.FindFqBillingTokenConfigPDA(solana.WrappedSol, input.FeeQuoter)
	feeQuoterConfigPDA, _, _ := state.FindFqConfigPDA(input.FeeQuoter)
	// router
	feeBillingSignerPDA, _, _ := state.FindFeeBillingSignerPDA(input.Router)
	routerConfigPDA, _, _ := state.FindConfigPDA(input.Router)
	// rmn remote
	rmnRemoteCursePDA, _, _ := state.FindRMNRemoteCursesPDA(input.RMNRemote)
	rmnRemoteConfigPDA, _, _ := state.FindRMNRemoteConfigPDA(input.RMNRemote)

	return []solana.PublicKey{
		// TODO: we can probably add more offramp related accounts here
		input.OffRamp,
		offRampConfigPDA,
		offRampReferenceAddressesPDA,
		offRampBillingSignerPDA,
		// TODO: we don't update prices via offramp in 2.0, so some of them can go away
		// TODO: pricer will likely update fee quoter directly
		// fee quoter
		feeQuoterConfigPDA,
		input.FeeQuoter,
		linkFqBillingConfigPDA,
		wsolFqBillingConfigPDA,
		// router
		input.Router,
		routerConfigPDA,
		feeBillingSignerPDA,
		// rmn remote
		input.RMNRemote,
		rmnRemoteConfigPDA,
		rmnRemoteCursePDA,
		// token pools
		input.BurnMintTokenPool,
		input.LockReleaseTokenPool,
	}, nil
}

func GetHighLowBits(n *big.Int) (low, high uint64) {
	mask := big.NewInt(0).SetUint64(0xFFFFFFFFFFFFFFFF) // 64-bit mask

	lowBig := big.NewInt(0).And(n, mask)
	low = lowBig.Uint64()

	highBig := big.NewInt(0).Rsh(n, 64) // Shift right by 64 bits
	high = highBig.Uint64()

	return low, high
}

// requireIDLVersions checks that each program about to be deployed declares, in its IDL, the version
// the deploy parameters ask for. Programs with an existing ref on the same major are reused, not deployed.
func requireIDLVersions(programsPath string, existing []datastore.AddressRef, chainSelector uint64, params ccvadapters.DeployContractParams) error {
	requested := params.OffRamp.Version
	if requested == nil {
		return nil
	}
	programs := []struct {
		contractType datastore.ContractType
		program      string
	}{
		{datastore.ContractType(routerops.ContractType), routerops.ProgramName},
		{datastore.ContractType(offrampops.ContractType), offrampops.ProgramName},
		{datastore.ContractType(fqops.ContractType), fqops.ProgramName},
		{datastore.ContractType(executorops.ContractType), executorops.ProgramName},
		{datastore.ContractType(cvops.ContractType), cvops.ProgramName},
	}
	for _, p := range programs {
		if _, err := solcommon.FindCompatibleRefInRefs(existing, chainSelector, datastore.AddressRef{Type: p.contractType, Version: requested}); err == nil {
			continue
		}
		info, err := solcommon.InspectArtifact(programsPath, p.program)
		if err != nil || info.IDLPath == "" {
			continue // a missing artifact or IDL is reported by the deploy itself
		}
		idlVersion, err := solcommon.IDLVersion(info.IDLPath)
		if err != nil {
			return err
		}
		if !idlVersion.Equal(requested) {
			return fmt.Errorf("refusing to deploy %s: its IDL declares version %s but %s was requested", p.program, idlVersion, requested)
		}
	}
	return nil
}
