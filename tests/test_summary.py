import tempfile
import unittest
from datetime import date
from pathlib import Path

from ai_bricklaying.models import AgentTarget, SessionRecord, SessionSource, SummaryConfig
from ai_bricklaying.summarizer import build_summary


class SummaryTests(unittest.TestCase):
    def test_summary_includes_source_and_template(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            target = AgentTarget("OpenCode", root / "skills", "model")
            source = SessionSource("opencode", "OpenCode", tuple(), "NOPE")
            config = SummaryConfig(target, (source,), "Korean", ("file",), "daily", root)
            record = SessionRecord("OpenCode", root / "session.md", "We implemented a CLI and verified tests for the session summary skill.")

            summary = build_summary(config, [record])

            self.assertIn(f"AI Bricklaying Daily Summary - {date.today().isoformat()}", summary)
            self.assertIn("OpenCode: 1 session artifact", summary)
            self.assertIn("Write the summary in Korean", summary)


if __name__ == "__main__":
    unittest.main()
