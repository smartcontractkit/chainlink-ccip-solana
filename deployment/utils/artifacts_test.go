package utils

import "testing"

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
