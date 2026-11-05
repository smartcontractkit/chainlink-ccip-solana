package devenv

import (
	"context"
	"errors"
	"fmt"

	"github.com/gagliardetto/solana-go"
	computebudget "github.com/gagliardetto/solana-go/programs/compute-budget"
	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-ccv/build/devenv/cciptestinterfaces"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/ccip_router"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/example_ccip_sender"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/latest/example_ccip_sender_v2"
	"github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/ccip"
	solcommon "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/common"
	solstate "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/state"
	soltokens "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
	deploysolcommon "github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
	testsenderops "github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/testsender"
)

// TestSenderV2Address resolves the v2 example sender devenv preloads and configures.
func (c *Chain) TestSenderV2Address() (solana.PublicKey, error) {
	return deploysolcommon.GetDataStoreAddress(c.e.DataStore, datastore.NewAddressRefKey(
		c.chainDetails.ChainSelector,
		datastore.ContractType(testsenderops.ContractType),
		testsenderops.Version,
		DefaultSenderQualifier,
	))
}

// SendViaTestSenderV2 sends data to dest through the v2 example sender's ccip_send, with the
// deployer paying the fee in native SOL. The router bills at most maxFee lamports. The recipient
// and extra args are the ones stored in the sender's config for dest. Returns the router's sent
// event and the transaction signature.
//
// tokenAmount, when set, is transferred from the deployer's token account. The sender pulls it
// with its ccip_sender PDA as the transfer authority, so the deployer delegates the amount to that
// PDA in the same transaction; the PDA's own token account for the mint is created first if
// missing.
func (c *Chain) SendViaTestSenderV2(ctx context.Context, dest uint64, data []byte, tokenAmount *example_ccip_sender_v2.SvmTokenAmount, maxFee uint64) (cciptestinterfaces.MessageSentEvent, solana.Signature, error) {
	signer := c.chain.DeployerKey
	if signer == nil {
		return cciptestinterfaces.MessageSentEvent{}, solana.Signature{}, fmt.Errorf("no deployer key for chain %d", c.chainDetails.ChainSelector)
	}
	sender, err := c.TestSenderV2Address()
	if err != nil {
		return cciptestinterfaces.MessageSentEvent{}, solana.Signature{}, fmt.Errorf("resolve test sender v2: %w", err)
	}
	routerProgramID, err := routerPubkeyFromDataStore(c.e.DataStore, c.chainDetails.ChainSelector, "")
	if err != nil {
		return cciptestinterfaces.MessageSentEvent{}, solana.Signature{}, err
	}

	// The CPI wraps ccip_send_v2, so it needs at least the router's own budget.
	instructions := []solana.Instruction{
		computebudget.NewSetComputeUnitLimitInstruction(computebudget.MAX_COMPUTE_UNIT_LIMIT).Build(),
	}
	var transferMint solana.PublicKey
	if tokenAmount != nil {
		transferMint = tokenAmount.Token
		approveIx, approveErr := c.prepareTestSenderV2Transfer(ctx, sender, *signer, *tokenAmount)
		if approveErr != nil {
			return cciptestinterfaces.MessageSentEvent{}, solana.Signature{}, approveErr
		}
		instructions = append(instructions, approveIx)
	}

	// The zero fee token pays in native SOL; fees are still quoted against wrapped SOL, which is
	// the mint the resolver derives the fee accounts from.
	resolved, err := ccip.ResolveExampleSenderV2SendAccounts(
		ctx,
		c.chain.Client,
		sender,
		routerProgramID,
		example_ccip_sender_v2.CcipSendParams{
			DestChainSelector: dest,
			TokenAmount:       tokenAmount,
			Data:              data,
			FeeToken:          solana.PublicKey{},
			MaxFee:            maxFee,
		},
		solana.WrappedSol,
		transferMint,
		signer.PublicKey(),
		c.ccipLggr,
	)
	if err != nil {
		return cciptestinterfaces.MessageSentEvent{}, solana.Signature{}, fmt.Errorf("resolve test sender v2 ccip_send accounts: %w", err)
	}
	lutMap, err := c.sendLookupTables(ctx, resolved.LookupTables)
	if err != nil {
		return cciptestinterfaces.MessageSentEvent{}, solana.Signature{}, err
	}

	instructions = append(instructions, resolved.Ix)
	result, err := solcommon.SendAndConfirmWithLookupTablesAndRetries(ctx, c.chain.Client,
		instructions, *signer, rpc.CommitmentConfirmed, lutMap)
	if err != nil {
		return cciptestinterfaces.MessageSentEvent{}, solana.Signature{}, fmt.Errorf("send and confirm test sender v2 ccip_send: %w", err)
	}
	tx, err := result.Transaction.GetTransaction()
	if err != nil {
		return cciptestinterfaces.MessageSentEvent{}, solana.Signature{}, fmt.Errorf("get transaction: %w", err)
	}
	txSig := tx.Signatures[0]

	event, err := solcommon.ExtractCPIEvent(tx, result.Meta, routerProgramID, ccip_router.ParseEvent_CcipMessageSentV2)
	if err != nil {
		return cciptestinterfaces.MessageSentEvent{}, solana.Signature{}, fmt.Errorf("extract router CPI event: %w", err)
	}
	c.logger.Info().
		Uint64("dest", dest).
		Hex("messageID", event.MessageId[:]).
		Str("sender", sender.String()).
		Str("txSignature", txSig.String()).
		Msg("Solana: CCIPMessageSentV2 emitted via example sender v2")

	sent, err := messageSentEvent(event)
	if err != nil {
		return cciptestinterfaces.MessageSentEvent{}, solana.Signature{}, err
	}
	return sent, txSig, nil
}

// prepareTestSenderV2Transfer creates the v2 sender's ccip_sender token account for the transferred
// mint if missing, and returns the instruction delegating the amount from owner's token account to
// the ccip_sender PDA, which the sender pulls the tokens with.
func (c *Chain) prepareTestSenderV2Transfer(ctx context.Context, sender solana.PublicKey, owner solana.PrivateKey, tokenAmount example_ccip_sender_v2.SvmTokenAmount) (solana.Instruction, error) {
	tokenProgram, err := testsenderops.TokenProgramForMint(ctx, c.chain, tokenAmount.Token)
	if err != nil {
		return nil, err
	}
	bundle := operations.NewBundle(func() context.Context { return ctx }, c.e.Logger, operations.NewMemoryReporter())
	if _, err = operations.ExecuteOperation(bundle, testsenderops.InitCcipSenderATA, c.chain, testsenderops.InitCcipSenderATAParams{
		Sender:        sender,
		Mint:          tokenAmount.Token,
		SenderVersion: testsenderops.Version,
	}); err != nil {
		return nil, fmt.Errorf("init test sender v2 token account for mint %s: %w", tokenAmount.Token, err)
	}
	ccipSenderPDA, err := testsenderops.FindCcipSenderPDA(sender)
	if err != nil {
		return nil, err
	}
	ownerATA, _, err := soltokens.FindAssociatedTokenAddress(tokenProgram, tokenAmount.Token, owner.PublicKey())
	if err != nil {
		return nil, fmt.Errorf("derive token account of %s for mint %s: %w", owner.PublicKey(), tokenAmount.Token, err)
	}
	// Decimals only: a mint's total supply can exceed math.MaxInt64, which TokenSupply cannot parse.
	decimals, err := soltokens.TokenDecimals(ctx, c.chain.Client, tokenAmount.Token, rpc.CommitmentConfirmed)
	if err != nil {
		return nil, fmt.Errorf("get decimals of mint %s: %w", tokenAmount.Token, err)
	}
	ix, err := soltokens.TokenApproveChecked(tokenAmount.Amount, decimals, tokenProgram, ownerATA, tokenAmount.Token, ccipSenderPDA, owner.PublicKey(), nil)
	if err != nil {
		return nil, fmt.Errorf("build delegation to test sender v2 ccip_sender PDA: %w", err)
	}
	return ix, nil
}

// TestSenderV1Address resolves the v1 example sender devenv preloads and configures.
func (c *Chain) TestSenderV1Address() (solana.PublicKey, error) {
	return deploysolcommon.GetDataStoreAddress(c.e.DataStore, datastore.NewAddressRefKey(
		c.chainDetails.ChainSelector,
		datastore.ContractType(testsenderops.ContractType),
		testsenderops.LegacyVersion,
		DefaultSenderQualifier,
	))
}

// SendViaTestSenderV1 sends data to dest through the v1 example sender's ccip_send, which wraps the
// router's 1.6 ccip_send and pays the fee in native SOL. The recipient is the one stored in the
// sender's config for dest. Returns the router's legacy sent event and the transaction signature.
//
// The lane must exist on the router's 1.6 interface (see routerops.AddLegacyDestChain). Messages
// sent this way are not picked up by the CCIP 2.0 offchain stack.
func (c *Chain) SendViaTestSenderV1(ctx context.Context, dest uint64, data []byte) (*ccip_router.CcipMessageSent, solana.Signature, error) {
	signer := c.chain.DeployerKey
	if signer == nil {
		return nil, solana.Signature{}, fmt.Errorf("no deployer key for chain %d", c.chainDetails.ChainSelector)
	}
	sender, err := c.TestSenderV1Address()
	if err != nil {
		return nil, solana.Signature{}, fmt.Errorf("resolve test sender v1: %w", err)
	}
	routerProgramID, err := routerPubkeyFromDataStore(c.e.DataStore, c.chainDetails.ChainSelector, "")
	if err != nil {
		return nil, solana.Signature{}, err
	}
	accounts, err := c.testSenderV1SendAccounts(ctx, sender, routerProgramID, dest, signer.PublicKey())
	if err != nil {
		return nil, solana.Signature{}, err
	}

	// The sender reads the caller's fee token account even when paying in native SOL, so it must
	// exist up front.
	if err = c.ensureAssociatedTokenAccount(ctx, *signer, solana.TokenProgramID, solana.WrappedSol); err != nil {
		return nil, solana.Signature{}, err
	}

	ix, err := example_ccip_sender.NewCcipSendInstruction(
		dest,
		[]example_ccip_sender.SvmTokenAmount{},
		data,
		solana.PublicKey{},
		[]uint8{},
		accounts.state,
		accounts.chainConfig,
		accounts.ccipSender,
		accounts.callerFeeTokenAccount,
		signer.PublicKey(),
		solana.SystemProgramID,
		routerProgramID,
		accounts.routerConfig,
		accounts.destChainState,
		accounts.nonce,
		solana.TokenProgramID,
		solana.WrappedSol,
		accounts.senderFeeTokenAccount,
		accounts.feeTokenReceiver,
		accounts.feeBillingSigner,
		accounts.feeQuoter,
		accounts.feeQuoterConfig,
		accounts.feeQuoterDestChain,
		accounts.feeTokenBillingConfig,
		accounts.linkBillingConfig,
		accounts.rmnRemote,
		accounts.rmnRemoteCurses,
		accounts.rmnRemoteConfig,
	)
	if err != nil {
		return nil, solana.Signature{}, fmt.Errorf("build test sender v1 ccip_send: %w", err)
	}

	// The sender quotes the fee through a get_fee CPI before the send CPI, so it needs more than
	// the default budget.
	instructions := []solana.Instruction{
		computebudget.NewSetComputeUnitLimitInstruction(computebudget.MAX_COMPUTE_UNIT_LIMIT).Build(),
		ix,
	}
	result, err := solcommon.SendAndConfirmWithLookupTablesAndRetries(ctx, c.chain.Client,
		instructions, *signer, rpc.CommitmentConfirmed, map[solana.PublicKey]solana.PublicKeySlice{})
	if err != nil {
		return nil, solana.Signature{}, fmt.Errorf("send and confirm test sender v1 ccip_send: %w", err)
	}
	tx, err := result.Transaction.GetTransaction()
	if err != nil {
		return nil, solana.Signature{}, fmt.Errorf("get transaction: %w", err)
	}
	txSig := tx.Signatures[0]

	event, err := solcommon.ParseEventFromLogs(result.Meta.LogMessages, ccip_router.ParseEvent_CcipMessageSent)
	if err != nil {
		return nil, solana.Signature{}, fmt.Errorf("extract router legacy sent event: %w", err)
	}
	c.logger.Info().
		Uint64("dest", dest).
		Hex("messageID", event.Message.Header.MessageId[:]).
		Str("sender", sender.String()).
		Str("txSignature", txSig.String()).
		Msg("Solana: CCIPMessageSent emitted via example sender v1")
	return event, txSig, nil
}

// testSenderV1Accounts are the accounts the v1 sender's ccip_send names, other than the caller,
// the programs and the fee mint.
type testSenderV1Accounts struct {
	state, chainConfig, ccipSender                 solana.PublicKey
	callerFeeTokenAccount, senderFeeTokenAccount   solana.PublicKey
	routerConfig, destChainState, nonce            solana.PublicKey
	feeTokenReceiver, feeBillingSigner             solana.PublicKey
	feeQuoter, feeQuoterConfig, feeQuoterDestChain solana.PublicKey
	feeTokenBillingConfig, linkBillingConfig       solana.PublicKey
	rmnRemote, rmnRemoteCurses, rmnRemoteConfig    solana.PublicKey
}

// testSenderV1SendAccounts derives the v1 sender's ccip_send accounts for a native SOL fee. The
// fee quoter, RMN remote and LINK mint come from the router's config.
func (c *Chain) testSenderV1SendAccounts(ctx context.Context, sender, router solana.PublicKey, dest uint64, caller solana.PublicKey) (testSenderV1Accounts, error) {
	var a testSenderV1Accounts
	var err error

	if a.state, _, err = solana.FindProgramAddress([][]byte{[]byte("state")}, sender); err != nil {
		return a, fmt.Errorf("derive test sender v1 state PDA: %w", err)
	}
	if a.chainConfig, _, err = solana.FindProgramAddress(
		[][]byte{[]byte("remote_chain_config"), solcommon.Uint64ToLE(dest)}, sender); err != nil {
		return a, fmt.Errorf("derive test sender v1 chain config PDA: %w", err)
	}
	if a.ccipSender, err = testsenderops.FindCcipSenderPDA(sender); err != nil {
		return a, err
	}

	if a.routerConfig, _, err = solstate.FindConfigPDA(router); err != nil {
		return a, fmt.Errorf("derive router config PDA: %w", err)
	}
	var routerConfig ccip_router.Config
	if err = solcommon.GetAccountDataBorshIntoAnchor(ctx, c.chain.Client, a.routerConfig, rpc.CommitmentConfirmed, &routerConfig); err != nil {
		return a, fmt.Errorf("read router config: %w", err)
	}
	if a.destChainState, err = solstate.FindDestChainStatePDA(dest, router); err != nil {
		return a, fmt.Errorf("derive legacy dest chain state PDA: %w", err)
	}
	if a.nonce, err = solstate.FindNoncePDA(dest, a.ccipSender, router); err != nil {
		return a, fmt.Errorf("derive nonce PDA: %w", err)
	}
	if a.feeBillingSigner, _, err = solstate.FindFeeBillingSignerPDA(router); err != nil {
		return a, fmt.Errorf("derive fee billing signer PDA: %w", err)
	}

	for _, ata := range []struct {
		owner solana.PublicKey
		out   *solana.PublicKey
	}{
		{caller, &a.callerFeeTokenAccount},
		{a.ccipSender, &a.senderFeeTokenAccount},
		{a.feeBillingSigner, &a.feeTokenReceiver},
	} {
		if *ata.out, _, err = soltokens.FindAssociatedTokenAddress(solana.TokenProgramID, solana.WrappedSol, ata.owner); err != nil {
			return a, fmt.Errorf("derive wSOL token account of %s: %w", ata.owner, err)
		}
	}

	a.feeQuoter = routerConfig.FeeQuoter
	if a.feeQuoterConfig, _, err = solstate.FindFqConfigPDA(a.feeQuoter); err != nil {
		return a, fmt.Errorf("derive fee quoter config PDA: %w", err)
	}
	if a.feeQuoterDestChain, _, err = solstate.FindFqDestChainPDA(dest, a.feeQuoter); err != nil {
		return a, fmt.Errorf("derive fee quoter dest chain PDA: %w", err)
	}
	if a.feeTokenBillingConfig, _, err = solstate.FindFqBillingTokenConfigPDA(solana.WrappedSol, a.feeQuoter); err != nil {
		return a, fmt.Errorf("derive wSOL billing config PDA: %w", err)
	}
	if a.linkBillingConfig, _, err = solstate.FindFqBillingTokenConfigPDA(routerConfig.LinkTokenMint, a.feeQuoter); err != nil {
		return a, fmt.Errorf("derive LINK billing config PDA: %w", err)
	}

	a.rmnRemote = routerConfig.RmnRemote
	if a.rmnRemoteCurses, _, err = solstate.FindRMNRemoteCursesPDA(a.rmnRemote); err != nil {
		return a, fmt.Errorf("derive RMN remote curses PDA: %w", err)
	}
	if a.rmnRemoteConfig, _, err = solstate.FindRMNRemoteConfigPDA(a.rmnRemote); err != nil {
		return a, fmt.Errorf("derive RMN remote config PDA: %w", err)
	}
	return a, nil
}

// ensureAssociatedTokenAccount creates owner's associated token account for mint unless it exists.
func (c *Chain) ensureAssociatedTokenAccount(ctx context.Context, owner solana.PrivateKey, tokenProgram, mint solana.PublicKey) error {
	ata, _, err := soltokens.FindAssociatedTokenAddress(tokenProgram, mint, owner.PublicKey())
	if err != nil {
		return fmt.Errorf("derive token account of %s for mint %s: %w", owner.PublicKey(), mint, err)
	}
	// A System-owned account at the ATA address only holds lamports someone sent there, and the
	// associated token program still initializes it.
	info, err := c.chain.Client.GetAccountInfo(ctx, ata)
	switch {
	case err == nil && info.Value.Owner.Equals(tokenProgram):
		return nil
	case err == nil && !info.Value.Owner.Equals(solana.SystemProgramID):
		return fmt.Errorf("token account %s is owned by %s, not %s", ata, info.Value.Owner, tokenProgram)
	case err != nil && !errors.Is(err, rpc.ErrNotFound):
		return fmt.Errorf("get token account %s: %w", ata, err)
	}
	ix, _, err := soltokens.CreateAssociatedTokenAccount(tokenProgram, mint, owner.PublicKey(), owner.PublicKey())
	if err != nil {
		return fmt.Errorf("build create token account instruction: %w", err)
	}
	if _, err = solcommon.SendAndConfirmWithLookupTablesAndRetries(ctx, c.chain.Client,
		[]solana.Instruction{ix}, owner, rpc.CommitmentConfirmed, map[solana.PublicKey]solana.PublicKeySlice{}); err != nil {
		return fmt.Errorf("create token account %s: %w", ata, err)
	}
	return nil
}

// LegacyDestChainExists reports whether the router has a 1.6 lane to dest, which the v1 example
// sender needs.
func (c *Chain) LegacyDestChainExists(ctx context.Context, dest uint64) (bool, error) {
	routerProgramID, err := routerPubkeyFromDataStore(c.e.DataStore, c.chainDetails.ChainSelector, "")
	if err != nil {
		return false, err
	}
	destChainState, err := solstate.FindDestChainStatePDA(dest, routerProgramID)
	if err != nil {
		return false, fmt.Errorf("derive legacy dest chain state PDA: %w", err)
	}
	if _, err = c.chain.Client.GetAccountInfo(ctx, destChainState); err != nil {
		if errors.Is(err, rpc.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("get legacy dest chain state %s: %w", destChainState, err)
	}
	return true, nil
}
