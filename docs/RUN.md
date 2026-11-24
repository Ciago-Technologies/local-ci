# Running a failing step locally — `localci run`

`localci diff` tells you *what* differs between your laptop and CI.
`localci run` lets you *reproduce* the failure: it runs one command
in a Docker container that matches the CI environment, with your repo
mounted at `/work`.

It is deliberately not a CI runner. One command, one container, no
workflow engine, no services, no `act` clone. If you need the whole
pipeline, use `act`.

## Requirements

Docker. That's it — `localci run` shells out to the `docker` CLI.
Without Docker it fails with install guidance, and `--dry-run`
still shows the exact command that would run.

## Usage

```sh
# Run any command in the CI-matching container:
localci run -- npm test

# Pick the image explicitly:
localci run --image node:20 -- npm test

# Run one workflow step's run: script, in the container matching
# that job's runs-on label:
localci run --workflow .github/workflows/ci.yml --job test --step Test
```

## How the image is picked

In order, first match wins:

1. `--image` flag
2. CI fingerprint's runner image — `localci run --fingerprint ci.json ...`
   maps e.g. `ubuntu24/20251123.1` to `ubuntu:24.04`
3. CI fingerprint's Dockerfile image (the repo's `FROM` line)
4. The local repo's Dockerfile `FROM` line
5. The workflow job's `runs-on` label — `ubuntu-latest` → `ubuntu:24.04`
6. Default: `ubuntu:24.04`

macOS and Windows runners cannot run in Docker — `localci run`
refuses them and tells you to pass a Linux image with `--image`.

## What runs, and what doesn't

- The repo directory (or `--dir`) is mounted at `/work`; the command
  runs there as your user id, so files it creates stay yours.
- The command's exit code is passed through: 0 on success, the
  container's code on failure. `localci run` itself exits 2 only
  for its own errors (no Docker, bad flags, missing step).
- `docker run --rm` — the container is removed when it exits.

## Env vars and secrets

By default, **no** host env vars are passed into the container.
Pass them explicitly, and only the ones the command needs:

```sh
localci run --env NODE_ENV=test --env STRIPE_KEY -- npm test
```

- `--env NAME` takes the value from your shell's environment.
- `--env NAME=VALUE` sets it directly.
- Values are never printed — `--dry-run` shows `NAME=***`.

When running a workflow step, its literal `env:` values are passed
through. Values containing `${{ ... }}` expressions (secrets,
contexts) are skipped with a warning — LocalCI cannot resolve them
and never guesses. Pass them yourself with `--env`.

## What it won't do

- `uses:` steps — actions are code, not shell commands. `localci run`
  refuses them and names the action.
- Job `services:` (e.g. `postgres:16`) — it warns and leaves them to
  you. Start them yourself first (`docker run -d postgres:16`), then
  run the step.
- Matrix jobs — pick one cell's workflow/job and run that.

## Checking what it would run

```sh
localci run --dry-run --workflow .github/workflows/ci.yml --job test --step Test
# image: ubuntu:24.04 (from runs-on ubuntu-latest)
# docker run --rm -v /home/you/repo:/work -w /work --user 1000:1000 -e 'NODE_ENV=***' ubuntu:24.04 sh -c 'npm test'
```

## See also

- [ACTION.md](ACTION.md) — the GitHub Action (finds the diff in CI)
- [ROADMAP.md](ROADMAP.md) — Phase 6, the container run
