package sequences

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/smartcontractkit/chainlink-ccip-solana/deployment/solcommon"
)

// executableHashesFile lists the solana-verify executable hash of every program in a release.
const executableHashesFile = "executable-hashes.txt"

// verifyReleaseArtifacts returns the integrity check for an extracted release tarball.
// It proves the directory is complete and is the release that was asked for:
//   - provenance.json exists and names exactly the requested release,
//   - every target/deploy/*.so has a hash in executable-hashes.txt that matches its
//     bytes (sha256 with trailing zero padding removed, as solana-verify computes it),
//   - no program is listed without a binary, or shipped without a hash.
func verifyReleaseArtifacts(release string) func(dir string) error {
	return func(dir string) error {
		target := filepath.Join(dir, "target")

		prov, ok, err := solcommon.ReadProvenance(target)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("missing %s", filepath.Join("target", solcommon.ProvenanceFile))
		}
		if prov.Release != release {
			return fmt.Errorf("provenance is for release %q, expected %q", prov.Release, release)
		}

		raw, err := os.ReadFile(filepath.Join(target, executableHashesFile))
		if err != nil {
			return fmt.Errorf("read %s: %w", executableHashesFile, err)
		}
		want := map[string]string{}
		for line := range strings.SplitSeq(string(raw), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			if len(fields) != 2 {
				return fmt.Errorf("malformed line %q in %s", line, executableHashesFile)
			}
			want[fields[1]] = fields[0]
		}

		binaries, err := filepath.Glob(filepath.Join(target, "deploy", "*.so"))
		if err != nil {
			return err
		}
		if len(binaries) == 0 {
			return fmt.Errorf("no programs under %s", filepath.Join("target", "deploy"))
		}
		for _, bin := range binaries {
			name := filepath.Base(bin)
			expected, listed := want[name]
			if !listed {
				return fmt.Errorf("%s has no entry in %s", name, executableHashesFile)
			}
			data, readErr := os.ReadFile(bin)
			if readErr != nil {
				return readErr
			}
			sum := sha256.Sum256(bytes.TrimRight(data, "\x00"))
			if got := hex.EncodeToString(sum[:]); got != expected {
				return fmt.Errorf("%s hash %s does not match %s in %s", name, got, expected, executableHashesFile)
			}
			delete(want, name)
		}
		for name := range want {
			return fmt.Errorf("%s is listed in %s but missing from the release", name, executableHashesFile)
		}
		return nil
	}
}
