# Document Review Results

**Document:** `PRD-ai-bricklaying-worklog.md`

**Type:** requirements

**Reviewers:** coherence, feasibility, product-lens, design-lens, security-lens, scope-guardian, adversarial

**Contract refresh:** 2026-07-16. 아래 결정은 Phase 1A 구현 계약에 맞춰 Claude-first daily scope로 갱신했으며 weekly와 ChatGPT MCP는 planned/not shipped로 분리했다.

- product-lens: 제품의 핵심 가치, habit 형성, multi-source와 multi-host의 구분을 검토했다.
- design-lens: daily/weekly interview, 편집, 실패, resume, accessibility 상태를 검토했다.
- security-lens: 로컬 session, remote host agent, credential, artifact lifecycle의 trust boundary를 검토했다.
- scope-guardian: P0/P1/P2와 release slice가 최소한의 검증 순서를 따르는지 검토했다.
- adversarial: success metric, confirmation bias, 5분 목표, weekly coverage 가정을 반증 관점에서 검토했다.

Applied 2 auto-fixes. 25 strategic findings were resolved in the revised PRD.

## Auto-fixes Applied

- daily metadata 상태를 `draft/interviewing/confirmed`로 통일하고, 미확정 본문을 “확인된 업무”가 아닌 “업무 후보”로 표현했다.
- Daily는 request IANA timezone/date를 사용하고 생략 시 process local fallback을 사용한다. 별도의 saved/configured timezone이 있다고 약속하지 않는다. Planned weekly review timezone은 Phase 1B 결정으로 분리했다.

## Strategic Decisions Applied

- Phase 1A의 기준 host agent를 Claude/Claude Code generated skill 하나로 좁히고 multi-source collection과 multi-host interview를 분리했다.
- CLI는 protocol 1.0 local data/state plane과 유일한 writer를 담당하고, Claude skill은 초안과 자연어 interview를 담당하도록 실행 경계를 고정했다.
- terminal-only AI interview와 두 번째 host agent는 P1으로 이동했다.
- daily vertical slice를 먼저 dogfood하고 gate를 통과한 뒤 weekly MVP increment를 만든다.
- source 실패, `unsupported_schema`/`unsupported_storage`, 실제 무활동을 구분하는 상태와 recovery action을 추가했다.
- 동의 전 source/status/count만 제공하고 explicit true/false를 저장하는 consent gate, untrusted session data 처리, common credential best-effort redaction fixture, owner-only permission을 P0에 추가했다.
- state mutation에 flow ID, revision, idempotency key를 요구해 stale write를 막는다.
- 숫자형 confidence를 사용자 화면에서 제거하고 근거와 불확실한 이유를 표시한다.
- 6개를 넘는 업무 후보의 concise flow와 안정적인 번호 기반 편집·undo를 정의했다.
- weekly missing day를 “AI activity가 있으나 confirmed log가 없는 날”로 정의하고 3일 미만 coverage에서는 추세를 단정하지 않는다.
- Same-date status/resume만 P0로 유지하고 cross-day nudge와 local OS notification은 후속 opt-in 범위로 뒀다.
- Legacy 단일 `--sources` summary config와 generated skill의 explicit five-catalog-key daily profile을 분리하고, key 존재를 전체 history schema 지원으로 표현하지 않도록 고쳤다.
- Claude Code/Codex strict event schema, Cursor/Copilot experimental privacy-conservative schema, OpenCode current DB의 `unsupported_storage` 계약과 각 excluded content 범위를 source matrix로 고정했다.
- Daily locale은 deterministic Korean/English로 한정하고 다른 legacy/request language는 English fallback으로 정리했다.
- Confirmed worklog는 Phase 1A local-only이며 Gmail/Slack handoff는 legacy summary 전용으로 유지했다.
- Phase 0에 최소 5개 daily flow와 명시적인 통과·중단 기준을 추가했다.

## Residual Concerns

| # | Concern | Source |
|---|---|---|
| 1 | best-effort-redacted 근거의 양과 업무 제목 정확도 사이의 trade-off는 실제 dogfood 전에는 확정할 수 없다. | feasibility, security-lens, adversarial |
| 2 | Claude host UI 자체의 screen reader 품질은 제품이 직접 통제할 수 없다. | design-lens |
| 3 | 바쁜 날의 미확정 일지가 주간 감정·장애 패턴을 과소대표할 수 있다. | product-lens, adversarial |
| 4 | 자연어 재실행과 same-date resume만으로 daily habit이 형성되는지는 2주 dogfood가 필요하다. | product-lens |
| 5 | Cursor record에 event timestamp가 없을 때의 명시적 file-mtime fallback은 자정 경계와 timezone 이동에서 일부 activity를 잘못 귀속할 수 있다. Claude Code, Codex, Copilot은 event time을 사용한다. | feasibility, adversarial |

## Deferred Questions

| # | Question | Decision point |
|---|---|---|
| 1 | weekly artifact의 최종 filename은 무엇인가? Daily filename은 SSOT에 확정됐다. | Phase 1B SSOT 변경 |
| 2 | confirmed revision history를 얼마나 보존할 것인가? | state schema 설계 |
| 3 | daily draft/state/artifact의 기본 retention 기간은 무엇인가? | privacy contract |
| 4 | 자연어 재실행만으로 부족할 때 local notification을 기본 제안할 것인가? | 2주 dogfood 이후 |

## Coverage

| Persona | Status | Findings | Auto | Resolved by decision | Residual |
|---|---|---:|---:|---:|---:|
| coherence | completed | 4 | 2 | 2 | 1 |
| feasibility | completed | 6 | 0 | 6 | 3 |
| product-lens | completed | 4 | 0 | 4 | 3 |
| design-lens | completed | 3 | 0 | 3 | 1 |
| security-lens | completed | 4 | 0 | 4 | 3 |
| scope-guardian | completed | 1 | 0 | 1 | 2 |
| adversarial | completed | 5 | 0 | 5 | 3 |

Review complete.
