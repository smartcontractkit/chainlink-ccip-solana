#!/usr/bin/env bash
# scripts/ci/ci-helpers.sh
#
# Workflow-shaped helpers for the chainlink-ccv tracking + auto-bump stack.
# Each subcommand replaces an inline bash block in .github/workflows/*.yml.
# Single-value subcommands emit `key=value` lines to ${GITHUB_OUTPUT:-/dev/stdout}.
# Multi-line subcommands (changelogs) write markdown to stdout by default, or to
# ${GITHUB_OUTPUT:-/dev/stdout} as a heredoc when an output key is provided.
#
# Usage:
#   ci-helpers.sh check-upstream
#       Reads $PINNED, $MODULE, $TRACKING; resolves upstream tracking ref via git ls-remote.
#       Emits: needs_bump, latest_sha, latest_short, compare_url
#
#   ci-helpers.sh changelogs <repo> <from-sha> <to-sha> [output-key]
#       Lists added changelog/* files in the upstream diff. Emits markdown bullets
#       to stdout, or writes them to GITHUB_OUTPUT under [output-key] when provided.
#
#   ci-helpers.sh manual-commit-guard <branch> [base-branch]
#       Inspects auto-PR branch for commits authored by anyone other than the
#       github-actions[bot]. Emits: should_skip, pr_number

set -euo pipefail

CMD="${1:?Usage: ci-helpers.sh <subcommand> [args...]}"
shift

# Bot commit author email suffixes — github-actions[bot] (default) and the GATI
# app-token-issuer bot (used when sign-commits: true rewrites the author).
BOT_EMAIL_SUFFIXES=(
  '+github-actions[bot]@users.noreply.github.com'
  '+app-token-issuer-integrations[bot]@users.noreply.github.com'
)

check_upstream() {
  local out="${GITHUB_OUTPUT:-/dev/stdout}"
  : "${PINNED:?PINNED env required}"
  : "${MODULE:?MODULE env required}"
  : "${TRACKING:?TRACKING env required}"

  local repo_url latest
  repo_url="https://github.com/${MODULE#*/}"
  latest=$(git ls-remote "$repo_url" "$TRACKING" | awk '{print $1}')
  if [[ -z "$latest" ]]; then
    echo "ERROR: could not resolve ${MODULE} ${TRACKING} HEAD" >&2
    exit 1
  fi

  {
    if [[ "$latest" == "$PINNED" ]]; then
      echo "needs_bump=false"
    else
      echo "needs_bump=true"
    fi
    echo "latest_sha=${latest}"
    echo "latest_short=${latest:0:12}"
    echo "compare_url=${repo_url}/compare/${PINNED}...${latest}"
  } >> "$out"
}

changelogs() {
  local repo="${1:?repo required}"
  local from="${2:?from-sha required}"
  local to="${3:?to-sha required}"
  local key="${4:-}"
  local out="${GITHUB_OUTPUT:-/dev/stdout}"
  local target="${key:+$out}" # Redirect to $out only if $key is set
  target="${target:-/dev/stdout}" # Default to stdout if no key

  # Get added changelog/* files via GitHub API
  local files
  files=$(gh api "repos/${repo}/compare/${from}...${to}" \
    --jq '.files[]? | select((.filename | startswith("changelog/")) and .status == "added") | .filename' \
    2>/dev/null || true)

  # Start GITHUB_OUTPUT heredoc if key provided
  if [[ -n "$key" ]]; then
    echo "${key}<<CHANGELOG_EOF" >> "$out"
  fi

  if [[ -z "$files" ]]; then
    echo "_no new changelog entries_" >> "$target"
    [[ -z "$key" ]] || echo "" >> "$out" # newline for EOF
  else
    # Format files as markdown links
    while IFS= read -r f; do
      [[ -z "$f" ]] && continue
      echo "- [\`$f\`](https://github.com/${repo}/blob/${to}/$f)" >> "$target"
    done <<< "$files"
  fi

  # Close GITHUB_OUTPUT heredoc
  if [[ -n "$key" ]]; then
    echo "CHANGELOG_EOF" >> "$out"
  fi
}

manual_commit_guard() {
  local branch="${1:?Usage: ci-helpers.sh manual-commit-guard <branch> [base-branch]}"
  local base_branch="${2:-main}"
  local out="${GITHUB_OUTPUT:-/dev/stdout}"
  local should_skip="false"
  local pr_number=""

  # 1. Lookup open PR number for the branch
  pr_number=$(gh pr list --head "$branch" --base "$base_branch" --state open --json number --jq '.[0].number // ""')

  # 2. If PR exists, check for human commits
  if [[ -n "$pr_number" ]]; then
    # Fetch branch and base refs for comparison (silently)
    git fetch --no-tags origin "+refs/heads/${base_branch}:refs/remotes/origin/${base_branch}" "+refs/heads/${branch}:refs/remotes/origin/${branch}" >/dev/null 2>&1 || true

    if git rev-parse --verify "origin/${branch}" >/dev/null 2>&1; then
      # Check author emails: skip if any email doesn't end with a known bot suffix
      local suffixes_re
      suffixes_re=$(printf '%s\n' "${BOT_EMAIL_SUFFIXES[@]}" | sed 's/[.[\*^$()+?{|]/\\&/g' | paste -sd'|' -)
      if git log "origin/${base_branch}..origin/${branch}" --pretty=format:'%ae' | \
           grep -vE "(${suffixes_re})$" | grep -q .; then
        should_skip="true"
      fi
    fi
  fi

  # 3. Emit outputs to GITHUB_OUTPUT
  {
    echo "should_skip=${should_skip}"
    echo "pr_number=${pr_number}"
  } >> "$out"
}

case "$CMD" in
  check-upstream)        check_upstream "$@" ;;
  changelogs)   changelogs "$@" ;;
  manual-commit-guard)   manual_commit_guard "$@" ;;
  *) echo "Usage: ci-helpers.sh <check-upstream|changelogs|manual-commit-guard>" >&2; exit 1 ;;
esac
