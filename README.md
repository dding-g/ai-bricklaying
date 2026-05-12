# ai-bricklaying

![ai-bricklaying logo](assets/ai-bricklaying.png)

`ai-bricklaying` is a small prompt-based CLI for creating a reusable AI session summary skill brief. It guides the user through English prompts, lets the user choose the result language, saves a Markdown skill file, and records optional Gmail MCP or Slack MCP delivery details.

## What The CLI Collects

- Target AI agent/model where the generated skill should be saved.
- AI agents whose sessions should be summarized: OpenCode, Claude Code, Codex, Cursor, GitHub Copilot.
- Result language for the summary output.
- A default English summary template focused on work done, learnings, results, improvements, and compound engineering philosophy.
- Delivery choices: file save is always enabled; Gmail MCP and Slack MCP are optional.
- Integration details needed for selected optional delivery channels.

## Install From npm

```bash
npm install -g ai-bricklaying
ai-bricklaying --help
```

Or run without installing:

```bash
npx ai-bricklaying --help
```

The npm package is a thin Node.js launcher around the same Python stdlib CLI. It requires Python 3.10+ to be available as `python3`, `python`, or through `PYTHON=/path/to/python`.

## Run From Source

```bash
python3 -m ai_bricklaying.cli --help
```

For editable install with the console script:

```bash
python3 -m pip install -e .
ai-bricklaying --help
```

## Interactive Flow

```bash
python3 -m ai_bricklaying.cli
```

The CLI asks each question in English with Korean context, then writes:

- `ai-bricklaying-summary-skill.md` in the selected output directory.
- `ai-bricklaying-summary-skill.json` metadata in the selected output directory.
- `SKILL.md` inside `<selected skill directory>/<skill-name>/`.

## Non-Interactive Example

```bash
python3 -m ai_bricklaying.cli \
  --non-interactive \
  --target-agent opencode \
  --sources opencode,claude-code,codex \
  --language Korean \
  --output-modes gmail-mcp,slack-mcp \
  --gmail-recipient team@example.com \
  --gmail-subject "AI session summary" \
  --slack-channel "#engineering" \
  --output-dir /tmp/ai-bricklaying-demo/out \
  --skill-dir /tmp/ai-bricklaying-demo/skills
```

The same flags work through npm:

```bash
npx ai-bricklaying --non-interactive --output-dir /tmp/ai-bricklaying-demo/out --skill-dir /tmp/ai-bricklaying-demo/skills
```

`--skill-name` must be a path-safe lowercase slug. This keeps generated skills under the selected `--skill-dir`.

## Test

```bash
python3 -m unittest discover -s tests
npm pack --dry-run --json
```

The project intentionally uses only the Python standard library for the CLI and tests.
