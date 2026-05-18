---
title: ai-bricklaying SSOT 작성 규칙
description: ai-bricklaying 제품 정본 문서의 구조, 표현, 검증 기준을 정의한다
ref:
  - ssot/index.md
  - ssot/interfaces/ai-bricklaying-cli.md
  - ssot/domains/cli/index.md
---

# SSOT 작성 규칙

## 1. 목적

- SSOT는 `CLI behavior -> README -> tests -> generated skill`의 상위 입력 문서다.
- 문서는 사람이 읽기 쉬워야 하지만, 더 중요한 기준은 AI agent가 추론 없이 현재 제품 계약과 변경 범위를 뽑아낼 수 있어야 한다는 점이다.
- 이 프로젝트의 핵심 domain은 `cli` 하나이며, 새 domain을 추가하기 전에는 `ssot/index.md`에서 domain boundary를 먼저 갱신한다.
- SSOT에는 구현 회고나 임시 작업 로그를 섞지 않는다. 과거 session에서 확인된 결정은 현재 계약으로 승격된 경우에만 정본 문서에 반영한다.

## 2. 기본 원칙

- 흐름은 `SSOT -> implementation/docs/tests -> generated artifacts` 단방향으로 본다.
- README는 사용자 문서이고 SSOT는 계약 문서다. README가 SSOT보다 새 behavior를 먼저 선언하면 drift로 본다.
- Public npm behavior는 `package.json`의 `bin.ai-bricklaying`이 가리키는 `bin/ai-bricklaying.js`를 기준으로 판단한다.
- Python package surface는 테스트 대상이지만 npm 배포 계약을 자동 대표하지 않는다.
- External delivery는 opt-in이다. `file` save는 항상 수행한다.
- Secret은 정본 예시에서도 실제 값처럼 보이게 쓰지 않는다. 예시는 `https://hooks.slack.com/services/...`처럼 축약한다.
- Compound engineering 품질은 출력 template의 필수 기능이다. 요약은 `lessons`, `evidence`, `better AI usage`, `follow-up prompt`를 포함해야 한다.

## 3. 문서 구조 규칙

```text
ssot/
  index.md
  rules.md
  evaluation.md
  interfaces/
    ai-bricklaying-cli.md
  domains/
    AGENTS.md
    cli/
      index.md
```

- `ssot/index.md`는 전체 SSOT entrypoint다.
- `ssot/rules.md`는 작성/검증 공통 규칙만 담는다.
- `ssot/evaluation.md`는 SSOT 품질 평가와 개선 backlog를 담는다.
- `ssot/interfaces/ai-bricklaying-cli.md`는 command, flag, stdout/stderr, exit code, artifact path 같은 외부 CLI interface를 담는다.
- 단일 domain인 `cli`는 `index.md` 하나에 10개 축을 모두 포함한다.
- domain이 커져 `index.md`가 추론하기 어려워지면 아래 번호 문서 세트로 분리한다.

```text
01-service-overview.md
02-usecase-inventory.md
03-entity-and-field-dictionary.md
04-output-contract.md
05-state-machine-and-invariants.md
06-permissions-and-policies.md
07-query-and-discovery-contract.md
08-external-dependencies-and-async.md
09-error-model.md
10-test-fixtures-and-acceptance.md
```

## 4. frontmatter 규칙

모든 SSOT 문서는 frontmatter에 아래 3개 키만 사용한다.

```yaml
---
title: <문서 제목>
description: <문서 요약>
ref:
  - <연관된 SSOT 문서 경로>
---
```

- `title`: 문서 제목
- `description`: 문서 목적 요약
- `ref`: 함께 읽어야 하는 관련 SSOT 문서

## 5. 필수 내용 축

단일 파일로 유지하더라도 아래 축은 빠지면 안 된다.

1. Service overview: 제품 목적, 사용자, 포함/비포함 범위
2. Usecase inventory: 사용자 행동과 CLI trigger
3. Entity and field dictionary: config, summary, skill, session record 같은 저장/출력 객체
4. Output contract: Markdown, JSON metadata, Slack payload, skill file contract
5. State and invariants: wizard/non-interactive flow, always-save, no-send invariant
6. Permissions and policies: local filesystem write, secret handling, external delivery opt-in
7. Query and discovery contract: session source discovery, env var override, date boundary
8. External dependencies and async: Slack block conversion, Gmail MCP handoff, no implicit send
9. Error model: validation, IO, unsupported option, missing secret handling
10. Test fixtures and acceptance: happy path와 실패 축

## 6. 표현 규칙

- 설명형 문장만 두지 말고 YAML 구조 블록, 고정 option 목록, output file 목록, acceptance matrix를 함께 둔다.
- Option 이름은 CLI에 그대로 존재하는 spelling을 사용한다.
- Source key는 `opencode`, `claude-code`, `codex`, `cursor`, `github-copilot` 중 하나로 쓴다.
- Output mode는 `file`, `gmail-mcp`, `slack-webhook` 중 하나로 쓴다.
- Actor와 side effect를 분리한다. `summary 생성`과 `external delivery`를 같은 행동으로 뭉개지 않는다.

## 7. 검증 방법

### 7.1 구조 검증

- `ssot/index.md`가 모든 정본 문서로 연결되는가
- 각 SSOT 문서 frontmatter가 `title`, `description`, `ref`만 사용하는가
- `ssot/interfaces/ai-bricklaying-cli.md`가 CLI flag와 output file 계약을 포함하는가
- `ssot/domains/cli/index.md`가 10개 필수 축을 모두 포함하는가

### 7.2 계약 정합성 검증

- README의 CLI option 목록이 SSOT usecase와 맞는가
- `package.json`의 `bin`, `files`, `scripts`가 SSOT의 배포/검증 설명과 맞는가
- Node CLI test가 SSOT acceptance matrix의 핵심 behavior를 검증하는가
- Generated artifact 이름이 README, SSOT, test에서 서로 일치하는가

### 7.3 Compound engineering 검증

- Summary template이 단순 작업 목록이 아니라 재사용 가능한 lesson과 better AI usage를 요구하는가
- Skill output이 다음 session에서 바로 쓸 수 있는 workflow를 제공하는가
- Missing session path에서도 follow-up prompt와 template이 남는가

## 8. 작성 언어 규칙

- Markdown 본문은 한글로 작성한다.
- Code identifier, file path, command, option, package name, source key는 원문을 유지한다.
