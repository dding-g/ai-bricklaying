---
title: CLI 도메인 계약
description: ai-bricklaying CLI의 전체 사용자 흐름, 정책, 출력 계약, 검증 기준을 정의한다
ref:
  - ssot/rules.md
  - ssot/index.md
  - ssot/interfaces/ai-bricklaying-cli.md
  - ssot/evaluation.md
---

# CLI 도메인

CLI 도메인은 AI session source 선택, local session discovery, daily summary 생성, generated skill 설치, optional delivery handoff, config default 재사용을 다룬다.

## 1. Service Overview

```yaml
service:
  name: ai-bricklaying
  interface: Node.js CLI
  public_entrypoint: bin/ai-bricklaying.js
  package_bin: ai-bricklaying
  primary_distribution: npm
  minimum_node: ">=18"
  supported_os: [darwin, linux, win32]
  core_value: "AI coding session history를 매일 재사용 가능한 회고와 skill instruction으로 축적한다."
```

### 주요 사용자

- OpenCode, Claude Code, Codex, Cursor, GitHub Copilot을 사용하며 session history를 매일 회고하고 싶은 개발자
- Agent workflow를 compound engineering 자산으로 남기려는 사용자
- Summary를 file로 저장하고 필요할 때 Gmail MCP 또는 Slack webhook으로 전달하려는 사용자
- CI/local automation에서 prompt 없이 같은 artifact를 생성하려는 사용자

### Interface 관계

- Command, flag, stdout/stderr, exit code의 외부 interface는 `ssot/interfaces/ai-bricklaying-cli.md`가 정본이다.
- 이 문서는 그 interface가 어떤 제품 usecase, output artifact, invariant, acceptance에 연결되는지 정의한다.

### 포함 범위

- Target AI agent 하나 이상 선택
- Selected target 중 summary source 하나 선택
- Summary language 선택
- Local session artifact best-effort discovery
- Daily reflection Markdown 생성
- Metadata JSON 생성
- Slack-ready payload JSON 생성
- Generated skill `SKILL.md` 설치
- Local config default와 delivery setting 저장
- Secret redaction과 filesystem safety policy 적용

### 비포함 범위

- LLM API를 호출해 abstractive summary를 생성하는 기능
- CLI가 Gmail MCP를 직접 호출해 email을 보내는 기능
- CLI가 Slack webhook으로 즉시 전송하는 기능
- Remote/cloud session history 동기화
- Secret vault 관리
- Generated artifact를 정본 문서로 관리하는 workflow

## 2. Usecase Inventory

```yaml
usecases:
  - id: UC_SHOW_HELP
    trigger: "ai-bricklaying --help 또는 -h"
    actor: user_or_automation
    purpose: "지원 option과 usage를 확인한다."
    outputs: [stdout_help]
    artifact_write: false
    exit_code: 0

  - id: UC_SHOW_VERSION
    trigger: "ai-bricklaying --version 또는 -v"
    actor: user_or_automation
    purpose: "설치된 package version을 확인한다."
    outputs: [stdout_version]
    artifact_write: false
    exit_code: 0

  - id: UC_INTERACTIVE_SETUP_TUI
    trigger: "ai-bricklaying"
    actor: user
    purpose: "TTY wizard로 target, source, language, output mode를 선택해 summary와 skill을 생성한다."
    preconditions:
      - "stdin/stdout이 TTY이고 raw mode를 지원한다."
    inputs:
      target_agents: "OpenCode, Claude Code, Codex, Cursor, GitHub Copilot 중 하나 이상"
      source: "선택한 target agent 중 정확히 하나"
      language: "summary template에 들어갈 출력 언어"
      output_dir: "file save directory"
      output_modes: "file은 고정, gmail-mcp와 slack-webhook은 선택"
    side_effects: [local_filesystem_write]
    external_send: false

  - id: UC_INTERACTIVE_SETUP_LINE_MODE
    trigger: "ai-bricklaying with non-TTY stdin/stdout"
    actor: user_or_test_harness
    purpose: "raw-mode wizard가 불가능한 환경에서 numbered prompt fallback으로 같은 설정을 완료한다."
    rules:
      - "target list와 output mode list는 [x]/[ ] marker를 보여준다."
      - "blank/default answer는 prompt default를 사용한다."
      - "source prompt는 selected target에 대응하는 source만 보여준다."
      - "기존 Slack webhook은 [configured]로만 표시한다."
    side_effects: [local_filesystem_write]
    external_send: false

  - id: UC_NON_INTERACTIVE_RUN
    trigger: "ai-bricklaying --non-interactive"
    actor: user_or_automation
    purpose: "Prompt 없이 flags와 saved config defaults로 artifact를 생성한다."
    validation:
      - "unknown argument는 실패한다."
      - "--sources는 정확히 하나다."
      - "--sources는 selected target 중 하나다."
      - "--skill-name은 path-safe lowercase slug다."
      - "non-interactive gmail-mcp에는 recipient와 subject가 필요하다."
      - "non-interactive slack-webhook에는 webhook URL이 필요하다."
    side_effects: [local_filesystem_write]
    external_send: false

  - id: UC_REUSE_CONFIG_DEFAULTS
    trigger: "ai-bricklaying 또는 ai-bricklaying --non-interactive with existing config"
    actor: user_or_automation
    purpose: "이전 실행의 target/source/language/output/dir/delivery 설정을 default로 재사용한다."
    precedence:
      - "command-line flags override saved config"
      - "saved config fills missing args"
      - "built-in defaults fill remaining args"
    side_effects:
      - "config.json is rewritten with current defaults and delivery settings"

  - id: UC_SELECT_MULTIPLE_TARGETS_SINGLE_SOURCE
    trigger: "--target-agent opencode,codex --sources opencode"
    actor: user_or_automation
    purpose: "하나의 source summary로 여러 target agent에 같은 generated skill을 설치한다."
    postconditions:
      - "각 selected target skill directory에 SKILL.md가 생성된다."
      - "metadata.target_agents는 display name 배열이다."
      - "metadata.sessions는 source key 하나다."

  - id: UC_DISCOVER_TODAY_SESSIONS
    trigger: "successful run after source selection"
    actor: cli_process
    purpose: "selected source의 local roots에서 오늘 수정된 session artifact를 읽어 lightweight signal을 추출한다."
    rules:
      - "default path 또는 AI_BRICKLAYING_*_DIRS override를 사용한다."
      - "path delimiter로 여러 override root를 받을 수 있다."
      - "file root와 directory root를 모두 허용한다."
      - "directory traversal 중 symbolic link는 따라가지 않는다."
      - "읽을 수 없는 directory/file은 skip한다."
      - "오늘 local date와 mtime이 맞는 artifact만 후보가 된다."
      - "source당 최대 12개 record를 수집한다."
      - "각 file은 최대 20000 chars까지 읽는다."
      - "지원 확장자는 .json, .jsonl, .md, .txt, .log다."

  - id: UC_HANDLE_MISSING_SESSIONS
    trigger: "selected source에서 오늘 readable session signal이 없음"
    actor: cli_process
    purpose: "session이 없어도 summary template과 skill을 생성해 reflection workflow를 유지한다."
    exit_code: 0
    postconditions:
      - "summary contains no clear session signals guidance"
      - "generated skill still exists"

  - id: UC_BUILD_DAILY_SUMMARY
    trigger: "session discovery completed"
    actor: cli_process
    purpose: "session signal을 raw artifact log가 아니라 lightweight reflection prompt로 바꾼다."
    rules:
      - "summary H1은 AI Bricklaying Daily Summary - YYYY-MM-DD다."
      - "local path dump를 포함하지 않는다."
      - "themes are extracted from keyword families when signals exist."
      - "template language instruction uses selected language."
      - "delivery notes reflect selected output modes."

  - id: UC_WRITE_ARTIFACTS
    trigger: "summary built"
    actor: cli_process
    purpose: "summary, metadata, config, optional Slack payload, generated skills를 안전하게 쓴다."
    rules:
      - "summary는 항상 저장한다."
      - "metadata는 항상 저장한다."
      - "config는 항상 저장한다."
      - "Slack payload는 slack-webhook mode일 때만 저장한다."
      - "SKILL.md는 selected target별로 저장한다."
      - "기존 output symlink overwrite는 거부한다."
      - "file write는 temp file 후 rename으로 수행한다."

  - id: UC_PREPARE_SLACK_PAYLOAD
    trigger: "--output-modes slack-webhook 또는 saved config output_modes includes slack-webhook"
    actor: user_or_automation
    purpose: "Summary Markdown을 Slack Block Kit payload JSON으로 변환해 저장한다."
    required_input:
      - "non-interactive mode에서는 --slack-webhook-url 또는 saved config delivery.slack_webhook_url"
    interactive_behavior:
      - "interactive mode에서는 Slack webhook 입력이 optional이며 missing이면 not provided/configured false로 기록한다."
    postconditions:
      - "저장된 Markdown summary가 Slack payload 생성의 source of truth다."
      - "Slack payload는 Markdown 최종본에서 생성하며 별도 short summary로 재작성하지 않는다."
      - "Markdown의 모든 top-level section과 bullet 순서를 유지한다."
      - "Slack length limit 대응은 block/message batch 분리로만 수행한다."
      - "top-level text와 blocks는 first batch를 담는다."
      - "messages 배열은 모든 batch를 담는다."
      - "verification은 모든 top-level section이 payload에 포함됐는지 기록한다."
    external_send: false

  - id: UC_PREPARE_GMAIL_HANDOFF
    trigger: "--output-modes gmail-mcp 또는 saved config output_modes includes gmail-mcp"
    actor: user_or_automation
    purpose: "Gmail MCP로 보낼 recipient와 subject를 summary/delivery notes와 generated skill에 기록한다."
    required_input:
      - "non-interactive mode에서는 --gmail-recipient 또는 saved config delivery.gmail_recipient"
      - "non-interactive mode에서는 --gmail-subject 또는 saved config delivery.gmail_subject"
    interactive_behavior:
      - "interactive mode에서는 recipient와 subject 입력이 optional이며 missing이면 summary에 not provided로 기록한다."
    external_send: false

  - id: UC_GENERATE_SKILL
    trigger: "successful run"
    actor: cli_process
    purpose: "다음 AI session에서 사용할 reusable skill instruction을 target skill directory에 설치한다."
    rules:
      - "frontmatter name은 --skill-name이다."
      - "delivery mode section은 current run의 selected modes만 설명한다."
      - "file-only mode에서는 Gmail/Slack을 시도하지 말라는 guardrail을 포함한다."
      - "gmail/slack mode에서는 missing recipient/webhook/auth를 추측하지 말고 보고하라고 지시한다."
      - "slack-webhook mode에서는 saved Markdown을 Slack delivery source of truth로 삼고, Markdown 최종본에서 Block Kit payload를 만들며, 사용자 명시 없이는 Slack 전용 short summary를 만들지 말라는 guardrail을 포함한다."
      - "slack-webhook mode에서는 Markdown section/bullet order 유지와 전송 전 top-level section coverage 검증을 지시한다."

  - id: UC_PRINT_COMPLETION
    trigger: "all artifacts written"
    actor: cli_process
    purpose: "사용자가 결과 위치와 generated skill command를 확인한다."
    stdout_must_include:
      - "AI Bricklaying files generated"
      - "summary, metadata, config paths"
      - "skill paths"
      - "Use the generated skill: /<skill-name>"
      - "refresh command"
    stdout_conditional:
      - "Slack payload path if created"
      - "OpenCode restart hint if OpenCode selected"
      - "Gmail/Slack delivery selected notes"

  - id: UC_REFRESH_GENERATED_SKILL
    trigger: "npm install -g ai-bricklaying@latest && ai-bricklaying"
    actor: user
    purpose: "CLI 업데이트 후 기존 config defaults로 generated skill을 재생성한다."
    postconditions:
      - "selected target skill directory의 SKILL.md가 최신 template으로 갱신된다."
```

## 3. Entity And Field Dictionary

```yaml
entities:
  AgentTarget:
    meaning: "Generated skill을 설치할 AI agent host"
    fields:
      key: "CLI flag value. opencode, claude-code, codex, cursor, github-copilot"
      name: "사용자 표시 이름"
      default_skill_dir: "기본 skill 저장 directory"
      model_hint: "artifact metadata에 기록할 model label"

  SessionSource:
    meaning: "Summary에 사용할 local session history source"
    fields:
      key: "source identifier"
      label: "사용자 표시 이름"
      default_paths: "best-effort discovery root 목록"
      env_var: "source path override env var"

  SessionRecord:
    meaning: "Discovery에서 읽은 session signal"
    fields:
      source: "source label"
      path: "internal path; summary에는 직접 노출하지 않는다"
      text: "secret redaction 후 추출된 text"

  SummaryArtifact:
    meaning: "실행 결과로 저장되는 Markdown summary"
    fields:
      file_name: "YYYY-MM-DD-ai-bricklaying-daily-summary.md"
      language: "user selected language"
      source_coverage: "찾은 session signal 수 또는 missing-session 안내"
      themes: "keyword family 기반 lightweight session signal"
      template_sections: "compound engineering summary template"

  MetadataArtifact:
    meaning: "automation이 실행 결과를 parse할 JSON sidecar"
    fields:
      cli_version: "package version"
      deliveries: "selected output modes"
      sessions: "selected source key array"
      target_agents: "selected target display names"
      skill_dirs: "generated skill directories"
      slack_payload_path: "Slack payload path or null"

  SlackPayloadArtifact:
    meaning: "Slack webhook caller가 사용할 Block Kit payload"
    fields:
      text: "fallback text"
      blocks: "first batch blocks"
      messages: "all batches"

  SkillArtifact:
    meaning: "AI agent가 재사용할 generated skill"
    fields:
      directory: "<skill-dir>/<skill-name>"
      file: "SKILL.md"
      delivery_modes: "해당 run에서 선택된 output modes"
      summary_template: "selected language instruction 포함"

  LocalConfig:
    meaning: "다음 실행 default와 delivery 설정"
    fields:
      path: "~/.config/ai-bricklaying/config.json 또는 --config-dir/config.json"
      delivery: "gmail/slack handoff 설정; secret 포함 가능"
      defaults: "target, source, model, language, output modes, skill/output directories, cli version"
```

## 4. Output Contract

```yaml
outputs:
  summary_markdown:
    required: true
    default_dir: "~/ai-bricklaying"
    override: "--output-dir"
    filename: "YYYY-MM-DD-ai-bricklaying-daily-summary.md"
    must_include:
      - "AI Bricklaying Daily Summary - YYYY-MM-DD"
      - "Language"
      - "Target skill"
      - "Summary source"
      - "Lightweight Session Signals"
      - "Summary Template For AI Agent"
      - "Delivery Notes"
      - "Today's Takeaways"
      - "Lessons Learned"
      - "What Improved"
      - "Better AI Usage Next Time"
      - "Tomorrow's Best Next Step"
    must_not_include:
      - "raw local session artifact path"
      - "Slack webhook secret"
      - "Bearer token"
      - "password or API key value"

  metadata_json:
    required: true
    filename: "ai-bricklaying-summary-skill.json"
    must_include:
      - "config_path"
      - "deliveries"
      - "gmail_recipient"
      - "gmail_subject"
      - "language"
      - "cli_version"
      - "session_count"
      - "sessions"
      - "skill_dirs"
      - "slack_webhook_configured"
      - "slack_payload_path"
      - "summary_path"
      - "target_agents"
      - "target_model"

  slack_payload_json:
    required_when: "slack-webhook output mode"
    filename: "ai-bricklaying-slack-payload.json"
    must_include:
      - "text"
      - "blocks"
      - "messages"
    rule: "Slack에는 raw Markdown 대신 Block Kit JSON을 보낼 수 있어야 한다."

  config_json:
    required: true
    filename: "<config-dir>/config.json"
    contains_secret: true
    permission: "0600 best effort; parent directory 0700 best effort"

  generated_skill:
    required: true
    filename: "<skill-dir>/<skill-name>/SKILL.md"
    must_include:
      - "frontmatter name"
      - "description"
      - "Sources"
      - "Output Locations"
      - "CLI Result Delivery Modes"
      - "Workflow"
      - "Summary Template"
      - "configured summary directory"
      - "metadata/config/payload path references"
      - "file-only mode no-send instruction when only file selected"
```

## 5. State Machine And Invariants

```yaml
states:
  - START
  - PARSE_ARGS
  - INFORMATION_FLAG_EXIT
  - LOAD_CONFIG_DEFAULTS
  - SELECT_TARGETS
  - SELECT_SOURCE
  - SELECT_LANGUAGE
  - SELECT_OUTPUT_DIR
  - SELECT_OUTPUT_MODES
  - COLLECT_DELIVERY_DETAILS
  - VALIDATE_SKILL_NAME
  - BUILD_CONFIG
  - DISCOVER_SESSIONS
  - BUILD_SUMMARY
  - WRITE_SUMMARY
  - WRITE_SLACK_PAYLOAD_OPTIONAL
  - WRITE_METADATA
  - WRITE_CONFIG
  - WRITE_SKILLS
  - PRINT_COMPLETION
  - FAILED_VALIDATION
  - FAILED_UNEXPECTED

invariants:
  - id: ALWAYS_SAVE_FILE
    rule: "모든 successful generation run은 summary markdown을 저장한다."
  - id: FILE_MODE_ALWAYS_INCLUDED
    rule: "output modes에는 항상 file이 포함된다."
  - id: NO_IMPLICIT_SEND
    rule: "CLI는 Gmail이나 Slack으로 직접 전송하지 않는다. 단, generated skill은 선택된 delivery mode에 따라 이후 agent가 전송하도록 안내할 수 있다."
  - id: SOURCE_WITHIN_TARGETS
    rule: "summary source는 selected target agent 중 하나여야 한다."
  - id: SINGLE_SOURCE
    rule: "한 run의 summary source는 정확히 하나다."
  - id: MULTI_TARGET_ALLOWED
    rule: "target agent는 하나 이상 선택할 수 있다."
  - id: MISSING_SESSIONS_STILL_SUCCEED
    rule: "오늘 session artifact를 찾지 못해도 summary와 skill은 생성한다."
  - id: SECRET_REDACTION
    rule: "session snippet과 stdout에는 secret이 노출되면 안 된다."
  - id: CONFIG_FLAGS_PRECEDENCE
    rule: "command-line flags가 saved config보다 우선한다."
  - id: GENERATED_OUTPUT_NON_CANONICAL
    rule: "summary, metadata, Slack payload, generated SKILL.md는 실행 결과물이지 SSOT 정본이 아니다."
```

## 6. Permissions And Policies

```yaml
policies:
  - id: LOCAL_WRITE_POLICY
    subject: "CLI process"
    allowed:
      - "--output-dir 아래 summary, metadata, optional Slack payload 작성"
      - "target skill dir 아래 SKILL.md 작성"
      - "--config-dir/config.json 작성"
    denied:
      - "선택되지 않은 target skill dir 쓰기"
      - "summary output symlink overwrite"
      - "generated artifact를 SSOT 정본처럼 수동 관리"

  - id: SAFE_WRITE_POLICY
    subject: "filesystem writes"
    rules:
      - "parent directory를 먼저 생성한다."
      - "existing target이 symbolic link면 CliError로 거부한다."
      - "temp file을 wx로 만들고 write 후 rename한다."
      - "summary/metadata/skill/slack payload mode는 0644 best effort다."
      - "config file mode는 0600 best effort다."
      - "config directory mode는 0700 best effort다."

  - id: SECRET_POLICY
    subject: "Slack webhook, tokens, passwords, API keys, private keys"
    allowed:
      - "private config file 저장"
      - "configured 여부 표시"
      - "metadata에 slack_webhook_configured boolean 저장"
    denied:
      - "stdout 출력"
      - "summary session snippet 출력"
      - "Slack payload preview에 원문 secret 포함"
      - "README/SSOT example에 실제 secret 형태 사용"
    redaction_patterns:
      - "Slack webhook URL"
      - "private key PEM block"
      - "Bearer token"
      - "token/api_key/secret/password key-value"
      - "sk_/pk_ long token"

  - id: EXTERNAL_DELIVERY_POLICY
    subject: "Gmail MCP and Slack webhook"
    allowed:
      - "selected mode일 때 handoff note 또는 payload 준비"
      - "generated skill이 selected delivery mode를 따라 후속 전송을 수행하도록 안내"
    denied:
      - "CLI가 사용자 확인 없이 직접 Gmail/Slack 전송"
      - "file-only generated skill이 Gmail/Slack 전송 시도"

  - id: OUTPUT_PRIVACY_POLICY
    subject: "summary markdown and stdout"
    rules:
      - "local session artifact path를 summary 본문에 덤프하지 않는다."
      - "stdout path는 control character를 제거한다. 현재 CLI는 printed path 안의 secret-looking substring을 별도 redact하지 않으므로 사용자는 output/skill/config path에 secret을 넣지 않아야 한다."
      - "NO_COLOR가 있으면 ANSI escape를 출력하지 않는다."

  - id: PACKAGE_RELEASE_POLICY
    subject: "npm package"
    rules:
      - "package.json#files에 있는 파일만 npm package에 포함되는 정본 배포물이다."
      - "maintainer release command는 bun run release이며 release-it이 version bump, git tag, GitHub release, npm publish를 수행한다."
      - "package contents 변경 시 bun run pack:dry-run으로 확인한다."
      - "Node CLI behavior 변경 시 bun run test를 실행한다."
```

## 7. Query And Discovery Contract

```yaml
session_sources:
  - key: opencode
    default_paths: ["~/.local/share/opencode"]
    env_var: AI_BRICKLAYING_OPENCODE_DIRS
  - key: claude-code
    default_paths: ["~/.claude/projects"]
    env_var: AI_BRICKLAYING_CLAUDE_DIRS
  - key: codex
    default_paths: ["~/.codex/sessions", "~/.codex"]
    env_var: AI_BRICKLAYING_CODEX_DIRS
  - key: cursor
    default_paths: ["~/Library/Application Support/Cursor/User/workspaceStorage"]
    env_var: AI_BRICKLAYING_CURSOR_DIRS
  - key: github-copilot
    default_paths: ["~/Library/Application Support/Code/User/workspaceStorage"]
    env_var: AI_BRICKLAYING_COPILOT_DIRS

discovery_rules:
  date_boundary: "local timezone의 today"
  env_override_separator: "platform path delimiter"
  candidate_roots: "file or directory"
  symbolic_links: "directory traversal에서 skip"
  unreadable_paths: "skip without failing run"
  file_extensions: [.json, .jsonl, .md, .txt, .log]
  max_records_per_source: 12
  max_chars_per_file: 20000
  json_extraction_preferred_keys: [text, content, message, prompt, response, summary]
  sensitive_json_keys: "password|secret|token|api_key|access_key|private_key|webhook 계열은 [REDACTED]로 치환"
  missing_root: "failure가 아니라 no session coverage로 처리한다."
```

## 8. External Dependencies And Async

```yaml
external_dependencies:
  - name: markdown-to-slack-blocks
    used_by: "Slack payload generation"
    package_source: package.json#dependencies
    failure_mapping: "Unexpected runtime failure unless explicitly converted; file summary invariant should be checked after any change."
  - name: Gmail MCP
    used_by: "handoff instruction only"
    failure_mapping: "CLI는 직접 호출하지 않으므로 missing authorization은 generated skill 실행 시 보고한다."
  - name: Slack webhook
    used_by: "handoff payload target stored in config"
    failure_mapping: "non-interactive에서 webhook URL이 없으면 slack-webhook mode validation failure; interactive에서는 optional not-provided handoff로 계속 진행"

async_behavior:
  direct_async_send: false
  accepted_vs_completed: "CLI artifact write 완료와 external delivery 완료는 다른 사건이다. CLI는 artifact write까지만 완료로 본다."
```

## 9. Error Model

```yaml
errors:
  - code: INFO_EXIT
    cases:
      - "--help"
      - "--version"
    user_result: "stdout only, exit code 0, no artifact write"

  - code: INVALID_ARGUMENT
    cases:
      - "unknown CLI option"
      - "flag missing required value"
      - "unknown target agent"
      - "unknown source"
      - "unknown output mode"
      - "unsafe skill name"
      - "multiple summary sources"
      - "source outside selected targets"
    user_result: "stderr message와 exit code 2"

  - code: MISSING_DELIVERY_SETTING
    cases:
      - "non-interactive slack-webhook selected without webhook URL"
      - "non-interactive gmail-mcp selected without recipient or subject"
    user_result: "stderr message와 exit code 2"

  - code: CANCELLED
    cases:
      - "TTY wizard에서 Ctrl-C, q, Escape"
    user_result: "Cancelled. message와 exit code 2"

  - code: CONFIG_READ_FAILURE
    cases:
      - "config.json invalid or unreadable"
    user_result: "Could not read config file message와 exit code 2"

  - code: FILESYSTEM_WRITE_FAILURE
    cases:
      - "output dir write denied"
      - "skill dir write denied"
      - "config dir write denied"
      - "symlink overwrite refused"
    user_result: "stderr message와 non-zero exit; CliError면 2, unexpected면 1"

  - code: SESSION_DISCOVERY_EMPTY
    cases:
      - "selected source has no readable artifact for today"
    user_result: "success exit, summary includes missing-session guidance"
```

## 10. Test Fixtures And Acceptance

```yaml
acceptance_scenarios:
  - id: A_HELP_VERSION
    usecase: [UC_SHOW_HELP, UC_SHOW_VERSION]
    expects:
      - "--version stdout equals package.json version"
      - "--help lists supported flags"
      - "exit code 0"

  - id: A_NON_INTERACTIVE_ALL_OUTPUTS
    usecase: UC_NON_INTERACTIVE_RUN
    command: "ai-bricklaying --non-interactive --target-agent opencode --sources opencode --output-modes gmail-mcp,slack-webhook"
    expects:
      - "summary markdown exists"
      - "metadata json exists"
      - "Slack payload json exists"
      - "config json exists"
      - "SKILL.md exists"
      - "stdout includes generated skill command and refresh command"
      - "NO_COLOR suppresses ANSI"

  - id: A_CONFIG_DEFAULT_REUSE
    usecase: UC_REUSE_CONFIG_DEFAULTS
    expects:
      - "saved defaults fill missing non-interactive flags"
      - "command-line flags override saved config"
      - "saved config is rewritten with cli_version"

  - id: A_INTERACTIVE_LINE_MODE
    usecase: UC_INTERACTIVE_SETUP_LINE_MODE
    expects:
      - "line-mode target list shows checkbox markers"
      - "file save is shown as required"
      - "source choices are limited to selected targets"
      - "configured Slack secret is not printed"

  - id: A_MULTI_TARGET_SINGLE_SOURCE
    usecase: UC_SELECT_MULTIPLE_TARGETS_SINGLE_SOURCE
    expects:
      - "multiple target skill paths exist"
      - "metadata target_agents contains all selected targets"
      - "metadata sessions contains one source"

  - id: A_FILE_ONLY_SKILL_NO_SEND
    usecase: UC_GENERATE_SKILL
    expects:
      - "generated skill includes file-only no external delivery instruction"
      - "generated skill omits gmail/slack delivery instructions when not selected"

  - id: A_SLACK_PAYLOAD_HANDOFF
    usecase: UC_PREPARE_SLACK_PAYLOAD
    expects:
      - "Slack payload json exists"
      - "payload has text, blocks, messages"
      - "fallback text has no Markdown heading marker"
      - "summary says Slack webhook URL is configured"
      - "raw webhook secret is not printed"

  - id: A_GMAIL_HANDOFF
    usecase: UC_PREPARE_GMAIL_HANDOFF
    expects:
      - "summary records recipient and subject"
      - "generated skill includes gmail-mcp instruction only when selected"
      - "CLI does not send email directly"

  - id: A_MISSING_SESSION_SUCCESS
    usecase: UC_HANDLE_MISSING_SESSIONS
    expects:
      - "exit code 0"
      - "summary includes no clear session signals guidance"
      - "generated skill still exists"

  - id: A_VALIDATION_FAILURES
    usecase: UC_NON_INTERACTIVE_RUN
    expects:
      - "multiple sources fail with exit code 2"
      - "source outside selected targets fails with exit code 2"
      - "missing Slack webhook fails with exit code 2"
      - "unsafe skill name fails with exit code 2 and does not create escaped path"

  - id: A_FILESYSTEM_SECURITY
    usecase: UC_WRITE_ARTIFACTS
    expects:
      - "config directory mode 0700 on POSIX"
      - "config file mode 0600 on POSIX"
      - "summary symlink overwrite is refused"

  - id: A_SECRET_REDACTION
    usecase: UC_DISCOVER_TODAY_SESSIONS
    expects:
      - "summary does not include Slack webhook path secret"
      - "summary does not include bearer token"
      - "summary does not include password/API key value from structured JSON"
      - "summary does not include raw /Users/ path"
```

## README 동기화 규칙

- `README.md`와 `README.ko.md`의 옵션 목록은 `ssot/interfaces/ai-bricklaying-cli.md`의 flags와 이 문서의 usecase/output contract를 풀어 쓴 것이다.
- README에 새 option, output file, delivery promise, config behavior를 추가하면 interface, domain usecase, output contract, acceptance를 함께 갱신한다.
