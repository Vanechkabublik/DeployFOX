import { spawnSync } from "node:child_process";
import { openSync, closeSync } from "node:fs";
import { resolve } from "node:path";

// Use real child exit codes and UTF-8 logs, including on Windows PowerShell 5.
const root = resolve(import.meta.dirname, "../..");
const kind = process.argv[2];
const commands = {
  local: [
    "docker",
    [
      "compose",
      "--progress",
      "plain",
      "-f",
      "docker-compose.local.yml",
      "up",
      "-d",
      "--build",
      "--wait",
    ],
  ],
  agent: [
    "docker",
    [
      "build",
      "--progress",
      "plain",
      "--target",
      "agent",
      "-t",
      "deployfox-agent:local",
      "backend",
    ],
  ],
  qa: [process.execPath, ["infrastructure/scripts/qa.mjs"]],
  backup: [process.execPath, ["infrastructure/scripts/test-backup.mjs"]],
  mysql: ["docker", ["pull", "mysql:8.4"]],
};
if (!commands[kind])
  throw new Error("Choose local, agent, qa, backup or mysql");
const logPath = resolve(root, `${kind}-check.log`);
const log = openSync(logPath, "w");
let result;
try {
  result = spawnSync(commands[kind][0], commands[kind][1], {
    cwd: root,
    env: process.env,
    stdio: ["ignore", log, log],
    windowsHide: true,
  });
} finally {
  closeSync(log);
}
if (result.error) throw result.error;
console.log(
  `${kind}: ${result.status === 0 ? "passed" : "failed"}; log: ${logPath}`,
);
process.exitCode = result.status ?? 1;
