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

> **Status: Phase 1 — MVP `capture` + `diff` works.**
> See [docs/ROADMAP.md](docs/ROADMAP.md).

## Why LocalCI, not another CI runner?

Tools like `act` run your whole pipeline locally. LocalCI does the opposite, smaller job — and does it fast:

- **Don't run all of CI locally. Find the difference.**
- One static Go binary, no daemon, no server.
- Works for Go, Node, Python, Rust — any repo with CI.
- Built for the "works on my machine" moment, not for re-implementing GitHub Actions.

## Install

Phase 0 has no release binary yet. To build from source (Go 1.23+):

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
```

Example output:

```text
Found 4 difference(s) — local vs CI:
  env:STRIPE_KEY: local = set, CI = (missing)  (missing in CI)
  lockfile:package-lock.json: local = a1b2c3d4e5f6, CI = d4e5f6a1b2c3  (lockfile changed — run the package manager install step again)
  runtime:node: local = 20.11.0, CI = 20.9.0
  timezone: local = Asia/Kolkata, CI = UTC
```

CPU and memory differences are reported with `[info]` — a laptop and a CI runner rarely have the same hardware, so they don't count as a likely cause on their own.

## What gets fingerprinted (Phase 1)

- Runtime versions: Go, Node, Python
- OS, arch, CPU / memory
- Timezone, locale
- Env var names only — never values
- Lockfile hash (`go.sum`, `package-lock.json`, `poetry.lock`, etc.)
- Git commit
- File case-sensitivity

## Roadmap

- **Phase 0** — Repo, vision, CLI skeleton (this phase)
- **Phase 1** — MVP `capture` + `diff` CLI
- **Phase 2** — GitHub Action: capture in CI, compare in PR
- **Phase 3** — Fix suggestions for common mismatches
- **Phase 4** — Run the failing step locally in a matching container
- **Phase 5** — Launch: demo, docs, good-first-issues

Full plan: [docs/ROADMAP.md](docs/ROADMAP.md)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Good-first-issues will be added after Phase 1 lands.

## License

Dual-licensed under **Apache-2.0 / MIT**, per Ciago Technologies org policy. See `LICENSE-APACHE` and `LICENSE-MIT`.
