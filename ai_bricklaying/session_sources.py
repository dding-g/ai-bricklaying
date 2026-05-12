from __future__ import annotations

import json
import os
from datetime import date, datetime
from pathlib import Path

from .models import SessionRecord, SessionSource


HOME = Path.home()

SESSION_SOURCES: tuple[SessionSource, ...] = (
    SessionSource("opencode", "OpenCode", (HOME / ".local/share/opencode", HOME / ".config/opencode"), "AI_BRICKLAYING_OPENCODE_DIRS"),
    SessionSource("claude-code", "Claude Code", (HOME / ".claude/projects", HOME / ".claude"), "AI_BRICKLAYING_CLAUDE_DIRS"),
    SessionSource("codex", "Codex", (HOME / ".codex/sessions", HOME / ".codex"), "AI_BRICKLAYING_CODEX_DIRS"),
    SessionSource("cursor", "Cursor", (HOME / "Library/Application Support/Cursor/User/workspaceStorage",), "AI_BRICKLAYING_CURSOR_DIRS"),
    SessionSource("github-copilot", "GitHub Copilot", (HOME / "Library/Application Support/Code/User/workspaceStorage",), "AI_BRICKLAYING_COPILOT_DIRS"),
)

TEXT_EXTENSIONS = {".json", ".jsonl", ".md", ".txt", ".log"}


def source_paths(source: SessionSource) -> list[Path]:
    configured = os.environ.get(source.env_var)
    if configured:
        return [Path(part).expanduser() for part in configured.split(os.pathsep) if part]
    return list(source.default_paths)


def collect_today_sessions(source: SessionSource, today: date | None = None, limit: int = 12) -> list[SessionRecord]:
    today = today or date.today()
    records: list[SessionRecord] = []
    for root in source_paths(source):
        if not root.exists():
            continue
        for path in sorted(_candidate_files(root), key=lambda item: item.stat().st_mtime, reverse=True):
            if len(records) >= limit:
                return records
            if datetime.fromtimestamp(path.stat().st_mtime).date() != today:
                continue
            text = _read_session_text(path)
            if text:
                records.append(SessionRecord(source=source.label, path=path, text=text))
    return records


def _candidate_files(root: Path) -> list[Path]:
    if root.is_file():
        return [root] if root.suffix.lower() in TEXT_EXTENSIONS else []
    files: list[Path] = []
    for path in root.rglob("*"):
        if path.is_file() and path.suffix.lower() in TEXT_EXTENSIONS:
            files.append(path)
    return files


def _read_session_text(path: Path, max_chars: int = 20_000) -> str:
    try:
        raw = path.read_text(encoding="utf-8", errors="ignore")[:max_chars]
    except OSError:
        return ""
    if not raw.strip():
        return ""
    if path.suffix.lower() == ".jsonl":
        return _jsonl_to_text(raw)
    if path.suffix.lower() == ".json":
        return _json_to_text(raw)
    return raw.strip()


def _jsonl_to_text(raw: str) -> str:
    lines: list[str] = []
    for line in raw.splitlines():
        try:
            payload = json.loads(line)
        except json.JSONDecodeError:
            if line.strip():
                lines.append(line.strip())
            continue
        lines.extend(_extract_text_values(payload))
    return "\n".join(lines).strip()


def _json_to_text(raw: str) -> str:
    try:
        payload = json.loads(raw)
    except json.JSONDecodeError:
        return raw.strip()
    return "\n".join(_extract_text_values(payload)).strip()


def _extract_text_values(value: object) -> list[str]:
    if isinstance(value, str):
        text = value.strip()
        return [text] if len(text) > 20 else []
    if isinstance(value, list):
        result: list[str] = []
        for item in value:
            result.extend(_extract_text_values(item))
        return result
    if isinstance(value, dict):
        result: list[str] = []
        for key in ("text", "content", "message", "prompt", "response", "summary"):
            if key in value:
                result.extend(_extract_text_values(value[key]))
        return result
    return []
