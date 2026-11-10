import importlib.util
import hashlib
import unittest
from pathlib import Path
from unittest.mock import patch


spec = importlib.util.spec_from_file_location("push_kalshi_report", Path(__file__).with_name("push_kalshi_report.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class PublishTests(unittest.TestCase):
    markdown = '# Report\n\n```json manifest.json\n{"date":"2026-11-09"}\n```\n'

    def test_unchanged_skips_post(self):
        digest = hashlib.sha256(self.markdown.encode("utf-8")).hexdigest()
        with patch.object(module, "request_json", return_value={"date": "2026-11-09", "published": True, "content_hash": digest}) as request:
            self.assertEqual(module.publish("https://bbs.example.com", "token", self.markdown), ("unchanged", "2026-11-09"))
        request.assert_called_once()

    def test_changed_snapshot_posts_once(self):
        with patch.object(module, "request_json", side_effect=[{"date": "2026-11-09", "published": True, "content_hash": "old"}, {"result": "updated", "date": "2026-11-09"}]) as request:
            self.assertEqual(module.publish("https://bbs.example.com", "token", self.markdown), ("updated", "2026-11-09"))
        self.assertEqual(request.call_count, 2)

    def test_unpublished_snapshot_creates(self):
        with patch.object(module, "request_json", side_effect=[{"date": "2026-11-09", "published": False}, {"result": "created", "date": "2026-11-09"}]):
            self.assertEqual(module.publish("https://bbs.example.com", "token", self.markdown), ("created", "2026-11-09"))

    def test_business_failure_never_succeeds(self):
        with patch.object(module, "request_json", side_effect=[{"date": "2026-11-09", "published": False}, module.UploadError("success=false")]):
            with self.assertRaises(module.UploadError):
                module.publish("https://bbs.example.com", "token", self.markdown)

    def test_status_failure_does_not_upload(self):
        with patch.object(module, "request_json", return_value={"date": "2026-11-09"}) as request:
            with self.assertRaises(module.UploadError):
                module.publish("https://bbs.example.com", "token", self.markdown)
        request.assert_called_once()


if __name__ == "__main__":
    unittest.main()
