#!/usr/bin/env bash
# Build .golangci.yml = upstream chainlink config + this repo's overrides.
# Overrides live in .golangci.local.yml; this deep-merges them (arrays appended).
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."

upstream_url="${GOLANGCI_CONFIG_URL:-https://raw.githubusercontent.com/smartcontractkit/chainlink/develop/.golangci.yml}"

tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT

{
  printf '%s\n' \
    '# GENERATED FILE - DO NOT EDIT.' \
    '# Built by scripts/ci/fetch-golangci-config.sh (upstream chainlink config + .golangci.local.yml).' \
    '# Edit .golangci.local.yml, then run: make fetch-golangci-config'
  # Merge is append-only; drop upstream entries this repo must not inherit.
  curl -fsSL "$upstream_url" | yq '
    . *+ load(".golangci.local.yml")
    | del(.formatters.settings.gci.sections[] | select(. == "prefix(github.com/smartcontractkit/chainlink)"))
  ' -
} > "$tmp"

mv "$tmp" .golangci.yml
