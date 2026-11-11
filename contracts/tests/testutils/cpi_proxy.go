// cpi_proxy.go provides helper functions to build instructions that invoke programs
// via the arbitrary-cpi-proxy program. This proxy signs CPI calls with a PDA derived
// from caller-provided seed bytes.
//
// # Purpose
//
// In tests, we often need to simulate cross-program invocations (CPIs) that would
// normally be signed by privileged PDAs (e.g., the offramp calling token pools).
// The arbitrary-cpi-proxy program allows tests to make these calls by:
//  1. Deriving a signer PDA from caller-provided seed bytes
//  2. Signing the inner instruction with that PDA
//  3. Forwarding the call to the target program
//
// # Creating New Proxy Methods
//
// To add a new proxy helper for a different instruction:
//
//  1. Define the seed components:
//     var mySeedPrefix = []byte("my_seed_prefix")
//
//  2. Create a function following this pattern:
//
//     func NewMyProxyInstruction(
//     params MyInstructionParams,   // instruction parameters
//     targetProgram solana.PublicKey, // program to call
//     cpiSigner solana.PublicKey,     // the PDA that will sign
//     // ... other accounts required by the target instruction
//     ) (solana.Instruction, error) {
//     // Serialize the instruction data (discriminator + args)
//     ixData, err := serializeIxData(target_program.Instruction_MyInstruction, params)
//     if err != nil {
//     return nil, fmt.Errorf("serialize params: %w", err)
//     }
//
//     ix, err := arbitrary_cpi_proxy.NewExecuteCpiInstruction(
//     ixData,                                            // serialized instruction data
//     mySeedPrefix,                                      // seed prefix for PDA derivation
//     targetProgram,                                     // target program account (also used as second PDA seed)
//     )
//     if err != nil {
//     return nil, err
//     }
//
//     // Append target instruction's accounts as remaining_accounts.
//     // Include cpiSigner at the position the target program expects.
//     generic := ix.(*solana.GenericInstruction)
//     generic.AccountValues = append(generic.AccountValues,
//     solana.Meta(cpiSigner),         // PDA signer at position expected by target
//     solana.Meta(account1).WRITE(),  // mark writable if needed
//     solana.Meta(account2),           // read-only
//     // ... remaining accounts
//     )
//
//     return ix, nil
//     }
//
//  3. The signer PDA is derived as: PDA(seeds...) under ArbitraryCpiProxyProgram.
//     Use state.FindAllowedOfframpPDA and friends to derive PDAs for setup.
package testutils

import (
	"bytes"
	"fmt"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/arbitrary_cpi_proxy"
	tp "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/burnmint_token_pool"
	ccip_receiver "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/example_ccip_receiver"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/lombard_token_pool"
)

var (
	poolSeedPrefix     = []byte("external_token_pools_signer")
	receiverSeedPrefix = []byte("external_execution_config")
	ccvSeedPrefix      = []byte("ccv_signer")
)

// serializeIxData serializes an Anchor instruction discriminator + borsh-encoded args.
func serializeIxData(discriminator [8]byte, args any) ([]byte, error) {
	buf := new(bytes.Buffer)
	enc := bin.NewBorshEncoder(buf)
	if _, err := buf.Write(discriminator[:]); err != nil {
		return nil, fmt.Errorf("write discriminator: %w", err)
	}
	if err := enc.Encode(args); err != nil {
		return nil, fmt.Errorf("encode args: %w", err)
	}
	return buf.Bytes(), nil
}

// NewPoolProxyLockOrBurnInstruction builds an ExecuteCpi instruction that calls
// lock_or_burn_tokens on a token pool program, signing with the pool signer PDA.
//
// The remaining_accounts follow the on-chain TokenOnramp struct order:
//
//	cpi_signer, state, token_program, mint, pool_signer, pool_token_account,
//	rmn_remote, rmn_remote_curses, rmn_remote_config, chain_config
func NewPoolProxyLockOrBurnInstruction(
	lockOrBurn tp.LockOrBurnInV1,
	poolProgram solana.PublicKey,
	cpiSigner solana.PublicKey,
	poolState solana.PublicKey,
	tokenProgram solana.PublicKey,
	mint solana.PublicKey,
	poolSigner solana.PublicKey,
	poolTokenAccount solana.PublicKey,
	rmnRemote solana.PublicKey,
	rmnRemoteCurses solana.PublicKey,
	rmnRemoteConfig solana.PublicKey,
	chainConfig solana.PublicKey,
) (solana.Instruction, error) {
	ixData, err := serializeIxData(tp.Instruction_LockOrBurnTokens, lockOrBurn)
	if err != nil {
		return nil, fmt.Errorf("serialize LockOrBurnInV1: %w", err)
	}

	ix, err := arbitrary_cpi_proxy.NewExecuteCpiInstruction(
		ixData,
		poolSeedPrefix,
		poolProgram,
	)
	if err != nil {
		return nil, err
	}

	generic, ok := ix.(*solana.GenericInstruction)
	if !ok {
		return nil, fmt.Errorf("expected *solana.GenericInstruction, got %T", ix)
	}
	generic.AccountValues = append(generic.AccountValues,
		solana.Meta(cpiSigner),
		solana.Meta(poolState).WRITE(),
		solana.Meta(tokenProgram),
		solana.Meta(mint).WRITE(),
		solana.Meta(poolSigner),
		solana.Meta(poolTokenAccount).WRITE(),
		solana.Meta(rmnRemote),
		solana.Meta(rmnRemoteCurses),
		solana.Meta(rmnRemoteConfig),
		solana.Meta(chainConfig).WRITE(),
	)

	return ix, nil
}

// NewPoolProxyLockOrBurnV2Instruction builds an ExecuteCpi instruction that calls
// lock_or_burn_tokens_v2 on a token pool program, signing with the pool signer PDA.
//
// `feeVaultTokenAccount` is the ATA, for the pool's own mint, of the pool's ["fee_vault", mint]
// PDA, which burnmint and lockrelease take the in-token BPS fee into. Pass the zero pubkey on a
// pool that has no such account in its onramp context (cctp), as the account order has to mirror
// the pool's `TokenOnrampV2` struct exactly.
func NewPoolProxyLockOrBurnV2Instruction(
	lockOrBurn any, // use `any` to avoid casting between cctp/bnm/lnr's equivalent LockOrBurnInV2
	poolProgram solana.PublicKey,
	cpiSigner solana.PublicKey,
	poolState solana.PublicKey,
	tokenProgram solana.PublicKey,
	mint solana.PublicKey,
	poolSigner solana.PublicKey,
	poolTokenAccount solana.PublicKey,
	chainConfig solana.PublicKey,
	chainConfigV2 solana.PublicKey,
	chainConfigOverrides solana.PublicKey,
	feeVaultTokenAccount solana.PublicKey,
	eventAuthority solana.PublicKey,
) (solana.Instruction, error) {
	ixData, err := serializeIxData(tp.Instruction_LockOrBurnTokensV2, lockOrBurn)
	if err != nil {
		return nil, fmt.Errorf("serialize LockOrBurnInV2: %w", err)
	}

	ix, err := arbitrary_cpi_proxy.NewExecuteCpiInstruction(
		ixData,
		poolSeedPrefix,
		poolProgram,
	)
	if err != nil {
		return nil, err
	}

	generic, ok := ix.(*solana.GenericInstruction)
	if !ok {
		return nil, fmt.Errorf("expected *solana.GenericInstruction, got %T", ix)
	}
	generic.AccountValues = append(generic.AccountValues,
		solana.Meta(cpiSigner),
		solana.Meta(poolState).WRITE(),
		solana.Meta(tokenProgram),
		solana.Meta(mint).WRITE(),
		solana.Meta(poolSigner),
		solana.Meta(poolTokenAccount).WRITE(),
		solana.Meta(chainConfig).WRITE(),
		solana.Meta(chainConfigV2),
		solana.Meta(chainConfigOverrides).WRITE(),
	)
	if !feeVaultTokenAccount.IsZero() {
		generic.AccountValues = append(generic.AccountValues,
			solana.Meta(feeVaultTokenAccount).WRITE(),
		)
	}
	generic.AccountValues = append(generic.AccountValues,
		solana.Meta(eventAuthority), // for emit_cpi!
		solana.Meta(poolProgram),    // for emit_cpi!
	)

	return ix, nil
}

// NewPoolProxyReleaseOrMintInstruction builds an ExecuteCpi instruction that calls
// release_or_mint_tokens on a token pool program, signing with the pool signer PDA.
//
// The remaining_accounts follow the on-chain TokenOfframp struct order:
//
//	cpi_signer, offramp_program, allowed_offramp, state, token_program, mint,
//	pool_signer, pool_token_account, chain_config,
//	rmn_remote, rmn_remote_curses, rmn_remote_config, receiver_token_account
func NewPoolProxyReleaseOrMintInstruction(
	releaseOrMint tp.ReleaseOrMintInV1,
	poolProgram solana.PublicKey,
	cpiSigner solana.PublicKey,
	offrampProgram solana.PublicKey,
	allowedOfframp solana.PublicKey,
	poolState solana.PublicKey,
	tokenProgram solana.PublicKey,
	mint solana.PublicKey,
	poolSigner solana.PublicKey,
	poolTokenAccount solana.PublicKey,
	chainConfig solana.PublicKey,
	rmnRemote solana.PublicKey,
	rmnRemoteCurses solana.PublicKey,
	rmnRemoteConfig solana.PublicKey,
	receiverTokenAccount solana.PublicKey,
) (solana.Instruction, error) {
	ixData, err := serializeIxData(tp.Instruction_ReleaseOrMintTokens, releaseOrMint)
	if err != nil {
		return nil, fmt.Errorf("serialize ReleaseOrMintInV1: %w", err)
	}

	ix, err := arbitrary_cpi_proxy.NewExecuteCpiInstruction(
		ixData,
		poolSeedPrefix,
		poolProgram,
	)
	if err != nil {
		return nil, err
	}

	generic, ok := ix.(*solana.GenericInstruction)
	if !ok {
		return nil, fmt.Errorf("expected *solana.GenericInstruction, got %T", ix)
	}
	generic.AccountValues = append(generic.AccountValues,
		solana.Meta(cpiSigner),
		solana.Meta(offrampProgram),
		solana.Meta(allowedOfframp),
		solana.Meta(poolState).WRITE(),
		solana.Meta(tokenProgram),
		solana.Meta(mint).WRITE(),
		solana.Meta(poolSigner),
		solana.Meta(poolTokenAccount).WRITE(),
		solana.Meta(chainConfig).WRITE(),
		solana.Meta(rmnRemote),
		solana.Meta(rmnRemoteCurses),
		solana.Meta(rmnRemoteConfig),
		solana.Meta(receiverTokenAccount).WRITE(),
	)

	return ix, nil
}

// NewPoolProxyReleaseOrMintV2Instruction builds an ExecuteCpi instruction that calls
// release_or_mint_tokens_v2 on a token pool program, signing with the pool signer PDA.
//
// The remaining_accounts follow the on-chain TokenOfframp struct order
func NewPoolProxyReleaseOrMintV2Instruction(
	releaseOrMint any, // use `any` to avoid casting between cctp/bnm/lnr's equivalent ReleaseOrMintInV2
	poolProgram solana.PublicKey,
	cpiSigner solana.PublicKey,
	offrampProgram solana.PublicKey,
	allowedOfframp solana.PublicKey,
	poolState solana.PublicKey,
	tokenProgram solana.PublicKey,
	mint solana.PublicKey,
	poolSigner solana.PublicKey,
	poolTokenAccount solana.PublicKey,
	chainConfig solana.PublicKey,
	chainConfigOverride solana.PublicKey,
	receiverTokenAccount solana.PublicKey,
	eventAuthority solana.PublicKey,
) (solana.Instruction, error) {
	ixData, err := serializeIxData(tp.Instruction_ReleaseOrMintTokensV2, releaseOrMint)
	if err != nil {
		return nil, fmt.Errorf("serialize ReleaseOrMintInV2: %w", err)
	}

	ix, err := arbitrary_cpi_proxy.NewExecuteCpiInstruction(
		ixData,
		poolSeedPrefix,
		poolProgram,
	)
	if err != nil {
		return nil, err
	}

	generic, ok := ix.(*solana.GenericInstruction)
	if !ok {
		return nil, fmt.Errorf("expected *solana.GenericInstruction, got %T", ix)
	}
	generic.AccountValues = append(generic.AccountValues,
		solana.Meta(cpiSigner),
		solana.Meta(offrampProgram),
		solana.Meta(allowedOfframp),
		solana.Meta(poolState).WRITE(),
		solana.Meta(tokenProgram),
		solana.Meta(mint).WRITE(),
		solana.Meta(poolSigner),
		solana.Meta(poolTokenAccount).WRITE(),
		solana.Meta(chainConfig).WRITE(),
		solana.Meta(chainConfigOverride).WRITE(),
		solana.Meta(receiverTokenAccount).WRITE(),
		solana.Meta(eventAuthority), // for emit_cpi!
		solana.Meta(poolProgram),    // for emit_cpi!
	)

	return ix, nil
}

// NewVerifierVerifyInstruction builds an ExecuteCpi instruction that calls lombard_token_pool's
// `verify` instruction, signing with the ccv_signer PDA - standing in for the real offramp's
// `call_verify`, which signs with the same [ccv_signer, ccv_program] PDA under its own program id.
//
// The remaining_accounts follow the on-chain Verify struct order:
//
//	verifier_config, remote_chain_config, cpi_signer, offramp_program, allowed_offramp, state,
//	token_program, mint, pool_signer, receiver_token_account, mint_authority, token_authority,
//	bridge, bridge_config, mailbox, mailbox_config, local_token_config, system_program,
//	remote_bridge_config, remote_token_config, chain_config, inbound_message_path, message_info,
//	message_handled
func NewVerifierVerifyInstruction(
	params lombard_token_pool.VerifyParams,
	verifierProgram solana.PublicKey,
	cpiSigner solana.PublicKey,
	verifierConfig solana.PublicKey,
	remoteChainConfig solana.PublicKey,
	offrampProgram solana.PublicKey,
	allowedOfframp solana.PublicKey,
	state solana.PublicKey,
	tokenProgram solana.PublicKey,
	mint solana.PublicKey,
	poolSigner solana.PublicKey,
	receiverTokenAccount solana.PublicKey,
	mintAuthority solana.PublicKey,
	tokenAuthority solana.PublicKey,
	bridge solana.PublicKey,
	bridgeConfig solana.PublicKey,
	mailbox solana.PublicKey,
	mailboxConfig solana.PublicKey,
	localTokenConfig solana.PublicKey,
	systemProgram solana.PublicKey,
	remoteBridgeConfig solana.PublicKey,
	remoteTokenConfig solana.PublicKey,
	chainConfig solana.PublicKey,
	inboundMessagePath solana.PublicKey,
	messageInfo solana.PublicKey,
	messageHandled solana.PublicKey,
) (solana.Instruction, error) {
	ixData, err := serializeIxData(lombard_token_pool.Instruction_Verify, params)
	if err != nil {
		return nil, fmt.Errorf("serialize VerifyParams: %w", err)
	}

	ix, err := arbitrary_cpi_proxy.NewExecuteCpiInstruction(
		ixData,
		ccvSeedPrefix,
		verifierProgram,
	)
	if err != nil {
		return nil, err
	}

	generic, ok := ix.(*solana.GenericInstruction)
	if !ok {
		return nil, fmt.Errorf("expected *solana.GenericInstruction, got %T", ix)
	}
	generic.AccountValues = append(generic.AccountValues,
		solana.Meta(verifierConfig).WRITE(),
		solana.Meta(remoteChainConfig),
		solana.Meta(cpiSigner),
		solana.Meta(offrampProgram),
		solana.Meta(allowedOfframp),
		solana.Meta(state),
		solana.Meta(tokenProgram),
		solana.Meta(mint).WRITE(),
		solana.Meta(poolSigner).WRITE(),
		solana.Meta(receiverTokenAccount).WRITE(),
		solana.Meta(mintAuthority),
		solana.Meta(tokenAuthority),
		solana.Meta(bridge),
		solana.Meta(bridgeConfig),
		solana.Meta(mailbox),
		solana.Meta(mailboxConfig),
		solana.Meta(localTokenConfig),
		solana.Meta(systemProgram),
		solana.Meta(remoteBridgeConfig),
		solana.Meta(remoteTokenConfig).WRITE(),
		solana.Meta(chainConfig),
		solana.Meta(inboundMessagePath),
		solana.Meta(messageInfo).WRITE(),
		solana.Meta(messageHandled).WRITE(),
	)

	return ix, nil
}

// NewReceiverProxyExecuteInstruction builds an ExecuteCpi instruction that calls
// ccip_receive on a receiver program, signing with the external execution config PDA.
//
// The remaining_accounts follow the on-chain struct order:
//
//	cpi_signer, offramp_program, allowed_offramp, [remaining receiver-specific accounts via Append]
func NewReceiverProxyExecuteInstruction(
	message ccip_receiver.Any2SvmMessage,
	receiverProgram solana.PublicKey,
	cpiSigner solana.PublicKey,
	offrampProgram solana.PublicKey,
	allowedOfframp solana.PublicKey,
) (solana.Instruction, error) {
	ixData, err := serializeIxData(ccip_receiver.Instruction_CcipReceive, message)
	if err != nil {
		return nil, fmt.Errorf("serialize Any2SvmMessage: %w", err)
	}

	ix, err := arbitrary_cpi_proxy.NewExecuteCpiInstruction(
		ixData,
		receiverSeedPrefix,
		receiverProgram,
	)
	if err != nil {
		return nil, err
	}

	generic, ok := ix.(*solana.GenericInstruction)
	if !ok {
		return nil, fmt.Errorf("expected *solana.GenericInstruction, got %T", ix)
	}
	generic.AccountValues = append(generic.AccountValues,
		solana.Meta(cpiSigner),
		solana.Meta(offrampProgram),
		solana.Meta(allowedOfframp),
	)

	return ix, nil
}
