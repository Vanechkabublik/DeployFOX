import { execFileSync, spawnSync } from "node:child_process";
import { randomBytes } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";

const root = resolve(import.meta.dirname, "../..");
const compose = [
  "compose",
  "-p",
  "deployfox-qa",
  "-f",
  "docker-compose.local.yml",
  "-f",
  "docker-compose.qa.yml",
];
const saved = resolve(root, ".qa-credentials.json");
let credentials;
try {
  credentials = JSON.parse(readFileSync(saved, "utf8"));
} catch {
  credentials = {
    email: "qa@deployfox.test",
    password: randomBytes(24).toString("hex"),
  };
}
const status = await fetch("http://localhost:3001/api/v1/setup/status").then(
  (r) => r.json(),
);
if (!status.initialized) {
  const token = execFileSync(
    "docker",
    [
      ...compose,
      "exec",
      "-T",
      "api",
      "sh",
      "-c",
      "sed -n 's/^SETUP_TOKEN=//p' /run/deployfox/env",
    ],
    { cwd: root, encoding: "utf8" },
  ).trim();
  const result = await fetch("http://localhost:3001/api/v1/setup", {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-Setup-Token": token },
    body: JSON.stringify(credentials),
  });
  if (!result.ok) throw new Error(`QA initialization failed: ${result.status}`);
  writeFileSync(saved, JSON.stringify(credentials), { mode: 0o600 });
}
const rateKeys = execFileSync(
  "docker",
  [
    ...compose,
    "exec",
    "-T",
    "redis",
    "redis-cli",
    "--scan",
    "--pattern",
    "rate:*",
  ],
  { cwd: root, encoding: "utf8" },
)
  .trim()
  .split("\n")
  .map((key) => key.trim())
  .filter((key) => key.startsWith("rate:"));
if (rateKeys.length)
  execFileSync(
    "docker",
    [...compose, "exec", "-T", "redis", "redis-cli", "DEL", ...rateKeys],
    { cwd: root, stdio: "pipe" },
  );
execFileSync(
  "docker",
  [
    ...compose,
    "exec",
    "-T",
    "postgres",
    "psql",
    "-U",
    "deployfox",
    "-d",
    "deployfox",
    "-c",
    "INSERT INTO settings(key,value) VALUES('setup.completed','false') ON CONFLICT(key) DO UPDATE SET value='false'",
  ],
  { cwd: root, stdio: "pipe" },
);
const result = spawnSync(
  process.execPath,
  ["node_modules/@playwright/test/cli.js", "test"],
  {
    cwd: resolve(root, "frontend"),
    env: {
      ...process.env,
      E2E_URL: "http://localhost:3001",
      E2E_EMAIL: credentials.email,
      E2E_PASSWORD: credentials.password,
    },
    stdio: "inherit",
  },
);
process.exitCode = result.status ?? 1;
