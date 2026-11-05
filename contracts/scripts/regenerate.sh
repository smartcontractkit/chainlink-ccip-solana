#!/usr/bin/env bash
set -euxo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
echo "Generating program IDs..."
pushd ..
# Regenerate program IDs
go run scripts/generate_keys.go
popd
cp target/deploy/ccip_router-keypair.json target/deploy/ccip_common-keypair.json
anchor keys sync
echo "Rebuilding programs..."
bash "./scripts/verify_build.sh"
