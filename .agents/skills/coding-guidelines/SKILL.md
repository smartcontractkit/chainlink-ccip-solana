---
name: coding-guidelines
description: Keep code comments concise and function signatures simple when writing, editing, or reviewing code. Use for a focused pass that removes redundant, verbose, or historical comments and multi-value returns from a diff.
---

# Comment discipline

When writing or changing code:

- Prefer clear names and structure to explanatory comments. Add a comment only if a reader would miss an important fact from the code and nearby types.
- Explain current invariants, security boundaries, protocol requirements, or surprising behavior. State the reason directly and briefly.
- Never describe the development journey: previous implementations, bugs that led to the change, review discussion, or claims about what the code "now" does. Put that context in the PR description.
- Do not link Jira tickets or internal issue trackers in code comments.
- Avoid comments that paraphrase the next line, justify a naming choice with counts or anecdotes, or use a paragraph for a local detail.
- Preserve required license headers, public API documentation, and useful existing comments. Do not remove a comment that explains a real constraint merely to shorten the file.

Before finishing, scan added and edited comments in the diff. Delete or shorten any comment that fails these rules; if the code is clear without it, delete it.

# Function signatures

- Do not return many values separated by commas. Beyond a value plus `error` or an `ok` bool, return a named struct so call sites read by field and never by position (avoid shapes like `a, _, _, b, ok := f()`).
- Prefer pointer fields (`nil` = absent) over a companion bool, such as `add *Ref` instead of `ref Ref, add bool`.
- When a function has many parameters of the same type, group them into an input struct.
