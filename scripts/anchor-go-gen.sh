#!/usr/bin/env bash

set -e

# Delete all .go files (anchor-go v1.0.0 generates all needed types directly)
find ./contracts/gobindings/v2/latest -name "*.go" -type f -delete

# anchor-go internally runs "go mod tidy", "go fmt", and "go build" after generating
# each package and panics on failure. Since we generate into subdirs of an existing
# go.mod (using -no-go-mod), these commands either fail or are redundant.
# Create a wrapper that no-ops them so anchor-go completes, then we run go fmt at the end.
REAL_GO=$(which go)
GO_WRAPPER_DIR=$(mktemp -d)
trap 'rm -rf "$GO_WRAPPER_DIR"' EXIT
cat > "$GO_WRAPPER_DIR/go" << GOWRAPPER
#!/bin/bash
if [ "\$1" = "fmt" ] || [ "\$1" = "build" ] || [ "\$1" = "mod" ]; then
  exit 0
fi
exec "$REAL_GO" "\$@"
GOWRAPPER
chmod +x "$GO_WRAPPER_DIR/go"

function generate_bindings() {
  local idl_path_str="$1"
  IFS='/' read -r -a idl_path <<< "${idl_path_str}"
  IFS='.' read -r -a idl_name <<< "${idl_path[3]}"
  PATH="$GO_WRAPPER_DIR:$PATH" anchor-go -idl "${idl_path_str}" -output ./contracts/gobindings/v2/latest/"${idl_name}" -no-go-mod
}

# Generate bindings for all IDLs (including vendor)
for idl_path_str in "contracts/target/idl"/*
do
  generate_bindings "${idl_path_str}"
done

# Fix vendor IDL instruction discriminators: The vendor .so files were compiled with
# old Anchor (snake_case discriminators), but the vendor IDL JSON files use the new
# Anchor format (camelCase discriminators). Recompute instruction discriminators using
# snake_case naming to match the deployed .so binaries.
python3 - << 'PYEOF'
import hashlib, json, re, glob

def camel_to_snake(name):
    s1 = re.sub('(.)([A-Z][a-z]+)', r'\1_\2', name)
    return re.sub('([a-z0-9])([A-Z])', r'\1_\2', s1).lower()

for path in glob.glob('contracts/target/vendor/*.json'):
    with open(path) as f:
        idl = json.load(f)
    modified = False
    for ix in idl.get('instructions', []):
        snake = camel_to_snake(ix['name'])
        correct = list(hashlib.sha256(f'global:{snake}'.encode()).digest()[:8])
        if ix['discriminator'] != correct:
            ix['discriminator'] = correct
            modified = True
    if modified:
        with open(path, 'w') as f:
            json.dump(idl, f, indent=2)
            f.write('\n')
PYEOF

for idl_path_str in "contracts/target/vendor"/*.json
do
  generate_bindings "${idl_path_str}"
done

# Restore original vendor IDLs (we only modified them temporarily for generation)
git checkout -- contracts/target/vendor/*.json

# gobindings/v2/ is a separate go.mod; go fmt must cd in to reach it.
(cd contracts/gobindings/v2 && go fmt ./...)
