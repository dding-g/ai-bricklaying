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

CLI 도메인은 legacy single-source summary와 delivery handoff를 유지하면서, Claude generated skill이 사용하는 local daily worklog data plane, interview state, confirmed local artifact를 제공한다.

## 1. Service Overview

```yaml
service:
  name: ai-bricklaying
  interface: Go CLI
  implementation_binary: "dist/ai-bricklaying-<platform>-<arch>"
  npm_launcher: bin/ai-bricklaying.js
  package_bin: ai-bricklaying
  primary_distribution: npm
  launcher_runtime_node: ">=18 when invoked through npm or npx"
  first_release_binaries: [darwin-arm64, darwin-amd64, linux-amd64, linux-arm64]
  unsupported_platform_exit: 1
  core_value: "당일 여러 AI coding session history와 사용자 확인을 결합해 재개 가능한 daily worklog로 축적한다."
```

### 주요 사용자

- OpenCode, Claude Code, Codex, Cursor, GitHub Copilot을 사용하며 session history를 매일 회고하고 싶은 개발자
- Agent workflow를 compound engineering 자산으로 남기려는 사용자
- Legacy summary를 file로 저장하고 필요할 때 Gmail MCP 또는 Slack webhook handoff를 준비하려는 사용자
- CI/local automation에서 prompt 없이 같은 artifact를 생성하려는 사용자

### Interface 관계

- Command, flag, stdout/stderr, exit code의 외부 interface는 `ssot/interfaces/ai-bricklaying-cli.md`가 정본이다.
- npm 사용자는 계속 `ai-bricklaying` 또는 `npx ai-bricklaying`을 호출한다. npm package의 `bin/ai-bricklaying.js`는 bundled Go binary를 실행하는 launcher이며, CLI product behavior는 Go binary가 구현한다.
- 이 문서는 그 interface가 어떤 제품 usecase, output artifact, invariant, acceptance에 연결되는지 정의한다.

### 포함 범위

- Target AI agent 하나 이상 선택
- Selected target 중 summary source 하나 선택
- Summary language 선택
- Local session artifact best-effort discovery
- Daily reflection Markdown 생성
- Claude generated skill이 사용하는 consent-aware daily worklog machine protocol
- Generated Claude skill이 catalog source key 5개를 명시하고 adapter별 실제 coverage를 구분하는 daily worklog flow
- Daily interview의 draft/interviewing/confirmed 상태, revision conflict 방지, 중단 후 재개
- Confirmed worklog Markdown과 structured JSON의 owner-only 저장
- Per-day state lock과 output/date finalize artifact lock
- Metadata JSON 생성
- Slack-ready payload JSON 생성
- Generated skill `SKILL.md` 설치
- Local config default와 delivery setting 저장
- Secret redaction과 filesystem safety policy 적용

### 비포함 범위

- LLM API를 호출해 abstractive summary를 생성하는 기능
- CLI가 Gmail MCP를 직접 호출해 email을 보내는 기능
- CLI가 Slack webhook을 호출해 message를 보내는 기능
- Remote/cloud session history 동기화
- Secret vault 관리
- Generated artifact를 정본 문서로 관리하는 workflow
- ChatGPT custom MCP 배포와 remote scheduler 운영은 planned/not shipped다. 후속 adapter는 daily machine contract를 재사용한다.
- Monday-Sunday weekly review command, state, artifact는 planned/not shipped다. Daily dogfood gate 이후 후속 범위다.
- Confirmed worklog의 Gmail/Slack delivery. Phase 1A worklog는 local-only이며 기존 delivery mode는 legacy summary에만 적용된다.

Host가 별도로 제공하는 scheduler는 generated skill을 깨울 수 있지만, stored state에서 consent나 final confirmation을 추론하거나 `disclose`, `finalize`, legacy delivery를 자동 실행하는 제품 기능으로 간주하지 않는다.

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
    purpose: "설치된 npm package version을 확인한다."
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
      language: "legacy summary template에 들어갈 출력 언어. daily flow는 이를 Korean/English로 normalize하고 그 외 값은 English로 fallback한다."
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
      - "--skill-name은 1~64자의 lowercase a-z/숫자/단일 hyphen slug이고 leading/trailing/consecutive hyphen, dot, underscore는 실패한다."
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
      - "각 file은 최대 16 MiB까지 scan하고, legacy summary record의 extracted text는 최대 20,000자로 제한한다."
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
      - "legacy summary template language instruction uses the selected setup language."
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
      - "모든 selected target을 먼저 preflight하고, 같은 config path와 skill name의 generated owner 또는 정확히 식별되는 이전 generated skill만 갱신한다. 사용자 작성 file, malformed/wrong-owner skill, nonempty foreign directory, symlink와 충돌하면 어느 target도 사전 계획 단계에서 쓰지 않고 exit 2다."
      - "기존 output symlink overwrite는 거부한다."
      - "replace 대상 file은 temp file sync 후 rename하고, confirmed JSON/Markdown 신규 artifact는 temp file sync 후 atomic hard-link create-if-absent로 publish한다."

  - id: UC_PREPARE_SLACK_PAYLOAD
    trigger: "--output-modes slack-webhook 또는 saved config output_modes includes slack-webhook"
    actor: user_or_automation
    purpose: "Summary Markdown을 Slack Block Kit payload JSON으로 변환해 저장한다."
    required_input:
      - "non-interactive mode에서는 --slack-webhook-url 또는 saved config delivery.slack_webhook_url"
    interactive_behavior:
      - "interactive mode에서는 Slack webhook 입력이 optional이며 missing이면 not provided/configured false로 기록한다."
      - "Unix TTY에서는 secret 입력 중 terminal echo를 끄고 q/Esc/Ctrl-C 취소를 지원한다. hidden reader failure는 echoed input으로 fallback하지 않고 fail closed한다."
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
      - "skill directory basename도 --skill-name과 정확히 같다."
      - "current first adapter는 Claude/Claude Code이며 새 설치 기본 target은 claude-code다."
      - "daily prepare request에는 protocol_version 1.0과 opencode, claude-code, codex, cursor, github-copilot source 배열을 명시한다. legacy --sources 하나의 의미는 바꾸지 않는다."
      - "daily interview/worklog 질문과 artifact는 deterministic Korean/English template만 사용하고 다른 legacy language 설정은 English로 fallback한다."
      - "재사용 실행 전에 ai-bricklaying이 host local PATH에 있는지 확인하고, 없으면 global install 안내 후 중단한다. host는 local shell과 source file 접근을 허용해야 한다."
      - "delivery mode section은 current run의 selected modes만 설명한다."
      - "file-only mode에서는 Gmail/Slack을 시도하지 말라는 guardrail을 포함한다."
      - "gmail/slack mode에서는 missing recipient/webhook/auth를 추측하지 말고 보고하라고 지시한다."
      - "slack-webhook mode에서는 saved Markdown을 Slack delivery source of truth로 삼고, Markdown 최종본에서 Block Kit payload를 만들며, 사용자 명시 없이는 Slack 전용 short summary를 만들지 말라는 guardrail을 포함한다."
      - "slack-webhook mode에서는 Markdown section/bullet order 유지와 전송 전 top-level section coverage 검증을 지시한다."
      - "gmail/slack 안내는 legacy summary handoff이며 confirmed worklog delivery가 아님을 명시한다."

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
    purpose: "npm package 업데이트 후 기존 config defaults로 generated skill을 재생성한다."
    postconditions:
      - "selected target skill directory의 SKILL.md가 최신 template으로 갱신된다."

  - id: UC_MACHINE_PREPARE_DAILY
    trigger: "ai-bricklaying machine daily prepare"
    actor: claude_skill_or_future_mcp
    purpose: "오늘의 여러 local AI source를 수집하고 private resumable flow를 만든다."
    inputs: [protocol_version_1_0, date_optional, timezone_optional, language_optional, sources_optional, output_dir_optional, config_dir_optional]
    outputs: [machine_json_public_flow_without_evidence]
    postconditions:
      - "새 flow이면 state=draft, revision=1로 생성한다."
      - "같은 config_dir/date flow가 이미 있으면 stored sources/coverage, output_dir, language, timezone, state, revision을 유지하고 새 profile input을 적용하지 않는다."
      - "stdout에는 public flow control metadata와 source coverage가 있고 evidence text는 없다. generated skill은 동의 전에 provider/source/status/count만 사용자에게 보여준다."
      - "language가 정확한 ko, BCP-47 형식의 ko-*, 또는 case-insensitive Korean/한국 substring을 포함하면 Korean으로 normalize한다. 그 밖의 request 또는 saved legacy language 값은 모두 English로 fallback한다."
    source_resolution:
      - "generated Claude skill은 catalog source key 5개를 sources 배열에 명시하고 unsupported schema/storage를 no_activity와 구분한다."
      - "sources를 생략한 direct caller는 saved configured summary source 하나, 그것도 없으면 claude-code 하나를 사용한다."

  - id: UC_MACHINE_STATUS_DAILY
    trigger: "ai-bricklaying machine daily status"
    actor: claude_skill_or_future_mcp
    purpose: "요청한 date의 flow와 다음 행동을 read-only로 조회한다. generated skill은 현재 local date만 조회해 같은 날짜 flow만 resume한다."
    required_input: [protocol_version_1_0]
    optional_input: [date, timezone, config_dir]
    outputs: [machine_json_without_evidence]

  - id: UC_MACHINE_DISCLOSE_DAILY
    trigger: "ai-bricklaying machine daily disclose"
    actor: claude_skill_or_future_mcp
    purpose: "사용자의 명시적 boolean 결정을 저장하고, true일 때만 bounded best-effort-redacted evidence를 host agent에 제공한다. false는 denied를 저장하고 evidence 없이 성공한다."
    required_input: [protocol_version_1_0, flow_id, date, expected_revision, idempotency_key, consent_boolean]
    outputs: [machine_json_with_untrusted_evidence_when_granted, machine_json_without_evidence_when_denied]

  - id: UC_MACHINE_CHECKPOINT_DAILY
    trigger: "ai-bricklaying machine daily checkpoint"
    actor: claude_skill_or_future_mcp
    purpose: "title 교정과 reflection 답변을 답변마다 원자 저장한다."
    required_input: [protocol_version_1_0, flow_id, date, expected_revision, idempotency_key, work_items, no_work_confirmed, reflection, interview]
    postconditions:
      - "state is interviewing"
      - "revision increases once"

  - id: UC_MACHINE_FINALIZE_DAILY
    trigger: "ai-bricklaying machine daily finalize"
    actor: claude_skill_or_future_mcp
    purpose: "최종 preview를 사용자가 확인한 뒤 confirmed worklog를 저장한다."
    required_input: [protocol_version_1_0, flow_id, date, expected_revision, idempotency_key, user_confirmed]
    preconditions:
      - "flow state is interviewing"
      - "interview stage is preview, next_question is empty, and title_review/reflection_result/reflection_difficulty_feeling/reflection_learning_next are completed"
      - "every work item is confirmed or excluded"
      - "at least one confirmed item exists, or no_work_confirmed=true was explicitly checkpointed"
    outputs: [confirmed_markdown, confirmed_json, machine_json]
    postconditions:
      - "confirmed flow는 read-only이며 다시 interviewing으로 열거나 같은 날짜 artifact를 덮어쓰지 않는다."
      - "confirmed worklog는 Phase 1A local-only다."
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
    default_skill_dirs:
      opencode: "~/.config/opencode/skills"
      claude-code: "~/.claude/skills"
      codex: "~/.codex/skills"
      cursor: "~/.cursor/skills"
      github-copilot: "$COPILOT_HOME/skills when COPILOT_HOME is non-empty, otherwise ~/.copilot/skills"

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
      language: "user-selected legacy lightweight summary language"
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
      language_instruction: "normalized Korean/English interview/worklog template 선택을 포함하며 unsupported legacy language는 English로 fallback"

  DailyWorklogFlow:
    meaning: "host agent와 분리된 CLI-owned daily interview state"
    fields:
      schema_version: "1.0"
      flow_id: "crypto-random opaque ID"
      date: "request IANA timezone의 YYYY-MM-DD. 생략 시 resolved process local timezone/today"
      language: "Korean 또는 English로 normalize된 daily locale. unsupported legacy/request 값은 English"
      state: "draft, interviewing, confirmed"
      revision: "새로 commit된 non-duplicate mutation마다 증가하는 integer. 같은 idempotency key의 동일 재시도는 증가하지 않는다."
      consent: "remote evidence disclosure granted/denied/pending"
      coverage: "source별 status와 발견/사용 count"
      evidence: "common credential을 best-effort redaction한 bounded, untrusted excerpt. discovery record path field는 없지만 자유 텍스트의 모든 path-like 문자열 제거를 보장하지 않는다."
      work_items: "stable ID, title, one-line evidence_summary, one-line uncertainty, performed, outcome, verification, issues, evidence IDs, status, origin"
      work_item_status: "candidate, confirmed, excluded. session-derived candidate는 사용자가 확인하기 전까지 proposed/untrusted다."
      work_item_origin: "session_inference, user_recall, session_and_user"
      no_work_confirmed: "accepted work item이 없을 때 사용자가 기록할 업무 없음으로 명시적으로 확인했는지 여부"
      reflection: "meaningful result, difficulty, feeling, learning, next action"
      interview: "stage, completed_questions, next_question ID. 허용 ID는 title_review, reflection_result, reflection_difficulty_feeling, reflection_learning_next, preview, complete다."
      idempotency_keys: "중복 mutation을 막는 durable audit list. 최대 100개이며 한도 뒤 새 mutation은 거부한다."

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
      - "configured Slack webhook raw value"
      - "value matching the supported Bearer token pattern"
      - "value matching the supported password or API key pattern"

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
      - "configured summary directory"
      - "metadata/config/payload path references"
      - "file-only mode no-send instruction when only file selected"
      - "protocol_version 1.0 machine command instruction"
      - "all five catalog source keys and per-adapter coverage caveats in daily prepare profile"
      - "local ai-bricklaying PATH and shell/source access preflight"
      - "legacy summary delivery is not confirmed worklog delivery"

  daily_flow_state:
    required_when: "machine daily prepare"
    filename: "<config-dir>/state/v1/daily/YYYY-MM-DD.json"
    permission: "0600; parent 0700"
    rules:
      - "discovery record path field와 full transcript를 저장하지 않는다."
      - "common credential은 best-effort로 redaction하며 알려지지 않은 secret/path-like text의 완전 제거를 보장하지 않는다."

  confirmed_daily_worklog:
    required_when: "machine daily finalize"
    markdown: "<output-dir>/worklogs/daily/YYYY-MM-DD-ai-bricklaying-worklog.md"
    json: "<output-dir>/worklogs/daily/YYYY-MM-DD-ai-bricklaying-worklog.json"
    permission: "0600; parent 0700"
    markdown_must_include: [collection_coverage, confirmed_work, daily_reflection, next_action, uncertainty]
    json_must_include: [schema_version, flow_id, date, timezone, language, state, revision, coverage, work_items, no_work_confirmed, reflection, confirmed_at]
    schema_note: "uncertainty는 coverage status/reason에서 Markdown에 파생한다. next_action은 JSON의 reflection.next_action이다."
    rules:
      - "language는 Korean 또는 English이며 각 locale의 deterministic Markdown template을 사용한다. unsupported input은 English artifact로 fallback한다."
      - "evidence excerpt field와 discovery record path field를 render하지 않는다."
      - "user-controlled work item/reflection 자유 텍스트에서 모든 secret 또는 path-like 문자열의 제거를 보장하지 않는다."
      - "Phase 1A에서는 local-only이며 legacy Gmail/Slack handoff 대상이 아니다."

  internal_lock_files:
    state_lock: "<config-dir>/state/v1/daily/YYYY-MM-DD.json.lock"
    artifact_lock: "<output-dir>/.ai-bricklaying/locks/v1/daily/YYYY-MM-DD.lock"
    permission: "0600; parent 0700"
    lifetime: "빈 coordination file로 process lock release 뒤에도 남을 수 있다. 공유 artifact가 아니다."
```

## 5. State Machine And Invariants

```yaml
legacy_generation_events:
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

daily_flow_states: [draft, interviewing, confirmed]
daily_consent_states: [pending, granted, denied]
daily_interview_step_ids: [title_review, reflection_result, reflection_difficulty_feeling, reflection_learning_next, preview, complete]
error_outcomes: [validation_error, revision_conflict, flow_busy, artifact_busy, artifact_conflict, mutation_limit_reached, unexpected_failure]

invariants:
  - id: ALWAYS_SAVE_FILE
    rule: "모든 successful generation run은 summary markdown을 저장한다."
  - id: FILE_MODE_ALWAYS_INCLUDED
    rule: "output modes에는 항상 file이 포함된다."
  - id: NO_IMPLICIT_SEND
    rule: "CLI는 Gmail이나 Slack으로 직접 전송하지 않는다. Generated skill의 selected delivery mode 안내는 legacy lightweight summary에만 적용되며 Phase 1A confirmed worklog는 local-only다."
  - id: SOURCE_WITHIN_TARGETS
    rule: "summary source는 selected target agent 중 하나여야 한다. --target-agent 없이 known --sources 하나만 명시한 legacy invocation은 source를 sole target으로 승격하며, 두 flag를 모두 명시한 mismatch는 실패한다."
  - id: SINGLE_SOURCE
    rule: "한 run의 summary source는 정확히 하나다."
  - id: MULTI_TARGET_ALLOWED
    rule: "target agent는 하나 이상 선택할 수 있다."
  - id: MISSING_SESSIONS_STILL_SUCCEED
    rule: "오늘 session artifact를 찾지 못해도 summary와 skill은 생성한다."
  - id: SECRET_REDACTION
    rule: "session snippet과 stdout에는 알려진 common credential pattern을 best-effort로 redaction한다. 알려지지 않은 secret 형식까지 완전 탐지한다고 보장하지 않는다."
  - id: CONFIG_FLAGS_PRECEDENCE
    rule: "command-line flags가 saved config보다 우선한다."
  - id: GENERATED_OUTPUT_NON_CANONICAL
    rule: "summary, metadata, Slack payload, generated SKILL.md는 실행 결과물이지 SSOT 정본이 아니다."
  - id: DAILY_CLI_SOLE_WRITER
    rule: "generated skill과 future MCP는 daily state/worklog를 직접 쓰지 않고 machine mutation을 호출한다."
  - id: DAILY_CONSENT_GATE
    rule: "prepare와 status envelope는 evidence를 제외한 public flow control metadata와 source coverage를 반환한다. generated skill은 동의 전에 provider/source/status/count만 보여준다. disclose consent=true만 evidence를 반환하며 consent=false는 denied를 저장하고 evidence 없이 성공한다. consent 누락은 실패한다."
  - id: DAILY_OPTIMISTIC_CONCURRENCY
    rule: "mutation은 per-day process lock 안에서 expected revision이 최신일 때만 쓰고 stale write는 기존 파일을 바꾸지 않는다. 단, retained idempotency record와 command/request hash가 정확히 같은 재시도는 revision 검사 전에 성공 결과를 재사용한다."
  - id: DAILY_IDEMPOTENCY
    rule: "같은 idempotency key의 같은 operation 재시도는 중복 revision이나 artifact를 만들지 않는다. Durable mutation record는 flow당 최대 100개이며 그 뒤 새 mutation은 mutation_limit_reached로 실패한다."
  - id: DAILY_CONFIRMATION
    rule: "finalize에는 state=interviewing, preview stage, empty next_question, 네 title/reflection completed question ID, 모든 item의 confirmed/excluded 결정, user_confirmed=true가 필요하다. accepted item이 없으면 사용자가 no_work_confirmed=true를 checkpoint해야 한다."
  - id: DAILY_SAME_DATE_RESUME
    rule: "generated skill은 현재 local date의 flow만 status/resume하며 과거 날짜 flow를 오늘 interview로 자동 연결하지 않는다."
  - id: DAILY_LOCAL_ONLY_DELIVERY
    rule: "Phase 1A confirmed worklog는 local-only다. Gmail/Slack mode는 legacy lightweight summary handoff에만 적용된다."
  - id: DAILY_PRIVATE_STORAGE
    rule: "flow state는 owner-only atomic replace를 사용한다. Confirmed JSON/Markdown 신규 publish는 owner-only filesystem no-clobber를 사용하며 symlink를 거부한다. 동일 flow의 검증 가능한 interrupted finalize만 누락 파일/state를 복구한다."
    limit: "symlink ancestor 검사는 path-based이며 concurrent malicious ancestor replacement에 대한 descriptor-relative 방어는 제공하지 않는다."
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
      - "--config-dir/state/v1/daily 아래 private daily flow 작성"
      - "--config-dir/state/v1/daily 아래 state lock file 작성"
      - "--output-dir/worklogs/daily 아래 confirmed Markdown/JSON 작성"
      - "--output-dir/.ai-bricklaying/locks/v1/daily 아래 finalize artifact lock file 작성"
    denied:
      - "선택되지 않은 target skill dir 쓰기"
      - "summary output symlink overwrite"
      - "generated artifact를 SSOT 정본처럼 수동 관리"

  - id: SAFE_WRITE_POLICY
    subject: "filesystem writes"
    rules:
      - "parent directory를 먼저 생성한다."
      - "existing target이 symbolic link면 CliError로 거부한다."
      - "temp file을 wx로 만들고 contents/mode를 sync한다. Replace 대상은 rename하고 confirmed 신규 artifact는 hard-link create-if-absent로 publish한 뒤 parent directory를 sync한다."
      - "summary/metadata/skill/slack payload mode는 0644 best effort다."
      - "config file mode는 0600 best effort다."
      - "config directory mode는 0700 best effort다."
      - "daily flow와 confirmed worklog file mode는 0600이다. 해당 state/worklog directory mode는 0700이다."
      - "state/artifact lock file mode는 0600이고 lock directory mode는 0700이다. 빈 lock file은 실행 후 남을 수 있다."

  - id: SECRET_POLICY
    subject: "Slack webhook, tokens, passwords, API keys, private keys"
    allowed:
      - "private config file 저장"
      - "configured 여부 표시"
      - "metadata에 slack_webhook_configured boolean 저장"
    denied:
      - "configured Slack webhook raw value의 stdout 출력"
      - "지원 common credential pattern과 일치하는 raw value의 summary session snippet 출력"
      - "지원 common credential pattern과 일치하는 raw value의 Slack payload preview 포함"
      - "README/SSOT example에 실제 secret 형태 사용"
    guarantee_boundary:
      - "아래 common credential pattern은 best-effort redaction 대상이다."
      - "임의의 새 token 형식, 사용자 정의 secret, 자유 텍스트의 모든 path-like 문자열까지 완전 탐지하는 보장은 아니다."
    redaction_patterns:
      - "Slack webhook URL"
      - "private key PEM block"
      - "credential-bearing URL과 Basic/Bearer authorization"
      - "GitHub, OpenAI, AWS access key, JWT pattern"
      - "token/api_key/access_key/secret/password/cookie/session/authorization key-value"
      - "sk_/pk_ long token과 suspicious encoded credential-bearing line"

  - id: EXTERNAL_DELIVERY_POLICY
    subject: "Gmail MCP and Slack webhook"
    allowed:
      - "selected mode일 때 legacy summary handoff note 또는 payload 준비"
      - "generated skill이 selected delivery mode를 legacy summary에만 적용하도록 안내"
    denied:
      - "CLI가 사용자 확인 없이 직접 Gmail/Slack 전송"
      - "file-only generated skill이 Gmail/Slack 전송 시도"
      - "Phase 1A confirmed worklog를 Gmail/Slack legacy summary payload로 전달"

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
      - "first release package에는 darwin-arm64, darwin-amd64, linux-amd64, linux-arm64 Go binary만 포함한다. Windows binary는 first release shipped target이 아니다."
      - "bin/ai-bricklaying.js는 npm launcher로 유지하며 CLI implementation logic을 중복 구현하지 않는다."
      - "maintainer release command는 bun run release이며 release-it이 version bump, git tag, GitHub release, npm publish를 수행한다."
      - "package contents 변경 시 bun run pack:dry-run으로 확인한다."
      - "Go CLI behavior나 npm launcher behavior 변경 시 bun run test를 실행한다."
```

## 7. Query And Discovery Contract

```yaml
session_sources:
  - key: opencode
    default_paths: ["~/.local/share/opencode"]
    home_env_var: XDG_DATA_HOME
    home_env_behavior: "설정되면 ~/.local/share 대신 XDG_DATA_HOME을 data root로 사용한다."
    env_var: AI_BRICKLAYING_OPENCODE_DIRS
  - key: claude-code
    default_paths: ["~/.claude/projects"]
    home_env_var: CLAUDE_CONFIG_DIR
    home_env_behavior: "설정되면 ~/.claude 대신 CLAUDE_CONFIG_DIR을 root로 사용한다."
    env_var: AI_BRICKLAYING_CLAUDE_DIRS
  - key: codex
    default_paths: ["~/.codex/sessions", "~/.codex/archived_sessions"]
    home_env_var: CODEX_HOME
    home_env_behavior: "설정되면 ~/.codex 대신 CODEX_HOME을 root로 사용한다."
    env_var: AI_BRICKLAYING_CODEX_DIRS
  - key: cursor
    default_paths: ["~/.cursor/projects"]
    env_var: AI_BRICKLAYING_CURSOR_DIRS
  - key: github-copilot
    default_paths_by_platform:
      darwin: ["~/Library/Application Support/Code/User/workspaceStorage"]
      linux: ["~/.config/Code/User/workspaceStorage"]
    env_var: AI_BRICKLAYING_COPILOT_DIRS

source_path_precedence: "machine request의 PathLookup > AI_BRICKLAYING_*_DIRS explicit multi-root > tool 자체 home/data env > strict default paths"

strict_daily_adapters:
  claude-code:
    support: supported
    accepted: "~/.claude/projects 아래 main-project JSONL의 event-time visible user/assistant text"
    excluded: [system, tool, metadata, sidechain, memory, subagent]
  codex:
    support: supported
    accepted: "CODEX_HOME의 sessions/archived_sessions에서 relevant-day lifecycle item_completed > direct message > response fallback"
    timestamp_basis: event_time
    excluded: [context, tool, reasoning]
  cursor:
    support: experimental_privacy_conservative
    accepted: "~/.cursor/projects/**/agent-transcripts well-formed JSONL의 role=user <user_query> content"
    excluded: [assistant, reasoning, skill]
    timestamp_basis: "event_time; missing timestamp만 file_mtime fallback과 basis 표시"
  github-copilot:
    support: experimental
    accepted: "VS Code workspaceStorage/*/chatSessions schema v3 mutation replay; extension ID GitHub.copilot-chat 필수"
    excluded: [hidden, system, tool, thinking, other_agent]
    fail_closed: "unknown schema와 oversized dependent state를 추측하지 않는다."
  opencode:
    support: legacy_json_only
    current_storage: "opencode.db는 unsupported_storage"
    accepted: "recognized legacy JSON text part만"
    never_scan: [logs, auth]

legacy_summary_discovery:
  scope: "선택한 source 하나의 generic best-effort Discover"
  compatibility: "기존 동작을 유지하며 strict daily adapter matrix와 경로/schema 계약을 공유한다고 가정하지 않는다."

discovery_rules:
  legacy_summary_date_boundary: "CLI process local timezone의 today"
  machine_daily_date_boundary: "request date를 request IANA timezone의 local day로 해석한다. date/timezone 생략 시 resolved local today/timezone을 사용한다."
  env_override_separator: "platform path delimiter"
  candidate_roots: "file or directory"
  symbolic_links: "directory traversal에서 skip"
  unreadable_paths: "skip without failing run"
  legacy_summary_file_extensions: [.json, .jsonl, .md, .txt, .log]
  legacy_max_records_per_source: 12
  legacy_candidate_file_limit_per_source: 5000
  max_scan_bytes_per_file: 16777216
  legacy_max_extracted_chars_per_record: 20000
  machine_daily_limits:
    max_newest_adapter_matched_artifacts_retained_per_source: 5000
    candidate_selection_memory: "O(max_newest_adapter_matched_artifacts_retained_per_source)"
    directory_traversal_entry_limit: null
    wall_clock_scan_timeout: null
    max_records_per_source: 6
    max_extracted_chars_per_record: 4000
    max_excerpt_chars: "1200 + optional truncation ellipsis"
    max_work_items: 20
  machine_daily_source_status: [complete, no_activity, not_found, unreadable, parse_error, truncated, unsupported_schema, unsupported_storage]
  machine_daily_path_rule: "coverage/evidence schema는 discovery root와 record path field를 제공하지 않는다. excerpt의 common home prefix 최소화는 best-effort이며 모든 path-like 자유 텍스트 제거를 보장하지 않는다."
  json_extraction_preferred_keys: [text, content, message, prompt, response, summary]
  sensitive_json_keys: "preferred text key 밖의 branch는 evidence text extraction 대상에서 제외하고, 추출 text의 common credential pattern은 best-effort redaction한다."
  missing_root: "failure가 아니라 no session coverage로 처리한다."
```

## 8. External Dependencies And Async

```yaml
external_dependencies:
  - name: internal Slack payload converter
    used_by: "Slack payload generation"
    package_source: "bundled Go implementation"
    failure_mapping: "Unexpected runtime failure unless explicitly converted; file summary invariant should be checked after any change."
  - name: Gmail MCP
    used_by: "legacy summary handoff instruction only"
    failure_mapping: "CLI는 직접 호출하지 않으므로 missing authorization은 generated skill 실행 시 보고한다."
  - name: Slack webhook
    used_by: "legacy summary handoff payload target stored in config"
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

  - code: MACHINE_VALIDATION
    cases:
      - "invalid JSON or schema"
      - "protocol_version missing or not 1.0"
      - "flow not found"
      - "consent missing"
      - "user confirmation missing"
    user_result: "parseable machine error envelope와 exit code 2"

  - code: MACHINE_REVISION_CONFLICT
    cases:
      - "expected revision differs from stored revision"
    user_result: "latest revision과 retryable=true를 포함한 machine error envelope, no write, exit code 2"

  - code: MACHINE_FLOW_BUSY
    cases:
      - "다른 process가 같은 날짜 flow를 mutation 중이고 bounded lock wait가 끝남"
    user_result: "retryable=true와 reload/retry next action을 포함한 machine error envelope, exit code 2"

  - code: MACHINE_ARTIFACT_BUSY
    cases:
      - "다른 process가 같은 output directory/date의 confirmed artifact를 finalize 중이고 bounded lock wait가 끝남"
    user_result: "artifact_busy, retryable=true와 reload/retry next action을 포함한 machine error envelope, no write, exit code 2"

  - code: MACHINE_ARTIFACT_CONFLICT
    cases:
      - "같은 output directory/date의 confirmed Markdown 또는 JSON이 이미 존재해 현재 flow가 안전하게 소유권을 증명할 수 없음"
    user_result: "artifact_conflict, retryable=false, 기존 artifact를 바꾸지 않고 fix_request_and_retry next action을 포함한 envelope, exit code 2"

  - code: MACHINE_MUTATION_LIMIT
    cases:
      - "daily flow의 durable idempotency mutation record가 100개에 도달한 뒤 새 mutation 요청"
    user_result: "mutation_limit_reached validation envelope, no write, exit code 2"
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
      - "Unix TTY webhook input is not locally echoed and only [hidden] is rendered after entry"

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
      - "--target-agent와 --sources를 모두 명시했을 때 source outside selected targets fails with exit code 2"
      - "missing Slack webhook fails with exit code 2"
      - "unsafe explicit skill name fails with exit code 2 and does not create escaped path"
      - "explicit dot, underscore, uppercase, leading/trailing/consecutive hyphen, and names longer than 64 characters fail before skill write"

  - id: A_LEGACY_CONFIG_AND_SOURCE_COMPATIBILITY
    usecase: UC_NON_INTERACTIVE_RUN
    expects:
      - "known --sources 하나만 명시한 호출은 그 source를 sole target으로 사용한다."
      - "저장된 안전한 lowercase legacy skill name은 separator run을 단일 hyphen으로 바꿔 재저장한다."
      - "unsafe 저장값과 명시적 --skill-name은 자동 교정하지 않는다."

  - id: A_FILESYSTEM_SECURITY
    usecase: UC_WRITE_ARTIFACTS
    expects:
      - "config directory mode 0700 on POSIX"
      - "config file mode 0600 on POSIX"
      - "summary symlink overwrite is refused"

  - id: A_SECRET_REDACTION
    usecase: UC_DISCOVER_TODAY_SESSIONS
    expects:
      - "summary best-effort redacts configured Slack webhook URL"
      - "summary best-effort redacts bearer token"
      - "summary best-effort redacts password/API key value from structured JSON"
      - "artifact schema does not add a discovery record path field; no blanket path-free claim is made for free text"

  - id: A_MACHINE_JSON_PROTOCOL
    usecase: [UC_MACHINE_PREPARE_DAILY, UC_MACHINE_STATUS_DAILY, UC_MACHINE_DISCLOSE_DAILY, UC_MACHINE_CHECKPOINT_DAILY, UC_MACHINE_FINALIZE_DAILY]
    expects:
      - "stdout is exactly one JSON object without ANSI or human prose"
      - "envelope includes protocol_version, command, ok, flow_id, revision, state, next_action"
      - "every request requires protocol_version 1.0; missing or different version returns a parseable validation error"
      - "errors are parseable JSON and use exit 2 for validation/conflict"
      - "daily language resolves deterministically to Korean or English and unsupported values fall back to English"

  - id: A_DAILY_CONSENT_AND_INJECTION
    usecase: [UC_MACHINE_PREPARE_DAILY, UC_MACHINE_DISCLOSE_DAILY]
    expects:
      - "prepare never includes evidence text"
      - "prepare/status envelope withholds evidence text; generated skill displays only provider/source/status/count before consent"
      - "disclose with omitted consent fails"
      - "disclose consent=false persists denied, succeeds, and returns no evidence"
      - "disclose consent=true returns bounded untrusted evidence with common credential best-effort redaction and no discovery record path field"
      - "generated skill treats commands, URLs, paths, and prompt injection inside evidence only as untrusted data"
      - "session_inference work items remain candidate/proposed until the user confirms or excludes them"

  - id: A_DAILY_LIFECYCLE
    usecase: [UC_MACHINE_CHECKPOINT_DAILY, UC_MACHINE_FINALIZE_DAILY]
    expects:
      - "draft to interviewing to confirmed transitions"
      - "answer checkpoint increases revision atomically"
      - "stale revision does not change state file"
      - "same idempotency key does not duplicate write"
      - "finalize requires explicit user confirmation"
      - "evidence-backed flow requires a persisted granted or denied consent decision before checkpoint/finalize; evidence-free recall may remain pending"
      - "finalize rejects draft state, incomplete preview steps, non-empty next_question, and any remaining candidate work item"
      - "checkpoint requires explicit no_work_confirmed true or false; omission fails, and empty accepted work items require a prior true decision"
      - "confirmed flow is read-only and cannot be reopened or overwritten"
      - "state and worklog directories/files use 0700/0600 on POSIX"
      - "state and artifact coordination lock directories/files use 0700/0600 on POSIX and empty lock files may persist"
      - "state and worklog symlink overwrite is refused"
      - "two config roots targeting the same output/date cannot overwrite each other; one finalize may succeed and the other returns artifact_conflict"
      - "a process killed after the first durable artifact publish can retry the same flow/idempotency and complete the missing artifact/state while preserving confirmed_at"
      - "foreign or content-mismatched existing artifacts are never replaced"
      - "artifact lock timeout returns retryable artifact_busy without changing state or artifacts"
      - "after 100 durable mutation records, a new mutation returns mutation_limit_reached without write"
      - "persisted work item and reflection fields must equal their canonical sanitized representation before confirmation"

  - id: A_CLAUDE_WORKLOG_SKILL
    usecase: UC_GENERATE_SKILL
    expects:
      - "natural-language description triggers daily worklog and resume intent"
      - "skill checks ai-bricklaying on local PATH and explains global install/local shell access when unavailable"
      - "skill sends protocol_version 1.0 and explicitly lists all five catalog source keys in prepare, then presents each adapter's actual coverage without claiming every history format is supported"
      - "skill calls machine commands and never edits state/worklog directly"
      - "skill asks one question at a time and targets 3-5 minutes"
      - "skill displays stable candidate numbers, title, one-line basis, and uncertainty without numeric confidence"
      - "skill persists allowlisted question IDs but renders human questions from deterministic Korean/English templates; unsupported legacy language falls back to English"
      - "skill supports accept, rename, merge, split, exclude, add, skip, back, and resume"
      - "back only restores the immediate in-memory snapshot in the current conversation; a new session resumes the latest persisted checkpoint without undo history"
      - "skill only resumes the current local date and does not route an older flow into today's interview"
      - "skill obtains explicit yes/no consent before evidence disclosure; denial persists and continues with manual recall"
      - "a host-scheduled invocation is only a wake-up and does not auto-disclose, auto-finalize, or auto-deliver without current explicit decisions"
      - "skill explains that confirmed worklogs are local-only and legacy Gmail/Slack handoff is not worklog delivery"
```

## README 동기화 규칙

- `README.md`와 `README.ko.md`의 옵션 목록은 `ssot/interfaces/ai-bricklaying-cli.md`의 flags와 이 문서의 usecase/output contract를 풀어 쓴 것이다.
- README에 새 option, output file, delivery promise, config behavior를 추가하면 interface, domain usecase, output contract, acceptance를 함께 갱신한다.
