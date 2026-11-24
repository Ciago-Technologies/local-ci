#!/usr/bin/env bash
# Simulate the LocalCI GitHub Action locally.
#
# Replays the action's steps (build, capture, diff, artifacts) for
# three scenarios, without needing GitHub Actions or `act`:
#   1. match     — baseline and CI fingerprints match → check passes
#   2. mismatch  — they differ → check fails with a diff
#   3. missing   — no baseline → warning, check passes
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

echo ""
if [ "$fail" -ne 0 ]; then
  echo "SIMULATION FAILED"
  exit 1
fi
echo "SIMULATION PASSED: 3/3 scenarios"
