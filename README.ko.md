# ai-bricklaying

<p align="center">
  <img src="assets/ai-bricklaying.png" alt="ai-bricklaying logo" style="width:400px;"/>
</p>

`ai-bricklaying`은 AI 코딩 에이전트의 오늘 세션 기록을 요약해 재사용 가능한 스킬 브리프로 만들어 주는 작은 프롬프트 기반 CLI입니다. 영어 프롬프트로 진행되며, 결과 언어를 선택하고, Markdown 요약 파일과 `SKILL.md`를 저장하며, 선택적으로 Gmail MCP 또는 Slack webhook 전달에 필요한 정보를 기록합니다.

## CLI가 수집하는 정보

- 생성된 스킬을 저장할 대상 AI Agent/model. 여러 대상을 선택할 수 있습니다.
- 요약할 세션 소스 하나: OpenCode, Claude Code, Codex, Cursor, GitHub Copilot
- 요약 결과 언어
- 수행한 일, 배운 점, 결과, 개선점, compound engineering 관점을 담은 기본 영어 summary template
- 출력 방식: 파일 저장은 항상 활성화되며 Gmail MCP, Slack webhook은 선택 사항
- 선택한 전달 채널에 필요한 연동 정보

## npm으로 설치

```bash
npm install -g ai-bricklaying
ai-bricklaying --help
```

설치 없이 실행하려면:

```bash
npx ai-bricklaying --help
```

npm 패키지는 이제 의존성 없는 native Node.js CLI로 실행됩니다. Python 패키지는 Python 개발과 테스트용으로 남아 있지만, npm 사용자는 런타임에 Python이 필요하지 않습니다.

## 로컬에서 실행

로컬 CLI 사용도 Node/npm entrypoint를 사용합니다.

```bash
node bin/ai-bricklaying.js --help
```

## 대화형 흐름

```bash
ai-bricklaying
```

CLI는 checkbox 형태의 선택지, TTY 세션의 keyboard navigation, pipe 환경의 comma-separated fallback prompt, `NO_COLOR` 지원, redirect 시 ANSI color 비활성화를 갖춘 terminal-first wizard로 진행되며 다음 파일을 저장합니다.

- 선택한 output directory의 `ai-bricklaying-summary-skill.md`
- 선택한 output directory의 `ai-bricklaying-summary-skill.json` metadata
- Slack webhook 전달을 선택한 경우 `ai-bricklaying-slack-payload.json` Slack mrkdwn payload
- `<selected skill directory>/<skill-name>/SKILL.md`

## 비대화형 예시

```bash
ai-bricklaying \
  --non-interactive \
  --target-agent opencode,codex \
  --sources opencode \
  --language Korean \
  --output-modes gmail-mcp,slack-webhook \
  --gmail-recipient team@example.com \
  --gmail-subject "AI session summary" \
  --slack-webhook-url "https://hooks.slack.com/services/..." \
  --output-dir /tmp/ai-bricklaying-demo/out \
  --skill-dir /tmp/ai-bricklaying-demo/skills
```

npm으로 실행해도 동일한 플래그를 사용할 수 있습니다.

```bash
npx ai-bricklaying --non-interactive --output-dir /tmp/ai-bricklaying-demo/out --skill-dir /tmp/ai-bricklaying-demo/skills
```

비대화형 실행에서 `--target-agent`는 comma-separated skill target 목록을 받고, `--sources`는 요약할 source 하나만 받습니다.

생성된 skill을 OpenCode에 설치하려면 `--skill-dir`를 OpenCode user skills directory로 지정하세요.

```bash
ai-bricklaying \
  --non-interactive \
  --target-agent opencode \
  --sources opencode \
  --language Korean \
  --skill-name daily-ai-session-summary \
  --skill-dir ~/.config/opencode/skills
```

실행 후 skill 파일은 `~/.config/opencode/skills/daily-ai-session-summary/SKILL.md`에 생성되어야 합니다.

`--skill-name`은 path-safe lowercase slug여야 합니다. 생성된 스킬이 선택한 `--skill-dir` 아래에만 저장되도록 하기 위한 제한입니다.

Slack webhook URL은 기본적으로 `~/.config/ai-bricklaying/config.json`에 저장됩니다. 테스트나 자동화에서는 `--config-dir`로 위치를 바꿀 수 있습니다.

Slack webhook 전달을 선택하면 CLI는 Slack `mrkdwn`과 blocks 형식을 사용하는 `ai-bricklaying-slack-payload.json`도 생성합니다. 섹션 이름은 `*Work Completed*`처럼 Slack bold 문법으로 강조되고, skill 이름은 inline code로 표시됩니다.

대상 agent가 OpenCode인 경우 생성된 skill은 선택한 OpenCode skills directory 아래에 저장됩니다. OpenCode는 세션 시작 시점에 skill을 로드하므로, 바로 보이지 않으면 OpenCode를 재시작하거나 새 세션을 열어 주세요.

npm 개발용으로는 native Node CLI를 직접 실행합니다.

```bash
node bin/ai-bricklaying.js --help
```

Python 개발용으로는 Python 모듈을 `python3 -m ai_bricklaying.cli`로 직접 실행할 수 있습니다.

## 테스트

```bash
npm test
npm pack --dry-run --json
```

npm CLI는 dependency-free Node.js로 동작합니다. Python package와 테스트는 계속 Python standard library만 사용합니다.
