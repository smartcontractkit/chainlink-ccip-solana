# CI Scripts and Automation

This directory contains scripts and configuration for managing the `chainlink-ccv` dependency and automating its updates.

## Core Components

### 1. `ccv-ref.yaml`

```yaml
pinned: <commit-sha>
tracking: main
module: github.com/smartcontractkit/chainlink-ccv
submodules:
  - build/devenv
```

The source of truth for the pinned `chainlink-ccv` dependency. It stores:

- `module`: The root Go module name.
- `tracking`: The upstream branch to watch for updates (e.g., `main`).
- `pinned`: The current Git commit SHA integrated into this repository.
- `submodules`: A list of internal packages within the module that must be pinned in sync.

### 2. `ccv-ref.sh`

A utility for reading and updating the CCV reference across the entire project.

- **`read <key>...`**: Emits metadata (tracking, pinned, module) to `GITHUB_OUTPUT` or stdout.
- **`pin <sha>`**: Updates `ccv-ref.yaml` and pins the root module and all submodules in every `go.mod` file found in the repo. Pinning resolves each module's pseudo-version with `go list -m` and sets it via `go mod edit -require` (never `go get`), deferring `go mod tidy` until after every ccv module is pinned — `go get`'s eager import loading deadlocks against a half-updated module graph when an upstream dependency moved packages between the old and new pins.
- **`validate`**: Ensures all `go.mod` files are synchronized with the SHA in `ccv-ref.yaml`.

### 3. `ci-helpers.sh`

A collection of "workflow-shaped" helpers used primarily by GitHub Actions.

- **`check-upstream`**: Resolves the latest SHA of the tracking branch on the remote repository.
- **`changelogs`**: Summarizes new changelog entries between the pinned and latest SHAs.
- **`manual-commit-guard`**: Protects the auto-bump branch from being force-pushed if a human has added commits to an open PR.

## Automation: Auto-bump pinned ccv

The `.github/workflows/auto-bump-ccv.yml` workflow runs daily to keep the project up to date with `chainlink-ccv`.

### Workflow Diagram

 ```mermaid
   graph TD
      Start([Daily Schedule]) --> Check[Check Upstream]
      Check --> Match{SHA Matches<br/>ccv-ref.yaml?}
 
      Match -- Yes --> Done
      Match -- No --> Guard[Manual Commit Check]
 
      Guard --> PRState{Open PR with<br/>Human Commits?}
 
      PRState -- Yes --> Skip[Comment, Skip Force-push]
      Skip --> Done
 
      PRState -- No --> Pin[Run pin-ccv-all]
      Pin --> GoMod[Update all go.mod files]
      GoMod --> Changelog[Fetch Upstream Changelogs]
      Changelog --> Push[Force-push, Upsert PR]
      Push --> Done
 ```

### How it works

1. **Check**: It compares the `pinned` SHA in `ccv-ref.yaml` with the `tracking` branch on the upstream repo.
2. **Guard**: If an update is needed, it checks for an existing open PR on `auto/bump-ccv-pinned`.
   - If a human has added commits to that PR, the workflow **skips** the update to avoid overwriting manual work.
   - If no human commits are found (or no PR exists), it proceeds.
3. **Bump**: It runs `make pin-ccv-all REF=<latest_sha>`, which updates all `go.mod` files and regenerates any dependent code.
4. **PR**: It force-pushes the changes to `auto/bump-ccv-pinned` and creates/updates a draft PR with a summary of the upstream changelog entries.

## Common Tasks

### Manually pinning to a specific SHA

```bash
make pin-ccv-all REF=<commit-sha>
```

### Validating synchronization

```bash
make validate-ccv
```
