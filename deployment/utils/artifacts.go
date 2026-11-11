package utils //nolint:revive,nolintlint

// NOTE: taken from chainlink-ton/deployment/utils/artifacts.go, dedup in the future

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	// Limit decompressed size to 100MB (adjust as needed)
	maxDecompressedSize = 100 * 1024 * 1024
)

// githubBaseURL is the base URL for downloading release artifacts.
// Tests can override this to point at an httptest.Server.
var githubDomain = "github.com"
var githubBaseURL = "https://" + githubDomain

// Artifact is a generic single file from a contracts package.
// It can be reused by other chain implementations.
type Artifact struct {
	Filename string
	Data     []byte
}

// DownloadArtifactsOpts specifies what to download and where to cache it.
type DownloadArtifactsOpts struct {
	Host         string
	Organization string
	Repository   string
	Release      string
	Asset        string
	PkgsDir      string // base directory for the local package cache; empty = use default
	// Verify, when set, must accept the extracted package before it is cached or
	// returned. It runs on freshly downloaded packages and on cache hits; a cached
	// package it rejects is discarded and downloaded again.
	Verify func(dir string) error
}

// DownloadArtifacts fetches a release tar.gz from GitHub and extracts it to a local directory.
// The destination is derived deterministically from the input fields under PkgsDir.
// If the destination directory already exists and passes verification, the download is skipped (disk cache).
// Returns local path where the package was extracted into after download.

func DownloadArtifacts(ctx context.Context, in DownloadArtifactsOpts) (string, error) {
	if in.Host != githubDomain && in.Host != githubBaseURL {
		return "", fmt.Errorf("expected %s or %s as a host for remote releases, got %s", githubDomain, githubBaseURL, in.Host)
	}

	destDir, err := packageDestDir(in)
	if err != nil {
		return "", err
	}

	// Cache check: a package is only reused if it passes verification. Anything else
	// (empty, partial, corrupt or from an older broken version) is discarded.
	if _, statErr := os.Stat(destDir); statErr == nil {
		if verifyErr := verifyPackage(in, destDir); verifyErr == nil {
			return destDir, nil
		}
		if err = os.RemoveAll(destDir); err != nil {
			return "", fmt.Errorf("failed to remove invalid cached package %s: %w", destDir, err)
		}
	}

	downloadURL := artifactDownloadURL(in)

	rawTarGz, err := getBytesFromURL(ctx, downloadURL, os.Getenv("GITHUB_TOKEN"))
	if err != nil {
		return "", fmt.Errorf("failed to download artifacts from %s: %w", downloadURL, err)
	}

	parent := filepath.Dir(destDir)
	if err = os.MkdirAll(parent, 0o755); err != nil {
		return "", fmt.Errorf("failed to create cache directory %s: %w", parent, err)
	}

	// Extract into a private temp directory and publish it with one rename, so the
	// destination either does not exist or is complete and verified, even if this
	// process dies mid-way or another one is populating the same cache entry.
	tmpDir, err := os.MkdirTemp(parent, filepath.Base(destDir)+".tmp-")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory in %s: %w", parent, err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }() // no-op once renamed

	if err = extractFilesToDir(rawTarGz, tmpDir); err != nil {
		return "", fmt.Errorf("failed to extract artifacts from %s: %w", downloadURL, err)
	}
	if err = verifyPackage(in, tmpDir); err != nil {
		return "", fmt.Errorf("artifacts from %s failed verification: %w", downloadURL, err)
	}

	if err = os.Rename(tmpDir, destDir); err != nil {
		// Another process may have published the same package first.
		if verifyErr := verifyPackage(in, destDir); verifyErr == nil {
			return destDir, nil
		}
		return "", fmt.Errorf("failed to publish artifacts to %s: %w", destDir, err)
	}

	return destDir, nil
}

// verifyPackage requires a non-empty package that passes the caller's Verify hook.
func verifyPackage(in DownloadArtifactsOpts, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return errors.New("package is empty")
	}
	if in.Verify != nil {
		return in.Verify(dir)
	}
	return nil
}

func artifactDownloadURL(in DownloadArtifactsOpts) string {
	return fmt.Sprintf(
		"%s/%s/%s/releases/download/%s/%s",
		githubBaseURL, in.Organization, in.Repository, url.PathEscape(in.Release), in.Asset,
	)
}

// packageDestDir returns the directory where a downloaded package should be stored.
func packageDestDir(in DownloadArtifactsOpts) (string, error) {
	base := in.PkgsDir
	if base == "" {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("failed to determine user cache dir: %w", err)
		}
		base = filepath.Join(cacheDir, "chainlink-ccip-solana", "packages")
	}
	// Derive a stable, filesystem-safe name from org/repo@release.
	name := sanitizePackageName(fmt.Sprintf("%s-%s-%s", in.Organization, in.Repository, in.Release))
	return filepath.Join(base, name), nil
}

// sanitizePackageName replaces characters that are problematic in directory names.
func sanitizePackageName(s string) string {
	return strings.NewReplacer("/", "_", "@", "_", " ", "_", "\\", "_").Replace(s)
}

// AssetNameFromReleaseTag derives the release asset filename from a release tag.
// Convention: replace "/" and "+" with "-" and append ".tar.gz".
// For example, "contracts/1.6.0" → "contracts-1.6.0.tar.gz".
// For example, "contracts/1.6.0+abc123" → "contracts-1.6.0-abc123.tar.gz".
func AssetNameFromReleaseTag(tag string) string {
	tag = strings.ReplaceAll(tag, "/", "-")
	tag = strings.ReplaceAll(tag, "+", "-")
	return tag + ".tar.gz"
}

// extractFilesToDir extracts all regular files from rawTarGz into destDir,
// preserving their relative paths (e.g. target/deploy/x.so).
func extractFilesToDir(rawTarGz []byte, destDir string) error {
	gzipReader, err := gzip.NewReader(bytes.NewReader(rawTarGz))
	if err != nil {
		return err
	}
	defer func() { _ = gzipReader.Close() }()

	tarReader := tar.NewReader(io.LimitReader(gzipReader, maxDecompressedSize))

	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		if !isSafeRelPath(header.Name) {
			continue
		}

		clean := filepath.Clean(header.Name)
		destPath := filepath.Join(destDir, clean)

		data, err := readLimited(tarReader, maxDecompressedSize, clean)
		if err != nil {
			return err
		}

		if err = os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", destPath, err)
		}

		if err = os.WriteFile(destPath, data, 0o600); err != nil {
			return fmt.Errorf("failed to write %s: %w", destPath, err)
		}
	}

	return nil
}

// isSafeRelPath returns true if name is a relative path that stays inside the extraction
// directory (not absolute, no ".." components). It does not filter by content type.
func isSafeRelPath(name string) bool {
	clean := filepath.Clean(name)
	if clean == "" || clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) {
		return false
	}
	if slices.Contains(strings.Split(filepath.ToSlash(clean), "/"), "..") {
		return false
	}
	return true
}

func readLimited(r io.Reader, limit int64, name string) ([]byte, error) {
	var buf bytes.Buffer

	n, err := io.Copy(&buf, io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("error while read %q: %w", name, err)
	}
	if n > limit {
		return nil, fmt.Errorf("file %q exceeds size limit", name)
	}

	return buf.Bytes(), nil
}

func getBytesFromURL(ctx context.Context, url, githubToken string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+githubToken)
	}

	cl := &http.Client{Timeout: 90 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GET %s responded with an error: %s: %s", url, resp.Status, string(b))
	}

	return io.ReadAll(resp.Body)
}
