# ai-bricklaying

<p align="center">
  <img src="assets/ai-bricklaying.png" alt="ai-bricklaying logo" style="width:400px;"/>
</p>

`ai-bricklaying`은 AI 코딩 세션 기록을 가벼운 하루 회고로 만들고, 사용 중인 AI 도구에 재사용 가능한 skill을 설치해 주는 Node.js CLI입니다. 오늘 얻은 교훈, 개선한 점, 다음에 AI를 더 잘 쓰는 방법을 남기고 싶은 사용자를 위한 도구입니다.

## 하는 일

- OpenCode, Claude Code, Codex, Cursor, GitHub Copilot 중 하나의 세션 소스를 요약합니다.
- 선택한 하나 이상의 AI agent skill directory에 generated skill을 설치합니다.
- 가벼운 Markdown summary를 `YYYY-MM-DD-ai-bricklaying-daily-summary.md` 형식으로 저장합니다.
- `ai-bricklaying-summary-skill.json` metadata를 저장합니다.
- 선택적으로 Gmail MCP 전달 정보를 준비합니다.
- 선택적으로 생성된 summary에서 Slack 전송용 payload를 만듭니다.
- 다음 실행에서 재사용할 기본값을 `~/.config/ai-bricklaying/config.json`에 저장합니다.

## 요구사항

- Node.js 18 이상
- npm 또는 npx

## 설치

```bash
npm install -g ai-bricklaying
ai-bricklaying --help
```

설치 없이 실행하려면:

```bash
npx ai-bricklaying --help
```

설치된 버전을 확인하려면:

```bash
ai-bricklaying --version
```

## 대화형 설정

실행:

```bash
ai-bricklaying
```

Wizard는 다음을 물어봅니다.

1. Generated skill을 설치할 target AI agent. 여러 개를 선택할 수 있습니다.
2. 요약할 session source 하나. 2번 선택지는 1번에서 선택한 agent로 제한됩니다. Skill이 설치되는 agent의 세션을 요약해야 하기 때문입니다.
3. Summary 언어.
4. File save directory. 기본값은 `~/ai-bricklaying`입니다.
5. Output mode. File save는 항상 켜져 있으며, Gmail MCP와 Slack webhook은 선택 사항입니다.

생성이 완료되면 CLI는 마지막에 skill 사용법을 bold로 출력합니다.

```text
Use the generated skill: /daily-ai-session-summary
```

OpenCode에 설치했는데 바로 보이지 않으면 OpenCode를 재시작하거나 새 세션을 여세요. OpenCode는 세션 시작 시점에 skill을 로드합니다.

## 생성되는 파일

기본 저장 경로는 `~/ai-bricklaying`입니다. 다른 위치를 쓰려면 `--output-dir`를 지정하세요.

- `YYYY-MM-DD-ai-bricklaying-daily-summary.md`: 교훈, 개선점, 더 나은 AI 사용법 중심의 가벼운 summary.
- `ai-bricklaying-summary-skill.json`: metadata, 선택한 target, delivery mode, summary path, generated skill directory.
- `ai-bricklaying-slack-payload.json`: `slack-webhook` 선택 시 생성되는 Slack payload.
- `<skill-dir>/<skill-name>/SKILL.md`: 재사용 가능한 generated skill. 이후 agent 실행이 agent-local workspace 대신 같은 output 위치에 summary를 저장하도록 configured summary directory, metadata path, config path, Slack payload path를 포함합니다.

`--skill-name`은 `daily-ai-session-summary`처럼 path-safe lowercase slug여야 합니다.

## Slack 전달

Slack으로 보낼 payload를 만들려면 `slack-webhook`을 선택합니다.

```bash
ai-bricklaying \
  --non-interactive \
  --target-agent opencode \
  --sources opencode \
  --output-modes slack-webhook \
  --slack-webhook-url "https://hooks.slack.com/services/..."
```

Webhook URL은 저장된 뒤 다시 출력되지 않습니다. 기존 config에 webhook이 있으면 prompt에는 실제 URL 대신 `[configured]`로 표시됩니다.

Slack payload 동작:

- 저장된 Markdown summary를 Slack 전달의 source of truth로 사용합니다.
- Slack payload는 확정된 Markdown에서 생성하며, 사용자가 명시하지 않으면 Slack 전용 짧은 요약으로 줄이거나 재작성하지 않습니다.
- Markdown 섹션과 bullet 순서는 유지합니다. 긴 summary는 Slack block length limit을 맞추기 위해서만 `messages` 배열로 나눕니다.
- 간단한 webhook 사용을 위해 첫 batch는 top-level `text`와 `blocks`에도 들어갑니다. Heading과 list가 제대로 보이려면 `blocks`를 전송하세요.
- Payload에는 모든 top-level Markdown section이 포함됐는지 확인하는 verification metadata가 들어갑니다.

## Gmail MCP 전달

Gmail MCP로 보낼 계획이라면 `gmail-mcp`를 선택합니다.

```bash
ai-bricklaying \
  --non-interactive \
  --target-agent opencode \
  --sources opencode \
  --output-modes gmail-mcp \
  --gmail-recipient team@example.com \
  --gmail-subject "AI session summary"
```

Generated skill은 이 mode가 선택된 경우에만 Gmail MCP를 사용하라고 명시합니다.

## Config 기본값

CLI는 로컬 설정을 아래 경로에 저장합니다.

```text
~/.config/ai-bricklaying/config.json
```

저장되는 값에는 delivery 설정과 target agent, source, language, output mode, skill name, skill directory, output directory 같은 기본값이 포함됩니다. 다음 실행 시 CLI는 이 파일을 읽어 기본값으로 사용합니다.

Command-line flag는 항상 저장된 config보다 우선합니다. 테스트나 자동화에서 다른 config 위치를 쓰려면 `--config-dir`를 사용하세요.

## 최신화 방법

npm에서 CLI를 업데이트한 뒤 다시 실행하면 generated skill을 새 버전으로 갱신할 수 있습니다.

```bash
npm install -g ai-bricklaying@latest
ai-bricklaying
```

CLI는 저장된 config를 기본값으로 다시 사용하므로, 보통은 기존 prompt 값을 그대로 받아 `SKILL.md`를 재생성하면 됩니다. OpenCode에 설치했다면 재생성 후 OpenCode를 재시작하거나 새 세션을 여세요.

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
  --output-dir ~/ai-bricklaying \
  --skill-name daily-ai-session-summary
```

비대화형 실행 규칙:

- `--target-agent`는 comma-separated list를 받습니다.
- `--sources`는 정확히 하나만 받습니다.
- `--sources`는 선택한 target agent 중 하나여야 합니다.
- `--output-modes`는 `file`, `gmail-mcp`, `slack-webhook`을 받을 수 있으며 `file`은 항상 켜져 있습니다.

## OpenCode에 설치

OpenCode user skills directory에 바로 설치하려면:

```bash
ai-bricklaying \
  --non-interactive \
  --target-agent opencode \
  --sources opencode \
  --language Korean \
  --skill-name daily-ai-session-summary \
  --skill-dir ~/.config/opencode/skills
```

실행 후 skill은 아래 위치에 생성됩니다.

```text
~/.config/opencode/skills/daily-ai-session-summary/SKILL.md
```

사용 방법:

```text
/daily-ai-session-summary
```

## CLI 옵션

```text
--non-interactive                 prompt 없이 default와 flag로 실행
--target-agent <agents>           skill target: opencode,claude-code,codex,cursor,github-copilot
--target-model <label>            생성 artifact에 기록할 model label
--sources, --sessions <source>    요약할 session source 하나
--language <language>             summary 언어 [English]
--output-modes, --delivery <list> file, gmail-mcp, slack-webhook
--skill-name <slug>               generated skill directory name
--skill-dir <dir>                 skill folder를 저장할 directory
--output-dir <dir>                summary file 저장 directory [~/ai-bricklaying]
--gmail-recipient, --gmail-to     Gmail MCP recipient
--gmail-subject <subject>         Gmail MCP subject
--slack-webhook-url <url>         Slack incoming webhook URL
--config-dir <dir>                ai-bricklaying config directory
-v, --version                     version 출력
-h, --help                        help 출력
```
