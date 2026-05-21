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

`ai-bricklaying`은 AI coding session history를 lightweight daily reflection으로 요약하고, 선택한 AI agent skill directory에 reusable skill을 설치하는 CLI다.

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

- 사용자는 AI coding session을 매일 다시 읽지 않고도 오늘의 교훈, 개선점, 다음 prompt를 남기고 싶다.
- CLI는 selected source의 local session artifact를 best-effort로 읽고, 요약 Markdown과 reusable skill을 생성한다.
- File save는 항상 수행한다.
- Gmail MCP와 Slack webhook은 selected mode일 때만 handoff 정보를 만들거나 payload를 준비한다.
- Secret과 private session path는 요약/출력에서 노출하지 않는다.

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
    decision: "CLI는 Gmail/Slack을 직접 전송하지 않는다. 선택된 mode는 local handoff/payload와 generated skill instruction으로만 반영한다."
    evidence:
      - README.md Gmail MCP Delivery
      - README.md Slack Delivery
      - tests/cli-node.test.js delivery tests
```

## 변경 영향 판단

- CLI option, output file name, delivery behavior가 바뀌면 `ssot/interfaces/ai-bricklaying-cli.md`, `ssot/domains/cli/index.md`, `README.md`, `README.ko.md`, `tests/cli-node.test.js`를 함께 확인한다.
- Generated skill instruction이 바뀌면 compound engineering sections와 no-send invariant가 유지되는지 확인한다.
- npm package contents가 바뀌면 `package.json#files`와 `bun run pack:dry-run` 결과를 함께 확인한다.
