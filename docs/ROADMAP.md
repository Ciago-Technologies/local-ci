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
- [ ] Publish to the GitHub Marketplace as release v0.4.0 (repo is
      public; use the "Draft a release" banner, tag v0.4.0)

Exit criteria: a matrix build (Ubuntu + macOS, Node 20 + 22) compares
each cell against its own baseline, and a stale baseline or a new
runner image is flagged instead of silently passing.

## Phase 5 — Fingerprint coverage

- Runner image + Docker image version, system packages
- Rust / Java / Ruby / .NET versions
- Service versions (Postgres, Redis, ...) from the workflow's
  `services:` block
- Lockfile detail: which package changed, not just the hash
- Line endings, file permissions, symlinks

## Phase 6 — Run the failing step locally

- Optional: run a single command in a container matching the CI image, using the fingerprint to pick the image
- Explicitly out of scope for Phases 1–5 — do not build a full `act` clone

## Phase 7 — Launch

- Dogfood the Action on real repos; GitLab support
- Demo video / GIF: 30-second aha moment
- Docs site / README polish, good-first-issues, submit to awesome lists
- Measure: stars, installs, issues from real repos — not just our own

## Non-goals

- Running full GitHub Actions workflows locally
- Collecting env var values or secrets by default — values only with
  an explicit `--include-values` opt-in for named vars; never secrets
- A server, daemon, or hosted service
