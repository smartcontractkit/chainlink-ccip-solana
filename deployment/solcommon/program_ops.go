package solcommon

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Masterminds/semver/v3"
	solbinary "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	solrpc "github.com/gagliardetto/solana-go/rpc"

	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/datastore"
	cldf_deployment "github.com/smartcontractkit/chainlink-deployments-framework/deployment"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"
)

func GetSolProgramSize(chain cldf_solana.Chain, programID solana.PublicKey) (int, error) {
	accountInfo, err := chain.Client.GetAccountInfoWithOpts(context.Background(), programID, &solrpc.GetAccountInfoOpts{
		Commitment: cldf_solana.SolDefaultCommitment,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get account info: %w", err)
	}
	if accountInfo == nil || accountInfo.Value == nil {
		return 0, fmt.Errorf("program account not found: %s", programID.String())
	}
	programBytes := len(accountInfo.Value.Data.GetBinary())
	return programBytes, nil
}

func GetSolProgramData(client *solrpc.Client, programID solana.PublicKey) (struct {
	DataType uint32
	Address  solana.PublicKey
}, error) {
	var programData struct {
		DataType uint32
		Address  solana.PublicKey
	}
	data, err := client.GetAccountInfoWithOpts(context.Background(), programID, &solrpc.GetAccountInfoOpts{
		Commitment: solrpc.CommitmentConfirmed,
	})
	if err != nil {
		return programData, fmt.Errorf("failed to get program account info: %w", err)
	}
	if data == nil || data.Value == nil {
		return programData, fmt.Errorf("program account does not exist: %s", programID.String())
	}

	err = solbinary.UnmarshalBorsh(&programData, data.Bytes())
	if err != nil {
		return programData, fmt.Errorf("failed to unmarshal program data: %w", err)
	}
	return programData, nil
}

func DeployContractIfNotInDatastore(
	b operations.Bundle,
	chain cldf_solana.Chain,
	input []datastore.AddressRef,
	contractType cldf_deployment.ContractType,
	contractVersion *semver.Version,
	contractQualifier string,
	programName string,
) (datastore.AddressRef, error) {
	// Any ref on the same major counts, so a datastore holding only 2.0.1 does not deploy a second program.
	var existing *datastore.AddressRef
	for i := range input {
		ref := input[i]
		if ref.Type != datastore.ContractType(contractType) || !RefVersionMatches(ref.Version, contractVersion) {
			continue
		}
		if contractQualifier != "" && ref.Qualifier != contractQualifier {
			continue
		}
		if existing == nil || ref.Version.GreaterThan(existing.Version) {
			existing = &input[i]
		}
	}
	if existing != nil {
		b.Logger.Infof("Using existing %s at %s (version %s)", contractType, existing.Address, existing.Version)
		return *existing, nil
	}
	// Security.txt, IDL and a hashable artifact are preconditions for deploying,
	// not optional follow-up steps. Enforced here so no path can skip them.
	artifact, err := RequireDeployable(chain, programName)
	if err != nil {
		return datastore.AddressRef{}, err
	}
	b.Logger.Infow("Deploying verified artifact",
		"program", programName,
		"executableHash", artifact.ExecutableHash,
		"securityTxt", artifact.HasSecurityTxt,
		"idl", artifact.IDLPath)

	// The IDL declares the deployed version; it must be on the requested major.
	var idlVersion *semver.Version
	if artifact.IDLPath != "" {
		if idlVersion, err = IDLVersion(artifact.IDLPath); err != nil {
			return datastore.AddressRef{}, err
		}
		if !RefVersionMatches(idlVersion, contractVersion) {
			return datastore.AddressRef{}, fmt.Errorf("refusing to deploy %s: its IDL declares version %s but %s was requested",
				programName, idlVersion, contractVersion)
		}
	}

	// The deploy gets its address from the program keypair, so the keypair is
	// mandatory here and the artifact must match the address it derives.
	// Upgrades do not need it: they reuse the existing address.
	keypairID, err := DeployKeypairID(chain.ProgramsPath, programName)
	if err != nil {
		return datastore.AddressRef{}, err
	}
	if err = RequireArtifactForID(chain.ProgramsPath, programName, keypairID); err != nil {
		return datastore.AddressRef{}, err
	}

	programID, err := chain.DeployProgram(b.Logger, cldf_solana.ProgramInfo{
		Name: programName,
	}, false, false)
	if err != nil {
		return datastore.AddressRef{}, err
	}
	// validate deployed programID
	deployedID := solana.MustPublicKeyFromBase58(programID)
	b.Logger.Infof("Deployed %s at %s", contractType, programID)

	// Confirm the bytes on chain are the bytes we meant to ship. A mismatch
	// here means the deploy did not land the artifact we inspected above.
	if err = VerifyDeployedProgram(chain.Client, deployedID, artifact.ExecutableHash); err != nil {
		return datastore.AddressRef{}, err
	}
	b.Logger.Infow("Verified deployed program matches artifact",
		"program", programName, "address", programID, "executableHash", artifact.ExecutableHash)

	// Publish the verified-build record and the IDL. A freshly deployed program
	// is still deployer-owned, so both execute directly; ownership moves to the
	// timelock later, after which upgrades go through a proposal instead.
	batches, err := RecordDeployment(b, chain, deployedID, programName)
	if err != nil {
		return datastore.AddressRef{}, err
	}
	if len(batches) > 0 {
		// Nothing here can carry a proposal, and dropping one would leave the
		// program deployed but unrecorded.
		return datastore.AddressRef{}, fmt.Errorf(
			"deploying %s produced %d operation(s) needing the upgrade authority, "+
				"which means it is already governed and should be upgraded rather than deployed",
			programName, len(batches))
	}

	// Programs without an IDL are recorded at the requested version.
	deployedVersion := contractVersion
	if artifact.IDLPath != "" {
		deployedVersion = idlVersion
	}
	ref := datastore.AddressRef{
		Address:       programID,
		ChainSelector: chain.Selector,
		Qualifier:     contractQualifier,
		Type:          datastore.ContractType(contractType),
		Version:       deployedVersion,
	}
	// Record the hash alongside the address. A Solana upgrade keeps the same
	// program ID, so the address alone cannot tell you which binary is running.
	ref.Labels.Add(ExecutableHashLabel(artifact.ExecutableHash))

	return ref, nil
}

// UpgradeableLoaderState mirrors the Rust enum in the Solana SDK.
type UpgradeableLoaderState struct {
	Type          uint32
	Program       *Program
	ProgramData   *ProgramData
	Uninitialized bool
}

// Program holds the address of the ProgramData account.
type Program struct {
	ProgramData solana.PublicKey
}

// ProgramData holds the optional UpgradeAuthority.
type ProgramData struct {
	Slot            uint64
	AuthorityOption uint32 // 0 = none, 1 = present
	Authority       solana.PublicKey
}

func decodeUpgradeableLoaderState(data []byte) (*UpgradeableLoaderState, error) {
	if len(data) < 4 {
		return nil, errors.New("data too short")
	}
	state := &UpgradeableLoaderState{}
	state.Type = binary.LittleEndian.Uint32(data[:4])

	switch state.Type {
	case 2: // Program
		if len(data) < 36 {
			return nil, errors.New("program data too short")
		}
		state.Program = &Program{
			ProgramData: solana.PublicKeyFromBytes(data[4:36]),
		}
	case 3: // ProgramData
		slot := binary.LittleEndian.Uint64(data[4:12])
		opt := data[12]
		var auth *solana.PublicKey
		if opt == 1 {
			if len(data) < 45 {
				return nil, errors.New("missing authority pubkey")
			}
			pk := solana.PublicKeyFromBytes(data[13:45])
			auth = &pk
		}
		state.ProgramData = &ProgramData{
			Slot:            slot,
			AuthorityOption: uint32(opt),
		}
		if state.ProgramData.AuthorityOption == 1 {
			state.ProgramData.Authority = *auth
		}
	default:
		// other variants (Uninitialized, Buffer) are not needed here
	}
	return state, nil
}

func getUpgradeableLoaderState(client *solrpc.Client, progPubkey solana.PublicKey) (*UpgradeableLoaderState, error) {
	resp, err := client.GetAccountInfo(context.Background(), progPubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch program account: %w", err)
	}
	if resp.Value == nil {
		return nil, errors.New("program account does not exist")
	}

	state, err := decodeUpgradeableLoaderState(resp.Value.Data.GetBinary())
	if err != nil {
		return nil, fmt.Errorf("decode error: %w", err)
	}
	return state, nil
}

func GetUpgradeAuthority(client *solrpc.Client, progDataPubkey solana.PublicKey) (solana.PublicKey, error) {
	data, err := GetSolProgramData(client, progDataPubkey)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to get program data for %s: %w", progDataPubkey.String(), err)
	}
	state, err := getUpgradeableLoaderState(client, data.Address)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("failed to get upgrade authority for program data %s: %w", progDataPubkey.String(), err)
	}

	if state.ProgramData == nil {
		return solana.PublicKey{}, errors.New("unexpected state: no programdata")
	}

	if state.ProgramData.AuthorityOption == 0 {
		// No authority – the program is immutable
		return solana.PublicKey{}, nil
	}
	return state.ProgramData.Authority, nil
}

func UpgradeProgram(
	b operations.Bundle,
	chain cldf_solana.Chain,
	contractType cldf_deployment.ContractType,
	programID solana.PublicKey,
	programName string,
) ([]solana.Instruction, error) {
	b.Logger.Infow("Upgrading program", "contractType", contractType)

	// An upgrade replaces the binary in place, so the same artifact policy that
	// governs a first deploy has to govern the replacement.
	artifact, err := RequireDeployable(chain, programName)
	if err != nil {
		return nil, err
	}
	b.Logger.Infow("Upgrading to verified artifact",
		"program", programName,
		"executableHash", artifact.ExecutableHash,
		"securityTxt", artifact.HasSecurityTxt,
		"idl", artifact.IDLPath)

	rawProgramBuffer, err := chain.DeployProgram(b.Logger, cldf_solana.ProgramInfo{
		Name: programName,
	}, true, false)
	if err != nil {
		return nil, err
	}
	programBuffer := solana.MustPublicKeyFromBase58(rawProgramBuffer)

	upgradeAuthority, err := GetUpgradeAuthority(chain.Client, programID)
	if err != nil {
		return nil, err
	}

	ixn := SetUpgradeAuthority(programBuffer, chain.Deployer().PublicKey(), upgradeAuthority, true)
	if err = chain.Confirm([]solana.Instruction{ixn}); err != nil {
		return nil, fmt.Errorf("failed to confirm setUpgradeAuthority: %w", err)
	}

	upgradeIx, err := generateUpgradeIxn(
		programID,
		programBuffer,
		chain.Deployer().PublicKey(),
		upgradeAuthority,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate upgrade instruction: %w", err)
	}

	bufferSize, err := GetSolProgramSize(chain, programBuffer)
	if err != nil {
		return nil, fmt.Errorf("failed to get buffer size: %w", err)
	}
	closeIx, err := generateCloseBufferIxn(
		programBuffer,
		chain.Deployer().PublicKey(),
		upgradeAuthority,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate close buffer instruction: %w", err)
	}
	// Extend transaction is permissionless, so it is always executed by deployer key
	// It is a solana limitation that PDAs (MCMs in this case) can not manage memory
	extendIxn, err := generateExtendIxn(
		chain,
		programID,
		chain.Deployer().PublicKey(),
		bufferSize,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate extend buffer instruction: %w", err)
	}
	if extendIxn != nil {
		if err := chain.Confirm([]solana.Instruction{extendIxn}); err != nil {
			return nil, fmt.Errorf("failed to confirm instructions: %w", err)
		}
		if err := waitPastDeploySlot(b.GetContext(), chain.Client, programID); err != nil {
			return nil, err
		}
	}
	return []solana.Instruction{upgradeIx, closeIx}, nil
}

// waitPastDeploySlot blocks until the finalized slot is past the slot a program was last deployed
// or extended in. The loader rejects an Upgrade sent in that same slot.
func waitPastDeploySlot(ctx context.Context, client *solrpc.Client, programID solana.PublicKey) error {
	// DeploySlot reads at confirmed: a finalized read would not yet see the extend.
	deploySlot, err := DeploySlot(client, programID)
	if err != nil {
		return err
	}
	for {
		slot, err := client.GetSlot(ctx, solrpc.CommitmentFinalized)
		if err != nil {
			return fmt.Errorf("get current slot: %w", err)
		}
		if slot > deploySlot {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(slotPollInterval):
		}
	}
}

const slotPollInterval = 200 * time.Millisecond

// SetUpgradeAuthority creates a transaction to set the upgrade authority for a program
func SetUpgradeAuthority(programID solana.PublicKey, currentUpgradeAuthority solana.PublicKey, newUpgradeAuthority solana.PublicKey, isBuffer bool) solana.Instruction {
	// Buffers use the program account as the program data account
	programDataSlice := solana.NewAccountMeta(programID, true, false)
	if !isBuffer {
		// Actual program accounts use the program data account
		programDataAddress, _, _ := solana.FindProgramAddress([][]byte{programID.Bytes()}, solana.BPFLoaderUpgradeableProgramID)
		programDataSlice = solana.NewAccountMeta(programDataAddress, true, false)
	}

	keys := solana.AccountMetaSlice{
		programDataSlice, // Program account (writable)
		solana.NewAccountMeta(currentUpgradeAuthority, false, true), // Current upgrade authority (signer)
		solana.NewAccountMeta(newUpgradeAuthority, false, false),    // New upgrade authority
	}

	instruction := solana.NewInstruction(
		solana.BPFLoaderUpgradeableProgramID,
		keys,
		// https://github.com/solana-playground/solana-playground/blob/2998d4cf381aa319d26477c5d4e6d15059670a75/vscode/src/commands/deploy/bpf-upgradeable/bpf-upgradeable.ts#L72
		[]byte{4, 0, 0, 0}, // 4-byte SetAuthority instruction identifier
	)

	return instruction
}

func generateUpgradeIxn(
	programID solana.PublicKey,
	bufferAddress solana.PublicKey,
	spillAddress solana.PublicKey,
	upgradeAuthority solana.PublicKey,
) (solana.Instruction, error) {
	// Derive the program data address
	programDataAccount, _, _ := solana.FindProgramAddress([][]byte{programID.Bytes()}, solana.BPFLoaderUpgradeableProgramID)

	// Accounts involved in the transaction
	keys := solana.AccountMetaSlice{
		solana.NewAccountMeta(programDataAccount, true, false), // Program account (writable)
		solana.NewAccountMeta(programID, true, false),
		solana.NewAccountMeta(bufferAddress, true, false),             // Buffer account (writable)
		solana.NewAccountMeta(spillAddress, true, false),              // Spill account (writable)
		solana.NewAccountMeta(solana.SysVarRentPubkey, false, false),  // System program
		solana.NewAccountMeta(solana.SysVarClockPubkey, false, false), // System program
		solana.NewAccountMeta(upgradeAuthority, false, true),          // Current upgrade authority (signer)
	}

	instruction := solana.NewInstruction(
		solana.BPFLoaderUpgradeableProgramID,
		keys,
		// https://github.com/solana-playground/solana-playground/blob/2998d4cf381aa319d26477c5d4e6d15059670a75/vscode/src/commands/deploy/bpf-upgradeable/bpf-upgradeable.ts#L66
		[]byte{3, 0, 0, 0}, // 4-byte Upgrade instruction identifier
	)

	return instruction, nil
}

func generateExtendIxn(
	chain cldf_solana.Chain,
	programID solana.PublicKey,
	payer solana.PublicKey,
	newProgramSize int,
) (*solana.GenericInstruction, error) {
	// Derive the program data address
	programDataAccount, _, _ := solana.FindProgramAddress([][]byte{programID.Bytes()}, solana.BPFLoaderUpgradeableProgramID)

	programDataSize, err := GetSolProgramSize(chain, programDataAccount)
	if err != nil {
		return nil, fmt.Errorf("failed to get program size: %w", err)
	}
	// GetSolProgramSize returns the raw account length, so newProgramSize
	// (buffer) includes the 45-byte buffer header. The bpf-loader Upgrade
	// additionally requires programData to exceed the buffer by the 45-byte
	// programData header: programData >= buffer + 45. RMN failed with equal
	// lengths (buffer grew by exactly 45) because a plain >= comparison
	// skipped the extend while the Upgrade still needed 45 more bytes.
	const bpfLoaderHeaderSize = 45
	if newProgramSize+bpfLoaderHeaderSize <= programDataSize {
		// Buffer account size is less than program account size
		return nil, nil
	}
	extraBytes := newProgramSize + bpfLoaderHeaderSize - programDataSize
	if extraBytes > math.MaxUint32 {
		return nil, fmt.Errorf("extra bytes %d exceeds maximum value %d", extraBytes, math.MaxUint32)
	}
	// https://github.com/solana-labs/solana/blob/7700cb3128c1f19820de67b81aa45d18f73d2ac0/sdk/program/src/loader_upgradeable_instruction.rs#L146
	data := binary.LittleEndian.AppendUint32([]byte{}, 6) // 4-byte Extend instruction identifier
	// extraBytes is exact (buffer + 45-byte header - programData). The
	// bpf-loader runtime rejects extends below 10240 additional bytes
	// ("ExtendProgram requires a minimum of 10240 additional bytes", seen
	// live on devnet 2026-09-07), so floor the request at 10240; 4096 is
	// extra headroom on top.
	const minExtendBytes = 10240
	const headroomBytes = 4096
	requested := max(extraBytes, minExtendBytes-headroomBytes) + headroomBytes
	if requested > math.MaxUint32 {
		return nil, fmt.Errorf("requested bytes %d exceeds maximum value %d", requested, math.MaxUint32)
	}
	data = binary.LittleEndian.AppendUint32(data, uint32(requested))

	keys := solana.AccountMetaSlice{
		solana.NewAccountMeta(programDataAccount, true, false),      // Program data account (writable)
		solana.NewAccountMeta(programID, true, false),               // Program account (writable)
		solana.NewAccountMeta(solana.SystemProgramID, false, false), // System program
		solana.NewAccountMeta(payer, true, true),                    // Payer for rent
	}

	ixn := solana.NewInstruction(
		solana.BPFLoaderUpgradeableProgramID,
		keys,
		data,
	)

	return ixn, nil
}

func generateCloseBufferIxn(
	programAddress solana.PublicKey,
	recipient solana.PublicKey,
	upgradeAuthority solana.PublicKey,
) (solana.Instruction, error) {
	keys := solana.AccountMetaSlice{
		solana.Meta(programAddress).WRITE(),
		solana.Meta(recipient).WRITE(),
		solana.Meta(upgradeAuthority).SIGNER(),
	}

	instruction := solana.NewInstruction(
		solana.BPFLoaderUpgradeableProgramID,
		keys,
		// https://github.com/solana-playground/solana-playground/blob/2998d4cf381aa319d26477c5d4e6d15059670a75/vscode/src/commands/deploy/bpf-upgradeable/bpf-upgradeable.ts#L78
		[]byte{5, 0, 0, 0}, // 4-byte Close instruction identifier
	)

	return instruction, nil
}
