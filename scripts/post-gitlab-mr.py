#!/usr/bin/env python3
"""Post a LocalCI diff as a GitLab merge-request note.

Reads localci-diff.md (written by examples/gitlab-ci.yml) and posts
it to $CI_API_V4_URL/projects/$CI_PROJECT_ID/merge_requests/
$CI_MERGE_REQUEST_IID/notes using $CI_JOB_TOKEN.

Best-effort: exits 0 even when posting fails, so a comment failure
never hides the diff (which is already in the job log) or changes
the job outcome. Prints a clear message either way.
"""
import json
import os
import sys
import urllib.error
import urllib.request


def main() -> int:
    mr = os.environ.get("CI_MERGE_REQUEST_IID", "")
    project = os.environ.get("CI_PROJECT_ID", "")
    if not mr or not project:
        print("LocalCI: not a merge request pipeline — no MR comment to post.")
        return 0
    token = os.environ.get("CI_JOB_TOKEN", "")
    if not token:
        print("LocalCI: no CI_JOB_TOKEN — cannot post MR comment.")
        return 0
    try:
        with open("localci-diff.md", encoding="utf-8") as f:
            body = f.read()
    except OSError as e:
        print(f"LocalCI: could not read localci-diff.md: {e}")
        return 0
    api = os.environ.get("CI_API_V4_URL", "https://gitlab.com/api/v4")
    url = f"{api}/projects/{project}/merge_requests/{mr}/notes"
    payload = json.dumps({"body": body}).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=payload,
        headers={"Content-Type": "application/json", "JOB-TOKEN": token},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            print(f"LocalCI: posted MR comment (HTTP {resp.status})")
    except urllib.error.HTTPError as e:
        print(f"LocalCI: could not post MR comment (HTTP {e.code}); the diff is in the job log.")
    except Exception as e:  # network / DNS / timeout — never fail the job
        print(f"LocalCI: could not post MR comment ({e}); the diff is in the job log.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
