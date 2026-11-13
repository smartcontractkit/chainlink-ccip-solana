package solcommon

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	solrpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	solCommonUtil "github.com/smartcontractkit/chainlink-ccip/chains/solana/utils/common"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	cldf_solana "github.com/smartcontractkit/chainlink-deployments-framework/chain/solana"
	"github.com/smartcontractkit/chainlink-deployments-framework/operations"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/v2_0_0/operations/metadata"
)

// PublishIDL is exercised against a real validator because almost everything it
// can get wrong is invisible to a unit test: the split between what the deployer
// signs and what the upgrade authority signs, passing the buffer as an extra
// signer, and reclaiming a buffer left over from an interrupted run.
//
// See the metadata package's publish test for why the validator is started with
// --upgradeable-program and --clone-upgradeable-program, and why it has to be
// Agave 4 or newer.

const (
	testProgramName  = "rmn_remote"
	devnetURL        = "https://api.devnet.solana.com"
	validatorEnv     = "SOLANA_TEST_VALIDATOR"
	minValidatorMajr = 4
)

func testValidator(t *testing.T) string {
	t.Helper()

	if path := os.Getenv(validatorEnv); path != "" {
		return path
	}

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	candidates, _ := filepath.Glob(filepath.Join(
		home, ".local/share/solana/install/releases/*/solana-release/bin/solana-test-validator"))

	for _, path := range candidates {
		out, vErr := exec.CommandContext(t.Context(), path, "--version").Output()
		if vErr != nil {
			continue
		}
		m := regexp.MustCompile(`\s(\d+)\.\d+\.\d+`).FindSubmatch(out)
		if m == nil {
			continue
		}
		if major, cErr := strconv.Atoi(string(m[1])); cErr == nil && major >= minValidatorMajr {
			return path
		}
	}

	t.Skipf("no Agave %d+ validator found; run scripts/setup-contract-env.sh or point %s at one",
		minValidatorMajr, validatorEnv)

	return ""
}

func contractsDir(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../../contracts")
	require.NoError(t, err)

	return abs
}

func declaredProgramID(t *testing.T, idlPath string) solana.PublicKey {
	t.Helper()

	raw, err := os.ReadFile(idlPath)
	require.NoError(t, err)

	var doc struct {
		Address  string `json:"address"`
		Metadata struct {
			Address string `json:"address"`
		} `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))

	addr := doc.Metadata.Address
	if addr == "" {
		addr = doc.Address
	}
	require.NotEmpty(t, addr)

	id, err := solana.PublicKeyFromBase58(addr)
	require.NoError(t, err)

	return id
}

// stageArtifact lays out a programs directory the way a release tarball does,
// so InspectArtifact finds the binary and the IDL side by side.
func stageArtifact(t *testing.T, contracts string) string {
	t.Helper()

	dir := t.TempDir()
	for subdir, ext := range map[string]string{"deploy": ".so", "idl": ".json"} {
		raw, err := os.ReadFile(filepath.Join(contracts, "target", subdir, testProgramName+ext))
		if err != nil {
			t.Skipf("missing %s%s; run make build-contracts", testProgramName, ext)
		}
		// The name is assembled from a constant and a literal extension, so it
		// cannot escape dir; gosec only sees that it came from a loop variable.
		//nolint:gosec // G703: the destination name is built here, not supplied
		require.NoError(t, os.WriteFile(filepath.Join(dir, testProgramName+ext), raw, 0o600))
	}

	return dir
}

func startValidator(t *testing.T, programID solana.PublicKey, soPath string, authority solana.PublicKey) (*solrpc.Client, string) {
	t.Helper()

	var lc net.ListenConfig
	listener, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())

	bin := testValidator(t)

	cmd := exec.CommandContext(t.Context(), bin,
		"--ledger", filepath.Join(t.TempDir(), "ledger"),
		"--rpc-port", strconv.Itoa(port),
		"--upgradeable-program", programID.String(), soPath, authority.String(),
		"--url", devnetURL,
		"--clone-upgradeable-program", metadata.ProgramID.String(),
		"--ticks-per-slot", "8",
		"--limit-ledger-size", "200000000",
		"--reset", "--quiet",
	)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := solrpc.New(url)
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		if _, hErr := client.GetHealth(context.Background()); hErr == nil {
			return client, url
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			t.Skipf("validator exited during genesis; cloning %s from devnet needs network access",
				metadata.ProgramID)
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("validator did not become healthy")

	return nil, ""
}

// testChain wires Confirm the same way the framework does, so the extra signer
// handling under test is the real one.
func testChain(client *solrpc.Client, url, programsPath string, deployer solana.PrivateKey) cldf_solana.Chain {
	return cldf_solana.Chain{
		Selector:     124615329519749607, // solana-devnet
		Client:       client,
		URL:          url,
		DeployerKey:  &deployer,
		ProgramsPath: programsPath,
		Confirm: func(ixs []solana.Instruction, opts ...solCommonUtil.TxModifier) error {
			_, err := solCommonUtil.SendAndConfirm(
				context.Background(), client, ixs, deployer, solrpc.CommitmentConfirmed, opts...)

			return err
		},
	}
}

func testBundle(t *testing.T) operations.Bundle {
	t.Helper()

	return operations.NewBundle(
		context.Background, logger.Test(t), operations.NewMemoryReporter())
}

func TestPublishIDLEndToEnd(t *testing.T) {
	// TODO: stands up a solana-test-validator whose genesis clones the SPL
	// Token-Metadata program from devnet. The clone rate-limits and hangs
	// genesis, so the test stalls for the full health deadline. Vendor the
	// metadata program binary as testdata and load it locally, then re-enable.
	t.Skip("stands up a validator and clones a program from devnet")

	contracts := contractsDir(t)
	programsPath := stageArtifact(t, contracts)
	idlPath := filepath.Join(programsPath, testProgramName+".json")
	programID := declaredProgramID(t, idlPath)

	deployer, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	// The deployer is the upgrade authority here, so the publish runs straight
	// through instead of coming back as a proposal.
	client, url := startValidator(t, programID, filepath.Join(programsPath, testProgramName+".so"), deployer.PublicKey())

	ctx := context.Background()
	sig, err := client.RequestAirdrop(ctx, deployer.PublicKey(), 20*solana.LAMPORTS_PER_SOL, solrpc.CommitmentFinalized)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		res, sErr := client.GetSignatureStatuses(ctx, true, sig)

		return sErr == nil && len(res.Value) > 0 && res.Value[0] != nil && res.Value[0].Err == nil
	}, 45*time.Second, 300*time.Millisecond, "airdrop did not confirm")

	chain := testChain(client, url, programsPath, deployer)
	b := testBundle(t)

	batch, err := PublishIDL(b, chain, programID, testProgramName)
	require.NoError(t, err)
	require.Nil(t, batch, "a deployer-owned program publishes directly, with no proposal")

	account, err := metadata.CanonicalPDA(programID, metadata.SeedIDL)
	require.NoError(t, err)

	stored, err := client.GetAccountInfoWithOpts(ctx, account,
		&solrpc.GetAccountInfoOpts{Commitment: solrpc.CommitmentConfirmed})
	require.NoError(t, err)
	require.NotNil(t, stored.Value)

	acct, err := metadata.ParseAccount(stored.Value.Data.GetBinary())
	require.NoError(t, err)
	require.True(t, acct.Canonical, "publishing as the upgrade authority must be canonical")
	require.Equal(t, metadata.SeedIDL, acct.Seed)

	published, err := metadata.Decompress(acct.Data)
	require.NoError(t, err)

	source, err := os.ReadFile(idlPath)
	require.NoError(t, err)

	var got, want any
	require.NoError(t, json.Unmarshal(published, &got))
	require.NoError(t, json.Unmarshal(source, &want))
	require.Equal(t, want, got, "the published idl must match the artifact")

	// The buffer is closed on the direct path, so its rent came back.
	buffer, err := idlBufferKey(t.Context(), chain, programID)
	require.NoError(t, err)
	leftover, err := readMetadataAccount(client, buffer.PublicKey())
	require.NoError(t, err)
	require.False(t, leftover.Initialized, "the buffer should be closed once the document has landed")

	// Republishing must be idempotent: the account already exists, so this goes
	// down the SetData path rather than trying to create it again.
	batch, err = PublishIDL(b, chain, programID, testProgramName)
	require.NoError(t, err)
	require.Nil(t, batch)
}
