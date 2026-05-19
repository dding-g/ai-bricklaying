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
    metadata_path = config.output_dir / "ai-bricklaying-summary-skill.json"
    config_path = (config.config_dir / "config.json") if config.config_dir else (Path.home() / ".config" / "ai-bricklaying" / "config.json")
    slack_payload = config.output_dir / "ai-bricklaying-slack-payload.json"
    slack_payload_line = f"- Slack payload file: `{slack_payload}`" if "slack-webhook" in config.output_modes else "- Slack payload file: not generated unless `slack-webhook` is selected"
    delivery_modes = ", ".join(config.output_modes)
    delivery_instructions = [
        f"- `file`: always save the final markdown summary under `{config.output_dir}` and report the saved path. Do not substitute the agent automation workspace unless the user explicitly asks for a different directory."
    ]
    if "gmail-mcp" in config.output_modes:
        delivery_instructions.append("- `gmail-mcp`: when the CLI result includes this mode, prepare or send the saved markdown summary through Gmail MCP using the configured recipient and subject. If recipient, subject, or authorization is missing, report the exact missing requirement instead of guessing.")
    if "slack-webhook" in config.output_modes:
        delivery_instructions.append(f"- `slack-webhook`: when the CLI result includes this mode, read `{slack_payload}` and post each entry in `messages` to the saved webhook from `{config_path}`. Send the JSON blocks, not the raw Markdown text, so headings and lists render as Slack Block Kit. Report any missing webhook URL instead of exposing or inventing secrets.")
    if list(config.output_modes) == ["file"]:
        delivery_instructions.append("- File-only mode: do not attempt Gmail, Slack, or any external delivery unless the user explicitly asks for a new delivery mode later.")
    slack_payload_contract = """
Slack payload content must mirror the saved Markdown summary:

- Treat the saved Markdown file as the source of truth for Slack delivery.
- Generate or update `ai-bricklaying-slack-payload.json` from the saved Markdown content after the Markdown file is finalized.
- Do not send a shortened, rewritten, or separately summarized Slack version unless the user explicitly asks for a Slack-specific summary.
- Preserve all Markdown sections and bullets in order. Convert headings and lists into Slack Block Kit `blocks`; split long sections only to satisfy Slack block length limits.
- Before posting, verify that the Slack payload covers every top-level section in the saved Markdown summary.
""" if "slack-webhook" in config.output_modes else ""
    return f"""---
name: {config.skill_name}
description: Summarize today's AI coding agent sessions into a useful compound-engineering briefing for the user.
---

# {config.skill_name}

Use this skill when the user asks for a daily summary of AI coding work, session history, agent activity, or compound-engineering learnings.

## Sources

Default session sources: {source_names or "none selected"}.

## Output Locations

- Summary directory: `{config.output_dir}`
- Metadata file: `{metadata_path}`
- Config file: `{config_path}`
{slack_payload_line}

Use the summary directory above for final markdown files created by this skill. The configured CLI output directory is part of this skill's contract.

## CLI Result Delivery Modes

This skill was generated from the CLI result with delivery modes: {delivery_modes}.

Follow the CLI-selected delivery modes exactly:

{chr(10).join(delivery_instructions)}
{slack_payload_contract}

## Workflow

1. Gather today's session history from the selected agents.
2. Identify actual work completed, decisions made, verification evidence, failed attempts, and reusable lessons.
3. Write the result in {config.language}.
4. Apply the CLI result delivery modes above: save the final markdown summary under the configured summary directory, then optionally deliver through Gmail MCP or Slack webhook only when those modes were selected. For Slack delivery, generate the Block Kit payload from the saved Markdown summary so Slack receives the same content in Slack-native formatting.
5. Report saved files and delivery outcomes without printing secrets.

## Summary Template

{render_template(config.language)}
"""


def write_summary_file(config: SummaryConfig, markdown: str) -> Path:
    config.output_dir.mkdir(parents=True, exist_ok=True)
    path = config.output_dir / "ai-bricklaying-summary-skill.md"
    path.write_text(markdown, encoding="utf-8")
    return path
