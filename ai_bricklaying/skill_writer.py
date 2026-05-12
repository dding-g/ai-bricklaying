from __future__ import annotations

from pathlib import Path

from .models import SummaryConfig
from .template import render_template


def write_skill(config: SummaryConfig) -> Path:
    skill_dir = config.target.default_skill_dir / config.skill_name
    skill_dir.mkdir(parents=True, exist_ok=True)
    skill_file = skill_dir / "SKILL.md"
    skill_file.write_text(_skill_markdown(config), encoding="utf-8")
    return skill_file


def _skill_markdown(config: SummaryConfig) -> str:
    source_names = ", ".join(source.label for source in config.selected_sources)
    return f"""---
name: {config.skill_name}
description: Summarize today's AI coding agent sessions into a useful compound-engineering briefing for the user.
---

# {config.skill_name}

Use this skill when the user asks for a daily summary of AI coding work, session history, agent activity, or compound-engineering learnings.

## Sources

Default session sources: {source_names or "none selected"}.

## Workflow

1. Gather today's session history from the selected agents.
2. Identify actual work completed, decisions made, verification evidence, failed attempts, and reusable lessons.
3. Write the result in {config.language}.
4. Save a markdown file even if the user also asks to send the result through Gmail or Slack.
5. If Gmail MCP or Slack MCP delivery is requested, prepare the message through the available MCP integration and clearly report any missing recipient, channel, or authorization.

## Summary Template

{render_template(config.language)}
"""


def write_summary_file(config: SummaryConfig, markdown: str) -> Path:
    config.output_dir.mkdir(parents=True, exist_ok=True)
    path = config.output_dir / "ai-bricklaying-summary-skill.md"
    path.write_text(markdown, encoding="utf-8")
    return path
