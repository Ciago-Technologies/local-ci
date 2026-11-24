# LocalCI

**Find out locally why CI will fail — before you push.**

LocalCI is a local-first CI debugger from [Ciago Technologies](https://github.com/Ciago-Technologies).

Every developer has hit it: tests pass on your laptop, fail in CI. Or the opposite. You waste hours guessing — is it the Node version? The timezone? A missing env var? A file that only differs by case?

LocalCI answers that in seconds:

```bash
localci capture > local.json      # on your laptop
localci capture > ci.json         # in CI
localci diff local.json ci.json
```

```text
Node:     local 20.11.0, CI 20.9.0
Timezone: local Asia/Kolkata, CI UTC
Missing in CI: STRIPE_KEY
Lockfile: local a1b2c3, CI d4e5f6
```

No secrets. LocalCI only records env var **names**, never values.

> **Status: Phase 5 — fingerprint coverage. Published on the GitHub Marketplace as v0.4.1.**
> See [docs/ROADMAP.md](docs/ROADMAP.md) and [docs/ACTION.md](docs/ACTION.md).

## Why LocalCI, not another CI runner?

Tools like `act` run your whole pipeline locally. LocalCI does the opposite, smaller job — and does it fast:

- **Don't run all of CI locally. Find the difference.**
- One static Go binary, no daemon, no server.
- Works for Go, Node, Python, Rust — any repo with CI.
- Built for the "works on my machine" moment, not for re-implementing GitHub Actions.

## Install

Release binaries (linux / macOS / Windows, amd64 + arm64) are built
by GoReleaser on every `v*` tag — see [examples/release.yml](examples/release.yml).
Until the first tagged release, build from source (Go 1.23+):

```bash
git clone https://github.com/Ciago-Technologies/local-ci.git
cd local-ci
go build -o localci .
./localci --help
```

## Usage

```bash
localci --help
localci --version

# On your laptop, in your repo:
localci capture -o local.json
# In CI, same command:
localci capture -o ci.json

localci diff local.json ci.json
# Exit 0 when they match, 1 when differences are found.

localci diff --json local.json ci.json  # machine-readable output
localci capture --dir /path/to/repo    # fingerprint a different directory

# Workflow-aware: only warn about runtimes / env vars the jobs
# in .github/workflows actually use. Timezone, CPU, and other
# unused mismatches are hidden (use --show-unused to see them).
localci diff --workflows .github/workflows local.json ci.json

# Opt-in values for named, safe vars only (never by default):
localci capture --include-values NODE_ENV,TZ -o local.json
```

Example output (workflow-aware — 7 raw differences, 3 that matter):

```text
Found 3 difference(s) — local vs CI (2 critical, 1 warning, 0 info):
  [critical] env:STRIPE_KEY: local = set, CI = (missing)  (missing in CI)
      Fix: Add STRIPE_KEY in repo Settings > Secrets and variables > Actions, then re-run the workflow.
  [critical] lockfile:package-lock.json: local = a1b2c3d4e5f6, CI = d4e5f6a1b2c3  (lockfile changed — run the package manager install step again)
      Fix: Re-run the install step locally (npm ci / go mod tidy / pip install) and commit the updated lockfile.
  [warning] runtime:node: local = 20.11.0, CI = 20.9.0
      Fix: Pin node-version: 20.9.0 in the workflow, or update your local node to 20.9.0.
4 difference(s) hidden — not used by any workflow job (use --show-unused to see them).
```

Each difference has a severity: **critical** (missing env/runtime,
major version change, lockfile drift), **warning** (minor version
drift, timezone, locale), **info** (CPU/memory — a laptop and a CI
runner rarely have the same hardware).

## Run a failing step locally

```sh
# One command in the CI-matching container (repo mounted at /work):
localci run -- npm test

# One workflow step's run: script, in the container matching its job:
localci run --workflow .github/workflows/ci.yml --job test --step Test

# See the docker command without running it:
localci run --dry-run -- npm test
```

The image is picked from `--image`, the CI fingerprint's runner
image, the repo Dockerfile, the job's `runs-on` label, or the
`ubuntu:24.04` default. Docker is required. No host env vars are
passed unless you name them with `--env NAME` or `--env NAME=VALUE`
(values are never printed). `uses:` steps can't run here, and job
`services:` are left to you. Full details: [docs/RUN.md](docs/RUN.md).

## GitHub Action

On every PR, capture in CI and compare with a baseline committed from your laptop:

```bash
localci capture -o .localci/local.json
git add .localci/local.json && git commit -m "Add LocalCI baseline"
```

Then copy `examples/pr-check.yml` to `.github/workflows/localci.yml` in your repo. The action diffs CI against the baseline, comments the diff on the PR, uploads both fingerprints as artifacts, and fails the check when they differ.

**Matrix builds:** capture one baseline per cell (`.localci/ubuntu-node20.json`, `.localci/macos-node22.json`, …) and use `baseline-dir: .localci` — the action picks the baseline matching each runner. See `examples/pr-check-matrix.yml`. The diff also warns when the baseline is older than 30 days, and when GitHub's runner image changed.

Full setup, inputs, and FAQ: [docs/ACTION.md](docs/ACTION.md)

## What gets fingerprinted

- Runtime versions: Go, Node, Python, Rust, Java, Ruby, .NET
- OS, arch, OS release (`ubuntu 24.04`), CPU / memory, runner image
- Timezone, locale
- Env var names only — never values (opt-in values with `--include-values`)
- Lockfile hash **plus which packages changed** (`react 18.2.0 → 18.3.0, axios added`) — package-lock, yarn, pnpm, Cargo, poetry, Gemfile, composer, go.sum, Pipfile
- Services from the workflow (`postgres:16`), Dockerfile base image
- Git commit, file case-sensitivity, line endings, executable files, symlinks

## Roadmap

- **Phase 0** — Repo, vision, CLI skeleton
- **Phase 1** — MVP `capture` + `diff` CLI
- **Phase 2** — GitHub Action: capture in CI, compare in PR
- **Phase 3** — Workflow-aware diff + fix suggestions + severity (done)
- **Phase 4** — Matrix + baseline freshness (done)
- **Phase 5** — Fingerprint coverage (runtimes, services, lockfile detail) (done)
- **Phase 6** — Run the failing step locally in a matching container (done)
- **Phase 7** — Launch: demo, docs, GitLab, good-first-issues

Full plan: [docs/ROADMAP.md](docs/ROADMAP.md)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Good-first-issues will be added after Phase 1 lands.

## License

Dual-licensed under **Apache-2.0 / MIT**, per Ciago Technologies org policy. See `LICENSE-APACHE` and `LICENSE-MIT`.
