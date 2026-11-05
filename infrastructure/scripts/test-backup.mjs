import { execFileSync } from "node:child_process";
import { resolve } from "node:path";

const root = resolve(import.meta.dirname, "../..");
const shell =
  process.platform === "win32" ? "C:/Program Files/Git/bin/bash.exe" : "bash";
const env = {
  ...process.env,
  COMPOSE_PROJECT_NAME: "deployfox-qa",
  COMPOSE_FILE: "docker-compose.local.yml",
  COMPOSE_OVERRIDE_FILE: "docker-compose.qa.yml",
};
const compose = [
  "compose",
  "-p",
  "deployfox-qa",
  "-f",
  "docker-compose.local.yml",
  "-f",
  "docker-compose.qa.yml",
];
function sql(query) {
  return execFileSync(
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
      "-Atc",
      query,
    ],
    { cwd: root, encoding: "utf8" },
  ).trim();
}
const before = sql("SELECT count(*) FROM projects");
const output = execFileSync(shell, ["infrastructure/scripts/backup.sh"], {
  cwd: root,
  env,
  encoding: "utf8",
});
const match = output.match(/backups\/\d{8}T\d{6}Z/);
if (!match) throw new Error("Backup script did not return a backup directory");
sql(
  "INSERT INTO audit_events(action,target) VALUES ('qa.restore.marker','temporary-after-backup')",
);
execFileSync(
  shell,
  ["infrastructure/scripts/restore.sh", resolve(root, match[0])],
  { cwd: root, env, input: "RESTORE\n", stdio: ["pipe", "pipe", "pipe"] },
);
const after = sql("SELECT count(*) FROM projects");
const markers = sql(
  "SELECT count(*) FROM audit_events WHERE action='qa.restore.marker'",
);
if (before !== after || markers !== "0")
  throw new Error("Control-plane restore did not recover the expected data");
console.log(
  `Control-plane backup/restore verified: ${before} projects retained, post-backup marker removed.`,
);
