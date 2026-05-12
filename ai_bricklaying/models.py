from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class AgentTarget:
    name: str
    default_skill_dir: Path
    model_hint: str


@dataclass(frozen=True)
class SessionSource:
    key: str
    label: str
    default_paths: tuple[Path, ...]
    env_var: str


@dataclass(frozen=True)
class SessionRecord:
    source: str
    path: Path
    text: str


@dataclass(frozen=True)
class SummaryConfig:
    target: AgentTarget
    selected_sources: tuple[SessionSource, ...]
    language: str
    output_modes: tuple[str, ...]
    skill_name: str
    output_dir: Path
    gmail_recipient: str | None = None
    gmail_subject: str | None = None
    slack_channel: str | None = None
    slack_thread: str | None = None
