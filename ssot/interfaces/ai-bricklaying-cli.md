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
  implementation_language: Go
  executable: ai-bricklaying
  implementation_binary: "dist/ai-bricklaying-<platform>-<arch>"
  npm_launcher: bin/ai-bricklaying.js
  package_bin_field: package.json#bin.ai-bricklaying
  launcher_runtime_node: ">=18 when invoked through npm or npx"
  shipped_binaries:
    - darwin-arm64
    - darwin-amd64
    - linux-amd64
    - linux-arm64
  unsupported_platform_exit:
    code: 1
    stderr: "ai-bricklaying: unsupported platform <platform>-<arch>; supported binaries: darwin-arm64, darwin-amd64, linux-amd64, linux-arm64"
  missing_binary_exit:
    code: 1
    stderr: "ai-bricklaying: bundled binary not found for <platform>-<arch>; reinstall the package or report a release packaging issue"
  invocation_modes:
    - interactive wizard
    - non-interactive flags
    - information flags
    - versioned machine JSON protocol for agent skills and future MCP adapters
  primary_consumers:
    - human developer
    - local automation script
    - AI coding agent that prepares daily reflection artifacts
  source_of_truth:
    - ssot/domains/cli/index.md
    - Go internal CLI packages
    - bin/ai-bricklaying.js launcher contract
    - tests/cli-node.test.js
```

이 문서는 public npm CLI의 외부 interface 계약을 정의한다. `ai-bricklaying` 실행 파일의 동작은 bundled Go binary가 구현하며, `bin/ai-bricklaying.js`는 npm과 npx 사용자를 위해 platform binary를 찾아 인자, stdio, signal, exit code를 전달하는 launcher다. Legacy Python package surface는 Go contract equivalent 통과 후 제거되었으며, npm 사용자가 호출하는 interface 정본이 아니다.

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
    behavior: "npm package version을 stdout에 출력하고 artifact를 쓰지 않는다."

  - command: "ai-bricklaying machine daily prepare"
    mode: machine_json
    behavior: "stdin JSON으로 protocol_version 1.0, date, timezone, sources, output/config directory를 받아 다중 source를 수집하고 resumable daily flow를 준비한다. generated Claude skill은 catalog source key 5개를 배열로 명시하고 adapter별 실제 coverage를 표시하며, 5개 history 형식이 모두 지원된다고 가정하지 않는다. stdout envelope에는 evidence text를 제외한 public flow control metadata와 source coverage가 포함되며 skill은 동의 전에 provider/source/status/count만 사용자에게 표시한다."
    source_fallback: "sources를 생략한 direct machine caller는 saved configured summary source 하나, 그것도 없으면 claude-code 하나를 사용한다. generated skill은 sources를 생략하지 않는다."

  - command: "ai-bricklaying machine daily status"
    mode: machine_json
    behavior: "date의 최신 daily flow와 다음 행동을 조회한다. state나 final artifact를 직접 수정하지 않는다."

  - command: "ai-bricklaying machine daily disclose"
    mode: machine_json
    behavior: "명시적 boolean consent와 expected revision을 검증한다. true이면 common credential을 best-effort redaction한 bounded, untrusted evidence를 stdout JSON에 포함하고, false이면 denied 결정을 저장한 뒤 evidence 없이 성공한다. consent 생략은 validation error다."

  - command: "ai-bricklaying machine daily checkpoint"
    mode: machine_json
    behavior: "agent가 인터뷰에서 확인한 work item과 reflection snapshot을 expected revision 및 idempotency key로 원자 저장한다."

  - command: "ai-bricklaying machine daily finalize"
    mode: machine_json
    behavior: "명시적 user_confirmed, expected revision, idempotency key를 검증하고 CLI가 결정론적으로 confirmed JSON과 Markdown을 저장한다."
```

모든 `machine` command는 `protocol_version: "1.0"`을 포함한 stdin JSON object 하나만 request로 받고 stdout JSON envelope 하나만 response로 쓴다. 사용자 답변이나 evidence를 command-line argument에 넣지 않는다. Claude generated skill이 첫 end-to-end adapter이며, ChatGPT MCP adapter와 weekly command/artifact는 아직 배포되지 않은 후속 범위다.

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
    default: saved config 또는 claude-code
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
    rule: "정확히 하나이며, selected target agents 중 하나여야 한다. 단, --target-agent 없이 known --sources 하나만 명시한 legacy invocation은 해당 source를 sole target으로 승격한다. 두 flag를 모두 명시한 mismatch는 실패한다."
    maps_to: "session discovery source"

  --language:
    type: string
    default: English
    maps_to: "legacy summary template instruction. daily skill/machine locale은 Korean 또는 English로 normalize하고 그 외 값은 English로 fallback한다."

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
    default: ai-bricklaying-worklog
    compatibility: "Agent Skills name contract"
    validation: "length 1..64 and ^[a-z0-9]+(?:-[a-z0-9]+)*$"
    rule: "lowercase a-z, 숫자, 단일 hyphen만 허용한다. leading/trailing/consecutive hyphen, dot, underscore, slash, absolute path, uppercase는 실패한다. skill directory basename과 SKILL.md frontmatter name은 입력값과 정확히 같아야 한다."
    saved_config_compatibility: "flag 미지정 시 안전한 lowercase legacy separator(dot, underscore, hyphen) 이름만 단일 hyphen slug로 migration한다. 같은 config path와 migrated name 소유권이 확인되는 generated skill만 갱신하며, 다른 file/directory와 충돌하면 원문과 저장 config를 덮어쓰지 않고 exit 2다."

  --skill-dir:
    type: path
    default: "target별 default skill directory"
    behavior: "지정하면 모든 selected target의 skill directory를 같은 directory로 override한다. --target-agent 없이 known --sources 하나가 다른 sole target으로 승격되면 이전 target에서 상속된 skill_dir는 버리고 새 target default를 사용하되, 명시적 --skill-dir는 유지한다. 단일 GitHub Copilot target의 이전 product default는 flag 미지정 시 현재 Copilot default로 migration하고 custom/multi-target path는 보존한다."

  --output-dir:
    type: path
    default: "~/ai-bricklaying"
    behavior: "legacy summary, metadata, optional Slack payload와 confirmed worklog 및 finalize lock이 저장되는 root directory다."

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
    behavior: "defaults/delivery config와 resumable daily state 및 per-day state lock을 읽고 쓸 root directory다."

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
    default_skill_dir: "$COPILOT_HOME/skills when COPILOT_HOME is non-empty, otherwise ~/.copilot/skills"
    default_model_hint: "configured Copilot model"
```

```yaml
session_sources:
  opencode:
    label: OpenCode
    default_paths:
      - "~/.local/share/opencode"
    home_env_var: XDG_DATA_HOME
    home_env_behavior: "설정되면 ~/.local/share 대신 XDG_DATA_HOME을 data root로 사용한다."
    env_var: AI_BRICKLAYING_OPENCODE_DIRS
  claude-code:
    label: Claude Code
    default_paths:
      - "~/.claude/projects"
    home_env_var: CLAUDE_CONFIG_DIR
    home_env_behavior: "설정되면 ~/.claude 대신 CLAUDE_CONFIG_DIR을 root로 사용한다."
    env_var: AI_BRICKLAYING_CLAUDE_DIRS
  codex:
    label: Codex
    default_paths:
      - "~/.codex/sessions"
      - "~/.codex/archived_sessions"
    home_env_var: CODEX_HOME
    home_env_behavior: "설정되면 ~/.codex 대신 CODEX_HOME을 root로 사용한다."
    env_var: AI_BRICKLAYING_CODEX_DIRS
  cursor:
    label: Cursor
    default_paths:
      - "~/.cursor/projects"
    env_var: AI_BRICKLAYING_CURSOR_DIRS
  github-copilot:
    label: GitHub Copilot
    default_paths_by_platform:
      darwin:
        - "~/Library/Application Support/Code/User/workspaceStorage"
      linux:
        - "~/.config/Code/User/workspaceStorage"
    env_var: AI_BRICKLAYING_COPILOT_DIRS
```

```yaml
strict_daily_adapters:
  claude-code:
    support: supported
    input: "~/.claude/projects 아래 main-project JSONL"
    accepted: "event time이 relevant local day인 visible user/assistant text"
    excluded: [system, tool, metadata, sidechain, memory, subagent]
  codex:
    support: supported
    input: "CODEX_HOME의 sessions와 archived_sessions"
    relevant_day_precedence: [item_completed, direct_message, response_fallback]
    timestamp_basis: event_time
    excluded: [context, tool, reasoning]
  cursor:
    support: experimental_privacy_conservative
    input: "~/.cursor/projects/**/agent-transcripts의 well-formed JSONL"
    accepted: "role=user인 <user_query> content"
    excluded: [assistant, reasoning, skill]
    timestamp_basis: "event_time; 없을 때만 file_mtime fallback을 timestamp_basis에 표시"
  github-copilot:
    support: experimental
    input: "VS Code workspaceStorage/*/chatSessions schema v3 mutation replay"
    required_extension_id: GitHub.copilot-chat
    excluded: [hidden, system, tool, thinking, other_agent]
    fail_closed: "unknown schema 또는 oversized dependent state를 추측해 복구하지 않는다."
  opencode:
    support: legacy_json_only
    current_storage: "opencode.db는 unsupported_storage"
    accepted: "인식된 legacy JSON text part"
    never_scan: [logs, auth]
legacy_summary_discovery: "선택한 --sources 하나를 generic best-effort 방식으로 찾는 기존 Discover contract를 그대로 유지하며 strict daily matrix와 섞지 않는다."
```

## 5. Config And Environment Inputs

```yaml
config:
  path: "<config-dir>/config.json"
  read_behavior:
    missing_file: "empty config로 처리한다."
    invalid_json: "exit code 2 with read error message"
    legacy_skill_name_migration: "명시적 --skill-name이 없을 때 저장된 lowercase legacy name이 a-z, 숫자, dot, underscore, hyphen만 사용하고 '..'을 포함하지 않으면 separator run을 단일 hyphen으로 바꿔 strict slug로 migration한다. unsafe 저장값과 명시적 flag는 자동 교정하지 않는다."
  write_behavior:
    directory_permission_posix: "0700 best effort"
    file_permission_posix: "0600 best effort"
    contains_secret: true
  defaults:
    target_agents: "string[]; flag 미지정 시 --target-agent default"
    source: "string|null; flag 미지정 시 --sources default"
    target_model: "string"
    language: "string; legacy lightweight summary setting. daily machine flow는 이를 그대로 임의 언어 계약으로 사용하지 않는다."
    output_modes: "string[]"
    skill_name: "string"
    skill_dir: "string|null; selected targets의 skill dir가 모두 같을 때만 저장"
    output_dir: "string"
    cli_version: "string"
  delivery:
    gmail_recipient: "string|null"
    gmail_subject: "string|null"
    slack_webhook_url: "string|null; secret"
  interactive_secret_input:
    unix_tty: "terminal echo를 끄고 x/term ReadPassword로 읽으며 q, Esc, Ctrl-C는 취소한다. hidden reader 실패 시 echoed scanner로 fallback하지 않고 실패한다."
    non_tty_or_unsupported_platform: "line input fallback; 입력값 자체는 CLI output으로 다시 쓰지 않는다."
    success_output: "[hidden] marker만 표시한다."
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
    - "configured Slack webhook raw value"
    - "values matching the supported common token/password/API key patterns"
    - "raw private session excerpts"
  machine_json:
    shape: "exactly one JSON envelope; no ANSI, progress text, or Markdown"
    required: [protocol_version, command, ok, flow_id, revision, state, next_action]
    request_version_rule: "모든 request는 protocol_version 1.0을 포함해야 한다. 없거나 다른 version이면 validation error envelope와 exit 2다."
    evidence_rule: "prepare/status/checkpoint/finalize는 evidence text를 생략한다. disclose consent=true 성공만 best-effort-redacted evidence를 포함하며 consent=false 성공은 evidence를 포함하지 않는다."

stderr:
  validation_failure: "human-readable CliError message"
  cancellation: "Cancelled."
  unexpected_failure: "Unexpected error: <message>"

exit_codes:
  0: "success or information flag"
  2: "CliError: local validation failure, cancellation, config read error, symlink refusal, generated skill destination collision"
  1: "unexpected runtime failure"
  machine_rule: "machine validation, not-found, consent 누락, revision conflict도 exit 2이고 parseable error envelope를 stdout에 남긴다. consent=false는 validation failure가 아니라 denied persistence success다. internal failure만 exit 1이다."
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
    content_rule: "generated Claude skill은 daily prepare에 catalog source key 5개를 명시하고 adapter별 coverage를 표시하며 protocol_version 1.0의 machine command만 사용한다. 재사용 실행에는 ai-bricklaying executable이 host local PATH에 있고 host가 local shell/source file 접근을 허용해야 한다. configured summary/delivery path는 legacy summary용이며 confirmed worklog delivery가 아니다."
    mode: "0644 best effort"

  - path: "<config-dir>/config.json"
    required: true
    mode: "0600 best effort"

  - path: "<config-dir>/state/v1/daily/YYYY-MM-DD.json"
    required_when: "machine daily prepare가 성공한 경우"
    content_rule: "schema version, flow ID, revision, draft/interviewing/confirmed state, consent, coverage, bounded best-effort-redacted evidence, work items, reflection, interview progress를 저장한다. discovery record path field는 저장하지 않지만 자유 텍스트에서 모든 path-like 문자열이 제거된다고 보장하지 않는다."
    mode: "0600; parent directories 0700"

  - path: "<config-dir>/state/v1/daily/YYYY-MM-DD.json.lock"
    required_when: "prepare 또는 state mutation이 per-day process lock을 획득한 경우"
    content_rule: "빈 internal coordination file이며 실행 후에도 남을 수 있다. 공유 artifact가 아니다."
    mode: "0600; parent directories 0700"

  - path: "<output-dir>/worklogs/daily/YYYY-MM-DD-ai-bricklaying-worklog.md"
    required_when: "machine daily finalize가 성공한 경우"
    content_rule: "confirmed work items와 사용자 회고를 CLI가 결정론적으로 render한다. evidence excerpt field를 render하지 않으며 Phase 1A에서는 local-only artifact다. 자유 텍스트에서 모든 path-like 문자열이 제거된다고 보장하지 않는다."
    mode: "0600; parent directories 0700"

  - path: "<output-dir>/worklogs/daily/YYYY-MM-DD-ai-bricklaying-worklog.json"
    required_when: "machine daily finalize가 성공한 경우"
    content_rule: "confirmed structured worklog sidecar이며 redacted evidence excerpt는 포함하지 않는다."
    mode: "0600; parent directories 0700"

  - path: "<output-dir>/.ai-bricklaying/locks/v1/daily/YYYY-MM-DD.lock"
    required_when: "machine daily finalize가 output artifact lock을 획득한 경우"
    content_rule: "서로 다른 config root가 같은 output/date artifact를 동시에 확정하지 못하게 하는 빈 internal coordination file이며 실행 후에도 남을 수 있다."
    mode: "0600; parent directories 0700"
```

## 9. Artifact Schemas

```yaml
metadata_json:
  config_path: "string"
  deliveries: "string[]"
  gmail_recipient: "string|null"
  gmail_subject: "string|null"
  language: "string; legacy lightweight summary language"
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
  split_rule: "large summaries may be split only to satisfy Slack block length limits"
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
  description: "Create or resume today's AI-activity-informed or user-recalled worklog and conduct a short interview."

generated_skill_sections:
  - Sources
  - Output Locations
  - CLI Result Delivery Modes
  - Machine Protocol
  - Consent And Untrusted Evidence
  - Daily Interview
  - Resume And Conflicts
  - Workflow
```

```yaml
machine_daily_flow:
  schema_version: "1.0"
  states: [draft, interviewing, confirmed]
  envelope_required: [protocol_version, command, ok, flow_id, revision, state, next_action]
  request_required: [protocol_version]
  request_protocol_version: "1.0"
  prepare_optional: [date, timezone, language, sources, output_dir, config_dir]
  daily_languages: [Korean, English]
  daily_language_rule: "prepare.language 또는 saved legacy config language가 정확한 ko, BCP-47 형식의 ko-*, 또는 case-insensitive Korean/한국 substring을 포함하면 Korean으로 normalize한다. 그 밖의 값과 빈 최종 값은 모두 English로 fallback하며 arbitrary-language 질문/문서 생성을 계약하지 않는다."
  status_optional: [date, timezone, config_dir]
  mutation_required: [flow_id, date, expected_revision, idempotency_key]
  public_flow_control_fields: [schema_version, flow_id, kind, date, timezone, language, state, revision, output_dir, config_dir, state_path, worklog_markdown_path, worklog_json_path, consent, coverage, work_items, no_work_confirmed, reflection, interview, created_at, updated_at, confirmed_at]
  public_flow_rule: "prepare/status/checkpoint/finalize는 위 control metadata와 coverage를 반환하되 evidence와 idempotency audit record를 제외한다. home 아래 path는 ~ prefix로 최소화할 수 있지만 모든 path-like 자유 텍스트 제거를 보장하지 않는다."
  source_status: [complete, no_activity, not_found, unreadable, parse_error, truncated, unsupported_schema, unsupported_storage]
  strict_candidate_retention_limit_per_source: 5000
  strict_candidate_retention_rule: "adapter-matched candidate의 newest top-k 보관과 candidate-selection memory 한도다. directory traversal entry, wall-clock, scan, total resource bound는 아니다."
  evidence_fields: [id, source_key, source_label, modified_at, timestamp_basis, excerpt, untrusted]
  forbidden_evidence_schema_fields: [raw_path, full_transcript, credential]
  evidence_content_boundary: "common credential pattern과 home prefix는 best-effort로 최소화하지만 excerpt 자유 텍스트가 완전히 secret/path-free라고 보장하지 않는다."
  conflict_rule: "expected revision이 최신 revision과 다르면 write 없이 conflict error를 반환한다. 단, retained idempotency record와 command/request hash가 정확히 같은 재시도는 revision 검사 전에 성공 결과를 재사용한다."
  lock_rule: "state read-check-write mutation 전체를 owner-only per-day state lock으로 직렬화하고 finalize artifact 쓰기는 output/date lock으로 직렬화한다. lock 대기 한도를 넘으면 각각 retryable flow_busy 또는 artifact_busy를 반환한다."
  idempotency_rule: "같은 flow와 idempotency key의 같은 operation 재시도는 중복 write 없이 현재 결과를 반환한다. Durable mutation record는 flow당 최대 100개이며 한도 뒤 새 mutation은 mutation_limit_reached로 실패한다."
  prepare_existing_rule: "같은 config_dir/date의 flow가 이미 있으면 prepare는 stored sources/coverage, output_dir, language, timezone, state, revision을 유지하고 새 profile input을 적용하지 않는다. 새 flow만 draft/revision 1로 생성한다."
  interview_question_ids: [title_review, reflection_result, reflection_difficulty_feeling, reflection_learning_next, preview, complete]
  work_item_fields: [id, title, evidence_summary, uncertainty, performed, outcome, verification, issues, evidence_ids, status, origin]
  candidate_display_rule: "generated skill은 stable numeric label, title, one-line evidence_summary, one-line uncertainty를 normalized daily language인 Korean 또는 English로 보여준다. user_recall basis는 사용자 회상임을 명시하고 numeric confidence는 만들지 않는다."
  question_render_rule: "persisted interview field에는 allowlisted ID만 저장하고 human question은 generated skill의 deterministic Korean/English template에서 렌더링한다. unsupported legacy language는 English template을 사용한다."
  consent_rule: "disclose consent=true는 evidence disclosure granted를, consent=false는 denied를 저장한다. 둘 다 성공 mutation이며 consent 누락만 validation failure다. Evidence가 있는 flow는 granted 또는 denied가 저장되기 전 checkpoint/finalize를 허용하지 않는다. Evidence가 없는 recall-only flow는 pending으로 진행할 수 있다."
  scheduler_rule: "host scheduler는 skill 호출을 깨울 수만 있다. 이전 요청이나 stored state에서 consent/user_confirmed를 추론하지 않고, explicit current user decision 없이 disclose/finalize/legacy delivery를 자동 실행하지 않는다."
  checkpoint_snapshot_rule: "work_items, reflection, interview와 no_work_confirmed true|false가 모두 명시된 complete snapshot만 받으며 no_work_confirmed 생략은 실패한다."
  finalize_rule: "state=interviewing, interview.stage=preview, next_question empty, completed_questions에 title_review/reflection_result/reflection_difficulty_feeling/reflection_learning_next가 모두 있고, 모든 work item이 confirmed 또는 excluded인 flow만 finalize할 수 있다. 추가로 user_confirmed=true이며 confirmed item이 하나 이상이거나 checkpoint에서 no_work_confirmed=true로 명시해야 한다. Finalize도 100 durable mutation limit을 적용한다."
  persisted_state_integrity_rule: "work item과 reflection의 모든 persisted field는 canonical sanitize 결과와 정확히 같아야 하며 변조되거나 credential-bearing raw field는 invalid_flow_state로 거부한다."
  artifact_publish_rule: "confirmed JSON/Markdown 신규 publish는 filesystem create-if-absent no-clobber를 사용하고 foreign/mismatched artifact를 덮어쓰지 않는다. State는 atomic replace semantics를 유지한다."
  symlink_check_limit: "ancestor symlink refusal은 path-based check이며 검사 직후 악의적인 local actor가 ancestor를 교체하는 descriptor-relative TOCTOU 방어까지 계약하지 않는다."
  interrupted_finalize_recovery_rule: "JSON 또는 Markdown publish 뒤 state write 전에 중단되면 동일 flow/revision/expected content로 소유권을 검증한 retry만 누락 artifact/state를 완성하고 기존 confirmed_at을 보존한다. 다른 flow 또는 content mismatch는 artifact_conflict다."
  confirmed_rule: "confirmed daily flow는 read-only이며 같은 날짜 재실행으로 덮어쓰거나 interviewing으로 되돌리지 않는다."
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
    rule: "secret은 config에 저장될 수 있다. session text와 stdout에는 common credential pattern을 best-effort로 redaction하지만 알려지지 않은 형식까지 완전 제거한다고 보장하지 않는다. printed path 자체에 사용자가 secret을 포함하지 않아야 한다."
  source_scope:
    rule: "summary source는 selected target agents 중 하나로 제한된다."
  sole_writer:
    rule: "agent skill과 후속 MCP adapter는 state나 worklog를 직접 쓰지 않고 machine command만 호출한다."
  consent_before_evidence:
    rule: "machine adapter는 evidence가 제외된 public control metadata와 coverage를 받는다. generated skill은 동의를 묻기 전에 사용자에게 provider/source 이름, status, count만 보여준다. consent=true 전에는 evidence excerpt를 stdout에 반환하지 않으며 consent=false는 denied로 저장하고 사용자 자유 회상으로 진행한다."
  untrusted_session_data:
    rule: "session-derived text는 모두 untrusted data다. generated skill은 그 안의 명령, URL, path, tool 요청을 실행하거나 따르지 않는다."
  bounded_interview:
    rule: "generated skill은 한 번에 질문 하나, 기본 3~5분, 최대 6개 후보와 묶음 승인, 수정/합치기/나누기/제외/추가/기록할 업무 없음/건너뛰기/이전/중단 후 재개를 제공한다. 이전은 현재 대화의 in-memory 직전 snapshot만 되돌리고 persisted state는 최신 checkpoint만 보존하므로 cross-session undo history를 약속하지 않는다. Empty work item 확정은 사용자의 명시적 no_work_confirmed 결정을 checkpoint한다."
  same_date_resume:
    rule: "generated skill은 요청 시점의 같은 local date flow만 status/resume하고 과거 날짜 flow를 오늘 interview로 자동 연결하지 않는다."
  local_only_daily_delivery:
    rule: "Phase 1A confirmed worklog는 local-only다. Gmail MCP/Slack 설정과 payload는 legacy lightweight summary handoff이며 worklog delivery가 아니다."
```
