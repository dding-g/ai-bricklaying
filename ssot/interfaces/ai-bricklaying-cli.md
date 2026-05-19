---
title: ai-bricklaying CLI Interface
description: 사용자와 automation agent가 ai-bricklaying CLI를 호출할 때의 command, flag, config, stdout/stderr, exit, artifact 계약을 정의한다
ref:
  - ssot/rules.md
  - ssot/index.md
  - ssot/domains/cli/index.md
---

# ai-bricklaying CLI Interface

## 1. Interface 정의

```yaml
interface:
  name: ai-bricklaying
  implementation_language: Node.js
  executable: ai-bricklaying
  package_entrypoint: bin/ai-bricklaying.js
  package_bin_field: package.json#bin.ai-bricklaying
  minimum_node: ">=18"
  invocation_modes:
    - interactive wizard
    - non-interactive flags
    - information flags
  primary_consumers:
    - human developer
    - local automation script
    - AI coding agent that prepares daily reflection artifacts
  source_of_truth:
    - ssot/domains/cli/index.md
    - bin/ai-bricklaying.js
    - tests/cli-node.test.js
```

이 문서는 public npm CLI의 외부 interface 계약을 정의한다. Python package의 `ai_bricklaying.cli`는 regression surface일 수 있지만 npm 사용자가 호출하는 interface 정본은 아니다.

## 2. Command Model

```yaml
commands:
  - command: "ai-bricklaying"
    mode: interactive
    behavior: "wizard로 target agents, summary source, language, output modes, delivery details를 묻고 artifact를 생성한다."
    prompts:
      - "target agents multi-select"
      - "selected target agent 중 summary source 하나"
      - "summary language"
      - "file save directory"
      - "output modes"
      - "gmail/slack details only when selected"

  - command: "ai-bricklaying --non-interactive"
    mode: non_interactive
    behavior: "flags와 saved config defaults만 사용해 prompt 없이 artifact를 생성한다."
    validation_rule: "필수 값 누락이나 enum 오류는 artifact write 전에 exit code 2로 실패한다."

  - command: "ai-bricklaying --help"
    mode: information
    behavior: "지원 flags와 사용법을 stdout에 출력하고 artifact를 쓰지 않는다."

  - command: "ai-bricklaying --version"
    mode: information
    behavior: "package.json version을 stdout에 출력하고 artifact를 쓰지 않는다."
```

## 3. Flags And Aliases

```yaml
flags:
  --non-interactive:
    type: boolean
    default: false
    side_effect: "prompt를 모두 비활성화한다."

  --target-agent:
    type: comma_separated_enum
    values: [opencode, claude-code, codex, cursor, github-copilot]
    default: saved config 또는 opencode
    rule: "여러 target을 선택할 수 있다."
    maps_to: "generated skill install destinations"

  --target-model:
    type: string
    default: "configured model"
    maps_to: "metadata.target_model and generated artifact context"

  --sources:
    aliases: [--sessions]
    type: enum
    values: [opencode, claude-code, codex, cursor, github-copilot]
    default: "selected target agents 중 첫 source"
    rule: "정확히 하나이며, selected target agents 중 하나여야 한다."
    maps_to: "session discovery source"

  --language:
    type: string
    default: English
    maps_to: "summary template instruction and generated skill workflow"

  --output-modes:
    aliases: [--delivery]
    type: comma_separated_enum
    values: [file, gmail-mcp, slack-webhook]
    aliases_for_values:
      gmail: gmail-mcp
      slack: slack-webhook
    invariant: "file은 항상 포함된다."

  --skill-name:
    type: slug
    default: daily-ai-session-summary
    validation: "^[a-z0-9][a-z0-9._-]*$ and must not contain .."
    rule: "lowercase path-safe slug만 허용한다. slash, absolute path, uppercase는 실패한다."

  --skill-dir:
    type: path
    default: "target별 default skill directory"
    behavior: "지정하면 모든 selected target의 skill directory를 같은 directory로 override한다."

  --output-dir:
    type: path
    default: "~/ai-bricklaying"
    behavior: "summary, metadata, optional Slack payload가 저장되는 directory다."

  --gmail-recipient:
    aliases: [--gmail-to]
    type: string
    required_when: "non-interactive gmail-mcp selected and saved config does not provide recipient"

  --gmail-subject:
    type: string
    required_when: "non-interactive gmail-mcp selected and saved config does not provide subject"

  --slack-webhook-url:
    type: secret_url
    required_when: "non-interactive slack-webhook selected and saved config does not provide webhook"
    stdout_rule: "실제 URL은 stdout에 출력하지 않는다. interactive prompt에서는 saved value를 [configured]로 표시하고 blank/configured 입력은 기존 값을 유지한다."

  --config-dir:
    type: path
    default: "~/.config/ai-bricklaying"
    behavior: "defaults와 delivery config를 읽고 쓸 directory다."

  -v:
    alias_for: --version
  --version:
    type: information_flag
  -h:
    alias_for: --help
  --help:
    type: information_flag
```

## 4. Target And Source Catalog

```yaml
target_agents:
  opencode:
    label: OpenCode
    default_skill_dir: "~/.config/opencode/skills"
    default_model_hint: "configured OpenCode model"
  claude-code:
    label: Claude Code
    default_skill_dir: "~/.claude/skills"
    default_model_hint: "configured Claude model"
  codex:
    label: Codex
    default_skill_dir: "~/.codex/skills"
    default_model_hint: "configured Codex model"
  cursor:
    label: Cursor
    default_skill_dir: "~/.cursor/skills"
    default_model_hint: "configured Cursor model"
  github-copilot:
    label: GitHub Copilot
    default_skill_dir: "~/.github-copilot/skills"
    default_model_hint: "configured Copilot model"
```

```yaml
session_sources:
  opencode:
    label: OpenCode
    default_paths:
      - "~/.local/share/opencode"
    env_var: AI_BRICKLAYING_OPENCODE_DIRS
  claude-code:
    label: Claude Code
    default_paths:
      - "~/.claude/projects"
    env_var: AI_BRICKLAYING_CLAUDE_DIRS
  codex:
    label: Codex
    default_paths:
      - "~/.codex/sessions"
      - "~/.codex"
    env_var: AI_BRICKLAYING_CODEX_DIRS
  cursor:
    label: Cursor
    default_paths:
      - "~/Library/Application Support/Cursor/User/workspaceStorage"
    env_var: AI_BRICKLAYING_CURSOR_DIRS
  github-copilot:
    label: GitHub Copilot
    default_paths:
      - "~/Library/Application Support/Code/User/workspaceStorage"
    env_var: AI_BRICKLAYING_COPILOT_DIRS
```

## 5. Config And Environment Inputs

```yaml
config:
  path: "<config-dir>/config.json"
  read_behavior:
    missing_file: "empty config로 처리한다."
    invalid_json: "exit code 2 with read error message"
  write_behavior:
    directory_permission_posix: "0700 best effort"
    file_permission_posix: "0600 best effort"
    contains_secret: true
  defaults:
    target_agents: "string[]; flag 미지정 시 --target-agent default"
    source: "string|null; flag 미지정 시 --sources default"
    target_model: "string"
    language: "string"
    output_modes: "string[]"
    skill_name: "string"
    skill_dir: "string|null; selected targets의 skill dir가 모두 같을 때만 저장"
    output_dir: "string"
    cli_version: "string"
  delivery:
    gmail_recipient: "string|null"
    gmail_subject: "string|null"
    slack_webhook_url: "string|null; secret"
```

```yaml
env_overrides:
  format: "platform path delimiter separated path list"
  expansion: "~ is expanded to user home"
  variables:
    - AI_BRICKLAYING_OPENCODE_DIRS
    - AI_BRICKLAYING_CLAUDE_DIRS
    - AI_BRICKLAYING_CODEX_DIRS
    - AI_BRICKLAYING_CURSOR_DIRS
    - AI_BRICKLAYING_COPILOT_DIRS
```

## 6. Interactive UX Contract

```yaml
interactive:
  tui_mode:
    condition: "stdin/stdout are TTY and stdin.setRawMode exists"
    keys:
      - "Space toggles selection"
      - "Enter confirms"
      - "j/k or arrow keys move cursor"
      - "q, Escape, or Ctrl-C cancels with exit code 2"
    output: "ANSI color may be used unless NO_COLOR disables it."

  line_mode_fallback:
    condition: "TTY wizard is unavailable, including scripted stdin tests"
    target_selection: "prints numbered [x]/[ ] list; blank/default can select all target defaults"
    source_selection: "prints only sources available from selected targets"
    output_modes: "file is shown [x] and required"
    configured_secret_prompt: "existing Slack webhook is shown as [configured], never as raw URL"
```

## 7. Stdout, Stderr, Exit Codes

```yaml
stdout:
  success_human_summary:
    - "AI Bricklaying files generated"
    - "Summary path"
    - "Metadata path"
    - "Config path"
    - "Slack payload path when created"
    - "one Skill path per selected target"
    - "OpenCode restart/new session hint when OpenCode target is selected"
    - "Gmail delivery selected note when gmail-mcp selected"
    - "Slack webhook delivery selected note when slack-webhook selected"
    - "Use the generated skill: /<skill-name>"
    - "To refresh later: npm install -g ai-bricklaying@latest && ai-bricklaying"
  information_flags:
    --help: "help text only"
    --version: "package version only"
  color_policy:
    default: "color allowed on TTY"
    no_color_env: "NO_COLOR suppresses ANSI output"
  sanitation:
    - "control characters are removed from printed paths"
    - "printed paths are not promised to redact secret-looking path segments; users must not put secrets in output, skill, or config paths"
  must_not_include:
    - "Slack webhook secret"
    - "token/password/API key"
    - "raw private session excerpts"

stderr:
  validation_failure: "human-readable CliError message"
  cancellation: "Cancelled."
  unexpected_failure: "Unexpected error: <message>"

exit_codes:
  0: "success or information flag"
  2: "CliError: local validation failure, cancellation, config read error, symlink refusal"
  1: "unexpected runtime failure"
```

## 8. Output Files

```yaml
output_files:
  - path: "<output-dir>/YYYY-MM-DD-ai-bricklaying-daily-summary.md"
    required: true
    filename_rule: "summary H1에서 title을 뽑아 date prefix와 path-safe slug를 만든다. 현재 H1 기준 결과는 날짜별 ai-bricklaying-daily-summary 파일이다."
    mode: "0644 best effort"

  - path: "<output-dir>/ai-bricklaying-summary-skill.json"
    required: true
    mode: "0644 best effort"

  - path: "<output-dir>/ai-bricklaying-slack-payload.json"
    required_when: "slack-webhook selected"
    content_rule: "저장된 Markdown summary 최종본을 source of truth로 사용해 생성하며 별도 Slack short summary로 재작성하지 않는다."
    mode: "0644 best effort"

  - path: "<skill-dir>/<skill-name>/SKILL.md"
    required: true
    multiplicity: "one per selected target unless --skill-dir overrides all targets to same directory"
    content_rule: "generated skill은 이후 agent 실행이 같은 위치에 summary를 저장하도록 configured output directory, metadata path, config path, optional Slack payload path를 포함한다. slack-webhook mode에서는 saved Markdown source-of-truth, Block Kit payload 생성, Slack 전용 short summary 금지, section/bullet order 유지, top-level section coverage 검증 지침을 포함한다."
    mode: "0644 best effort"

  - path: "<config-dir>/config.json"
    required: true
    mode: "0600 best effort"
```

## 9. Artifact Schemas

```yaml
metadata_json:
  config_path: "string"
  deliveries: "string[]"
  gmail_recipient: "string|null"
  gmail_subject: "string|null"
  language: "string"
  cli_version: "string"
  session_count: "number"
  sessions: "source key[]"
  skill_dirs: "string[]"
  slack_webhook_configured: "boolean"
  slack_payload_path: "string|null"
  summary_path: "string"
  target_agents: "display name[]"
  target_model: "string"
```

```yaml
slack_payload_json:
  text: "fallback text; no raw Markdown heading markers"
  blocks: "Block Kit blocks for first batch"
  messages:
    type: "array"
    item:
      text: "fallback text with batch suffix when split"
      blocks: "Block Kit blocks"
  source_of_truth: "saved Markdown summary"
  split_rule: "large summaries may be split by markdown-to-slack-blocks splitBlocksWithText only to satisfy Slack block length limits"
  order_rule: "Markdown sections and bullets stay in source order"
  verification:
    source: "saved_markdown"
    top_level_sections: "string[]"
    covered_top_level_sections: "string[]"
    all_top_level_sections_covered: "boolean"
```

```yaml
generated_skill_frontmatter:
  name: "<skill-name>"
  description: "Summarize today's AI coding agent sessions into a useful compound-engineering briefing for the user."

generated_skill_sections:
  - Sources
  - Output Locations
  - CLI Result Delivery Modes
  - Workflow
  - Summary Template
```

## 10. Agent-Friendly Rules

```yaml
agent_rules:
  validation_first:
    rule: "필수 flag나 enum이 틀리면 artifact write 전에 exit code 2로 실패한다."
  deterministic_paths:
    rule: "automation은 --output-dir, --skill-dir, --config-dir을 temp directory로 지정해 실제 user home write를 피할 수 있다."
  no_network_side_effect:
    rule: "CLI success는 local artifact write 완료를 뜻하며 Gmail/Slack 전송 완료를 뜻하지 않는다."
  parseable_artifacts:
    rule: "automation은 metadata json과 Slack payload json을 읽어 후속 전송을 수행한다."
  secret_safety:
    rule: "secret은 config에는 저장될 수 있지만 stdout, summary excerpts, Slack preview에는 노출하지 않는다. 단, printed path 자체에 사용자가 secret을 포함한 경우 현재 CLI는 control character만 제거하므로 path secret redaction을 계약으로 약속하지 않는다."
  source_scope:
    rule: "summary source는 selected target agents 중 하나로 제한된다."
```
