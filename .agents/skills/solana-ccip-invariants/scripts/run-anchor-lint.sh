#!/usr/bin/env bash
# Run anchor-lint on Solana CCIP contracts
#
# Usage:
#   ./run-anchor-lint.sh                     # Check all programs
#   ./run-anchor-lint.sh --strict            # CI mode (exit 1 on any finding)
#   ./run-anchor-lint.sh --only INV-LINT-PDA # Check specific rule(s)
#   ./run-anchor-lint.sh --list-checks       # List all checks
#
# Exit codes:
#   0 - No errors (warnings/info allowed)
#   1 - Errors found (or any finding with --strict)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
LINT_SCRIPT="${REPO_ROOT}/scripts/anchor-lint/src/anchor-lint.py"
PROGRAMS_DIR="${REPO_ROOT}/contracts/programs"

if [[ ! -f "$LINT_SCRIPT" ]]; then
    echo "Error: anchor-lint.py not found at $LINT_SCRIPT" >&2
    exit 1
fi

echo "Running anchor-lint on: $PROGRAMS_DIR"
echo "---"

python3 "$LINT_SCRIPT" --root "$PROGRAMS_DIR" "$@"
