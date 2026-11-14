import importlib.util
import hashlib
import unittest
from pathlib import Path
from unittest.mock import patch


spec = importlib.util.spec_from_file_location("push_kalshi_report", Path(__file__).with_name("push_kalshi_report.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class PublishTests(unittest.TestCase):
    markdown = '# Report\n\n```json manifest.json\n{"date":"2026-11-09","metadata":{"run_slot":"morning"},"publication_ready":true}\n```\n'

    def test_markdown_limit_matches_server(self):
        self.assertEqual(module.MAX_MARKDOWN_BYTES, 16 * 1024 * 1024)

    def status(self, published=False, content_hash=None):
        return {"date": "2026-11-09", "slot": "morning", "published": published, **({"content_hash": content_hash} if published else {})}

    def test_unchanged_skips_post(self):
        digest = hashlib.sha256(self.markdown.encode("utf-8")).hexdigest()
        with patch.object(module, "request_json", return_value=self.status(True, digest)) as request:
            self.assertEqual(module.publish("https://bbs.example.com", "token", self.markdown), ("unchanged", "2026-11-09", "morning"))
        request.assert_called_once()
        self.assertIn("slot=morning", request.call_args.args[0])

    def test_changed_snapshot_posts_once(self):
        digest = hashlib.sha256(self.markdown.encode()).hexdigest()
        with patch.object(module, "request_json", side_effect=[self.status(True, "0" * 64), {"result": "updated", "date": "2026-11-09", "slot": "morning", "content_hash": digest}, self.status(True, digest)]) as request:
            self.assertEqual(module.publish("https://bbs.example.com", "token", self.markdown), ("updated", "2026-11-09", "morning"))
        self.assertEqual(request.call_count, 3)

    def test_unpublished_snapshot_creates(self):
        digest = hashlib.sha256(self.markdown.encode()).hexdigest()
        with patch.object(module, "request_json", side_effect=[self.status(), {"result": "created", "date": "2026-11-09", "slot": "morning", "content_hash": digest}, self.status(True, digest)]):
            self.assertEqual(module.publish("https://bbs.example.com", "token", self.markdown), ("created", "2026-11-09", "morning"))

    def test_business_failure_never_succeeds(self):
        with patch.object(module, "request_json", side_effect=[self.status(), module.UploadError("success=false")]):
            with self.assertRaises(module.UploadError):
                module.publish("https://bbs.example.com", "token", self.markdown)

    def test_status_failure_does_not_upload(self):
        with patch.object(module, "request_json", return_value={"date": "2026-11-09"}) as request:
            with self.assertRaises(module.UploadError):
                module.publish("https://bbs.example.com", "token", self.markdown)
        request.assert_called_once()

    def test_reject_wrong_slot_or_post_upload_hash(self):
        digest = hashlib.sha256(self.markdown.encode()).hexdigest()
        with patch.object(module, "request_json", return_value={"date": "2026-11-09", "slot": "evening", "published": False}):
            with self.assertRaises(module.UploadError):
                module.publish("https://bbs.example.com", "token", self.markdown)
        with patch.object(module, "request_json", side_effect=[self.status(), {"result": "created", "date": "2026-11-09", "slot": "morning", "content_hash": digest}, self.status()]):
            with self.assertRaisesRegex(module.UploadError, "post-upload status"):
                module.publish("https://bbs.example.com", "token", self.markdown)

    def test_invalid_manifest_never_queries(self):
        with patch.object(module, "request_json") as request:
            for source in (self.markdown.replace('"morning"', '"unknown"'), self.markdown.replace('"publication_ready":true', '"publication_ready":false')):
                with self.assertRaises(module.UploadError):
                    module.publish("https://bbs.example.com", "token", source)
        request.assert_not_called()


if __name__ == "__main__":
    unittest.main()
