package solcommon

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
)

// networkChain points at a non-local RPC so the full policy applies.
func networkChain(programsPath string) cldf_solana.Chain {
	return cldf_solana.Chain{URL: "https://api.mainnet-beta.solana.com", ProgramsPath: programsPath}
}

// localChain points at a test validator, where only the IDL is required.
func localChain(programsPath string) cldf_solana.Chain {
	return cldf_solana.Chain{URL: "http://localhost:8899", ProgramsPath: programsPath}
}

// elfV3 wraps a payload in a minimal ELF64 header that declares the expected
// SBPF version.
func elfV3(payload []byte) []byte {
	so := make([]byte, 0x34, 0x34+len(payload))
	copy(so, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	so[0x30] = byte(ExpectedSBPFVersion)
	return append(so, payload...)
}

// writeArtifact drops a fake .so (and optionally an IDL) into a temp programs
// dir. The .so gets a minimal ELF64 header that declares SBPF v3, with the
// given body after it.
func writeArtifact(t *testing.T, name string, body []byte, withIDL bool) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name+".so"), elfV3(body), 0o600))
	if withIDL {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name+".json"), []byte(`{"version":"2.0.0"}`), 0o600))
	}
	return dir
}

func writeProvenance(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, WriteProvenance(dir, Provenance{
		Release: "v2.0.0",
		Commit:  "b74d7c75a1b2c3d4e5f60718293a4b5c6d7e8f90",
	}))
}

func withSecurityTxt(payload string) []byte {
	return []byte("ELF\x00\x00" + securityTxtMarker + "\x00" + payload + "\x00=======END SECURITY.TXT V1=======")
}

func TestEnforcedProgramRequiresSecurityTxtAndIDL(t *testing.T) {
	const prog = "ccip_router"
	require.True(t, IsEnforced(prog))

	t.Run("missing both", func(t *testing.T) {
		dir := writeArtifact(t, prog, []byte("ELF no metadata"), false)
		_, err := RequireDeployable(networkChain(dir), prog)
		require.Error(t, err)
		require.Contains(t, err.Error(), "security.txt")
		require.Contains(t, err.Error(), "IDL")
	})

	t.Run("missing idl only", func(t *testing.T) {
		dir := writeArtifact(t, prog, withSecurityTxt("name: CCIP Router"), false)
		_, err := RequireDeployable(networkChain(dir), prog)
		require.Error(t, err)
		require.Contains(t, err.Error(), "IDL")
		require.NotContains(t, err.Error(), "security.txt (add")
	})

	t.Run("missing security txt only", func(t *testing.T) {
		dir := writeArtifact(t, prog, []byte("ELF no metadata"), true)
		_, err := RequireDeployable(networkChain(dir), prog)
		require.Error(t, err)
		require.Contains(t, err.Error(), "security.txt")
	})

	t.Run("complete", func(t *testing.T) {
		dir := writeArtifact(t, prog, withSecurityTxt("name: CCIP Router"), true)
		writeProvenance(t, dir)
		info, err := RequireDeployable(networkChain(dir), prog)
		require.NoError(t, err)
		require.True(t, info.HasSecurityTxt)
		require.NotEmpty(t, info.IDLPath)
		require.Len(t, info.ExecutableHash, 64)
	})
}

// Security.txt is the one requirement that applies only to the enforced set,
// so an unenforced program deploys without one. The other requirements, IDL
// and provenance, still apply to every program.
func TestUnenforcedProgramNeedsNoSecurityTxt(t *testing.T) {
	const prog = "test_ccip_receiver"
	require.False(t, IsEnforced(prog))

	dir := writeArtifact(t, prog, []byte("ELF no metadata"), true)
	writeProvenance(t, dir)
	info, err := RequireDeployable(networkChain(dir), prog)
	require.NoError(t, err)
	require.False(t, info.HasSecurityTxt)
	require.NotEmpty(t, info.IDLPath)
	require.Len(t, info.ExecutableHash, 64, "still hashed, just not blocked")
}

func TestMissingArtifactIsAnError(t *testing.T) {
	_, err := RequireDeployable(networkChain(t.TempDir()), "ccip_router")
	require.Error(t, err)
	require.Contains(t, err.Error(), "read program artifact")
}

// The on-chain account is padded, so the hash must ignore trailing zeros or a
// local artifact will never match the deployed program.
func TestExecutableHashIgnoresTrailingPadding(t *testing.T) {
	body := []byte("program bytes")
	padded := append(append([]byte{}, body...), make([]byte, 4096)...)
	require.Equal(t, ExecutableHash(body), ExecutableHash(padded))

	require.NotEqual(t, ExecutableHash(body), ExecutableHash([]byte("program byteS")))
	// interior zeros are content, not padding
	interior := append(append([]byte{}, body...), append([]byte{0}, body...)...)
	require.NotEqual(t, ExecutableHash(body), ExecutableHash(interior))
}

func TestEnforcedSetCoversDeployedProgramsOnly(t *testing.T) {
	for _, p := range []string{"ccip_router", "ccip_offramp", "fee_quoter", "rmn_remote",
		"committee_verifier", "executor", "burnmint_token_pool",
		"lockrelease_token_pool", "cctp_token_pool", "lombard_token_pool"} {
		require.True(t, IsEnforced(p), "%s is deployed and must be enforced", p)
	}
	for _, p := range []string{"test_program", "example_ccip_sender", "mock_lombard_bridge",
		"arbitrary_cpi_proxy", "base_token_pool", "ccip_common"} {
		require.False(t, IsEnforced(p), "%s is not independently deployed", p)
	}
	for p := range enforcedPrograms {
		require.False(t, strings.HasPrefix(p, "test_") || strings.HasPrefix(p, "mock_"),
			"%s looks like a test program", p)
	}
}

// The on-chain account carries a fixed metadata prefix before the executable.
// Getting the offset wrong yields a hash that never matches anything.
func TestExecutableHashFromProgramData(t *testing.T) {
	body := []byte("the actual program bytes")

	raw := make([]byte, ProgramDataHeaderLen, ProgramDataHeaderLen+len(body)+2048)
	raw[0] = 3 // ProgramData enum tag
	raw = append(raw, body...)
	raw = append(raw, make([]byte, 2048)...) // account padding

	got, err := ExecutableHashFromProgramData(raw)
	require.NoError(t, err)
	require.Equal(t, ExecutableHash(body), got,
		"on-chain hash must equal the local artifact hash")
}

func TestExecutableHashFromProgramDataRejectsShortAccount(t *testing.T) {
	_, err := ExecutableHashFromProgramData(make([]byte, ProgramDataHeaderLen-1))
	require.Error(t, err)
	require.Contains(t, err.Error(), "too short")
}

func TestExecutableHashLabelRoundTrip(t *testing.T) {
	const hash = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"

	label := ExecutableHashLabel(hash)
	got, ok := ParseExecutableHashLabel(label)
	require.True(t, ok)
	require.Equal(t, hash, got)

	for _, notALabel := range []string{"UniBTCWhiteglove", "", executableHashLabelPrefix} {
		_, ok := ParseExecutableHashLabel(notALabel)
		require.False(t, ok, "%q is not an executable hash label", notALabel)
	}
}

// On a local validator the artifact comes from the working tree, so security.txt
// is not required. The IDL still is, because uploading it works anywhere.
func TestLocalValidatorRequiresIDLOnly(t *testing.T) {
	const prog = "ccip_router"

	dir := writeArtifact(t, prog, []byte("ELF no metadata"), true)
	info, err := RequireDeployable(localChain(dir), prog)
	require.NoError(t, err, "security.txt is not required locally")
	require.False(t, info.HasSecurityTxt)

	dir = writeArtifact(t, prog, withSecurityTxt("name: CCIP Router"), false)
	_, err = RequireDeployable(localChain(dir), prog)
	require.Error(t, err, "IDL is still required locally")
	require.Contains(t, err.Error(), "IDL")
	require.Contains(t, err.Error(), "local")
}

// A locally built tree has no provenance, so it cannot be registered as a
// verified build and must not reach a real network.
func TestNetworkDeployRequiresProvenance(t *testing.T) {
	const prog = "ccip_router"
	dir := writeArtifact(t, prog, withSecurityTxt("name: CCIP Router"), true)

	_, err := RequireDeployable(networkChain(dir), prog)
	require.ErrorContains(t, err, "provenance")

	writeProvenance(t, dir)
	_, err = RequireDeployable(networkChain(dir), prog)
	require.NoError(t, err)
}

func TestLocalDeployNeedsNoProvenance(t *testing.T) {
	const prog = "ccip_router"
	dir := writeArtifact(t, prog, []byte("ELF from working tree"), true)

	_, err := RequireDeployable(localChain(dir), prog)
	require.NoError(t, err, "a local validator has no registry to publish to")
}

func TestProvenanceRoundTrip(t *testing.T) {
	dir := t.TempDir()

	_, ok, err := ReadProvenance(dir)
	require.NoError(t, err)
	require.False(t, ok, "absence is not an error")

	want := Provenance{
		Release:         "v2.0.0",
		Commit:          "b74d7c75a1b2c3d4e5f60718293a4b5c6d7e8f90",
		VerifierVersion: "0.5.0",
		BuildArgs:       []string{"--library-name", "ccip_router"},
	}
	require.NoError(t, WriteProvenance(dir, want))

	got, ok, err := ReadProvenance(dir)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, want.Commit, got.Commit)
	require.Equal(t, want.Release, got.Release)
	require.Equal(t, DefaultRepoURL, got.Repo(), "repo defaults when unset")
}

func TestProvenanceRejectsUnusableManifests(t *testing.T) {
	require.Error(t, WriteProvenance(t.TempDir(), Provenance{Release: "v2.0.0"}),
		"a manifest without a commit names nothing")
	require.Error(t, WriteProvenance(t.TempDir(), Provenance{Commit: "abc"}),
		"an abbreviated commit cannot identify a revision")

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ProvenanceFile), []byte("{not json"), 0o600))
	_, _, err := ReadProvenance(dir)
	require.ErrorContains(t, err, "parse provenance")
}

func TestBuildRecordDefaultsArgsToLibraryName(t *testing.T) {
	p := Provenance{Commit: "b74d7c75a1b2", Release: "v2.0.0"}
	rec := p.BuildRecordFor(verifyTestProgram, verifyTestProgram, "ccip_router", 42)
	require.Equal(t, []string{"--library-name", "ccip_router"}, rec.Args)
	require.Equal(t, uint64(42), rec.DeploySlot)
	require.Equal(t, DefaultRepoURL, rec.GitURL)
}

// TestIDLFoundInSiblingIDLDir covers the layout a locally built tree has:
// `anchor build` writes binaries to target/deploy and IDLs to target/idl, and
// devenv points ProgramsPath at the former. Looking only next to the .so found
// no IDL, so RequireIDL rejected every enforced program before it could deploy.
func TestIDLFoundInSiblingIDLDir(t *testing.T) {
	const prog = "ccip_router"

	target := t.TempDir()
	deploy := filepath.Join(target, "deploy")
	idl := filepath.Join(target, "idl")
	require.NoError(t, os.MkdirAll(deploy, 0o750))
	require.NoError(t, os.MkdirAll(idl, 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(deploy, prog+".so"), elfV3(withSecurityTxt("name: CCIP Router")), 0o600))

	// Without the IDL in either place the program is still refused.
	_, err := RequireDeployable(localChain(deploy), prog)
	require.Error(t, err)
	require.Contains(t, err.Error(), "IDL")

	idlPath := filepath.Join(idl, prog+".json")
	require.NoError(t, os.WriteFile(idlPath, []byte(`{"version":"2.0.0"}`), 0o600))

	info, err := RequireDeployable(localChain(deploy), prog)
	require.NoError(t, err)
	require.Equal(t, idlPath, info.IDLPath)
}

// TestIDLNextToBinaryWins is the release-tarball layout, which stages IDLs
// alongside the binaries. That copy takes precedence over the sibling dir.
func TestIDLNextToBinaryWins(t *testing.T) {
	const prog = "ccip_router"

	target := t.TempDir()
	deploy := filepath.Join(target, "deploy")
	idl := filepath.Join(target, "idl")
	require.NoError(t, os.MkdirAll(deploy, 0o750))
	require.NoError(t, os.MkdirAll(idl, 0o750))
	require.NoError(t, os.WriteFile(
		filepath.Join(deploy, prog+".so"), withSecurityTxt("name: CCIP Router"), 0o600))
	require.NoError(t, os.WriteFile(
		filepath.Join(idl, prog+".json"), []byte(`{"version":"stale"}`), 0o600))

	staged := filepath.Join(deploy, prog+".json")
	require.NoError(t, os.WriteFile(staged, []byte(`{"version":"2.0.0"}`), 0o600))

	info, err := InspectArtifact(deploy, prog)
	require.NoError(t, err)
	require.Equal(t, staged, info.IDLPath)
}

func TestArtifactDeclaresProgramID(t *testing.T) {
	key, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	programID := key.PublicKey()

	dir := writeArtifact(t, "prog", append([]byte("ELF"), programID[:]...), false)
	ok, err := ArtifactDeclaresProgramID(dir, "prog", programID)
	require.NoError(t, err)
	require.True(t, ok)

	// A program that only compares against its ID can have the 32 bytes folded
	// into 32-bit immediates, so they are present but not contiguous.
	split := []byte("ELF")
	for i := 0; i < len(programID); i += 4 {
		split = append(split, programID[i:i+4]...)
		split = append(split, 0, 0, 0, 0)
	}
	dir = writeArtifact(t, "prog", split, false)
	ok, err = ArtifactDeclaresProgramID(dir, "prog", programID)
	require.NoError(t, err)
	require.True(t, ok)

	other, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	ok, err = ArtifactDeclaresProgramID(dir, "prog", other.PublicKey())
	require.NoError(t, err)
	require.False(t, ok)
}

func TestInspectArtifactReadsSBPFVersion(t *testing.T) {
	header := func(flags uint32) []byte {
		b := make([]byte, 0x34)
		copy(b, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
		binary.LittleEndian.PutUint32(b[0x30:], flags)
		return b
	}

	writeRaw := func(body []byte) string {
		t.Helper()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "prog.so"), body, 0o600))
		return dir
	}

	dir := writeRaw(header(3))
	info, err := InspectArtifact(dir, "prog")
	require.NoError(t, err)
	require.Equal(t, uint32(3), info.SBPFVersion)

	dir = writeRaw(header(2))
	info, err = InspectArtifact(dir, "prog")
	require.NoError(t, err)
	require.Equal(t, uint32(2), info.SBPFVersion)

	// A file that is not an ELF parses to zero, so the policy checks fail.
	dir = writeRaw([]byte("not an elf"))
	info, err = InspectArtifact(dir, "prog")
	require.NoError(t, err)
	require.Zero(t, info.SBPFVersion)
}

func TestIDLDeclaresProgramID(t *testing.T) {
	key, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	programID := key.PublicKey()

	writeIDL := func(dir, name, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, name+".json"), []byte(body), 0o600))
	}

	// The IDL spec keeps the address at the top level.
	dir := writeArtifact(t, "prog", []byte("ELF"), false)
	writeIDL(dir, "prog", `{"address":"`+programID.String()+`"}`)
	ok, err := IDLDeclaresProgramID(dir, "prog", programID)
	require.NoError(t, err)
	require.True(t, ok)

	// An IDL built for another address fails.
	other, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	dir = writeArtifact(t, "prog", []byte("ELF"), false)
	writeIDL(dir, "prog", `{"address":"`+other.PublicKey().String()+`"}`)
	ok, err = IDLDeclaresProgramID(dir, "prog", programID)
	require.NoError(t, err)
	require.False(t, ok)

	// No IDL means nothing to check: the artifact policy enforces presence.
	dir = writeArtifact(t, "prog", []byte("ELF"), false)
	ok, err = IDLDeclaresProgramID(dir, "prog", programID)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestDeployKeypairID(t *testing.T) {
	key, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	dir := t.TempDir()
	raw, err := json.Marshal([]byte(key))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, KeypairFile("prog")), raw, 0o600))

	id, err := DeployKeypairID(dir, "prog")
	require.NoError(t, err)
	require.True(t, key.PublicKey().Equals(id))

	// No keypair file means no deploy.
	_, err = DeployKeypairID(dir, "other")
	require.Error(t, err)
	require.Contains(t, err.Error(), "read keypair")
}

func TestRequireDeployableRejectsWrongSBPFVersion(t *testing.T) {
	const prog = "ccip_router"
	// Everything is in place except the arch: a v2 header, security.txt, IDL
	// and provenance.
	body := withSecurityTxt("name: CCIP Router")
	so := make([]byte, 0x34, 0x34+len(body))
	copy(so, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	so[0x30] = 2
	so = append(so, body...)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, prog+".so"), so, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, prog+".json"), []byte(`{"version":"2.0.0"}`), 0o600))
	writeProvenance(t, dir)

	_, err := RequireDeployable(networkChain(dir), prog)
	require.Error(t, err)
	require.Contains(t, err.Error(), "SBPF")
}
