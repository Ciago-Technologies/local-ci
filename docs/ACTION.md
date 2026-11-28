# LocalCI GitHub Action

Find out in CI why it differs from local — before you merge.

## How it works

1. You commit a baseline fingerprint from your laptop: `.localci/local.json`
2. On every PR, the action captures a fresh fingerprint in CI
3. It diffs the two, comments the diff on the PR, uploads both fingerprints as artifacts, and fails the check when they differ

It never sees env var values — `localci` only records env var **names** (unless you opt in with `--include-values` for specific safe vars at capture time).

The diff is **workflow-aware** (Phase 3): it reads `.github/workflows`
and only reports mismatches in runtimes / env vars the jobs actually
use. Each difference has a severity (`critical` / `warning` / `info`)
and a suggested fix.

## Setup (2 minutes)

On your laptop, in your repo:

```bash
localci capture -o .localci/local.json
git add .localci/local.json
git commit -m "Add LocalCI baseline"
```

Copy `examples/pr-check.yml` to `.github/workflows/localci.yml` in your repo, push, and open a PR. That's it.

## Inputs

| Input | Default | Description |
| --- | --- | --- |
| `local-fingerprint` | `.localci/local.json` | Path to the baseline fingerprint |
| `fail-on-diff` | `'true'` | Fail the check when differences are found |
| `comment-on-pr` | `'true'` | Comment the diff on the pull request |
| `workflows-dir` | `.github/workflows` | Workflow directory for workflow-aware diffing; set to `''` to show all differences |
| `baseline-dir` | `''` | Directory of baselines (`.localci`); when set, picks the baseline matching this runner (matrix support) instead of `local-fingerprint` |

## Outputs

| Output | Description |
| --- | --- |
| `has-differences` | `'true'` when CI differs from the baseline |
| `diff` | Human-readable diff output (empty when they match) |

## What the check does

- **Baseline missing:** the check passes with a warning and tells you how to create the baseline. It does not fail your first PR.
- **Differences found:** the diff is in the job log, as a PR comment, and in the `localci-fingerprints` artifact (`ci.json` + `localci-diff.md`). The check fails unless `fail-on-diff` is `'false'`.
- **No differences:** the check passes silently.
- **CPU / memory:** reported as `[info]` in the diff, and hidden by workflow-aware diffing. A laptop and a CI runner rarely have the same hardware, so treat those lines as context, not as a failure cause.
- **Severity:** `[critical]` (missing env/runtime, major version change, lockfile drift) is the most likely cause of a CI failure; `[warning]` (minor version drift, timezone, locale) may be; `[info]` usually isn't. Every difference also carries a `Fix:` suggestion.
- **Lockfile detail:** when a lockfile changes, the diff names the packages — "react 18.2.0 → 18.3.0, axios added, moment removed" — instead of just showing two hashes. Supported: package-lock, yarn, pnpm, Cargo, poetry, Gemfile, composer, go.sum, Pipfile.
- **Services & images:** the diff also covers workflow service images (`postgres:16`), the Dockerfile base image, and the OS release (`ubuntu 24.04`).

## Updating the baseline

When your environment intentionally changes — new Node version, new env var, new lockfile — re-capture and commit:

```bash
localci capture -o .localci/local.json
git add .localci/local.json
git commit -m "Update LocalCI baseline"
```

If you don't, the next PR will (correctly) flag the change.

**Staleness:** `diff` warns when the baseline is older than 30 days.
A stale baseline is a warning, not a failure — but re-capture it,
because GitHub updates runner images regularly and old baselines
go stale even when nothing in the repo changed. A runner image
change shows up as a `runner_image` warning in the diff; pin
`runs-on` to a specific image (e.g. `ubuntu-22.04`) if you want
the image frozen.

## Matrix builds

One baseline can't cover Ubuntu + macOS, Node 20 + 22. Capture one
baseline per matrix cell and commit them all under `.localci/`:

```bash
localci capture -o .localci/ubuntu-node20.json
localci capture -o .localci/ubuntu-node22.json
localci capture -o .localci/macos-node20.json
git add .localci && git commit -m "Add LocalCI baselines"
```

Then use `baseline-dir` (see `examples/pr-check-matrix.yml`).
The action picks the baseline matching each runner — same OS and
arch, same runtime major versions. Matching is by fingerprint
content, not by file name, so any naming convention works. A cell
with no matching baseline warns and passes, like a missing baseline.

## FAQ

**Do I need a release of localci first?**
No. The action builds `localci` from source with Go 1.23, so it works from `main` / a tag / a branch.

**Which version should I pin?**
`Ciago-Technologies/local-ci@v0.7.4` — the current Marketplace release.
`@v1` comes at launch, once the API is stable.

**Does it work on forks?**
The diff and artifacts work on forks. The PR comment can fail on forks (GitHub token permissions) — the action logs a warning and continues; the diff is still in the job log.

**Why doesn't the check fail on the git commit?**
A baseline is an environment snapshot, not a commit pointer: its
commit always differs from the PR head. The action runs the diff
with `--ignore git_commit`, so a PR fails only on environment
differences. Standalone `localci diff` still reports the commit
unless you pass `--ignore git_commit` yourself.

**Dogfood findings (v0.7.0 → v0.7.1):** the repo's own PR #2
exposed three gaps, all fixed in v0.7.1: the comment step called
`gh` without `GH_TOKEN` set, so no comment was ever posted (the
fallback swallowed the error); the diff was never printed to the
job log, only to the artifact; and `git_commit` was compared, which
would have failed every PR on the commit alone.

**What permissions does the workflow need?**
`contents: read` and `pull-requests: write` (for the comment). See `examples/pr-check.yml`. This is not optional: without
`pull-requests: write` on the calling job, the default token is
read-only and the comment step fails with `GraphQL: Resource not
accessible by integration (addComment)` — the action logs a
`::warning::` and continues, but no comment is posted. The repo's
own dogfood workflow shipped without this block and two test runs
posted nothing (fixed in v0.7.2).

**Can I use it without failing PRs?**
Yes — set `fail-on-diff: 'false'`. You'll still get the diff as a comment and artifact.

## Testing the action

We can't run GitHub Actions locally without `act`. Instead, `scripts/simulate-action.sh` replays the action's steps (build, capture, diff, artifacts) locally for three scenarios: matching fingerprints, mismatched fingerprints, and a missing baseline. Run it with `./scripts/simulate-action.sh`.
