# AI Bricklaying Worklog Discovery

**작성일**: 2026-07-16

**상태**: Discovery complete

**대상 제품**: 기존 `ai-bricklaying` CLI와 generated agent skill

## 1. Discovery Context

### 해결하려는 문제

AI coding agent를 여러 번 사용한 날에는 실제로 한 일, 결정한 내용, 막힌 지점, 느낀 점이 여러 세션에 흩어진다. 현재 제품은 단일 source의 당일 신호와 회고 템플릿을 만들지만, 사용자가 바로 활용할 수 있는 업무일지 초안이나 확인 인터뷰는 만들지 않는다.

### 목표 사용자

- Codex, Claude Code, OpenCode, Cursor, GitHub Copilot을 업무에 사용하는 개발자
- 하루가 끝난 뒤 여러 AI 대화를 다시 읽지 않고 업무일지를 남기고 싶은 사용자
- Daily habit이 검증된 뒤 주간 단위로 성과, 반복 장애, 감정 변화, AI 활용 습관을 돌아보고 싶은 사용자

### 원하는 결과

- 당일 여러 AI 기록을 근거로 실제 내용이 채워진 업무일지 초안을 얻는다.
- AI가 추출한 업무 제목을 사용자가 빠르게 확인하거나 교정한다.
- 3~5분 인터뷰로 기록에 없는 경험과 감정을 보완한다.
- Daily dogfood gate 이후 월요일부터 일요일까지의 확정 일지를 사용하는 주간 회고로 확장한다.
- Full transcript를 외부로 보내지 않고, common credential을 best-effort redaction한 bounded evidence만 명시적 동의 뒤 전달한다.

### 이미 확정한 제품 결정

- 단순 템플릿이 아니라 실제 업무일지 초안을 만든다.
- 초안 직후 같은 흐름에서 인터뷰를 시작한다.
- 인터뷰는 3~5분을 기본으로 하고 답변에 따라 필요한 질문만 깊게 묻는다.
- 후속 주간 회고의 범위는 월요일부터 일요일까지다. Weekly command/artifact는 현재 배포 범위가 아니다.
- AI가 관찰한 사실과 사용자가 직접 말한 경험을 구분한다.

## 2. Divergent Ideas

### Product Manager 관점

1. **Confirmed Worklog**: AI 초안과 사용자 확인을 `draft`와 `confirmed` 상태로 구분한다.
2. **Weekly Compound Review**: 확정된 일지를 모아 성과와 반복 패턴을 주간 회고로 전환한다.
3. **Private and Share Views**: 개인 회고 정본에서 민감한 내용을 제거한 공유본을 파생한다.
4. **Multi-source Daily Coverage**: catalog source key 5개를 요청하고 adapter별 실제 coverage와 unsupported 상태를 구분한다.
5. **Review Quality Metrics**: 제목 수정률, 누락 업무 수, 인터뷰 완료율로 품질을 측정한다.

### Product Designer 관점

1. **Interview Where Work Happens**: 사용 중인 AI agent 대화에서 자연어로 일지를 시작한다.
2. **Evidence-backed Title Review**: 제목, 한 줄 근거, 불확실한 이유를 함께 보여준다.
3. **Fast Confirmation Actions**: 맞음, 변경, 합치기, 나누기, 제외, 근거 보기를 짧은 답으로 처리한다.
4. **Adaptive Reflection**: 하루 전체에 대한 감정과 배움을 먼저 묻고 필요한 경우에만 후속 질문을 한다.
5. **Calm Resume Flow**: 답변마다 저장하고 중단되면 다음 실행에서 이어서 시작한다.

### Software Engineer 관점

1. **CLI Data Plane + Agent Interaction**: CLI가 redacted data와 상태를 관리하고 agent skill이 자연어 초안과 인터뷰를 담당한다.
2. **Versioned Activity Manifest**: 여러 source의 활동을 하나의 안정된 JSON contract로 정규화한다.
3. **Atomic Interview Checkpoints**: 질문별 답변과 상태 변경을 원자적으로 저장한다.
4. **Provenance and Deduplication**: source, 시간, project signal을 사용해 중복 활동을 합치고 근거를 보존한다.
5. **Provider-neutral Protocol**: 특정 model에 종속되지 않는 draft와 interview 입출력 contract를 둔다.

## 3. Prioritized Product Ideas

| 우선순위 | 아이디어 | 선택 이유 | 검증할 핵심 가정 |
|---|---|---|---|
| 1 | Evidence-backed multi-source draft | 현재 제품과 원하는 결과 사이의 가장 큰 gap을 직접 해결한다. | 여러 source에서 충분히 정확한 업무 제목과 결과를 추출할 수 있다. |
| 2 | In-agent adaptive interview | 사용자가 이미 일하는 화면에서 시작하므로 접근 비용이 가장 낮다. | 사용자는 3~5분 인터뷰를 완료하고 agent별 skill 동작이 충분히 일관된다. |
| 3 | Draft/confirmed state and resume | 중단과 잘못된 AI 추론을 안전하게 처리한다. | 질문별 저장이 인터뷰 흐름을 방해하지 않고 신뢰를 높인다. |
| 4 | Weekly compound review | 매일 일지를 남길 장기적인 이유를 만든다. | 확정 일지의 누적이 사용자가 행동할 만한 주간 통찰을 만든다. |
| 5 | Accessible CLI fallback | agent skill이 없거나 실패해도 제품을 사용할 수 있다. | 선형 line-mode 인터뷰가 screen reader와 비-TTY 환경에서도 이해 가능하다. |

## 4. Assumption Map

| 영역 | 가정 | 잘못되면 생기는 문제 | 현재 confidence | 가장 싼 검증 |
|---|---|---|---|---|
| Value | 사용자는 AI 세션을 다시 읽는 것보다 초안을 확인하는 방식을 선호한다. | 기존 메모 습관보다 제품 가치가 낮다. | Medium | 과거 5일 기록으로 concierge 초안을 만들고 실제 사용 시간을 비교한다. |
| Value | 주간 회고가 매일 인터뷰를 지속할 이유가 된다. | 초기 호기심 이후 사용이 중단된다. | Low | 2주 dogfood 후 주간 회고에서 행동 가능한 통찰 수를 측정한다. |
| Usability | 제목과 한 줄 근거만으로 업무 후보를 빠르게 판단할 수 있다. | 사용자가 원문을 계속 열어봐야 한다. | Medium | 제목 확인율, 근거 보기 비율, 수정률을 측정한다. |
| Usability | 적응형 인터뷰가 5분 안에 끝난다. | 피로 때문에 `draft`만 쌓인다. | Medium | 실제 인터뷰 시간과 완료율을 측정한다. |
| Usability | 사용자는 AI agent 안에서 자연어로 시작하는 경로를 발견할 수 있다. | 기능이 설치되어도 사용되지 않는다. | Medium | 첫 실행 안내 후 다음 날 자발적 재실행률을 측정한다. |
| Viability | local-first와 명시적 외부 전송 정책이 신뢰를 만든다. | 민감한 회고와 session 원문 때문에 도입을 거부한다. | High | preview에서 전송 범위 인지 여부와 opt-in 비율을 확인한다. |
| Feasibility | 명시적으로 지원하는 strict schema의 당일 기록을 안정적으로 발견하고 redaction할 수 있다. | 초안 coverage가 낮거나 unsupported schema를 업무 없음으로 오인하고 secret이 노출된다. | Medium | adapter별 fixture와 실제 로컬 기록에서 coverage status와 secret leakage를 검사한다. |
| Feasibility | Claude skill이 versioned CLI 상태 contract를 지킬 수 있다. | agent가 state를 직접 수정하거나 resume 동작이 달라진다. | Medium | Claude generated skill과 CLI protocol 1.0 fixture contract test를 실행한다. |
| Feasibility | 여러 source의 중복 업무를 실용적인 수준으로 합칠 수 있다. | 같은 업무가 여러 제목으로 반복된다. | Low | 동일 project의 교차-agent fixture로 duplicate rate를 측정한다. |

## 5. Assumption Priority

| 우선순위 | 가정 | Impact | Risk | 결정 |
|---|---|---|---|---|
| 1 | 여러 source에서 정확한 업무 제목과 결과를 추출할 수 있다. | High | High | 구현 전후에 fixture와 dogfood로 검증한다. |
| 2 | 3~5분 인터뷰를 사용자가 끝까지 완료한다. | High | High | 가장 짧은 end-to-end prototype으로 먼저 측정한다. |
| 3 | Claude generated skill이 CLI-owned 상태 contract를 지킨다. | High | High | 첫 adapter의 protocol 1.0 contract test를 먼저 만든다. 두 번째 host는 daily gate 이후 검증한다. |
| 4 | 중복 제거가 업무일지 신뢰도를 유지한다. | High | High | 보수적으로 합치고 불확실하면 사용자에게 묻는다. |
| 5 | 주간 회고가 행동 가능한 통찰을 만든다. | High | High | 2주간 실제 확정 일지로 검증한다. |
| 6 | local-first가 충분한 접근성을 제공한다. | Medium | Low | MVP에서 진행하고 웹·모바일은 보류한다. |
| 7 | 공유본과 외부 알림이 초기 채택에 필요하다. | Low | High | MVP에서 제외한다. |

## 6. Validation Experiments

| 가정 | 실험 | 측정값 | 성공 기준 |
|---|---|---|---|
| 업무 제목 정확도 | 실제 과거 5일의 여러 AI 기록으로 초안을 만들고 사용자가 제목을 확인한다. | 그대로 승인된 제목 비율, 누락 업무 수 | 제목 70% 이상 그대로 승인, 누락 업무 하루 1개 이하 |
| 인터뷰 부담 | 제목 확인과 회고 질문을 포함한 최소 prototype을 사용한다. | 소요 시간, 완료율, 건너뛴 질문 수 | median 5분 이하, 완료율 80% 이상 |
| multi-source 안전성 | strict adapter fixture와 실제 best-effort-redacted sample을 수집한다. | adapter별 coverage status, 발견 coverage, 중복률, 지원 common credential leakage | 읽을 수 있다고 명시한 schema의 활동 80% 이상 발견, 중복 10% 이하, 지원 credential fixture 노출 0건, unsupported schema/storage 오분류 0건 |
| Claude adapter 계약 | Claude generated skill에서 같은 daily flow를 반복 실행한다. | protocol 1.0 상태 전이와 최종 문서 구조 | 모든 Phase 1A contract test 통과 |
| 주간 가치 | Daily gate 이후 2주간 confirmed 일지를 만들고 월~일 회고를 진행한다. | 행동 가능한 패턴, 다음 주 행동, 재사용 의향 | Phase 1B에서 회고마다 행동 가능한 통찰 2개 이상 |

## 7. Discovery Decision

Phase 1A는 **Claude/Claude Code generated skill을 첫 인터뷰 화면으로 사용하고 CLI를 versioned local data/state engine으로 사용하는 구조**로 진행한다. 이는 사용자가 이미 AI와 대화하는 화면에서 초안 확인과 감정 인터뷰를 할 수 있어 새로운 UI를 배우거나 별도 API key를 입력하는 부담이 가장 낮기 때문이다. 재사용 skill은 host local `PATH`의 `ai-bricklaying`과 local shell/source file 접근이 필요하다. `npx` 1회 setup만으로 이후 skill 실행 환경이 보장되지는 않는다.

CLI는 `protocol_version: "1.0"`의 상태 변경 contract를 제공하고 모든 쓰기를 소유한다. Generated skill은 catalog source key 5개를 daily prepare에 명시하며, legacy `--sources` summary source 하나의 의미는 유지한다. Catalog key는 history schema 전체 지원을 뜻하지 않는다. Claude Code와 Codex는 명시한 strict event schema를 지원하고, Cursor와 GitHub Copilot은 privacy-conservative experimental adapter이며, 현재 OpenCode `opencode.db`는 `unsupported_storage`로 구분하고 legacy JSON text part만 인식한다. `unsupported_schema`와 `unsupported_storage`를 `no_activity`와 분리한다. `prepare`는 source/status/count만 보여준다. Explicit consent=true 뒤에만 bounded, best-effort-redacted, untrusted evidence를 전달하고 false는 denied를 저장한 채 자유 회상으로 진행한다. Daily 질문과 artifact는 deterministic Korean/English template을 사용하고 다른 legacy/request language는 English로 fallback한다. 같은 local 날짜 flow만 resume하며 confirmed worklog는 Phase 1A local-only다. ChatGPT MCP, weekly, local web, mobile UI, Slack bot, 자동 worklog 전송은 검증 이후 범위로 둔다.
