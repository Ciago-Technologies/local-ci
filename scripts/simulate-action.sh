#!/usr/bin/env bash
# Simulate the LocalCI GitHub Action locally.
#
# Replays the action's steps (build, capture, diff, artifacts) for
# four scenarios, without needing GitHub Actions or `act`:
#   1. match     — baseline and CI fingerprints match → check passes
#   2. mismatch  — they differ → check fails with a diff
#   3. missing   — no baseline → warning, check passes
#   4. workflow-aware — same mismatch, but with --workflows the
#      diff only shows what the job actually uses (3 of 7)
#   7. run — `localci run --dry-run` shows the exact docker command
#      for a workflow step (image from runs-on, secrets skipped).
#
# Usage: ./scripts/simulate-action.sh
# Requires: go 1.23+, bash. Exits non-zero if any scenario misbehaves.
set -u

GO="${GO:-go}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "== Building localci =="
(cd "$ROOT" && "$GO" build -o "$TMP/localci" .) || { echo "FAIL: build"; exit 1; }

fail=0
run_scenario() {
  local name="$1" baseline="$2" ci="$3" expect_exit="$4"
  echo ""
  echo "== Scenario: $name (expect exit $expect_exit) =="
  mkdir -p "$TMP/$name"
  if [ -n "$baseline" ]; then
    cp "$baseline" "$TMP/$name/local.json"
  fi
  # Capture a "CI" fingerprint from the fixture (copy, don't re-capture,
  # so the scenario is deterministic).
  cp "$ci" "$TMP/$name/ci.json"
  if [ ! -f "$TMP/$name/local.json" ]; then
    echo "No baseline fingerprint at .localci/local.json."
    echo "Run 'localci capture -o .localci/local.json' on your laptop and commit it."
    got_exit=0
  else
    "$TMP/localci" diff "$TMP/$name/local.json" "$TMP/$name/ci.json" > "$TMP/$name/diff.txt" 2>&1
    got_exit=$?
    cat "$TMP/$name/diff.txt"
    cp "$TMP/$name/ci.json" "$TMP/$name/localci-diff.md" 2>/dev/null || true
  fi
  if [ "$got_exit" -ne "$expect_exit" ]; then
    echo "FAIL: scenario $name exited $got_exit, want $expect_exit"
    fail=1
  else
    echo "PASS: scenario $name"
  fi
}

FIX="$ROOT/internal/fingerprint/testdata"
# 1. Match: same file on both sides.
run_scenario "match" "$FIX/local.json" "$FIX/local.json" 0
# 2. Mismatch: the known fixture mismatch (Node, timezone, env, lockfile).
run_scenario "mismatch" "$FIX/local.json" "$FIX/ci.json" 1
# 3. Missing baseline: no local.json at all.
run_scenario "missing" "" "$FIX/ci.json" 0

# 4. Workflow-aware: same mismatch, filtered to what the job uses.
echo ""
echo "== Scenario: workflow-aware (expect exit 1, 3 shown, 4 hidden) =="
mkdir -p "$TMP/workflow-aware"
cp "$FIX/local.json" "$TMP/workflow-aware/local.json"
cp "$FIX/ci.json" "$TMP/workflow-aware/ci.json"
"$TMP/localci" diff --workflows "$ROOT/internal/workflow/testdata" \
  "$TMP/workflow-aware/local.json" "$TMP/workflow-aware/ci.json" > "$TMP/workflow-aware/diff.txt" 2>&1
got_exit=$?
cat "$TMP/workflow-aware/diff.txt"
if [ "$got_exit" -ne 1 ]; then
  echo "FAIL: scenario workflow-aware exited $got_exit, want 1"
  fail=1
elif ! grep -q "Found 3 difference(s)" "$TMP/workflow-aware/diff.txt" || ! grep -q "4 difference(s) hidden" "$TMP/workflow-aware/diff.txt"; then
  echo "FAIL: scenario workflow-aware did not show 3 diffs + 4 hidden"
  fail=1
else
  echo "PASS: scenario workflow-aware"
fi

# 5. Matrix: --baseline-dir picks the baseline matching the CI env.
echo ""
echo "== Scenario: matrix baseline pick (expect exit 1, picked ubuntu-node20) =="
mkdir -p "$TMP/matrix"
cp "$FIX/ci.json" "$TMP/matrix/ci.json"
"$TMP/localci" diff --baseline-dir "$ROOT/internal/fingerprint/testdata/baselines" \
  "$TMP/matrix/ci.json" > "$TMP/matrix/diff.txt" 2>&1
got_exit=$?
cat "$TMP/matrix/diff.txt"
if [ "$got_exit" -ne 1 ]; then
  echo "FAIL: scenario matrix exited $got_exit, want 1"
  fail=1
elif ! grep -q "Baseline: .*/ubuntu-node20.json" "$TMP/matrix/diff.txt"; then
  echo "FAIL: scenario matrix did not pick ubuntu-node20.json"
  fail=1
else
  echo "PASS: scenario matrix"
fi

# 6. Lockfile detail: the diff names the packages that changed.
echo ""
echo "== Scenario: lockfile detail (expect exit 1, react bump named) =="
mkdir -p "$TMP/lockdetail"
printf '{"packages":{"node_modules/react":{"version":"18.2.0"},"node_modules/moment":{"version":"2.29.0"}}}' > "$TMP/lockdetail/package-lock.json"
"$TMP/localci" capture --dir "$TMP/lockdetail" -o "$TMP/lockdetail/local.json" > /dev/null 2>&1
printf '{"packages":{"node_modules/react":{"version":"18.3.0"},"node_modules/axios":{"version":"1.7.0"}}}' > "$TMP/lockdetail/package-lock.json"
"$TMP/localci" capture --dir "$TMP/lockdetail" -o "$TMP/lockdetail/ci.json" > /dev/null 2>&1
"$TMP/localci" diff "$TMP/lockdetail/local.json" "$TMP/lockdetail/ci.json" > "$TMP/lockdetail/diff.txt" 2>&1
got_exit=$?
cat "$TMP/lockdetail/diff.txt"
if [ "$got_exit" -ne 1 ]; then
  echo "FAIL: scenario lockfile detail exited $got_exit, want 1"
  fail=1
elif ! grep -q "react 18.2.0 → 18.3.0" "$TMP/lockdetail/diff.txt" || ! grep -q "axios added" "$TMP/lockdetail/diff.txt"; then
  echo "FAIL: scenario lockfile detail did not name the changed packages"
  fail=1
else
  echo "PASS: scenario lockfile detail"
fi

# 7. Run: dry-run a workflow step — no Docker needed for the check.
echo ""
echo "== Scenario: run dry-run (expect exit 0, ubuntu:24.04 from runs-on) =="
mkdir -p "$TMP/run"
"$TMP/localci" run --dry-run --dir "$TMP/run" \
  --workflow "$ROOT/internal/workflow/testdata/run.yml" --job test --step Test \
  > "$TMP/run/out.txt" 2>&1
got_exit=$?
cat "$TMP/run/out.txt"
if [ "$got_exit" -ne 0 ]; then
  echo "FAIL: scenario run exited $got_exit, want 0"
  fail=1
elif ! grep -q "image: ubuntu:24.04 (from runs-on ubuntu-latest)" "$TMP/run/out.txt" \
  || ! grep -q "sh -c 'npm test -- --watchAll=false'" "$TMP/run/out.txt" \
  || ! grep -q "STRIPE_KEY reference secrets" "$TMP/run/out.txt" \
  || ! grep -q "services (postgres:16)" "$TMP/run/out.txt"; then
  echo "FAIL: scenario run did not show the expected image, command, and warnings"
  fail=1
else
  echo "PASS: scenario run"
fi

# 8. Ignore git_commit: identical except the commit → exit 0 with
#    --ignore git_commit (dogfood: a baseline's commit always
#    differs from the PR head, so the Action ignores it).
echo ""
echo "== Scenario: ignore git_commit (expect exit 0) =="
mkdir -p "$TMP/ignore-commit"
cp "$FIX/local.json" "$TMP/ignore-commit/local.json"
python3 -c "
import json
d = json.load(open('$FIX/local.json'))
d['git_commit'] = 'different00000000000000000000000000000000'
json.dump(d, open('$TMP/ignore-commit/ci.json', 'w'))
"
"$TMP/localci" diff --ignore git_commit "$TMP/ignore-commit/local.json" "$TMP/ignore-commit/ci.json" > "$TMP/ignore-commit/diff.txt" 2>&1
got_exit=$?
cat "$TMP/ignore-commit/diff.txt"
if [ "$got_exit" -ne 0 ]; then
  echo "FAIL: scenario ignore-commit exited $got_exit, want 0"
  fail=1
elif ! grep -q "No differences" "$TMP/ignore-commit/diff.txt"; then
  echo "FAIL: scenario ignore-commit did not report no differences"
  fail=1
else
  echo "PASS: scenario ignore-commit"
fi

# 9. Runner image drift is hidden by the workflow filter: identical
#    except runner_image, with --workflows -> exit 0 (dogfood: the
#    baseline's image drifted two days after capture; GitHub updates
#    images roughly weekly, so showing it would fail every PR).
echo ""
echo "== Scenario: runner image drift hidden (expect exit 0) =="
mkdir -p "$TMP/image-drift"
python3 -c "
import json
d = json.load(open('$FIX/local.json'))
d['runner_image'] = 'ubuntu26/20261116.88.1'
json.dump(d, open('$TMP/image-drift/local.json', 'w'))
d2 = json.load(open('$FIX/local.json'))
d2['runner_image'] = 'ubuntu26/20261116.202.1'
json.dump(d2, open('$TMP/image-drift/ci.json', 'w'))
"
"$TMP/localci" diff --workflows "$ROOT/internal/workflow/testdata" "$TMP/image-drift/local.json" "$TMP/image-drift/ci.json" > "$TMP/image-drift/diff.txt" 2>&1
got_exit=$?
cat "$TMP/image-drift/diff.txt"
if [ "$got_exit" -ne 0 ]; then
  echo "FAIL: scenario image-drift exited $got_exit, want 0"
  fail=1
elif ! grep -q "No differences" "$TMP/image-drift/diff.txt"; then
  echo "FAIL: scenario image-drift did not report no differences"
  fail=1
else
  echo "PASS: scenario image-drift"
fi

echo ""
if [ "$fail" -ne 0 ]; then
  echo "SIMULATION FAILED"
  exit 1
fi
echo "SIMULATION PASSED: 9/9 scenarios"
