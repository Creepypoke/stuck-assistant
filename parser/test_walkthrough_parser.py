import json
import unittest
from unittest.mock import patch

import walkthrough_parser


class WalkthroughParserTests(unittest.TestCase):
    def test_main_writes_only_json_to_stdout(self):
        with patch("sys.stdin") as stdin, patch("sys.stdout") as stdout:
            stdin.buffer.read.return_value = b"<html><title>Guide</title><body><p>Step one.</p></body></html>"
            self.assertEqual(walkthrough_parser.main(), 0)

        result = json.loads("".join(call.args[0] for call in stdout.write.call_args_list))
        self.assertEqual(result["title"], "Guide")
        self.assertIn("Step one.", result["markdown"])

    @patch("walkthrough_parser.trafilatura.extract", return_value=None)
    def test_extract_markdown_falls_back_to_visible_text(self, _extract):
        markdown = walkthrough_parser.extract_markdown(
            "<html><body><script>ignore()</script><p>First step.</p><p>Second step.</p></body></html>"
        )

        self.assertEqual(markdown, "First step.\n\nSecond step.")


if __name__ == "__main__":
    unittest.main()
