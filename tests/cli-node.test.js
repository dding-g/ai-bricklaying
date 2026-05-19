const assert = require('assert');
const fs = require('fs');
const os = require('os');
const path = require('path');
const { spawnSync } = require('child_process');
const packageJson = require('../package.json');

const cli = path.join(__dirname, '..', 'bin', 'ai-bricklaying.js');

function tempRoot() {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'ai-bricklaying-node-'));
}

function localDateKey(date = new Date()) {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

function summaryPath(outputDir) {
  return path.join(outputDir, `${localDateKey()}-ai-bricklaying-daily-summary.md`);
}

function run(args, options = {}) {
  return spawnSync(process.execPath, [cli, ...args], {
    encoding: 'utf8',
    env: { ...process.env, NO_COLOR: '1', ...options.env },
    input: options.input,
  });
}

(function testVersionFlagPrintsPackageVersion() {
  const result = run(['--version']);

  assert.strictEqual(result.status, 0, result.stderr);
  assert.strictEqual(result.stdout.trim(), packageJson.version);
})();

(function testNonInteractiveFlagsWriteOnlyToRequestedDirs() {
  const root = tempRoot();
  const outputDir = path.join(root, 'out');
  const skillDir = path.join(root, 'skills');
  const configDir = path.join(root, 'config');
  const skillName = 'test-ai-session-summary';

  const result = run([
    '--non-interactive',
    '--target-agent', 'opencode',
    '--sources', 'opencode',
    '--language', 'Korean',
    '--output-modes', 'gmail-mcp,slack-webhook',
    '--gmail-recipient', 'team@example.com',
    '--gmail-subject', 'AI session summary',
    '--slack-webhook-url', 'https://hooks.slack.com/services/T000/B000/secret',
    '--skill-name', skillName,
    '--output-dir', outputDir,
    '--skill-dir', skillDir,
    '--config-dir', configDir,
  ]);

  assert.strictEqual(result.status, 0, result.stderr);
  const markdownPath = summaryPath(outputDir);
  const metadataPath = path.join(outputDir, 'ai-bricklaying-summary-skill.json');
  const slackPayloadPath = path.join(outputDir, 'ai-bricklaying-slack-payload.json');
  const skillPath = path.join(skillDir, skillName, 'SKILL.md');
  const configPath = path.join(configDir, 'config.json');

  for (const filePath of [markdownPath, metadataPath, slackPayloadPath, skillPath, configPath]) {
    assert.ok(fs.existsSync(filePath), `${filePath} should exist`);
  }

  const summary = fs.readFileSync(markdownPath, 'utf8');
  assert.ok(summary.includes('Gmail MCP: prepare an email draft for team@example.com with subject AI session summary'));
  assert.ok(summary.includes('Slack webhook URL: configured'));
  assert.ok(summary.includes('Lightweight Session Signals'));
  assert.ok(summary.includes("Today's Takeaways"));
  assert.ok(summary.includes('Lessons Learned'));
  assert.ok(summary.includes('What Improved'));
  assert.ok(summary.includes('Better AI Usage Next Time'));
  assert.ok(summary.includes("Tomorrow's Best Next Step"));
  assert.strictEqual(summary.includes('/Users/'), false);
  assert.ok(fs.readFileSync(configPath, 'utf8').includes('https://hooks.slack.com/services/T000/B000/secret'));
  const slackPayload = JSON.parse(fs.readFileSync(slackPayloadPath, 'utf8'));
  const metadata = JSON.parse(fs.readFileSync(metadataPath, 'utf8'));
  const slackJson = JSON.stringify(slackPayload);
  assert.strictEqual(metadata.cli_version, packageJson.version);
  assert.ok(slackPayload.text.startsWith('AI Bricklaying Daily Summary'));
  assert.strictEqual(slackPayload.text.includes('##'), false);
  assert.ok(Array.isArray(slackPayload.messages));
  assert.ok(slackPayload.messages.length >= 1);
  assert.strictEqual(slackPayload.messages[0].text.includes('##'), false);
  assert.ok(slackPayload.blocks.some((block) => block.type === 'header'));
  assert.ok(slackJson.includes("Today&apos;s Takeaways") || slackJson.includes("Today's Takeaways"));
  assert.ok(slackJson.includes('Better AI Usage Next Time'));
  assert.ok(slackJson.includes('Summary Template For AI Agent'));
  assert.ok(slackJson.includes('Gmail MCP'));
  assert.ok(slackJson.includes('team@example.com'));
  assert.ok(slackJson.includes('AI session summary'));
  assert.ok(slackJson.includes('test-ai-session-summary'));
  const skill = fs.readFileSync(skillPath, 'utf8');
  assert.ok(skill.includes('## Output Locations'));
  assert.ok(skill.includes(`Summary directory: \`${outputDir}\``));
  assert.ok(skill.includes(`Metadata file: \`${metadataPath}\``));
  assert.ok(skill.includes(`Config file: \`${configPath}\``));
  assert.ok(skill.includes(`Slack payload file: \`${slackPayloadPath}\``));
  assert.ok(skill.includes('Do not substitute the agent automation workspace'));
  assert.ok(skill.includes('This skill was generated from the CLI result with delivery modes: file, gmail-mcp, slack-webhook.'));
  assert.ok(skill.includes(`\`file\`: always save the final markdown summary under \`${outputDir}\``));
  assert.ok(skill.includes('`gmail-mcp`: when the CLI result includes this mode'));
  assert.ok(skill.includes('post each entry in `messages`'));
  assert.ok(skill.includes('Send the JSON blocks, not the raw Markdown text'));
  assert.ok(result.stdout.includes('Restart OpenCode or open a new session'));
  assert.ok(result.stdout.includes('Use the generated skill: /test-ai-session-summary'));
  assert.ok(result.stdout.includes('npm install -g ai-bricklaying@latest && ai-bricklaying'));
  assert.strictEqual(result.stdout.includes('\u001b['), false, 'NO_COLOR should suppress ANSI output');
})();

(function testExistingConfigProvidesNonInteractiveDefaults() {
  const root = tempRoot();
  const outputDir = path.join(root, 'configured-out');
  const skillDir = path.join(root, 'configured-skills');
  const configDir = path.join(root, 'config');
  fs.mkdirSync(configDir);
  fs.writeFileSync(path.join(configDir, 'config.json'), JSON.stringify({
    delivery: {
      gmail_recipient: 'saved@example.com',
      gmail_subject: 'Saved subject',
      slack_webhook_url: 'https://hooks.slack.com/services/T000/B000/saved',
    },
    defaults: {
      target_agents: ['opencode'],
      source: 'opencode',
      language: 'Korean',
      output_modes: ['file', 'gmail-mcp', 'slack-webhook'],
      skill_name: 'saved-session-summary',
      skill_dir: skillDir,
      output_dir: outputDir,
    },
  }, null, 2));

  const result = run(['--non-interactive', '--config-dir', configDir]);

  assert.strictEqual(result.status, 0, result.stderr);
  assert.ok(fs.existsSync(summaryPath(outputDir)));
  assert.ok(fs.existsSync(path.join(outputDir, 'ai-bricklaying-slack-payload.json')));
  assert.ok(fs.existsSync(path.join(skillDir, 'saved-session-summary', 'SKILL.md')));
  const summary = fs.readFileSync(summaryPath(outputDir), 'utf8');
  const savedConfig = JSON.parse(fs.readFileSync(path.join(configDir, 'config.json'), 'utf8'));
  assert.strictEqual(savedConfig.defaults.cli_version, packageJson.version);
  assert.ok(summary.includes('Language: Korean'));
  assert.ok(summary.includes('Gmail MCP: prepare an email draft for saved@example.com with subject Saved subject'));
  assert.ok(summary.includes('Slack webhook URL: configured'));
})();

(function testInteractiveConfigDefaultsDoNotPrintSlackSecret() {
  const root = tempRoot();
  const outputDir = path.join(root, 'configured-out');
  const skillDir = path.join(root, 'configured-skills');
  const configDir = path.join(root, 'config');
  const webhook = 'https://hooks.slack.com/services/T000/B000/saved';
  fs.mkdirSync(configDir);
  fs.writeFileSync(path.join(configDir, 'config.json'), JSON.stringify({
    delivery: { slack_webhook_url: webhook },
    defaults: {
      target_agents: ['opencode'],
      source: 'opencode',
      output_modes: ['file', 'slack-webhook'],
      skill_dir: skillDir,
      output_dir: outputDir,
    },
  }, null, 2));

  const result = run(['--config-dir', configDir], { input: '\n\n\n\n\n\n' });

  assert.strictEqual(result.status, 0, result.stderr);
  assert.ok(result.stdout.includes('Slack webhook URL (optional) [configured]'));
  assert.strictEqual(result.stdout.includes(webhook), false);
  assert.ok(fs.existsSync(path.join(outputDir, 'ai-bricklaying-slack-payload.json')));
})();

(function testMultipleSkillTargetsWithSingleSummarySource() {
  const root = tempRoot();
  const outputDir = path.join(root, 'out');
  const result = run([
    '--non-interactive',
    '--target-agent', 'opencode,codex',
    '--sources', 'opencode',
    '--output-dir', outputDir,
    '--config-dir', path.join(root, 'config'),
  ], { env: { HOME: root } });

  assert.strictEqual(result.status, 0, result.stderr);
  assert.ok(fs.existsSync(path.join(root, '.config', 'opencode', 'skills', 'daily-ai-session-summary', 'SKILL.md')));
  assert.ok(fs.existsSync(path.join(root, '.codex', 'skills', 'daily-ai-session-summary', 'SKILL.md')));
  const metadata = JSON.parse(fs.readFileSync(path.join(outputDir, 'ai-bricklaying-summary-skill.json'), 'utf8'));
  assert.deepStrictEqual(metadata.target_agents, ['OpenCode', 'Codex']);
  assert.deepStrictEqual(metadata.sessions, ['opencode']);
})();

(function testGeneratedSkillKeepsFileOnlyModeLocal() {
  const root = tempRoot();
  const skillName = 'file-only-session-summary';
  const result = run([
    '--non-interactive',
    '--target-agent', 'opencode',
    '--sources', 'opencode',
    '--output-modes', 'file',
    '--skill-name', skillName,
    '--output-dir', path.join(root, 'out'),
    '--skill-dir', path.join(root, 'skills'),
    '--config-dir', path.join(root, 'config'),
  ]);

  assert.strictEqual(result.status, 0, result.stderr);
  const skill = fs.readFileSync(path.join(root, 'skills', skillName, 'SKILL.md'), 'utf8');
  assert.ok(skill.includes('This skill was generated from the CLI result with delivery modes: file.'));
  assert.ok(skill.includes(`Summary directory: \`${path.join(root, 'out')}\``));
  assert.ok(skill.includes('Slack payload file: not generated unless `slack-webhook` is selected'));
  assert.ok(skill.includes('File-only mode: do not attempt Gmail, Slack, or any external delivery'));
  assert.strictEqual(skill.includes('`gmail-mcp`: when the CLI result includes this mode'), false);
  assert.strictEqual(skill.includes('`slack-webhook`: when the CLI result includes this mode'), false);
})();

(function testRejectsMultipleSummarySources() {
  const root = tempRoot();
  const result = run([
    '--non-interactive',
    '--sources', 'opencode,codex',
    '--output-dir', path.join(root, 'out'),
    '--skill-dir', path.join(root, 'skills'),
    '--config-dir', path.join(root, 'config'),
  ]);

  assert.strictEqual(result.status, 2);
  assert.ok(result.stderr.includes('--sources accepts exactly one summary source'));
})();

(function testRejectsSummarySourceOutsideSelectedTargets() {
  const root = tempRoot();
  const result = run([
    '--non-interactive',
    '--target-agent', 'opencode',
    '--sources', 'codex',
    '--output-dir', path.join(root, 'out'),
    '--skill-dir', path.join(root, 'skills'),
    '--config-dir', path.join(root, 'config'),
  ]);

  assert.strictEqual(result.status, 2);
  assert.ok(result.stderr.includes('--sources must be one of the selected target agents: opencode'));
})();

(function testInteractiveLineModeShowsCheckboxWizardFallback() {
  const root = tempRoot();
  const result = run([
    '--output-dir', path.join(root, 'out'),
    '--skill-dir', path.join(root, 'skills'),
    '--config-dir', path.join(root, 'config'),
  ], { input: '\n1\n\n\n1\n' });

  assert.strictEqual(result.status, 0, result.stderr);
  assert.ok(result.stdout.includes('[x] OpenCode'));
  assert.ok(result.stdout.includes('[ ] Claude Code'));
  assert.ok(result.stdout.includes('[x] File save (always enabled)'));
  assert.ok(result.stdout.includes('AI Bricklaying files generated'));
})();

(function testInteractiveSourceChoicesAreLimitedToSelectedTargets() {
  const root = tempRoot();
  const result = run([
    '--output-dir', path.join(root, 'out'),
    '--skill-dir', path.join(root, 'skills'),
    '--config-dir', path.join(root, 'config'),
  ], { input: '3\n1\n\n\n1\n' });

  assert.strictEqual(result.status, 0, result.stderr);
  const sourceSection = result.stdout.slice(
    result.stdout.indexOf('2. Select one AI agent whose sessions should be summarized'),
    result.stdout.indexOf('3. Result language')
  );
  assert.ok(sourceSection.includes('Codex'));
  assert.strictEqual(sourceSection.includes('OpenCode'), false);
  assert.strictEqual(sourceSection.includes('Claude Code'), false);
})();

(function testMissingSlackWebhookFails() {
  const root = tempRoot();
  const result = run([
    '--non-interactive',
    '--output-modes', 'slack-webhook',
    '--output-dir', path.join(root, 'out'),
    '--skill-dir', path.join(root, 'skills'),
    '--config-dir', path.join(root, 'config'),
  ]);

  assert.strictEqual(result.status, 2);
  assert.ok(result.stderr.includes('slack-webhook requires --slack-webhook-url'));
})();

(function testSkillNameMustBePathSafe() {
  for (const skillName of ['../escape', '/tmp/escape', 'nested/skill', 'NestedSkill']) {
    const root = tempRoot();
    const result = run([
      '--non-interactive',
      '--skill-name', skillName,
      '--output-dir', path.join(root, 'out'),
      '--skill-dir', path.join(root, 'skills'),
      '--config-dir', path.join(root, 'config'),
    ]);

    assert.strictEqual(result.status, 2, `${skillName} should fail`);
    assert.ok(result.stderr.includes('--skill-name must be a path-safe slug'));
    assert.strictEqual(fs.existsSync(path.join(root, 'escape')), false);
  }
})();

(function testConfigFileUsesPrivatePermissions() {
  if (process.platform === 'win32') return;
  const root = tempRoot();
  const result = run([
    '--non-interactive',
    '--target-agent', 'opencode',
    '--sources', 'opencode',
    '--output-modes', 'slack-webhook',
    '--slack-webhook-url', 'https://hooks.slack.com/services/T000/B000/secret',
    '--output-dir', path.join(root, 'out'),
    '--skill-dir', path.join(root, 'skills'),
    '--config-dir', path.join(root, 'config'),
  ]);

  assert.strictEqual(result.status, 0, result.stderr);
  assert.strictEqual(fs.statSync(path.join(root, 'config')).mode & 0o777, 0o700);
  assert.strictEqual(fs.statSync(path.join(root, 'config', 'config.json')).mode & 0o777, 0o600);
})();

(function testRefusesSymlinkOutputClobber() {
  if (process.platform === 'win32') return;
  const root = tempRoot();
  const outputDir = path.join(root, 'out');
  fs.mkdirSync(outputDir);
  fs.symlinkSync(path.join(root, 'target.md'), summaryPath(outputDir));

  const result = run([
    '--non-interactive',
    '--target-agent', 'opencode',
    '--sources', 'opencode',
    '--output-dir', outputDir,
    '--skill-dir', path.join(root, 'skills'),
    '--config-dir', path.join(root, 'config'),
  ]);

  assert.strictEqual(result.status, 2);
  assert.ok(result.stderr.includes('Refusing to overwrite symbolic link'));
})();

(function testRedactsSecretsFromSessionSnippets() {
  const root = tempRoot();
  const sessionDir = path.join(root, 'sessions');
  fs.mkdirSync(sessionDir);
  fs.writeFileSync(path.join(sessionDir, 'today.log'), 'fix deployed with https://hooks.slack.com/services/T000/B000/secret and Bearer abcdef123456', 'utf8');
  const outputDir = path.join(root, 'out');

  const result = run([
    '--non-interactive',
    '--target-agent', 'opencode',
    '--sources', 'opencode',
    '--output-dir', outputDir,
    '--skill-dir', path.join(root, 'skills'),
    '--config-dir', path.join(root, 'config'),
  ], { env: { AI_BRICKLAYING_OPENCODE_DIRS: sessionDir } });

  assert.strictEqual(result.status, 0, result.stderr);
  const summary = fs.readFileSync(summaryPath(outputDir), 'utf8');
  assert.ok(summary.includes('Likely themes to reflect on: debugging'));
  assert.strictEqual(summary.includes('T000/B000/secret'), false);
  assert.strictEqual(summary.includes('abcdef123456'), false);
  assert.strictEqual(summary.includes('hooks.slack.com'), false);
  assert.strictEqual(summary.includes('/Users/'), false);
})();

(function testRedactsStructuredJsonSecrets() {
  const root = tempRoot();
  const sessionDir = path.join(root, 'sessions');
  fs.mkdirSync(sessionDir);
  fs.writeFileSync(path.join(sessionDir, 'today.json'), JSON.stringify({
    message: 'implemented useful summary workflow',
    password: 'hunter2',
    api_key: 'abc123',
  }), 'utf8');
  const outputDir = path.join(root, 'out');

  const result = run([
    '--non-interactive',
    '--target-agent', 'opencode',
    '--sources', 'opencode',
    '--output-dir', outputDir,
    '--skill-dir', path.join(root, 'skills'),
    '--config-dir', path.join(root, 'config'),
  ], { env: { AI_BRICKLAYING_OPENCODE_DIRS: sessionDir } });

  assert.strictEqual(result.status, 0, result.stderr);
  const summary = fs.readFileSync(summaryPath(outputDir), 'utf8');
  assert.ok(summary.includes('Likely themes to reflect on: implementation'));
  assert.strictEqual(summary.includes('hunter2'), false);
  assert.strictEqual(summary.includes('abc123'), false);
})();

console.log('Node CLI tests passed');
