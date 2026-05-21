const assert = require('assert');
const { EventEmitter } = require('events');
const fs = require('fs');
const os = require('os');
const path = require('path');

const launcher = require('../bin/ai-bricklaying.js');

function tempRoot() {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'ai-bricklaying-launcher-'));
}

function captureStream() {
  let output = '';
  return {
    stream: { write(chunk) { output += chunk; } },
    output() { return output; },
  };
}

function currentTarget() {
  return launcher.targetKey(process.platform, process.arch);
}

function writeStubBinary(distDir, body) {
  fs.mkdirSync(distDir, { recursive: true });
  const binaryPath = path.join(distDir, `ai-bricklaying-${currentTarget()}`);
  fs.writeFileSync(binaryPath, `#!/usr/bin/env node\n${body}\n`, 'utf8');
  fs.chmodSync(binaryPath, 0o755);
  return binaryPath;
}

async function testSupportedPlatformDispatchesToStubBinary() {
  const root = tempRoot();
  const distDir = path.join(root, 'dist');
  const argsPath = path.join(root, 'args.json');
  writeStubBinary(distDir, `
const fs = require('fs');
fs.writeFileSync(${JSON.stringify(argsPath)}, JSON.stringify(process.argv.slice(2)));
process.stdout.write('stub stdout\\n');
process.stderr.write('stub stderr\\n');
process.exit(7);
`);

  let stdout = '';
  let stderr = '';
  const code = await launcher.run(['--probe', 'value'], {
    distDir,
    stdio: ['ignore', 'pipe', 'pipe'],
    onChild(child) {
      child.stdout.on('data', (chunk) => { stdout += chunk; });
      child.stderr.on('data', (chunk) => { stderr += chunk; });
    },
  });

  assert.strictEqual(code, 7);
  assert.deepStrictEqual(JSON.parse(fs.readFileSync(argsPath, 'utf8')), ['--probe', 'value']);
  assert.strictEqual(stdout, 'stub stdout\n');
  assert.strictEqual(stderr, 'stub stderr\n');
}

async function testMissingBinaryPrintsOnlyConfiguredDiagnostic() {
  const stderr = captureStream();
  const root = tempRoot();
  const distDir = path.join(root, 'missing-dist');
  const code = await launcher.run(['--version'], { distDir, stderr: stderr.stream });

  assert.strictEqual(code, 1);
  assert.strictEqual(stderr.output(), `${launcher.missingBinaryMessage(currentTarget())}\n`);
  assert.deepStrictEqual(fs.readdirSync(root), []);
}

async function testUnsupportedPlatformPrintsOnlyConfiguredDiagnostic() {
  const stderr = captureStream();
  const code = await launcher.run([], {
    platform: 'win32',
    arch: 'arm64',
    stderr: stderr.stream,
  });

  assert.strictEqual(code, 1);
  assert.strictEqual(
    stderr.output(),
    'ai-bricklaying: unsupported platform win32-arm64; supported binaries: darwin-arm64, darwin-amd64, linux-amd64, linux-arm64\n'
  );
}

function testPlatformArchMappingMatchesReleaseNames() {
  assert.strictEqual(launcher.targetKey('darwin', 'arm64'), 'darwin-arm64');
  assert.strictEqual(launcher.targetKey('darwin', 'x64'), 'darwin-amd64');
  assert.strictEqual(launcher.targetKey('linux', 'x64'), 'linux-amd64');
  assert.strictEqual(launcher.targetKey('linux', 'arm64'), 'linux-arm64');
}

function testSignalForwardingKillsChildWithReceivedSignal() {
  const child = new EventEmitter();
  const processLike = new EventEmitter();
  let forwardedSignal;
  child.kill = (signal) => { forwardedSignal = signal; };

  const unwire = launcher.wireSignalForwarding(child, processLike);
  processLike.emit('SIGINT');
  unwire();
  processLike.emit('SIGTERM');

  assert.strictEqual(forwardedSignal, 'SIGINT');
}

function testChildSignalExitCodeUsesInterruptConvention() {
  assert.strictEqual(launcher.signalExitCode('SIGINT'), 130);
  assert.strictEqual(launcher.signalExitCode('SIGTERM'), 143);
}

module.exports = async function runLauncherTests() {
  testPlatformArchMappingMatchesReleaseNames();
  await testSupportedPlatformDispatchesToStubBinary();
  await testMissingBinaryPrintsOnlyConfiguredDiagnostic();
  await testUnsupportedPlatformPrintsOnlyConfiguredDiagnostic();
  testSignalForwardingKillsChildWithReceivedSignal();
  testChildSignalExitCodeUsesInterruptConvention();
};
