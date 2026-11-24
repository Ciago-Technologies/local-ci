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

echo ""
if [ "$fail" -ne 0 ]; then
  echo "SIMULATION FAILED"
  exit 1
fi
echo "SIMULATION PASSED: 5/5 scenarios"
