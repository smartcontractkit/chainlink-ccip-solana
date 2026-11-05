package solcommon

import (
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
)

var verifyTestProgram = solana.MustPublicKeyFromBase58("Ccip842gzYHhvdDkSyi2YVCoAWPbYJoApMFzSxQroE9C")

func TestPolicyIsRelaxedOnlyForLocalValidators(t *testing.T) {
	for _, url := range []string{
		"http://localhost:8899", "http://127.0.0.1:8899",
		"http://0.0.0.0:8899", "http://host.docker.internal:8899",
	} {
		p := PolicyForChain(cldf_solana.Chain{URL: url})
		require.Equal(t, "local", p.Network, url)
		require.True(t, p.RequireIDL, "IDL upload works locally so it stays required")
		require.False(t, p.RequireVerifiedBuild, "nothing to verify against locally")
		require.False(t, p.RequireSecurityTxt)
	}

	for _, url := range []string{
		"https://api.mainnet-beta.solana.com",
		"https://api.devnet.solana.com",
		"https://some-private-rpc.example.com",
	} {
		p := PolicyForChain(cldf_solana.Chain{URL: url})
		require.Equal(t, "network", p.Network, url)
		require.True(t, p.RequireSecurityTxt, url)
		require.True(t, p.RequireIDL, url)
		require.True(t, p.RequireVerifiedBuild, url)
	}
}

// Staging runs a local build on a real network, so there is no release to verify
// against; everything else about the real-network policy still applies.
func TestStagingPolicyDropsOnlyVerifiedBuild(t *testing.T) {
	t.Setenv(ContractsReleaseEnvVar, LocalArtifactsValue)
	p := PolicyForChain(cldf_solana.Chain{URL: "https://api.devnet.solana.com"})
	require.Equal(t, "staging", p.Network)
	require.True(t, p.RequireSecurityTxt)
	require.True(t, p.RequireIDL)
	require.True(t, p.RequirePublishedIDL)
	require.False(t, p.RequireVerifiedBuild)
}

// A remote endpoint whose URL merely mentions a local name must not be read as
// local: doing so drops the whole real-network policy silently, so a mainnet
// deploy would skip security.txt, verified builds and the published IDL.
func TestRemoteURLsNamingLocalhostAreNotLocal(t *testing.T) {
	for _, url := range []string{
		"https://rpc.example.com/localhost",
		"https://localhost.example.com",
		"https://127.0.0.1.example.com",
		"https://rpc.example.com/?fallback=http://localhost:8899",
		"https://host.docker.internal.attacker.example",
	} {
		require.False(t, IsLocalValidator(url), url)
		require.Equal(t, "network", PolicyForChain(cldf_solana.Chain{URL: url}).Network, url)
	}
}

// The forms a validator URL actually takes, including the ones url.Parse does
// not treat as URLs, and the rest of the loopback range.
func TestLocalValidatorURLForms(t *testing.T) {
	for _, url := range []string{
		"localhost:8899", "127.0.0.1:8899", "http://127.0.0.2:8899",
		"http://[::1]:8899", "HTTP://LocalHost:8899", " http://localhost:8899 ",
	} {
		require.True(t, IsLocalValidator(url), url)
	}
}

// The PDA includes the authority, so a deployer-signed record and a
// timelock-signed record are different accounts. Getting this wrong would write
// the record where nobody looks for it.
func TestVerifyPDADependsOnAuthority(t *testing.T) {
	deployer := solana.NewWallet().PublicKey()
	timelock := solana.NewWallet().PublicKey()

	a, err := VerifyPDA(deployer, verifyTestProgram)
	require.NoError(t, err)
	b, err := VerifyPDA(timelock, verifyTestProgram)
	require.NoError(t, err)

	require.NotEqual(t, a, b)

	again, err := VerifyPDA(deployer, verifyTestProgram)
	require.NoError(t, err)
	require.Equal(t, a, again, "derivation must be deterministic")
}

func TestAnchorDiscriminatorMatchesSpec(t *testing.T) {
	for _, name := range []string{"initialize", "update", "close"} {
		sum := sha256.Sum256([]byte("global:" + name))
		require.Equal(t, sum[:8], anchorDiscriminator(name), name)
	}
	require.NotEqual(t, anchorDiscriminator("initialize"), anchorDiscriminator("update"))
}

// Borsh order follows the Rust struct: version, git_url, commit, args, deploy_slot.
func TestInputParamsBorshLayout(t *testing.T) {
	rec := BuildRecord{
		ProgramID:  verifyTestProgram,
		Authority:  solana.NewWallet().PublicKey(),
		Version:    "0.5.0",
		GitURL:     "https://github.com/smartcontractkit/chainlink-ccip-solana.git",
		Commit:     "b74d7c75",
		Args:       []string{"--library-name", "ccip_router"},
		DeploySlot: 123456789,
	}
	data := encodeInputParams(rec)

	read := func(off int) (string, int) {
		n := int(binary.LittleEndian.Uint32(data[off : off+4]))
		return string(data[off+4 : off+4+n]), off + 4 + n
	}

	v, off := read(0)
	require.Equal(t, "0.5.0", v)

	g, off := read(off)
	require.Equal(t, "https://github.com/smartcontractkit/chainlink-ccip-solana", g,
		".git suffix is trimmed so the URL matches what the registry expects")

	c, off := read(off)
	require.Equal(t, "b74d7c75", c)

	require.Equal(t, uint32(2), binary.LittleEndian.Uint32(data[off:off+4]))
	off += 4
	a1, off := read(off)
	a2, off := read(off)
	require.Equal(t, "--library-name", a1)
	require.Equal(t, "ccip_router", a2)

	require.Equal(t, uint64(123456789), binary.LittleEndian.Uint64(data[off:off+8]))
	require.Len(t, data, off+8, "no trailing bytes")
}

func TestRegisterBuildInstruction(t *testing.T) {
	authority := solana.NewWallet().PublicKey()
	rec := BuildRecord{
		ProgramID: verifyTestProgram,
		Authority: authority,
		Version:   "0.5.0",
		Commit:    "b74d7c75",
	}

	for _, tc := range []struct {
		name   string
		update bool
		method string
	}{{"initialize", false, "initialize"}, {"update", true, "update"}} {
		t.Run(tc.name, func(t *testing.T) {
			ix, err := RegisterBuildInstruction(rec, tc.update)
			require.NoError(t, err)

			data, err := ix.Data()
			require.NoError(t, err)
			require.Equal(t, anchorDiscriminator(tc.method), data[:8])

			require.Equal(t, solana.MustPublicKeyFromBase58(VerifyProgramID), ix.ProgramID())

			pda, err := VerifyPDA(authority, verifyTestProgram)
			require.NoError(t, err)

			accts := ix.Accounts()
			require.Len(t, accts, 4)
			require.Equal(t, pda, accts[0].PublicKey)
			require.True(t, accts[0].IsWritable)
			require.Equal(t, authority, accts[1].PublicKey)
			require.True(t, accts[1].IsSigner && accts[1].IsWritable, "authority signs and pays rent")
			require.Equal(t, verifyTestProgram, accts[2].PublicKey)
			require.Equal(t, solana.SystemProgramID, accts[3].PublicKey)
		})
	}
}

func TestRegisterBuildInstructionRequiresProvenance(t *testing.T) {
	authority := solana.NewWallet().PublicKey()

	_, err := RegisterBuildInstruction(BuildRecord{ProgramID: verifyTestProgram, Authority: authority}, false)
	require.ErrorContains(t, err, "commit is required")

	_, err = RegisterBuildInstruction(BuildRecord{Authority: authority, Commit: "abc"}, false)
	require.ErrorContains(t, err, "program id is required")

	_, err = RegisterBuildInstruction(BuildRecord{ProgramID: verifyTestProgram, Commit: "abc"}, false)
	require.ErrorContains(t, err, "authority is required")
}

func TestCloseBuildRecordInstruction(t *testing.T) {
	authority := solana.NewWallet().PublicKey()
	ix, err := CloseBuildRecordInstruction(authority, verifyTestProgram)
	require.NoError(t, err)

	data, err := ix.Data()
	require.NoError(t, err)
	require.Equal(t, anchorDiscriminator("close"), data)
	require.Len(t, ix.Accounts(), 3)
}
