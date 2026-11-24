# LocalCI GitHub Action

Find out in CI why it differs from local — before you merge.

## How it works

1. You commit a baseline fingerprint from your laptop: `.localci/local.json`
2. On every PR, the action captures a fresh fingerprint in CI
3. It diffs the two, comments the diff on the PR, uploads both fingerprints as artifacts, and fails the check when they differ

It never sees env var values — `localci` only records env var **names**.

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

## Outputs

| Output | Description |
| --- | --- |
| `has-differences` | `'true'` when CI differs from the baseline |
| `diff` | Human-readable diff output (empty when they match) |

## What the check does

- **Baseline missing:** the check passes with a warning and tells you how to create the baseline. It does not fail your first PR.
- **Differences found:** the diff is in the job log, as a PR comment, and in the `localci-fingerprints` artifact (`ci.json` + `localci-diff.md`). The check fails unless `fail-on-diff` is `'false'`.
- **No differences:** the check passes silently.
- **CPU / memory:** reported as `[info]` in the diff. A laptop and a CI runner rarely have the same hardware, so treat those lines as context, not as a failure cause.

## Updating the baseline

When your environment intentionally changes — new Node version, new env var, new lockfile — re-capture and commit:

```bash
localci capture -o .localci/local.json
git add .localci/local.json
git commit -m "Update LocalCI baseline"
```

If you don't, the next PR will (correctly) flag the change.

## FAQ

**Do I need a release of localci first?**
No. The action builds `localci` from source with Go 1.23, so it works from `main` / a tag / a branch.

**Which version should I pin?**
`Ciago-Technologies/local-ci@v1` once v1 is tagged. Until then, pin a commit SHA or a branch for stability.

**Does it work on forks?**
The diff and artifacts work on forks. The PR comment can fail on forks (GitHub token permissions) — the action logs a warning and continues; the diff is still in the job log.

**What permissions does the workflow need?**
`contents: read` and `pull-requests: write` (for the comment). See `examples/pr-check.yml`.

**Can I use it without failing PRs?**
Yes — set `fail-on-diff: 'false'`. You'll still get the diff as a comment and artifact.

## Testing the action

We can't run GitHub Actions locally without `act`. Instead, `scripts/simulate-action.sh` replays the action's steps (build, capture, diff, artifacts) locally for three scenarios: matching fingerprints, mismatched fingerprints, and a missing baseline. Run it with `./scripts/simulate-action.sh`.
