# ai-bricklaying

<p align="center">
  <img src="assets/ai-bricklaying.png" alt="ai-bricklaying logo" style="width:400px;"/>
</p>

`ai-bricklaying` turns AI coding session history into a lightweight daily reflection and installs a reusable skill for the AI tools you use. It is for people who want to remember what they learned, what improved, and how to use AI better next time.

## What It Does

- Summarizes one selected session source: OpenCode, Claude Code, Codex, Cursor, or GitHub Copilot.
- Installs the generated skill into one or more selected AI agent skill directories.
- Saves a lightweight Markdown summary using `YYYY-MM-DD-ai-bricklaying-daily-summary.md`.
- Writes metadata to `ai-bricklaying-summary-skill.json`.
- Optionally prepares Gmail MCP delivery details.
- Optionally creates a Slack-ready payload from the generated summary.
- Stores setup defaults in `~/.config/ai-bricklaying/config.json` so the next run can reuse them.

## Requirements

- npm or npx
- Node.js 18 or newer when you run the npm or npx launcher
- A first-release supported platform: `darwin-arm64`, `darwin-amd64`, `linux-amd64`, or `linux-arm64`

The CLI behavior is implemented by a bundled Go binary. The npm package keeps `bin/ai-bricklaying.js` as a small launcher that selects the matching binary and forwards your command.

## Install

```bash
npm install -g ai-bricklaying
ai-bricklaying --help
```

Or run without installing:

```bash
npx ai-bricklaying --help
```

Check your installed version:

```bash
ai-bricklaying --version
```

If your platform is not one of the bundled first-release targets, the launcher exits with an unsupported platform message instead of falling back to a different implementation.

## Interactive Setup

Run:

```bash
ai-bricklaying
```

The wizard asks for:

1. Target AI agents where the generated skill should be installed. You can select multiple targets.
2. One summary source. This list is limited to the agents selected in step 1, because the summary should come from a tool where the skill is being installed.
3. Summary language.
4. File save directory. The default is `~/ai-bricklaying`.
5. Output modes. File save is always enabled; Gmail MCP and Slack webhook are optional.

When generation completes, the CLI prints the generated skill command in bold:

```text
Use the generated skill: /daily-ai-session-summary
```

If the target is OpenCode and the skill does not appear immediately, restart OpenCode or open a new session. OpenCode loads skills at session startup.

## Outputs

By default, file outputs are written to `~/ai-bricklaying`. Use `--output-dir` to choose another directory.

- `YYYY-MM-DD-ai-bricklaying-daily-summary.md`: lightweight summary focused on takeaways, improvements, and better AI usage.
- `ai-bricklaying-summary-skill.json`: metadata, selected targets, delivery modes, summary path, and generated skill directories.
- `ai-bricklaying-slack-payload.json`: Slack payload when `slack-webhook` is selected.
- `<skill-dir>/<skill-name>/SKILL.md`: generated reusable skill. It embeds the configured summary directory, metadata path, config path, and Slack payload path so later agent runs save summaries to the same output location instead of an agent-local workspace.

`--skill-name` must be a path-safe lowercase slug such as `daily-ai-session-summary`.

## Slack Delivery

Select `slack-webhook` to create a Slack-ready payload. The CLI prepares local files only; it does not call the webhook for you.

```bash
ai-bricklaying \
  --non-interactive \
  --target-agent opencode \
  --sources opencode \
  --output-modes slack-webhook \
  --slack-webhook-url "https://hooks.slack.com/services/..."
```

The CLI does not print the webhook secret after it is saved. Existing webhook config is shown as `[configured]` in prompts.

Slack payload behavior:

- The saved Markdown summary is the source of truth for Slack delivery.
- The Slack payload is generated from the finalized Markdown; it is not shortened or rewritten unless you explicitly ask for a Slack-specific summary.
- Markdown sections and bullets stay in order. Large summaries are split into a `messages` array only to satisfy Slack block length limits.
- The first batch is also exposed as top-level `text` and `blocks` for simple webhook usage. Send `blocks` to render headings and lists properly.
- The payload includes verification metadata showing whether every top-level Markdown section is covered.

## Gmail MCP Delivery

Select `gmail-mcp` when you want the generated skill instructions and summary metadata to include Gmail MCP delivery details. The CLI prepares handoff details only; it does not send email.

```bash
ai-bricklaying \
  --non-interactive \
  --target-agent opencode \
  --sources opencode \
  --output-modes gmail-mcp \
  --gmail-recipient team@example.com \
  --gmail-subject "AI session summary"
```

The generated skill explicitly tells the agent to use Gmail MCP only when this mode was selected.

## Config Defaults

The CLI stores local configuration at:

```text
~/.config/ai-bricklaying/config.json
```

Saved config includes delivery settings and defaults such as target agents, source, language, output modes, skill name, skill directory, and output directory. On the next run, the CLI reads this file and uses those values as defaults.

Command-line flags always override saved config. Use `--config-dir` to use a different config location for tests or automation.

## Keeping It Updated

Update the npm package, including its bundled Go binary, then run the CLI again to refresh the generated skill:

```bash
npm install -g ai-bricklaying@latest
ai-bricklaying
```

The CLI reuses your saved config as defaults, so refreshing usually means accepting the existing prompts and regenerating `SKILL.md`. If you installed the skill into OpenCode, restart OpenCode or open a new session after regenerating it.

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
  --output-dir ~/ai-bricklaying \
  --skill-name daily-ai-session-summary
```

Rules for non-interactive runs:

- `--target-agent` accepts a comma-separated list.
- `--sources` accepts exactly one source.
- `--sources` must be one of the selected target agents.
- `--output-modes` accepts `file`, `gmail-mcp`, and `slack-webhook`; `file` is always enabled.

## Install Into OpenCode

To install directly into OpenCode's user skills directory:

```bash
ai-bricklaying \
  --non-interactive \
  --target-agent opencode \
  --sources opencode \
  --language Korean \
  --skill-name daily-ai-session-summary \
  --skill-dir ~/.config/opencode/skills
```

After this runs, the skill should exist at:

```text
~/.config/opencode/skills/daily-ai-session-summary/SKILL.md
```

Use it as:

```text
/daily-ai-session-summary
```

## CLI Options

```text
--non-interactive                 Use defaults and flags without prompting
--target-agent <agents>           Skill targets: opencode,claude-code,codex,cursor,github-copilot
--target-model <label>            Model label recorded in generated artifacts
--sources, --sessions <source>    Single session source to summarize
--language <language>             Language for the generated summary [English]
--output-modes, --delivery <list> file, gmail-mcp, slack-webhook; file is always enabled
--skill-name <slug>               Generated skill directory name
--skill-dir <dir>                 Directory where the skill folder is written
--output-dir <dir>                Directory for summary files [~/ai-bricklaying]
--gmail-recipient, --gmail-to     Gmail MCP recipient
--gmail-subject <subject>         Gmail MCP subject
--slack-webhook-url <url>         Slack incoming webhook URL
--config-dir <dir>                ai-bricklaying config directory
-v, --version                     Show CLI version
-h, --help                        Show help
```
