package sequences

import (
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"

	chainsel "github.com/smartcontractkit/chain-selectors"
	tokenapi "github.com/smartcontractkit/chainlink-ccip/deployment/tokens"
	common_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils"
	datastore_utils "github.com/smartcontractkit/chainlink-ccip/deployment/utils/datastore"
	"github.com/smartcontractkit/chainlink-ccip/deployment/utils/sequences"
	ccvadapters "github.com/smartcontractkit/chainlink-ccip/deployment/v2_0_0/adapters"
	cldf_chain "github.com/smartcontractkit/chainlink-deployments-framework/chain"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	routerops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/router"
	tokenpoolops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokenpools"
	tokensops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/tokens"
)

// ConfigureTokenForTransfersSequence registers a token admin, sets the pool on the router, and configures
// remote chain configs with rate limits. On Solana, it resolves the token program ID (SPL or SPL2022) from the datastore.
func (a *SolanaAdapter) ConfigureTokenForTransfersSequence() *operations.Sequence[tokenapi.ConfigureTokenForTransfersInput, sequences.OnChainOutput, cldf_chain.BlockChains] {
	return operations.NewSequence(
		"solana-adapter:configure-token-for-transfers",
		solcommon.Version2_0_0,
		"Configure a token for cross-chain transfers across multiple chains",
		func(b operations.Bundle, chains cldf_chain.BlockChains, input tokenapi.ConfigureTokenForTransfersInput) (sequences.OnChainOutput, error) {
			var result sequences.OnChainOutput
			chain, ok := chains.SolanaChains()[input.ChainSelector]
			if !ok {
				return sequences.OnChainOutput{}, fmt.Errorf("chain with selector %d not defined", input.ChainSelector)
			}

			b.Logger.Info("SVM Registering token:", input)

			routerAddr, err := a.GetRouterAddress(input.ExistingDataStore, input.ChainSelector)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to get router address: %w", err)
			}

			tokenAddr, tokenProgramID, err := solcommon.GetTokenMintAndTokenProgram(input.ExistingDataStore, input.TokenRef, chain)
			if err != nil {
				return sequences.OnChainOutput{}, err
			}
			tpAddr := solana.MustPublicKeyFromBase58(input.TokenPoolAddress)

			// if no token admin provided, ccip admin becomes the admin
			var tokenAdmin solana.PublicKey
			if input.ExternalAdmin != "" {
				tokenAdmin = solana.MustPublicKeyFromBase58(input.ExternalAdmin)
			}

			rtarOut, err := operations.ExecuteOperation(b, routerops.RegisterTokenAdminRegistry, chain, routerops.TokenAdminRegistryParams{
				Router:            solana.PublicKeyFromBytes(routerAddr),
				TokenMint:         solana.MustPublicKeyFromBase58(tokenAddr.Address),
				Admin:             tokenAdmin,
				ExistingAddresses: input.ExistingDataStore.Addresses().Filter(),
			})
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to register token admin registry: %w", err)
			}
			mergeOnChainOutput(&result, rtarOut.Output.OnChainOutput)
			pendingSigner := rtarOut.Output.PendingSigner

			// If PendingSigner is zero, the TAR was already registered and accepted — nothing to do.
			// Otherwise, attempt to accept. The operation itself validates whether we can sign.
			if !pendingSigner.IsZero() {
				atarOut, acceptErr := operations.ExecuteOperation(b, routerops.AcceptTokenAdminRegistry, chain, routerops.TokenAdminRegistryParams{
					Router:            solana.PublicKeyFromBytes(routerAddr),
					TokenMint:         solana.MustPublicKeyFromBase58(tokenAddr.Address),
					Admin:             pendingSigner,
					ExistingAddresses: input.ExistingDataStore.Addresses().Filter(),
				})
				if acceptErr != nil {
					b.Logger.Warnw("WARNING: failed to accept token admin registry — the pending admin must accept separately before the token is fully configured",
						"tokenMint", tokenAddr.Address, "pendingSigner", pendingSigner.String(), "error", acceptErr)
				} else {
					mergeOnChainOutput(&result, atarOut.Output)
				}
			}

			b.Logger.Info("SVM Setting token pool:", input)

			fqAddr, err := a.GetFQAddress(input.ExistingDataStore, input.ChainSelector)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to get fee quoter address: %w", err)
			}

			spOut, err := operations.ExecuteOperation(b, routerops.SetPool, chain, routerops.PoolParams{
				ExistingAddresses: input.ExistingDataStore.Addresses().Filter(),
				Router:            solana.PublicKeyFromBytes(routerAddr),
				FeeQuoter:         solana.PublicKeyFromBytes(fqAddr),
				TokenMint:         solana.MustPublicKeyFromBase58(tokenAddr.Address),
				TokenPool:         tpAddr,
				TokenProgramID:    tokenProgramID,
				TokenPoolType:     input.PoolType,
				// The token's own datastore ref is already qualified by its symbol.
				TokenSymbol: tokenAddr.Qualifier,
			})
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to set token pool: %w", err)
			}
			mergeOnChainOutput(&result, spOut.Output)

			// Set interface_version=2 for v2 pools so the offramp uses the v2 CPI path;
			// otherwise the token defaults to v1 (legacy release/mint, no remote chain selector).
			// Resolve the version from the pool ref registered under this qualifier; an address
			// lookup is ambiguous because the pool program is double-registered under multiple
			// versions (deploy_chain_contracts.go HACK).
			poolRef, err := solcommon.PoolRefByQualifier(input.ExistingDataStore, input.ChainSelector, input.PoolType, input.TokenRef.Qualifier)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("resolve pool ref for qualifier %q: %w", input.TokenRef.Qualifier, err)
			}
			if poolRef.Version.Compare(solcommon.Version2_0_0) >= 0 {
				// WithoutExtendedLookupTable: 2.0 lanes skip the lookup table's additional entries and rely on the
				// pool's resolver alone, while 1.6 lanes, which only read whether resolution is enabled, keep them.
				// ManualRegistration appends a multisig mint authority to the table, and only 1.6 lanes need it
				// there: on 2.0 the burn/mint resolver returns it, and a table entry would arrive ahead of the
				// accounts the pool resolves and shift them. With no additional entries this is the same as Standard.
				spiOut, spiErr := operations.ExecuteOperation(b, routerops.SetPoolInterfaceImplementation, chain, routerops.SetPoolInterfaceImplParams{
					Router:                      solana.PublicKeyFromBytes(routerAddr),
					TokenMint:                   solana.MustPublicKeyFromBase58(tokenAddr.Address),
					InterfaceVersion:            2,
					ImplementsAccountResolution: uint8(ccip_router.ResolutionType_WithoutExtendedLookupTable),
				})
				if spiErr != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to set pool interface implementation: %w", spiErr)
				}
				mergeOnChainOutput(&result, spiOut.Output)
			}

			tokenMint := solana.MustPublicKeyFromBase58(tokenAddr.Address)
			for remoteChainSelector, remoteChainConfig := range input.RemoteChains {
				op := tokenpoolops.UpsertRemoteChainConfigBurnMint
				tprl := tokenpoolops.UpsertRateLimitsBurnMint
				switch input.PoolType {
				case common_utils.BurnMintTokenPool.String():
				case common_utils.LockReleaseTokenPool.String():
					op = tokenpoolops.UpsertRemoteChainConfigLockRelease
					tprl = tokenpoolops.UpsertRateLimitsLockRelease
				default:
					return sequences.OnChainOutput{}, unsupportedTokenPoolTypeError(input.PoolType)
				}
				// Store the remote pool address in the width the OffRamp passes to pools: it
				// trims the 32-byte padded message value back to 20 bytes for EVM remotes.
				remoteAddrLen, err := remoteAddressBytesLength(remoteChainSelector)
				if err != nil {
					return sequences.OnChainOutput{}, err
				}
				remotePool, err := solcommon.NormalizeAddressSliceLength(remoteChainConfig.RemotePool, remoteAddrLen)
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("normalize remote pool address for chain %d: %w", remoteChainSelector, err)
				}
				upsertOut, err := operations.ExecuteOperation(b, op, chain,
					tokenpoolops.RemoteChainConfig{
						TokenPool:          tpAddr,
						TokenMint:          tokenMint,
						TokenProgramID:     tokenProgramID,
						RemoteSelector:     remoteChainSelector,
						RemoteTokenAddress: remoteChainConfig.RemoteToken,
						RemoteDecimals:     remoteChainConfig.RemoteDecimals,
						RemotePoolAddress:  remotePool,
					})
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to upsert remote chain config for token pool: %w", err)
				}
				localDecimals, err := solcommon.GetTokenDecimals(chain, tokenMint)
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to get token decimals for token on chain with selector %d: %w", chain.Selector, err)
				}
				var outboundInput, inboundInput tokenapi.RateLimiterConfigFloatInput
				if ob, ok := remoteChainConfig.GetOutboundRateLimitBuckets().DefaultBucket(); ok {
					outboundInput = ob.RateLimit
				}
				if ib, ok := remoteChainConfig.GetInboundRateLimitBuckets().DefaultBucket(); ok {
					inboundInput = ib.RateLimit
				}
				obRL, ibRL := tokenapi.GenerateTPRLConfigs(
					outboundInput,
					inboundInput,
					localDecimals,
					remoteChainConfig.RemoteDecimals,
					chain.Family(),
					solcommon.Version2_0_0,
					input.PoolType,
				)
				mergeOnChainOutput(&result, upsertOut.Output)
				rateLimitOut, err := operations.ExecuteOperation(b, tprl, chain,
					tokenpoolops.RemoteChainConfig{
						TokenPool:                 tpAddr,
						TokenMint:                 tokenMint,
						TokenProgramID:            tokenProgramID,
						RemoteSelector:            remoteChainSelector,
						InboundRateLimiterConfig:  ibRL,
						OutboundRateLimiterConfig: obRL,
					})
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to set rate limits for token pool: %w", err)
				}
				mergeOnChainOutput(&result, rateLimitOut.Output)
			}
			return result, nil
		})
}

// AddressRefToBytes converts an AddressRef to a 32-byte Solana public key.
func (a *SolanaAdapter) AddressRefToBytes(ref datastore.AddressRef) ([]byte, error) {
	return solcommon.ToByteArray(ref)
}

func (a *SolanaAdapter) DeriveTokenAddress(e deployment.Environment, chainSelector uint64, poolRef datastore.AddressRef) (string, error) {
	// A pool config PDA (from ResolveTokenPoolRef's label or the Address field) holds the mint on-chain.
	if chain, ok := e.BlockChains.SolanaChains()[chainSelector]; ok {
		mint, found, err := a.derivePoolMintFromPDA(e.GetContext(), chain, poolRef)
		if err != nil {
			return "", err
		}
		if found {
			return mint.String(), nil
		}
	}
	// Otherwise the token (SPL mint) and pool share the same qualifier (symbol).
	// Look up the token by its qualifier in the datastore.
	tokenRef, err := solcommon.FindLatestRef(e.DataStore, datastore.AddressRef{
		Type:      datastore.ContractType(solcommon.SPLTokens),
		Qualifier: poolRef.Qualifier,
	}, chainSelector)
	if err != nil {
		return "", fmt.Errorf("failed to find token address for pool qualifier %q on chain %d: %w", poolRef.Qualifier, chainSelector, err)
	}
	return tokenRef.Address, nil
}

// DeriveTokenDecimals reads the decimals from the on-chain SPL token mint account.
func (a *SolanaAdapter) DeriveTokenDecimals(e deployment.Environment, chainSelector uint64, poolRef datastore.AddressRef, token []byte) (uint8, error) {
	chain, ok := e.BlockChains.SolanaChains()[chainSelector]
	if !ok {
		return 0, fmt.Errorf("chain with selector %d not defined", chainSelector)
	}
	return solcommon.GetTokenDecimals(chain, solana.PublicKeyFromBytes(token))
}

// DeriveTokenPoolCounterpart derives the token pool config PDA using the token mint and pool program as seeds.
func (a *SolanaAdapter) DeriveTokenPoolCounterpart(e deployment.Environment, chainSelector uint64, tokenPool []byte, token []byte) ([]byte, error) {
	decodedTokenPool := solana.PublicKeyFromBytes(tokenPool)
	decodedToken := solana.PublicKeyFromBytes(token)
	// For Solana, the token pool counterpart is derived using the token mint address and the token pool address as seeds.
	pool, err := tokens.TokenPoolConfigAddress(decodedToken, decodedTokenPool)
	if err != nil {
		return nil, fmt.Errorf("failed to derive token pool counterpart: %w", err)
	}
	return pool.Bytes(), nil
}

// ManualRegistration in Solana registers a token admin registry for a given token and initializes the token pool in CLL Token Pool Program.
// This sequence is used when the owner of the token pool doesn't have the mint authority over the token, but they want to self serve.
// So, this sequence includes the minimum configuration that CCIP Admin needs to do in the Token Admin Registry and in the Token Pool Program
func (a *SolanaAdapter) ManualRegistration() *operations.Sequence[tokenapi.ManualRegistrationSequenceInput, sequences.OnChainOutput, cldf_chain.BlockChains] {
	return operations.NewSequence(
		"svm-adapter:manual-registration",
		solcommon.Version2_0_0,
		"Manually register a token and token pool on Solana Chain",
		func(b operations.Bundle, chains cldf_chain.BlockChains, input tokenapi.ManualRegistrationSequenceInput) (sequences.OnChainOutput, error) {
			var result sequences.OnChainOutput
			chain, ok := chains.SolanaChains()[input.ChainSelector]
			if !ok {
				return sequences.OnChainOutput{}, fmt.Errorf("chain with selector %d not defined", input.ChainSelector)
			}
			proposedOwner, err := validateManualRegistrationInput(input)
			if err != nil {
				return sequences.OnChainOutput{}, err
			}
			needTokenMultisig := input.SVMExtraArgs != nil && len(input.SVMExtraArgs.CustomerMintAuthorities) > 0
			transferMintAuthority := input.SVMExtraArgs != nil && input.SVMExtraArgs.TransferMintAuthority
			skipPoolInit := input.SVMExtraArgs != nil && input.SVMExtraArgs.SkipTokenPoolInit

			// Resolve the token from the datastore first to avoid RPC calls. A customer-deployed
			// token may not be there yet, so fall back to reading it from chain when the ref
			// carries an address.
			var tokenMint, tokenProgram solana.PublicKey
			tokenSymbol := input.TokenRef.Qualifier
			if tokRef, tokProgram, dsErr := solcommon.GetTokenMintAndTokenProgram(input.ExistingDataStore, input.TokenRef, chain); dsErr == nil {
				if tokenMint, err = solana.PublicKeyFromBase58(tokRef.Address); err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to parse token mint %q: %w", tokRef.Address, err)
				}
				tokenProgram = tokProgram
				tokenSymbol = tokRef.Qualifier
			} else {
				b.Logger.Warnf("Token not found in datastore for ref (%s): %v. Falling back to on-chain lookup.", datastore_utils.SprintRef(input.TokenRef), dsErr)
				if input.TokenRef.Address == "" {
					return sequences.OnChainOutput{}, errors.New("token could not be resolved from the datastore and the token ref has no address for an on-chain lookup")
				}
				if tokenMint, err = solana.PublicKeyFromBase58(input.TokenRef.Address); err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to parse token mint %q: %w", input.TokenRef.Address, err)
				}
				if tokenProgram, err = solcommon.FetchTokenProgramID(b.GetContext(), chain, tokenMint); err != nil {
					return sequences.OnChainOutput{}, err
				}
				// The symbol is only needed to label the multisig and the new token ref.
				if needTokenMultisig && tokenSymbol == "" {
					meta, _, metaErr := tokens.GetTokenMetadata(b.GetContext(), chain.Client, tokenMint)
					if metaErr != nil {
						return sequences.OnChainOutput{}, fmt.Errorf("failed to get token metadata for mint %s: %w", tokenMint, metaErr)
					}
					tokenSymbol = meta.Data.Symbol
				}
			}

			// The pool is needed for initialization, the multisig and the mint authority transfer.
			var tokenPoolRef datastore.AddressRef
			var tokenPool solana.PublicKey
			if !skipPoolInit || needTokenMultisig {
				if tokenPoolRef, err = solcommon.FindAndFormatCompatibleRef(input.ExistingDataStore, input.TokenPoolRef, chain.Selector, datastore_utils.FullRef); err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to find token pool address using the specified reference (%+v): %w", input.TokenPoolRef, err)
				}
				if tokenPool, err = solana.PublicKeyFromBase58(tokenPoolRef.Address); err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to parse token pool address %q: %w", tokenPoolRef.Address, err)
				}
			}

			// Check a requested mint authority transfer before any write, so an invalid request
			// fails without leaving a partial registration behind.
			if transferMintAuthority {
				var alreadyTransferred bool
				if alreadyTransferred, err = checkMintAuthorityTransfer(chain, tokenPoolRef, tokenPool, tokenMint, input.ExistingDataStore.Addresses().Filter()); err != nil {
					return sequences.OnChainOutput{}, err
				}
				if alreadyTransferred {
					b.Logger.Infof("Mint authority of %s is already held by its token multisig, skipping the transfer", tokenMint)
					transferMintAuthority = false
				}
			}

			routerAddr, err := a.GetRouterAddress(input.ExistingDataStore, chain.Selector)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to get router address: %w", err)
			}
			router := solana.PublicKeyFromBytes(routerAddr)

			////////////////////////////
			/// Token Admin Registry ///
			////////////////////////////

			// An empty proposed owner makes the CCIP admin the token admin.
			rtarOut, err := operations.ExecuteOperation(b, routerops.RegisterTokenAdminRegistry, chain, routerops.TokenAdminRegistryParams{
				Router:            router,
				TokenMint:         tokenMint,
				Admin:             proposedOwner,
				ExistingAddresses: input.ExistingDataStore.Addresses().Filter(),
			})
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to register token admin registry: %w", err)
			}
			mergeOnChainOutput(&result, rtarOut.Output.OnChainOutput)

			/////////////////////////////
			/// Initialize Token Pool ///
			/////////////////////////////

			if !skipPoolInit {
				initTPOp := tokenpoolops.InitializeBurnMint
				transferOwnershipTPOp := tokenpoolops.TransferOwnershipBurnMint
				switch tokenPoolRef.Type.String() {
				case common_utils.BurnMintTokenPool.String():
					// Already set to burn mint
				case common_utils.LockReleaseTokenPool.String():
					initTPOp = tokenpoolops.InitializeLockRelease
					transferOwnershipTPOp = tokenpoolops.TransferOwnershipLockRelease
				default:
					return sequences.OnChainOutput{}, unsupportedTokenPoolTypeError(tokenPoolRef.Type.String())
				}
				rmnRemoteAddr, err := a.GetRMNRemoteAddress(input.ExistingDataStore, chain.Selector)
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to get RMN remote address: %w", err)
				}

				initTPOut, err := operations.ExecuteOperation(b, initTPOp, chain, tokenpoolops.Params{
					TokenPool:      tokenPool,
					TokenMint:      tokenMint,
					TokenProgramID: tokenProgram,
					Router:         router,
					RMNRemote:      solana.PublicKeyFromBytes(rmnRemoteAddr),
				})
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to initialize token pool: %w", err)
				}
				mergeOnChainOutput(&result, initTPOut.Output.OnChainOutput)

				transferOwnershipOut, err := operations.ExecuteOperation(b, transferOwnershipTPOp, chain, tokenpoolops.TokenPoolTransferOwnershipInput{
					Program:   tokenPool,
					NewOwner:  proposedOwner,
					TokenMint: tokenMint,
				})
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to transfer ownership: %w", err)
				}
				mergeOnChainOutput(&result, transferOwnershipOut.Output)
			}

			/////////////////////////////
			/// Create Token Multisig ///
			/////////////////////////////

			if needTokenMultisig {
				// The multisig will be used as the mint authority (or owner) depending on the pool flow.
				// We include the TokenPoolSigner PDA as one of the multisig signers so the Token Pool Program
				// can "sign" via PDA seeds when it needs to act (PDA signing).
				poolSigner, err := tokens.TokenPoolSignerAddress(tokenMint, tokenPool)
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token pool signer address: %w", err)
				}
				createTokenMultisigOutput, err := operations.ExecuteOperation(b, tokensops.CreateTokenMultisig, chain, tokensops.TokenMultisigParams{
					TokenProgram:      tokenProgram,
					Signers:           multisigSigners(poolSigner, input.SVMExtraArgs.CustomerMintAuthorities),
					TokenMint:         tokenMint,
					TokenSymbol:       tokenSymbol,
					ExistingAddresses: input.ExistingDataStore.Addresses().Filter(),
				})
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to create token multisig on-chain: %w", err)
				}
				mergeOnChainOutput(&result, createTokenMultisigOutput.Output)
				if len(createTokenMultisigOutput.Output.Addresses) == 0 {
					return sequences.OnChainOutput{}, errors.New("create token multisig did not return a multisig address")
				}
				multisig, err := solana.PublicKeyFromBase58(createTokenMultisigOutput.Output.Addresses[0].Address)
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to parse multisig address: %w", err)
				}

				// The pool reaches the multisig through the token's lookup table.
				if _, err = operations.ExecuteOperation(b, tokensops.ExtendTokenPoolLookupTable, chain, tokensops.ExtendTokenPoolLookupTableParams{
					Router:    router,
					TokenMint: tokenMint,
					Accounts:  []solana.PublicKey{multisig},
				}); err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to extend token pool lookup table with multisig: %w", err)
				}

				if transferMintAuthority {
					transferOut, err := operations.ExecuteOperation(b, tokenpoolops.TransferMintAuthorityBurnMint, chain, tokenpoolops.Params{
						TokenPool:        tokenPool,
						TokenMint:        tokenMint,
						TokenProgramID:   tokenProgram,
						NewMintAuthority: multisig,
					})
					if err != nil {
						return sequences.OnChainOutput{}, fmt.Errorf("failed to transfer mint authority: %w", err)
					}
					mergeOnChainOutput(&result, transferOut.Output)
				}
			}

			// A token resolved from chain is added to the datastore, when we know enough to build
			// its full ref and it is not there already.
			if tokenSymbol != "" {
				tokenRef, err := newSolanaTokenRef(input.ChainSelector, tokenMint, tokenProgram, tokenSymbol)
				if err != nil {
					return sequences.OnChainOutput{}, err
				}
				existing := input.ExistingDataStore.Addresses().Filter(
					datastore.AddressRefByChainSelector(tokenRef.ChainSelector),
					datastore.AddressRefByQualifier(tokenRef.Qualifier),
					datastore.AddressRefByAddress(tokenRef.Address),
					datastore.AddressRefByType(tokenRef.Type),
				)
				if len(existing) == 0 {
					b.Logger.Infof("Adding token ref (%s) to the datastore", datastore_utils.SprintRef(tokenRef))
					result.Addresses = append(result.Addresses, tokenRef)
				}
			}

			return result, nil
		})
}

// validateManualRegistrationInput checks the input before anything is read from chain, and
// returns the proposed owner (zero when unset).
func validateManualRegistrationInput(input tokenapi.ManualRegistrationSequenceInput) (solana.PublicKey, error) {
	var proposedOwner solana.PublicKey
	if input.ProposedOwner != "" {
		owner, err := solana.PublicKeyFromBase58(input.ProposedOwner)
		if err != nil {
			return solana.PublicKey{}, fmt.Errorf("failed to parse proposed owner %q: %w", input.ProposedOwner, err)
		}
		proposedOwner = owner
	}
	args := input.SVMExtraArgs
	if (args == nil || !args.SkipTokenPoolInit) && proposedOwner.IsZero() {
		return solana.PublicKey{}, errors.New("proposed owner is required to initialize the token pool")
	}
	if args != nil {
		// The multisig takes the pool signer (MultisigThreshold times) plus the non-zero customer
		// authorities (see multisigSigners), up to MaxMultisigSigners. Check that before any write.
		nonZero := 0
		for _, authority := range args.CustomerMintAuthorities {
			if !authority.IsZero() {
				nonZero++
			}
		}
		if args.TransferMintAuthority && nonZero == 0 {
			return solana.PublicKey{}, errors.New("transfer mint authority is set to true but no non-zero customer mint authorities are provided")
		}
		if len(args.CustomerMintAuthorities) > 0 && nonZero == 0 {
			return solana.PublicKey{}, errors.New("customer mint authorities must contain at least one non-zero key")
		}
		if nonZero > maxCustomerMintAuthorities {
			return solana.PublicKey{}, fmt.Errorf("too many non-zero customer mint authorities: %d > %d", nonZero, maxCustomerMintAuthorities)
		}
	}
	return proposedOwner, nil
}

// checkMintAuthorityTransfer verifies that the pool can hand the mint authority to a multisig:
// it must be an upgradable BurnMint pool whose signer PDA currently holds the mint authority.
// It reports true when a token multisig recorded for the mint already holds the authority.
func checkMintAuthorityTransfer(chain cldf_solana.Chain, poolRef datastore.AddressRef, pool, mint solana.PublicKey, existing []datastore.AddressRef) (bool, error) {
	if poolRef.Type.String() != common_utils.BurnMintTokenPool.String() {
		return false, fmt.Errorf("transfer mint authority is only supported for BurnMint token pools, but got pool type '%s'", poolRef.Type)
	}
	authority, err := solcommon.GetUpgradeAuthority(chain.Client, pool)
	if err != nil {
		return false, fmt.Errorf("failed to get upgrade authority for token pool: %w", err)
	}
	if authority.IsZero() {
		return false, errors.New("transfer mint authority requires an upgradable BurnMint token pool, but it is immutable (no upgrade authority)")
	}
	poolSigner, err := tokens.TokenPoolSignerAddress(mint, pool)
	if err != nil {
		return false, fmt.Errorf("failed to derive token pool signer address: %w", err)
	}
	current, err := solcommon.GetTokenMintAuthority(chain, mint)
	if err != nil {
		return false, err
	}
	if current != poolSigner {
		for _, ref := range existing {
			if ref.ChainSelector == chain.Selector && ref.Type == datastore.ContractType(tokensops.TokenMultisigType) &&
				ref.Address == current.String() && ref.Labels.Contains(mint.String()) {
				return true, nil
			}
		}
		return false, fmt.Errorf("transfer mint authority requires the token mint authority to be the pool signer PDA %s, but got %s; refusing to create an orphan multisig", poolSigner, current)
	}
	return false, nil
}

// unsupportedTokenPoolTypeError rejects pool types a sequence does not handle. CCTP and Lombard
// pools are supported for pool updates (rate limits, remote pools, router and admins, ownership),
// but are deployed and configured for lanes by their own changesets, so the error points there.
func unsupportedTokenPoolTypeError(poolType string) error {
	switch poolType {
	case common_utils.CCTPTokenPool.String():
		return fmt.Errorf("token pool type '%s' is deployed and configured for lanes by DeployCCTPChains, not by this changeset", poolType)
	case tokenpoolops.LombardTokenPool.String():
		return fmt.Errorf("token pool type '%s' is deployed and configured for lanes by DeployLombardChains, not by this changeset", poolType)
	default:
		return fmt.Errorf("unsupported token pool type '%s' for Solana", poolType)
	}
}

// The BurnMint pool requires the pool signer to appear at least m times in the multisig (so the
// pool can mint on its own) and at most n-m times. The pool signer is listed exactly m times, so
// the customer authorities fill the remaining slots up to the SPL token multisig limit.
const maxCustomerMintAuthorities = tokensops.MaxMultisigSigners - int(tokensops.MultisigThreshold)

// multisigSigners lists the pool signer PDA MultisigThreshold times, then the non-zero customer
// authorities.
func multisigSigners(poolSigner solana.PublicKey, customer []solana.PublicKey) []solana.PublicKey {
	signers := make([]solana.PublicKey, 0, int(tokensops.MultisigThreshold)+len(customer))
	for range tokensops.MultisigThreshold {
		signers = append(signers, poolSigner)
	}
	for _, s := range customer {
		if !s.IsZero() {
			signers = append(signers, s)
		}
	}
	return signers
}

// newSolanaTokenRef builds the datastore ref for a token mint.
func newSolanaTokenRef(chainSelector uint64, mint, tokenProgram solana.PublicKey, symbol string) (datastore.AddressRef, error) {
	tokenType, err := solcommon.GetTokenProgramType(tokenProgram)
	if err != nil {
		return datastore.AddressRef{}, err
	}
	return datastore.AddressRef{
		ChainSelector: chainSelector,
		Qualifier:     symbol,
		Address:       mint.String(),
		Version:       tokensops.Version,
		Labels:        datastore.NewLabelSet(),
		Type:          datastore.ContractType(tokenType),
	}, nil
}

// SetTokenPoolRateLimits configures inbound/outbound rate limits on a Solana token pool.
// It dispatches to the BurnMint, LockRelease, CCTP or Lombard operation based on the pool type.
func (a *SolanaAdapter) SetTokenPoolRateLimits() *operations.Sequence[tokenapi.TPRLRemotes, sequences.OnChainOutput, cldf_chain.BlockChains] {
	return operations.NewSequence(
		"SetTokenPoolRateLimits",
		solcommon.Version2_0_0,
		"Sets rate limits for a token pool on Solana",
		func(b operations.Bundle, chains cldf_chain.BlockChains, input tokenapi.TPRLRemotes) (sequences.OnChainOutput, error) {
			chain := chains.SolanaChains()[input.ChainSelector]

			// The CCTP operation takes its own input type, so it is run separately below.
			op := tokenpoolops.UpsertRateLimitsBurnMint
			isCctp := false
			switch input.TokenPoolRef.Type.String() {
			case common_utils.BurnMintTokenPool.String():
			case common_utils.LockReleaseTokenPool.String():
				op = tokenpoolops.UpsertRateLimitsLockRelease
			case tokenpoolops.LombardTokenPool.String():
				op = tokenpoolops.UpsertRateLimitsLombard
			case common_utils.CCTPTokenPool.String():
				isCctp = true
			default:
				return sequences.OnChainOutput{}, unsupportedTokenPoolTypeError(input.TokenPoolRef.Type.String())
			}
			tokenAddr, tokenProgramID, err := solcommon.GetTokenMintAndTokenProgram(input.ExistingDataStore, input.TokenRef, chain)
			if err != nil {
				return sequences.OnChainOutput{}, err
			}

			tokenMint := solana.MustPublicKeyFromBase58(tokenAddr.Address)
			tokenPool := solana.MustPublicKeyFromBase58(input.TokenPoolRef.Address)
			cfg := tokenpoolops.RemoteChainConfig{
				TokenPool:                 tokenPool,
				TokenMint:                 tokenMint,
				TokenProgramID:            tokenProgramID,
				RemoteSelector:            input.RemoteChainSelector,
				InboundRateLimiterConfig:  input.InboundRateLimiterConfig,
				OutboundRateLimiterConfig: input.OutboundRateLimiterConfig,
			}
			var rateLimitOut sequences.OnChainOutput
			if isCctp {
				report, err := operations.ExecuteOperation(b, tokenpoolops.UpsertRateLimitsCctp, chain, tokenpoolops.CctpRemoteChainConfig{RemoteChainConfig: cfg})
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to set rate limits for token pool: %w", err)
				}
				rateLimitOut = report.Output
			} else {
				report, err := operations.ExecuteOperation(b, op, chain, cfg)
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to set rate limits for token pool: %w", err)
				}
				rateLimitOut = report.Output
			}
			return sequences.OnChainOutput{
				BatchOps: rateLimitOut.BatchOps,
			}, nil
		},
	)
}

// DeployToken creates an SPL (or SPL2022) token mint, associated token accounts, and optionally uploads on-chain metadata.
func (a *SolanaAdapter) DeployToken() *operations.Sequence[tokenapi.DeployTokenInput, sequences.OnChainOutput, cldf_chain.BlockChains] {
	return operations.NewSequence(
		"DeployToken",
		solcommon.Version2_0_0,
		"Deploys a token contract on Solana",
		func(b operations.Bundle, chains cldf_chain.BlockChains, input tokenapi.DeployTokenInput) (sequences.OnChainOutput, error) {
			var result sequences.OnChainOutput
			b.Logger.Info("SVM Deploying token:", input)
			chain := chains.SolanaChains()[input.ChainSelector]
			needsTokenDeploy := true
			var rawTokenAddr string

			tokenAddr, err := solcommon.FindLatestRef(input.ExistingDataStore, datastore.AddressRef{
				Type:      datastore.ContractType(input.Type),
				Qualifier: input.Symbol,
			}, chain.Selector)
			if err == nil {
				b.Logger.Info("Token already deployed at address:", tokenAddr.Address)
				needsTokenDeploy = false
				rawTokenAddr = tokenAddr.Address
				// Return the existing ref so callers (e.g. token expansion) get the token ref on a
				// re-run, making the sequence idempotent. Same as the EVM adapter's behavior.
				result.Addresses = append(result.Addresses, tokenAddr)
			}
			if needsTokenDeploy {
				var privateKey solana.PrivateKey
				if input.TokenPrivKey != "" {
					privateKey = solana.MustPrivateKeyFromBase58(input.TokenPrivKey)
				}
				ataList := make([]solana.PublicKey, 0, len(input.Senders))
				for _, sender := range input.Senders {
					ataList = append(ataList, solana.MustPublicKeyFromBase58(sender))
				}
				// tokenapi.DeployTokenInput uses *uint64 for PreMint, nil means no premint (0).
				var premint uint64
				if input.PreMint != nil {
					premint = *input.PreMint
				}
				deployOut, err1 := operations.ExecuteOperation(b, tokensops.DeploySolanaToken, chain, tokensops.Params{
					ExistingAddresses:      input.ExistingDataStore.Addresses().Filter(),
					TokenProgramName:       input.Type,
					TokenPrivKey:           privateKey,
					TokenSymbol:            input.Symbol,
					ATAList:                ataList,
					PreMint:                premint,
					DisableFreezeAuthority: input.DisableFreezeAuthority,
					TokenDecimals:          input.Decimals,
				})
				if err1 != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to deploy token: %w", err1)
				}
				result.Addresses = append(result.Addresses, deployOut.Output)
				rawTokenAddr = deployOut.Output.Address
			}
			// irrespective of whether the token was just deployed or already existed, we attempt to upload metadata if it was provided, since the metadata might not have been uploaded in a previous deployment
			if input.TokenMetadata != nil {
				input.TokenMetadata.TokenPubkey = rawTokenAddr
				_, err = operations.ExecuteOperation(b, tokensops.UpsertTokenMetadata, chain, tokensops.TokenMetadataInput{
					ExistingAddresses: input.ExistingDataStore.Addresses().Filter(),
					Metadata:          *input.TokenMetadata,
				})
				if err != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to upload token metadata: %w", err)
				}
			}
			return result, nil
		},
	)
}

// DeployTokenVerify is a no-op for Solana; token deployment is verified implicitly by subsequent operations.
func (a *SolanaAdapter) DeployTokenVerify(e deployment.Environment, in tokenapi.DeployTokenInput) error {
	return nil
}

// DeployTokenPoolForToken initializes a token pool account (not a new program), creates the pool signer PDA's ATA,
// and transfers mint authority to the pool signer for BurnMint pools.
func (a *SolanaAdapter) DeployTokenPoolForToken() *operations.Sequence[tokenapi.DeployTokenPoolInput, sequences.OnChainOutput, cldf_chain.BlockChains] {
	return operations.NewSequence(
		"DeployTokenPoolForToken",
		solcommon.Version2_0_0,
		"Configures a token pool for a given token on Solana. Doesn't actually deploy a new token pool contract, as Solana token pools are just accounts.",
		func(b operations.Bundle, chains cldf_chain.BlockChains, input tokenapi.DeployTokenPoolInput) (sequences.OnChainOutput, error) {
			var result sequences.OnChainOutput
			b.Logger.Info("SVM Deploying token pool:", input)
			chain := chains.SolanaChains()[input.ChainSelector]

			op := tokenpoolops.InitializeBurnMint
			switch input.PoolType {
			case common_utils.BurnMintTokenPool.String():
			case common_utils.LockReleaseTokenPool.String():
				op = tokenpoolops.InitializeLockRelease
			default:
				return sequences.OnChainOutput{}, unsupportedTokenPoolTypeError(input.PoolType)
			}
			tokenAddr, tokenProgramID, err := solcommon.GetTokenMintAndTokenProgram(input.ExistingDataStore, *input.TokenRef, chain)
			if err != nil {
				return sequences.OnChainOutput{}, err
			}

			// On Solana, token pool programs are pre-deployed and loaded into the validator
			// (via env.toml) and registered in the datastore during PostDeployContractsForSelector
			// with the "program" qualifier. Look up the program address by pool type + version.
			poolProgramRef, err := solcommon.FindAndFormatCompatibleRef(input.ExistingDataStore, datastore.AddressRef{
				ChainSelector: chain.Selector,
				Type:          datastore.ContractType(input.PoolType),
				Version:       input.TokenPoolVersion,
				Qualifier:     solcommon.PoolProgramQualifier,
			}, chain.Selector, datastore_utils.FullRef)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to find pool program for type %q version %s in datastore: %w", input.PoolType, input.TokenPoolVersion, err)
			}
			tokenPool := solana.MustPublicKeyFromBase58(poolProgramRef.Address)
			routerAddr, err := a.GetRouterAddress(input.ExistingDataStore, input.ChainSelector)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to get router address: %w", err)
			}
			rmnRemoteAddr, err := a.GetRMNRemoteAddress(input.ExistingDataStore, input.ChainSelector)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to get RMN remote address: %w", err)
			}

			tokenMint := solana.MustPublicKeyFromBase58(tokenAddr.Address)

			deployOut, err := operations.ExecuteOperation(b, op, chain, tokenpoolops.Params{
				TokenPool:      tokenPool,
				TokenMint:      tokenMint,
				TokenProgramID: tokenProgramID,
				Router:         solana.PublicKeyFromBytes(routerAddr),
				RMNRemote:      solana.PublicKeyFromBytes(rmnRemoteAddr),
			})
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to initialize token pool: %w", err)
			}
			mergeOnChainOutput(&result, deployOut.Output.OnChainOutput)

			// Register the pool instance in the output so the framework adds it to the
			// datastore. Downstream steps (ConfigureTokenForTransfers, etc.) look it up by
			// qualifier + type.
			result.Addresses = append(result.Addresses, datastore.AddressRef{
				Address:       tokenPool.String(),
				ChainSelector: chain.Selector,
				Type:          datastore.ContractType(input.PoolType),
				Version:       input.TokenPoolVersion,
				Qualifier:     input.TokenPoolQualifier,
			})

			poolSigner, err := tokens.TokenPoolSignerAddress(tokenMint, tokenPool)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to derive token pool signer address: %w", err)
			}

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

			atas := make([]ATA, 0)
			// ATA for token pool
			atas = append(atas, ataFrom(tokenProgramID, tokenMint, poolSigner))

			linkMint, err := findLinkMint(input.ExistingDataStore.Addresses().Filter(), input.ChainSelector)
			if err != nil {
				return sequences.OnChainOutput{}, err
			}
			fees, err := feeTokens(b.GetContext(), chain, linkMint)
			if err != nil {
				return sequences.OnChainOutput{}, err
			}
			for _, ft := range fees {
				vaultPda, _, aerr := state.FindFeeVaultPDA(tokenPool, tokenMint.Bytes())
				if aerr != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to find pool fee vault: %w", aerr)
				}
				atas = append(atas, ataFrom(ft.program, ft.mint, vaultPda))
			}

			for _, ata := range atas {
				ixn, _, ataErr := tokens.CreateAssociatedTokenAccount(
					ata.tokenProgram,
					ata.mint,
					ata.authority,
					chain.Deployer().PublicKey(),
				)
				if ataErr != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to create the instruction to create associated token account for tokenpool (mint: %s, pool: %s, ata/vaultAta: %s): %w", tokenMint.String(), tokenPool.String(), ata.ata.String(), ataErr)
				}
				if confirmErr := chain.Confirm([]solana.Instruction{ixn}); confirmErr != nil {
					return sequences.OnChainOutput{}, fmt.Errorf("failed to create associated token account for tokenpool (mint: %s, pool: %s, ata/vaultATA: %s): %w", tokenMint.String(), tokenPool.String(), ata.ata.String(), confirmErr)
				}
			}

			mintAuthority, err := solcommon.GetTokenMintAuthority(chain, tokenMint)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to get token mint authority for %s: %w", tokenMint, err)
			}
			// make pool mint_authority for token, if necessary
			if input.PoolType == common_utils.BurnMintTokenPool.String() && tokenMint != solana.WrappedSol {
				if mintAuthority.String() == chain.Deployer().PublicKey().String() {
					ixn, err := tokens.SetTokenMintAuthority(
						tokenProgramID,
						poolSigner,
						tokenMint,
						chain.Deployer().PublicKey(),
					)
					if err != nil {
						return sequences.OnChainOutput{}, fmt.Errorf("failed to generate instructions: %w", err)
					}
					if err := chain.Confirm([]solana.Instruction{ixn}); err != nil {
						return sequences.OnChainOutput{}, fmt.Errorf("failed to set mint authority: %w", err)
					}
					b.Logger.Infow("Setting mint authority", "poolSigner", poolSigner.String())
				} else {
					b.Logger.Warnw("Token's mint authority is not with deployer key, skipping setting poolSigner as mint authority",
						"poolType", input.PoolType, "tokenMint", tokenMint,
						"deployer", chain.Deployer().PublicKey().String(), "poolSigner", poolSigner.String())
				}
			} else {
				b.Logger.Warnw("PoolType is not a BurnMintTokenPool, skipping setting poolSigner as mint authority",
					"poolType", input.PoolType, "tokenMint", tokenMint,
					"deployer", chain.Deployer().PublicKey().String(), "poolSigner", poolSigner.String())
			}

			return result, nil
		},
	)
}

// UpdateAuthorities transfers token pool ownership and rate-limit admin to the timelock signer PDA.
func (a *SolanaAdapter) UpdateAuthorities() *operations.Sequence[tokenapi.UpdateAuthoritiesInput, sequences.OnChainOutput, *deployment.Environment] {
	return operations.NewSequence(
		"svm-adapter:update-authorities",
		solcommon.Version2_0_0,
		"Update authorities for a token and token pool on Solana Chain",
		func(b operations.Bundle, e *deployment.Environment, input tokenapi.UpdateAuthoritiesInput) (sequences.OnChainOutput, error) {
			var result sequences.OnChainOutput
			chain, ok := e.BlockChains.SolanaChains()[input.ChainSelector]
			if !ok {
				return sequences.OnChainOutput{}, fmt.Errorf("chain with selector %d not defined", input.ChainSelector)
			}
			ds := e.DataStore
			tokenRef, _, err := solcommon.GetTokenMintAndTokenProgram(ds, input.TokenRef, chain)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to get token and token program address using the specified reference (%+v): %w", input.TokenRef, err)
			}

			timelockSigner, err := solcommon.GetTimelockSignerPDA(
				ds.Addresses().Filter(),
				chain.Selector,
				common_utils.CLLQualifier,
			)
			if err != nil {
				return sequences.OnChainOutput{}, err
			}

			tokenPoolRef, err := solcommon.FindAndFormatCompatibleRef(ds, input.TokenPoolRef, chain.Selector, datastore_utils.FullRef)
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to find token pool address using the specified reference (%+v): %w", input.TokenPoolRef, err)
			}
			tokenMint := solana.MustPublicKeyFromBase58(tokenRef.Address)
			tokenPool := solana.MustPublicKeyFromBase58(tokenPoolRef.Address)

			transferOwnershipTPOp := tokenpoolops.TransferOwnershipBurnMint
			acceptOwnershipTPOp := tokenpoolops.AcceptOwnershipBurnMint
			tprlAdminTPOp := tokenpoolops.UpdateRateLimitAdminBurnMint
			switch tokenPoolRef.Type.String() {
			case common_utils.BurnMintTokenPool.String():
				// Already set to burn mint
			case common_utils.LockReleaseTokenPool.String():
				transferOwnershipTPOp = tokenpoolops.TransferOwnershipLockRelease
				acceptOwnershipTPOp = tokenpoolops.AcceptOwnershipLockRelease
				tprlAdminTPOp = tokenpoolops.UpdateRateLimitAdminLockRelease
			case common_utils.CCTPTokenPool.String():
				transferOwnershipTPOp = tokenpoolops.TransferOwnershipCctp
				acceptOwnershipTPOp = tokenpoolops.AcceptOwnershipCctp
				tprlAdminTPOp = tokenpoolops.UpdateRateLimitAdminCctp
			case tokenpoolops.LombardTokenPool.String():
				transferOwnershipTPOp = tokenpoolops.TransferOwnershipLombard
				acceptOwnershipTPOp = tokenpoolops.AcceptOwnershipLombard
				tprlAdminTPOp = tokenpoolops.UpdateRateLimitAdminLombard
			default:
				return sequences.OnChainOutput{}, unsupportedTokenPoolTypeError(tokenPoolRef.Type.String())
			}

			tprlAdminOut, err := operations.ExecuteOperation(b, tprlAdminTPOp, chain, tokenpoolops.TokenPoolTransferOwnershipInput{
				Program:   tokenPool,
				TokenMint: tokenMint,
				NewOwner:  timelockSigner,
			})
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to update TPRL admin: %w", err)
			}
			mergeOnChainOutput(&result, tprlAdminOut.Output)

			b.Logger.Infof("Transferring ownership of token pool %s to timelock signer %s", tokenPool.String(), timelockSigner.String())
			transferOwnershipOut, err := operations.ExecuteOperation(b, transferOwnershipTPOp, chain, tokenpoolops.TokenPoolTransferOwnershipInput{
				Program:   tokenPool,
				NewOwner:  timelockSigner,
				TokenMint: tokenMint,
			})
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to transfer ownership: %w", err)
			}
			mergeOnChainOutput(&result, transferOwnershipOut.Output)

			b.Logger.Infof("Accepting ownership of token pool %s by timelock signer %s", tokenPool.String(), timelockSigner.String())
			acceptOwnershipOut, err := operations.ExecuteOperation(b, acceptOwnershipTPOp, chain, tokenpoolops.TokenPoolTransferOwnershipInput{
				Program:   tokenPool,
				NewOwner:  timelockSigner,
				TokenMint: tokenMint,
			})
			if err != nil {
				return sequences.OnChainOutput{}, fmt.Errorf("failed to accept ownership: %w", err)
			}
			mergeOnChainOutput(&result, acceptOwnershipOut.Output)

			return result, nil
		})
}

// MigrateLockReleasePoolLiquiditySequence is not yet implemented for Solana.
func (a *SolanaAdapter) MigrateLockReleasePoolLiquiditySequence() *operations.Sequence[tokenapi.MigrateLockReleasePoolLiquidityInput, sequences.OnChainOutput, cldf_chain.BlockChains] {
	return operations.NewSequence(
		"svm-adapter:migrate-lockrelease-pool-liquidity",
		solcommon.Version2_0_0,
		"Migrate lock-release pool liquidity on Solana",
		func(b operations.Bundle, chains cldf_chain.BlockChains, input tokenapi.MigrateLockReleasePoolLiquidityInput) (sequences.OnChainOutput, error) {
			return sequences.OnChainOutput{}, errors.New("MigrateLockReleasePoolLiquiditySequence not yet implemented for Solana")
		},
	)
}

func mergeOnChainOutput(dst *sequences.OnChainOutput, src sequences.OnChainOutput) {
	dst.Addresses = append(dst.Addresses, src.Addresses...)
	dst.BatchOps = append(dst.BatchOps, src.BatchOps...)
}

// remoteAddressBytesLength returns the address width of the remote chain's family, as reported by
// its registered chain family adapter (the same source the lane config's AddressBytesLength uses).
func remoteAddressBytesLength(remoteSelector uint64) (uint8, error) {
	family, err := chainsel.GetSelectorFamily(remoteSelector)
	if err != nil {
		return 0, fmt.Errorf("get chain family for selector %d: %w", remoteSelector, err)
	}
	remoteAdapter, ok := ccvadapters.GetChainFamilyRegistry().GetChainFamily(family)
	if !ok {
		return 0, fmt.Errorf("no chain family adapter registered for %s (selector %d)", family, remoteSelector)
	}
	return remoteAdapter.GetAddressBytesLength(), nil
}
