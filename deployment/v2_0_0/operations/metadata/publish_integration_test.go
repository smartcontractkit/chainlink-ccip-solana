package metadata

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
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"
)

// Publishing runs against a real validator, which is the only way to know the
// hand-encoded instructions are right: a wrong discriminator, a missing
// optional-account sentinel or a byte out of place still type-checks and is
// caught only by the program.
//
// The validator is stood up here rather than relying on devenv, using two
// genesis flags that between them remove every obstacle we hit before:
//
//   - --upgradeable-program takes a bare address, so rmn_remote is loaded at
//     the address its own declare_id! names. No key generation, no binary
//     patching, and no DeclaredProgramIdMismatch. It also loads through the
//     upgradeable loader, which the metadata program requires to read an
//     upgrade authority at all; the non-upgradeable --bpf-program is rejected
//     with "Program is not deployed". Passing the payer as the upgrade
//     authority is what makes the publish below canonical.
//
//   - --clone-upgradeable-program copies the real metadata program from
//     devnet, so this exercises the deployed bytecode rather than a stub.
//
// Skipped when the pinned toolchain, the built artifacts, or the network are
// unavailable.

const testProgramName = "rmn_remote"

const devnetURL = "https://api.devnet.solana.com"

// validatorEnv overrides the validator binary. Creating a metadata account
// makes the program CPI into system instruction 13, CreateAccountAllowPrefund,
// which Agave only gained in 4.0. The toolchain pinned in Anchor.toml builds
// the contracts but is older than that, so the validator is resolved
// separately: this variable first, then any installed release new enough.
const validatorEnv = "SOLANA_TEST_VALIDATOR"

// minValidatorMajor is the first Agave major with system instruction 13.
const minValidatorMajor = 4

// testValidator finds a validator that can create a metadata account.
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
		// e.g. "solana-test-validator 4.2.2 (src:... client:Agave)"
		m := regexp.MustCompile(`\s(\d+)\.\d+\.\d+`).FindSubmatch(out)
		if m == nil {
			continue
		}
		major, cErr := strconv.Atoi(string(m[1]))
		if cErr == nil && major >= minValidatorMajor {
			return path
		}
	}

	t.Skipf("no Agave %d+ validator found; creating a metadata account needs system "+
		"instruction 13 (CreateAccountAllowPrefund). Point %s at one, e.g. "+
		"sh -c \"$(curl -sSfL https://release.anza.xyz/v4.2.2/install)\"",
		minValidatorMajor, validatorEnv)

	return ""
}

func repoRoot(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../../../..")
	require.NoError(t, err)

	return abs
}

// declaredProgramID reads the address the binary was compiled against.
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
	require.NotEmpty(t, addr, "idl carries no program address")

	id, err := solana.PublicKeyFromBase58(addr)
	require.NoError(t, err)

	return id
}

func freePort(t *testing.T) int {
	t.Helper()

	var lc net.ListenConfig
	l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()

	return l.Addr().(*net.TCPAddr).Port
}

func startValidator(t *testing.T, programID solana.PublicKey, soPath string, upgradeAuthority solana.PublicKey) *rpc.Client {
	t.Helper()

	bin := testValidator(t)
	port := freePort(t)

	cmd := exec.CommandContext(t.Context(), bin,
		"--ledger", filepath.Join(t.TempDir(), "ledger"),
		"--rpc-port", strconv.Itoa(port),
		"--upgradeable-program", programID.String(), soPath, upgradeAuthority.String(),
		"--url", devnetURL,
		"--clone-upgradeable-program", ProgramID.String(),
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

	client := rpc.New(fmt.Sprintf("http://127.0.0.1:%d", port))
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := client.GetHealth(context.Background()); err == nil {
			return client
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			t.Skipf("validator exited during genesis; cloning %s from devnet needs network access", ProgramID)
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("validator did not become healthy")

	return nil
}

func waitForSig(t *testing.T, client *rpc.Client, sig solana.Signature) {
	t.Helper()

	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		res, err := client.GetSignatureStatuses(context.Background(), true, sig)
		if err == nil && len(res.Value) > 0 && res.Value[0] != nil {
			require.Nil(t, res.Value[0].Err, "transaction failed on chain: %v", res.Value[0].Err)
			switch res.Value[0].ConfirmationStatus {
			case rpc.ConfirmationStatusConfirmed, rpc.ConfirmationStatusFinalized:
				return
			case rpc.ConfirmationStatusProcessed:
				// Seen by a node but not yet durable, so keep waiting.
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", sig)
}

// send signs with every key in signers, paying with the first.
func send(t *testing.T, client *rpc.Client, signers []solana.PrivateKey, ixs ...solana.Instruction) {
	t.Helper()

	bh, err := client.GetLatestBlockhash(context.Background(), rpc.CommitmentFinalized)
	require.NoError(t, err)

	tx, err := solana.NewTransaction(ixs, bh.Value.Blockhash, solana.TransactionPayer(signers[0].PublicKey()))
	require.NoError(t, err)

	_, err = tx.Sign(func(k solana.PublicKey) *solana.PrivateKey {
		for i := range signers {
			if k.Equals(signers[i].PublicKey()) {
				return &signers[i]
			}
		}

		return nil
	})
	require.NoError(t, err)

	sig, err := client.SendTransactionWithOpts(context.Background(), tx,
		rpc.TransactionOpts{PreflightCommitment: rpc.CommitmentConfirmed})
	require.NoError(t, err)
	waitForSig(t, client, sig)
}

// programDataAddress derives the account the loader keeps a program's upgrade
// authority in. The metadata program reads it to decide whether a write is
// canonical.
func programDataAddress(t *testing.T, programID solana.PublicKey) solana.PublicKey {
	t.Helper()

	addr, _, err := solana.FindProgramAddress(
		[][]byte{programID.Bytes()}, solana.BPFLoaderUpgradeableProgramID)
	require.NoError(t, err)

	return addr
}

func TestPublishIDLOnChain(t *testing.T) {
	// TODO: stands up a solana-test-validator whose genesis clones the SPL
	// Token-Metadata program from devnet; the clone hangs when devnet
	// rate-limits. Vendor the program binary as testdata and re-enable.
	t.Skip("stands up a validator and clones a program from devnet")

	root := repoRoot(t)
	soPath := filepath.Join(root, "contracts", "target", "deploy", testProgramName+".so")
	idlPath := filepath.Join(root, "contracts", "target", "idl", testProgramName+".json")

	for _, p := range []string{soPath, idlPath} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("missing %s; run make build-contracts", filepath.Base(p))
		}
	}

	programID := declaredProgramID(t, idlPath)

	// The payer doubles as the program's upgrade authority so the publish takes
	// the canonical path. In production these are different: the upgrade
	// authority is the timelock signer PDA, and only the instructions in
	// plan.Governed() go through MCMS.
	payer, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)
	buffer, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	ctx := context.Background()
	client := startValidator(t, programID, soPath, payer.PublicKey())

	sig, err := client.RequestAirdrop(ctx, payer.PublicKey(), 10*solana.LAMPORTS_PER_SOL, rpc.CommitmentFinalized)
	require.NoError(t, err)
	waitForSig(t, client, sig)

	// Both programs must be loaded, or every failure below is noise.
	for name, id := range map[string]solana.PublicKey{"rmn_remote": programID, "program-metadata": ProgramID} {
		acct, aErr := client.GetAccountInfoWithOpts(ctx, id, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentConfirmed})
		require.NoError(t, aErr, "%s account", name)
		require.NotNil(t, acct.Value, "%s is not present", name)
		require.True(t, acct.Value.Executable, "%s is not executable", name)
		t.Logf("%-16s %s loaded", name, id)
	}

	document, err := LoadJSON(idlPath)
	require.NoError(t, err)

	plan, err := PlanPublish(PublishConfig{
		Program:     programID,
		ProgramData: programDataAddress(t, programID),
		Authority:   payer.PublicKey(),
		Payer:       payer.PublicKey(),
		Buffer:      buffer.PublicKey(),
		RentExemptFor: func(space int) (uint64, error) {
			if space < 0 {
				return 0, fmt.Errorf("negative account space %d", space)
			}

			return client.GetMinimumBalanceForRentExemption(ctx, uint64(space), rpc.CommitmentFinalized)
		},
		Seed:     SeedIDL,
		Format:   FormatJSON,
		Document: document,
	})
	require.NoError(t, err)
	t.Logf("publishing %d bytes of idl as %d compressed, in %d chunks, to %s",
		len(document), plan.PayloadLen, len(plan.Writes), plan.Metadata)

	// Fund and allocate the buffer, then fill it. In production this half runs
	// on the deployer key with no proposal.
	send(t, client, []solana.PrivateKey{payer, buffer}, plan.Prepare...)
	for i, w := range plan.Writes {
		send(t, client, []solana.PrivateKey{payer, buffer}, w)
		if (i+1)%10 == 0 || i == len(plan.Writes)-1 {
			t.Logf("  wrote chunk %d/%d", i+1, len(plan.Writes))
		}
	}

	// The governed half: these are the instructions that become MCMS
	// operations, and they are the only ones the upgrade authority signs.
	for _, ix := range plan.Governed() {
		send(t, client, []solana.PrivateKey{payer}, ix)
	}

	stored, err := client.GetAccountInfoWithOpts(ctx, plan.Metadata, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentConfirmed})
	require.NoError(t, err)
	require.NotNil(t, stored.Value, "metadata account should exist after publishing")

	acct, err := ParseAccount(stored.Value.Data.GetBinary())
	require.NoError(t, err)
	require.True(t, acct.Canonical, "publishing as the upgrade authority must produce a canonical account")
	require.Equal(t, programID, acct.Program)
	require.Equal(t, SeedIDL, acct.Seed)
	require.Equal(t, CompressionZlib, acct.Compression)
	require.Equal(t, FormatJSON, acct.Format)

	published, err := Decompress(acct.Data)
	require.NoError(t, err)

	var got, want any
	require.NoError(t, json.Unmarshal(published, &got))
	require.NoError(t, json.Unmarshal(document, &want))
	require.Equal(t, want, got, "the published idl must match the source document")

	// The buffer rent is recoverable once the document has landed.
	send(t, client, []solana.PrivateKey{payer, buffer}, plan.CloseBuffer)
	_, err = client.GetAccountInfoWithOpts(ctx, plan.Buffer, &rpc.GetAccountInfoOpts{Commitment: rpc.CommitmentConfirmed})
	require.ErrorIs(t, err, rpc.ErrNotFound, "buffer should be closed and its rent refunded")
}
