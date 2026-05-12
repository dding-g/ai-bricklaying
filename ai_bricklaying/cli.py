from __future__ import annotations

import argparse
import json
import re
from dataclasses import replace
from pathlib import Path

from .models import SummaryConfig
from .session_sources import SESSION_SOURCES, collect_today_sessions
from .skill_writer import write_skill, write_summary_file
from .summarizer import build_summary
from .tui import AGENT_TARGETS, choose_many, choose_one


def _slug(value: str) -> str:
    return value.lower().replace(" ", "-")


TARGET_BY_KEY = {_slug(target.name): target for target in AGENT_TARGETS}
SOURCE_BY_KEY = {source.key: source for source in SESSION_SOURCES}
OUTPUT_MODES = ("file", "gmail-mcp", "slack-mcp")
SKILL_NAME_PATTERN = re.compile(r"^[a-z0-9][a-z0-9._-]*$")


def main(argv: list[str] | None = None) -> int:
    return _run(argv)


def run(argv: list[str] | None = None) -> int:
    try:
        return _run(argv)
    except SystemExit as error:
        return error.code if isinstance(error.code, int) else 2


def _run(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Summarize today's AI agent sessions and generate a reusable skill.")
    parser.add_argument("--non-interactive", action="store_true", help="Use defaults without prompting.")
    parser.add_argument("--target-agent", choices=sorted(TARGET_BY_KEY), help="Target AI agent/model where the skill is saved.")
    parser.add_argument("--target-model", default="configured model", help="Model label to record in generated artifacts.")
    parser.add_argument("--sources", "--sessions", dest="sources", help="Comma-separated session sources to summarize: opencode, claude-code, codex, cursor, github-copilot.")
    parser.add_argument("--language", default="English", help="Language for the generated summary.")
    parser.add_argument("--output-modes", "--delivery", dest="output_modes", default="file", help="Comma-separated delivery modes: file, gmail-mcp, slack-mcp.")
    parser.add_argument("--skill-name", default="daily-ai-session-summary", help="Generated skill directory name.")
    parser.add_argument("--skill-dir", help="Directory where the generated skill folder should be written.")
    parser.add_argument("--output-dir", default="summaries", help="Directory for generated summary files.")
    parser.add_argument("--gmail-recipient", "--gmail-to", dest="gmail_recipient", help="Gmail MCP recipient to record in delivery notes.")
    parser.add_argument("--gmail-subject", help="Gmail MCP subject to record in delivery notes.")
    parser.add_argument("--slack-channel", help="Slack MCP channel to record in delivery notes.")
    parser.add_argument("--slack-thread", help="Optional Slack MCP thread timestamp to record in delivery notes.")
    args = parser.parse_args(argv)

    if args.non_interactive:
        target = _target_from_args(args)
        sources = _sources_from_args(args)
        output_modes = _output_modes_from_args(args)
        gmail_recipient = args.gmail_recipient
        gmail_subject = args.gmail_subject
        slack_channel = args.slack_channel
        slack_thread = args.slack_thread
    else:
        target = choose_one("1. Select AI agent/model target for the generated skill", AGENT_TARGETS, lambda item: f"{item.name} - {item.default_skill_dir}")
        sources = choose_many("2. Select AI agents whose sessions should be summarized")
        language = input(f"\n3. Result language [{args.language}]: ").strip() or args.language
        args.language = language
        print("\n4. Default summary template will be embedded in English and instructed to output your chosen language.")
        output_modes = _choose_output_modes()
        gmail_recipient = input("Gmail MCP recipient (optional): ").strip() or None if "gmail-mcp" in output_modes else None
        gmail_subject = input("Gmail MCP subject (optional): ").strip() or None if "gmail-mcp" in output_modes else None
        slack_channel = input("Slack MCP channel (optional): ").strip() or None if "slack-mcp" in output_modes else None
        slack_thread = input("Slack MCP thread timestamp (optional): ").strip() or None if "slack-mcp" in output_modes else None

    if args.skill_dir:
        target = replace(target, default_skill_dir=Path(args.skill_dir))
    _validate_skill_name(args.skill_name)

    config = SummaryConfig(
        target=target,
        selected_sources=sources,
        language=args.language,
        output_modes=output_modes,
        skill_name=args.skill_name,
        output_dir=Path(args.output_dir),
        gmail_recipient=gmail_recipient,
        gmail_subject=gmail_subject,
        slack_channel=slack_channel,
        slack_thread=slack_thread,
    )
    records = []
    for source in sources:
        records.extend(collect_today_sessions(source))
    summary = build_summary(config, records)
    summary_path = write_summary_file(config, summary)
    metadata_path = write_metadata_file(config, summary_path, len(records))
    skill_path = write_skill(config)

    print(f"\nSaved summary: {summary_path}")
    print(f"Saved metadata: {metadata_path}")
    print(f"Saved skill: {skill_path}")
    if "gmail-mcp" in output_modes:
        print("Gmail delivery selected: use your Gmail MCP to send the saved markdown content.")
    if "slack-mcp" in output_modes:
        print("Slack delivery selected: use your Slack MCP to post the saved markdown content.")
    return 0


def write_metadata_file(config: SummaryConfig, summary_path: Path, session_count: int) -> Path:
    config.output_dir.mkdir(parents=True, exist_ok=True)
    path = config.output_dir / "ai-bricklaying-summary-skill.json"
    payload = {
        "target_agent": config.target.name,
        "target_model": config.target.model_hint,
        "sessions": [source.key for source in config.selected_sources],
        "language": config.language,
        "deliveries": list(config.output_modes),
        "summary_path": str(summary_path),
        "skill_dir": str(config.target.default_skill_dir / config.skill_name),
        "session_count": session_count,
        "gmail_recipient": config.gmail_recipient,
        "gmail_subject": config.gmail_subject,
        "slack_channel": config.slack_channel,
        "slack_thread": config.slack_thread,
    }
    path.write_text(json.dumps(payload, indent=2, sort_keys=True), encoding="utf-8")
    return path


def _choose_output_modes() -> tuple[str, ...]:
    print("\n5. Select output modes. File save is always enabled.")
    print("  1. only file save")
    print("  2. Gmail MCP")
    print("  3. Slack MCP")
    answer = input("Select comma-separated numbers [1]: ").strip()
    modes = {"file"}
    if "2" in answer:
        modes.add("gmail-mcp")
    if "3" in answer:
        modes.add("slack-mcp")
    return tuple(sorted(modes))


def _csv(value: str | None) -> tuple[str, ...]:
    if not value:
        return tuple()
    return tuple(part.strip() for part in value.split(",") if part.strip())


def _target_from_args(args: argparse.Namespace):
    if not args.target_agent:
        return replace(AGENT_TARGETS[0], model_hint=args.target_model)
    return replace(TARGET_BY_KEY[args.target_agent], model_hint=args.target_model)


def _sources_from_args(args: argparse.Namespace):
    keys = tuple(_source_key(key) for key in _csv(args.sources))
    if not keys:
        return tuple()
    unknown = [key for key in keys if key not in SOURCE_BY_KEY]
    if unknown:
        raise SystemExit(f"Unknown source(s): {', '.join(unknown)}")
    return tuple(SOURCE_BY_KEY[key] for key in keys)


def _output_modes_from_args(args: argparse.Namespace) -> tuple[str, ...]:
    modes = {"file", *(_output_mode_key(mode) for mode in _csv(args.output_modes))}
    unknown = sorted(modes.difference(OUTPUT_MODES))
    if unknown:
        raise SystemExit(f"Unknown output mode(s): {', '.join(unknown)}")
    if "gmail-mcp" in modes and (not args.gmail_recipient or not args.gmail_subject):
        raise SystemExit("gmail-mcp requires --gmail-recipient and --gmail-subject")
    if "slack-mcp" in modes and not args.slack_channel:
        raise SystemExit("slack-mcp requires --slack-channel")
    return tuple(mode for mode in OUTPUT_MODES if mode in modes)


def _source_key(value: str) -> str:
    return value


def _output_mode_key(value: str) -> str:
    aliases = {"gmail": "gmail-mcp", "slack": "slack-mcp"}
    return aliases.get(value, value)


def _validate_skill_name(value: str) -> None:
    if not SKILL_NAME_PATTERN.fullmatch(value) or ".." in value:
        raise SystemExit("--skill-name must be a path-safe slug using lowercase letters, numbers, dots, underscores, or hyphens")


if __name__ == "__main__":
    raise SystemExit(main())
