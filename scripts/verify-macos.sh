#!/usr/bin/env bash
# Verify `localci capture` on a real Mac.
#
# The macOS code paths (memory via sysctl, OS release via sw_vers)
# have only ever been verified on Linux. Run this on a Mac:
#
#   bash scripts/verify-macos.sh
#
# It builds from source, captures a fingerprint, and checks the
# fields a Mac must get right. Paste the output (or any failure)
# into the macOS good-first-issue, or fix it if it is small.
set -euo pipefail
cd "$(dirname "$0")/.."
go build -o /tmp/localci .
echo "== capture =="
/tmp/localci capture -o /tmp/localci-macos.json
cat /tmp/localci-macos.json
echo "== checks =="
python3 - <<'PY'
import json, sys
with open("/tmp/localci-macos.json") as f:
    fp = json.load(f)
checks = [
    ("os == darwin", fp.get("os") == "darwin"),
    ("memory_mb sane (>0)", isinstance(fp.get("memory_mb"), int) and fp["memory_mb"] > 0),
    ("os_release looks like macOS", str(fp.get("os_release", "")).startswith("macos ")),
    ("go runtime recorded", bool(fp.get("runtimes", {}).get("go"))),
    ("case_sensitive is a bool", isinstance(fp.get("case_sensitive"), bool)),
]
ok = True
for name, passed in checks:
    print(("PASS " if passed else "FAIL ") + name)
    ok = ok and passed
sys.exit(0 if ok else 1)
PY
echo "macOS verify: done — paste the capture above into the issue if anything failed."
