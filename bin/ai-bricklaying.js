#!/usr/bin/env node

const fs = require('fs');
const path = require('path');
const { spawn } = require('child_process');

const SUPPORTED_TARGETS = [
  'darwin-arm64',
  'darwin-amd64',
  'linux-amd64',
  'linux-arm64',
];

const SIGNAL_EXIT_CODES = {
  SIGHUP: 129,
  SIGINT: 130,
  SIGTERM: 143,
};

function normalizeArch(arch) {
  return arch === 'x64' ? 'amd64' : arch;
}

function targetKey(platform = process.platform, arch = process.arch) {
  return `${platform}-${normalizeArch(arch)}`;
}

function unsupportedPlatformMessage(target) {
  return `ai-bricklaying: unsupported platform ${target}; supported binaries: ${SUPPORTED_TARGETS.join(', ')}`;
}

function missingBinaryMessage(target) {
  return `ai-bricklaying: bundled binary not found for ${target}; reinstall the package or report a release packaging issue`;
}

function resolveBinary(options = {}) {
  const platform = options.platform || process.platform;
  const arch = options.arch || process.arch;
  const target = targetKey(platform, arch);

  if (!SUPPORTED_TARGETS.includes(target)) {
    return { target, error: unsupportedPlatformMessage(target) };
  }

  const distDir = options.distDir || path.join(__dirname, '..', 'dist');
  return {
    target,
    binaryPath: path.join(distDir, `ai-bricklaying-${target}`),
  };
}

function writeDiagnostic(stderr, message) {
  stderr.write(`${message}\n`);
}

function wireSignalForwarding(child, processLike = process) {
  const signals = ['SIGINT', 'SIGTERM', 'SIGHUP'];
  const handlers = new Map();

  for (const signal of signals) {
    const handler = () => {
      if (!child.killed) child.kill(signal);
    };
    handlers.set(signal, handler);
    processLike.on(signal, handler);
  }

  return () => {
    for (const [signal, handler] of handlers) {
      processLike.removeListener(signal, handler);
    }
  };
}

function signalExitCode(signal) {
  return SIGNAL_EXIT_CODES[signal] || 1;
}

function run(argv = process.argv.slice(2), options = {}) {
  const processLike = options.process || process;
  const stderr = options.stderr || processLike.stderr;
  const resolved = resolveBinary(options);

  if (resolved.error) {
    writeDiagnostic(stderr, resolved.error);
    return Promise.resolve(1);
  }

  const exists = options.existsSync || fs.existsSync;
  if (!exists(resolved.binaryPath)) {
    writeDiagnostic(stderr, missingBinaryMessage(resolved.target));
    return Promise.resolve(1);
  }

  const spawnChild = options.spawn || spawn;
  const child = spawnChild(resolved.binaryPath, argv, {
    stdio: options.stdio || 'inherit',
  });
  if (options.onChild) options.onChild(child);
  const unwireSignals = wireSignalForwarding(child, processLike);

  return new Promise((resolve) => {
    child.on('error', (error) => {
      unwireSignals();
      writeDiagnostic(stderr, `ai-bricklaying: failed to launch bundled binary: ${error.message}`);
      resolve(1);
    });
    child.on('close', (code, signal) => {
      unwireSignals();
      resolve(signal ? signalExitCode(signal) : (code ?? 1));
    });
  });
}

if (require.main === module) {
  run().then((code) => {
    process.exit(code);
  });
}

module.exports = {
  SUPPORTED_TARGETS,
  missingBinaryMessage,
  normalizeArch,
  resolveBinary,
  run,
  signalExitCode,
  targetKey,
  unsupportedPlatformMessage,
  wireSignalForwarding,
};
