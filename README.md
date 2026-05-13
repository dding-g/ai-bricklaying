# ai-bricklaying

<p align="center">
  <img src="assets/ai-bricklaying.png" alt="drawing" style="width:400px;"/>
</p>

`ai-bricklaying` is a small prompt-based CLI for creating a reusable AI session summary skill brief. It guides the user through English prompts, lets the user choose the result language, saves a Markdown skill file, and records optional Gmail MCP or Slack webhook delivery details.

## What The CLI Collects

- Target AI agents/models where generated skills should be saved. Multiple targets can be selected.
- One AI agent whose sessions should be summarized: OpenCode, Claude Code, Codex, Cursor, GitHub Copilot.
- Result language for the summary output.
- A default English summary template focused on work done, learnings, results, improvements, and compound engineering philosophy.
- Delivery choices: file save is always enabled; Gmail MCP and Slack webhook are optional.
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

The npm package now runs through a native Node.js CLI. The Python package remains available for Python development and tests, but npm users no longer need Python at runtime.

## Run Locally

Use the Node/npm entrypoint for local CLI usage:

```bash
node bin/ai-bricklaying.js --help
```

## Interactive Flow

```bash
ai-bricklaying
```

The CLI uses a terminal-first wizard with checkbox-style choices, keyboard navigation in TTY sessions, comma-separated fallback prompts when piped, `NO_COLOR` support, and no ANSI color when output is redirected. It writes:

- `YYYY-MM-DD-{title}.md` in the selected output directory. The default is `~/ai-bricklaying`.
- `ai-bricklaying-summary-skill.json` metadata in the selected output directory.
- `ai-bricklaying-slack-payload.json` Slack mrkdwn payload when Slack webhook delivery is selected.
- `SKILL.md` inside `<selected skill directory>/<skill-name>/`.

## Non-Interactive Example

```bash
ai-bricklaying \
  --non-interactive \
  --target-agent opencode,codex \
  --sources opencode \
  --language Korean \
  --output-modes gmail-mcp,slack-webhook \
  --gmail-recipient team@example.com \
  --gmail-subject "AI session summary" \
  --slack-webhook-url "https://hooks.slack.com/services/..." \
  --output-dir /tmp/ai-bricklaying-demo/out \
  --skill-dir /tmp/ai-bricklaying-demo/skills
```

The same flags work through npm:

```bash
npx ai-bricklaying --non-interactive --output-dir /tmp/ai-bricklaying-demo/out --skill-dir /tmp/ai-bricklaying-demo/skills
```

For non-interactive runs, `--target-agent` accepts a comma-separated list of skill targets, while `--sources` accepts exactly one summary source.

Use `--output-dir` to choose where file-save artifacts are written. If omitted, the CLI writes to `~/ai-bricklaying`.

To install the generated skill into OpenCode, point `--skill-dir` at OpenCode's user skills directory:

```bash
ai-bricklaying \
  --non-interactive \
  --target-agent opencode \
  --sources opencode \
  --language Korean \
  --skill-name daily-ai-session-summary \
  --skill-dir ~/.config/opencode/skills
```

After this runs, the skill file should exist at `~/.config/opencode/skills/daily-ai-session-summary/SKILL.md`.

`--skill-name` must be a path-safe lowercase slug. This keeps generated skills under the selected `--skill-dir`.

The Slack webhook URL is saved in `~/.config/ai-bricklaying/config.json` by default. Use `--config-dir` to override that location in tests or automation.

When Slack webhook delivery is selected, the CLI also writes `ai-bricklaying-slack-payload.json` by converting the full Markdown summary into Slack Block Kit JSON with `markdown-to-slack-blocks`. Large summaries are split into a `messages` array so senders can post every batch instead of dropping content.

When the target agent is OpenCode, the generated skill is saved under the selected OpenCode skills directory. OpenCode loads skills at session startup, so restart OpenCode or open a new session if the skill does not appear immediately.

For npm development, run the native Node CLI directly:

```bash
node bin/ai-bricklaying.js --help
```

For Python development, the Python module can still be run with `python3 -m ai_bricklaying.cli`.

## Test

```bash
npm test
npm pack --dry-run --json
```

The npm CLI runs on Node.js. The Python package and tests intentionally continue to use only the Python standard library.
