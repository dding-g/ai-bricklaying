from __future__ import annotations

import curses
from pathlib import Path

from .models import AgentTarget, SessionSource
from .session_sources import SESSION_SOURCES


AGENT_TARGETS: tuple[AgentTarget, ...] = (
    AgentTarget("OpenCode", Path.home() / ".config/opencode/skills", "configured OpenCode model"),
    AgentTarget("Claude Code", Path.home() / ".claude/skills", "configured Claude model"),
    AgentTarget("Codex", Path.home() / ".codex/skills", "configured Codex model"),
    AgentTarget("Cursor", Path.home() / ".cursor/skills", "configured Cursor model"),
    AgentTarget("GitHub Copilot", Path.home() / ".github-copilot/skills", "configured Copilot model"),
)


def choose_one(title: str, options: tuple[object, ...], label=lambda item: str(item)):
    print(f"\n{title}")
    for index, option in enumerate(options, 1):
        print(f"  {index}. {label(option)}")
    while True:
        answer = input("Select number: ").strip()
        if answer.isdigit() and 1 <= int(answer) <= len(options):
            return options[int(answer) - 1]
        print("Please enter a valid number.")


def choose_many(title: str, options: tuple[SessionSource, ...] = SESSION_SOURCES) -> tuple[SessionSource, ...]:
    try:
        return curses.wrapper(_choose_many_curses, title, options)
    except curses.error:
        return _choose_many_line_mode(title, options)


def _choose_many_curses(stdscr, title: str, options: tuple[SessionSource, ...]) -> tuple[SessionSource, ...]:
    selected: set[int] = set(range(len(options)))
    cursor = 0
    curses.curs_set(0)
    while True:
        stdscr.clear()
        stdscr.addstr(0, 0, title)
        stdscr.addstr(1, 0, "Space toggles, Enter confirms, arrows move.")
        for index, option in enumerate(options):
            prefix = ">" if index == cursor else " "
            marker = "[x]" if index in selected else "[ ]"
            stdscr.addstr(index + 3, 0, f"{prefix} {marker} {option.label}")
        key = stdscr.getch()
        if key in (curses.KEY_UP, ord("k")):
            cursor = max(0, cursor - 1)
        elif key in (curses.KEY_DOWN, ord("j")):
            cursor = min(len(options) - 1, cursor + 1)
        elif key == ord(" "):
            if cursor in selected:
                selected.remove(cursor)
            else:
                selected.add(cursor)
        elif key in (10, 13):
            return tuple(options[index] for index in sorted(selected))


def _choose_many_line_mode(title: str, options: tuple[SessionSource, ...]) -> tuple[SessionSource, ...]:
    print(f"\n{title}")
    for index, option in enumerate(options, 1):
        print(f"  {index}. {option.label}")
    answer = input("Select comma-separated numbers, or press Enter for all: ").strip()
    if not answer:
        return options
    selected = []
    for part in answer.split(","):
        number = part.strip()
        if number.isdigit() and 1 <= int(number) <= len(options):
            selected.append(options[int(number) - 1])
    return tuple(selected) or options
