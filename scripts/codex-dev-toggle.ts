import { homedir } from "os";
import { join, resolve } from "path";
import { codexDevMode, switchCodexDev } from "../src/cli/codex-dev.js";

const [action, repoArg] = process.argv.slice(2);
if (!repoArg || !["status", "mode", "dev", "prod"].includes(action)) {
  console.error(
    "Usage: bun scripts/codex-dev-toggle.ts <status|mode|dev|prod> <repo>",
  );
  process.exit(1);
}
const repo = resolve(repoArg);
const scopes = [
  {
    label: "Global",
    repo,
    configDir: process.env.CODEX_HOME || join(homedir(), ".codex"),
    skillsDir: join(homedir(), ".agents", "skills"),
  },
  {
    label: "Project",
    repo,
    configDir: join(repo, ".codex"),
    skillsDir: join(repo, ".agents", "skills"),
  },
];

try {
  if (action === "mode") {
    const modes = scopes
      .map(codexDevMode)
      .filter((mode) => mode !== "not-installed");
    console.log(
      modes.length === 0
        ? "not-installed"
        : new Set(modes).size === 1
          ? modes[0]
          : "mixed",
    );
  } else {
    for (const paths of scopes) {
      const status =
        action === "status"
          ? codexDevMode(paths)
          : switchCodexDev(paths, action as "dev" | "prod");
      console.log(`    ${paths.label}: ${status} (${paths.configDir})`);
    }
  }
} catch (error) {
  console.error(`Codex: ${error instanceof Error ? error.message : error}`);
  process.exit(1);
}
