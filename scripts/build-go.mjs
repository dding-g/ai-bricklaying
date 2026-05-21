import { mkdirSync, readFileSync, readdirSync, rmSync, statSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { join } from "node:path";

const targets = [
  ["darwin", "arm64"],
  ["darwin", "amd64"],
  ["linux", "amd64"],
  ["linux", "arm64"],
];

const distDir = "dist";
const packageJson = JSON.parse(readFileSync("package.json", "utf8"));
const packageVersion = packageJson.version;

if (typeof packageVersion !== "string" || packageVersion.length === 0) {
  console.error("package.json version is required to build Go binaries");
  process.exit(1);
}

mkdirSync(distDir, { recursive: true });

for (const name of readdirSync(distDir)) {
  if (name.startsWith("ai-bricklaying-")) {
    rmSync(join(distDir, name));
  }
}

for (const [goos, goarch] of targets) {
  const output = join(distDir, `ai-bricklaying-${goos}-${goarch}`);
  const result = spawnSync("go", [
    "build",
    "-trimpath",
    "-ldflags",
    `-X ai-bricklaying/internal/cli.cliVersion=${packageVersion}`,
    "-o",
    output,
    "./cmd/ai-bricklaying",
  ], {
    env: { ...process.env, CGO_ENABLED: "0", GOOS: goos, GOARCH: goarch },
    stdio: "inherit",
  });

  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
}

const actual = readdirSync(distDir)
  .filter((name) => name.startsWith("ai-bricklaying-"))
  .sort();
const expected = targets.map(([goos, goarch]) => `ai-bricklaying-${goos}-${goarch}`).sort();

if (JSON.stringify(actual) !== JSON.stringify(expected)) {
  console.error(`unexpected Go build outputs: ${actual.join(", ")}`);
  console.error(`expected: ${expected.join(", ")}`);
  process.exit(1);
}

for (const name of actual) {
  const mode = statSync(join(distDir, name)).mode;
  if ((mode & 0o111) === 0) {
    console.error(`build output is not executable: ${join(distDir, name)}`);
    process.exit(1);
  }
}

if (process.env.AI_BRICKLAYING_BUILD_GO_QUIET !== "1") {
  console.log(actual.map((name) => join(distDir, name)).join("\n"));
}
