#!/usr/bin/env python3
"""Run the exact draft workflow checked by the Go API tests. Standard library only."""
import argparse
import io
import json
from pathlib import Path
import re
import sys
import urllib.error
import urllib.request
import uuid
import zipfile


def pointer(value, path):
    for part in path.strip("/").split("/"):
        part = part.replace("~1", "/").replace("~0", "~")
        value = value[int(part)] if isinstance(value, list) else value[part]
    return value


def request(url, step):
    body = json.dumps(step["body"]).encode() if "body" in step else None
    req = urllib.request.Request(url + step["path"], body, {"Content-Type": "application/json"}, method=step["method"])
    try:
        with urllib.request.urlopen(req, timeout=35) as response:
            return response.status, response.read(), response.headers.get_content_type()
    except urllib.error.HTTPError as response:
        return response.code, response.read(), response.headers.get_content_type()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", default="http://127.0.0.1:8081")
    parser.add_argument("--draft-id", default="shipping-guide-" + uuid.uuid4().hex[:8])
    parser.add_argument("--export", type=Path, help="save the final project ZIP to this path")
    args = parser.parse_args()
    if not re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}", args.draft_id):
        parser.error("draft id must be 1-64 letters, digits, _ or -")
    args.url = args.url.rstrip("/")
    fixture = Path(__file__).resolve().parents[1] / "docs/tutorials/draft-workflow.json"
    def substitute(value):
        if isinstance(value, str):
            return value.replace("{draft}", args.draft_id)
        if isinstance(value, list):
            return [substitute(item) for item in value]
        if isinstance(value, dict):
            return {key: substitute(item) for key, item in value.items()}
        return value
    guide = substitute(json.loads(fixture.read_text()))
    # Keep a full copy to prove no draft operation edits the service reference.
    status, raw, _ = request(args.url, {"method": "GET", "path": "/api/v1/base"})
    if status != 200:
        raise RuntimeError("Cannot read the reference project: " + raw.decode())
    reference = json.loads(raw)["files"]
    for number, step in enumerate(guide["steps"], 1):
        status, raw, content_type = request(args.url, step)
        expected = step["expect"]
        if status != expected["status"]:
            raise RuntimeError(f"{step['title']}: expected HTTP {expected['status']}, got {status}: {raw.decode(errors='replace')}")
        if "archive_member" in expected:
            if content_type != "application/zip":
                raise RuntimeError("Expected an exported ZIP project")
            with zipfile.ZipFile(io.BytesIO(raw)) as archive:
                source = archive.read(expected["archive_member"]).decode()
                if expected["archive_contains"] not in source:
                    raise RuntimeError("The exported project is missing the saved example")
            if args.export:
                if args.export.exists():
                    raise RuntimeError(f"Export already exists: {args.export}; choose another path")
                with args.export.open("xb") as output:
                    output.write(raw)
        else:
            value = json.loads(raw)
            for path, wanted in expected.get("json", {}).items():
                actual = pointer(value, path)
                if actual != wanted:
                    raise RuntimeError(f"{step['title']}: {path} expected {wanted!r}, got {actual!r}")
            if expected.get("reference_unchanged") and value["files"] != reference:
                raise RuntimeError("The reference project changed")
        print(f"{number:2}. {step['title']} — OK")
    print(f"\nDraft: {args.draft_id} (version 4). Quote: 2799 cents ($27.99).")
    print("Nothing was promoted. Drafts are in memory and disappear when the service stops.")
    if args.export:
        print(f"Export: {args.export}")
    else:
        print(f"Export before stopping: GET {args.url}/api/v1/drafts/{args.draft_id}/export")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, KeyError, RuntimeError, zipfile.BadZipFile) as error:
        print(f"draft walkthrough: {error}", file=sys.stderr)
        sys.exit(1)
