package sequences

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeRelease(t *testing.T, release string, binary []byte, hash string) string {
	t.Helper()
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	require.NoError(t, os.MkdirAll(filepath.Join(target, "deploy"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "deploy", "p.so"), binary, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(target, "provenance.json"),
		[]byte(`{"release":"`+release+`","commit":"b9ee948f0080184d4e8eef77670eee459f2a68b6"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(target, "executable-hashes.txt"), []byte(hash+"  p.so\n"), 0o600))
	return dir
}

func TestVerifyReleaseArtifacts(t *testing.T) {
	bin := append([]byte("program-bytes"), 0, 0, 0) // padded like a real .so
	sum := sha256.Sum256([]byte("program-bytes"))
	good := hex.EncodeToString(sum[:])

	require.NoError(t, verifyReleaseArtifacts("contracts/v2.0.1")(writeRelease(t, "contracts/v2.0.1", bin, good)))
	require.ErrorContains(t, verifyReleaseArtifacts("contracts/v2.0.1")(writeRelease(t, "contracts/v2.0.0", bin, good)), "expected")
	require.ErrorContains(t, verifyReleaseArtifacts("contracts/v2.0.1")(writeRelease(t, "contracts/v2.0.1", bin, hex.EncodeToString(make([]byte, 32)))), "does not match")
	require.ErrorContains(t, verifyReleaseArtifacts("contracts/v2.0.1")(t.TempDir()), "missing")
}

func TestRemoveStaleReleaseArtifacts(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"old.so", "old.json", "provenance.json", "fee_quoter-keypair.json", "notes.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o600))
	}
	require.NoError(t, removeStaleReleaseArtifacts(dir))
	for _, gone := range []string{"old.so", "old.json", "provenance.json"} {
		require.NoFileExists(t, filepath.Join(dir, gone))
	}
	require.FileExists(t, filepath.Join(dir, "fee_quoter-keypair.json"))
	require.FileExists(t, filepath.Join(dir, "notes.txt"))
	require.NoError(t, removeStaleReleaseArtifacts(filepath.Join(dir, "missing")))
}
