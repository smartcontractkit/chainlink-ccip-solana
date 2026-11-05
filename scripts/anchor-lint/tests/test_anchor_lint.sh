#!/usr/bin/env bash
# Test runner for anchor-lint.py
# Runs the linter against the test fixture and asserts expected finding counts.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/../src/anchor-lint.py"
FIXTURE="$SCRIPT_DIR/resources"

echo "=== Running anchor-lint.py against test fixture ==="

# Run linter, capture output (it will exit 1 due to errors — that's expected)
output=$(python3 "$LINT" --root "$FIXTURE" --no-baseline 2>&1 || true)
echo "$output"
echo ""

# Count findings per check ID by looking for the [error/warning/info] + check-id pattern
count_check() {
    local check_id="$1"
    # Each finding line contains the check ID in bold; count occurrences
    echo "$output" | grep -F -c "$check_id" || true
}

pass=true

assert_count() {
    local check_id="$1"
    local expected="$2"
    local actual
    actual=$(count_check "$check_id")
    if [ "$actual" -eq "$expected" ]; then
        echo "  ✓ $check_id: $actual (expected $expected)"
    else
        echo "  ✗ $check_id: $actual (expected $expected)"
        pass=false
    fi
}

echo "=== Checking expected finding counts ==="
assert_count "INV-LINT-SERIAL"  1
assert_count "INV-LINT-ASSERT"  1
assert_count "INV-LINT-EVENT"   2
assert_count "INV-LINT-SIGNER"  2
assert_count "INV-LINT-INIT"    1
assert_count "INV-LINT-PDA"     1
assert_count "INV-LINT-CLONE"   1
assert_count "INV-LINT-ARITH"   1
assert_count "INV-LINT-HEAP"    2
assert_count "INV-LINT-CAST"    1
assert_count "INV-LINT-UNWRAP"  1
assert_count "INV-LINT-REALLOC" 1
assert_count "INV-LINT-PARAMS"  1
assert_count "INV-LINT-ACCTINIT" 4
assert_count "INV-LINT-ATAINIT" 2
assert_count "INV-LINT-REMACCT" 1
assert_count "INV-LINT-CREATEACCT" 1
assert_count "INV-LINT-SET-INNER" 2
echo ""

# ── SAFETY: suppression window ──────────────────────────────────────────────
# hot.rs has three heap allocations; the third carries a `// SAFETY:` comment on
# the preceding line and must not be reported. Guards the suppression window
# (preceding 3 / following 2 lines) that HEAP shares with CAST/UNWRAP/ARITH/CLONE.
echo ""
echo "=== Checking SAFETY: suppression of INV-LINT-HEAP ==="
safety_line=$(grep -n "data.to_vec()" "$FIXTURE/programs/test-lint/src/ccip_send/hot.rs" | tail -1 | cut -d: -f1)
if echo "$output" | grep -F "INV-LINT-HEAP" | grep -q "hot.rs:$safety_line"; then
    echo "  ✗ SAFETY:-annotated allocation at hot.rs:$safety_line was reported"
    pass=false
else
    echo "  ✓ SAFETY:-annotated allocation at hot.rs:$safety_line suppressed"
fi

# ── Baseline mechanism ──────────────────────────────────────────────────────
# Suppression lives in a JSON baseline instead of source comments. Verify that a
# baseline entry absorbs exactly `count` findings and that one extra is reported.
BASELINE_TMP="$SCRIPT_DIR/.baseline-test.json"
trap 'rm -f "$BASELINE_TMP"' EXIT

cat > "$BASELINE_TMP" <<'JSON'
{
  "suppressions": [
    { "file": "programs/test-lint/src/ccip_send/hot.rs", "check": "INV-LINT-HEAP", "lines": [25] }
  ]
}
JSON

echo ""
echo "=== Checking baseline suppression ==="
baseline_out=$(python3 "$LINT" --root "$FIXTURE" --baseline "$BASELINE_TMP" 2>&1 || true)
baseline_heap=$(echo "$baseline_out" | grep -F -c "INV-LINT-HEAP" || true)
# 2 findings in the file; the baseline names line 25 only, so exactly 1 must remain.
if [ "$baseline_heap" -eq 1 ]; then
    echo "  ✓ baseline absorbed 1 of 2 INV-LINT-HEAP findings (1 reported)"
else
    echo "  ✗ baseline: expected 1 remaining INV-LINT-HEAP, got $baseline_heap"
    pass=false
fi

# A listed line with no finding on it is stale and must be flagged, so the baseline
# gets pruned rather than silently masking a future regression.
cat > "$BASELINE_TMP" <<'JSON'
{
  "suppressions": [
    { "file": "programs/test-lint/src/ccip_send/hot.rs", "check": "INV-LINT-HEAP", "lines": [25, 9999] }
  ]
}
JSON
stale_out=$(python3 "$LINT" --root "$FIXTURE" --baseline "$BASELINE_TMP" 2>&1 || true)
if echo "$stale_out" | grep -q "stale baseline entr"; then
    echo "  ✓ stale baseline entry reported"
else
    echo "  ✗ stale baseline entry was not reported"
    pass=false
fi

echo ""
if [ "$pass" = true ]; then
    echo "=== All assertions passed ✓ ==="
    exit 0
else
    echo "=== Some assertions FAILED ✗ ==="
    exit 1
fi
