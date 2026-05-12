import tempfile
import unittest
from pathlib import Path

from ai_bricklaying.cli import main


class CliTests(unittest.TestCase):
    def test_non_interactive_flags_write_only_to_requested_dirs(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            output_dir = root / "out"
            skill_dir = root / "skills"
            skill_name = "test-ai-session-summary"

            exit_code = main(
                [
                    "--non-interactive",
                    "--target-agent",
                    "opencode",
                    "--sources",
                    "opencode,claude-code,codex",
                    "--language",
                    "Korean",
                    "--output-modes",
                    "gmail-mcp,slack-mcp",
                    "--gmail-recipient",
                    "team@example.com",
                    "--gmail-subject",
                    "AI session summary",
                    "--slack-channel",
                    "#engineering",
                    "--slack-thread",
                    "123.456",
                    "--skill-name",
                    skill_name,
                    "--output-dir",
                    str(output_dir),
                    "--skill-dir",
                    str(skill_dir),
                ]
            )

            self.assertEqual(exit_code, 0)
            summary_path = output_dir / "ai-bricklaying-summary-skill.md"
            metadata_path = output_dir / "ai-bricklaying-summary-skill.json"
            skill_path = skill_dir / skill_name / "SKILL.md"
            self.assertTrue(summary_path.exists())
            self.assertTrue(metadata_path.exists())
            self.assertTrue(skill_path.exists())
            self.assertFalse((Path.home() / f".config/opencode/skills/{skill_name}/SKILL.md").exists())
            summary = summary_path.read_text(encoding="utf-8")
            self.assertIn("Gmail MCP: prepare an email draft for team@example.com with subject AI session summary", summary)
            self.assertIn("Slack MCP: prepare a message for #engineering; thread 123.456", summary)
            self.assertIn("Work Completed", summary)
            self.assertIn("Lessons Learned", summary)
            self.assertIn("Results And Evidence", summary)
            self.assertIn("Improvement Backlog", summary)
            self.assertIn("Compound Engineering Notes", summary)

    def test_non_interactive_rejects_missing_slack_details(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            with self.assertRaises(SystemExit) as raised:
                main(
                    [
                        "--non-interactive",
                        "--output-modes",
                        "slack-mcp",
                        "--output-dir",
                        str(root / "out"),
                        "--skill-dir",
                        str(root / "skills"),
                    ]
                )

            self.assertEqual(str(raised.exception), "slack-mcp requires --slack-channel")

    def test_skill_name_must_be_path_safe(self):
        unsafe_names = ("../escape", "/tmp/escape", "nested/skill", "NestedSkill")
        for skill_name in unsafe_names:
            with self.subTest(skill_name=skill_name), tempfile.TemporaryDirectory() as temp_dir:
                root = Path(temp_dir)
                with self.assertRaises(SystemExit) as raised:
                    main(
                        [
                            "--non-interactive",
                            "--skill-name",
                            skill_name,
                            "--output-dir",
                            str(root / "out"),
                            "--skill-dir",
                            str(root / "skills"),
                        ]
                    )

                self.assertIn("--skill-name must be a path-safe slug", str(raised.exception))
                self.assertFalse((root / "escape").exists())


if __name__ == "__main__":
    unittest.main()
