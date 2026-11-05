#!/usr/bin/env bash
# Render load test metrics JSON files as markdown (job summary + PR comment).
set -euo pipefail

METRICS_DIR="${1:?metrics directory}"
CCV_REF="${2:?ccv ref}"
RUN_URL="${3:-}"
ENV_LABEL="${4:-}"

# Cap the rendered message ID list so huge failure runs cannot exceed GitHub's
# comment size limit; the full list is in the load-metrics artifact.
MAX_LISTED_FAILURES=50

row_from_file() {
  local file="$1"
  local fallback_label="$2"
  if [[ ! -f "$file" ]]; then
    echo "| ${fallback_label} | — | — | — | — | — | — | skipped |"
    return
  fi
  jq -r --arg fb "$fallback_label" '
    def payload_mix:
      if (.message_profile_mix // [] | length) == 0 then
        "N/A"
      else
        (.message_profile_mix | map(
          "\((.expected_pct * 10 | round) / 10)% \(.name)"
          + (if .data_bytes > 0 then " (\(.data_bytes) B)" else "" end)
        ) | join(", "))
      end;
    def gun_fails:
      ((.gun_stats.failed // 0) | tostring)
      + (if (.gun_stats.call_timeout // 0) > 0 then " (+\(.gun_stats.call_timeout) timeouts)" else "" end);
    "| \(.direction // $fb) | \(.total_sent)/\(.total_received) | \(gun_fails) | \((.failed_messages // []) | length) | \(.p90_latency_sec | . * 100 | round / 100)s | \(.max_latency_sec | . * 100 | round / 100)s | \(payload_mix) | \(if .passed then "pass" else "fail" end) |"
  ' "$file"
}

# failures_details renders a collapsible list of failed message IDs (sent but
# never executed on the destination) for one direction, if any failed.
failures_details() {
  local file="$1"
  local label="$2"
  [[ -f "$file" ]] || return 0
  local count
  count=$(jq '(.failed_messages // []) | length' "$file")
  [[ "$count" -gt 0 ]] || return 0
  echo "<details><summary>Failed message IDs — ${label} (${count})</summary>"
  echo
  echo '```text'
  jq -r --argjson cap "$MAX_LISTED_FAILURES" \
    '(.failed_messages // [])[:$cap][] | "seqNo=\(.seq_no) messageID=\(.message_id)"' "$file"
  if [[ "$count" -gt "$MAX_LISTED_FAILURES" ]]; then
    echo "... and $((count - MAX_LISTED_FAILURES)) more (see load-metrics artifact)"
  fi
  echo '```'
  echo "</details>"
  echo
}

evm_file="${METRICS_DIR}/load-metrics-TestEVM2SolanaLoad.json"
sol_file="${METRICS_DIR}/load-metrics-TestSolana2EVMLoad.json"

{
  if [[ -n "$ENV_LABEL" ]]; then
    echo "## CCIP Load Test Results ($ENV_LABEL)"
  else
    echo "## CCIP Load Test Results"
  fi
  echo
  if [[ -n "$RUN_URL" ]]; then
    echo "Run: [workflow](${RUN_URL})"
    echo "CCV pin: \`${CCV_REF}\`"
    echo
  fi
  echo "| Direction | Sent/Recv | Send fails | Failed msgs | P90 | Max | Payload mix | Status |"
  echo "|-----------|-----------|------------|-------------|-----|-----|-------------|--------|"
  row_from_file "$evm_file" "EVM→Sol"
  row_from_file "$sol_file" "Sol→EVM"
  echo
  failures_details "$evm_file" "EVM→Sol"
  failures_details "$sol_file" "Sol→EVM"
  echo "_Send fails are gun-level send errors (message never sent); Failed msgs were sent but never executed on the destination (full list in the load-metrics artifact). Payload mix is the configured profile ratio; actual counts vary slightly due to per-send random selection._"
}
