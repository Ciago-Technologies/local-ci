# Contributing to LocalCI

Thanks for helping make "works on my machine" a solved problem.

## Good first issues

New here? Start with [docs/GOOD_FIRST_ISSUES.md](docs/GOOD_FIRST_ISSUES.md)
— small, self-contained tasks with a clear "done when". Pick one,
comment on the issue (or open it from the draft), and send a PR.

## How to contribute

1. Fork the repo and create a branch from `main`.
2. Keep changes small and focused — one fix per PR.
3. Run `go vet ./...` and `go test ./...` before pushing.
4. Never add secrets, tokens, or env var values to code, tests, or fixtures. Env var **names** only.

## Reporting issues

Open an issue with:
- What you ran (`localci --version`, OS, Go version)
- What you expected
- What happened, with the exact output

Security issues: do **not** open a public issue. Email security@ciagotech.com, per Ciago Technologies policy.
