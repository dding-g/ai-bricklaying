#!/usr/bin/env node

const fs = require('fs');
const os = require('os');
const path = require('path');
const readline = require('readline');
const { markdownToBlocks, splitBlocksWithText } = require('markdown-to-slack-blocks');

const PACKAGE_JSON = require('../package.json');

const HOME = os.homedir();
const CLI_VERSION = PACKAGE_JSON.version;
const DEFAULT_OUTPUT_DIR = path.join(HOME, 'ai-bricklaying');
const OUTPUT_MODES = ['file', 'gmail-mcp', 'slack-webhook'];
const TEXT_EXTENSIONS = new Set(['.json', '.jsonl', '.md', '.txt', '.log']);
const SKILL_NAME_PATTERN = /^[a-z0-9][a-z0-9._-]*$/;
const SENSITIVE_KEY_PATTERN = /(?:password|secret|token|api[_-]?key|access[_-]?key|private[_-]?key|webhook)/i;

const AGENT_TARGETS = [
  ['OpenCode', path.join(HOME, '.config/opencode/skills'), 'configured OpenCode model'],
  ['Claude Code', path.join(HOME, '.claude/skills'), 'configured Claude model'],
  ['Codex', path.join(HOME, '.codex/skills'), 'configured Codex model'],
  ['Cursor', path.join(HOME, '.cursor/skills'), 'configured Cursor model'],
  ['GitHub Copilot', path.join(HOME, '.github-copilot/skills'), 'configured Copilot model'],
].map(([name, defaultSkillDir, modelHint]) => ({ name, defaultSkillDir, modelHint }));

const SESSION_SOURCES = [
  ['opencode', 'OpenCode', [path.join(HOME, '.local/share/opencode')], 'AI_BRICKLAYING_OPENCODE_DIRS'],
  ['claude-code', 'Claude Code', [path.join(HOME, '.claude/projects')], 'AI_BRICKLAYING_CLAUDE_DIRS'],
  ['codex', 'Codex', [path.join(HOME, '.codex/sessions'), path.join(HOME, '.codex')], 'AI_BRICKLAYING_CODEX_DIRS'],
  ['cursor', 'Cursor', [path.join(HOME, 'Library/Application Support/Cursor/User/workspaceStorage')], 'AI_BRICKLAYING_CURSOR_DIRS'],
  ['github-copilot', 'GitHub Copilot', [path.join(HOME, 'Library/Application Support/Code/User/workspaceStorage')], 'AI_BRICKLAYING_COPILOT_DIRS'],
].map(([key, label, defaultPaths, envVar]) => ({ key, label, defaultPaths, envVar }));

const TARGET_BY_KEY = new Map(AGENT_TARGETS.map((target) => [slug(target.name), target]));
const SOURCE_BY_KEY = new Map(SESSION_SOURCES.map((source) => [source.key, source]));
let bufferedPromptAnswers;

const DEFAULT_SUMMARY_TEMPLATE = `# Daily AI Bricklaying Summary

Write the summary in {language}.

Keep the summary lightweight and useful for the user. Do not list raw file paths, session artifact paths, or long evidence dumps unless the user explicitly asks for them.

## Today's Takeaways
Summarize the day in 3-5 short bullets. Focus on what changed in the user's understanding, workflow, or judgment.

## Lessons Learned
Capture reusable lessons: mistakes corrected, assumptions challenged, prompts that worked or failed, tool limits discovered, and decisions worth remembering.

## What Improved
Describe the practical improvements made today: process, product, code quality, documentation, verification habits, or agent workflow.

## Better AI Usage Next Time
Evaluate how the user worked with AI today. Suggest sharper prompts, better review habits, better delegation choices, or checks that would improve the next session.

## Tomorrow's Best Next Step
Give 1-3 concrete next actions. Keep them small, high-leverage, and easy to start.

## Follow-Up Prompt
Write one concise prompt the user can paste into an AI coding agent tomorrow. It should carry forward the lesson, not a long transcript.
`;

const SUMMARY_THEMES = [
  ['implementation', ['implement', 'build', 'ship', 'feature']],
  ['debugging', ['fix', 'debug', 'error', 'bug', 'fail']],
  ['verification', ['test', 'verify', 'check', 'review']],
  ['planning', ['plan', 'scope', 'decide', 'design']],
  ['refactoring', ['refactor', 'cleanup', 'simplify']],
  ['prompting', ['prompt', 'skill', 'agent', 'session']],
  ['learning', ['learn', 'lesson', 'insight', 'improve']],
];

function useColor() {
  return Boolean(process.stdout.isTTY) && !process.env.NO_COLOR;
}

const color = {
  bold: (value) => useColor() ? `\x1b[1m${value}\x1b[22m` : value,
  muted: (value) => useColor() ? `\x1b[2m${value}\x1b[22m` : value,
  cyan: (value) => useColor() ? `\x1b[36m${value}\x1b[39m` : value,
  green: (value) => useColor() ? `\x1b[32m${value}\x1b[39m` : value,
  yellow: (value) => useColor() ? `\x1b[33m${value}\x1b[39m` : value,
};

function slug(value) {
  return value.toLowerCase().replace(/ /g, '-');
}

function fileSlug(value) {
  return String(value)
    .toLowerCase()
    .replace(/`/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '') || 'summary';
}

function titleFromMarkdown(markdown) {
  const heading = String(markdown).split('\n').find((line) => line.startsWith('# '));
  return heading ? heading.replace(/^#\s+/, '').trim() : 'AI Bricklaying Summary';
}

function summaryFileName(markdown, dateKey = localDateKey(new Date())) {
  const title = titleFromMarkdown(markdown)
    .replace(new RegExp(`(^|\\s+-\\s+)${dateKey}$`), '')
    .trim();
  return `${dateKey}-${fileSlug(title)}.md`;
}

function expandHome(value) {
  if (!value) return value;
  if (value === '~') return HOME;
  if (value.startsWith(`~${path.sep}`)) return path.join(HOME, value.slice(2));
  return value;
}

function parseArgs(argv) {
  const args = {
    provided: new Set(),
    nonInteractive: false,
    targetModel: 'configured model',
    language: 'English',
    outputModes: 'file',
    skillName: 'daily-ai-session-summary',
    outputDir: DEFAULT_OUTPUT_DIR,
    configDir: path.join(HOME, '.config/ai-bricklaying'),
  };
  const aliases = {
    '--sessions': '--sources',
    '--delivery': '--output-modes',
    '--gmail-to': '--gmail-recipient',
  };
  const keyMap = {
    '--target-agent': 'targetAgent',
    '--target-model': 'targetModel',
    '--sources': 'sources',
    '--language': 'language',
    '--output-modes': 'outputModes',
    '--skill-name': 'skillName',
    '--skill-dir': 'skillDir',
    '--output-dir': 'outputDir',
    '--gmail-recipient': 'gmailRecipient',
    '--gmail-subject': 'gmailSubject',
    '--slack-webhook-url': 'slackWebhookUrl',
    '--config-dir': 'configDir',
  };

  for (let index = 0; index < argv.length; index += 1) {
    const raw = argv[index];
    const [flagPart, inlineValue] = raw.split(/=(.*)/s, 2);
    const flag = aliases[flagPart] || flagPart;
    if (flag === '--help' || flag === '-h') {
      args.help = true;
      continue;
    }
    if (flag === '--version' || flag === '-v') {
      args.version = true;
      continue;
    }
    if (flag === '--non-interactive') {
      args.nonInteractive = true;
      args.provided.add('nonInteractive');
      continue;
    }
    const key = keyMap[flag];
    if (!key) {
      throw new CliError(`Unknown argument: ${raw}`);
    }
    const value = inlineValue !== undefined ? inlineValue : argv[index + 1];
    if (value === undefined || value.startsWith('--')) {
      throw new CliError(`${flag} requires a value`);
    }
    args[key] = value;
    args.provided.add(key);
    if (inlineValue === undefined) index += 1;
  }
  return args;
}

function readConfigFile(configDir) {
  const filePath = path.join(path.resolve(expandHome(configDir)), 'config.json');
  if (!fs.existsSync(filePath)) return {};
  try {
    return JSON.parse(fs.readFileSync(filePath, 'utf8'));
  } catch (error) {
    throw new CliError(`Could not read config file ${filePath}: ${error.message}`);
  }
}

function applyConfigDefaults(args, storedConfig) {
  const delivery = storedConfig.delivery || {};
  const defaults = storedConfig.defaults || {};
  const provided = args.provided;

  if (!provided.has('targetAgent') && Array.isArray(defaults.target_agents) && defaults.target_agents.length) {
    args.targetAgent = defaults.target_agents.join(',');
  }
  if (!provided.has('sources') && defaults.source) args.sources = defaults.source;
  if (!provided.has('targetModel') && defaults.target_model) args.targetModel = defaults.target_model;
  if (!provided.has('language') && defaults.language) args.language = defaults.language;
  if (!provided.has('outputModes') && Array.isArray(defaults.output_modes) && defaults.output_modes.length) {
    args.outputModes = defaults.output_modes.join(',');
  }
  if (!provided.has('skillName') && defaults.skill_name) args.skillName = defaults.skill_name;
  if (!provided.has('skillDir') && defaults.skill_dir) args.skillDir = defaults.skill_dir;
  if (!provided.has('outputDir') && defaults.output_dir) args.outputDir = defaults.output_dir;
  if (!provided.has('gmailRecipient') && delivery.gmail_recipient) args.gmailRecipient = delivery.gmail_recipient;
  if (!provided.has('gmailSubject') && delivery.gmail_subject) args.gmailSubject = delivery.gmail_subject;
  if (!provided.has('slackWebhookUrl') && delivery.slack_webhook_url) args.slackWebhookUrl = delivery.slack_webhook_url;
  return args;
}

class CliError extends Error {}

function safeConsole(value) {
  return sanitizeControl(String(value));
}

function sanitizeControl(value) {
  return value.replace(/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f\x9b]/g, '');
}

function redactSecrets(value) {
  return sanitizeControl(String(value))
    .replace(/https:\/\/hooks\.slack\.com\/services\/[^\s)\]}'\"`<>]+/g, '[REDACTED SLACK WEBHOOK]')
    .replace(/-----BEGIN [^-]+ PRIVATE KEY-----[\s\S]*?-----END [^-]+ PRIVATE KEY-----/g, '[REDACTED PRIVATE KEY]')
    .replace(/\bBearer\s+[^\s,;]+/gi, 'Bearer=[REDACTED]')
    .replace(/([\"']?(?:token|api[_-]?key|secret|password)[\"']?\s*[:=]\s*)[\"']?[^\s,;}\"']+/gi, '$1[REDACTED]')
    .replace(/\b(?:sk|pk)_[A-Za-z0-9]{20,}\b/g, '[REDACTED TOKEN]');
}

function printHelp() {
  console.log(`Summarize today's AI agent sessions and generate a reusable skill.

Usage:
  ai-bricklaying [options]

Options:
  --non-interactive                 Use defaults and flags without prompting
  --target-agent <agents>           Skill targets: opencode,claude-code,codex,cursor,github-copilot
  --target-model <label>            Model label recorded in generated artifacts
  --sources, --sessions <source>    Single session source to summarize
  --language <language>             Language for the generated summary [English]
  --output-modes, --delivery <list> file, gmail-mcp, slack-webhook
  --skill-name <slug>               Generated skill directory name
  --skill-dir <dir>                 Directory where the skill folder is written
  --output-dir <dir>                Directory for summary files [~/ai-bricklaying]
  --gmail-recipient, --gmail-to     Gmail MCP recipient
  --gmail-subject <subject>         Gmail MCP subject
  --slack-webhook-url <url>         Slack incoming webhook URL
  --config-dir <dir>                ai-bricklaying config directory
  -v, --version                     Show CLI version
  -h, --help                        Show this help
`);
}

function csv(value) {
  return value ? value.split(',').map((part) => part.trim()).filter(Boolean) : [];
}

function outputModeKey(value) {
  return { gmail: 'gmail-mcp', slack: 'slack-webhook' }[value] || value;
}

function outputModesFromValue(value) {
  const modes = new Set(['file', ...csv(value).map(outputModeKey)]);
  const unknown = [...modes].filter((mode) => !OUTPUT_MODES.includes(mode)).sort();
  if (unknown.length) throw new CliError(`Unknown output mode(s): ${unknown.join(', ')}`);
  return OUTPUT_MODES.filter((mode) => modes.has(mode));
}

function targetsFromArgs(args) {
  const keys = csv(args.targetAgent || 'opencode');
  const unknown = keys.filter((key) => !TARGET_BY_KEY.has(key));
  if (unknown.length) throw new CliError(`Unknown target agent(s): ${unknown.join(', ')}`);
  return keys.map((key) => ({ ...TARGET_BY_KEY.get(key), modelHint: args.targetModel }));
}

function sourceOptionsForTargets(targets) {
  const seen = new Set();
  const sources = [];
  for (const target of targets) {
    const key = slug(target.name);
    const source = SOURCE_BY_KEY.get(key);
    if (source && !seen.has(source.key)) {
      sources.push(source);
      seen.add(source.key);
    }
  }
  return sources;
}

function sourcesFromArgs(args, targets) {
  const allowedSources = sourceOptionsForTargets(targets);
  const allowedKeys = new Set(allowedSources.map((source) => source.key));
  const keys = csv(args.sources);
  if (keys.length === 0) return allowedSources.slice(0, 1);
  if (keys.length > 1) throw new CliError('--sources accepts exactly one summary source');
  const unknown = keys.filter((key) => !SOURCE_BY_KEY.has(key));
  if (unknown.length) throw new CliError(`Unknown source(s): ${unknown.join(', ')}`);
  const notSelected = keys.filter((key) => !allowedKeys.has(key));
  if (notSelected.length) {
    throw new CliError(`--sources must be one of the selected target agents: ${[...allowedKeys].join(', ')}`);
  }
  return keys.map((key) => SOURCE_BY_KEY.get(key));
}

function outputModesFromArgs(args) {
  const modes = new Set(outputModesFromValue(args.outputModes));
  if (modes.has('gmail-mcp') && (!args.gmailRecipient || !args.gmailSubject)) {
    throw new CliError('gmail-mcp requires --gmail-recipient and --gmail-subject');
  }
  if (modes.has('slack-webhook') && !args.slackWebhookUrl) {
    throw new CliError('slack-webhook requires --slack-webhook-url');
  }
  return OUTPUT_MODES.filter((mode) => modes.has(mode));
}

function validateSkillName(value) {
  if (!SKILL_NAME_PATTERN.test(value) || value.includes('..')) {
    throw new CliError('--skill-name must be a path-safe slug using lowercase letters, numbers, dots, underscores, or hyphens');
  }
}

async function prompt(question, defaultValue) {
  const suffix = defaultValue === undefined ? '' : ` [${defaultValue}]`;
  if (!process.stdin.isTTY) {
    if (!bufferedPromptAnswers) {
      bufferedPromptAnswers = fs.readFileSync(0, 'utf8').split(/\r?\n/);
    }
    process.stdout.write(`${question}${suffix}: `);
    const answer = bufferedPromptAnswers.length ? bufferedPromptAnswers.shift() : '';
    process.stdout.write(`${answer || ''}\n`);
    return answer.trim() || defaultValue || '';
  }
  const rl = readline.createInterface({ input: process.stdin, output: process.stdout });
  const answer = await new Promise((resolve) => rl.question(`${question}${suffix}: `, resolve));
  rl.close();
  return answer.trim() || defaultValue || '';
}

async function promptConfiguredSecret(question, currentValue) {
  const answer = await prompt(question, currentValue ? 'configured' : undefined);
  if (answer === 'configured') return currentValue || null;
  return answer || null;
}

function printIntro() {
  if (!process.stdout.isTTY) return;
  console.log('');
  console.log(color.bold('AI Bricklaying'));
  console.log(color.muted('Build a reusable daily AI-session summary skill.'));
  console.log(color.muted('[Space] toggle  [Enter] confirm  [j/k] move  [q] quit'));
}

function canUseWizard() {
  return Boolean(process.stdin.isTTY && process.stdout.isTTY && process.stdin.setRawMode);
}

async function chooseOne(title, options, label) {
  if (canUseWizard()) {
    const selected = await runSelectionWizard({
      title,
      options,
      label,
      multiple: false,
      selectedIndexes: [0],
    });
    return selected[0];
  }
  return chooseOneLineMode(title, options, label);
}

async function chooseOneLineMode(title, options, label) {
  console.log(`\n${color.cyan(title)}`);
  options.forEach((option, index) => {
    const marker = index === 0 ? '[x]' : '[ ]';
    console.log(`  ${index + 1}. ${marker} ${label(option)}`);
  });
  while (true) {
    const answer = await prompt('Select number', '1');
    const number = Number(answer);
    if (Number.isInteger(number) && number >= 1 && number <= options.length) {
      return options[number - 1];
    }
    console.log(color.yellow('Enter one of the listed numbers.'));
  }
}

async function chooseMany(title, options, label = (option) => option.label, promptLabel = 'Selection', selectedIndexes = options.map((_option, index) => index)) {
  if (canUseWizard()) {
    return runSelectionWizard({
      title,
      options,
      label,
      multiple: true,
      selectedIndexes,
    });
  }
  return chooseManyLineMode(title, options, label, promptLabel, selectedIndexes);
}

async function chooseManyLineMode(title, options, label = (option) => option.label, promptLabel = 'Selection', selectedIndexes = options.map((_option, index) => index)) {
  const defaultIndexes = selectedIndexes.length ? selectedIndexes : options.map((_option, index) => index);
  const defaultAnswer = defaultIndexes.length === options.length ? 'all' : defaultIndexes.map((index) => index + 1).join(',');
  console.log(`\n${color.cyan(title)}`);
  options.forEach((option, index) => {
    const marker = defaultIndexes.includes(index) ? '[x]' : '[ ]';
    console.log(`  ${index + 1}. ${marker} ${label(option)}`);
  });
  console.log(color.muted('Tip: leave blank for all, or enter comma-separated numbers like 1,3,5.'));
  const answer = await prompt(promptLabel, defaultAnswer);
  if (answer.toLowerCase() === 'all') return options;
  const selected = [];
  for (const part of answer.split(',')) {
    const number = Number(part.trim());
    if (Number.isInteger(number) && number >= 1 && number <= options.length) {
      selected.push(options[number - 1]);
    }
  }
  return selected.length ? selected : options;
}

async function chooseOutputModes(defaultModes = ['file']) {
  const options = [
    { mode: 'file', label: 'File save (always enabled)', fixed: true },
    { mode: 'gmail-mcp', label: 'Gmail MCP delivery notes' },
    { mode: 'slack-webhook', label: 'Slack webhook config' },
  ];
  const defaultIndexes = options
    .map((option, index) => defaultModes.includes(option.mode) ? index : null)
    .filter((index) => index !== null);
  if (canUseWizard()) {
    const selected = await runSelectionWizard({
      title: '5. Select output modes',
      options,
      label: (option) => option.label,
      multiple: true,
      selectedIndexes: defaultIndexes.length ? defaultIndexes : [0],
      fixedIndexes: [0],
    });
    const modes = new Set(selected.map((option) => option.mode));
    modes.add('file');
    return OUTPUT_MODES.filter((mode) => modes.has(mode));
  }
  return chooseOutputModesLineMode(options, defaultIndexes.length ? defaultIndexes : [0]);
}

async function chooseOutputModesLineMode(options, defaultIndexes = [0]) {
  console.log(`\n${color.cyan('5. Select output modes')}`);
  options.forEach((option, index) => {
    const marker = option.fixed || defaultIndexes.includes(index) ? '[x]' : '[ ]';
    const fixed = option.fixed ? ' - required' : '';
    console.log(`  ${index + 1}. ${marker} ${option.label}${fixed}`);
  });
  const answer = await prompt('Optional modes', defaultIndexes.map((index) => index + 1).join(','));
  const modes = new Set(['file']);
  if (answer.includes('2')) modes.add('gmail-mcp');
  if (answer.includes('3')) modes.add('slack-webhook');
  return OUTPUT_MODES.filter((mode) => modes.has(mode));
}

function renderWizard({ title, options, label, cursor, selected, fixed }) {
  const lines = [
    '',
    color.cyan(title),
    color.muted('[Space] toggle  [Enter] confirm  [j/k] move  [q] quit'),
    '',
  ];
  options.forEach((option, index) => {
    const pointer = index === cursor ? '>' : ' ';
    const checked = selected.has(index) || fixed.has(index) ? '[x]' : '[ ]';
    const suffix = fixed.has(index) ? color.muted(' (required)') : '';
    const text = `${pointer} ${checked} ${label(option)}${suffix}`;
    lines.push(index === cursor ? color.bold(text) : text);
  });
  lines.push('', color.muted('Wizard step: choose with the keyboard, then press Enter to continue.'));
  return `${lines.join('\n')}\n`;
}

function runSelectionWizard({ title, options, label, multiple, selectedIndexes = [], fixedIndexes = [] }) {
  return new Promise((resolve, reject) => {
    const selected = new Set(selectedIndexes);
    const fixed = new Set(fixedIndexes);
    let cursor = 0;
    const stdin = process.stdin;
    const previousRawMode = stdin.isRaw;

    function cleanup() {
      stdin.removeListener('keypress', onKeypress);
      if (stdin.setRawMode) stdin.setRawMode(Boolean(previousRawMode));
      stdin.pause();
      readline.cursorTo(process.stdout, 0, 0);
      readline.clearScreenDown(process.stdout);
    }

    function finish() {
      cleanup();
      const indexes = multiple
        ? [...selected].sort((left, right) => left - right)
        : [cursor];
      const safeIndexes = indexes.length
        ? indexes
        : (fixed.size ? [...fixed].sort((left, right) => left - right) : options.map((_option, index) => index));
      resolve(safeIndexes.map((index) => options[index]).filter(Boolean));
    }

    function redraw() {
      readline.cursorTo(process.stdout, 0, 0);
      readline.clearScreenDown(process.stdout);
      process.stdout.write(renderWizard({ title, options, label, cursor, selected, fixed }));
    }

    function toggle() {
      if (fixed.has(cursor)) return;
      if (!multiple) {
        selected.clear();
        selected.add(cursor);
        return;
      }
      if (selected.has(cursor)) selected.delete(cursor);
      else selected.add(cursor);
    }

    function move(delta) {
      cursor = Math.max(0, Math.min(options.length - 1, cursor + delta));
      if (!multiple) {
        selected.clear();
        selected.add(cursor);
      }
    }

    function onKeypress(_chunk, key = {}) {
      if (key.ctrl && key.name === 'c') {
        cleanup();
        return reject(new CliError('Cancelled.'));
      }
      if (key.name === 'up' || key.name === 'k') move(-1);
      else if (key.name === 'down' || key.name === 'j') move(1);
      else if (key.name === 'space') toggle();
      else if (key.name === 'return' || key.name === 'enter') return finish();
      else if (key.name === 'escape' || key.name === 'q') {
        cleanup();
        return reject(new CliError('Cancelled.'));
      }
      redraw();
      return undefined;
    }

    readline.emitKeypressEvents(stdin);
    stdin.setRawMode(true);
    stdin.resume();
    stdin.on('keypress', onKeypress);
    redraw();
  });
}

function renderTemplate(language) {
  return DEFAULT_SUMMARY_TEMPLATE.replace('{language}', language);
}

function collectTodaySessions(source, today = new Date(), limit = 12) {
  const todayKey = localDateKey(today);
  const records = [];
  for (const root of sourcePaths(source)) {
    if (!fs.existsSync(root)) continue;
    const candidates = candidateFiles(root).sort((left, right) => fs.statSync(right).mtimeMs - fs.statSync(left).mtimeMs);
    for (const filePath of candidates) {
      if (records.length >= limit) return records;
      if (localDateKey(fs.statSync(filePath).mtime) !== todayKey) continue;
      const text = readSessionText(filePath);
      if (text) records.push({ source: source.label, path: filePath, text });
    }
  }
  return records;
}

function sourcePaths(source) {
  const configured = process.env[source.envVar];
  if (configured) return configured.split(path.delimiter).filter(Boolean).map((part) => path.resolve(expandHome(part)));
  return source.defaultPaths;
}

function candidateFiles(root) {
  const stat = safeLstat(root);
  if (!stat) return [];
  if (stat.isFile()) return TEXT_EXTENSIONS.has(path.extname(root).toLowerCase()) ? [root] : [];
  const files = [];
  const stack = [root];
  while (stack.length) {
    const current = stack.pop();
    let entries;
    try {
      entries = fs.readdirSync(current, { withFileTypes: true });
    } catch (_error) {
      continue;
    }
    for (const entry of entries) {
      const next = path.join(current, entry.name);
      if (entry.isSymbolicLink()) continue;
      if (entry.isDirectory()) stack.push(next);
      if (entry.isFile() && TEXT_EXTENSIONS.has(path.extname(entry.name).toLowerCase())) files.push(next);
    }
  }
  return files;
}

function safeLstat(filePath) {
  try {
    return fs.lstatSync(filePath);
  } catch (_error) {
    return null;
  }
}

function rejectSymlink(filePath) {
  try {
    if (fs.lstatSync(filePath).isSymbolicLink()) {
      throw new CliError(`Refusing to overwrite symbolic link: ${filePath}`);
    }
  } catch (error) {
    if (error instanceof CliError) throw error;
    if (error && error.code !== 'ENOENT') throw error;
  }
}

function writeFileSafely(filePath, contents, mode = 0o644) {
  ensureDir(path.dirname(filePath));
  rejectSymlink(filePath);
  const tempPath = path.join(path.dirname(filePath), `.${path.basename(filePath)}.${process.pid}.${Date.now()}.${Math.random().toString(36).slice(2)}.tmp`);
  let descriptor;
  try {
    descriptor = fs.openSync(tempPath, 'wx', mode);
    fs.writeFileSync(descriptor, contents, { encoding: 'utf8' });
    fs.fchmodSync(descriptor, mode);
  } finally {
    if (descriptor !== undefined) fs.closeSync(descriptor);
  }
  fs.renameSync(tempPath, filePath);
  try {
    fs.chmodSync(filePath, mode);
  } catch (_error) {
    // Some platforms ignore POSIX modes; the file was still written safely.
  }
}

function readSessionText(filePath, maxChars = 20000) {
  let raw;
  try {
    raw = redactSecrets(fs.readFileSync(filePath, 'utf8').slice(0, maxChars));
  } catch (_error) {
    return '';
  }
  if (!raw.trim()) return '';
  const extension = path.extname(filePath).toLowerCase();
  if (extension === '.jsonl') return jsonlToText(raw);
  if (extension === '.json') return jsonToText(raw);
  return raw;
}

function jsonlToText(raw) {
  const lines = [];
  for (const line of raw.split(/\r?\n/)) {
    if (!line.trim()) continue;
    try {
      const value = JSON.parse(line);
      const text = extractText(value);
      if (text) lines.push(text);
    } catch (_error) {
      lines.push(line.trim());
    }
  }
  return lines.join('\n');
}

function jsonToText(raw) {
  try {
    const text = extractText(JSON.parse(raw));
    return text || raw;
  } catch (_error) {
    return raw;
  }
}

function extractText(value) {
  if (typeof value === 'string') return value;
  if (Array.isArray(value)) return value.map(extractText).filter(Boolean).join('\n');
  if (value && typeof value === 'object') {
    const preferred = ['text', 'content', 'message', 'prompt', 'response', 'summary'];
    const parts = [];
    for (const key of preferred) {
      if (Object.prototype.hasOwnProperty.call(value, key)) {
        const text = extractText(value[key]);
        if (text) parts.push(text);
      }
    }
    for (const [key, child] of Object.entries(value)) {
      if (SENSITIVE_KEY_PATTERN.test(key)) {
        parts.push(`${key}: [REDACTED]`);
      } else if (!preferred.includes(key)) {
        const text = extractText(child);
        if (text) parts.push(text);
      }
    }
    return parts.join('\n');
  }
  return '';
}

function localDateKey(date) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

function buildSummary(config, records) {
  const themes = summarizeThemes(records);

  const lines = [
    `# AI Bricklaying Daily Summary - ${localDateKey(new Date())}`,
    '',
    `Language: ${config.language}`,
    `Target skill: ${config.skillName}`,
    `Summary source: ${config.selectedSources.map((source) => source.label).join(', ') || 'none selected'}`,
    '',
    '## Lightweight Session Signals',
  ];

  if (records.length) {
    lines.push(`- Found ${records.length} local session signal(s) from ${config.selectedSources.map((source) => source.label).join(', ')}.`);
    if (themes.length) {
      lines.push(`- Likely themes to reflect on: ${themes.join(', ')}.`);
    }
    lines.push('- Use the template below to turn those signals into a short lesson-focused reflection instead of an artifact log.');
  } else {
    lines.push('- No clear session signals were found today. Use the template below as a lightweight reflection prompt.');
  }

  lines.push('', '## Summary Template For AI Agent', '', renderTemplate(config.language), '', '## Delivery Notes');
  lines.push('- File save: enabled');
  if (config.outputModes.includes('gmail-mcp')) {
    lines.push(`- Gmail MCP: prepare an email draft for ${config.gmailRecipient || 'not provided'} with subject ${config.gmailSubject || 'not provided'}`);
  }
  if (config.outputModes.includes('slack-webhook')) {
    const status = config.slackWebhookUrl ? 'configured' : 'not provided';
    lines.push(`- Slack webhook URL: ${status}; use the saved config file for delivery`);
  }
  return `${lines.join('\n').trim()}\n`;
}

function summarizeThemes(records) {
  const text = records.map((record) => record.text.toLowerCase()).join('\n');
  return SUMMARY_THEMES
    .map(([theme, words]) => ({
      theme,
      count: words.reduce((total, word) => total + (text.match(new RegExp(`\\b${word}`, 'g')) || []).length, 0),
    }))
    .filter((item) => item.count > 0)
    .sort((left, right) => right.count - left.count)
    .slice(0, 4)
    .map((item) => item.theme);
}

function slackFallbackText(markdown, index = 0, total = 1) {
  const suffix = total > 1 ? ` (${index + 1}/${total})` : '';
  return `${titleFromMarkdown(markdown)}${suffix}`;
}

function buildSlackPayload(markdown) {
  const blocks = markdownToBlocks(markdown);
  const batches = splitBlocksWithText(blocks);
  const messages = batches.map((message, index) => ({
    text: slackFallbackText(markdown, index, batches.length),
    blocks: message.blocks,
  }));
  const [firstMessage] = messages;

  return {
    text: firstMessage ? firstMessage.text : slackFallbackText(markdown),
    blocks: firstMessage ? firstMessage.blocks : [],
    messages,
  };
}

function ensureDir(dirPath, mode = 0o755) {
  fs.mkdirSync(dirPath, { recursive: true, mode });
}

function ensurePrivateDir(dirPath) {
  ensureDir(dirPath, 0o700);
  try {
    fs.chmodSync(dirPath, 0o700);
  } catch (_error) {
    // Best effort on platforms without POSIX mode support.
  }
}

function writeSummaryFile(config, markdown) {
  ensureDir(config.outputDir);
  const filePath = path.join(config.outputDir, summaryFileName(markdown));
  writeFileSafely(filePath, markdown);
  return filePath;
}

function writeMetadataFile(config, summaryPath, sessionCount) {
  ensureDir(config.outputDir);
  const filePath = path.join(config.outputDir, 'ai-bricklaying-summary-skill.json');
  const payload = {
    config_path: configPath(config),
    deliveries: config.outputModes,
    gmail_recipient: config.gmailRecipient || null,
    gmail_subject: config.gmailSubject || null,
    language: config.language,
    cli_version: CLI_VERSION,
    session_count: sessionCount,
    sessions: config.selectedSources.map((source) => source.key),
    skill_dirs: config.targets.map((target) => path.join(target.defaultSkillDir, config.skillName)),
    slack_webhook_configured: Boolean(config.slackWebhookUrl),
    slack_payload_path: config.outputModes.includes('slack-webhook') ? slackPayloadPath(config) : null,
    summary_path: summaryPath,
    target_agents: config.targets.map((target) => target.name),
    target_model: config.target.modelHint,
  };
  writeFileSafely(filePath, `${JSON.stringify(payload, null, 2)}\n`);
  return filePath;
}

function slackPayloadPath(config) {
  return path.join(config.outputDir, 'ai-bricklaying-slack-payload.json');
}

function writeSlackPayloadFile(config, markdown) {
  if (!config.outputModes.includes('slack-webhook')) return null;
  ensureDir(config.outputDir);
  const filePath = slackPayloadPath(config);
  writeFileSafely(filePath, `${JSON.stringify(buildSlackPayload(markdown), null, 2)}\n`);
  return filePath;
}

function configPath(config) {
  return path.join(config.configDir, 'config.json');
}

function writeConfigFile(config) {
  const filePath = configPath(config);
  ensurePrivateDir(path.dirname(filePath));
  const sharedSkillDir = config.targets.length && config.targets.every((target) => target.defaultSkillDir === config.targets[0].defaultSkillDir)
    ? config.targets[0].defaultSkillDir
    : null;
  const payload = {
    delivery: {
      gmail_recipient: config.gmailRecipient || null,
      gmail_subject: config.gmailSubject || null,
      slack_webhook_url: config.slackWebhookUrl || null,
    },
    defaults: {
      target_agents: config.targets.map((target) => slug(target.name)),
      source: config.selectedSources[0] ? config.selectedSources[0].key : null,
      target_model: config.target.modelHint,
      language: config.language,
      output_modes: config.outputModes,
      skill_name: config.skillName,
      skill_dir: sharedSkillDir,
      output_dir: config.outputDir,
      cli_version: CLI_VERSION,
    },
  };
  writeFileSafely(filePath, `${JSON.stringify(payload, null, 2)}\n`, 0o600);
  return filePath;
}

function writeSkill(config, target) {
  const skillDir = path.join(target.defaultSkillDir, config.skillName);
  ensureDir(skillDir);
  const filePath = path.join(skillDir, 'SKILL.md');
  const sourceNames = config.selectedSources.map((source) => source.label).join(', ') || 'none selected';
  const deliveryModes = config.outputModes.join(', ');
  const deliveryInstructions = [
    '- `file`: always save the final markdown summary locally and report the saved path.',
  ];
  if (config.outputModes.includes('gmail-mcp')) {
    deliveryInstructions.push('- `gmail-mcp`: when the CLI result includes this mode, prepare or send the saved markdown summary through Gmail MCP using the configured recipient and subject. If recipient, subject, or authorization is missing, report the exact missing requirement instead of guessing.');
  }
  if (config.outputModes.includes('slack-webhook')) {
    deliveryInstructions.push('- `slack-webhook`: when the CLI result includes this mode, read `ai-bricklaying-slack-payload.json` and post each entry in `messages` to the saved webhook. Send the JSON blocks, not the raw Markdown text, so headings and lists render as Slack Block Kit. Report any missing webhook URL instead of exposing or inventing secrets.');
  }
  if (config.outputModes.length === 1 && config.outputModes[0] === 'file') {
    deliveryInstructions.push('- File-only mode: do not attempt Gmail, Slack, or any external delivery unless the user explicitly asks for a new delivery mode later.');
  }
  const markdown = `---
name: ${config.skillName}
description: Summarize today's AI coding agent sessions into a useful compound-engineering briefing for the user.
---

# ${config.skillName}

Use this skill when the user asks for a daily summary of AI coding work, session history, agent activity, or compound-engineering learnings.

## Sources

Default session sources: ${sourceNames}.

## CLI Result Delivery Modes

This skill was generated from the CLI result with delivery modes: ${deliveryModes}.

Follow the CLI-selected delivery modes exactly:

${deliveryInstructions.join('\n')}

## Workflow

1. Gather today's session history from the selected agents.
2. Identify actual work completed, decisions made, verification evidence, failed attempts, and reusable lessons.
3. Write the result in ${config.language}.
4. Apply the CLI result delivery modes above: save locally, then optionally deliver through Gmail MCP or Slack webhook only when those modes were selected.
5. Report saved files and delivery outcomes without printing secrets.

## Summary Template

${renderTemplate(config.language)}
`;
  writeFileSafely(filePath, markdown);
  return filePath;
}

function writeSkills(config) {
  return config.targets.map((target) => writeSkill(config, target));
}

function printCompletion(config, paths) {
  console.log(`\n${color.green('[ok]')} ${color.bold('AI Bricklaying files generated')}`);
  console.log(`  Summary:  ${safeConsole(paths.summaryPath)}`);
  console.log(`  Metadata: ${safeConsole(paths.metadataPath)}`);
  console.log(`  Config:   ${safeConsole(paths.configPath)}`);
  if (paths.slackPayloadPath) console.log(`  Slack:    ${safeConsole(paths.slackPayloadPath)}`);
  for (const skillPath of paths.skillPaths) {
    console.log(`  Skill:    ${safeConsole(skillPath)}`);
  }
  if (config.targets.some((target) => target.name === 'OpenCode')) {
    console.log(color.muted('Hint: OpenCode loads skills when a session starts. Restart OpenCode or open a new session if the skill is not visible yet.'));
  }
  if (config.outputModes.includes('gmail-mcp')) {
    console.log('Gmail delivery selected: use your Gmail MCP to send the saved markdown content.');
  }
  if (config.outputModes.includes('slack-webhook')) {
    console.log('Slack webhook delivery selected: use the saved config file to post the markdown content.');
  }
  console.log(color.bold(`Use the generated skill: /${config.skillName}`));
  console.log(`To refresh later: npm install -g ai-bricklaying@latest && ai-bricklaying`);
}

async function run(argv) {
  const args = parseArgs(argv);
  if (args.help) {
    printHelp();
    return 0;
  }
  if (args.version) {
    console.log(CLI_VERSION);
    return 0;
  }
  applyConfigDefaults(args, readConfigFile(args.configDir));

  let targets;
  let sources;
  let outputModes;
  let gmailRecipient = args.gmailRecipient || null;
  let gmailSubject = args.gmailSubject || null;
  let slackWebhookUrl = args.slackWebhookUrl || null;

  if (args.nonInteractive) {
    targets = targetsFromArgs(args);
    sources = sourcesFromArgs(args, targets);
    outputModes = outputModesFromArgs(args);
  } else {
    printIntro();
    const targetKeys = csv(args.targetAgent);
    const targetDefaultIndexes = targetKeys.length
      ? AGENT_TARGETS
        .map((target, index) => targetKeys.includes(slug(target.name)) ? index : null)
        .filter((index) => index !== null)
      : AGENT_TARGETS.map((_target, index) => index);
    targets = await chooseMany(
      '1. Select AI agent/model targets for generated skills',
      AGENT_TARGETS,
      (item) => `${item.name} - ${item.defaultSkillDir}`,
      'Skill targets',
      targetDefaultIndexes
    );
    targets = targets.map((target) => ({ ...target, modelHint: args.targetModel }));
    sources = [await chooseOne('2. Select one AI agent whose sessions should be summarized', sourceOptionsForTargets(targets), (item) => item.label)];
    args.language = await prompt('\n3. Result language', args.language);
    args.outputDir = await prompt('4. File save directory', args.outputDir);
    console.log(color.muted('\n5. Default summary template will be embedded in English and instructed to output your chosen language.'));
    outputModes = await chooseOutputModes(outputModesFromValue(args.outputModes));
    if (outputModes.includes('gmail-mcp')) {
      gmailRecipient = await prompt('Gmail MCP recipient (optional)', gmailRecipient || undefined) || null;
      gmailSubject = await prompt('Gmail MCP subject (optional)', gmailSubject || undefined) || null;
    }
    if (outputModes.includes('slack-webhook')) {
      slackWebhookUrl = await promptConfiguredSecret('Slack webhook URL (optional)', slackWebhookUrl);
    }
  }

  if (args.skillDir) {
    const skillDir = path.resolve(expandHome(args.skillDir));
    targets = targets.map((target) => ({ ...target, defaultSkillDir: skillDir }));
  }
  validateSkillName(args.skillName);

  const config = {
    configDir: path.resolve(expandHome(args.configDir)),
    gmailRecipient,
    gmailSubject,
    language: args.language,
    cliVersion: CLI_VERSION,
    outputDir: path.resolve(expandHome(args.outputDir)),
    outputModes,
    selectedSources: sources,
    skillName: args.skillName,
    slackWebhookUrl,
    target: targets[0],
    targets,
  };

  const records = [];
  for (const source of sources) {
    records.push(...collectTodaySessions(source));
  }

  const summary = buildSummary(config, records);
  const summaryPath = writeSummaryFile(config, summary);
  const slackPayloadPath = writeSlackPayloadFile(config, summary);
  const metadataPath = writeMetadataFile(config, summaryPath, records.length);
  const savedConfigPath = writeConfigFile(config);
  const skillPaths = writeSkills(config);
  printCompletion(config, { summaryPath, metadataPath, configPath: savedConfigPath, slackPayloadPath, skillPaths });
  return 0;
}

run(process.argv.slice(2)).then((code) => {
  process.exit(code);
}).catch((error) => {
  const message = error instanceof CliError ? error.message : `Unexpected error: ${error.message}`;
  console.error(message);
  process.exit(error instanceof CliError ? 2 : 1);
});
