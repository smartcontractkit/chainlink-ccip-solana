package utils

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestArtifactDownloadURLWithContractsTag(t *testing.T) {
	got := artifactDownloadURL(DownloadArtifactsOpts{
		Organization: "smartcontractkit",
		Repository:   "chainlink-ccip-solana",
		Release:      "contracts/v2.0.0",
		Asset:        "artifacts.tar.gz",
	})
	want := "https://github.com/smartcontractkit/chainlink-ccip-solana/releases/download/contracts%2Fv2.0.0/artifacts.tar.gz"
	if got != want {
		t.Fatalf("artifactDownloadURL() = %q, want %q", got, want)
	}
}

func TestExtractFilesToDirKeepsNestedPaths(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{
		"target/deploy/a.so":     "bin",
		"target/provenance.json": "{}",
		"../escape.txt":          "x",
		"/abs.txt":               "x",
	} {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(body)), Typeflag: tar.TypeReg}))
		_, err := tw.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())

	dir := t.TempDir()
	require.NoError(t, extractFilesToDir(buf.Bytes(), dir))
	require.FileExists(t, filepath.Join(dir, "target/deploy/a.so"))
	require.FileExists(t, filepath.Join(dir, "target/provenance.json"))
	require.NoFileExists(t, filepath.Join(dir, "../escape.txt"))
	require.NoFileExists(t, filepath.Join(dir, "abs.txt"))
}

func TestDownloadArtifactsRedownloadsEmptyCache(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "target/provenance.json", Mode: 0o600, Size: 2, Typeflag: tar.TypeReg}))
	_, err := tw.Write([]byte("{}"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()

	oldBase, oldDomain := githubBaseURL, githubDomain
	githubBaseURL, githubDomain = srv.URL, "github.com"
	defer func() { githubBaseURL, githubDomain = oldBase, oldDomain }()

	opts := DownloadArtifactsOpts{Host: "github.com", Organization: "o", Repository: "r", Release: "contracts/v2.0.1", Asset: "a.tar.gz", PkgsDir: t.TempDir()}
	dest, err := packageDestDir(opts)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dest, 0o755)) // empty leftover from a broken run

	got, err := DownloadArtifacts(context.Background(), opts)
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(got, "target/provenance.json"))
}

func tarGzOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(body)), Typeflag: tar.TypeReg}))
		_, err := tw.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func TestDownloadArtifactsVerification(t *testing.T) {
	payload := tarGzOf(t, map[string]string{"target/provenance.json": "{}"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	defer srv.Close()
	oldBase, oldDomain := githubBaseURL, githubDomain
	githubBaseURL, githubDomain = srv.URL, "github.com"
	defer func() { githubBaseURL, githubDomain = oldBase, oldDomain }()

	opts := DownloadArtifactsOpts{Host: "github.com", Organization: "o", Repository: "r", Release: "contracts/v2.0.1", Asset: "a.tar.gz", PkgsDir: t.TempDir()}
	dest, err := packageDestDir(opts)
	require.NoError(t, err)

	// A package that fails verification is never published to the cache.
	opts.Verify = func(string) error { return errors.New("bad") }
	_, err = DownloadArtifacts(context.Background(), opts)
	require.ErrorContains(t, err, "failed verification")
	require.NoDirExists(t, dest)
	entries, err := os.ReadDir(filepath.Dir(dest))
	require.NoError(t, err)
	require.Empty(t, entries, "temporary directories are cleaned up")

	// A cached package that no longer verifies is replaced by a fresh download.
	require.NoError(t, os.MkdirAll(dest, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dest, "partial"), []byte("x"), 0o600))
	opts.Verify = func(dir string) error {
		_, statErr := os.Stat(filepath.Join(dir, "target", "provenance.json"))
		return statErr
	}
	got, err := DownloadArtifacts(context.Background(), opts)
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(got, "target", "provenance.json"))
	require.NoFileExists(t, filepath.Join(got, "partial"))
}
