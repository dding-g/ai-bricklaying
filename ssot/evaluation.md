---
title: SSOT 평가와 개선
description: ai-bricklaying SSOT 문서 세트의 품질 기준, 현재 평가, 개선 backlog를 기록한다
ref:
  - ssot/index.md
  - ssot/rules.md
  - ssot/interfaces/ai-bricklaying-cli.md
  - ssot/domains/cli/index.md
---

# SSOT 평가와 개선

## 평가 기준

```yaml
evaluation_axes:
  contract_completeness:
    question: "AI agent가 SSOT만 보고 public CLI behavior를 바꿀 수 있는가"
  implementation_alignment:
    question: "SSOT, README, package metadata, tests가 같은 계약을 말하는가"
  verification_readiness:
    question: "변경자가 어떤 test와 manual check를 해야 하는지 명확한가"
  compound_engineering_fit:
    question: "요약 결과가 다음 작업을 더 쉽게 만드는 reusable asset을 남기는가"
  secret_and_side_effect_safety:
    question: "외부 전송과 secret handling의 금지선이 명확한가"
```

## 현재 평가

```yaml
current_assessment:
  contract_completeness: good
  implementation_alignment: mixed
  verification_readiness: good
  compound_engineering_fit: good
  secret_and_side_effect_safety: good
```

### 근거

- `README.md`와 `README.ko.md`는 legacy summary와 Claude-first daily interview, consent, same-date resume, local-only confirmed worklog의 경계를 설명한다.
- Go/Node tests는 legacy no-send/Slack/config/redaction/path/source 계약과 daily protocol 1.0, consent, revision/idempotency, lock/artifact conflict, confirmed lifecycle을 검증한다.
- `package.json`은 npm public entrypoint를 `bin/ai-bricklaying.js` launcher로 고정하고, CLI behavior는 bundled Go binary가 구현한다.
- Legacy Python package surface는 Go contract equivalent 통과 후 제거되었으므로, 변경자는 public npm invocation과 Go CLI contract를 기준으로 판단해야 한다.
- 핵심 철학은 `mandatory file save`, `no implicit external send`, `compound engineering legacy summary`, `explicit daily evidence consent`, `CLI sole writer`, `same-date resume`, `local-only confirmed worklog`다.
- Phase 1A daily locale은 deterministic Korean/English이며 unsupported legacy/request language는 English로 fallback한다.

## Drift 위험

```yaml
drift_risks:
  - id: DUAL_RUNTIME_CONFUSION
    risk: "제거된 Python package surface가 문서나 테스트 기대값에 남아 Go CLI contract와 혼동될 수 있다."
    mitigation: "AGENTS.md와 SSOT에서 npm launcher와 Go implementation binary의 역할을 분리하고, 변경 시 Go/Node contract tests를 확인한다."

  - id: README_FIRST_CONTRACT
    risk: "README에 새 option이나 delivery promise가 먼저 추가될 수 있다."
    mitigation: "README 변경 시 ssot/domains/cli/index.md의 usecase/output/acceptance를 함께 갱신한다."

  - id: DELIVERY_SIDE_EFFECT_CREEP
    risk: "Legacy summary용 Slack/Gmail mode가 Phase 1A confirmed worklog delivery처럼 보이거나 payload 준비를 넘어 직접 전송으로 바뀔 수 있다."
    mitigation: "NO_IMPLICIT_SEND와 DAILY_LOCAL_ONLY_DELIVERY invariant를 함께 유지한다."

  - id: SECRET_EXPOSURE
    risk: "Session excerpts나 config echo에서 webhook/token이 노출될 수 있다."
    mitigation: "common credential best-effort redaction tests, untrusted-data boundary, private config permission checks를 유지하고 완전 redaction을 약속하지 않는다."

  - id: ADAPTER_SCOPE_CONFUSION
    risk: "Claude generated skill, future ChatGPT MCP, unshipped weekly flow가 모두 현재 기능처럼 문서화될 수 있다."
    mitigation: "Claude-first shipped adapter와 planned ChatGPT/weekly scope를 README와 SSOT에서 분리한다."

  - id: DAILY_LOCALE_DRIFT
    risk: "Legacy summary의 자유 형식 language 설정이 daily interview/worklog의 arbitrary-language 지원처럼 해석될 수 있다."
    mitigation: "Daily protocol과 generated skill은 Korean/English normalization 및 English fallback contract test를 유지한다."

  - id: DAILY_STATE_CONTRACT_DRIFT
    risk: "Generated skill이 machine command/version, consent false persistence, revision/idempotency, preview/no-work 조건을 빠뜨리거나 state/worklog를 직접 수정할 수 있다."
    mitigation: "prepare/status/disclose/checkpoint/finalize protocol 1.0 acceptance와 generated skill content test를 유지한다."

  - id: PRIVATE_ARTIFACT_SCOPE_DRIFT
    risk: "Private daily state, coordination lock, confirmed worklog가 legacy Gmail/Slack artifact 또는 canonical SSOT처럼 취급될 수 있다."
    mitigation: "owner-only permission, local-only delivery, generated-output noncanonical 규칙과 artifact collision tests를 유지한다."
```

## 개선 Backlog

```yaml
improvements:
  - id: I_INTERFACE_LINT
    priority: high
    change: "ssot/interfaces/ai-bricklaying-cli.md와 README option table, tests/cli-node.test.js validation cases를 교차 검사하는 script를 추가한다."
    reason: "CLI flag 계약은 가장 drift가 나기 쉬운 외부 interface다."

  - id: I_RUNTIME_DECISION_RECORD
    priority: high
    change: "npm launcher와 Go implementation binary의 관계를 별도 ADR 또는 README maintainer note로 정리한다."
    reason: "npm invocation은 유지되지만 runtime 구현이 Go로 이동하므로 신규 agent가 launcher와 implementation을 혼동할 수 있다."

  - id: I_DOC_LINK_FROM_README
    priority: medium
    change: "README maintainer section에서 ssot/index.md를 링크한다."
    reason: "사용자 문서와 maintainer 정본의 역할을 분리한다."

  - id: I_SSOT_LINT_SCRIPT
    priority: medium
    change: "frontmatter key와 required section heading을 검사하는 lightweight script를 추가한다."
    reason: "syncly-social-spec처럼 SSOT drift를 자동으로 잡을 수 있다."

  - id: I_OUTPUT_CONTRACT_FIXTURES
    priority: medium
    change: "generated summary, metadata, Slack payload의 fixture snapshot을 최소화해 추가한다."
    reason: "Output contract 변경을 리뷰하기 쉬워진다."

  - id: I_PRINTED_PATH_SECRET_TEST
    priority: medium
    change: "--output-dir, --skill-dir, --config-dir에 secret-looking 문자열이 들어갈 때 stdout redaction을 구현할지, 아니면 path secret 금지 정책만 유지할지 결정하고 테스트를 추가한다."
    reason: "현재 CLI는 printed path에서 control character만 제거한다."

  - id: I_SESSION_SOURCE_DOCS
    priority: low
    change: "각 source의 실제 default path와 env override를 README 또는 SSOT 하위 섹션으로 확장한다."
    reason: "사용자가 missing session path를 스스로 복구하기 쉬워진다."
```

## Review Checklist

SSOT 또는 CLI behavior 변경 PR은 아래를 확인한다.

```yaml
review_checklist:
  - "ssot/rules.md frontmatter 규칙을 지켰는가"
  - "ssot/interfaces/ai-bricklaying-cli.md의 flag/stdout/exit/artifact 계약이 변경 behavior를 설명하는가"
  - "ssot/domains/cli/index.md의 usecase와 acceptance가 변경 behavior를 설명하는가"
  - "README.md와 README.ko.md가 같은 사용자 약속을 말하는가"
  - "tests/cli-node.test.js와 Go contract tests가 변경된 계약을 검증하는가"
  - "file save always-on invariant가 유지되는가"
  - "Gmail/Slack no implicit send invariant가 유지되는가"
  - "secret redaction과 private config permission이 약화되지 않았는가"
  - "compound engineering template sections가 유지되는가"
  - "machine request/envelope의 protocol_version 1.0과 command 이름이 generated skill/README/SSOT에서 일치하는가"
  - "consent 전 evidence withholding, consent=false denial persistence, untrusted evidence 처리가 유지되는가"
  - "same-date resume, confirmed read-only, revision/idempotency/lock/artifact conflict 계약이 유지되는가"
  - "Phase 1A confirmed worklog가 local-only이고 legacy Gmail/Slack delivery와 분리되는가"
```
