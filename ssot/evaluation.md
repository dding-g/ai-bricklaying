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

- `README.md`와 `README.ko.md`는 사용자 flow, output modes, config defaults, install path를 잘 설명한다.
- `tests/cli-node.test.js`는 no-send, Slack payload, config defaults, secret redaction, path safety, source validation을 강하게 검증한다.
- `package.json`은 npm public entrypoint를 `bin/ai-bricklaying.js`로 고정한다.
- Python package와 Node CLI가 동시에 존재하므로, 변경자는 public npm behavior와 Python regression surface를 혼동하지 않아야 한다.
- 과거 session 결정에서 확인된 핵심 철학은 `mandatory file save`, `no implicit external send`, `compound engineering summary sections`, `missing session still produces artifact`다.

## Drift 위험

```yaml
drift_risks:
  - id: DUAL_RUNTIME_CONFUSION
    risk: "Python package와 Node CLI가 서로 다른 behavior를 갖게 될 수 있다."
    mitigation: "AGENTS.md와 SSOT에서 npm public entrypoint를 명시하고, 변경 시 두 test surface를 확인한다."

  - id: README_FIRST_CONTRACT
    risk: "README에 새 option이나 delivery promise가 먼저 추가될 수 있다."
    mitigation: "README 변경 시 ssot/domains/cli/index.md의 usecase/output/acceptance를 함께 갱신한다."

  - id: DELIVERY_SIDE_EFFECT_CREEP
    risk: "CLI의 Slack/Gmail mode가 payload 준비를 넘어 직접 전송으로 바뀌거나, generated skill의 selected-mode delivery instruction과 혼동될 수 있다."
    mitigation: "NO_IMPLICIT_SEND invariant와 tests를 유지한다."

  - id: SECRET_EXPOSURE
    risk: "Session excerpts나 config echo에서 webhook/token이 노출될 수 있다."
    mitigation: "secret redaction tests와 private config permission checks를 유지한다."
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
    change: "Node CLI와 Python package의 관계를 별도 ADR 또는 README maintainer note로 정리한다."
    reason: "현재 public npm path와 Python tests가 공존해 신규 agent가 entrypoint를 혼동할 수 있다."

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
  - "tests/cli-node.test.js 또는 Python tests가 변경된 계약을 검증하는가"
  - "file save always-on invariant가 유지되는가"
  - "Gmail/Slack no implicit send invariant가 유지되는가"
  - "secret redaction과 private config permission이 약화되지 않았는가"
  - "compound engineering template sections가 유지되는가"
```
