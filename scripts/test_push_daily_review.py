#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import json
import os
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("push_daily_review.py")
SPEC = importlib.util.spec_from_file_location("push_daily_review", SCRIPT_PATH)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class TestServer:
    def __init__(self, responses: list[tuple[int, dict]]) -> None:
        self.responses = list(responses)
        self.requests: list[dict] = []

        owner = self

        class Handler(BaseHTTPRequestHandler):
            def do_POST(self) -> None:
                length = int(self.headers.get("Content-Length", "0"))
                owner.requests.append(
                    {
                        "path": self.path,
                        "headers": dict(self.headers),
                        "body": self.rfile.read(length),
                    }
                )
                status, payload = owner.responses.pop(0)
                raw = json.dumps(payload).encode("utf-8")
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(raw)))
                self.end_headers()
                self.wfile.write(raw)

            def log_message(self, _format: str, *_args) -> None:
                return

        self.httpd = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.httpd.serve_forever, daemon=True)

    def __enter__(self):
        self.thread.start()
        host, port = self.httpd.server_address
        self.base_url = f"http://{host}:{port}"
        return self

    def __exit__(self, *_args) -> None:
        self.httpd.shutdown()
        self.httpd.server_close()
        self.thread.join(timeout=2)


def success(result: str = "created") -> dict:
    return {
        "success": True,
        "errorCode": 0,
        "message": "",
        "data": {"result": result, "kind": "focus", "date": "2026-10-28"},
    }


class PushDailyReviewTests(unittest.TestCase):
    def test_upload_sends_only_user_token_and_expected_payload(self) -> None:
        with TestServer([(200, success())]) as server:
            result = MODULE.upload(
                base_url=server.base_url,
                report_date="2026-10-28",
                kind="focus",
                markdown="# 重点公司\n",
                token="test-token",
                sleep_fn=lambda _: None,
            )
        self.assertEqual(result["result"], "created")
        self.assertEqual(len(server.requests), 1)
        request = server.requests[0]
        self.assertEqual(request["path"], "/api/daily-review/ingest/focus?date=2026-10-28")
        headers = {key.lower(): value for key, value in request["headers"].items()}
        self.assertEqual(headers["x-user-token"], "test-token")
        self.assertNotIn("authorization", headers)
        self.assertNotIn("cookie", headers)
        self.assertEqual(json.loads(request["body"]), {"markdown": "# 重点公司\n"})

    def test_success_false_is_failure_even_with_http_200(self) -> None:
        payload = {"success": False, "errorCode": 2, "message": "no permission", "data": None}
        with TestServer([(200, payload)]) as server:
            with self.assertRaisesRegex(MODULE.UploadError, "no permission"):
                MODULE.upload(
                    base_url=server.base_url,
                    report_date="2026-10-28",
                    kind="focus",
                    markdown="# content",
                    token="test-token",
                    sleep_fn=lambda _: None,
                )
        self.assertEqual(len(server.requests), 1)

    def test_authentication_errors_are_not_retried(self) -> None:
        for status in (401, 403):
            with self.subTest(status=status):
                payload = {"success": False, "message": "denied", "data": None}
                with TestServer([(status, payload)]) as server:
                    with self.assertRaisesRegex(MODULE.UploadError, "denied"):
                        MODULE.upload(
                            base_url=server.base_url,
                            report_date="2026-10-28",
                            kind="focus",
                            markdown="# content",
                            token="test-token",
                            sleep_fn=lambda _: None,
                        )
                self.assertEqual(len(server.requests), 1)

    def test_server_errors_are_retried(self) -> None:
        transient = {"success": False, "message": "temporarily unavailable", "data": None}
        with TestServer([(503, transient), (500, transient), (200, success("updated"))]) as server:
            result = MODULE.upload(
                base_url=server.base_url,
                report_date="2026-10-28",
                kind="focus",
                markdown="# content",
                token="test-token",
                retry_delay=0,
                sleep_fn=lambda _: None,
            )
        self.assertEqual(result["result"], "updated")
        self.assertEqual(len(server.requests), 3)

    def test_remote_http_is_rejected(self) -> None:
        with self.assertRaisesRegex(MODULE.UploadError, "must use HTTPS"):
            MODULE.validate_base_url("http://bbs.example.com")

    def test_missing_token_is_rejected(self) -> None:
        previous = os.environ.pop("TEST_MISSING_DAILY_REVIEW_TOKEN", None)
        self.addCleanup(
            lambda: previous is not None
            and os.environ.__setitem__("TEST_MISSING_DAILY_REVIEW_TOKEN", previous)
        )
        with self.assertRaisesRegex(MODULE.UploadError, "is required"):
            MODULE.load_token("TEST_MISSING_DAILY_REVIEW_TOKEN")

    def test_oversized_file_is_rejected(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "report.md"
            path.write_bytes(b"x" * (MODULE.MAX_MARKDOWN_BYTES + 1))
            with self.assertRaisesRegex(MODULE.UploadError, "exceeds"):
                MODULE.read_markdown(str(path))


if __name__ == "__main__":
    unittest.main()
