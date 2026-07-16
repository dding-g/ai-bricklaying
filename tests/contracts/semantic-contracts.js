const assert = require('assert');
const fs = require('fs');
const path = require('path');

const contractsDir = __dirname;

function assertIncludesInOrder(haystack, needles, label) {
  let cursor = -1;
  for (const needle of needles) {
    const next = haystack.indexOf(needle, cursor + 1);
    assert.notStrictEqual(next, -1, `${label} should include ${needle}`);
    assert.ok(next > cursor, `${label} should keep ${needle} in order`);
    cursor = next;
  }
}

function textFromPayload(payload) {
  return payload.messages
    .flatMap((message) => message.blocks)
    .flatMap((block) => [block.text && block.text.text, ...((block.elements || []).map((element) => element.text || element.value))])
    .filter(Boolean)
    .join('\n');
}

function markdownTopLevelSections(markdown) {
  return String(markdown)
    .split('\n')
    .filter((line) => line.startsWith('## ') && !line.startsWith('### '))
    .map((line) => line.replace(/^##\s+/, '').trim())
    .filter(Boolean);
}

function assertSlackPayloadSemanticContract(payload, expectedSections) {
  assert.ok(payload.text.startsWith('AI Bricklaying Daily Summary'), 'fallback text should use markdown title');
  assert.strictEqual(payload.text.includes('##'), false, 'fallback text should not expose raw markdown heading markers');
  assert.ok(Array.isArray(payload.blocks), 'top-level blocks should be present for webhook callers');
  assert.ok(Array.isArray(payload.messages), 'messages array should be present for split delivery');
  assert.ok(payload.messages.length >= 1, 'messages array should include at least one Slack message');
  assert.deepStrictEqual(payload.blocks, payload.messages[0].blocks, 'top-level blocks should mirror first message');
  assert.strictEqual(payload.verification.source, 'saved_markdown');
  assert.deepStrictEqual(payload.verification.top_level_sections, expectedSections);
  assert.deepStrictEqual(payload.verification.covered_top_level_sections, expectedSections);
  assert.strictEqual(payload.verification.all_top_level_sections_covered, true);
  for (const message of payload.messages) {
    assert.ok(message.text.startsWith('AI Bricklaying Daily Summary'));
    assert.strictEqual(message.text.includes('##'), false);
    assert.ok(Array.isArray(message.blocks));
    assert.ok(message.blocks.length > 0);
  }
  const combinedText = textFromPayload(payload);
  assertIncludesInOrder(combinedText, expectedSections, 'Slack block text');
}

function testHelpAndVersionContract({ run, packageJson }) {
  const help = run(['--help']);
  assert.strictEqual(help.status, 0, help.stderr);
  assert.ok(help.stdout.includes('Usage:'));
  assert.ok(help.stdout.includes('--non-interactive'));
  assert.ok(help.stdout.includes('--target-agent <agents>'));
  assert.ok(help.stdout.includes('--sources, --sessions <source>'));
  assert.ok(help.stdout.includes('--output-modes, --delivery <list>'));
  assert.ok(help.stdout.includes('--slack-webhook-url <url>'));

  const version = run(['--version']);
  assert.strictEqual(version.status, 0, version.stderr);
  assert.strictEqual(version.stdout.trim(), packageJson.version);
}

function testNonInteractiveArtifactContract({ run, tempRoot, summaryPath, packageJson }) {
  const root = tempRoot();
  const sessionDir = path.join(root, 'empty-sessions');
  fs.mkdirSync(sessionDir);
  const outputDir = path.join(root, 'out');
  const skillDir = path.join(root, 'skills');
  const configDir = path.join(root, 'config');

  const result = run([
    '--non-interactive',
    '--target-agent', 'opencode',
    '--sources', 'opencode',
    '--language', 'Korean',
    '--output-modes', 'slack-webhook',
    '--slack-webhook-url', 'https://hooks.slack.com/services/T000/B000/contract-secret',
    '--skill-name', 'contract-session-summary',
    '--output-dir', outputDir,
    '--skill-dir', skillDir,
    '--config-dir', configDir,
  ], { env: { AI_BRICKLAYING_OPENCODE_DIRS: sessionDir, HOME: root } });

  assert.strictEqual(result.status, 0, result.stderr);
  assert.strictEqual(result.stdout.includes('contract-secret'), false, 'stdout should not print Slack webhook secret');

  const markdownPath = summaryPath(outputDir);
  const metadataPath = path.join(outputDir, 'ai-bricklaying-summary-skill.json');
  const slackPayloadPath = path.join(outputDir, 'ai-bricklaying-slack-payload.json');
  const configPath = path.join(configDir, 'config.json');
  const skillPath = path.join(skillDir, 'contract-session-summary', 'SKILL.md');
  for (const filePath of [markdownPath, metadataPath, slackPayloadPath, configPath, skillPath]) {
    assert.ok(fs.existsSync(filePath), `${filePath} should exist`);
  }

  const summary = fs.readFileSync(markdownPath, 'utf8');
  const metadata = JSON.parse(fs.readFileSync(metadataPath, 'utf8'));
  const savedConfig = JSON.parse(fs.readFileSync(configPath, 'utf8'));
  const slackPayload = JSON.parse(fs.readFileSync(slackPayloadPath, 'utf8'));

  assert.ok(summary.includes('No clear session signals were found today'));
  assert.ok(summary.includes('Write the summary in Korean'));
  assert.ok(summary.includes('File save: enabled'));
  assert.ok(summary.includes('Slack webhook URL: configured'));
  assert.strictEqual(summary.includes('contract-secret'), false);
  assert.strictEqual(metadata.cli_version, packageJson.version);
  assert.strictEqual(metadata.session_count, 0);
  assert.deepStrictEqual(metadata.sessions, ['opencode']);
  assert.deepStrictEqual(metadata.target_agents, ['OpenCode']);
  assert.deepStrictEqual(metadata.deliveries, ['file', 'slack-webhook']);
  assert.strictEqual(metadata.summary_path, markdownPath);
  assert.strictEqual(metadata.slack_payload_path, slackPayloadPath);
  assert.strictEqual(metadata.slack_webhook_configured, true);
  assert.deepStrictEqual(savedConfig.defaults.output_modes, ['file', 'slack-webhook']);
  assert.strictEqual(savedConfig.delivery.slack_webhook_url, 'https://hooks.slack.com/services/T000/B000/contract-secret');
  assert.strictEqual(slackPayload.verification.source, 'saved_markdown');
  assert.strictEqual(slackPayload.verification.all_top_level_sections_covered, true);
}

function testValidationContracts({ run, tempRoot, summaryPath }) {
  const missingSlackRoot = tempRoot();
  const missingSlack = run([
    '--non-interactive',
    '--output-modes', 'slack-webhook',
    '--output-dir', path.join(missingSlackRoot, 'out'),
    '--skill-dir', path.join(missingSlackRoot, 'skills'),
    '--config-dir', path.join(missingSlackRoot, 'config'),
  ]);
  assert.strictEqual(missingSlack.status, 2);
  assert.ok(missingSlack.stderr.includes('slack-webhook requires --slack-webhook-url'));

  const invalidSourceRoot = tempRoot();
  const invalidSource = run([
    '--non-interactive',
    '--target-agent', 'opencode',
    '--sources', 'codex',
    '--output-dir', path.join(invalidSourceRoot, 'out'),
    '--skill-dir', path.join(invalidSourceRoot, 'skills'),
    '--config-dir', path.join(invalidSourceRoot, 'config'),
  ]);
  assert.strictEqual(invalidSource.status, 2);
  assert.ok(invalidSource.stderr.includes('--sources must be one of the selected target agents: opencode'));

  const unsafeRoot = tempRoot();
  const unsafe = run([
    '--non-interactive',
    '--skill-name', '../escape',
    '--output-dir', path.join(unsafeRoot, 'out'),
    '--skill-dir', path.join(unsafeRoot, 'skills'),
    '--config-dir', path.join(unsafeRoot, 'config'),
  ]);
  assert.strictEqual(unsafe.status, 2);
  assert.ok(unsafe.stderr.includes('--skill-name must be 1-64 lowercase letters'));
  assert.strictEqual(fs.existsSync(path.join(unsafeRoot, 'escape')), false);

  if (process.platform !== 'win32') {
    const symlinkRoot = tempRoot();
    const outputDir = path.join(symlinkRoot, 'out');
    fs.mkdirSync(outputDir);
    fs.symlinkSync(path.join(symlinkRoot, 'target.md'), summaryPath(outputDir));
    const symlink = run([
      '--non-interactive',
      '--target-agent', 'opencode',
      '--sources', 'opencode',
      '--output-dir', outputDir,
      '--skill-dir', path.join(symlinkRoot, 'skills'),
      '--config-dir', path.join(symlinkRoot, 'config'),
    ]);
    assert.strictEqual(symlink.status, 2);
    assert.ok(symlink.stderr.includes('Refusing to overwrite symbolic link'));
  }
}

function testRedactionContract({ run, tempRoot, summaryPath }) {
  const root = tempRoot();
  const sessionDir = path.join(root, 'sessions');
  fs.mkdirSync(sessionDir);
  fs.writeFileSync(path.join(sessionDir, 'today.jsonl'), JSON.stringify({
    message: 'debugged release verification and improved Slack payload coverage',
    password: 'hunter2',
    api_key: 'stripe_test_key_fixture_123456789012345678901234',
    note: 'https://hooks.slack.com/services/T000/B000/session-secret Bearer abcdef123456',
  }) + '\n', 'utf8');
  const outputDir = path.join(root, 'out');
  const result = run([
    '--non-interactive',
    '--target-agent', 'opencode',
    '--sources', 'opencode',
    '--output-modes', 'slack-webhook',
    '--slack-webhook-url', 'https://hooks.slack.com/services/T000/B000/config-secret',
    '--output-dir', outputDir,
    '--skill-dir', path.join(root, 'skills'),
    '--config-dir', path.join(root, 'config'),
  ], { env: { AI_BRICKLAYING_OPENCODE_DIRS: sessionDir, HOME: root } });

  assert.strictEqual(result.status, 0, result.stderr);
  const summary = fs.readFileSync(summaryPath(outputDir), 'utf8');
  const slackPayload = fs.readFileSync(path.join(outputDir, 'ai-bricklaying-slack-payload.json'), 'utf8');
  for (const forbidden of ['hunter2', 'stripe_test_key_fixture_123456789012345678901234', 'session-secret', 'abcdef123456', 'config-secret']) {
    assert.strictEqual(summary.includes(forbidden), false, `summary should redact ${forbidden}`);
    assert.strictEqual(slackPayload.includes(forbidden), false, `Slack payload should redact ${forbidden}`);
  }
  assert.ok(summary.includes('Likely themes to reflect on: debugging'));
}

function testSlackPayloadGeneratedFromGoCliContract({ run, tempRoot, summaryPath }) {
  const root = tempRoot();
  const sessionDir = path.join(root, 'sessions');
  fs.mkdirSync(sessionDir);
  fs.writeFileSync(
    path.join(sessionDir, 'today.md'),
    'implemented release verification with https://hooks.slack.com/services/T000/B000/fixture-secret and Bearer fixturetoken123456',
    'utf8'
  );
  const outputDir = path.join(root, 'out');
  const result = run([
    '--non-interactive',
    '--target-agent', 'opencode',
    '--sources', 'opencode',
    '--output-modes', 'slack-webhook',
    '--slack-webhook-url', 'https://hooks.slack.com/services/T000/B000/config-secret',
    '--output-dir', outputDir,
    '--skill-dir', path.join(root, 'skills'),
    '--config-dir', path.join(root, 'config'),
  ], { env: { AI_BRICKLAYING_OPENCODE_DIRS: sessionDir, HOME: root } });

  assert.strictEqual(result.status, 0, result.stderr);
  const markdown = fs.readFileSync(summaryPath(outputDir), 'utf8');
  const expectedSections = markdownTopLevelSections(markdown);
  const payload = JSON.parse(fs.readFileSync(path.join(outputDir, 'ai-bricklaying-slack-payload.json'), 'utf8'));

  assertSlackPayloadSemanticContract(payload, expectedSections);
  const payloadJSON = JSON.stringify(payload);
  for (const forbidden of ['fixture-secret', 'fixturetoken123456', 'config-secret']) {
    assert.strictEqual(markdown.includes(forbidden), false, `summary should redact ${forbidden}`);
    assert.strictEqual(payloadJSON.includes(forbidden), false, `Slack payload should redact ${forbidden}`);
  }
}

function testPythonSurfaceMappingDocument() {
  const mapping = fs.readFileSync(path.join(contractsDir, 'python-surface-mapping.md'), 'utf8');
  for (const expected of [
    'Removed Python Surface Coverage Mapping',
    'tests/test_cli.py',
    'tests/test_session_sources.py',
    'tests/test_summary.py',
    'tests/contracts/semantic-contracts.js',
    'removed legacy Python package',
    'public contract is the npm launcher plus bundled Go CLI behavior',
  ]) {
    assert.ok(mapping.includes(expected), `mapping should include ${expected}`);
  }
}

module.exports = function runSemanticContracts(context) {
  testHelpAndVersionContract(context);
  testNonInteractiveArtifactContract(context);
  testValidationContracts(context);
  testRedactionContract(context);
  testSlackPayloadGeneratedFromGoCliContract(context);
  testPythonSurfaceMappingDocument();
};
