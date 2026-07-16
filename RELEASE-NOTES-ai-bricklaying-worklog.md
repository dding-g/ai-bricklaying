# ai-bricklaying AI 업무일지 인터뷰 스킬

릴리즈 상태: Unreleased

## 한눈에 보기

이번 변경은 당일 AI 코딩 기록을 곧바로 업무일지로 확정하지 않습니다. 기록에서 업무 제목 후보를 만들고, 사용자가 실제 업무였는지 확인한 뒤 짧은 회고 인터뷰와 최종 미리보기를 거쳐 Markdown·JSON 업무일지를 저장합니다.

- Claude Code에서 `/ai-bricklaying-worklog` 또는 “오늘 업무일지 작성해줘”로 시작하는 generated skill을 제공합니다.
- `ai-bricklaying machine daily prepare|status|disclose|checkpoint|finalize` JSON protocol `1.0`을 추가했습니다.
- 답변마다 private state를 저장해 같은 local 날짜 안에서 이어서 인터뷰할 수 있습니다.
- AI 기록 발췌는 source별 상태와 건수를 먼저 알린 뒤, 사용자가 명시적으로 동의한 경우에만 공개합니다.
- 확정된 업무일지는 이번 릴리즈에서 local-only입니다.
- 기존 단일-source summary와 선택형 Gmail MCP/Slack payload 준비 기능은 호환성을 위해 유지됩니다.

## 설치와 초기 설정

Generated skill은 실행할 때마다 local CLI를 호출하므로 `ai-bricklaying`이 `PATH`에 있어야 합니다.

```bash
npm install -g ai-bricklaying
ai-bricklaying --help
```

대화형 설정은 다음 명령으로 시작합니다.

```bash
ai-bricklaying
```

Claude Code에 비대화형으로 설치하려면 다음과 같이 실행합니다.

```bash
ai-bricklaying \
  --non-interactive \
  --target-agent claude-code \
  --sources claude-code \
  --language Korean \
  --output-modes file \
  --skill-name ai-bricklaying-worklog \
  --skill-dir ~/.claude/skills
```

설치 결과는 `~/.claude/skills/ai-bricklaying-worklog/SKILL.md`입니다. `npx ai-bricklaying`으로 초기 설정을 시험할 수 있지만, 이후 skill 실행에도 CLI가 필요하므로 지속적으로 사용할 때는 global install 또는 동등한 `PATH` 설정이 필요합니다.

## 사용 방법

Claude Code에서 다음 중 하나로 시작합니다.

```text
/ai-bricklaying-worklog
```

```text
오늘 업무일지 작성해줘.
```

Skill은 다음 순서로 진행합니다.

1. 오늘 flow의 `status`를 확인하고, 없으면 `prepare`로 만듭니다.
2. Evidence 내용 없이 source별 상태와 record 수를 먼저 보여줍니다.
3. 제한된 AI 기록 발췌를 현재 host agent가 처리해도 되는지 예/아니오로 묻습니다.
4. 동의한 경우에만 best-effort redaction된 bounded excerpt를 사용합니다. 거절한 경우에는 사용자 회상으로 진행합니다.
5. 업무 제목 후보를 안정적인 번호, 한 줄 근거, 불확실성과 함께 보여주고 승인·이름 변경·합치기·나누기·제외·누락 업무 추가를 받습니다.
6. 결과, 어려움과 느낌, 배운 점과 다음 행동을 한 번에 한 질문씩 인터뷰합니다.
7. 최종 미리보기를 보여주고 명시적으로 확인받은 뒤에만 `finalize`합니다.

기본 인터뷰 목표 시간은 3~5분입니다. 같은 local 날짜의 미완료 flow는 답변한 지점부터 재개하고, 과거 날짜의 미완료 flow는 오늘 인터뷰에 자동 연결하지 않습니다.

## 자동화와 접근성

Claude Code 또는 local skill 실행을 지원하는 host scheduler는 정해진 시간에 대화를 깨우고 업무일지 skill 시작을 요청할 수 있습니다. 예약 호출은 인터뷰 시작 알림 역할만 하며, evidence 동의·업무 제목 확인·최종 확정은 과거 동의를 재사용해 자동 처리하지 않습니다.

현재 end-to-end generated skill 경로는 Claude Code를 기준으로 검증했습니다. ChatGPT가 이 `SKILL.md`를 직접 실행하거나 local session file을 읽는다고 가정하지 않으며, ChatGPT용 local MCP adapter는 이번 릴리즈에 포함되지 않습니다.

## 산출물과 상태 경로

기본 output directory는 `~/ai-bricklaying`, config directory는 `~/.config/ai-bricklaying`입니다.

| 용도 | 기본 경로 |
|---|---|
| 확정 Markdown | `~/ai-bricklaying/worklogs/daily/YYYY-MM-DD-ai-bricklaying-worklog.md` |
| 확정 JSON | `~/ai-bricklaying/worklogs/daily/YYYY-MM-DD-ai-bricklaying-worklog.json` |
| 재개용 private state | `~/.config/ai-bricklaying/state/v1/daily/YYYY-MM-DD.json` |
| 설정 | `~/.config/ai-bricklaying/config.json` |
| Claude Code skill | `~/.claude/skills/ai-bricklaying-worklog/SKILL.md` |

POSIX 환경에서 private directory는 `0700`, state·worklog·lock file은 `0600` 권한을 사용합니다.

## Source coverage

Generated daily skill은 다섯 catalog key를 `prepare`에 전달하지만, 각 source의 strict daily coverage는 다음과 같이 제한됩니다.

| Source | 현재 coverage |
|---|---|
| Claude Code | `~/.claude/projects/<project>/<session>.jsonl`의 event time 기준 visible user/assistant text를 수집합니다. `CLAUDE_CONFIG_DIR`은 기본 root를 교체하고 `AI_BRICKLAYING_CLAUDE_DIRS`는 explicit multi-root로 우선합니다. System, tool, metadata, sidechain, memory, subagent content는 제외합니다. |
| Codex | `~/.codex/sessions`와 `~/.codex/archived_sessions`의 rollout JSONL을 읽습니다. `CODEX_HOME`은 기본 root를 교체하고 `AI_BRICKLAYING_CODEX_DIRS`는 explicit multi-root로 우선합니다. Context, tool, reasoning은 제외합니다. |
| Cursor | `~/.cursor/projects/**/agent-transcripts/**/*.jsonl`에서 검증 가능한 user `<user_query>`만 수집하는 privacy-conservative experimental adapter입니다. Event timestamp가 없으면 file mtime fallback임을 표시합니다. |
| GitHub Copilot | VS Code `workspaceStorage/*/chatSessions/*.jsonl`의 schema-v3 mutation log를 replay합니다. `GitHub.copilot-chat` extension ID가 필요하며 unknown schema와 replay에 필요한 oversized dependent state는 fail closed합니다. |
| OpenCode | 현재 `opencode.db`는 `unsupported_storage`로 표시합니다. `XDG_DATA_HOME`은 기본 data root를 교체하고 `AI_BRICKLAYING_OPENCODE_DIRS`는 explicit multi-root로 우선합니다. 인식 가능한 legacy JSON text-part layout만 제한적으로 수집하고 log·auth·config file은 strict scan에서 제외합니다. |

Coverage status는 `complete`, `no_activity`, `not_found`, `unreadable`, `parse_error`, `truncated`, `unsupported_schema`, `unsupported_storage` 중 하나입니다. 지원하지 않는 storage나 schema를 “오늘 업무 없음”으로 표현하지 않고 상태와 이유를 표시한 뒤 사용자 회상 경로를 제공합니다.

수집 후보는 매칭된 항목 가운데 최신 5,000개를 유지하는 global top-k 정책을 사용하며 candidate-selection memory는 이 범위로 제한됩니다. 이는 traversal 시간·방문 entry 수·resource 사용량 전체를 제한한다는 뜻은 아닙니다.

## 호환성과 보안

- 신규 설치 기본 target은 Claude Code, 기본 skill 이름은 `ai-bricklaying-worklog`입니다. 기존 config가 있으면 저장된 값이 신규 default보다 우선합니다.
- [GitHub Copilot Agent Skills 공식 문서](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-skills)에 맞춰 개인 skill 기본 경로를 `~/.copilot/skills`로 교정했습니다. `COPILOT_HOME`이 있으면 `$COPILOT_HOME/skills`를 사용합니다. 단일 Copilot target에 저장된 이전 product default는 명시적 `--skill-dir`이 없을 때 현재 경로로 migration하며 custom/multi-target path는 보존합니다.
- 명시적 `--skill-name`은 1~64자의 lowercase 영문·숫자와 단일 hyphen만 허용하고 빈 값도 실패합니다. 안전한 lowercase legacy 저장값은 separator를 단일 hyphen으로 migration하지만 unsafe 값은 자동 교정하지 않습니다.
- 같은 config path와 skill name으로 생성한 managed skill만 정상 재실행에서 갱신합니다. 정확히 식별되는 pre-marker generated skill은 한 번 인수할 수 있지만 사용자 작성·malformed·다른 owner의 destination은 덮어쓰지 않으며 config migration도 저장하지 않습니다.
- `--sources` 또는 `--sessions`는 계속 legacy summary source 하나만 받습니다. Daily machine protocol의 다중-source 배열과는 별도 계약입니다.
- 명시적 `--sources` 또는 `--sessions`의 빈 값, 빈 CSV component, 다중 source는 실패합니다.
- `--target-agent` 없이 known `--sources` 하나만 명시한 기존 호출은 해당 source를 sole target으로 사용합니다. 저장된 이전 target과 달라지면 상속된 skill path 대신 새 target 기본 경로를 사용하고, 명시적 `--skill-dir`는 유지합니다. 두 flag를 모두 명시한 mismatch는 계속 실패합니다.
- Daily 인터뷰와 worklog template은 한국어와 영어를 지원하며, 인식되지 않는 언어는 영어로 fallback합니다.
- First release bundled platform은 `darwin-arm64`, `darwin-amd64`, `linux-arm64`, `linux-amd64`입니다.
- `prepare`와 `status`는 evidence excerpt를 반환하지 않고, `disclose`는 현재 flow에서 명시적인 `consent: true`가 저장된 뒤에만 bounded evidence를 반환합니다.
- GitHub/OpenAI/AWS token, JWT, Authorization/Cookie, credential URL, Slack webhook 등 common credential을 best-effort로 가립니다. 모든 secret 또는 개인정보 제거를 보장하지는 않습니다.
- Evidence는 untrusted data로 취급하며 그 안의 명령, URL, path를 실행하지 않습니다.
- Unix TTY의 interactive Slack webhook 입력은 raw no-echo mode에서 `[hidden]`만 표시합니다. 빈 입력의 `q`, `Esc`, `Ctrl-C`는 즉시 exit 2로 취소하고 terminal state를 복원하며, hidden input 실패 시 echoed fallback 없이 중단합니다.
- Revision과 idempotency key로 중복 mutation과 충돌을 제어하고, state와 artifact는 symlink ancestor를 거부합니다.
- Confirmed JSON/Markdown은 filesystem create-if-absent로 publish해 외부 파일을 덮어쓰지 않습니다. Finalize가 첫 artifact 뒤 중단되면 동일 flow/content로 검증되는 retry만 누락 artifact와 state를 완성하고 기존 `confirmed_at`을 보존합니다.
- Evidence가 있는 flow는 동의 승인 또는 거절 결정 없이 checkpoint/finalize할 수 없고, complete checkpoint에는 `no_work_confirmed`의 true 또는 false를 명시해야 합니다.
- Confirmed worklog는 local-only입니다. Gmail MCP와 Slack payload는 기존 lightweight summary용이며 worklog 자동 전달에 사용하지 않습니다.

## 알려진 제한사항

- ChatGPT용 local MCP adapter는 아직 제공하지 않습니다.
- 주간 회고 command, state, artifact는 아직 제공하지 않습니다.
- Skill 자체는 일정을 예약하지 않습니다.
- 다섯 source의 모든 과거·미래 history schema를 지원하지 않습니다.
- OpenCode의 현재 SQLite storage는 탐지만 하며 읽지 않습니다.
- Cursor와 GitHub Copilot adapter는 experimental 범위입니다.
- Source가 없거나 읽지 못해도 실제 업무가 없다고 단정하지 않고 사용자 회상으로 진행합니다.
- Confirmed flow는 read-only이며 같은 날짜에 자동으로 새 flow를 만들지 않습니다.
- Symlink ancestor 검사는 path 기반입니다. 동일 machine의 공격자가 검사 직후 ancestor를 교체하는 descriptor-relative TOCTOU까지 방어하는 구현은 이번 범위에 포함되지 않습니다.

## 검증

다음 명령으로 CLI, machine protocol, source adapter, generated skill, 보안 계약과 npm package 구성을 검증합니다.

```bash
bun run test
bun run pack:dry-run
go test -race ./internal/worklog ./internal/sources ./internal/safeio ./internal/skill ./internal/cli
go vet ./...
git diff --check
```

Generated `SKILL.md`는 Agent Skills 명세 검증기에도 통과해야 합니다.
