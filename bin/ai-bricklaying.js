#!/usr/bin/env node

const { spawnSync } = require('child_process');
const path = require('path');

const packageRoot = path.resolve(__dirname, '..');
const existingPythonPath = process.env.PYTHONPATH;
const env = {
  ...process.env,
  PYTHONPATH: existingPythonPath ? `${packageRoot}${path.delimiter}${existingPythonPath}` : packageRoot,
};

function runPython(command) {
  return spawnSync(command, ['-m', 'ai_bricklaying.cli', ...process.argv.slice(2)], {
    stdio: 'inherit',
    env,
  });
}

let result = runPython(process.env.PYTHON || 'python3');

if (result.error && result.error.code === 'ENOENT' && !process.env.PYTHON) {
  result = runPython('python');
}

if (result.error) {
  console.error('Failed to run ai-bricklaying: Python was not found.');
  console.error('Install Python 3.10+ or set PYTHON=/path/to/python before running this npm package.');
  process.exit(1);
}

process.exit(result.status || 0);
