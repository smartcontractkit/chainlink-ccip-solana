#!/usr/bin/env bash
# Brings the generated files in line with the program version after a bump.
#
# release-please only bumps [workspace.package] version in Cargo.toml. Run this
# on the release PR branch, then commit and push the result before merging.
set -euo pipefail
cd "$(dirname "$0")/.."

version=$(cargo metadata --no-deps --offline --format-version 1 | jq -r '
  [.packages[] | select(.manifest_path | contains("/programs/")) | .version] | unique
  | if length == 1 then .[0] else error("programs disagree on version: \(.)") end')
echo "Syncing generated files to $version"

cargo update --workspace --offline

for f in target/idl/*.json target/types/*.ts; do
    if [ "$(grep -c '^    "version": "[^"]*",$' "$f")" != 1 ]; then
        echo "$f: expected exactly one metadata version line" >&2
        exit 1
    fi
    VERSION="$version" perl -pi -e 's/^    "version": "[^"]*",$/    "version": "$ENV{VERSION}",/' "$f"
done
