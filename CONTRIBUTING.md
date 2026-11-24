# Contributing to LocalCI

Thanks for helping make "works on my machine" a solved problem.

## Phase 0 note

This repo is in Phase 0 — skeleton only. `capture` and `diff` are stubs until Phase 1 lands. Please don't open large feature PRs yet; start with docs, typos, or a discussion issue.

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
