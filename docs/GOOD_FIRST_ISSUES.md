# Good first issues

Ready-to-paste issue drafts for new contributors. Each is small,
self-contained, and has a clear "done when". (The GitHub App used
to push code cannot create issues, so these live here until a
maintainer files them — copy a section into a new issue as-is.)

---

## 1. Verify `localci capture` on macOS

**Labels:** good first issue, help wanted

`localci capture` has a macOS code path (memory via `sysctl`, OS
release via `sw_vers`) but it has only ever been verified on Linux.
Run it on a Mac and confirm the fingerprint looks right.

1. On a Mac, build from source (`go build -o localci .`) and run
   `./localci capture`.
2. Check the output: `os` should be `darwin`, `memory_mb` a sane
   number, `os_release` like `macOS 15.x`, runtimes detected.
3. Report anything wrong — fix it if it is small, or file a
   follow-up issue with the bad output.

**Done when:** a comment with the `capture` output from a real Mac,
and any fixes needed are merged (or a follow-up issue exists).

---

## 2. Add a `bun` runtime to capture

**Labels:** good first issue

`capture` detects Go, Node, Python, Rust, Java, Ruby and .NET, but
not Bun (`bun --version`). Add it the same way the others work:

1. Add `bun` to the runtime detectors in
   `internal/fingerprint/capture.go` (follow the `rustc` pattern).
2. Add a test in `capture_test.go` with a stub `bun` binary on PATH
   (follow `TestCapture_ExtraRuntimes`).
3. Add `bun` to the workflow parser's `setup-bun` handling if the
   workflow uses it (see `internal/workflow/workflow.go`).

**Done when:** `localci capture` in a repo with Bun installed shows
`runtimes.bun`, and the new test passes.

---

## 3. `localci run --service` — start a job's services first

**Labels:** good first issue

Today `localci run --workflow …` warns that job `services:` (e.g.
`postgres:16`) are not started and leaves them to the user. Add a
flag that starts them:

1. `--service` (repeatable, e.g. `--service postgres:16`) starts
   each image detached with `docker run -d --name localci-<name>`.
2. Print the container names and a reminder to stop them
   (`docker rm -f localci-postgres`).
3. Keep the default unchanged: no services unless asked.

**Done when:** `localci run --dry-run --service postgres:16 -- …`
shows the service start command, and a real run with Docker starts
the container before the step runs.

---

## 4. SARIF output for `localci diff`

**Labels:** good first issue

Let CI systems that understand SARIF (GitHub code scanning, GitLab)
show LocalCI diffs as findings. Add `localci diff --sarif -o out.sarif`:

1. One SARIF `result` per difference: rule id from the diff key
   (e.g. `env:STRIPE_KEY`), level `error` for critical, `warning`
   for warning, `note` for info, message = the diff detail + fix.
2. Keep the existing human and `--json` outputs untouched.
3. Add a test that the SARIF file parses and contains the known
   fixture differences.

**Done when:** `localci diff --sarif` on the test fixtures produces
valid SARIF naming STRIPE_KEY as an error and Node as a warning.
