package devenv

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gagliardetto/solana-go"

	"github.com/smartcontractkit/chainlink-deployments-framework/deployment"

	message_transmitter_v2 "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/cctp_message_transmitter_v2"
	token_messenger_minter_v2 "github.com/smartcontractkit/chainlink-ccip-solana/contracts/gobindings/v2/latest/cctp_token_messenger_minter_v2"
	utilscctp "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/cctp"
	soltokens "github.com/smartcontractkit/chainlink-ccip-solana/contracts/utils/tokens"
)

// CCTPAttesterPrivateKey is the devenv-only Circle attester key. The Solana MessageTransmitterV2
// accepts only attestations that this key signs, so the e2e tests sign fake attestations with it.
const CCTPAttesterPrivateKey = "8f2a55949038a9610f50fb23b5883af3b4ecb3c3bb792cbcefbd1542c692be63"

// cctpCustodyFunding is the USDC the devenv moves into Circle's custody account. Circle's Solana
// TokenMessengerMinterV2 releases inbound transfers from custody.
const cctpCustodyFunding uint64 = 100_000 * 1e6

// setupSolanaCircleCCTPV2 initializes the vendored Circle CCTP V2 programs for one EVM remote,
// like contracts/tests/ccip/cctp_tokenpool_test.go. The deployer is the admin, the upgrade
// authority, and the token controller.
func setupSolanaCircleCCTPV2(
	env *deployment.Environment,
	selector uint64,
	localDomain, remoteDomain uint32,
	usdcMint solana.PublicKey,
	remoteTokenMessenger, remoteUSDC common.Address,
) error {
	solChain, ok := env.BlockChains.SolanaChains()[selector]
	if !ok {
		return fmt.Errorf("solana chain not found for selector %d", selector)
	}
	deployer := solChain.DeployerKey.PublicKey()

	mt, err := utilscctp.GetMessageTransmitterV2PDAs()
	if err != nil {
		return err
	}
	tmm, err := utilscctp.GetTokenMessengerMinterV2PDAs(remoteDomain, usdcMint, remoteUSDC.Bytes())
	if err != nil {
		return err
	}
	programData := func(program solana.PublicKey) (solana.PublicKey, error) {
		pda, _, pdaErr := solana.FindProgramAddress([][]byte{program.Bytes()}, solana.BPFLoaderUpgradeableProgramID)
		return pda, pdaErr
	}
	mtProgramData, err := programData(mt.Program)
	if err != nil {
		return fmt.Errorf("derive message transmitter program data: %w", err)
	}
	tmmProgramData, err := programData(tmm.Program)
	if err != nil {
		return fmt.Errorf("derive token messenger minter program data: %w", err)
	}

	attesterKey, err := crypto.HexToECDSA(CCTPAttesterPrivateKey)
	if err != nil {
		return fmt.Errorf("parse CCTP attester key: %w", err)
	}
	attester := solana.PublicKeyFromBytes(common.LeftPadBytes(crypto.PubkeyToAddress(attesterKey.PublicKey).Bytes(), 32))

	deployerATA, _, err := soltokens.FindAssociatedTokenAddress(solana.TokenProgramID, usdcMint, deployer)
	if err != nil {
		return fmt.Errorf("derive deployer USDC account: %w", err)
	}

	steps := []struct {
		name  string
		build func() (solana.Instruction, error)
	}{
		{"initialize message transmitter", func() (solana.Instruction, error) {
			return message_transmitter_v2.NewInitializeInstruction(
				message_transmitter_v2.InitializeParams{
					LocalDomain:        localDomain,
					Attester:           attester,
					MaxMessageBodySize: 8192,
					Version:            1,
				},
				deployer, deployer, mt.MessageTransmitter, mtProgramData, mt.Program,
				solana.SystemProgramID, mt.EventAuthority, mt.Program,
			)
		}},
		{"initialize token messenger minter", func() (solana.Instruction, error) {
			return token_messenger_minter_v2.NewInitializeInstruction(
				token_messenger_minter_v2.InitializeParams{
					TokenController:    deployer,
					MinFee:             0,
					MessageBodyVersion: 1,
					FeeRecipient:       deployer,
					// Unused in the devenv, but the program rejects the zero address.
					Denylister:       solana.SysVarClockPubkey,
					MinFeeController: solana.SysVarClockPubkey,
				},
				deployer, deployer, tmm.AuthorityPda, tmm.TokenMessenger, tmm.TokenMinter, tmmProgramData,
				tmm.Program, solana.SystemProgramID, tmm.EventAuthority, tmm.Program,
			)
		}},
		{"add remote token messenger", func() (solana.Instruction, error) {
			return token_messenger_minter_v2.NewAddRemoteTokenMessengerInstruction(
				token_messenger_minter_v2.AddRemoteTokenMessengerParams{
					Domain:         remoteDomain,
					TokenMessenger: solana.PublicKeyFromBytes(common.LeftPadBytes(remoteTokenMessenger.Bytes(), 32)),
				},
				deployer, deployer, tmm.TokenMessenger, tmm.RemoteTokenMessenger,
				solana.SystemProgramID, tmm.EventAuthority, tmm.Program,
			)
		}},
		{"add local token", func() (solana.Instruction, error) {
			return token_messenger_minter_v2.NewAddLocalTokenInstruction(
				token_messenger_minter_v2.AddLocalTokenParams{},
				deployer, deployer, tmm.TokenMinter, tmm.LocalToken, tmm.CustodyTokenAccount, usdcMint,
				solana.TokenProgramID, solana.SystemProgramID, tmm.EventAuthority, tmm.Program,
			)
		}},
		{"link token pair", func() (solana.Instruction, error) {
			return token_messenger_minter_v2.NewLinkTokenPairInstruction(
				token_messenger_minter_v2.LinkTokenPairParams{
					LocalToken:   tmm.LocalToken,
					RemoteDomain: remoteDomain,
					RemoteToken:  solana.PublicKeyFromBytes(common.LeftPadBytes(remoteUSDC.Bytes(), 32)),
				},
				deployer, deployer, tmm.TokenMinter, tmm.TokenPair,
				solana.SystemProgramID, tmm.EventAuthority, tmm.Program,
			)
		}},
		{"set max burn amount per message", func() (solana.Instruction, error) {
			return token_messenger_minter_v2.NewSetMaxBurnAmountPerMessageInstruction(
				token_messenger_minter_v2.SetMaxBurnAmountPerMessageParams{BurnLimitPerMessage: cctpCustodyFunding},
				deployer, tmm.TokenMinter, tmm.LocalToken, tmm.EventAuthority, tmm.Program,
			)
		}},
		{"fund custody account", func() (solana.Instruction, error) {
			return soltokens.TokenTransferChecked(cctpCustodyFunding, USDCDecimals, solana.TokenProgramID,
				deployerATA, usdcMint, tmm.CustodyTokenAccount, deployer, nil)
		}},
	}
	for _, step := range steps {
		ix, buildErr := step.build()
		if buildErr != nil {
			return fmt.Errorf("build %s on chain %d: %w", step.name, selector, buildErr)
		}
		if err = solChain.Confirm([]solana.Instruction{ix}); err != nil {
			return fmt.Errorf("%s on chain %d: %w", step.name, selector, err)
		}
	}
	return nil
}
