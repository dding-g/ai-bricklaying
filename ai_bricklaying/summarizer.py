from __future__ import annotations

from collections import Counter, defaultdict
from datetime import date

from .models import SessionRecord, SummaryConfig
from .template import render_template


KEYWORDS = (
    "implement", "fix", "debug", "test", "review", "plan", "refactor", "error",
    "learn", "decide", "ship", "build", "verify", "prompt", "skill", "session",
)


def build_summary(config: SummaryConfig, records: list[SessionRecord]) -> str:
    grouped: dict[str, list[SessionRecord]] = defaultdict(list)
    for record in records:
        grouped[record.source].append(record)

    lines = [
        f"# AI Bricklaying Daily Summary - {date.today().isoformat()}",
        "",
        f"Language: {config.language}",
        f"Target skill: {config.skill_name}",
        f"Target agent/model: {config.target.name} ({config.target.model_hint})",
        "",
        "## Source Coverage",
    ]
    if grouped:
        for source, source_records in grouped.items():
            lines.append(f"- {source}: {len(source_records)} session artifact(s)")
    else:
        lines.append("- No session artifacts were found for today. The generated skill still includes the reusable summary template and follow-up workflow.")

    lines.extend(["", "## Extractive Session Notes"])
    if grouped:
        for source, source_records in grouped.items():
            lines.extend(["", f"### {source}"])
            for record in source_records:
                lines.append(f"- `{record.path}`")
                for sentence in _important_sentences(record.text)[:5]:
                    lines.append(f"  - {sentence}")
    else:
        lines.append("No local session snippets were available. Configure source directories with AI_BRICKLAYING_*_DIRS if your tools store history elsewhere.")

    lines.extend([
        "",
        "## Summary Template For AI Agent",
        "",
        render_template(config.language),
        "",
        "## Delivery Notes",
    ])
    lines.append("- File save: enabled")
    if "gmail-mcp" in config.output_modes:
        recipient = config.gmail_recipient or "not provided"
        subject = config.gmail_subject or "not provided"
        lines.append(f"- Gmail MCP: prepare an email draft for {recipient} with subject {subject}")
    if "slack-mcp" in config.output_modes:
        channel = config.slack_channel or "not provided"
        thread = config.slack_thread or "not provided"
        lines.append(f"- Slack MCP: prepare a message for {channel}; thread {thread}")
    return "\n".join(lines).strip() + "\n"


def _important_sentences(text: str) -> list[str]:
    candidates = [part.strip().replace("\n", " ") for part in text.replace("?", ".").replace("!", ".").split(".")]
    scored: list[tuple[int, str]] = []
    for sentence in candidates:
        if len(sentence) < 40:
            continue
        lowered = sentence.lower()
        score = sum(1 for keyword in KEYWORDS if keyword in lowered)
        if score:
            scored.append((score, sentence[:300]))
    if not scored:
        words = Counter(word.lower().strip(".,:;()[]{}") for word in text.split())
        common = {word for word, count in words.most_common(25) if len(word) > 5 and count > 1}
        for sentence in candidates:
            score = sum(1 for word in common if word in sentence.lower())
            if score and len(sentence) >= 40:
                scored.append((score, sentence[:300]))
    return [sentence for _, sentence in sorted(scored, key=lambda item: item[0], reverse=True)]
