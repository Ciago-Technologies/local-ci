# LocalCI Roadmap

Vision: **Find out locally why CI will fail — before you push.**

## Phase 0 — Repo setup (current)

- [x] Repo in `Ciago-Technologies/local-ci`, public, dual Apache-2.0 / MIT
- [x] README with vision and scope
- [x] Go module, CLI skeleton (`--help`, `--version`, stub `capture` / `diff`)
- [x] `internal/fingerprint` types — env names only, never values
- [x] CI workflow: build + vet + test on push

Exit criteria: repo is live, `go build ./...` passes in CI, scope is locked.

## Phase 1 — MVP `capture` + `diff` (done 2026-11-24)

- [x] `localci capture` writes a JSON fingerprint to stdout / file (`-o`, `--dir`)
- [x] Collect: Go / Node / Python versions, OS + arch, timezone, locale, CPU / memory, env names, lockfile hashes, git commit, case-sensitivity
- [x] `localci diff a.json b.json` prints a human-readable diff, exit 1 on differences, 0 on match
- [x] `--json` output for scripting; hardware differences marked `severity: info`
- [x] Tests: 6 passing, incl. fixtures for a known mismatch (Node 20.11 vs 20.9, Asia/Kolkata vs UTC, missing STRIPE_KEY, lockfile drift) and a no-secrets test
- [ ] Tested on macOS (Linux verified; macOS still to do)

Exit criteria: a real "passes locally, fails in CI" case is diagnosed in under a minute with a copy-pasteable diff.

## Phase 2 — GitHub Action (done 2026-11-24)

- [x] `Ciago-Technologies/local-ci` action (`action.yml`, composite): build from source, `capture` in CI, upload fingerprints as artifacts
- [x] Compare in PR: post the diff as a comment, fail the check with the diff (`fail-on-diff`, `comment-on-pr` inputs)
- [x] Baseline is a committed `.localci/local.json`; missing baseline warns and passes instead of failing the first PR
- [x] Docs: one workflow snippet a user can copy in 2 minutes (`examples/pr-check.yml`, `docs/ACTION.md`)
- [x] Tested locally with `scripts/simulate-action.sh`: match / mismatch / missing-baseline scenarios

Exit criteria: a user can copy one workflow file and get a LocalCI check on their next PR.

## Phase 3 — Workflow-aware diff + fixes (done 2026-11-24)

Revised after a gap review (2026-11-24): Phase 1–2 showed a diff, but
didn't say why CI would fail, and flagged harmless mismatches (timezone,
CPU) the same way as a missing secret. Phase 3 fixes that:

- [x] `internal/workflow`: parse `.github/workflows/*.yml` — runtimes
      (`setup-node` / `setup-go` / `setup-python` / ...), `runs-on`,
      `strategy.matrix`, `services`, and env var names from job/step env
- [x] `localci diff --workflows DIR`: only show mismatches in things a
      job actually uses; hide the rest (`--show-unused` to see them).
      Fail-open: unparsable workflows or no workflows = show everything.
- [x] Severity ranking: `critical` (missing env/runtime, major version
      change, lockfile drift), `warning` (minor version drift, timezone,
      locale, OS), `info` (CPU/memory)
- [x] Each diff line gets a suggested fix (`Fix` field in JSON, `Fix:`
      line in human output)
- [x] `localci capture --include-values NODE_ENV,TZ`: opt-in values for
      explicitly named, safe vars only — a wrong value is a critical diff
- [x] Release binary: `.goreleaser.yml` + `examples/release.yml` (copy
      to `.github/workflows/release.yml` manually; tag `v*` to publish)
- [x] Action: new `workflows-dir` input (default `.github/workflows`),
      passed to `diff`; docs updated
- [x] Tests: 10+ passing (workflow parser, workflow filtering,
      severity, fixes, env-value opt-in); demo: 7 fixture diffs → 3
      shown, 4 hidden with `--workflows`

Exit criteria: a diff now reads as "this job will fail because X",
with a fix — not "here are 7 differences, good luck".

## Phase 4 — Matrix + baseline freshness (done 2026-11-24)

- [x] Multiple baselines: `localci diff --baseline-dir .localci ci.json`
      picks the baseline matching the CI fingerprint (OS, arch,
      runtime major versions) — one baseline per matrix cell
      (`.localci/ubuntu-node20.json`, `.localci/macos-node22.json`)
- [x] Runner image fingerprinted: `runner_image` from the
      `ImageOS` / `ImageVersion` env vars on GitHub-hosted runners;
      a change is a warning diff with a fix (pin `runs-on`)
- [x] Baseline staleness: `diff` warns when the baseline is older
      than 30 days (`BaselineMaxAgeDays`); a stale baseline is a
      warning, not a difference
- [x] Action: new `baseline-dir` input; matrix example in
      `examples/pr-check-matrix.yml`; `branding` added for the
      GitHub Marketplace listing
- [x] Tests: 17 passing (baseline pick, no-match error, staleness,
      runner image); demo: correct cell picked, 40-day-old baseline
      warns, runner image change shows as a warning diff
- [x] Published to the GitHub Marketplace as release v0.4.1. (v0.4.0
      was published as a plain release first; Marketplace publishing
      failed because the action name "LocalCI" was not unique, so the
      action was renamed to "Ciago LocalCI" and released as v0.4.1.)

Exit criteria: a matrix build (Ubuntu + macOS, Node 20 + 22) compares
each cell against its own baseline, and a stale baseline or a new
runner image is flagged instead of silently passing.

## Phase 5 — Fingerprint coverage (done 2026-11-24)

- [x] Runtimes: Rust (`rustc`), Java, Ruby, .NET added to `capture`
      (Go / Node / Python were already there); the workflow parser
      already knew their `setup-*` actions
- [x] OS release: `os_release` from `/etc/os-release` (`ubuntu 24.04`)
      or `sw_vers` on macOS — distinguishes 22.04 from 24.04
- [x] Services: `services` map from the workflow's `services:` block
      (name -> image, e.g. postgres -> `postgres:16`); Dockerfile
      base image from the first `FROM` line
- [x] Lockfile detail: `lockfile_packages` parsed for 9 formats
      (package-lock.json incl. legacy layout, yarn.lock,
      pnpm-lock.yaml, Cargo.lock, poetry.lock, Gemfile.lock,
      composer.lock, go.sum, Pipfile.lock); the diff now names the
      packages — "react 18.2.0 → 18.3.0, axios added, moment removed"
- [x] Repo facts via `git ls-files`: line endings (lf / crlf /
      mixed, sampled from tracked text files), executable file count,
      symlink count
- [x] Diff + fixes + workflow-aware filtering for all new fields;
      full system-package inventory deliberately NOT done (too big,
      too noisy — OS release + runner image cover the common cases)
- [x] Tests: 34 passing (lockfile parsers for all 9 formats,
      runtimes via stub binaries, services, git facts, lockfile
      detail diff); demo: lockfile detail names the 3 changed
      packages, git facts capture mixed endings + exec + symlink

Exit criteria: a lockfile change now reads as "react went
18.2.0 → 18.3.0", not "hash changed" — and a repo that only
works on one OS / runtime is fingerprinted, not guessed.

## Phase 6 — Run the failing step locally (done, v0.6.0-phase6)

- `localci run -- <command>` runs one command in a Docker container
  matching the CI image; repo mounted at `/work`, exit code passed through
- `localci run --workflow FILE --job JOB --step NAME` runs one workflow
  step's `run:` script in the container matching that job's runs-on
- Image selection: `--image` flag, CI fingerprint runner image
  (ubuntu24 → ubuntu:24.04), Dockerfile FROM, runs-on label, default ubuntu:24.04
- `uses:` steps refused; `${{ ... }}` env values skipped with a warning;
  job services warned about, not started; `--env NAME[=VALUE]` opt-in only,
  values never printed (masked in --dry-run)
- Docker required at run time; `--dry-run` shows the exact docker command
- Deliberately not a full `act` clone: one command, one container, no services

Exit criteria: a failing `npm test` step can be reproduced locally in
the matching container with one command — and a machine without Docker
gets a clear error, not a silent failure.

## Phase 7 — Launch (done, v0.7.0)

- Dogfood the Action on real repos: `examples/dogfood.yml` — copy to
  `.github/workflows/localci.yml` via the web UI to run LocalCI on
  itself. (Workflow files can't be pushed by the GitHub App.)
- GitLab support: `examples/gitlab-ci.yml` + [GITLAB.md](GITLAB.md) —
  capture + diff run as plain CI YAML; no MR comment yet (see issue
  draft in GOOD_FIRST_ISSUES.md).
- Demo: `scripts/demo.sh` (the 30-second aha moment, runnable, no
  Docker needed) + animated `docs/demo.gif` (generated by
  `scripts/make-demo-gif.py`) embedded in the README.
- README polish: CI + Marketplace + license badges, `go install`
  one-liner, demo GIF, links to RUN.md / GITLAB.md.
- Good-first-issues: [GOOD_FIRST_ISSUES.md](GOOD_FIRST_ISSUES.md) —
  4 ready-to-paste drafts (macOS verify, Bun runtime, `run --service`,
  SARIF output). The GitHub App can't create issues, so a maintainer
  files them from these drafts.
- Repo's own CI verified green: latest run (Phase 6, 11a8bb44)
  completed with conclusion `success` via the GitHub API.
- Still open: submitting to awesome lists.

## v0.7.1 — Dogfood fixes (done)

Dogfooding the Action on its own repo (PR #1, PR #2) exposed three
real defects, all fixed here:

- `gh pr comment` ran without `GH_TOKEN`, so the PR comment was
  never posted (the `|| echo` fallback swallowed the error). The
  Action now sets `GH_TOKEN: ${{ github.token }}` on the comment step.
- The diff was never printed to the job log — only to the artifact —
  while the fail message said "see the diff above". The diff step
  now cats `localci-diff.txt`.
- `git_commit` was compared, which would fail every PR on the
  commit alone (a baseline's commit always differs from the PR
  head). New `localci diff --ignore` flag; the Action passes
  `--ignore git_commit`. Tests: 44 passing, simulation 8/8.

## v0.7.2 — Dogfood fix, part 2 (done)

v0.7.1 fixed the token but the comment still didn't post: `gh pr
comment` failed with `GraphQL: Resource not accessible by
integration (addComment)`. Root cause was in our own repo, not the
Action: the dogfood workflow (`.github/workflows/localci.yml`,
copied from `examples/dogfood.yml`) declared no `permissions:`,
while the other examples did — so its token was read-only for pull
requests. Fixed by adding `permissions: contents: read,
pull-requests: write` to `examples/dogfood.yml` and documenting
that the calling workflow must declare it. Also made the comment
step's fallback emit a `::warning::` annotation instead of a plain
echo, so a failed comment is visible in the run UI rather than
silently swallowed.

## v0.7.3 — Dogfood fix, part 3 (done)

The verified PR #2 comment showed a second, real diff: the runner
image had drifted two days after the baseline was captured
(`ubuntu26/20261116.88.1` vs `...202.1`). GitHub updates runner
images roughly weekly, so showing `runner_image` would fail every
PR on image drift alone. The workflow filter now hides it (like
cpu/memory); the `os` field and runtime versions still catch
meaningful platform change, the staleness warning covers baseline
age, and standalone `localci diff` still shows it. Tests: 45
passing, simulation 9/9.

Exit criteria: someone who has never seen LocalCI can install it,
run the demo, and understand the point in 30 seconds — and the
repo's own CI is green.

## Non-goals

- Running full GitHub Actions workflows locally
- Collecting env var values or secrets by default — values only with
  an explicit `--include-values` opt-in for named vars; never secrets
- A server, daemon, or hosted service
