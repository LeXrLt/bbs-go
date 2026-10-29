#!/usr/bin/env python3
"""Push one Markdown daily-review section to the authenticated BBS API."""

from __future__ import annotations

import argparse
import json
import os
import socket
import sys
import time
from datetime import date
from pathlib import Path
from typing import Any, Callable
from urllib.error import HTTPError, URLError
from urllib.parse import quote, urlencode, urlsplit
from urllib.request import Request, urlopen


KINDS = {
    "shareholding",
    "announcements",
    "movers",
    "focus",
    "buybacks",
    "shorts",
}
MAX_MARKDOWN_BYTES = 1_000_000
DEFAULT_TOKEN_ENV = "BBSGO_USER_TOKEN"
DEFAULT_TIMEOUT_SECONDS = 30.0
DEFAULT_MAX_ATTEMPTS = 3
DEFAULT_RETRY_DELAY_SECONDS = 1.0


class UploadError(RuntimeError):
    """A permanent validation, authentication, or API response error."""


class RetryableUploadError(UploadError):
    """A transient network or server error that may be retried."""


def parse_report_date(value: str) -> str:
    try:
        parsed = date.fromisoformat(value)
    except ValueError as exc:
        raise UploadError("date must use YYYY-MM-DD") from exc
    if parsed.isoformat() != value:
        raise UploadError("date must use YYYY-MM-DD")
    return value


def validate_kind(value: str) -> str:
    if value not in KINDS:
        raise UploadError(f"kind must be one of: {', '.join(sorted(KINDS))}")
    return value


def validate_base_url(value: str) -> str:
    value = value.strip().rstrip("/")
    parsed = urlsplit(value)
    if (
        parsed.scheme not in {"http", "https"}
        or not parsed.netloc
        or parsed.username is not None
        or parsed.password is not None
        or parsed.query
        or parsed.fragment
    ):
        raise UploadError("base URL must be an absolute HTTP(S) URL without credentials, query, or fragment")
    if parsed.scheme == "http" and parsed.hostname not in {"localhost", "127.0.0.1", "::1"}:
        raise UploadError("remote BBS URL must use HTTPS")
    return value


def load_token(env_name: str = DEFAULT_TOKEN_ENV) -> str:
    token = os.environ.get(env_name, "").strip()
    if not token:
        raise UploadError(f"environment variable {env_name} is required")
    return token


def read_markdown(path_value: str) -> str:
    path = Path(path_value)
    if not path.is_file():
        raise UploadError(f"Markdown file does not exist: {path}")
    raw = path.read_bytes()
    if len(raw) > MAX_MARKDOWN_BYTES:
        raise UploadError(f"Markdown exceeds {MAX_MARKDOWN_BYTES} bytes")
    try:
        markdown = raw.decode("utf-8")
    except UnicodeDecodeError as exc:
        raise UploadError("Markdown file must use UTF-8") from exc
    if not markdown.strip():
        raise UploadError("Markdown file must not be empty")
    return markdown


def response_message(payload: Any, fallback: str) -> str:
    if isinstance(payload, dict):
        message = payload.get("message")
        if isinstance(message, str) and message.strip():
            return message.strip()
    return fallback


def decode_response(raw: bytes) -> dict[str, Any]:
    try:
        payload = json.loads(raw.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise UploadError("BBS returned invalid JSON") from exc
    if not isinstance(payload, dict):
        raise UploadError("BBS returned an invalid response object")
    return payload


def request_once(
    *,
    base_url: str,
    report_date: str,
    kind: str,
    markdown: str,
    token: str,
    timeout: float,
) -> dict[str, Any]:
    query = urlencode({"date": report_date})
    url = f"{base_url}/api/daily-review/ingest/{quote(kind)}?{query}"
    body = json.dumps({"markdown": markdown}, ensure_ascii=False).encode("utf-8")
    request = Request(
        url,
        data=body,
        method="POST",
        headers={
            "Accept": "application/json",
            "Content-Type": "application/json; charset=utf-8",
            "X-User-Token": token,
        },
    )
    try:
        with urlopen(request, timeout=timeout) as response:
            status = response.status
            raw = response.read()
    except HTTPError as exc:
        raw = exc.read()
        try:
            payload = decode_response(raw)
        except UploadError:
            payload = None
        message = response_message(payload, f"HTTP {exc.code}")
        if 500 <= exc.code <= 599:
            raise RetryableUploadError(message) from exc
        raise UploadError(message) from exc
    except (URLError, TimeoutError, socket.timeout) as exc:
        raise RetryableUploadError(f"BBS request failed: {exc}") from exc

    if status < 200 or status >= 300:
        if status >= 500:
            raise RetryableUploadError(f"HTTP {status}")
        raise UploadError(f"HTTP {status}")

    payload = decode_response(raw)
    if payload.get("success") is not True:
        raise UploadError(response_message(payload, "BBS API reported failure"))
    data = payload.get("data")
    if not isinstance(data, dict):
        raise UploadError("BBS response is missing data")
    result = data.get("result")
    if result not in {"created", "updated", "unchanged"}:
        raise UploadError("BBS response contains an unknown ingest result")
    return data


def upload(
    *,
    base_url: str,
    report_date: str,
    kind: str,
    markdown: str,
    token: str,
    timeout: float = DEFAULT_TIMEOUT_SECONDS,
    max_attempts: int = DEFAULT_MAX_ATTEMPTS,
    retry_delay: float = DEFAULT_RETRY_DELAY_SECONDS,
    sleep_fn: Callable[[float], None] = time.sleep,
) -> dict[str, Any]:
    base_url = validate_base_url(base_url)
    report_date = parse_report_date(report_date)
    kind = validate_kind(kind)
    if len(markdown.encode("utf-8")) > MAX_MARKDOWN_BYTES:
        raise UploadError(f"Markdown exceeds {MAX_MARKDOWN_BYTES} bytes")
    if not markdown.strip():
        raise UploadError("Markdown must not be empty")
    if not token.strip():
        raise UploadError("token must not be empty")
    if max_attempts < 1:
        raise UploadError("max attempts must be positive")

    for attempt in range(1, max_attempts + 1):
        try:
            return request_once(
                base_url=base_url,
                report_date=report_date,
                kind=kind,
                markdown=markdown,
                token=token,
                timeout=timeout,
            )
        except RetryableUploadError:
            if attempt == max_attempts:
                raise
            sleep_fn(retry_delay * attempt)
    raise AssertionError("unreachable")


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", required=True, help="BBS root URL")
    parser.add_argument("--date", required=True, dest="report_date", help="review date in YYYY-MM-DD")
    parser.add_argument("--kind", required=True, choices=sorted(KINDS))
    parser.add_argument("--file", required=True, dest="markdown_file", help="UTF-8 Markdown file")
    parser.add_argument(
        "--token-env",
        default=DEFAULT_TOKEN_ENV,
        help=f"environment variable containing the user token (default: {DEFAULT_TOKEN_ENV})",
    )
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        result = upload(
            base_url=args.base_url,
            report_date=args.report_date,
            kind=args.kind,
            markdown=read_markdown(args.markdown_file),
            token=load_token(args.token_env),
        )
    except UploadError as exc:
        print(f"daily review upload failed: {exc}", file=sys.stderr)
        return 1
    print(f"{result['result']}: {result.get('date', args.report_date)} {result.get('kind', args.kind)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
