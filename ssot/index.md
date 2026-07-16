---
title: ai-bricklaying SSOT 인덱스
description: ai-bricklaying 정본 문서 세트의 진입점과 문서 책임을 정의한다
ref:
  - ssot/rules.md
  - ssot/interfaces/ai-bricklaying-cli.md
  - ssot/domains/cli/index.md
  - ssot/evaluation.md
---

# ai-bricklaying SSOT

`ai-bricklaying`은 legacy lightweight summary를 유지하면서 당일 여러 AI coding session history를 사용자 확인 daily worklog로 바꾸는 local CLI data plane과 reusable Claude skill을 제공한다.

## 문서 목록

1. [SSOT 작성 규칙](./rules.md)
2. [CLI Interface 계약](./interfaces/ai-bricklaying-cli.md)
3. [CLI 도메인 계약](./domains/cli/index.md)
4. [SSOT 평가와 개선](./evaluation.md)

## 정본 범위

```yaml
canonical_scope:
  product_contract:
    - ssot/rules.md
    - ssot/interfaces/ai-bricklaying-cli.md
    - ssot/domains/cli/index.md
  public_user_docs:
    - README.md
    - README.ko.md
  current_public_cli_entrypoint:
    - bin/ai-bricklaying.js npm launcher
  runtime_implementation:
    - bundled Go binaries under dist/
  npm_package_files:
    - assets/ai-bricklaying.png
    - bin/ai-bricklaying.js
    - dist/ai-bricklaying-darwin-arm64
    - dist/ai-bricklaying-darwin-amd64
    - dist/ai-bricklaying-linux-amd64
    - dist/ai-bricklaying-linux-arm64
    - README.md
    - README.ko.md
    - package.json
  verification:
    - tests/cli-node.test.js
    - Go contract tests
```

## 제품 정의

- 사용자는 여러 AI coding session을 매일 다시 읽지 않고도 실제 업무 제목과 경험을 확인한 local worklog를 남기고 싶다.
- Generated Claude skill은 catalog source key 5개를 daily machine protocol에 명시하고 adapter별 actual coverage를 표시한다. Key 존재는 모든 history schema 지원을 뜻하지 않으며 `unsupported_schema`/`unsupported_storage`를 `no_activity`와 구분한다. CLI는 local state와 confirmed worklog의 유일한 writer다.
- Phase 1A daily 질문과 worklog artifact는 deterministic Korean/English template을 사용하며 다른 legacy/request language는 English로 fallback한다.
- 기존 summary run은 selected source 하나의 local session artifact를 best-effort로 읽어 lightweight Markdown을 생성하는 호환 surface로 유지한다.
- Legacy summary run은 file save를 항상 수행한다. Daily flow는 confirmation 전 private checkpoint만 저장하고 confirmed worklog는 explicit finalize 뒤에만 생성한다.
- Gmail MCP와 Slack webhook은 selected mode일 때 legacy summary handoff 정보나 payload만 준비한다. Phase 1A confirmed worklog는 local-only다.
- Common credential pattern은 best-effort로 redaction하며 session-derived evidence는 항상 untrusted data로 취급한다. 자유 텍스트가 완전히 secret/path-free라고 보장하지 않는다.
- ChatGPT MCP adapter와 Monday-Sunday weekly review는 daily dogfood 이후의 계획이며 현재 배포 범위가 아니다.
- Private daily state/lock과 confirmed worklog는 사용자 실행 결과물이며 SSOT 정본 문서가 아니다.

## 주요 결정

```yaml
decisions:
  - id: CLI_RUNTIME_PUBLIC_ENTRYPOINT
    decision: "npm package의 public entrypoint는 bin/ai-bricklaying.js launcher이며 CLI behavior는 bundled Go binary가 구현한다."
    evidence:
      - package.json#bin
      - package.json#files
      - dist/ai-bricklaying-darwin-arm64
      - dist/ai-bricklaying-darwin-amd64
      - dist/ai-bricklaying-linux-amd64
      - dist/ai-bricklaying-linux-arm64
  - id: SINGLE_SUMMARY_SOURCE
    decision: "한 번의 summary run은 하나의 session source만 요약한다."
    evidence:
      - README.md CLI option rules
      - tests/cli-node.test.js rejects multiple sources
  - id: DAILY_MULTI_SOURCE_PROFILE
    decision: "Generated Claude daily skill은 prepare request에 OpenCode, Claude Code, Codex, Cursor, GitHub Copilot을 명시한다. legacy --sources 한 개의 의미는 유지한다."
    evidence:
      - ssot/interfaces/ai-bricklaying-cli.md machine command contract
      - ssot/domains/cli/index.md UC_MACHINE_PREPARE_DAILY
  - id: DAILY_PROTOCOL_AND_CONSENT
    decision: "모든 machine request는 protocol_version 1.0을 요구한다. prepare/status envelope는 evidence를 제외한 public control metadata와 source coverage를 반환하고 generated skill은 동의 전에 provider/source/status/count만 보여준다. disclose true만 evidence를 반환하며 false는 denied를 저장한다."
    evidence:
      - ssot/interfaces/ai-bricklaying-cli.md machine_daily_flow
      - ssot/domains/cli/index.md A_DAILY_CONSENT_AND_INJECTION
  - id: DAILY_KO_EN_LOCALE
    decision: "Phase 1A daily language는 Korean 또는 English로 normalize한다. unsupported legacy config와 machine request language는 English로 fallback하며 arbitrary-language 질문/문서 생성을 계약하지 않는다."
    evidence:
      - ssot/interfaces/ai-bricklaying-cli.md machine_daily_flow
      - ssot/domains/cli/index.md A_MACHINE_JSON_PROTOCOL
  - id: CLAUDE_FIRST_LOCAL_DATA_PLANE
    decision: "Claude/Claude Code generated skill이 첫 adapter이고 CLI가 daily state/worklog의 유일한 writer다. 재사용에는 host local PATH의 ai-bricklaying과 local shell/source 접근이 필요하다."
    evidence:
      - README.md Daily Worklog Interview
      - ssot/domains/cli/index.md UC_GENERATE_SKILL
  - id: DAILY_OUTPUT_NON_CANONICAL
    decision: "Private daily state/lock과 confirmed worklog는 CLI-owned 실행 결과물이고 SSOT 정본이 아니다. Phase 1A confirmed worklog는 local-only다."
    evidence:
      - ssot/domains/cli/index.md Output Contract
      - ssot/domains/AGENTS.md
  - id: MULTI_TARGET_SKILL_INSTALL
    decision: "target agent는 여러 개 선택할 수 있고, 각 target skill directory에 같은 generated skill을 설치한다."
    evidence:
      - README.md interactive setup
      - tests/cli-node.test.js multiple target test
  - id: ALWAYS_SAVE_FILE
    decision: "모든 output mode에서 local summary file은 반드시 생성한다."
    evidence:
      - README.md Outputs
      - tests/cli-node.test.js file save assertions
  - id: NO_IMPLICIT_EXTERNAL_SEND
    decision: "CLI는 Gmail/Slack을 직접 전송하지 않는다. 선택된 mode는 legacy summary의 local handoff/payload에만 반영하며 Phase 1A confirmed worklog는 local-only다."
    evidence:
      - README.md Gmail MCP Delivery
      - README.md Slack Delivery
      - tests/cli-node.test.js delivery tests
```

## 변경 영향 판단

- CLI option, output file name, delivery behavior가 바뀌면 `ssot/interfaces/ai-bricklaying-cli.md`, `ssot/domains/cli/index.md`, `README.md`, `README.ko.md`, `tests/cli-node.test.js`를 함께 확인한다.
- Generated skill instruction이 바뀌면 compound engineering sections와 no-send invariant가 유지되는지 확인한다.
- npm package contents가 바뀌면 `package.json#files`와 `bun run pack:dry-run` 결과를 함께 확인한다.
