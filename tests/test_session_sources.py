import os
import tempfile
import unittest
from datetime import date
from pathlib import Path

from ai_bricklaying.models import SessionSource
from ai_bricklaying.session_sources import collect_today_sessions


class SessionSourceTests(unittest.TestCase):
    def test_collect_today_sessions_reads_jsonl(self):
        env_var = "AI_BRICKLAYING_TEST_DIRS"
        original = os.environ.get(env_var)
        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            session_file = root / "session.jsonl"
            session_file.write_text('{"message":"Implemented a useful prompt and verified the CLI behavior."}\n', encoding="utf-8")
            source = SessionSource("test", "Test", tuple(), env_var)
            os.environ[env_var] = str(root)

            try:
                records = collect_today_sessions(source, today=date.today())
            finally:
                if original is None:
                    os.environ.pop(env_var, None)
                else:
                    os.environ[env_var] = original

            self.assertEqual(len(records), 1)
            self.assertIn("Implemented a useful prompt", records[0].text)


if __name__ == "__main__":
    unittest.main()
