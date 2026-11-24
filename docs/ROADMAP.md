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

## Phase 3 — Fix suggestions

- A small database of common culprits: runtime version mismatch, timezone UTC vs local, missing env name, lockfile drift, case-sensitive filename
- Each diff line gets a suggested fix, not just a difference

## Phase 4 — Run the failing step locally

- Optional: run a single command in a container matching the CI image, using the fingerprint to pick the image
- Explicitly out of scope for Phases 1–3 — do not build a full `act` clone

## Phase 5 — Launch

- Demo video / GIF: 30-second aha moment
- Docs site / README polish, good-first-issues, submit to awesome lists
- Measure: stars, installs, issues from real repos — not just our own

## Non-goals

- Running full GitHub Actions workflows locally
- Collecting env var values, secrets, or credentials — ever
- A server, daemon, or hosted service
