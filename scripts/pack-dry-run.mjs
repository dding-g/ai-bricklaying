import { spawnSync } from "node:child_process";

const requiredFiles = [
  "README.ko.md",
  "README.md",
  "assets/ai-bricklaying.png",
  "bin/ai-bricklaying.js",
  "dist/ai-bricklaying-darwin-amd64",
  "dist/ai-bricklaying-darwin-arm64",
  "dist/ai-bricklaying-linux-amd64",
  "dist/ai-bricklaying-linux-arm64",
];

const allowedDistBinaries = new Set(requiredFiles.filter((file) => file.startsWith("dist/ai-bricklaying-")));

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    encoding: "utf8",
    ...options,
  });

  if (result.status !== 0) {
    if (result.stdout) process.stdout.write(result.stdout);
    if (result.stderr) process.stderr.write(result.stderr);
    process.exit(result.status ?? 1);
  }

  return result;
}

run(process.execPath, ["scripts/build-go.mjs"], {
  env: { ...process.env, AI_BRICKLAYING_BUILD_GO_QUIET: "1" },
  stdio: "inherit",
});

const pack = run("npm", ["pack", "--dry-run", "--json", "--ignore-scripts"], {
  stdio: ["ignore", "pipe", "pipe"],
});

if (pack.stderr) process.stderr.write(pack.stderr);

let payload;
try {
  payload = JSON.parse(pack.stdout);
} catch (error) {
  process.stderr.write("Failed to parse npm pack JSON: " + error.message + "\n");
  process.stderr.write(pack.stdout);
  process.exit(1);
}

const files = new Set(payload.flatMap((entry) => entry.files.map((file) => file.path)));
const missing = requiredFiles.filter((file) => !files.has(file));
const forbidden = [...files].filter((file) => (
  file.startsWith("ai_bricklaying/") ||
  file.startsWith("tests/test_") ||
  file === "pyproject.toml"
));
const unexpectedDist = [...files].filter((file) => file.startsWith("dist/ai-bricklaying-") && !allowedDistBinaries.has(file));

if (missing.length > 0 || forbidden.length > 0 || unexpectedDist.length > 0) {
  if (missing.length > 0) process.stderr.write("Missing required package files: " + missing.join(", ") + "\n");
  if (forbidden.length > 0) process.stderr.write("Forbidden legacy package files: " + forbidden.join(", ") + "\n");
  if (unexpectedDist.length > 0) process.stderr.write("Unexpected dist binaries: " + unexpectedDist.join(", ") + "\n");
  process.exit(1);
}

process.stdout.write(JSON.stringify(payload, null, 2) + "\n");
