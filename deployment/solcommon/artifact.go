package solcommon

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gagliardetto/solana-go"
	solrpc "github.com/gagliardetto/solana-go/rpc"

	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
)

// securityTxtMarker is the header the solana-security-txt macro writes into the
// .security.txt section of a compiled program.
// https://github.com/neodyme-labs/solana-security-txt
const securityTxtMarker = "=======BEGIN SECURITY.TXT V1======="

// enforcedPrograms are the Chainlink-owned programs that reach a real network.
// Every one of them must ship a security.txt and an IDL.
//
// Test, example and mock programs are deliberately absent: they are devenv-only
// and carry no security policy. Partner programs such as lombard_token_pool are
// enforced too, since they are deployed, but they carry the partner's own
// contacts rather than Chainlink's.
var enforcedPrograms = map[string]bool{
	"ccip_router":            true,
	"ccip_offramp":           true,
	"fee_quoter":             true,
	"rmn_remote":             true,
	"committee_verifier":     true,
	"executor":               true,
	"burnmint_token_pool":    true,
	"lockrelease_token_pool": true,
	"cctp_token_pool":        true,
	"lombard_token_pool":     true,
}

// ExpectedSBPFVersion is the SBPF version every deployed or upgraded program
// must declare. It matches the --arch flag of contracts/scripts/verify_build.sh.
const ExpectedSBPFVersion uint32 = 3

// ArtifactInfo describes a compiled program about to be deployed or used to
// upgrade an existing one.
type ArtifactInfo struct {
	ProgramName string
	Path        string
	// ExecutableHash is the sha256 of the program bytes with trailing zero
	// padding removed. It matches `solana-verify get-executable-hash`, and can
	// be compared against the on-chain program data after deployment.
	ExecutableHash string
	HasSecurityTxt bool
	// SBPFVersion is the e_flags value of the ELF header: 0 for v0, 3 for v3.
	// A file that is not a parseable ELF64 object also reports 0.
	SBPFVersion uint32
	IDLPath     string // empty when no IDL sits next to the binary
}

// IsEnforced reports whether a program must satisfy the artifact policy.
func IsEnforced(programName string) bool { return enforcedPrograms[programName] }

// InspectArtifact reads the program binary and reports what it carries.
func InspectArtifact(programsPath, programName string) (ArtifactInfo, error) {
	soPath := filepath.Join(programsPath, programName+".so")
	data, err := os.ReadFile(soPath)
	if err != nil {
		return ArtifactInfo{}, fmt.Errorf("read program artifact %s: %w", soPath, err)
	}

	info := ArtifactInfo{
		ProgramName:    programName,
		Path:           soPath,
		ExecutableHash: ExecutableHash(data),
		HasSecurityTxt: bytes.Contains(data, []byte(securityTxtMarker)),
	}

	// The SBPF version sits in e_flags: 4 little-endian bytes at offset 0x30
	// of the ELF header. A file without a valid header keeps the zero value,
	// which the upgrade path rejects. SIMD-0500 makes v3 the only accepted
	// format for deploys and upgrades: https://solana.com/upgrade
	const eFlagsOffset = 0x30
	if len(data) >= eFlagsOffset+4 &&
		bytes.Equal(data[:4], []byte{0x7f, 'E', 'L', 'F'}) &&
		data[4] == 2 && // ELFCLASS64
		data[5] == 1 { // ELFDATA2LSB
		info.SBPFVersion = binary.LittleEndian.Uint32(data[eFlagsOffset:])
	}

	info.IDLPath = findIDL(programsPath, programName)

	return info, nil
}

// ArtifactDeclaresProgramID reports whether the compiled program embeds the
// given program ID. Anchor's declare_id! writes the 32 ID bytes into the
// binary, so a program built for a different ID fails this check.
//
// The program keypair is not needed: an upgrade is signed by the upgrade
// authority, and only the artifact must match the address in the datastore.
//
// A program that never references crate::ID can fail both scans: the linker
// drops an unused static, and no immediates remain. Anchor programs reference
// the ID for PDA and owner checks, so this stays theoretical.
func ArtifactDeclaresProgramID(programsPath, programName string, programID solana.PublicKey) (bool, error) {
	soPath := filepath.Join(programsPath, programName+".so")
	data, err := os.ReadFile(soPath)
	if err != nil {
		return false, fmt.Errorf("read program artifact %s: %w", soPath, err)
	}

	id := programID[:]
	if bytes.Contains(data, id) {
		return true, nil
	}

	// When the ID is only compared against, the compiler folds the comparison
	// into 32-bit immediates instead of storing the 32 bytes in rodata. Every
	// word must still be present, so an artifact built for another address
	// fails unless all eight words collide.
	for i := 0; i < len(id); i += 4 {
		if !bytes.Contains(data, id[i:i+4]) {
			return false, nil
		}
	}

	return true, nil
}

// IDLDeclaresProgramID reports whether the IDL staged next to the binary
// names the given program ID. Anchor copies declare_id! into the address
// field at build time, so this checks the source intent while
// ArtifactDeclaresProgramID checks the binary. A missing IDL is not an
// error: RequireDeployable decides where an IDL is mandatory.
func IDLDeclaresProgramID(programsPath, programName string, programID solana.PublicKey) (bool, error) {
	idlPath := findIDL(programsPath, programName)
	if idlPath == "" {
		return true, nil
	}
	data, err := os.ReadFile(idlPath)
	if err != nil {
		return false, fmt.Errorf("read IDL %s: %w", idlPath, err)
	}

	// The IDL spec keeps the address at the top level.
	var idl struct {
		Address string `json:"address"`
	}
	if err = json.Unmarshal(data, &idl); err != nil {
		return false, fmt.Errorf("parse IDL %s: %w", idlPath, err)
	}
	if idl.Address == "" {
		return false, fmt.Errorf("released IDL %s carries no address field", idlPath)
	}
	declared, err := solana.PublicKeyFromBase58(idl.Address)
	if err != nil {
		return false, fmt.Errorf("released IDL %s holds an invalid address %q: %w", idlPath, idl.Address, err)
	}

	return declared.Equals(programID), nil
}

// DeployKeypairID returns the address a fresh deploy of the program will get:
// the public key of its deploy keypair. An initial deploy needs the keypair,
// and the artifact must have been built for the address it derives.
func DeployKeypairID(programsPath, programName string) (solana.PublicKey, error) {
	kpFile := KeypairPath(programsPath, programName)
	privKey, err := solana.PrivateKeyFromSolanaKeygenFile(kpFile)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("read keypair %s: %w", kpFile, err)
	}

	return privKey.PublicKey(), nil
}

// RequireArtifactForID fails when the compiled program or its IDL was built
// for a different program ID than the given one. The upgrade path passes the
// datastore address; the deploy path passes the deploy keypair's address.
func RequireArtifactForID(programsPath, programName string, programID solana.PublicKey) error {
	declares, err := ArtifactDeclaresProgramID(programsPath, programName, programID)
	if err != nil {
		return err
	}
	if !declares {
		return fmt.Errorf("artifact for %s does not declare program ID %s: check the artifact",
			programName, programID)
	}
	idlDeclares, err := IDLDeclaresProgramID(programsPath, programName, programID)
	if err != nil {
		return err
	}
	if !idlDeclares {
		return fmt.Errorf("IDL for %s does not declare program ID %s: check the artifact",
			programName, programID)
	}

	return nil
}

// idlCandidates lists where a program's IDL may sit, in the order they are
// tried. The release tarball stages IDLs next to the binaries, but a locally
// built tree leaves them where Anchor writes them, in the sibling target/idl.
func idlCandidates(programsPath, programName string) []string {
	clean := filepath.Clean(programsPath)

	return []string{
		filepath.Join(clean, programName+".json"),
		filepath.Join(filepath.Dir(clean), "idl", programName+".json"),
	}
}

// KeypairFile is the file name solana-keygen is told to write a program's
// deploy keypair to, and the name that file keeps once it is staged next to
// the binaries.
func KeypairFile(programName string) string { return programName + "-keypair.json" }

// KeypairPath returns where a program's deploy keypair sits, so that the
// layout of programsPath is described here alongside the binary and IDL
// locations rather than rebuilt at each call site.
//
// The keypair is deliberately not part of ArtifactInfo: it is environment
// secret material rather than something the compiled program carries, and it
// is not covered by the artifact policy.
func KeypairPath(programsPath, programName string) string {
	return filepath.Join(programsPath, KeypairFile(programName))
}

// findIDL returns the first IDL path that exists, or "" when there is none.
func findIDL(programsPath, programName string) string {
	for _, candidate := range idlCandidates(programsPath, programName) {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return ""
}

// ExecutableHash returns sha256 over the program bytes with trailing zeros
// stripped, matching how solana-verify hashes both a local .so and an on-chain
// program account. The padding must go or the two never agree: the on-chain
// account is allocated larger than the binary.
func ExecutableHash(data []byte) string {
	sum := sha256.Sum256(bytes.TrimRight(data, "\x00"))
	return hex.EncodeToString(sum[:])
}

// RequireDeployable enforces the artifact policy for a program about to be
// deployed or upgraded, and is called from the deploy and upgrade paths rather
// than offered as a separate changeset so that it cannot be skipped.
//
// Two things narrow it. Security.txt is required only for the programs listed
// in enforcedPrograms, so example and test programs deploy without one. And
// the policy itself depends on the chain: a local validator builds from the
// working tree and has no registry to publish to, so only the IDL requirement
// applies there.
func RequireDeployable(chain cldf_solana.Chain, programName string) (ArtifactInfo, error) {
	info, err := InspectArtifact(chain.ProgramsPath, programName)
	if err != nil {
		return ArtifactInfo{}, err
	}
	// IDL and verified build requirements apply to every program; security.txt
	// is only required for the programs listed in enforcedPrograms.
	policy := PolicyForChain(chain)

	var missing []string
	if policy.RequireSecurityTxt && IsEnforced(programName) && !info.HasSecurityTxt {
		missing = append(missing, "security.txt (add a src/security.rs using the security_txt! macro)")
	}
	if policy.RequireIDL && info.IDLPath == "" {
		missing = append(missing,
			fmt.Sprintf("IDL (expected one of %v, produced by `anchor build` or the release tarball's target/idl)",
				idlCandidates(chain.ProgramsPath, programName)))
	}
	// Local validators may run an older Solana release that cannot load SBPF v3.
	// Their artifact policy is intentionally limited to the local IDL; the validator
	// itself remains the compatibility check for the executable format.
	if !IsLocalValidator(chain.URL) && info.SBPFVersion != ExpectedSBPFVersion {
		missing = append(missing, fmt.Sprintf(
			"an SBPF v%d ELF (the artifact declares SBPF v%d; rebuild with `cargo build-sbf --arch v%d`)",
			ExpectedSBPFVersion, info.SBPFVersion, ExpectedSBPFVersion))
	}
	if policy.RequireVerifiedBuild {
		_, ok, provErr := ReadProvenance(chain.ProgramsPath)
		if provErr != nil {
			return ArtifactInfo{}, provErr
		}
		if !ok {
			missing = append(missing, fmt.Sprintf(
				"provenance (expected %s naming the release and commit these artifacts were built from; "+
					"a locally built tree cannot be registered as a verified build)",
				filepath.Join(chain.ProgramsPath, ProvenanceFile)))
		}
	}
	if len(missing) > 0 {
		return ArtifactInfo{}, fmt.Errorf(
			"program %s cannot be deployed to %s, artifact is missing %d required item(s): %v",
			programName, policy.Network, len(missing), missing)
	}

	return info, nil
}

// ProgramDataHeaderLen is the fixed metadata prefix on an UpgradeableLoader
// ProgramData account: a 4-byte enum tag, an 8-byte slot, and a 33-byte
// Option<Pubkey> upgrade authority. Solana treats this as a constant whether or
// not the authority is set, so the executable always starts at this offset.
const ProgramDataHeaderLen = 45

// DeployedExecutableHash reads a deployed program's bytes off chain and hashes
// them the same way ExecutableHash hashes a local .so, so the two can be
// compared. This is the Go equivalent of `solana-verify get-program-hash`.
func DeployedExecutableHash(client *solrpc.Client, programID solana.PublicKey) (string, error) {
	// The program account points at a separate ProgramData account that holds
	// the executable.
	programData, err := GetSolProgramData(client, programID)
	if err != nil {
		return "", fmt.Errorf("locate program data for %s: %w", programID, err)
	}

	acct, err := client.GetAccountInfoWithOpts(context.Background(), programData.Address,
		&solrpc.GetAccountInfoOpts{Commitment: solrpc.CommitmentConfirmed})
	if err != nil {
		return "", fmt.Errorf("read program data account %s: %w", programData.Address, err)
	}
	if acct == nil || acct.Value == nil {
		return "", fmt.Errorf("program data account does not exist: %s", programData.Address)
	}

	return ExecutableHashFromProgramData(acct.Value.Data.GetBinary())
}

// ExecutableHashFromProgramData strips the ProgramData metadata prefix and
// hashes what remains.
func ExecutableHashFromProgramData(raw []byte) (string, error) {
	if len(raw) < ProgramDataHeaderLen {
		return "", fmt.Errorf("program data account too short: %d bytes, need at least %d",
			len(raw), ProgramDataHeaderLen)
	}
	return ExecutableHash(raw[ProgramDataHeaderLen:]), nil
}

// VerifyDeployedProgram checks that what is running on chain is byte-identical
// to the artifact that was deployed. A mismatch means the deploy did not land
// what was intended, so it is reported as an error rather than a warning.
func VerifyDeployedProgram(client *solrpc.Client, programID solana.PublicKey, expectedHash string) error {
	actual, err := DeployedExecutableHash(client, programID)
	if err != nil {
		return fmt.Errorf("verify deployed program %s: %w", programID, err)
	}
	if actual != expectedHash {
		return fmt.Errorf(
			"deployed program %s does not match the artifact: on chain %s, artifact %s",
			programID, actual, expectedHash)
	}
	return nil
}

// executableHashLabelPrefix namespaces the hash label on an address ref.
const executableHashLabelPrefix = "executable-hash:"

// ExecutableHashLabel formats a hash for storage as an address-ref label.
//
// A Solana program is upgraded in place, so its address never changes and
// cannot indicate which binary is deployed. Recording the hash on the ref is
// what makes the deployed build identifiable after the fact.
func ExecutableHashLabel(hash string) string {
	return executableHashLabelPrefix + hash
}

// ParseExecutableHashLabel returns the hash carried by a label, and whether the
// label was one.
func ParseExecutableHashLabel(label string) (string, bool) {
	if len(label) <= len(executableHashLabelPrefix) ||
		label[:len(executableHashLabelPrefix)] != executableHashLabelPrefix {
		return "", false
	}
	return label[len(executableHashLabelPrefix):], true
}

// VerifyUpgradedProgram re-reads the local artifact and checks that the program
// now running on chain matches it.
//
// Called after an upgrade is confirmed. The program ID is unchanged by an
// in-place upgrade, so this is the only signal that the new binary actually
// took effect.
func VerifyUpgradedProgram(chain cldf_solana.Chain, programID solana.PublicKey, programName string) error {
	artifact, err := InspectArtifact(chain.ProgramsPath, programName)
	if err != nil {
		return fmt.Errorf("re-read artifact after upgrading %s: %w", programName, err)
	}
	return VerifyDeployedProgram(chain.Client, programID, artifact.ExecutableHash)
}
