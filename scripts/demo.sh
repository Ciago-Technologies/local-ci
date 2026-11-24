#!/usr/bin/env bash
# The 30-second LocalCI demo — the aha moment, live in your terminal.
#
# Builds localci, then replays the story: tests pass on your laptop,
# fail in CI, and LocalCI names the difference in seconds — then shows
# how to reproduce the failing step locally.
#
# Usage: ./scripts/demo.sh
# Requires: go 1.23+, bash. No Docker needed (the run step is --dry-run).
set -u

GO="${GO:-go}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "== Building localci =="
(cd "$ROOT" && "$GO" build -o "$TMP/localci" .) || exit 1
BIN="$TMP/localci"

FIX="$ROOT/internal/fingerprint/testdata"

echo ""
echo "$ localci diff local.json ci.json   # laptop vs CI"
echo ""
"$BIN" diff "$FIX/local.json" "$FIX/ci.json" || true

echo ""
echo "-- but only 3 of those matter to this repo's workflow: --"
echo ""
echo "$ localci diff --workflows .github/workflows local.json ci.json"
echo ""
"$BIN" diff --workflows "$ROOT/internal/workflow/testdata" \
  "$FIX/local.json" "$FIX/ci.json" || true

echo ""
echo "-- and reproduce the failing step locally: --"
echo ""
echo "$ localci run --workflow .github/workflows/ci.yml --job test --step Test"
echo ""
"$BIN" run --dry-run --dir "$TMP" \
  --workflow "$ROOT/internal/workflow/testdata/run.yml" --job test --step Test 2>&1 || true

echo ""
echo "That's LocalCI: find out locally why CI will fail — before you push."
