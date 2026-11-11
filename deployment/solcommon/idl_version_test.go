package solcommon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/require"
)

func TestIDLVersion(t *testing.T) {
	write := func(content string) string {
		path := filepath.Join(t.TempDir(), "idl.json")
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		return path
	}

	got, err := IDLVersion(write(`{"metadata":{"name":"ccip_offramp","version":"2.0.1","spec":"0.1.0"}}`))
	require.NoError(t, err)
	require.Equal(t, "2.0.1", got.String())

	_, err = IDLVersion(write(`{"metadata":{"name":"ccip_offramp"}}`))
	require.ErrorContains(t, err, "metadata.version")
	_, err = IDLVersion(filepath.Join(t.TempDir(), "missing.json"))
	require.Error(t, err)
}

func TestRequireIDLVersionEqual(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "prog.so"), []byte("elf"), 0o600))
	idl := filepath.Join(dir, "prog.json")
	require.NoError(t, os.WriteFile(idl, []byte(`{"metadata":{"version":"2.0.1"}}`), 0o600))

	require.NoError(t, RequireIDLVersionEqual(dir, "prog", semver.MustParse("2.0.1")))
	require.ErrorContains(t, RequireIDLVersionEqual(dir, "prog", semver.MustParse("2.0.0")), "IDL declares 2.0.1")
	require.ErrorContains(t, RequireIDLVersionEqual(dir, "prog", semver.MustParse("3.0.1")), "IDL declares 2.0.1")

	require.NoError(t, os.Remove(idl))
	require.NoError(t, RequireIDLVersionEqual(dir, "prog", semver.MustParse("2.0.0")), "no IDL, nothing to compare")
}
