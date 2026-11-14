#!/usr/bin/env python3
"""Publish a validated Kalshi handoff document to BBS-Go."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import socket
import sys
import time
from datetime import date
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode, urlsplit
from urllib.request import Request, urlopen

MAX_MARKDOWN_BYTES = 16 * 1024 * 1024


class UploadError(Exception):
    pass


class RetryableUploadError(UploadError):
    pass


def manifest_identity(markdown: str) -> tuple[str, str]:
    blocks = re.findall(r"^```json manifest\.json\r?\n(.*?)^```\r?$", markdown, re.M | re.S)
    if len(blocks) != 1:
        raise UploadError("expected exactly one manifest.json block")
    try:
        manifest = json.loads(blocks[0])
        report_date = manifest["date"]
        if not re.fullmatch(r"\d{4}-\d{2}-\d{2}", report_date):
            raise ValueError("invalid date")
        if date.fromisoformat(report_date).isoformat() != report_date:
            raise ValueError("invalid date")
        slot = manifest["metadata"]["run_slot"]
        if slot not in {"morning", "evening"}:
            raise ValueError("invalid run_slot")
        if manifest["publication_ready"] is not True:
            raise ValueError("publication_ready is false")
        return report_date, slot
    except (TypeError, KeyError, ValueError, json.JSONDecodeError) as exc:
        raise UploadError("invalid manifest date, run_slot or publication_ready") from exc


def request_json(url: str, token: str, markdown: str | None = None) -> dict:
    body = json.dumps({"markdown": markdown}, ensure_ascii=False).encode("utf-8") if markdown is not None else None
    request = Request(url, data=body, method="POST" if body is not None else "GET", headers={
        "Accept": "application/json",
        "X-User-Token": token,
        **({"Content-Type": "application/json; charset=utf-8"} if body is not None else {}),
    })
    try:
        with urlopen(request, timeout=60) as response:
            raw = response.read()
    except HTTPError as exc:
        if exc.code >= 500:
            raise RetryableUploadError(f"HTTP {exc.code}") from exc
        raise UploadError(f"HTTP {exc.code}") from exc
    except (URLError, TimeoutError, socket.timeout) as exc:
        raise RetryableUploadError(f"request failed: {exc}") from exc
    try:
        payload = json.loads(raw)
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise UploadError("invalid JSON response") from exc
    if not isinstance(payload, dict) or payload.get("success") is not True:
        raise UploadError(f"BBS API reported failure: {payload.get('message', '') if isinstance(payload, dict) else ''}")
    data = payload.get("data")
    if not isinstance(data, dict):
        raise UploadError("invalid API response data")
    return data


def publish(base_url: str, token: str, markdown: str) -> tuple[str, str, str]:
    report_date, slot = manifest_identity(markdown)
    url = f"{base_url}/api/kalshi/status?{urlencode({'date': report_date, 'slot': slot})}"

    def status_hash() -> str | None:
        status = request_json(url, token)
        if status.get("date") != report_date or status.get("slot") != slot or not isinstance(status.get("published"), bool):
            raise UploadError("invalid publication status")
        if not status["published"]:
            return None
        digest = status.get("content_hash")
        if not isinstance(digest, str) or not re.fullmatch(r"[0-9a-f]{64}", digest):
            raise UploadError("missing published content hash")
        return digest

    digest = hashlib.sha256(markdown.encode("utf-8")).hexdigest()
    if status_hash() == digest:
        return "unchanged", report_date, slot

    for attempt in range(3):
        try:
            result = request_json(f"{base_url}/api/kalshi/ingest", token, markdown)
            if result.get("result") not in {"created", "updated", "unchanged"} or result.get("date") != report_date or result.get("slot") != slot or result.get("content_hash") != digest:
                raise UploadError("unexpected ingest result")
            if status_hash() != digest:
                raise UploadError("post-upload status hash mismatch")
            return result["result"], report_date, slot
        except RetryableUploadError:
            if attempt == 2:
                raise
            time.sleep(attempt + 1)
    raise AssertionError("unreachable")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", required=True)
    parser.add_argument("--file", required=True, help="absolute path to the UTF-8 handoff Markdown")
    args = parser.parse_args()
    try:
        base_url = args.base_url.strip().rstrip("/")
        parsed = urlsplit(base_url)
        if parsed.scheme not in {"http", "https"} or not parsed.netloc or parsed.username or parsed.password or parsed.query or parsed.fragment or (parsed.scheme == "http" and parsed.hostname not in {"localhost", "127.0.0.1", "::1"}):
            raise UploadError("base URL must use HTTPS (or localhost HTTP), without credentials or query")
        token = os.environ.get("BBSGO_USER_TOKEN", "").strip()
        if not token:
            raise UploadError("BBSGO_USER_TOKEN is required")
        source = Path(args.file).read_bytes()
        if not source or len(source) > MAX_MARKDOWN_BYTES:
            raise UploadError("Markdown is empty or too large")
        try:
            markdown = source.decode("utf-8")
        except UnicodeDecodeError as exc:
            raise UploadError("Markdown must use UTF-8") from exc
        result, report_date, slot = publish(base_url, token, markdown)
    except (UploadError, OSError) as exc:
        print(f"Kalshi upload failed: {exc}", file=sys.stderr)
        return 1
    print(f"{result}: {report_date} {slot}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
