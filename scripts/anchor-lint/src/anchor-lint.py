#!/usr/bin/env python3
"""
anchor-lint — Static analysis checks for Anchor/Solana programs.

Usage:
  python3 scripts/anchor-lint/src/anchor-lint.py             # scan from repo root
  python3 scripts/anchor-lint/src/anchor-lint.py --root contracts/
  python3 scripts/anchor-lint/src/anchor-lint.py --only INV-LINT-SERIAL INV-LINT-SIGNER
  python3 scripts/anchor-lint/src/anchor-lint.py --strict    # exit 1 on any finding

Checks
------
INV-LINT-SERIAL  .try_to_vec() usage                                    [error]
INV-LINT-ASSERT  require!() — consider a typed variant                  [info]
INV-LINT-EVENT   emit!/emit_cpi! inconsistency with #[event_cpi]        [warning]
INV-LINT-SIGNER  Signer without address/owner/seeds constraint          [error]
INV-LINT-INIT    init_if_needed without an authorised Signer            [warning]
INV-LINT-INSTR   #[instruction] param order ≠ fn param order            [error]
INV-LINT-PARAMS  #[program] handler args should use single *Params      [warning]
INV-LINT-PDA     PDA struct missing version/bump fields                 [error]
INV-LINT-PDA-SEEDS  PDA variable seeds not present as fields            [error]
INV-LINT-CLONE   Repeated .clone() on same var in hot path              [warning]
INV-LINT-ARITH   Raw +/-/* arithmetic in hot path                       [info]
INV-LINT-HEAP    Heap allocation (Box/Vec/to_vec) in hot path           [warning]
INV-LINT-CAST    Narrowing `as` cast (u64→u8, etc.)                     [warning]
INV-LINT-UNWRAP  try_into()/try_from().unwrap() — fallible conversion   [warning]
INV-LINT-REALLOC realloc without realloc::zero = true                   [info]
INV-LINT-BUMP    bare bump in non-init PDA (use stored bump)            [error]
INV-LINT-ACCTINIT account-absence keyed on AccountNotInitialized       [error]
INV-LINT-ATAINIT non-idempotent init on an associated token account    [error]
INV-LINT-REMACCT remaining_accounts manual deser without key check     [warning]
INV-LINT-CREATEACCT manual create_account without pre-funded handling  [warning]

See contracts/invariants/SOLANA_LINT_INVARIANTS.md for full documentation of each check.
"""

from __future__ import annotations

import argparse
import os
import re
import subprocess
import sys
from collections import defaultdict
from pathlib import Path
from typing import Dict, List, Optional, Set, Tuple

try:
    import tomllib  # Python 3.11+
except ModuleNotFoundError:
    try:
        import tomli as tomllib  # type: ignore[no-redef]
    except ModuleNotFoundError:
        tomllib = None  # type: ignore[assignment]

# ── Terminal colours ──────────────────────────────────────────────────────────

_TTY = sys.stdout.isatty()


def _c(t: str, code: str) -> str:
    return f"\033[{code}m{t}\033[0m" if _TTY else t


def red(t: str) -> str:    return _c(t, "91")
def yellow(t: str) -> str: return _c(t, "93")
def cyan(t: str) -> str:   return _c(t, "96")
def blue(t: str) -> str:   return _c(t, "94")
def green(t: str) -> str:  return _c(t, "92")
def bold(t: str) -> str:   return _c(t, "1")
def dim(t: str) -> str:    return _c(t, "2")


# ── Finding ───────────────────────────────────────────────────────────────────

SEV_COLOR = {"error": red, "warning": yellow, "info": blue}


class Finding:
    __slots__ = ("file", "line", "check_id", "message", "severity")

    def __init__(self, file: str, line: int, check_id: str, message: str, severity: str = "warning"):
        self.file = file
        self.line = line
        self.check_id = check_id
        self.message = message
        self.severity = severity

    def __str__(self) -> str:
        sev_fn = SEV_COLOR.get(self.severity, yellow)
        loc = f"{self.file}:{self.line}"
        return (
            f"  {cyan(loc):<60}  "
            f"[{sev_fn(self.severity):<7}]  "
            f"{bold(self.check_id)}  {self.message}"
        )


# Suppression now lives in the JSON baseline, not in source comments, so the
# checks that once honoured a `// anchor-lint: suppress ...` directive have no
# comment escape any more. `_NEVER` is a regex that cannot match, which keeps each
# rule's structure intact while disabling the comment path.
_NEVER = re.compile(r'(?!)')

_findings: List[Finding] = []
_test_lines_by_file: Dict[str, Set[int]] = {}


# ── Diff scoping ─────────────────────────────────────────────────────────────
# Analysis always covers the whole tree — INV-LINT-EVENT and INV-LINT-INSTR are
# cross-file checks whose answers depend on every file in the program, so
# narrowing the *inputs* would silently change their verdicts. Instead the full
# analysis runs and only the *findings* are filtered down to the changed lines.

_HUNK_HEADER = re.compile(
    r'^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@'
)


# ── Suppression baseline ─────────────────────────────────────────────────────
# Accepted findings live in a JSON baseline rather than in `// anchor-lint:`
# comments, so the source stays free of tool directives. An entry is keyed by
# (file, check) and lists the exact line numbers accepted, so an entry can be
# checked by eye against the source and a *swap* — one finding fixed while another
# appears elsewhere in the same file — is reported instead of cancelling out.
# The cost is that line numbers drift when code above them changes;
# `make anchor-lint-baseline` refreshes them, and a listed line that no longer has
# a finding is reported as stale.

DEFAULT_BASELINE = Path(__file__).resolve().parent.parent / "suppressions.json"


def load_baseline(path: Path) -> Tuple[Dict[Tuple[str, str], Set[int]], Dict[Tuple[str, str], str]]:
    """Read the baseline into {(file, check): {lines}} plus {(file, check): reason}."""
    import json
    try:
        raw = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError:
        return {}, {}
    except (OSError, ValueError) as exc:
        print(f"Error: cannot read baseline {path}: {exc}", file=sys.stderr)
        sys.exit(2)
    lines: Dict[Tuple[str, str], Set[int]] = {}
    reasons: Dict[Tuple[str, str], str] = {}
    entries = raw.get("suppressions", [])
    if not isinstance(entries, list):
        print(f"Error: {path}: \"suppressions\" must be a list", file=sys.stderr)
        sys.exit(2)
    # The baseline is CI- and human-edited, so report a bad entry precisely rather
    # than dying on a KeyError traceback.
    for i, entry in enumerate(entries):
        where = f"{path}: suppressions[{i}]"
        if not isinstance(entry, dict):
            print(f"Error: {where} must be an object", file=sys.stderr)
            sys.exit(2)
        missing = [k for k in ("file", "check") if not entry.get(k)]
        if missing:
            print(f"Error: {where} is missing required key(s): {', '.join(missing)}",
                  file=sys.stderr)
            sys.exit(2)
        raw_lines = entry.get("lines", [])
        if not isinstance(raw_lines, list) or not all(isinstance(n, int) for n in raw_lines):
            print(f"Error: {where} (\"{entry['file']}\", {entry['check']}): "
                  f"\"lines\" must be a list of integers", file=sys.stderr)
            sys.exit(2)
        key = (entry["file"], entry["check"])
        lines[key] = set(raw_lines)
        if entry.get("reason"):
            reasons[key] = entry["reason"]
    return lines, reasons


def write_baseline(path: Path, findings: List[Finding],
                   reasons: Dict[Tuple[str, str], str]) -> int:
    """Write every current finding into the baseline, preserving known reasons."""
    import json
    grouped: Dict[Tuple[str, str], Set[int]] = defaultdict(set)
    for f in findings:
        grouped[(f.file, f.check_id)].add(f.line)
    entries = []
    for (file, check) in sorted(grouped):
        entry = {"file": file, "check": check, "lines": sorted(grouped[(file, check)])}
        reason = reasons.get((file, check))
        if reason:
            entry["reason"] = reason
        entries.append(entry)
    doc = {
        "_comment": (
            "Accepted anchor-lint findings. Generated by `make anchor-lint-baseline`. "
            "Each entry accepts `check` at exactly the listed `lines` of `file`, so a finding "
            "that moves to a different line — or a different finding taking a listed line's "
            "place — is reported rather than silently absorbed. Line numbers drift when code "
            "above them changes; rerun the command to refresh. Delete entries as the debt is "
            "paid down, and add a `reason` to explain why a finding is accepted."
        ),
        "suppressions": entries,
    }
    path.write_text(json.dumps(doc, indent=2) + "\n", encoding="utf-8")
    return len(entries)


def apply_baseline(findings: List[Finding],
                   accepted: Dict[Tuple[str, str], Set[int]]) -> Tuple[List[Finding], List[str]]:
    """Drop findings whose (file, check, line) is listed; return the rest plus stale entries.

    Matching on the exact line means a baseline entry can be checked by eye against
    the source, and a *swap* — one finding fixed while another appears elsewhere in
    the same file — no longer cancels out the way a bare count would.
    """
    seen: Dict[Tuple[str, str], Set[int]] = defaultdict(set)
    kept: List[Finding] = []
    for f in findings:
        key = (f.file, f.check_id)
        if f.line in accepted.get(key, ()):
            seen[key].add(f.line)
            continue
        kept.append(f)
    stale = []
    for key, want in sorted(accepted.items()):
        gone = sorted(want - seen.get(key, set()))
        if gone:
            stale.append(f"{key[0]}  {key[1]}  (no finding at line{'s' if len(gone) > 1 else ''} "
                         f"{', '.join(map(str, gone))})")
    return kept, stale


def _git(args: List[str], cwd: Path) -> Optional[str]:
    try:
        out = subprocess.run(["git"] + args, cwd=str(cwd), capture_output=True, text=True)
    except OSError:
        return None
    return out.stdout if out.returncode == 0 else None


def _command(args: List[str], cwd: Path) -> Optional[str]:
    """Run a command and return stripped stdout on success."""
    try:
        out = subprocess.run(args, cwd=str(cwd), capture_output=True, text=True)
    except OSError:
        return None
    return out.stdout.strip() if out.returncode == 0 else None


def _resolve_branch_ref(branch: str, repo: Path) -> Optional[str]:
    """Resolve a branch name to a commit ref available in the local checkout."""
    candidates = (
        branch,
        f"origin/{branch}",
        f"refs/remotes/origin/{branch}",
    )
    for candidate in candidates:
        if _git(["rev-parse", "--verify", "--quiet", f"{candidate}^{{commit}}"], repo):
            return candidate
    return None


def detect_pr_base(root: Path) -> Optional[str]:
    """Detect the base ref of the current PR in CI or local development.

    GitHub Actions exposes the PR target branch in GITHUB_BASE_REF. Locally,
    Git does not know PR metadata, so use the GitHub CLI to query the PR
    associated with the current branch.
    """
    repo_root = _git(["rev-parse", "--show-toplevel"], root)
    if not repo_root:
        return None
    repo = Path(repo_root.strip())

    github_base = os.environ.get("GITHUB_BASE_REF")
    if github_base:
        return _resolve_branch_ref(github_base, repo)

    base_branch = _command(
        ["gh", "pr", "view", "--json", "baseRefName", "--jq", ".baseRefName"],
        repo,
    )
    if base_branch:
        return _resolve_branch_ref(base_branch, repo)

    return None


def changed_lines(base: str, root: Path) -> Optional[Tuple[Dict[str, Set[int]], Set[str]]]:
    """Return changed new-side lines and files containing pure deletions.

    Diffs from the merge-base of `base` and HEAD. Added/modified hunks are
    scoped to their exact new-side lines. Pure deletions have no new-side line
    to attach a finding to, and can create structural findings elsewhere in the
    same file, so files containing pure deletions are tracked separately and all
    remaining findings in those files are considered part of the PR diff.

    Returns None when the diff can't be computed (not a repo, unknown ref, no
    history) so the caller fails rather than silently passing a lint that never ran.
    """
    repo_root = _git(["rev-parse", "--show-toplevel"], root)
    if not repo_root:
        return None
    repo = Path(repo_root.strip())

    merge_base = _git(["merge-base", base, "HEAD"], repo)
    if not merge_base:
        return None

    diff = _git(["diff", "--unified=0", "--no-color", "--no-ext-diff",
                 merge_base.strip(), "HEAD", "--", str(root)], repo)
    if diff is None:
        return None

    changed: Dict[str, Set[int]] = defaultdict(set)
    deletion_files: Set[str] = set()
    current: Optional[str] = None

    for line in diff.splitlines():
        if line.startswith("+++ "):
            target = line[4:].strip()
            if target == "/dev/null":
                current = None
                continue
            # Strip git's `b/` prefix, then re-root the path onto `root`.
            path = target[2:] if target.startswith(("a/", "b/")) else target
            try:
                current = str((repo / path).resolve().relative_to(root.resolve()))
            except ValueError:
                current = None
            continue

        if current is None:
            continue

        m = _HUNK_HEADER.match(line)
        if not m:
            continue

        old_count = int(m.group(2)) if m.group(2) is not None else 1
        new_start = int(m.group(3))
        new_count = int(m.group(4)) if m.group(4) is not None else 1

        if new_count == 0 and old_count > 0:
            # A pure deletion has no new-side line to blame. Removing a field,
            # constraint, seed, etc. can create a structural finding reported
            # elsewhere in the same file, so scope the whole file into the PR.
            deletion_files.add(current)
            continue

        for n in range(new_start, new_start + new_count):
            changed[current].add(n)

    return changed, deletion_files


def report(file: str, line: int, check_id: str, message: str, severity: str = "warning") -> None:
    # Skip findings in test code (#[cfg(test)] blocks)
    if (line - 1) in _test_lines_by_file.get(file, set()):
        return
    _findings.append(Finding(file, line, check_id, message, severity))


# ── File helpers ──────────────────────────────────────────────────────────────

def rs_files(root: Path) -> List[Path]:
    return sorted(p for p in root.rglob("*.rs") if "target" not in p.parts)


def lines_of(path: Path) -> List[str]:
    try:
        return path.read_text(encoding="utf-8", errors="replace").splitlines()
    except Exception:
        return []


def rel(path: Path, root: Path) -> str:
    try:
        return str(path.relative_to(root))
    except ValueError:
        return str(path)


def program_key(path: Path) -> Optional[str]:
    """Return the top-level program directory (e.g. 'programs/ccip-router')."""
    parts = path.parts
    for i, part in enumerate(parts):
        if part in ("programs", "crates") and i + 1 < len(parts):
            return f"{part}/{parts[i + 1]}"
    return None


def is_comment(line: str) -> bool:
    return line.lstrip().startswith("//")


def strip_inline_comment(line: str) -> str:
    """Remove trailing // comments from a line (ignoring // inside strings)."""
    in_str = None
    i = 0
    while i < len(line):
        ch = line[i]
        if ch == '\\' and in_str:
            i += 2
            continue
        if ch in ('"', "'") and not in_str:
            in_str = ch
        elif ch == in_str:
            in_str = None
        elif ch == '/' and i + 1 < len(line) and line[i + 1] == '/' and not in_str:
            return line[:i]
        i += 1
    return line


def is_test_code(lines: List[str], line_idx: int) -> bool:
    """Return True if line_idx (0-based) is inside a #[cfg(test)] block."""
    return line_idx in _test_line_set(lines)


def _test_line_set(lines: List[str]) -> Set[int]:
    """Return 0-based line indices that fall inside #[cfg(test)] blocks."""
    result: Set[int] = set()
    i = 0
    while i < len(lines):
        if lines[i].strip().startswith('#[cfg(test)]'):
            result.add(i)
            # Find the opening '{' of the mod
            j = i + 1
            while j < len(lines) and '{' not in lines[j]:
                result.add(j)
                j += 1
            if j >= len(lines):
                break
            # Track brace depth from the opening '{'
            depth = 0
            while j < len(lines):
                depth += lines[j].count('{') - lines[j].count('}')
                result.add(j)
                j += 1
                if depth <= 0:
                    break
            i = j
        else:
            i += 1
    return result


# ── INV-LINT-SERIAL – .try_to_vec() ──────────────────────────────────────────

_TRY_TO_VEC = re.compile(r'\.try_to_vec\s*\(')


def inv_lint_serial(path: Path, lines: List[str], root: Path) -> None:
    for i, ln in enumerate(lines, 1):
        if not is_comment(ln) and _TRY_TO_VEC.search(ln):
            report(rel(path, root), i, "INV-LINT-SERIAL",
                   "do not use .try_to_vec() — use borsh::to_vec() or write directly to a buffer",
                   "error")


# ── INV-LINT-ASSERT – require! → consider typed variant ──────────────────────

_REQUIRE_CALL = re.compile(r'\brequire!\s*\(')
# Typed Anchor assertion macros for the suggestion hint
_ASSERT_HINT = (
    "consider a typed variant: "
    "require_eq!/require_neq! (needs Display), "
    "require_keys_eq!/require_keys_neq! (Pubkeys), "
    "require_gt!/require_gte! (PartialOrd+Display)"
)
_SUPPRESS_LINT_ASSERT = _NEVER
# Comparisons a typed macro can express. `<`/`<=` are absent on purpose: Anchor
# ships no require_lt!/require_lte!, so those must be rewritten backwards by hand.
_TYPED_COMPARISON = re.compile(r'(==|!=|>=|(?<![-=<>])>(?![=>]))')
# `&&` / `||` / a leading `!` make the predicate compound rather than one comparison.
_BOOL_COMBINATOR = re.compile(r'(\|\||&&|(?<![\w!=<>])!(?![=]))')


def _strip_nested_groups(text: str) -> str:
    """Drop everything inside nested (), [] or closures so only top-level operators remain."""
    out = []
    depth = 0
    for ch in text:
        if ch in '([':
            depth += 1
        elif ch in ')]':
            depth = max(0, depth - 1)
        elif depth == 0:
            out.append(ch)
    return "".join(out)


def _require_predicate(lines: List[str], idx: int) -> str:
    """Return the predicate text of the `require!(` starting at `lines[idx]`.

    Reads forward until the macro's parens balance, then drops the trailing error-code
    argument so a comparison inside it can't be mistaken for the condition.
    """
    text = ""
    depth = 0
    started = False
    for ln in lines[idx:idx + 8]:
        code = strip_inline_comment(ln)
        for ch in code:
            if ch == '(':
                depth += 1
                started = True
                if depth == 1:
                    continue
            elif ch == ')':
                depth -= 1
                if depth == 0:
                    return text.split(',')[0]
            if started and depth >= 1:
                text += ch
        text += " "
    return text.split(',')[0]


def inv_lint_assert(path: Path, lines: List[str], root: Path) -> None:
    """Flag require!() calls whose predicate a typed variant could actually replace.

    Only a bare comparison (`a == b`, `a != b`, `a > b`, `a >= b`) has a typed
    equivalent. Firing on *every* require! — including arbitrary boolean predicates
    like `!enabled || list.contains(x)`, which no typed macro expresses — made the
    check unactionable: the only available response was a suppression comment, and
    95 of the repo's 127 hits were of that kind.

    Accepted occurrences live in the JSON baseline, not in source comments.
    """
    for i, ln in enumerate(lines, 1):
        if is_comment(ln) or not _REQUIRE_CALL.search(ln):
            continue
        nearby = "\n".join(lines[max(0, i - 3):i])
        if _SUPPRESS_LINT_ASSERT.search(nearby):
            continue
        # The predicate is everything up to the error-code argument, and may wrap
        # across lines; join enough of them to see the whole condition.
        pred = _require_predicate(lines, i - 1)
        # Only a comparison at the top level is the predicate itself. One nested
        # inside a call or closure — `account_id.iter().any(|b| *b != 0)` — belongs
        # to that expression, and no typed macro replaces the outer call.
        if not _TYPED_COMPARISON.search(_strip_nested_groups(pred)):
            continue
        # A predicate that also contains boolean operators is a compound condition,
        # not a single comparison, so no typed macro replaces it wholesale.
        if _BOOL_COMBINATOR.search(pred):
            continue
        report(rel(path, root), i, "INV-LINT-ASSERT", _ASSERT_HINT, "info")


# ── INV-LINT-EVENT – emit!/emit_cpi! consistency ────────────────────────────
# Rule A (per-file): kept as a no-op — detection moved to cross-file Rule C.
# Rule B (per-program): emit_cpi! must not appear in a program that has no
#   #[event_cpi] contexts anywhere — those calls would reference non-existent
#   event-authority accounts, causing runtime failures.
# Rule C (per-program + deps): if a program has #[event_cpi] in ANY file, ALL
#   emit!() calls across the program must use emit_cpi!().  Additionally, if
#   the program depends on workspace crates (via path deps in Cargo.toml),
#   emit!() in those crates is also flagged.

_HAS_EVENT_CPI = re.compile(r'#\s*\[\s*event_cpi\s*\]')
_PLAIN_EMIT = re.compile(r'\bemit!\s*\(')
_CPI_EMIT = re.compile(r'\bemit_cpi!\s*\(')
_SUPPRESS_LINT_EVENT = _NEVER


def _workspace_deps(program_dir: Path) -> Set[str]:
    """Parse Cargo.toml and return the set of workspace-local dependency crate directories.

    E.g. for a dep like `base-token-pool = { path = "../base-token-pool" }`, returns
    {"base-token-pool"}.  Only path-based dependencies are considered.
    """
    cargo = program_dir / "Cargo.toml"
    if not cargo.is_file():
        return set()
    if tomllib is None:
        # Fallback: simple regex extraction when tomllib is unavailable.
        try:
            text = cargo.read_text(encoding="utf-8", errors="replace")
        except Exception:
            return set()
        return set(re.findall(r'path\s*=\s*"\.\./(\w[\w-]*)"', text))
    try:
        with open(cargo, "rb") as f:
            data = tomllib.load(f)
    except Exception:
        return set()
    result: Set[str] = set()
    for section in ("dependencies", "dev-dependencies", "build-dependencies"):
        deps = data.get(section, {})
        for _name, spec in deps.items():
            if isinstance(spec, dict):
                p = spec.get("path", "")
                if p.startswith("../"):
                    result.add(p.lstrip("../").split("/")[0])
    return result


def inv_lint_event(path: Path, lines: List[str], root: Path) -> None:
    """Rule A: no-op — detection moved to cross-file Rule C."""
    pass


def inv_lint_event_b(all_rs: List[Path], root: Path) -> None:
    """Rule B: emit_cpi! in a program that has no #[event_cpi] context anywhere."""
    prog_files: Dict[str, List[Path]] = defaultdict(list)
    for p in all_rs:
        prog_files[program_key(p) or "unknown"].append(p)

    for prog_key, files in prog_files.items():
        prog_has_event_cpi = any(
            _HAS_EVENT_CPI.search(ln)
            for p in files
            for ln in lines_of(p)
        )
        if prog_has_event_cpi:
            continue  # program correctly uses #[event_cpi] — handled by Rule C

        for p in files:
            lns = lines_of(p)
            for i, ln in enumerate(lns, 1):
                if not is_comment(ln) and _CPI_EMIT.search(ln):
                    report(rel(p, root), i, "INV-LINT-EVENT",
                           "emit_cpi!() used but this program has no #[event_cpi] context — "
                           "the event-authority accounts won't be present; add #[event_cpi] to the context",
                           "warning")


def inv_lint_event_c(all_rs: List[Path], root: Path) -> None:
    """Rule C: program has #[event_cpi] — flag ALL emit!() across the program and its deps."""
    prog_files: Dict[str, List[Path]] = defaultdict(list)
    for p in all_rs:
        prog_files[program_key(p) or "unknown"].append(p)

    # Determine which programs have #[event_cpi]
    programs_with_event_cpi: Set[str] = set()
    for prog_key, files in prog_files.items():
        if any(
            _HAS_EVENT_CPI.search(ln)
            for p in files
            for ln in lines_of(p)
        ):
            programs_with_event_cpi.add(prog_key)

    if not programs_with_event_cpi:
        return

    # Find dependency crates of programs that have #[event_cpi].
    # For each such program, parse its Cargo.toml to find workspace path deps,
    # then map those crate names to program_key strings.
    dep_crate_keys: Set[str] = set()
    for prog_key in programs_with_event_cpi:
        # prog_key looks like "programs/burnmint-token-pool"
        prog_dir = root / prog_key
        for dep_name in _workspace_deps(prog_dir):
            # dep_name like "base-token-pool" → key like "programs/base-token-pool"
            dep_key = f"programs/{dep_name}"
            if dep_key not in programs_with_event_cpi:
                dep_crate_keys.add(dep_key)

    # Flag emit!() in all programs with #[event_cpi] AND their dependency crates
    keys_to_check = programs_with_event_cpi | dep_crate_keys
    for prog_key in keys_to_check:
        files = prog_files.get(prog_key, [])
        is_dep = prog_key in dep_crate_keys
        for p in files:
            lns = lines_of(p)
            for i, ln in enumerate(lns, 1):
                if is_comment(ln) or not _PLAIN_EMIT.search(ln) or _CPI_EMIT.search(ln):
                    continue
                # Check for suppress comment in ±2 lines
                ctx = "\n".join(lns[max(0, i - 3):i + 2])
                if _SUPPRESS_LINT_EVENT.search(ctx):
                    continue
                if is_dep:
                    msg = ("this crate is a dependency of a program with #[event_cpi] — "
                           "use emit_cpi!() for consistency; "
                           "accept via `make anchor-lint-baseline`")
                else:
                    msg = ("this program has #[event_cpi] context(s) — use emit_cpi!() "
                           "for consistency; "
                           "accept via `make anchor-lint-baseline`")
                report(rel(p, root), i, "INV-LINT-EVENT", msg, "warning")


# ── INV-LINT-SIGNER – Signer<'info> without address/owner constraint ─────────

_DERIVE_ACCOUNTS = re.compile(r'#\s*\[\s*derive\s*\([^)]*\bAccounts\b[^)]*\)\s*\]')
_SIGNER_FIELD = re.compile(r'pub\s+(\w+)\s*:\s*Signer\s*<\s*\'info\s*>')
_ACCOUNT_ATTR = re.compile(r'#\s*\[\s*account\s*\(([^)]*(?:\([^)]*\)[^)]*)*)\)\s*\]', re.DOTALL)
_ADDR_CONSTRAINT = re.compile(r'\b(address|owner)\s*=')
_PERMISSIONLESS_COMMENT = re.compile(r'(permissionless|PERMISSIONLESS|anyone|any.*caller|payer.*init)', re.IGNORECASE)
_PDA_SIGNER_CONSTRAINT = re.compile(r'\bseeds\s*=')


def _signer_is_authorized(field_name: str, lines: List[str], signer_idx: int,
                          struct_block: str) -> bool:
    """
    Return True if the Signer field is authorized by any of:
      1. A direct `address =` or `owner =` constraint on its own #[account] attribute.
      2. A `seeds =` constraint (PDA signer — only the owning program can sign).
      3. An indirect constraint elsewhere in the struct referencing `<field>.key()`.
    """
    # Use the LAST (nearest) #[account(...)] in the slice before this Signer line
    slice_back = "\n".join(lines[max(0, signer_idx - 12):signer_idx + 1])
    all_matches = _ACCOUNT_ATTR.findall(slice_back)
    nearest_attr = all_matches[-1] if all_matches else ""

    if _ADDR_CONSTRAINT.search(nearest_attr):
        return True  # direct address/owner constraint

    if _PDA_SIGNER_CONSTRAINT.search(nearest_attr):
        return True  # PDA-derived signer — inherently restricted

    # Direct constraint= on the signer that references its own field name (e.g. authority.key)
    constraint_self = re.compile(r'\bconstraint\s*=.*\b' + re.escape(field_name) + r'\b')
    if constraint_self.search(nearest_attr):
        return True

    # Indirect: <field_name>.key() or &<field_name> referenced in any constraint in the same struct
    key_re = re.compile(r'\b' + re.escape(field_name) + r'\s*\.\s*key\s*\(\s*\)')
    if key_re.search(struct_block):
        return True

    # Indirect: &<field_name> passed as argument in a constraint (e.g. constraint = check_fn(&authority, ...))
    ref_re = re.compile(r'constraint\s*=.*&' + re.escape(field_name) + r'\b')
    if ref_re.search(struct_block):
        return True

    return False


def inv_lint_signer(path: Path, lines: List[str], root: Path) -> None:
    """Signer fields with no address= / owner= constraint need a permissionless justification."""
    i = 0
    while i < len(lines):
        if not _DERIVE_ACCOUNTS.search(lines[i]):
            i += 1
            continue

        # Collect the full struct block for indirect-auth checking
        struct_start = i + 1
        depth = 0
        j = struct_start
        while j < len(lines):
            depth += lines[j].count('{') - lines[j].count('}')
            j += 1
            if depth <= 0 and j > struct_start + 1:
                break
        struct_block = "\n".join(lines[struct_start:j])

        # Scan inside the pre-computed struct block for Signer fields
        for si in range(struct_start, j):
            m = _SIGNER_FIELD.search(lines[si])
            if m:
                field_name = m.group(1)
                if not _signer_is_authorized(field_name, lines, si, struct_block):
                    # Check for permissionless comment in ±5 lines
                    ctx_text = "\n".join(lines[max(0, si - 5):si + 4])
                    if not _PERMISSIONLESS_COMMENT.search(ctx_text):
                        report(rel(path, root), si + 1, "INV-LINT-SIGNER",
                               f"Signer `{field_name}` has no authorization constraint "
                               "(address=, owner=, seeds=, or indirect key() check) — "
                               "add a constraint or a // PERMISSIONLESS: <reason> comment",
                               "error")
        i = j


# ── INV-LINT-INIT – init_if_needed without authorised Signer ─────────────────

_INIT_IF_NEEDED = re.compile(r'\binit_if_needed\b')


def inv_lint_init(path: Path, lines: List[str], root: Path) -> None:
    """init_if_needed in a context without any address-constrained Signer is a reinitialisation risk."""
    i = 0
    while i < len(lines):
        if not _DERIVE_ACCOUNTS.search(lines[i]):
            i += 1
            continue

        # Collect the full struct block
        struct_start = i
        depth = 0
        block: List[str] = []
        init_lines: List[int] = []
        i += 1
        while i < len(lines):
            ln = lines[i]
            depth += ln.count('{') - ln.count('}')
            block.append(ln)
            if _INIT_IF_NEEDED.search(ln) and not is_comment(ln):
                init_lines.append(i + 1)  # 1-indexed
            if depth <= 0 and len(block) > 1:
                break
            i += 1

        if not init_lines:
            i += 1
            continue

        # Check if any Signer in this block has an authorization constraint
        # or a PERMISSIONLESS comment (same logic as inv_lint_signer)
        block_text = "\n".join(block)
        has_auth_signer = False
        for j, bl in enumerate(block):
            field_m = _SIGNER_FIELD.search(bl)
            if field_m:
                fname = field_m.group(1)
                # Reuse _signer_is_authorized which handles address=, owner=, seeds=, and indirect key()
                abs_idx = struct_start + 1 + j  # block[0] is the line after #[derive]
                if _signer_is_authorized(fname, block, j, block_text):
                    has_auth_signer = True
                    break
                # Check for permissionless comment in ±5 lines around the Signer
                ctx_text = "\n".join(block[max(0, j - 5):j + 4])
                if _PERMISSIONLESS_COMMENT.search(ctx_text):
                    has_auth_signer = True
                    break

        if not has_auth_signer:
            for ln_no in init_lines:
                report(rel(path, root), ln_no, "INV-LINT-INIT",
                       "init_if_needed without an address-constrained Signer — "
                       "a malicious actor could front-run account creation",
                       "warning")
        i += 1


# ── INV-LINT-INSTR – #[instruction] param order mismatch ─────────────────────

_INSTRUCTION_ATTR = re.compile(r'#\s*\[\s*instruction\s*\(([^)]+)\)\s*\]')
_PARAM_NAME = re.compile(r'(\w+)\s*:')
_STRUCT_NAME = re.compile(r'pub\s+struct\s+(\w+)')
_SUPPRESS_LINT_INSTR = _NEVER


def _fn_params_for_ctx(struct_name: str, program_rs: List[Path]) -> Optional[List[str]]:
    """
    Search within program_rs for a pub fn that takes Context<struct_name> and return
    its parameter names after the ctx argument.  Scoping to a single program avoids
    false positives when two programs share a context struct name (e.g. GetFee).
    """
    pattern = re.compile(
        r'(?:pub\s+fn|fn)\s+\w+\s*(?:<[^>]*>)?\s*\(\s*(?:&\s*self\s*,\s*)?(?:mut\s+)?(?:\w+\s*:\s*)?Context\s*<\s*(?:\'info\s*,\s*)?'
        + re.escape(struct_name)
        + r'(?:\s*<[^>]*>)?\s*>([^{]*?)\)',
        re.DOTALL,
    )
    candidates: List[List[str]] = []
    for p in program_rs:
        content = "\n".join(lines_of(p))
        for m in pattern.finditer(content):
            rest = m.group(1).lstrip(" ,\n")
            params = _PARAM_NAME.findall(rest)
            if params:
                candidates.append(params)
    if not candidates:
        return None
    # Prefer the candidate with the most parameters (the concrete fn over trait default)
    return max(candidates, key=len)


def inv_lint_instr(all_rs: List[Path], root: Path) -> None:
    # Group files by program so we don't match across programs with shared struct names
    prog_files: Dict[str, List[Path]] = defaultdict(list)
    for p in all_rs:
        key = program_key(p) or "unknown"
        prog_files[key].append(p)

    for prog_key, prog_rs in prog_files.items():
        for path in prog_rs:
            lines = lines_of(path)
            content = "\n".join(lines)
            for m in _INSTRUCTION_ATTR.finditer(content):
                instr_params = _PARAM_NAME.findall(m.group(1))
                if not instr_params:
                    continue
                line_no = content[: m.start()].count("\n") + 1

                # Check for suppress comment on the lines around the #[instruction] attribute
                nearby = "\n".join(lines[max(0, line_no - 3):line_no + 1])
                if _SUPPRESS_LINT_INSTR.search(nearby):
                    continue

                # Find the struct name immediately after this attribute
                after = content[m.end():]
                sn_m = _STRUCT_NAME.search(after[:300])
                if not sn_m:
                    continue
                struct_name = sn_m.group(1)

                fn_params = _fn_params_for_ctx(struct_name, prog_rs)
                if fn_params is None:
                    continue  # can't find the function in this program — skip

                fn_names_stripped = [p.lstrip('_') for p in fn_params]
                for idx, ip in enumerate(instr_params):
                    if idx >= len(fn_params):
                        report(rel(path, root), line_no, "INV-LINT-INSTR",
                               f"#[instruction] for `{struct_name}` lists `{ip}` at position {idx + 1} "
                               "but the function has no such parameter — causes incorrect deserialization",
                               "error")
                        break
                    # Strip leading underscores: Rust uses _param to suppress "unused variable"
                    # warnings, but #[instruction] always uses the canonical (no-underscore) name.
                    if ip.lstrip('_') != fn_names_stripped[idx]:
                        # Determine severity:
                        # - If the name APPEARS elsewhere in fn_params, names are swapped → error
                        # - If the name doesn't appear at all, it's likely a cosmetic rename → warning
                        if ip.lstrip('_') in fn_names_stripped:
                            sev = "error"
                            detail = "names appear swapped — Anchor deserialises by position"
                        else:
                            sev = "error"
                            detail = "param names differ between #[instruction] and fn signature — could be a silent positional mismatch"
                        report(rel(path, root), line_no, "INV-LINT-INSTR",
                               f"#[instruction] for `{struct_name}` position {idx + 1}: "
                               f"`{ip}` vs fn `{fn_params[idx]}` — {detail}",
                               sev)
                        break


# ── INV-LINT-PARAMS – #[program] handler should use single params arg ─────────

_PROGRAM_ATTR = re.compile(r'^\s*#\s*\[\s*program\s*\]\s*$')
_MOD_DECL = re.compile(r'^\s*(?:pub\s+)?mod\s+\w+')
_PUB_FN_START = re.compile(r'^\s*pub\s+fn\s+(\w+)')
_SUPPRESS_LINT_PARAMS = _NEVER


def _split_params_top_level(params_text: str) -> List[str]:
    parts: List[str] = []
    cur: List[str] = []
    depth_angle = depth_paren = depth_brack = depth_brace = 0
    for ch in params_text:
        if ch == '<':
            depth_angle += 1
        elif ch == '>' and depth_angle > 0:
            depth_angle -= 1
        elif ch == '(':
            depth_paren += 1
        elif ch == ')' and depth_paren > 0:
            depth_paren -= 1
        elif ch == '[':
            depth_brack += 1
        elif ch == ']' and depth_brack > 0:
            depth_brack -= 1
        elif ch == '{':
            depth_brace += 1
        elif ch == '}' and depth_brace > 0:
            depth_brace -= 1

        if ch == ',' and depth_angle == depth_paren == depth_brack == depth_brace == 0:
            token = ''.join(cur).strip()
            if token:
                parts.append(token)
            cur = []
        else:
            cur.append(ch)

    token = ''.join(cur).strip()
    if token:
        parts.append(token)
    return parts


def inv_lint_params(path: Path, lines: List[str], root: Path) -> None:
    """Warn when #[program] handlers use multiple args after Context instead of a single *Params struct."""
    i = 0
    while i < len(lines):
        if not _PROGRAM_ATTR.match(lines[i]):
            i += 1
            continue

        # Find the `mod ... {` that follows #[program]
        j = i + 1
        while j < len(lines) and not _MOD_DECL.search(lines[j]):
            j += 1
        if j >= len(lines):
            i += 1
            continue

        # Enter module block and scan pub fn signatures
        depth = 0
        k = j
        while k < len(lines):
            ln = lines[k]
            depth += ln.count('{') - ln.count('}')

            m_fn = _PUB_FN_START.match(ln)
            if m_fn and depth >= 1:
                fn_name = m_fn.group(1)
                start_line = k + 1

                # Collect signature until matching ')'
                sig = ln
                paren_depth = ln.count('(') - ln.count(')')
                t = k + 1
                while t < len(lines) and paren_depth > 0:
                    sig += "\n" + lines[t]
                    paren_depth += lines[t].count('(') - lines[t].count(')')
                    t += 1

                # Optional suppression in nearby lines
                nearby = "\n".join(lines[max(0, k - 2):min(len(lines), t + 1)])
                if _SUPPRESS_LINT_PARAMS.search(nearby):
                    k = t
                    continue

                open_idx = sig.find('(')
                # Find the ')' that matches `open_idx` (the end of the parameter list), not
                # `rfind(')')` — a return type such as `Result<()>` contains its own ')', and on a
                # multi-line signature rfind would match that, pulling return-type text into the
                # parameter list and miscounting args.
                close_idx = -1
                if open_idx != -1:
                    paren = 0
                    for idx in range(open_idx, len(sig)):
                        if sig[idx] == '(':
                            paren += 1
                        elif sig[idx] == ')':
                            paren -= 1
                            if paren == 0:
                                close_idx = idx
                                break
                if open_idx == -1 or close_idx == -1 or close_idx <= open_idx:
                    k = t
                    continue

                params_text = sig[open_idx + 1:close_idx].strip()
                params = _split_params_top_level(params_text)
                if not params:
                    k = t
                    continue

                # Remove self receivers (for completeness)
                params = [p for p in params if p.strip() not in ("&self", "&mut self", "self")]
                if not params:
                    k = t
                    continue

                # Require first param to be Context<...>
                first = params[0]
                if 'Context<' not in first:
                    k = t
                    continue

                trailing = params[1:]
                if len(trailing) <= 1:
                    k = t
                    continue

                report(rel(path, root), start_line, "INV-LINT-PARAMS",
                       f"`{fn_name}` has {len(trailing)} args after Context — "
                       "prefer a single `*Params` struct argument; "
                       "accept via `make anchor-lint-baseline`",
                       "warning")
                k = t
                continue

            if depth <= 0 and k > j:
                break
            k += 1

        i = k + 1


# ── INV-LINT-PDA – PDA #[account] struct missing version/bump ─────────────────
# Only checks structs that are decorated with #[account] (the Anchor account macro,
# not the field constraint).  Structs named *Params, *Event, *Config (simple value
# types) are excluded by the user via a suppress comment.

_ACCOUNT_MACRO = re.compile(r'^\s*#\s*\[\s*account\b')   # struct-level #[account] (allows indentation)
_ACCOUNT_FIELD_HINT = re.compile(r'seeds|init|mut|close|realloc|space|payer|constraint|address|owner|has_one')  # field constraint clues
_SUPPRESS_LINT_PDA = _NEVER
_HAS_VERSION = re.compile(r'\bpub\s+version\s*:')
_HAS_BUMP = re.compile(r'\bpub\s+bump\s*:')

# Structs that intentionally don't need version/bump
_SKIP_SUFFIXES = ("Params", "Event", "Response", "Output", "Input")


def inv_lint_pda(path: Path, lines: List[str], root: Path) -> None:
    i = 0
    while i < len(lines):
        ln = lines[i].rstrip()
        # Detect struct-level #[account] — must NOT contain field-constraint keywords
        if _ACCOUNT_MACRO.match(ln):
            # If the attribute spans multiple lines (has '(' but no ']' yet),
            # collect the full attribute text before checking for field hints.
            attr_text = ln
            if '(' in ln and ']' not in ln:
                for lookahead in range(1, 8):
                    if i + lookahead < len(lines):
                        attr_text += " " + lines[i + lookahead].strip()
                        if ']' in lines[i + lookahead]:
                            break
            if _ACCOUNT_FIELD_HINT.search(attr_text):
                i += 1
                continue
            # Find the struct definition that follows (skip other attributes)
            j = i + 1
            while j < len(lines) and not _STRUCT_NAME.search(lines[j]):
                if j - i > 6:
                    break
                j += 1
            sn_m = _STRUCT_NAME.search(lines[j]) if j < len(lines) else None
            if not sn_m:
                i += 1
                continue
            struct_name = sn_m.group(1)
            if struct_name.endswith(_SKIP_SUFFIXES):
                i = j + 1
                continue

            # Collect struct body
            k = j
            depth = 0
            while k < len(lines):
                depth += lines[k].count('{') - lines[k].count('}')
                k += 1
                if depth <= 0 and k > j + 1:
                    break
            body_lines = lines[j:k]
            body = "\n".join(body_lines)

            # Check for suppress comment
            if _SUPPRESS_LINT_PDA.search(body) or _SUPPRESS_LINT_PDA.search("\n".join(lines[max(0, i - 2):i + 1])):
                i = k
                continue

            missing = []
            if not _HAS_VERSION.search(body):
                missing.append("pub version: u8")
            if not _HAS_BUMP.search(body):
                missing.append("pub bump: u8")

            if missing:
                report(rel(path, root), j + 1, "INV-LINT-PDA",
                       f"PDA `{struct_name}` missing: {', '.join(missing)} — "
                       "mandatory for all new PDAs; suppress existing ones with "
                       "accept via `make anchor-lint-baseline`",
                       "error")
            i = k
        else:
            i += 1


# ── INV-LINT-PDA-SEEDS – PDA variable seeds must be account fields ────────────
# Every variable seed used to derive a PDA must be stored as a field in the account
# (directly or via a nested struct). This ensures the account is self-describing.

_SEEDS_COMMENT = re.compile(r'//.*(?:seeds\s*[=:]|PDA\s+with\s+seeds)', re.IGNORECASE)
_SEEDS_ARRAY = re.compile(r'\[\s*([^\]]+)\s*\]')
_SUPPRESS_LINT_PDA_SEEDS = _NEVER
_CONSTANT_SEED = re.compile(r'^[A-Z][A-Z0-9_]*$')  # ALL_CAPS constants
_FIELD_DECL = re.compile(r'pub\s+(\w+)\s*:')

# Cache for struct field resolution
_STRUCT_FIELDS_CACHE: Optional[Dict[str, Set[str]]] = None


def _build_struct_fields_cache(root: Path) -> Dict[str, Set[str]]:
    """Build a cache mapping struct names to their field names (recursive)."""
    global _STRUCT_FIELDS_CACHE
    if _STRUCT_FIELDS_CACHE is not None:
        return _STRUCT_FIELDS_CACHE

    structs: Dict[str, Set[str]] = {}
    scan_dirs = [root / "programs"]
    if (root / "crates").is_dir():
        scan_dirs.append(root / "crates")

    for d in scan_dirs:
        for path in rs_files(d):
            lns = lines_of(path)
            i = 0
            while i < len(lns):
                m = re.match(r'\s*pub\s+struct\s+(\w+)', lns[i])
                if m:
                    struct_name = m.group(1)
                    # Collect struct body
                    k = i
                    depth = 0
                    while k < len(lns):
                        depth += lns[k].count('{') - lns[k].count('}')
                        k += 1
                        if depth <= 0 and k > i + 1:
                            break
                    body = "\n".join(lns[i:k])
                    fields = set(_FIELD_DECL.findall(body))
                    structs[struct_name] = fields
                    i = k
                else:
                    i += 1

    _STRUCT_FIELDS_CACHE = structs
    return structs


def _extract_variable_seeds(seed_text: str) -> List[str]:
    """Extract variable seed names from a seed array text like '[SEED, chain_selector, mint]'."""
    match = _SEEDS_ARRAY.search(seed_text)
    if not match:
        return []

    seeds_str = match.group(1)
    variables = []

    for seed in seeds_str.split(','):
        seed = seed.strip()
        if not seed:
            continue
        # Extract the base variable name (before any method calls or conversions)
        # e.g. "chain_selector.to_le_bytes()" -> "chain_selector"
        # e.g. "mint.key().as_ref()" -> "mint"
        # e.g. "override_type.as_ref()" -> "override_type"
        base_name = seed.split('.')[0].strip()
        base_name = base_name.split('(')[0].strip()  # handle any function calls
        base_name = base_name.lstrip('&')  # remove reference prefix

        # Skip constants (ALL_CAPS) and byte literals
        if _CONSTANT_SEED.match(base_name):
            continue
        if base_name.startswith('b"') or base_name.startswith("b'"):
            continue

        variables.append(base_name)

    return variables


def _get_struct_fields_recursive(struct_name: str, structs_cache: Dict[str, Set[str]], visited: Optional[Set[str]] = None) -> Set[str]:
    """Get all fields of a struct, including fields from nested structs."""
    if visited is None:
        visited = set()
    if struct_name in visited:
        return set()
    visited.add(struct_name)

    direct_fields = structs_cache.get(struct_name, set())
    all_fields = set(direct_fields)

    # For each field, if there's a struct with that type name, add its fields too
    for field_name in direct_fields:
        # Try to find the field's type by looking for "pub field_name: TypeName"
        # This is approximate - we just check if there's a struct with a similar name
        for other_struct in structs_cache:
            if other_struct.lower() == field_name.lower() or other_struct.endswith(f'Config') or other_struct.startswith('Base'):
                nested = _get_struct_fields_recursive(other_struct, structs_cache, visited)
                all_fields.update(nested)

    return all_fields


def inv_lint_pda_seeds(path: Path, lines: List[str], root: Path) -> None:
    """Check that PDA structs have all variable seeds as fields."""
    structs_cache = _build_struct_fields_cache(root)

    i = 0
    while i < len(lines):
        ln = lines[i].rstrip()

        # Look for struct-level #[account] attribute
        if _ACCOUNT_MACRO.match(ln):
            # Check for field-constraint hints (these are field-level, not struct-level)
            attr_text = ln
            if '(' in ln and ']' not in ln:
                for lookahead in range(1, 8):
                    if i + lookahead < len(lines):
                        attr_text += " " + lines[i + lookahead].strip()
                        if ']' in lines[i + lookahead]:
                            break
            if _ACCOUNT_FIELD_HINT.search(attr_text):
                i += 1
                continue

            # Find the struct definition
            j = i + 1
            while j < len(lines) and not _STRUCT_NAME.search(lines[j]):
                if j - i > 6:
                    break
                j += 1
            sn_m = _STRUCT_NAME.search(lines[j]) if j < len(lines) else None
            if not sn_m:
                i += 1
                continue
            struct_name = sn_m.group(1)
            if struct_name.endswith(_SKIP_SUFFIXES):
                i = j + 1
                continue

            # Collect struct body
            k = j
            depth = 0
            while k < len(lines):
                depth += lines[k].count('{') - lines[k].count('}')
                k += 1
                if depth <= 0 and k > j + 1:
                    break
            body_lines = lines[j:k]
            body = "\n".join(body_lines)

            # Check for suppress comment
            context_before = "\n".join(lines[max(0, i - 10):i + 1])
            if _SUPPRESS_LINT_PDA_SEEDS.search(body) or _SUPPRESS_LINT_PDA_SEEDS.search(context_before):
                i = k
                continue

            # Look for seeds comment in the context before the struct
            seeds_context = "\n".join(lines[max(0, i - 15):j])
            if not _SEEDS_COMMENT.search(seeds_context):
                # No seeds documented, skip this struct
                i = k
                continue

            # Extract variable seeds from the comment
            variable_seeds = []
            for line_idx in range(max(0, i - 15), j):
                line = lines[line_idx]
                if _SEEDS_COMMENT.search(line):
                    # Collect the full seeds array (may span multiple lines)
                    seeds_text = line
                    for next_idx in range(line_idx + 1, min(line_idx + 5, j)):
                        seeds_text += " " + lines[next_idx]
                        if ']' in lines[next_idx]:
                            break
                    variable_seeds = _extract_variable_seeds(seeds_text)
                    break

            if not variable_seeds:
                i = k
                continue

            # Get all fields of this struct (including nested)
            direct_fields = set(_FIELD_DECL.findall(body))

            # Also check fields of nested struct types (e.g., pub base: BaseOverrideConfig)
            all_fields = set(direct_fields)
            type_pattern = re.compile(r'pub\s+\w+\s*:\s*(\w+)')
            for type_match in type_pattern.finditer(body):
                nested_type = type_match.group(1)
                if nested_type in structs_cache:
                    all_fields.update(_get_struct_fields_recursive(nested_type, structs_cache))

            # Normalize field names for comparison (handle remote_chain_selector vs chain_selector)
            normalized_fields = set()
            for f in all_fields:
                normalized_fields.add(f.lower())
                # Also add without common prefixes
                for prefix in ['remote_', 'source_', 'dest_']:
                    if f.lower().startswith(prefix):
                        normalized_fields.add(f.lower()[len(prefix):])

            # Check which seeds are missing
            missing_seeds = []
            for seed in variable_seeds:
                seed_lower = seed.lower()
                # Check direct match or with common prefixes
                found = seed_lower in normalized_fields
                if not found:
                    for prefix in ['remote_', 'source_', 'dest_']:
                        if (prefix + seed_lower) in normalized_fields:
                            found = True
                            break
                if not found:
                    missing_seeds.append(seed)

            if missing_seeds:
                report(rel(path, root), j + 1, "INV-LINT-PDA-SEEDS",
                       f"PDA `{struct_name}` has variable seeds not present as fields: {', '.join(missing_seeds)} — "
                       "all variable PDA seeds must be stored in the account; suppress existing ones with "
                       "accept via `make anchor-lint-baseline`",
                       "error")
            i = k
        else:
            i += 1


# ── INV-LINT-BUMP – bare bump in non-init PDA field constraint ────────────────
# When accessing (not initialising) a PDA via seeds, the bump should reference
# the stored value (e.g. bump = state.bump) instead of a bare "bump" which
# forces Anchor to recompute it at runtime.

_HAS_INIT = re.compile(r'\b(init|init_if_needed)\b')
_BARE_BUMP = re.compile(r'\bbump\b(?!\s*=)')
_STORED_BUMP = re.compile(r'\bbump\s*=')
_SUPPRESS_LINT_BUMP = _NEVER
_ACCOUNT_FIELD_START = re.compile(r'#\s*\[\s*account\s*\(')
_ACCOUNT_TYPE = re.compile(r'(?:Account|AccountLoader)\s*<\s*\'info\s*,\s*(\w+)\s*>')
_CROSS_PROGRAM_PDA = re.compile(r'seeds\s*::\s*program\b')
_NO_DATA_FIELD = re.compile(r':\s*(UncheckedAccount|Signer)\s*<')

_BUMP_SUPPRESSED_TYPES: Optional[Dict[str, Set[str]]] = None
_BUMP_ALL_TYPES: Optional[Dict[str, Set[str]]] = None


def _bump_suppressed_types_set(root: Path) -> Dict[str, Set[str]]:
    """Build per-program dict of struct names exempted from the bump requirement.
    Comment-based exemption is gone (suppression lives in the JSON baseline), so this
    now only populates the all-types map. Returns {program_key: {type_name, ...}}.
    Also populates _BUMP_ALL_TYPES with all struct names per program."""
    global _BUMP_SUPPRESSED_TYPES, _BUMP_ALL_TYPES
    if _BUMP_SUPPRESSED_TYPES is not None:
        return _BUMP_SUPPRESSED_TYPES
    suppressed: Dict[str, Set[str]] = defaultdict(set)
    all_types: Dict[str, Set[str]] = defaultdict(set)
    scan_dirs = [root / "programs"]
    if (root / "crates").is_dir():
        scan_dirs.append(root / "crates")
    for d in scan_dirs:
        for path in rs_files(d):
            pk = program_key(path) or ""
            lns = lines_of(path)
            for i, ln in enumerate(lns):
                m = re.match(r'\s*pub\s+struct\s+(\w+)', ln)
                if m:
                    all_types[pk].add(m.group(1))
                    ctx = "\n".join(lns[max(0, i - 5):i + 1])
                    if _SUPPRESS_LINT_BUMP.search(ctx) or _SUPPRESS_LINT_PDA.search(ctx):
                        suppressed[pk].add(m.group(1))
    _BUMP_SUPPRESSED_TYPES = suppressed
    _BUMP_ALL_TYPES = all_types
    return suppressed


def _is_type_bump_suppressed(type_name: str, current_pk: str, root: Path) -> bool:
    """Check if a type is bump-suppressed for a given program.
    If the type is defined in the current program, use that program's suppress status.
    Otherwise fall back to checking other programs/crates (shared types)."""
    suppressed_map = _bump_suppressed_types_set(root)
    all_types = _BUMP_ALL_TYPES or {}
    # If the current program defines this type, use its own suppress status
    if type_name in all_types.get(current_pk, set()):
        return type_name in suppressed_map.get(current_pk, set())
    # Fall back: type defined elsewhere (shared crate) — suppressed if any definition says so
    for pk, types in suppressed_map.items():
        if type_name in types:
            return True
    return False


def inv_lint_bump(path: Path, lines: List[str], root: Path) -> None:
    """Flag #[account] field constraints that use bare bump on non-init PDA fields."""
    i = 0
    while i < len(lines):
        if not _DERIVE_ACCOUNTS.search(lines[i]):
            i += 1
            continue

        # Collect the full struct block
        struct_start = i + 1
        depth = 0
        j = struct_start
        while j < len(lines):
            depth += lines[j].count('{') - lines[j].count('}')
            j += 1
            if depth <= 0 and j > struct_start + 1:
                break

        # Scan fields inside the struct
        fi = struct_start
        while fi < j:
            ln = lines[fi]
            # Look for field-level #[account(...)] attributes (may span multiple lines)
            if not _ACCOUNT_FIELD_START.search(ln):
                fi += 1
                continue

            # Collect full multi-line attribute text until bracket depth returns to 0
            attr_text = ln
            ai = fi
            bracket_depth = ln.count('[') - ln.count(']')
            while bracket_depth > 0 and ai + 1 < j:
                ai += 1
                attr_text += " " + lines[ai].strip()
                bracket_depth += lines[ai].count('[') - lines[ai].count(']')

            # Only interested in PDA fields (have seeds constraint)
            if not _PDA_SIGNER_CONSTRAINT.search(attr_text):
                fi = ai + 1
                continue

            # Skip init/init_if_needed — bare bump is expected
            if _HAS_INIT.search(attr_text):
                fi = ai + 1
                continue

            # Skip cross-program PDAs — bump belongs to another program
            if _CROSS_PROGRAM_PDA.search(attr_text):
                fi = ai + 1
                continue

            # Skip UncheckedAccount / Signer fields — no account data to store bump
            field_ctx = "\n".join(lines[ai + 1:min(ai + 4, j)])
            if _NO_DATA_FIELD.search(field_ctx):
                fi = ai + 1
                continue

            # Check for bare bump (no = after bump)
            if _BARE_BUMP.search(attr_text) and not _STORED_BUMP.search(attr_text):
                # Check for suppress comment in surrounding lines
                ctx = "\n".join(lines[max(0, fi - 2):ai + 3])
                if not _SUPPRESS_LINT_BUMP.search(ctx):
                    # Check if the field's Account type is suppressed at its struct definition
                    type_suppressed = False
                    pk = program_key(path) or ""
                    for fli in range(ai + 1, min(ai + 5, j)):
                        tm = _ACCOUNT_TYPE.search(lines[fli])
                        if tm and _is_type_bump_suppressed(tm.group(1), pk, root):
                            type_suppressed = True
                            break
                    if not type_suppressed:
                        report(rel(path, root), fi + 1, "INV-LINT-BUMP",
                               "PDA field uses bare `bump` — use a stored bump "
                               "(e.g. `bump = account.bump`) for non-init accounts; "
                               "accept via `make anchor-lint-baseline`",
                               "error")

            fi = ai + 1

        i = j


# ── INV-LINT-CLONE – Repeated .clone() on same variable in hot paths ─────────

_HOT_PATH_DIRS = ("ccip_send", "get_fee", "execute")
_CLONE_CALL = re.compile(r'([\w.]+)\s*\.\s*clone\s*\(\s*\)')
_SUPPRESS_LINT_CLONE = re.compile(r'SAFETY:', re.IGNORECASE)


def _is_hot_path(path: Path) -> bool:
    return any(hp in str(path) for hp in _HOT_PATH_DIRS)


def _block_ids(lines: List[str]) -> List[Tuple[int, ...]]:
    """Assign each line the identity of the innermost `{}` block containing it.

    Two lines share an id only when the same block encloses both, so clones in
    sibling `if`/`else` arms — or in different functions — get different ids and
    are never treated as repeats of one another. Without this the check is purely
    textual and flags mutually exclusive code paths, which can't both run.
    """
    ids: List[Tuple[int, ...]] = []
    stack: List[int] = []
    counter = 0
    for ln in lines:
        code = strip_inline_comment(ln)
        # The id of a line is the block it is *inside*, so record before applying
        # this line's own braces for opens, and after for closes.
        closes = code.count('}')
        opens = code.count('{')
        for _ in range(min(closes, len(stack))):
            stack.pop()
        ids.append(tuple(stack))
        for _ in range(opens):
            counter += 1
            stack.append(counter)
    return ids


def inv_lint_clone(path: Path, lines: List[str], root: Path) -> None:
    if not _is_hot_path(path):
        return
    test_lines: Set[int] = _test_line_set(lines)
    blocks = _block_ids(lines)
    # Key on (block, variable): only clones the same block can execute together
    # count as repeats.
    clone_sites: Dict[Tuple[Tuple[int, ...], str], List[int]] = defaultdict(list)
    for i, ln in enumerate(lines, 1):
        if is_comment(ln) or (i - 1) in test_lines:
            continue
        if _SUPPRESS_LINT_CLONE.search(ln):
            continue
        if any(_SUPPRESS_LINT_CLONE.search(lines[j]) for j in range(max(0, i - 4), i - 1)):
            continue
        for m in _CLONE_CALL.finditer(ln):
            clone_sites[(blocks[i - 1], m.group(1))].append(i)
    for (_, var), site_lines in clone_sites.items():
        if len(site_lines) >= 2:
            locs = ", ".join(str(l) for l in site_lines[:4])
            report(rel(path, root), site_lines[0], "INV-LINT-CLONE",
                   f"hot path: `{var}.clone()` appears {len(site_lines)}× (lines {locs}) "
                   "in the same block — consider passing by reference or restructuring to "
                   "avoid repeated clones; accept via `make anchor-lint-baseline`",
                   "warning")


# ── INV-LINT-ARITH – Raw arithmetic operators in hot paths ─────────────────────

# Match  a + b  /  a - b  /  a * b  but not  +=  -=  *=  or  ->
_RAW_ARITH = re.compile(
    r'(?<!\w)'                  # not preceded by word char
    r'(\w[\w.]*)'               # lhs operand
    r'\s*([+\-\*])'             # operator
    r'(?![>=\-])'               # not +=, -=, *=, ->, --
    r'\s*(\w[\w.]*)'            # rhs operand
)
_SAFE_ARITH = re.compile(r'checked_|saturating_|wrapping_|overflowing_')
_SUPPRESS_LINT_ARITH = re.compile(r'SAFETY:', re.IGNORECASE)
# Lines that are clearly not arithmetic expressions
_SKIP_ARITH_LINE = re.compile(r'^\s*(?:#|use\s|pub\s+(?:fn|struct|enum)|//|impl\s|let\s+\w+\s*[=<]|space\s*=)')
# Dereference *: the lhs is a keyword or the * is preceded by non-alphanumeric context
_DEREF_LHS = re.compile(r'^(if|match|while|return|assert|assert_eq|assert_ne|require|Ok|Err|Some)$')


def inv_lint_arith(path: Path, lines: List[str], root: Path) -> None:
    if not _is_hot_path(path):
        return
    # Pre-compute test-block line set for this file
    test_lines: Set[int] = _test_line_set(lines)
    for i, ln in enumerate(lines, 1):
        if is_comment(ln) or _SAFE_ARITH.search(ln) or _SKIP_ARITH_LINE.match(ln):
            continue
        if (i - 1) in test_lines:
            continue
        if _SUPPRESS_LINT_ARITH.search(ln):
            continue
        # Also check the preceding lines (up to 3) for a SAFETY comment
        if any(_SUPPRESS_LINT_ARITH.search(lines[j]) for j in range(max(0, i - 4), i - 1)):
            continue
        # Strip inline comments before matching to avoid false positives
        code = strip_inline_comment(ln)
        # Skip format strings, log lines
        if any(kw in code for kw in ('msg!(', 'format!', 'println!', '"', "'")):
            continue
        m = _RAW_ARITH.search(code)
        if m:
            op_sym = m.group(2)
            # Skip dereference *: `if *foo`, `match *bar`, etc.
            if op_sym == '*' and _DEREF_LHS.match(m.group(1)):
                continue
            op_name = {'+': 'add', '-': 'sub', '*': 'mul'}[op_sym]
            report(rel(path, root), i, "INV-LINT-ARITH",
                   f"hot path: raw `{op_sym}` — consider checked_{op_name}() "
                   "or document why overflow is impossible here; "
                   "suppress with // SAFETY: <reason>",
                   "info")


# ── INV-LINT-HEAP – Heap allocations in hot paths ───────────────────────────────

# `Vec::new()`, `String::new()` and the empty `vec![]` literal do NOT allocate —
# std defers the allocation until the first push — so they are deliberately absent
# here. Flagging them produced noise that drowned out the real allocations.
_HEAP_ALLOC = re.compile(
    r'\b(Box::new|vec!\s*\[|\.to_vec\s*\(|String::from\s*\()'
)
# `vec![]` / `vec![ ]` with nothing between the brackets: capacity 0, no allocation.
_EMPTY_VEC_LITERAL = re.compile(r'vec!\s*\[\s*\]')
_SUPPRESS_LINT_HEAP = re.compile(r'SAFETY:', re.IGNORECASE)


def inv_lint_heap(path: Path, lines: List[str], root: Path) -> None:
    if not _is_hot_path(path):
        return
    test_lines: Set[int] = _test_line_set(lines)
    for i, ln in enumerate(lines, 1):
        if is_comment(ln) or (i - 1) in test_lines:
            continue
        if _SUPPRESS_LINT_HEAP.search(ln):
            continue
        # Preceding 3 lines / following 2, matching the other suppressible rules.
        if any(_SUPPRESS_LINT_HEAP.search(lines[j]) for j in range(max(0, i - 4), i - 1)):
            continue
        if any(_SUPPRESS_LINT_HEAP.search(lines[j]) for j in range(i, min(len(lines), i + 2))):
            continue
        code = strip_inline_comment(ln)
        m = _HEAP_ALLOC.search(code)
        if not m:
            continue
        # An empty `vec![]` is the only `vec!` form that allocates nothing; skip it
        # unless the line also holds a genuinely allocating form.
        if m.group(1).startswith('vec!') and _EMPTY_VEC_LITERAL.search(code):
            rest = _EMPTY_VEC_LITERAL.sub('', code)
            m = _HEAP_ALLOC.search(rest)
            if not m:
                continue
        report(rel(path, root), i, "INV-LINT-HEAP",
               f"hot path: `{m.group(1)}` allocates on the heap — "
               "prefer stack buffers or pre-allocated structures; "
               "accept via `make anchor-lint-baseline`",
               "warning")


# ── INV-LINT-CAST – Narrowing `as` casts ─────────────────────────────────────

_NARROWING_CAST = re.compile(
    r'(\S+)\s+as\s+(u8|u16|u32|i8|i16|i32)\b'
)
_SUPPRESS_LINT_CAST = re.compile(r'SAFETY:', re.IGNORECASE)

_INT_WIDTH = {"u8": 8, "u16": 16, "u32": 32, "u64": 64, "u128": 128,
              "i8": 8, "i16": 16, "i32": 32, "i64": 64, "i128": 128}
# Declarations of the form `pub name: u16,` / `name: u16,` / `let name: u16 =`.
_INT_FIELD_DECL = re.compile(
    r'(?:pub\s+)?(?:let\s+(?:mut\s+)?)?(\w+)\s*:\s*(u8|u16|u32|u64|u128|i8|i16|i32|i64|i128)\s*[,;=)]')
_DECLARED_INT_TYPES: Optional[Dict[str, Set[str]]] = None


def _declared_int_types(root: Path) -> Dict[str, Set[str]]:
    """Map every field/binding name in the tree to the integer types it is declared as.

    Used only to prove a cast *widens*. A name is trusted solely when every
    declaration of it agrees, so a collision between two same-named fields of
    different widths falls back to reporting.
    """
    global _DECLARED_INT_TYPES
    if _DECLARED_INT_TYPES is not None:
        return _DECLARED_INT_TYPES
    types: Dict[str, Set[str]] = defaultdict(set)
    for p in root.rglob("*.rs"):
        try:
            for ln in p.read_text(encoding="utf-8", errors="ignore").splitlines():
                if is_comment(ln):
                    continue
                for m in _INT_FIELD_DECL.finditer(ln):
                    types[m.group(1)].add(m.group(2))
        except OSError:
            continue
    _DECLARED_INT_TYPES = types
    return types


def _local_int_types(lines: List[str]) -> Dict[str, Set[str]]:
    """Integer declarations visible in a single file."""
    types: Dict[str, Set[str]] = defaultdict(set)
    for ln in lines:
        if is_comment(ln):
            continue
        for m in _INT_FIELD_DECL.finditer(ln):
            types[m.group(1)].add(m.group(2))
    return types


def _cast_is_widening(operand: str, target: str, root: Path,
                      local: Dict[str, Set[str]]) -> bool:
    """True when the operand's declared type provably fits in `target`."""
    raw = operand.strip().strip('&*()?')
    dotted = '.' in raw
    name = raw.rsplit('.', 1)[1] if dotted else raw
    name = name.strip('()?')
    if not name.isidentifier():
        return False
    if dotted:
        # A field access carries a specific name, so a tree-wide lookup is safe and
        # is the only way to see a field declared in another file's state module.
        declared = _declared_int_types(root).get(name)
    else:
        # A bare local like `n` collides constantly across files — one unrelated
        # `n: u8` elsewhere would wrongly clear every `n as u8` in the tree. Trust
        # only declarations visible in this file.
        declared = local.get(name)
    if not declared:
        return False
    tw = _INT_WIDTH[target]
    t_signed = target.startswith('i')
    # Every declaration must fit, and signedness must match — `i16 as u32` is not
    # a widening conversion, it reinterprets negatives.
    return all(
        src in _INT_WIDTH and src.startswith('i') == t_signed and _INT_WIDTH[src] <= tw
        for src in declared
    )


def inv_lint_cast(path: Path, lines: List[str], root: Path) -> None:
    # Pre-compute test-block line set for this file
    test_lines: Set[int] = _test_line_set(lines)
    local_types = _local_int_types(lines)
    for i, ln in enumerate(lines, 1):
        if is_comment(ln):
            continue
        if (i - 1) in test_lines:
            continue
        if _SUPPRESS_LINT_CAST.search(ln):
            continue
        # Also check the preceding lines (up to 3) for a SAFETY comment
        if any(_SUPPRESS_LINT_CAST.search(lines[j]) for j in range(max(0, i - 4), i - 1)):
            continue
        # Also check the following lines (up to 2) for a SAFETY comment
        if any(_SUPPRESS_LINT_CAST.search(lines[j]) for j in range(i, min(len(lines), i + 2))):
            continue
        # Skip lines that are clearly not narrowing (format strings, use/mod decls)
        if any(kw in ln for kw in ('msg!(', 'format!', 'println!', '"', "'")):
            continue
        m = _NARROWING_CAST.search(ln)
        if m:
            operand, target = m.group(1), m.group(2)
            # `u16 as u32` and friends widen: no value can be lost, nothing to report.
            if _cast_is_widening(operand, target, root, local_types):
                continue
            report(rel(path, root), i, "INV-LINT-CAST",
                   f"narrowing `as {target}` cast — prefer .try_into()? or "
                   "validate the range before casting; suppress with // SAFETY: <reason>",
                   "warning")


# ── INV-LINT-UNWRAP – try_into()/try_from().unwrap() ──────────────────────────

_FALLIBLE_UNWRAP = re.compile(
    r'\.(try_into|try_from)\s*\([^)]*\)\s*\.\s*unwrap\s*\(\s*\)'
    r'|'
    r'\btry_from\s*\([^)]*\)\s*\.\s*unwrap\s*\(\s*\)'
)
_SUPPRESS_LINT_UNWRAP = re.compile(r'SAFETY:', re.IGNORECASE)


def inv_lint_unwrap(path: Path, lines: List[str], root: Path) -> None:
    for i, ln in enumerate(lines, 1):
        if is_comment(ln):
            continue
        if _SUPPRESS_LINT_UNWRAP.search(ln):
            continue
        # Also check the preceding lines (up to 3) for a SAFETY comment
        if any(_SUPPRESS_LINT_UNWRAP.search(lines[j]) for j in range(max(0, i - 4), i - 1)):
            continue
        if _FALLIBLE_UNWRAP.search(ln):
            report(rel(path, root), i, "INV-LINT-UNWRAP",
                   "try_into()/try_from().unwrap() can panic — "
                   "use .map_err(|_| error!(…))? or .ok_or(error!(…))? instead",
                   "warning")


# ── INV-LINT-REALLOC – realloc without realloc::zero = true ───────────────────

_REALLOC_KW = re.compile(r'\brealloc\s*=')
_REALLOC_ZERO = re.compile(r'realloc\s*::\s*zero\s*=\s*true')
_SUPPRESS_LINT_REALLOC = _NEVER


def inv_lint_realloc(path: Path, lines: List[str], root: Path) -> None:
    """Flag #[account(realloc = ...)] without realloc::zero = true."""
    i = 0
    while i < len(lines):
        ln = lines[i]
        if is_comment(ln) or not _REALLOC_KW.search(ln):
            i += 1
            continue
        # Collect the full attribute (may span multiple lines). Anchored at the
        # enclosing `#[account(` and closed on bracket depth rather than the first
        # `]`: an expression such as `dynamic_space(&senders, &[])` closes a bracket
        # mid-attribute, which would otherwise truncate the span and hide a
        # suppression comment sitting below it.
        start = i
        while start > 0 and '#[' not in lines[start]:
            start -= 1
        attr_text = ""
        depth = 0
        j = start
        while j < len(lines):
            code = strip_inline_comment(lines[j])
            attr_text += " " + lines[j].strip()
            depth += code.count('[') + code.count('(') - code.count(']') - code.count(')')
            if depth <= 0 and j >= i:
                break
            j += 1
        if _SUPPRESS_LINT_REALLOC.search(attr_text):
            i = j + 1
            continue
        if _REALLOC_KW.search(attr_text) and not _REALLOC_ZERO.search(attr_text):
            report(rel(path, root), i + 1, "INV-LINT-REALLOC",
                   "`realloc` without `realloc::zero = true` — extended space may "
                   "contain stale data; add `realloc::zero = true` or "
                   "accept via `make anchor-lint-baseline`",
                   "info")
        i = j + 1


# ── INV-LINT-ACCTINIT – account-absence keyed on AccountNotInitialized ───────
# An optional account must be recognised as "absent / uninitialised" from its
# ownership (owner == System Program), which is lamport-agnostic and cannot be
# perturbed by an unprivileged actor. Deciding absence from the specific
# `AccountNotInitialized` error alone is exploitable: Anchor returns that error
# only for a System-owned AND zero-lamport account, so pre-funding the (public-seed)
# PDA flips the result to `AccountOwnedByWrongProgram` and the "absent" branch is
# never taken. See contracts/invariants/ACCOUNT_INIT_INVARIANTS.md (INV-ACCT-INIT-2/3).

_ACCOUNT_NOT_INIT = re.compile(r'\bAccountNotInitialized\b')
# Lamport-agnostic guard: an owner-vs-System-Program comparison in the same function.
_SYSTEM_OWNER_GUARD = re.compile(r'\bsystem_program\s*::\s*(?:ID|id\s*\(\s*\))')
# Alternative guard: the function also handles the "wrong owner" error explicitly.
_WRONG_PROGRAM_GUARD = re.compile(r'\bAccountOwnedByWrongProgram\b')
_SUPPRESS_LINT_ACCTINIT = re.compile(r'SAFETY:', re.IGNORECASE)
_FN_DECL = re.compile(r'\bfn\s+(\w+)\s*(?:<[^>]*>)?\s*\(')
# A typed-account deserialization (Account/InterfaceAccount/AccountLoader::try_from) …
_ACCT_TRYFROM = re.compile(
    r'\b(?:Account|InterfaceAccount|AccountLoader)\s*::\s*<[^>]*>\s*::\s*try_from\s*\(')
# … whose Result is swallowed into an Option/default, discarding the error (and thus the
# owner check). `.ok_or(` is excluded (the `_` after `ok` breaks the `ok\s*\(` match).
_ERR_SWALLOW = re.compile(r'\.\s*(?:unwrap_or_default|unwrap_or|ok)\s*\(')


def _enclosing_fn_body(lines: List[str], idx: int) -> str:
    """Return the source of the function enclosing 0-based line `idx`.

    Walks backwards to the nearest `fn NAME(` declaration whose brace-delimited
    body contains `idx`; falls back to a local window if none is found.
    """
    for i in range(idx, -1, -1):
        if not _FN_DECL.search(lines[i]):
            continue
        depth = 0
        started = False
        j = i
        while j < len(lines):
            depth += lines[j].count('{') - lines[j].count('}')
            if '{' in lines[j]:
                started = True
            if started and depth <= 0:
                if i <= idx <= j:
                    return "\n".join(lines[i:j + 1])
                break  # this fn ended before idx — keep searching backwards
            j += 1
    # Fallback: a local window (keeps the check working outside a fn body).
    return "\n".join(lines[max(0, idx - 15):idx + 16])


def inv_lint_acctinit(path: Path, lines: List[str], root: Path) -> None:
    """Flag unsafe absence handling on a typed-account deserialization.

    Two forms are raised (both keyed to INV-ACCT-INIT-2/3), unless the enclosing
    function contains a lamport-agnostic safeguard (owner-vs-System-Program
    comparison or explicit `AccountOwnedByWrongProgram` handling):
      1. absence decided from the `AccountNotInitialized` error alone;
      2. the deserialization Result swallowed with `.ok()` / `unwrap_or(..)`, which
         also treats a wrong-owner or malformed account as "absent".
    """
    # Form 1 — narrow match on the AccountNotInitialized error.
    for i, ln in enumerate(lines, 1):
        if is_comment(ln) or not _ACCOUNT_NOT_INIT.search(ln):
            continue
        body = _enclosing_fn_body(lines, i - 1)
        if _SYSTEM_OWNER_GUARD.search(body) or _WRONG_PROGRAM_GUARD.search(body):
            continue  # lamport-agnostic absence handling present
        # Suppress via SAFETY / explicit suppress comment near the match arm.
        ctx = "\n".join(lines[max(0, i - 4):i + 1])
        if _SUPPRESS_LINT_ACCTINIT.search(ctx):
            continue
        report(rel(path, root), i, "INV-LINT-ACCTINIT",
               "account-absence decided from `AccountNotInitialized` alone — an "
               "unprivileged lamport transfer to the (public-seed) PDA makes Anchor "
               "return `AccountOwnedByWrongProgram` instead, so the absent branch is "
               "skipped and the instruction reverts; detect absence from "
               "`owner == system_program::ID` (lamport-agnostic) or handle both errors; "
               "accept via `make anchor-lint-baseline`",
               "error")

    # Form 2 — deserialization error swallowed with .ok()/.unwrap_or(..).
    for i, ln in enumerate(lines, 1):
        if is_comment(ln) or not _ACCT_TRYFROM.search(ln):
            continue
        window = ln + ("\n" + lines[i] if i < len(lines) else "")  # allow a chained .ok() on the next line
        if not _ERR_SWALLOW.search(window):
            continue
        body = _enclosing_fn_body(lines, i - 1)
        if _SYSTEM_OWNER_GUARD.search(body) or _WRONG_PROGRAM_GUARD.search(body):
            continue
        ctx = "\n".join(lines[max(0, i - 4):i + 1])
        if _SUPPRESS_LINT_ACCTINIT.search(ctx):
            continue
        report(rel(path, root), i, "INV-LINT-ACCTINIT",
               "typed-account deserialization error swallowed with `.ok()` / `unwrap_or` — "
               "this treats a wrong-owner or malformed account as absent, hiding a "
               "substituted or misconfigured account; match the error explicitly and detect "
               "absence from `owner == system_program::ID`; "
               "accept via `make anchor-lint-baseline`",
               "error")


# ── INV-LINT-ATAINIT – non-idempotent init of an associated token account ─────
# Associated Token Account creation is permissionless: anyone can create the ATA
# for any (mint, authority) pair. A non-idempotent `init` performs a hard create
# that fails if the account already exists, so a third party can pre-create the
# ATA and permanently block the instruction (including an admin recovery path).
# `init_if_needed` adopts an existing ATA instead of failing. See
# contracts/invariants/ACCOUNT_INIT_INVARIANTS.md (INV-ACCT-INIT-5).
#
# NOTE: this targets ATAs specifically, not program-owned PDAs created with `init`.
# Only the program can allocate/assign its own PDAs, so those cannot be pre-created
# by a third party (a mere lamport prefund does not block their `init`).

_ASSOC_TOKEN = re.compile(r'associated_token\s*::')
# Bare `init` — the \b after "init" fails to match `init_if_needed` (next char is `_`).
_BARE_INIT = re.compile(r'\binit\b')
# Non-idempotent ATA-creation CPI calls. The trailing `\(` fails to match the
# `_idempotent` variants (`create_associated_token_account_idempotent(`,
# `associated_token::create_idempotent(`), whose next char after the matched token is `_`.
_ATA_CPI_CREATE = re.compile(
    r'\bcreate_associated_token_account\s*\('
    r'|'
    r'\bassociated_token\s*::\s*create\s*\(')
_SUPPRESS_LINT_ATAINIT = _NEVER


def inv_lint_atainit(path: Path, lines: List[str], root: Path) -> None:
    """Flag non-idempotent creation of an associated token account.

    Two forms are detected:
      1. Anchor field constraint: `#[account(init, ..., associated_token::...)]`.
      2. A direct CPI to the non-idempotent create (`create_associated_token_account`
         / `associated_token::create`) rather than its `_idempotent` variant.
    ATA creation is permissionless, so a pre-created ATA makes a non-idempotent create
    fail permanently — blocking the instruction and any admin recovery that depends on it.
    """
    # Form 1 — Anchor `#[account(init, ... associated_token::...)]` field constraints.
    i = 0
    while i < len(lines):
        ln = lines[i]
        if is_comment(ln) or not _ACCOUNT_FIELD_START.search(ln):
            i += 1
            continue
        # Collect the full (possibly multi-line) #[account(...)] attribute.
        attr_text = ln
        j = i
        bracket_depth = ln.count('[') - ln.count(']')
        while bracket_depth > 0 and j + 1 < len(lines):
            j += 1
            attr_text += " " + lines[j].strip()
            bracket_depth += lines[j].count('[') - lines[j].count(']')
        if _ASSOC_TOKEN.search(attr_text) and _BARE_INIT.search(attr_text):
            ctx = "\n".join(lines[max(0, i - 2):j + 2])
            if not _SUPPRESS_LINT_ATAINIT.search(ctx):
                report(rel(path, root), i + 1, "INV-LINT-ATAINIT",
                       "`init` on an associated token account — ATA creation is "
                       "permissionless, so anyone can pre-create this ATA and make `init` "
                       "fail permanently, blocking the instruction (and any admin recovery "
                       "that depends on it); use `init_if_needed` to adopt an existing ATA; "
                       "accept via `make anchor-lint-baseline`",
                       "error")
        i = j + 1

    # Form 2 — direct CPI to the non-idempotent ATA-creation instruction.
    for k, ln in enumerate(lines, 1):
        if is_comment(ln) or not _ATA_CPI_CREATE.search(ln):
            continue
        ctx = "\n".join(lines[max(0, k - 3):k + 1])
        if _SUPPRESS_LINT_ATAINIT.search(ctx):
            continue
        report(rel(path, root), k, "INV-LINT-ATAINIT",
               "non-idempotent associated-token-account creation CPI — ATA creation is "
               "permissionless, so a pre-created ATA makes this call fail permanently; "
               "use the `_idempotent` variant (create_associated_token_account_idempotent "
               "/ associated_token::create_idempotent); "
               "accept via `make anchor-lint-baseline`",
               "error")


# ── INV-LINT-REMACCT – manual deser of a remaining_accounts entry w/o identity ─
# `Account::try_from` / `try_from_slice` on a remaining_accounts entry proves owner
# + discriminator but NOT the account's key/PDA identity. Without an explicit
# require_keys_eq! against the expected address, a different valid account of the
# same type can be substituted. See ACCOUNT_INIT_INVARIANTS.md (INV-ACCT-INIT-4).

_REMAINING_DESER = re.compile(
    r'(?:try_from|try_from_slice)\s*\(\s*&?\s*(?:ctx\s*\.\s*)?(?:accounts\s*\.\s*)?'
    r'remaining_accounts\s*\[')
_IDENTITY_PIN = re.compile(r'require_keys_eq!|require_keys_neq!|find_program_address')
_SUPPRESS_LINT_REMACCT = _NEVER


def inv_lint_remacct(path: Path, lines: List[str], root: Path) -> None:
    """Flag manual deserialization of a remaining_accounts entry without a key check."""
    for i, ln in enumerate(lines, 1):
        if is_comment(ln) or not _REMAINING_DESER.search(ln):
            continue
        body = _enclosing_fn_body(lines, i - 1)
        if _IDENTITY_PIN.search(body):
            continue  # identity pinned somewhere in the function
        ctx = "\n".join(lines[max(0, i - 4):i + 1])
        if _SUPPRESS_LINT_REMACCT.search(ctx):
            continue
        report(rel(path, root), i, "INV-LINT-REMACCT",
               "manual deserialization of a `remaining_accounts` entry without an identity "
               "check — try_from/try_from_slice prove owner + discriminator but not the "
               "account's key/PDA, so a different valid account could be substituted; pin it "
               "with require_keys_eq!(...) against the expected address before use "
               "(INV-ACCT-INIT-4); accept via `make anchor-lint-baseline`",
               "warning")


# ── INV-LINT-CREATEACCT – manual create_account without pre-funded handling ────
# The system program's create_account fails if the target address already holds
# lamports, so a public-seed PDA can be permanently griefed by a pre-fund. A robust
# manual creation handles the pre-funded path with allocate + assign. Anchor's
# `#[account(init)]` does this internally; a hand-rolled create_account must too.
# See ACCOUNT_INIT_INVARIANTS.md (INV-ACCT-INIT-6).

_CREATE_ACCOUNT_CPI = re.compile(r'\bcreate_account\s*\(')
_PREFUND_HANDLING = re.compile(r'\b(?:allocate|assign)\b')
_SUPPRESS_LINT_CREATEACCT = _NEVER


def inv_lint_createacct(path: Path, lines: List[str], root: Path) -> None:
    """Flag a manual create_account CPI whose function lacks pre-funded handling."""
    for i, ln in enumerate(lines, 1):
        if is_comment(ln) or not _CREATE_ACCOUNT_CPI.search(ln):
            continue
        body = _enclosing_fn_body(lines, i - 1)
        if _PREFUND_HANDLING.search(body):
            continue  # allocate/assign fallback present
        ctx = "\n".join(lines[max(0, i - 4):i + 1])
        if _SUPPRESS_LINT_CREATEACCT.search(ctx):
            continue
        report(rel(path, root), i, "INV-LINT-CREATEACCT",
               "manual `create_account` CPI — the system program's create_account fails if "
               "the target address was pre-funded, so a public-seed PDA can be permanently "
               "griefed; handle the pre-funded path with allocate + assign (or confirm the "
               "address cannot be pre-funded) (INV-ACCT-INIT-6); "
               "accept via `make anchor-lint-baseline`",
               "warning")


# ── Main ──────────────────────────────────────────────────────────────────────

CHECK_DESCRIPTIONS = {
    "INV-LINT-SERIAL": ("error",   ".try_to_vec() usage"),
    "INV-LINT-ASSERT": ("info",    "require!() — consider a typed variant (require_eq!, require_keys_eq!, ...)"),
    "INV-LINT-EVENT":  ("warning", "emit!/emit_cpi! inconsistency with #[event_cpi] contexts"),
    "INV-LINT-SIGNER": ("error",   "Signer without address/owner/seeds constraint"),
    "INV-LINT-INIT":   ("warning", "init_if_needed without authorised Signer"),
    "INV-LINT-INSTR":  ("error",   "#[instruction] param order mismatch"),
    "INV-LINT-PARAMS": ("warning", "#[program] handler should use single *Params arg after Context"),
    "INV-LINT-PDA":    ("error",   "PDA struct missing version/bump fields (mandatory for new PDAs)"),
    "INV-LINT-PDA-SEEDS": ("error", "PDA variable seeds not present as account fields"),
    "INV-LINT-CLONE":  ("warning", "Repeated .clone() in hot path"),
    "INV-LINT-ARITH":  ("info",    "Raw arithmetic operator in hot path"),
    "INV-LINT-HEAP":    ("warning", "Heap allocation in hot path"),
    "INV-LINT-CAST":    ("warning", "Narrowing `as` cast (INV-ENC-10/12)"),
    "INV-LINT-UNWRAP":  ("warning", "try_into()/try_from().unwrap() — fallible conversion (INV-ENC-10)"),
    "INV-LINT-REALLOC": ("info",    "realloc without realloc::zero = true (INV-ATOMIC-2)"),
    "INV-LINT-BUMP":    ("error",   "bare bump in non-init PDA field (use stored bump)"),
    "INV-LINT-ACCTINIT": ("error", "account-absence keyed on AccountNotInitialized (INV-ACCT-INIT-2/3)"),
    "INV-LINT-ATAINIT": ("error", "non-idempotent init on an associated token account (INV-ACCT-INIT-5)"),
    "INV-LINT-REMACCT": ("warning", "remaining_accounts manual deser without identity check (INV-ACCT-INIT-4)"),
    "INV-LINT-CREATEACCT": ("warning", "manual create_account without pre-funded handling (INV-ACCT-INIT-6)"),
    "INV-LINT-SET-INNER": ("info",   "field-by-field account population — prefer Account::set_inner"),
}

# ── INV-LINT-SET-INNER – prefer Account::set_inner over field-by-field population ─────────────
# A plain field assignment `<base>.<field> = <expr>` where <base> is a simple identifier or
# `ctx.accounts.<name>`. Excludes compound/`==` via the negative lookahead after `=`.
_FIELD_ASSIGN = re.compile(r'^\s*((?:ctx\.accounts\.)?[A-Za-z_]\w*)\.[A-Za-z_]\w*\s*=\s*(?!=)')
_SUPPRESS_LINT_SET_INNER = _NEVER


def inv_lint_set_inner(path: Path, lines: List[str], root: Path) -> None:
    """Flag a run of >=3 consecutive field assignments to the same account base and suggest
    `Account::set_inner`. Populating an (`init`ed) account field-by-field silently leaves any newly
    added field at its default when the struct grows; `set_inner(T { .. })` makes that a compile
    error. The >=3 threshold targets full-struct population while ignoring 1-2 field partial
    updates. Accepted occurrences live in the JSON baseline, not in source comments."""
    n = len(lines)
    i = 0
    while i < n:
        m = _FIELD_ASSIGN.match(lines[i])
        if not m or is_comment(lines[i]):
            i += 1
            continue
        base = m.group(1)
        j = i + 1
        while j < n:
            mj = _FIELD_ASSIGN.match(lines[j])
            if mj and not is_comment(lines[j]) and mj.group(1) == base:
                j += 1
            else:
                break
        run = j - i
        if run >= 3:
            nearby = "\n".join(lines[max(0, i - 2):j])
            if not _SUPPRESS_LINT_SET_INNER.search(nearby):
                report(rel(path, root), i + 1, "INV-LINT-SET-INNER",
                       f"{run} consecutive field assignments to `{base}` — prefer "
                       "`.set_inner(T {{ .. }})` so adding a field to the account struct is a "
                       "compile error, not a silently-defaulted field; "
                       "accept via `make anchor-lint-baseline`",
                       "info")
        i = j if j > i else i + 1


PER_FILE_CHECKS = [inv_lint_serial, inv_lint_assert, inv_lint_event,
                   inv_lint_signer, inv_lint_init, inv_lint_params, inv_lint_pda,
                   inv_lint_pda_seeds, inv_lint_clone, inv_lint_arith, inv_lint_heap,
                   inv_lint_cast, inv_lint_unwrap, inv_lint_realloc,
                   inv_lint_bump, inv_lint_acctinit, inv_lint_atainit,
                   inv_lint_remacct, inv_lint_createacct, inv_lint_set_inner]

CHECK_ID_MAP = {
    "INV-LINT-SERIAL": inv_lint_serial, "INV-LINT-ASSERT": inv_lint_assert, "INV-LINT-EVENT": inv_lint_event,
    "INV-LINT-SIGNER": inv_lint_signer, "INV-LINT-INIT": inv_lint_init, "INV-LINT-PARAMS": inv_lint_params,
    "INV-LINT-PDA": inv_lint_pda,
    "INV-LINT-PDA-SEEDS": inv_lint_pda_seeds, "INV-LINT-CLONE": inv_lint_clone, "INV-LINT-ARITH": inv_lint_arith,
    "INV-LINT-HEAP": inv_lint_heap, "INV-LINT-CAST": inv_lint_cast, "INV-LINT-UNWRAP": inv_lint_unwrap,
    "INV-LINT-REALLOC": inv_lint_realloc, "INV-LINT-BUMP": inv_lint_bump,
    "INV-LINT-ACCTINIT": inv_lint_acctinit, "INV-LINT-ATAINIT": inv_lint_atainit,
    "INV-LINT-REMACCT": inv_lint_remacct, "INV-LINT-CREATEACCT": inv_lint_createacct,
    "INV-LINT-SET-INNER": inv_lint_set_inner,
}


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--root", default=None, help="Contracts root (auto-detected if omitted)")
    parser.add_argument("--strict", action="store_true", help="Exit 1 on any finding (not just errors)")
    parser.add_argument("--only", nargs="+", metavar="CXX", help="Run only these check IDs")
    parser.add_argument("--list-checks", action="store_true", help="Print available checks and exit")
    parser.add_argument(
        "--diff",
        nargs="?",
        const="auto",
        metavar="BASE_REF",
        default=None,
        help=(
            "Report only findings on changed lines. With no BASE_REF, auto-detect "
            "the current PR base branch from GitHub Actions or `gh pr view`. "
            "An explicit ref such as `--diff origin/main` is also supported. "
            "Analysis still covers the whole tree so cross-file checks stay correct."
        ),
    )
    parser.add_argument("--baseline", metavar="PATH", default=str(DEFAULT_BASELINE),
                        help="JSON file of accepted findings (default: %(default)s)")
    parser.add_argument("--no-baseline", action="store_true",
                        help="Ignore the baseline and report every finding")
    parser.add_argument("--write-baseline", action="store_true",
                        help="Rewrite the baseline from the current findings, then exit 0")
    args = parser.parse_args()

    if args.list_checks:
        print(bold("Available checks:"))
        for cid, (sev, desc) in CHECK_DESCRIPTIONS.items():
            sev_fn = SEV_COLOR.get(sev, yellow)
            print(f"  {bold(cid)}  [{sev_fn(sev):<7}]  {desc}")
        return

    # Locate contracts root
    cwd = Path.cwd()
    if args.root:
        root = Path(args.root).resolve()
    elif (cwd / "contracts" / "programs").is_dir():
        root = cwd / "contracts"
    elif (cwd / "programs").is_dir():
        root = cwd
    else:
        print(f"Error: cannot find contracts/programs from {cwd}", file=sys.stderr)
        sys.exit(2)

    programs_root = root / "programs"
    if not programs_root.is_dir():
        print(f"Error: {programs_root} does not exist", file=sys.stderr)
        sys.exit(2)

    all_rs = rs_files(programs_root)
    crates_root = root / "crates"
    if crates_root.is_dir():
        all_rs += rs_files(crates_root)

    enabled: Optional[Set[str]] = set(args.only) if args.only else None

    def run(check_id: str) -> bool:
        return enabled is None or check_id in enabled

    print(bold(f"\nAnchor lint  —  {len(all_rs)} files under {root.relative_to(cwd) if root.is_relative_to(cwd) else root}"))
    print()

    # Per-file checks
    for path in all_rs:
        lns = lines_of(path)
        rp = rel(path, root)
        _test_lines_by_file[rp] = _test_line_set(lns)
        for fn in PER_FILE_CHECKS:
            cid = next(k for k, v in CHECK_ID_MAP.items() if v is fn)
            if run(cid):
                fn(path, lns, root)

    # Cross-file checks
    if run("INV-LINT-EVENT"):
        inv_lint_event_b(all_rs, root)
        inv_lint_event_c(all_rs, root)
    if run("INV-LINT-INSTR"):
        inv_lint_instr(all_rs, root)

    # ── Apply the suppression baseline ────────────────────────────────────────
    # Runs before diff scoping: the baseline records what the repo accepts, and
    # diff scoping then narrows what's left to the lines under review.
    baseline_path = Path(args.baseline)
    _, baseline_reasons = load_baseline(baseline_path)

    if args.write_baseline:
        n = write_baseline(baseline_path, _findings, baseline_reasons)
        print(bold(f"  Wrote {n} baseline entr{'y' if n == 1 else 'ies'} "
                   f"covering {len(_findings)} finding(s) to {baseline_path}"))
        print()
        return

    if not args.no_baseline:
        accepted, _ = load_baseline(baseline_path)
        if accepted:
            before = len(_findings)
            kept, stale = apply_baseline(_findings, accepted)
            _findings[:] = kept
            print(bold(f"  Baseline: {before - len(kept)} accepted finding(s) suppressed "
                       f"via {baseline_path.name}"))
            if stale:
                # A stale entry means the debt was paid but the baseline still
                # allows it, which would hide a genuine regression later.
                print(yellow(f"  {len(stale)} stale baseline entr"
                             f"{'y' if len(stale) == 1 else 'ies'} — rerun "
                             f"`make anchor-lint-baseline` to prune:"))
                for s in stale[:10]:
                    print(yellow(f"    {s}"))
                if len(stale) > 10:
                    print(yellow(f"    … and {len(stale) - 10} more"))
            print()

    # ── Scope findings to the diff ────────────────────────────────────────────
    pre_existing = 0
    if args.diff:
        base_ref = args.diff
        if base_ref == "auto":
            base_ref = detect_pr_base(root)
            if not base_ref:
                print(red(bold(
                    "  Error: cannot determine PR base branch. In GitHub Actions, "
                    "GITHUB_BASE_REF must be available; locally, authenticate `gh` "
                    "or pass an explicit base (e.g. `--diff origin/release/1.2`)."
                )), file=sys.stderr)
                sys.exit(2)

        diff_result = changed_lines(base_ref, root)
        if diff_result is None:
            print(red(bold(
                f"  Error: cannot diff against '{base_ref}' — unknown ref, missing history "
                f"(a shallow clone needs `fetch-depth: 0`), or not a git repo.")), file=sys.stderr)
            sys.exit(2)

        changed, deletion_files = diff_result
        total = len(_findings)
        kept = [
            f for f in _findings
            if f.file in deletion_files or f.line in changed.get(f.file, ())
        ]
        pre_existing = total - len(kept)
        _findings[:] = kept
        touched = len(set(changed) | deletion_files)
        print(bold(f"  Diff mode: {touched} changed file(s) vs {base_ref}; "
                   f"{pre_existing} pre-existing finding(s) outside the diff not reported."))
        print()

    # ── Print results ─────────────────────────────────────────────────────────
    if not _findings:
        print(green(bold("  ✓ No issues found.")))
        print()
        return

    errors   = [f for f in _findings if f.severity == "error"]
    warnings = [f for f in _findings if f.severity == "warning"]
    infos    = [f for f in _findings if f.severity == "info"]

    by_file: Dict[str, List[Finding]] = defaultdict(list)
    for f in _findings:
        by_file[f.file].append(f)

    for fp in sorted(by_file):
        print(bold(f"  {fp}"))
        for finding in sorted(by_file[fp], key=lambda f: f.line):
            print(str(finding))
        print()

    # Summary line
    parts = []
    if errors:
        parts.append(red(f"{len(errors)} error{'s' if len(errors) != 1 else ''}"))
    if warnings:
        parts.append(yellow(f"{len(warnings)} warning{'s' if len(warnings) != 1 else ''}"))
    if infos:
        parts.append(blue(f"{len(infos)} info"))
    print(bold("  Summary: ") + ", ".join(parts))
    print()

    if args.strict and _findings:
        sys.exit(1)
    if errors:
        sys.exit(1)


if __name__ == "__main__":
    main()
