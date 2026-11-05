#!/usr/bin/env bash
# scripts/ci/ccv-ref.sh
#
# Manages the chainlink-ccv dependency: ccv-ref.yaml, every go.mod in this
# repo, and (optionally) a sibling ../chainlink-ccv checkout.
#
# Usage:
#   ccv-ref.sh pin <commit-sha>      Pin root + every yaml submodule, then validate
#   ccv-ref.sh pin-submodules        Pin yaml submodules to the root pseudo-version
#                                    already in go.mod. Standalone after manual
#                                    `go get` of the root module.
#   ccv-ref.sh pin-devenv            Deprecated alias for pin-submodules
#   ccv-ref.sh validate              Cross-check yaml, every go.mod, and sibling checkout
#   ccv-ref.sh read <key>...         Emit '<key>=<value>' lines for tracking|pinned|module
#                                    to $GITHUB_OUTPUT (or stdout if unset).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
CCV_REF_FILE="$SCRIPT_DIR/ccv-ref.yaml"
CCV_DIR="$REPO_DIR/../chainlink-ccv"

if [[ ! -f "$CCV_REF_FILE" ]]; then
  echo "ERROR: ccv-ref.yaml not found" >&2
  exit 1
fi

MODULE=$(yq -r '.module' "$CCV_REF_FILE")

# Submodules array — may be empty/missing.
SUBMODULES=()
while IFS= read -r line; do
  [[ -z "$line" || "$line" == "null" ]] && continue
  SUBMODULES+=("$line")
done < <(yq e -r '.submodules[]?' "$CCV_REF_FILE" 2>/dev/null || true)

# go.mod files pinned this run; tidied at the end (see tidy_touched_gomods).
TOUCHED_GOMODS=()

CMD="${1:?Usage: ccv-ref.sh <pin|pin-submodules|validate|read> [args]}"
shift

# find_gomods: Locates all go.mod files in the repository.
find_gomods() {
  find "$REPO_DIR" -name "go.mod"
}

# module_require_versions: Prints the required version(s) of module_path in a
# go.mod, one per line. Matches the module path as a literal string (module
# paths contain regex metacharacters like '.') and handles both block-style
# `require ( ... )` and single-line `require <module> <version>` directives.
module_require_versions() {
  awk -v mod="$2" '
    $1 == "require" && $2 == "(" { inblock = 1; next }
    inblock && $1 == ")" { inblock = 0; next }
    inblock && $1 == mod { print $2; next }
    $1 == "require" && $2 == mod { print $3 }
  ' "$1"
}

# has_module: Checks if a specific module is required by a go.mod file.
has_module() {
  module_require_versions "$1" "$2" | grep -q .
}

# module_short_sha: Extracts the 12-char SHA for a module from a go.mod file.
# Works off the version field, so `// indirect` suffixes don't hide the SHA.
module_short_sha() {
  module_require_versions "$1" "$2" | grep -oE '[0-9a-f]{12}$' | head -n1 || true
}

# pin_module_in_gomod: Pins module_path to the pseudo-version resolving to sha in
# the given go.mod, WITHOUT loading the main module's imports. 'go get' loads
# imports against a half-updated module graph, which deadlocks when an upstream
# dependency (e.g. chains/evm) moved packages between the old and new pins —
# the stale-pinned ccv module's imports no longer resolve. 'go list -m' only
# resolves the version string and 'go mod edit -require' only touches the
# require line, so pinning always succeeds; import loading is deferred to
# tidy_touched_gomods, which runs after every ccv module is pinned.
pin_module_in_gomod() {
  local gomod="$1" module_path="$2" sha="$3"
  local dir pseudo
  dir=$(dirname "$gomod")
  pseudo=$(cd "$dir" && go list -m "${module_path}@${sha}" | awk '{print $2}')
  (cd "$dir" && go mod edit -require="${module_path}@${pseudo}")
}

# tidy_touched_gomods: Runs 'go mod tidy' once per go.mod pinned this run. Runs
# last, after root + all submodules are pinned, so tidy resolves imports against
# a fully updated module graph.
tidy_touched_gomods() {
  if [[ "${#TOUCHED_GOMODS[@]}" -eq 0 ]]; then
    return 0
  fi
  local gomod dir seen=" "
  for gomod in "${TOUCHED_GOMODS[@]}"; do
    case "$seen" in
      *" $gomod "*) continue ;;
    esac
    seen+="$gomod "
    dir=$(dirname "$gomod")
    (cd "$dir" && go mod tidy)
  done
}

# pin: The main entry point for updating the CCV version everywhere.
pin() {
  local sha="${1:?Usage: ccv-ref.sh pin <commit-sha>}"
  echo "Pinning ccv to ${sha:0:12}"

  # Update the source of truth yaml file
  echo "  Updating ccv-ref.yaml..."
  yq e -i ".pinned = \"$sha\"" "$CCV_REF_FILE"

  # Update the root module in every go.mod that uses it
  echo "  Updating go.mod files for ${MODULE}..."
  while IFS= read -r gomod; do
    if has_module "$gomod" "$MODULE"; then
      echo "    $gomod"
      pin_module_in_gomod "$gomod" "$MODULE" "$sha"
      TOUCHED_GOMODS+=("$gomod")
    fi
  done < <(find_gomods)

  # Sync the sibling local checkout if it exists
  if [[ -d "$CCV_DIR" ]]; then
    echo "  Checking out ../chainlink-ccv at ${sha:0:12}..."
    (cd "$CCV_DIR" && git fetch origin && git checkout "$sha" 2>/dev/null) \
      || echo "  WARNING: could not checkout ../chainlink-ccv at $sha" >&2
  fi

  # If submodules are defined, pin them to match the root module's pseudo-version
  if [[ "${#SUBMODULES[@]}" -gt 0 ]]; then
    pin_submodules
  fi

  # Tidy only after every ccv module is pinned (see pin_module_in_gomod).
  tidy_touched_gomods

  echo "  Validating..."
  validate
}

# pin_submodules: Forces all defined submodules to the same commit as the root module.
pin_submodules() {
  if [[ "${#SUBMODULES[@]}" -eq 0 ]]; then
    echo "No submodules configured in ccv-ref.yaml; nothing to pin."
    return 0
  fi

  # Pin each submodule by commit SHA and let 'go list -m' resolve the
  # module-correct pseudo-version. Submodules have their own tag lineages and do
  # not share the root's pseudo-version base (e.g. build/devenv resolves to
  # v0.3.1-0.<ts>-<sha>, deployment to v0.1.1-0.<ts>-<sha>, integration/evm to
  # v0.0.2-0.<ts>-<sha>, all trailing the root's v0.2.1-0 lineage). Reusing the
  # root's version string would try to fetch a version that does not exist for
  # such submodules, so pin by SHA instead.
  local pinned
  pinned=$(yq -r '.pinned' "$CCV_REF_FILE")
  if [[ -z "$pinned" || "$pinned" == "null" ]]; then
    echo "ERROR: pinned ref not set in ccv-ref.yaml" >&2
    exit 1
  fi

  local sub module_path gomod
  for sub in "${SUBMODULES[@]}"; do
    module_path="${MODULE}/${sub}"
    echo "Pinning ${module_path} to ${pinned:0:12}"
    while IFS= read -r gomod; do
      if has_module "$gomod" "$module_path"; then
        echo "  $gomod"
        pin_module_in_gomod "$gomod" "$module_path" "$pinned"
        TOUCHED_GOMODS+=("$gomod")
      fi
    done < <(find_gomods)
  done
}

# validate: Ensures every go.mod file is actually using the SHA from ccv-ref.yaml.
validate() {
  local pinned
  pinned=$(yq -r '.pinned' "$CCV_REF_FILE")
  if [[ -z "$pinned" || "$pinned" == "null" ]]; then
    echo "ERROR: pinned ref not set in ccv-ref.yaml" >&2
    exit 1
  fi

  local short="${pinned:0:12}"
  local errors=0
  local warnings=0
  local gomod sha sub module_path

  # Scan every go.mod for version mismatches
  echo "Checking go.mod files against ccv-ref.yaml pinned ($short)..."
  while IFS= read -r gomod; do
    if has_module "$gomod" "$MODULE"; then
      sha=$(module_short_sha "$gomod" "$MODULE")
      if [[ -n "$sha" && "$sha" == "$short" ]]; then
        echo "  OK: $gomod (${MODULE})"
      elif [[ -n "$sha" ]]; then
        echo "  MISMATCH: $gomod (${MODULE} $sha != $short)" >&2
        errors=1
      fi
    fi

    # Also check every defined submodule
    for sub in "${SUBMODULES[@]}"; do
      module_path="${MODULE}/${sub}"
      if has_module "$gomod" "$module_path"; then
        sha=$(module_short_sha "$gomod" "$module_path")
        if [[ -n "$sha" && "$sha" == "$short" ]]; then
          echo "  OK: $gomod (${module_path})"
        elif [[ -n "$sha" ]]; then
          echo "  MISMATCH: $gomod (${module_path} $sha != $short)" >&2
          errors=1
        fi
      fi
    done
  done < <(find_gomods)

  # Check local dev checkout consistency
  if [[ -d "$CCV_DIR" ]]; then
    echo "Checking ../chainlink-ccv checkout..."
    local ccv_sha
    ccv_sha=$(cd "$CCV_DIR" && git rev-parse HEAD 2>/dev/null || true)
    if [[ -n "$ccv_sha" && "$ccv_sha" == "$pinned" ]]; then
      echo "  OK: ../chainlink-ccv at $short"
    elif [[ -n "$ccv_sha" ]]; then
      echo "  MISMATCH: ../chainlink-ccv at ${ccv_sha:0:12}, expected $short" >&2
      warnings=1
    fi
  fi

  echo ""
  if [[ "$errors" -eq 1 ]]; then
    echo "FAILED: go.mod files out of sync. Run: make pin-ccv-all REF=$pinned" >&2
    exit 1
  elif [[ "$warnings" -eq 1 ]]; then
    echo "PASSED with warnings. Run: make pin-ccv-all REF=$pinned" >&2
  else
    echo "PASSED"
  fi
}

# read_keys: Emits ccv-ref.yaml values as GITHUB_OUTPUT (or stdout).
read_keys() {
  local out="${GITHUB_OUTPUT:-/dev/stdout}"
  if [[ "$#" -eq 0 ]]; then
    echo "ERROR: read requires at least one key (tracking|pinned|module)" >&2
    exit 1
  fi
  local key value
  for key in "$@"; do
    case "$key" in
      tracking|pinned|module) ;;
      *) echo "ERROR: unknown key '$key' (expected tracking|pinned|module)" >&2; exit 1 ;;
    esac
    value=$(yq e -r ".${key}" "$CCV_REF_FILE")
    if [[ -z "$value" || "$value" == "null" ]]; then
      echo "ERROR: .${key} not set in $CCV_REF_FILE" >&2
      exit 1
    fi
    echo "${key}=${value}" >> "$out"
  done
}

case "$CMD" in
  pin)             pin "$@" ;;
  pin-submodules)  pin_submodules && tidy_touched_gomods ;;
  validate)        validate ;;
  read)            read_keys "$@" ;;
  *)               echo "Usage: ccv-ref.sh <pin|pin-submodules|validate|read>" >&2; exit 1 ;;
esac
