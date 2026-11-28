# LocalCI on GitLab CI

LocalCI is a plain Go binary — nothing about `capture` and `diff`
is GitHub-specific. The GitHub Action (`action.yml`) is GitHub-only,
but the same check runs on GitLab with a few lines of YAML.
See [examples/gitlab-ci.yml](../examples/gitlab-ci.yml).

## Setup

1. On your laptop, capture a baseline and commit it:

   ```sh
   localci capture -o .localci/local.json
   git add .localci/local.json && git commit -m "Add LocalCI baseline"
   ```

2. Copy `examples/gitlab-ci.yml` to `.gitlab-ci.yml` in your repo.
   The job installs LocalCI, captures in CI, and diffs against the
   baseline. It fails the pipeline when differences are found.

3. Workflow-aware filtering works the same way: pass
   `--workflows .github/workflows` if your repo has GitHub workflow
   files, or point it at any directory of workflow YAML. If you
   don't have workflow files, leave the flag off — every difference
   is shown.

## Differences from the GitHub Action

| Feature | GitHub Action | GitLab (this file) |
|---|---|---|
| Capture + diff | yes | yes |
| Workflow-aware filtering | yes (`workflows-dir`) | yes (`--workflows`) |
| Matrix baselines | yes (`baseline-dir`) | yes (`--baseline-dir`) |
| PR comment with the diff | yes | yes — MR note via the GitLab API (`scripts/post-gitlab-mr.py`, `CI_JOB_TOKEN`); best-effort, diff stays in the job log if posting fails |
| Fingerprints as artifacts | yes | yes (`ci.json`, `localci-diff.txt`, `localci-diff.md`) |
| Fail the check on diff | yes (`fail-on-diff`) | yes — the job exits 1 |

The MR comment is presentation, not detection. The diff itself, including
severity and fix suggestions, is identical — it's the same binary.

## Matrix builds

GitLab `parallel:matrix` works with `--baseline-dir` the same way
the Action does: capture one baseline per cell
(`.localci/ubuntu-node20.json`, …) and the diff picks the baseline
matching each job's environment.

## `localci run` on GitLab

`localci run` is a local tool (it needs Docker on your machine).
It doesn't run in GitLab CI — reproduce the failing step on your
laptop with `localci run --workflow … --job … --step …` instead.
See [RUN.md](RUN.md).
