# ai-bricklaying 작업 규칙

## 목적

- 이 저장소는 AI coding session history를 daily reflection과 reusable skill로 바꾸는 CLI를 관리한다.
- `README.md`와 `README.ko.md`는 사용자 설치/사용 문서이고, `ssot/*`는 제품/계약/검증 의사결정의 정본이다.
- 구현 변경 전에는 관련 `ssot` 문서를 먼저 확인하고, 사용자에게 노출되는 행동이 바뀌면 SSOT와 README를 함께 갱신한다.

## 현재 구조

- `bin/ai-bricklaying.js`: npm package의 public CLI entrypoint이자 bundled Go binary를 실행하는 launcher다.
- legacy `ai_bricklaying/*` Python package surface는 Go contract replacement 이후 제거되었다. public npm behavior를 판단할 때는 `bin/ai-bricklaying.js`와 bundled Go binary를 기준으로 한다.
- `tests/cli-node.test.js`: npm CLI acceptance와 보안/출력 회귀 테스트다.
- `tests/contracts/python-surface-mapping.md`: 제거된 Python regression test category가 현재 Go/Node contract coverage로 어디에 보존되는지 기록한다.
- `README.md`, `README.ko.md`: 사용자-facing 설치, 옵션, delivery behavior 설명이다.
- `ssot/rules.md`: SSOT 작성/검증 규칙이다.
- `ssot/interfaces/ai-bricklaying-cli.md`: CLI command, flag, stdout/stderr, exit code, artifact path interface 계약이다.
- `ssot/domains/cli/index.md`: CLI domain 정본 계약이다.
- `ssot/evaluation.md`: SSOT 자체 평가와 개선 체크리스트다.

## 정본과 생성물

- 제품 계약 정본은 `ssot/*`와 `bin/ai-bricklaying.js`의 launcher behavior, bundled Go binary의 현재 CLI behavior다.
- npm 배포 정본은 `package.json`의 `bin`과 `files` 필드를 따른다.
- `1/*`, `summaries/*`, generated skill directories, `ai-bricklaying-summary-skill.json`, `ai-bricklaying-slack-payload.json`은 실행 결과물이다. 테스트 fixture가 아닌 한 정본처럼 수정하지 않는다.
- `opencode.json`은 제품 runtime 계약이 아니다. Team mode 활성 여부나 CLI behavior 판단 근거로 사용하지 않는다.

## 작성 규칙

- SSOT Markdown 본문은 한글로 작성하되, code identifier, file path, command, option, API name은 원문을 유지한다.
- SSOT frontmatter는 `title`, `description`, `ref`만 사용한다.
- 사용자-facing behavior가 바뀌면 `ssot/interfaces/ai-bricklaying-cli.md`의 외부 interface와 `ssot/domains/cli/index.md`의 usecase, output contract, acceptance를 먼저 갱신한다.
- README는 SSOT를 풀어 쓴 사용자 문서로 유지한다. README에만 존재하는 behavior를 만들지 않는다.
- compound engineering 관점에서 세션 요약은 단순 일지보다 reusable lessons, verification evidence, better AI usage, follow-up prompt를 우선한다.

## 구현 규칙

- 외부 전송은 명시적 선택 없이는 수행하지 않는다. `file` save는 항상 켜져 있어야 한다.
- Slack webhook URL, token, password, API key 같은 secret은 stdout, summary excerpt, Slack payload preview에 노출하지 않는다.
- OpenCode, Claude Code, Codex, Cursor, GitHub Copilot source path는 best-effort local discovery로 다룬다. source가 비어도 summary와 skill은 생성되어야 한다.
- `--sources`는 현재 README 기준 하나의 summary source만 허용한다. 다중 target skill 설치와 혼동하지 않는다.
- OpenCode skill은 세션 시작 시 로드되므로 생성 후 restart/new session 안내를 유지한다.

## 검증 규칙

```bash
bun run test
bun run pack:dry-run
```

- Node CLI behavior를 바꿨다면 최소 `bun run test`를 실행한다.
- npm 배포 파일 목록이 바뀌면 `bun run pack:dry-run`으로 package contents를 확인한다.
- 문서만 바꿨더라도 SSOT 링크, README와 SSOT의 behavioral claim, command 이름이 서로 맞는지 직접 읽어 검증한다.

## Release Notes

- Release command: `bun run release` (interactive `release-it` flow).
- Versioned releases bump `package.json`, create a `v<version>` tag, create a GitHub release, and publish the npm package with public access.
- Release hooks run `bun run test` before initialization and `bun run pack:dry-run` after the version bump.
- If release is interrupted, inspect `package.json`, `bun.lock`, git tags, and npm/GitHub release state before retrying.

## 금지 사항

- Secret 값을 README, SSOT, generated summary example에 실제 값으로 넣지 않는다.
- Gmail/Slack delivery를 selected mode 없이 자동 수행하는 behavior를 추가하지 않는다.
- Generated output을 맞추기 위해 테스트를 약화하지 않는다.
- `bin/ai-bricklaying.js`와 README/SSOT가 다른 계약을 말하게 두지 않는다.
