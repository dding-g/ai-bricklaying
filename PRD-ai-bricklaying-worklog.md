# Product Requirements Document: AI Bricklaying Worklog

**작성자**: Product owner / Codex

**작성일**: 2026-07-16

**상태**: Draft

**관련 문서**: `DISCOVERY-ai-bricklaying-worklog.md`, `ssot/interfaces/ai-bricklaying-cli.md`, `ssot/domains/cli/index.md`

## 1. Summary

AI Bricklaying Worklog의 현재 Phase 1A는 당일 사용한 여러 AI coding agent 기록을 모아 실제 내용이 채워진 업무일지 초안을 만들고, Claude 대화의 3~5분 인터뷰로 사용자가 사실과 경험을 확인하게 한다. CLI가 versioned local data/state plane과 confirmed artifact의 유일한 writer다. `prepare`/`status` machine envelope는 evidence text를 제외한 public control metadata와 source coverage를 반환하며, generated skill은 동의 전에 provider/source/status/count만 사용자에게 보여준다. 사용자가 명시적으로 동의한 경우에만 common credential을 best-effort로 redaction한 bounded evidence를 host agent에 전달한다. ChatGPT MCP adapter와 월요일~일요일 주간 회고는 daily dogfood gate 이후의 계획이며 아직 배포되지 않았다.

제품의 핵심은 “대화 기록을 요약하는 것”이 아니라 “AI가 관찰한 업무와 사용자가 기억하는 경험을 결합해 신뢰할 수 있는 일지를 만드는 것”이다.

## 2. Contacts

| 역할 | 담당 | 책임 |
|---|---|---|
| Product owner | 사용자 | 업무일지 품질과 인터뷰 방향 결정 |
| Engineering | 저장소 기여자 | CLI, source adapter, 상태 contract, generated skill 구현 |
| AI agent integrations | Claude/Claude Code 우선, 다른 host는 후속 | 초안 생성과 자연어 인터뷰 수행 |

## 3. Background

현재 `ai-bricklaying`은 OpenCode, Claude Code, Codex, Cursor, GitHub Copilot 중 선택한 하나의 source에서 당일 파일 신호를 발견한다. 생성되는 Markdown은 간단한 keyword theme와 AI agent가 나중에 채울 회고 템플릿을 포함한다. 따라서 사용자는 실제 업무일지를 얻기 위해 다시 agent를 호출하고, 어떤 업무가 맞는지 직접 재구성해야 한다.

이번 변경은 다음 gap을 해결한다.

- 단일 source가 아니라 당일 사용한 여러 AI 기록을 다뤄야 한다.
- template이 아니라 실제 업무 제목과 내용이 있는 초안을 만들어야 한다.
- AI 추론을 그대로 확정하지 않고 사용자에게 맞는 업무인지 물어야 한다.
- session 기록에 없는 감정, 어려움, 배움, 다음 행동을 인터뷰로 보완해야 한다.
- 매일 확정한 일지가 충분히 쌓인 뒤 월~일 주간 회고로 확장할 수 있어야 한다.
- 인터뷰는 사용자가 이미 일하는 AI 대화에서 쉽게 시작하고 중단 후 재개할 수 있어야 한다.

## 4. Objective

### Product objective

AI와 일한 개발자가 Claude 대화 안에서 하루 5분 이내를 목표로 신뢰할 수 있는 local 업무일지를 확정하도록 한다. Daily dogfood gate를 통과하면 그 기록을 월~일 주간 회고로 확장한다.

### Key results

| 지표 | 현재 | MVP 목표 | 측정 방법 |
|---|---:|---:|---|
| 당일 인터뷰 완료 시간 | 측정 없음 | median 5분 이하 | draft 생성부터 confirm까지 local event timestamp |
| daily interview 완료율 | 실제 interview 없음 | 시작한 flow의 80% 이상 | `interviewing`에 진입한 flow 중 `confirmed` 비율 |
| 업무 제목 사실 정확도 | 측정 없음 | 70% 이상 | 일부 dogfood에서 후보 노출 전 자유 회상한 업무와 제목·근거의 일치율 및 사용자 교정 결과 |
| 누락 업무 | 측정 없음 | 하루 평균 1개 이하 | 사용자가 추가한 missing task 수 |
| weekly review 가치 | 실제 weekly 없음 | Phase 1B에서 회고당 행동 가능한 통찰 2개 이상 | daily gate 이후 weekly dogfood |
| known credential 노출 | 회귀 테스트 존재 | 지원하는 common credential fixture 노출 0건 | best-effort redaction, stdout/artifact contract test |
| daily 준비 지연 | 측정 없음 | dogfood에서 baseline 측정 | 명령 시작부터 daily flow 준비까지의 local timestamp |

### Non-goals

- 근태, 근무 시간, 생산성을 자동 평가하지 않는다.
- 기록만으로 업무 완료 여부나 사용자 감정을 단정하지 않는다.
- 원격 AI 서비스의 cloud history를 자동 동기화하지 않는다.
- session 원문을 업무일지나 공유 payload에 저장하지 않는다.
- 사용자 확인 없이 Gmail, Slack 또는 다른 외부 서비스로 전송하지 않는다.
- MVP에서 mobile app, Slack bot, 상시 local web server를 만들지 않는다.
- Phase 1A에서 완전한 terminal-only AI interview, 두 개 이상의 host agent, ChatGPT MCP, weekly command/artifact를 지원하지 않는다.

## 5. Market Segments and Value Proposition

### Primary job

“여러 AI agent와 일한 하루가 끝났을 때, 대화 내용을 다시 읽지 않고도 내가 실제로 한 일과 느낀 점을 확인해 내일 이어갈 수 있는 기록을 남기고 싶다.”

### Primary segment

- 하루에 하나 이상의 AI coding agent를 사용하는 개발자
- local CLI와 agent skill을 설치할 수 있는 사용자
- 개인 회고를 정본으로 남기고 필요할 때만 공유본을 만들고 싶은 사용자

### Value proposition

- **이전 상태**: 세션별 대화를 다시 읽거나 기억에 의존해 일지를 쓴다.
- **제품 행동**: 여러 기록을 업무 후보로 합치고 제목과 근거를 제안한 뒤 짧게 인터뷰한다.
- **이후 상태**: 사실과 개인 경험이 구분된 local confirmed daily worklog를 갖는다. Daily gate 이후에는 이 기록을 주간 회고로 확장할 수 있다.
- **차별점**: 단순 요약보다 사용자 확인, local-first state/artifact, same-date 재개를 우선하고 weekly는 검증 뒤 추가한다.

## 6. Solution

### 6.1 Experience model

#### Primary entry point: Claude conversation

Phase 1A 사용자는 설치된 Claude skill을 통해 “오늘 업무일지 작성해줘”라고 말한다. skill은 host local `PATH`의 `ai-bricklaying` CLI machine contract를 호출하고 같은 대화에서 한 번에 하나씩 질문한다. 재사용하려면 Claude가 local shell과 catalog adapter input에 접근할 수 있어야 한다. 첫 실행에서는 source별 status/count와 host agent의 처리 위치만 보여주고 명시적으로 예/아니오 결정을 받는다. 동의 전에는 evidence text를 Claude stdout에 반환하지 않는다. 아니오를 선택하면 denied 상태를 저장하고 근거를 전송하지 않은 채 사용자의 자유 회상으로 interview를 이어간다.

Daily interview와 confirmed artifact는 Phase 1A에서 deterministic Korean/English template만 지원한다. 정확한 `ko`, BCP-47 형식의 `ko-*`, 또는 `Korean`/`한국`을 포함한 값은 Korean으로 normalize하고 그 밖의 machine request 또는 saved legacy summary language는 모두 English로 fallback한다. Arbitrary-language 질문 생성을 약속하지 않는다.

다른 source의 기록을 수집하는 것과 다른 host agent에서 interview를 제공하는 것은 별개다. Generated skill은 catalog source key 5개를 `prepare` request에 명시하고 adapter별 coverage를 표시하지만 모든 history schema가 지원된다고 가정하지 않는다. 첫 interview host는 Claude로 고정한다. 기존 `--sources`는 legacy summary source 하나라는 의미를 유지한다. ChatGPT는 Scheduled Tasks만으로 local file에 접근한다고 가정하지 않고, 같은 service/protocol을 호출하는 MCP adapter를 후속 구현한다.

#### CLI entry points

| 명령 | 행동 |
|---|---|
| `ai-bricklaying machine daily prepare` | `protocol_version: "1.0"`을 포함한 stdin JSON으로 당일 다중 source를 수집하고 private draft를 준비한다. stdout envelope는 evidence를 제외한 public flow control metadata와 source coverage를 반환하며 skill은 동의 전에 provider/source/status/count만 사용자에게 표시한다. |
| `ai-bricklaying machine daily status` | 날짜별 미완료 또는 confirmed daily flow를 조회한다. |
| `ai-bricklaying machine daily disclose` | 명시적 boolean consent와 expected revision을 받는다. true는 bounded best-effort-redacted untrusted evidence를 반환하고, false는 denied를 저장한 뒤 evidence 없이 성공한다. |
| `ai-bricklaying machine daily checkpoint` | 제목 교정과 답변 snapshot을 revision/idempotency guard로 저장한다. |
| `ai-bricklaying machine daily finalize` | 최종 preview 확인 후 structured JSON과 Markdown을 confirmed로 저장한다. |
| `ai-bricklaying` / 기존 flags | setup과 legacy summary/skill generation을 유지한다. 새 설치의 기본 target은 Claude Code, 기본 skill 이름은 `ai-bricklaying-worklog`다. |

generated skill은 현재 local date로 먼저 status를 호출하고 `flow_not_found`일 때 prepare를 호출해 같은 날짜 resume routing을 담당한다. 과거 날짜 flow를 오늘 interview로 자동 연결하지 않는다. 사람용 `daily`, `resume`, `weekly`, `status` alias는 dogfood 이후 후속 범위다. 기존 non-interactive flag invocation과 단일 `--sources` 의미는 유지한다.

#### Future entry points

- 사용자가 명시적으로 켜는 local OS reminder
- click-to-open local web interview
- mobile-friendly companion과 Slack bot

이 경로는 MVP 사용성 검증 이후 진행한다.

### 6.2 Daily flow

1. Request의 IANA timezone과 date를 사용하고, 생략하면 process local timezone의 today를 사용한다. Claude Code, Codex, GitHub Copilot은 event timestamp를 사용한다. Cursor도 event timestamp를 우선하며 timestamp가 없는 허용 record만 file mtime으로 fallback하고 `timestamp_basis`에 표시한다.
2. source별 결과를 `complete`, `no_activity`, `not_found`, `unreadable`, `parse_error`, `truncated`, `unsupported_schema`, `unsupported_storage`로 구분한다.
3. Discovery record path field는 제외하고 common credential을 best-effort redaction해 source, 시간, 짧은 근거를 정규화한다. 자유 텍스트가 모든 secret/path-like 문자열에서 완전히 안전하다고 가정하지 않는다.
4. Evidence content가 아니라 source/status/count와 remote processing 가능성만 보여준 뒤 명시적으로 예/아니오 결정을 받는다.
5. 동의하면 bounded untrusted evidence로, 거부하면 사용자 자유 회상으로 관련 활동을 업무 단위로 묶고 업무 제목, 수행 내용, 결과, 검증, 이슈 후보를 작성한다.
6. 제목별로 stable ID에서 파생한 안정적인 번호, 한 줄 `evidence_summary`, 한 줄 `uncertainty`를 저장하고 보여준다. User recall 후보의 근거는 사용자 회상임을 밝히며 검증되지 않은 numeric confidence는 표시하지 않는다.
7. 사용자는 제목을 승인하거나 변경, 합치기, 나누기, 제외할 수 있다. Accepted 업무가 없으면 “기록할 업무 없음”을 명시적으로 확인해 `no_work_confirmed`로 checkpoint한다.
8. 기록에 없는 업무를 추가할 기회를 제공한다. 품질 실험 일부에서는 후보를 보여주기 전에 먼저 자유 회상을 받는다.
9. 하루 전체에 대해 가장 의미 있었던 결과, 어려움, 느낀 점, 배운 점, 다음 행동을 묻는다.
10. 답변에서 막힘, 강한 감정, 불확실성이 발견될 때만 한 단계 더 묻는다.
11. 최종 preview를 보여주고 confirm하면 일지를 `confirmed`로 저장한다.

#### Daily interview rules

- 한 번에 질문 하나만 보여준다.
- 시작할 때 예상 시간과 진행률을 알려준다.
- `모두 맞음`, `건너뛰기`, `이전`, `중단하고 저장`을 제공한다.
- 업무별 감정을 반복해서 묻지 않고 하루 전체 질문을 우선한다.
- AI가 관찰한 내용, 사용자 답변, AI가 추론한 개선 제안을 구분한다.
- 사용자 답변을 더 긍정적이거나 전문적인 표현으로 왜곡하지 않는다.
- 기본 concise flow에서는 업무 후보를 최대 6개까지 먼저 보여준다. 그보다 많으면 낮은 근거의 후보를 보류하고 사용자가 펼쳐볼 수 있게 한다.
- 편집 명령은 안정적인 업무 번호를 사용한다. 합치기나 나누기 뒤에는 변경된 목록을 다시 preview한다. `이전`은 현재 대화의 in-memory 직전 snapshot만 되돌리며, persisted state는 최신 checkpoint만 보존하므로 새 session에서 undo history를 복원하지 않는다.

### 6.3 Daily artifact

Draft와 interviewing은 `<config-dir>/state/v1/daily/YYYY-MM-DD.json`에만 저장한다. `finalize` 성공 시 SSOT에 확정된 `worklogs/daily/YYYY-MM-DD-ai-bricklaying-worklog.{md,json}`을 생성한다. Confirmed Markdown은 다음 구조를 가진다.

1. 날짜와 상태
2. Source별 수집 범위
3. 확인된 업무별 제목, 한 줄 근거, 불확실성, 수행 내용, 결과, 검증, 남은 이슈
4. 가장 의미 있었던 결과, 어려움, 느낌, 배움
5. 다음 행동
6. 수집 불확실성

Private state에는 source별 coverage, `draft/interviewing/confirmed` 상태, consent, work item, reflection, interview progress, flow ID, revision, timezone, normalized `Korean`/`English` language, schema version과 bounded evidence를 저장한다. Confirmed JSON은 coverage, confirmed work item, reflection, flow ID, revision, timezone, normalized language, confirmed timestamp를 저장하고 evidence excerpt를 포함하지 않는다. Discovery record path field와 full transcript는 저장하지 않는다. Bounded evidence의 common credential redaction은 best-effort이며 자유 텍스트의 모든 path-like 문자열 제거를 보장하지 않는다.

### 6.4 Planned weekly flow — Phase 1B, not shipped

아래는 daily dogfood gate를 통과한 뒤 검증할 계획이다. 현재 CLI에는 weekly command, state, artifact가 없다.

1. Phase 1B review에서 명시적으로 선택한 IANA timezone으로 월요일 00:00부터 일요일 23:59까지의 confirmed daily log를 모은다.
2. “누락”은 AI 활동이 발견됐지만 confirmed daily log가 없는 날짜로 정의한다. AI 활동이 없는 주말과 휴일은 자동으로 누락 처리하지 않는다.
3. 미확정 날짜마다 `지금 daily 확정`, `이번 회고에서 제외`, `weekly 중단`을 제공한다.
4. confirmed active day가 3일 미만이면 추세를 단정하지 않고 partial recap만 만든다. 사용자가 원하면 누락 날짜를 짧게 보완할 수 있다.
5. 반복된 업무, 중요한 결과, 계속된 장애, 감정과 에너지의 변화, AI 사용 패턴을 제안한다.
6. 주간 통찰마다 안정적인 번호를 붙이고 승인, 수정, 제외할 수 있게 한다.
7. 가장 잘한 일, 아쉬운 일, 다음 주에 바꿀 일, 다음 주 핵심 목표를 짧게 인터뷰한다.
8. coverage가 표시된 최종 preview를 확인한 뒤 주간 회고와 다음 주 follow-up prompt를 저장한다.

주간 회고는 confirmed daily log를 우선 사용한다. draft를 자동으로 사실처럼 합치지 않는다.

주간 artifact는 주간 범위와 상태, coverage, 핵심 성과, 반복 문제, 감정·에너지 흐름, AI 활용 패턴, 사용자가 직접 말한 회고, 다음 주 목표와 행동, follow-up prompt, 불확실성 순서로 구성한다. metadata에는 포함·제외한 날짜, source coverage, 상태, flow ID, revision, timezone, schema version을 저장한다.

### 6.5 State and resume

- 상태는 `draft`, `interviewing`, `confirmed`를 구분한다.
- CLI가 state와 artifact의 유일한 writer다. generated skill은 파일을 직접 수정하지 않고 `protocol_version: "1.0"`의 `prepare`, `status`, `disclose`, `checkpoint`, `finalize` machine command만 호출한다.
- 각 mutation은 flow ID, expected revision, idempotency key를 포함한다. stale revision은 쓰기를 거부하고 최신 상태를 다시 읽도록 한다.
- 각 답변 뒤 상태를 owner-only permission으로 원자적으로 저장한다.
- 중단 시 완료한 답변을 다시 묻지 않는다.
- 같은 local 날짜에 skill을 다시 실행하면 해당 날짜의 미완료 interview만 이어간다. 과거 날짜 flow를 오늘 interview로 자동 연결하지 않는다.
- Confirmed flow는 read-only다. 같은 날짜 재실행으로 덮어쓰거나 `interviewing`으로 되돌리지 않고 저장 경로를 안내한다.
- Versioned daily state와 machine JSON protocol을 사용해 adapter가 동일한 contract를 따르게 한다.
- Weekly routing과 cross-day 미완료 nudge는 현재 범위가 아니다.

### 6.6 Accessibility

- Claude skill의 질문은 모든 행동을 keyboard와 평문 입력만으로 완료할 수 있는 선형 대화로 제공한다.
- CLI의 setup, status, daily flow 준비, 오류 안내는 raw-mode TUI에만 의존하지 않고 `NO_COLOR`를 지원한다.
- 질문 표현은 별도 장식이나 progress bar보다 “3개 중 1번째 질문” 같은 서술형 진행 정보를 사용한다.
- 입력 timeout을 두지 않는다.
- 질문을 건너뛰거나 되돌리고 나중에 resume할 수 있다.
- Deterministic Korean/English template으로 질문과 artifact를 일관되게 만들고 unsupported language는 English로 fallback한다.
- host agent UI 자체의 접근성은 제품이 통제할 수 없으므로 Claude의 접근성 지원 범위와 남은 한계를 문서화한다.

### 6.7 Privacy and delivery

- Legacy lightweight summary의 file 저장은 항상 활성화한다. Daily flow는 confirmation 전 private state를 checkpoint하고 confirmed worklog는 explicit finalize 뒤에만 생성한다.
- Host scheduler는 skill 호출을 깨울 수만 있으며 explicit current user decision 없이 evidence consent를 재사용하거나 `disclose`, `finalize`, legacy delivery를 자동 실행하지 않는다.
- “local-first”는 daily state와 artifact의 저장 위치를 뜻하며 모든 처리가 로컬이라는 뜻이 아니다. host agent가 remote이면 승인된 redacted 근거가 해당 provider로 전달된다.
- full session transcript는 로컬 discovery boundary 밖으로 보내지 않는다. Evidence excerpt의 common credential redaction은 best-effort이고 새로운 secret 형식의 완전 탐지를 보장하지 않는다.
- `prepare`/`status` public flow에는 resume에 필요한 state/revision/consent와 이전에 저장한 work item/reflection/interview control data가 포함될 수 있지만 local session evidence excerpt는 제외한다. Generated skill은 새 evidence 동의를 묻기 전에 source-derived 정보로 provider/source/status/count만 사용자에게 보여준다. 사용자가 consent=true를 선택한 뒤에만 업무 후보 작성에 필요한 bounded excerpt를 전달한다. consent=false는 denied로 저장하고 evidence 없이 자유 회상으로 진행한다.
- 모든 session-derived text는 신뢰하지 않는 data로 인용하고 instruction으로 실행하지 않는다. 내부 command, URL, path를 실행하지 않으며 follow-up prompt에 원문의 지시를 복사하지 않는다.
- 알려진 webhook, token, password, API key pattern은 stdout과 evidence에서 best-effort redaction한다. Discovery record path field를 public envelope에 추가하지 않지만 자유 텍스트가 완전히 path-free라고 약속하지 않는다.
- Private confirmed daily reflection이 Phase 1A 정본이다. Weekly reflection은 아직 없다.
- Private-to-share report는 후속 범위다. Phase 1A는 confirmed worklog를 외부 공유본으로 생성하거나 전달하지 않는다.
- Gmail과 Slack은 legacy lightweight summary의 명시적 opt-in handoff다. Phase 1A confirmed worklog delivery에는 적용하지 않는다.
- Private daily state와 confirmed worklog directory/file은 owner-only permission을 사용하고 symlink target 쓰기를 거부한다. Legacy summary artifact permission은 기존 계약을 유지한다.
- Slack webhook 같은 delivery credential은 private `config.json`에 저장될 수 있다. stdout에는 raw value 대신 configured 여부만 표시한다.

### 6.8 Source reliability and performance

- source별 discovery 결과는 `complete`, `no_activity`, `not_found`, `unreadable`, `parse_error`, `truncated`, `unsupported_schema`, `unsupported_storage` 중 하나다.
- `no_activity`만 정상적인 빈 결과다. 그 외 실패나 부분 수집은 초안과 preview에 원인과 영향을 표시한다.
- 일부 source가 실패하거나 부분 수집되면 해당 status/reason을 불확실성으로 표시하고, evidence가 부족할 때는 사용자의 자유 회상으로 이어간다.
- 전체 source가 실패해도 실제 업무가 없다고 단정하지 않고 draft를 유지한 채 source 설정 확인과 자유 회상 경로를 안내한다.
- Strict daily adapter의 5,000은 source별 adapter-matched candidate를 최신순으로 보관하는 top-k retention 한도일 뿐 directory traversal entry 수, wall-clock 시간, 전체 resource 사용량의 bound가 아니다. Configured root traversal에는 별도 entry/time bound가 없다. Artifact당 raw read 16 MiB, machine extracted text 4,000자, evidence excerpt 1,200자와 optional truncation ellipsis의 한도를 사용하고 결과 한도 도달 시 `truncated`로 기록한다. Legacy extractor는 별도로 candidate 5,000개와 extracted text 20,000자 한도를 유지한다. Phase 1A는 scan timeout이나 cancellation persistence를 계약으로 약속하지 않는다.
- Daily flow 준비 시간은 dogfood에서 baseline을 먼저 측정한 뒤 별도 성능 gate를 정한다.

| Catalog key | Strict daily support contract |
|---|---|
| Claude Code | `~/.claude/projects`의 main-project JSONL에서 event-time visible user/assistant text만 허용한다. System/tool/meta/sidechain/memory/subagent content는 제외한다. |
| Codex | `CODEX_HOME`의 `sessions`와 `archived_sessions`를 읽고 relevant-day lifecycle evidence를 `item_completed` > direct message > response fallback 순서로 선택한다. Context/tool/reasoning content는 제외하고 event time을 사용한다. |
| Cursor | `~/.cursor/projects/**/agent-transcripts`의 well-formed JSONL에서 `role=user` `<user_query>`만 허용하는 privacy-conservative experimental adapter다. Assistant/reasoning/skill content는 제외하고 timestamp가 없을 때만 file mtime fallback을 basis와 함께 기록한다. |
| GitHub Copilot | VS Code `workspaceStorage/*/chatSessions` schema v3 mutation을 replay하는 experimental adapter다. `GitHub.copilot-chat` extension ID가 필요하며 hidden/system/tool/thinking/other-agent content를 제외한다. Unknown 또는 oversized dependent state는 추측하지 않고 fail closed한다. |
| OpenCode | 현재 `opencode.db`는 `unsupported_storage`로 보고한다. Recognized legacy JSON text part만 허용하고 logs/auth는 strict-scan하지 않는다. |

### 6.9 Backward compatibility

- 기존 단일 `--sources <source>`와 `--sessions <source>` invocation은 legacy summary source 하나로 계속 유효하다. Machine daily `prepare`의 `sources` JSON 배열과 섞거나 comma-separated flag로 확장하지 않는다.
- 기존 config의 source string은 legacy summary default로 그대로 유지한다. Generated daily skill은 별도의 explicit five-catalog-key profile을 사용하고 각 adapter의 실제 coverage를 표시한다.
- 기존 daily summary와 신규 worklog는 서로 다른 filename과 schema version을 사용해 덮어쓰지 않는다.
- 기존 generated skill은 version mismatch를 감지해 재생성 안내를 제공한다. 신규 state를 구버전 skill이 직접 수정하지 못하게 한다.
- bare `ai-bricklaying`의 setup 동작은 config가 없는 경우 그대로 유지한다. non-interactive flags의 기존 validation과 exit code도 migration 기간에 보존한다.
- migration, rollback, 최소 지원 schema version은 구현 전에 `ssot/interfaces/ai-bricklaying-cli.md`와 `ssot/domains/cli/index.md`에 먼저 반영한다.

### 6.10 Requirements and acceptance criteria

#### P0 — Must have

| # | User story | Acceptance criteria |
|---|---|---|
| P0-1 | 여러 AI를 사용한 사용자로서 당일 활동을 한 번에 모으고 싶다. | Generated Claude skill이 catalog source key 5개를 `prepare` JSON 배열에 명시하고 source별 status/count와 unsupported schema/storage를 구분한다. 모든 history 형식 지원을 약속하지 않으며 legacy `--sources`는 summary source 하나로 유지한다. |
| P0-2 | 사용자로서 실제 업무 내용이 있는 초안을 받고 싶다. | Claude skill flow가 하나 이상의 업무 후보 또는 명확한 coverage 없음 안내를 작성하며 빈 회고 template만 출력하지 않는다. |
| P0-3 | 사용자로서 AI가 찾은 업무가 맞는지 빠르게 확인하고 싶다. | 모든 후보에 안정적인 번호와 한 줄 근거가 있고 승인, 변경, 합치기, 나누기, 제외, 누락 추가가 가능하다. 현재 대화에서는 in-memory 직전 snapshot을 되돌릴 수 있지만 cross-session undo history는 제공하지 않는다. Accepted 업무가 없으면 사용자가 “기록할 업무 없음”을 명시적으로 확인한다. |
| P0-4 | 사용자로서 내 경험과 감정을 일지에 반영하고 싶다. | 하루 전체에 대한 의미, 어려움, 느낌, 배움, 다음 행동을 묻고 답변을 사용자 발언으로 구분한다. |
| P0-5 | 바쁜 사용자로서 인터뷰를 중단하고 이어가고 싶다. | CLI가 모든 state mutation을 소유하고 답변별 atomic save, flow ID, revision conflict, idempotent checkpoint/finalize와 `status` contract test를 통과한다. |
| P0-6 | 사용자로서 오늘 인터뷰를 안전하게 재개하고 싶다. | Generated skill이 같은 local 날짜의 flow만 resume한다. Confirmed flow는 read-only이며 cross-day nudge나 overwrite를 하지 않는다. |
| P0-7 | 사용자로서 익숙한 AI 대화에서 인터뷰하고 싶다. | Claude generated skill 하나가 versioned daily machine protocol과 CLI-owned state로 daily flow를 완료한다. Korean/English 질문·artifact는 deterministic하고 unsupported language는 English로 fallback한다. ChatGPT MCP와 weekly는 gate 이후다. |
| P0-8 | 개인정보를 다루는 사용자로서 기록이 안전하길 원한다. | 동의 전 source/status/count만 보여주고 explicit true/false를 받는다. False는 denied로 저장한다. Common credential best-effort redaction, untrusted evidence 격리, owner-only permission, symlink 거부 테스트를 통과하며 blanket secret/path-free guarantee는 하지 않는다. |
| P0-9 | keyboard 또는 screen reader를 사용하는 사용자로서 인터뷰를 완료하고 싶다. | Claude skill 질문과 CLI setup/status/error가 평문, keyboard-only, 서술형 진행률, skip/back/resume contract를 따른다. 색상만으로 상태를 표현하지 않는다. |
| P0-10 | 사용자로서 private 업무일지를 원치 않게 전송하고 싶지 않다. | Confirmed worklog는 Phase 1A local-only다. Gmail/Slack mode와 payload는 legacy summary handoff임을 skill과 문서가 명시한다. |

#### P1 — Should have

| # | User story | Acceptance criteria |
|---|---|---|
| P1-1 | 사용자로서 질문 수를 조절하고 싶다. | concise와 deep interview depth를 설정할 수 있다. |
| P1-2 | 사용자로서 정해진 시간에 알림을 받고 싶다. | local OS notification을 명시적으로 opt-in하고 raw 내용은 notification에 넣지 않는다. |
| P1-3 | 사용자로서 긴 답변을 편하게 쓰고 싶다. | `$EDITOR` round-trip 후 입력을 preview하고 저장할 수 있다. |
| P1-4 | 사용자로서 팀 공유용 보고를 만들고 싶다. | private reflection에서 감정과 민감정보를 제거한 공유본을 preview 후 생성한다. |
| P1-5 | terminal만 사용하는 사용자로서도 AI 초안을 받고 싶다. | configured provider adapter가 있을 때 `ai-bricklaying daily`만으로 초안과 인터뷰를 완료한다. provider가 없으면 안전한 agent entry 안내를 제공한다. |
| P1-6 | ChatGPT 사용자로서 같은 흐름을 사용하고 싶다. | ChatGPT MCP adapter가 protocol 1.0의 daily machine contract test와 local access/privacy 검토를 통과한다. |
| P1-7 | 사용자로서 민감한 기록의 수명을 관리하고 싶다. | daily draft/state/artifact를 기간별로 조회·export·삭제하는 정책을 제공한다. Config secret lifecycle은 별도 privacy contract에서 결정한다. |
| P1-8 | 사용자로서 한 주를 회고하고 싶다. | Daily dogfood gate 이후 월~일 confirmed 일지를 집계하고 activity가 있으나 미확정인 날짜를 표시한다. 3일 미만 coverage에서는 추세를 단정하지 않는 partial artifact를 만든다. |

#### P2 — Future

| # | User story | Acceptance criteria |
|---|---|---|
| P2-1 | 사용자가 local browser에서 interview를 하고 싶다. | 명시적 실행 시에만 localhost UI가 열리고 인증·CSRF·종료 contract를 만족한다. |
| P2-2 | 사용자가 mobile 또는 Slack에서 답하고 싶다. | 별도 privacy review와 명시적 연결 후에만 최소 질문과 답변을 전송한다. |

## 7. Assumptions and Open Questions

### Assumptions

- Claude skill 대화가 첫 대상 사용자에게 가장 접근 비용이 낮은 인터뷰 화면이다.
- redacted session signal만으로도 업무 제목 70% 이상을 그대로 승인받을 수 있다.
- 3~5분 인터뷰는 daily habit으로 유지 가능한 수준이다.
- 월~일 confirmed 일지가 주간 패턴을 찾기에 충분하다는 가정은 Phase 1B에서 검증한다.
- Versioned machine protocol과 CLI-owned state contract로 host agent의 잘못된 state write를 막을 수 있다.

### Open questions

| 질문 | 결정 시점 |
|---|---|
| weekly artifact의 filename과 metadata migration 방식은 무엇인가? | Phase 1B SSOT 변경 시 |
| confirmed 일지를 수정할 때 revision history를 얼마나 보존할 것인가? | state schema 설계 시 |
| next-run 안내만으로 habit 형성이 부족해 P1 local notification이 필요한가? | 2주 dogfood 후 |
| daily draft/state/artifact의 기본 retention 기간은 무엇인가? | privacy contract 작성 시 |

## 8. Release

### Phase 0 — Contract and falsifiable prototype

- `DISCOVERY-ai-bricklaying-worklog.md`의 첫 세 실험을 수행할 Claude skill prototype을 만든다.
- daily machine request/envelope, 상태 전이, Korean/English locale normalization, consent와 best-effort redaction boundary를 SSOT에 정의한다.
- legacy config와 artifact migration, source 상태, file-mtime timestamp basis, permission, prompt-injection 방어 contract를 정의한다.
- 최소 5개의 서로 다른 daily flow를 실행한다. 두 종류 이상의 source 구성을 포함하고, 후보가 6개를 넘는 날을 최소 1회 포함한다.
- gate는 interview median 5분 이하, 시작 flow 완료율 80% 이상, 독립 회상과 교정 기준 제목 사실 정확도 70% 이상, 누락 업무 하루 평균 1개 이하, 지원 common credential fixture와 prompt-injection escape 0건이다. 성능은 baseline을 측정한 뒤 별도 gate를 정한다.
- gate를 통과하지 못하면 weekly와 두 번째 host agent 구현에 들어가지 않고 source 품질, 질문 수, 초안 구조를 먼저 수정한다.

### Phase 1A — Daily MVP

- multi-source daily collection
- evidence-backed work item draft
- Claude skill daily interview
- draft/interviewing/confirmed 상태와 resume
- same-date resume와 confirmed read-only contract
- explicit true/false consent, best-effort common credential redaction, untrusted-data handling, owner-only storage와 accessibility contract
- local-only confirmed worklog; legacy Gmail/Slack summary handoff와 분리
- Node/Go acceptance tests와 README/SSOT 동기화

### Phase 1A gate

- 실제 confirmed daily log 5개 이상을 만든다.
- 그중 같은 월~일 주간에 속하는 confirmed active day가 3개 이상이어야 한다.
- Phase 0의 시간, 완료율, 사실 정확도, 누락, privacy gate를 다시 통과해야 한다.

### Phase 1B — Weekly MVP increment

- 월~일 confirmed daily 집계
- missing active day 보완 또는 제외 flow
- coverage-aware weekly insight와 partial recap
- 주간 통찰 승인·수정·제외 interview
- weekly artifact와 다음 주 follow-up prompt

### Phase 2 — Adoption and sharing

- configurable interview depth
- next-run reminder
- private-to-share report
- ChatGPT MCP 또는 second agent end-to-end contract
- terminal provider adapter
- local notification과 retention/export/delete

### Phase 3 — Optional surfaces

- local web interview
- OS notification
- mobile 또는 Slack interaction

각 phase는 product owner가 이전 gate의 local measurement를 확인한 뒤 진행한다. 정확한 일정은 기술 spike와 dogfood 결과로 정한다.
