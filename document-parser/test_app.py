import tempfile
import unittest
from pathlib import Path

from app import ParseError, clean, parse_txt


class ParserTest(unittest.TestCase):
    def test_clean_normalizes_controls_and_lines(self):
        self.assertEqual(clean("a\r\nb\x00  \r"), "a\nb")

    def test_txt_accepts_utf8_bom(self):
        with tempfile.NamedTemporaryFile(delete=False) as target:
            target.write(b"\xef\xbb\xbf# title\r\n\r\nbody")
            path = target.name
        try:
            self.assertEqual(parse_txt(path)[0]["markdown"], "# title\n\nbody")
        finally:
            Path(path).unlink()

    def test_txt_rejects_non_utf8(self):
        with tempfile.NamedTemporaryFile(delete=False) as target:
            target.write(b"\xff")
            path = target.name
        try:
            with self.assertRaises(ParseError):
                parse_txt(path)
        finally:
            Path(path).unlink()


if __name__ == "__main__":
    unittest.main()
