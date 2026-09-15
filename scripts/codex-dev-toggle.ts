import { existsSync, realpathSync } from "fs";
import { homedir } from "os";
import { join, resolve } from "path";
import {
  codexAideHookRegistrations,
  codexDevMode,
  switchCodexDev,
} from "../src/cli/codex-dev.js";

const [action, repoArg] = process.argv.slice(2);
if (!repoArg || !["status", "mode", "dev", "prod"].includes(action)) {
  console.error(
    "Usage: bun scripts/codex-dev-toggle.ts <status|mode|dev|prod> <repo>",
  );
  process.exit(1);
}
const repo = resolve(repoArg);
const configIdentity = (dir: string) =>
  existsSync(dir) ? realpathSync(dir) : resolve(dir);
const seenConfigs = new Set<string>();
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
].filter((paths) => {
  const identity = configIdentity(paths.configDir);
  if (seenConfigs.has(identity)) return false;
  seenConfigs.add(identity);
  return true;
});

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
    // Hooks from global and project settings accumulate. When global aide is
    // installed, it owns development hooks; retain project MCP configuration.
    const inheritGlobalHooks = codexDevMode(scopes[0]) !== "not-installed";
    for (const [index, paths] of scopes.entries()) {
      const status =
        action === "status"
          ? codexDevMode(paths)
          : switchCodexDev(
              paths,
              action as "dev" | "prod",
              index === 1 && inheritGlobalHooks ? "inherit" : "local",
            );
      console.log(`    ${paths.label}: ${status} (${paths.configDir})`);
    }
    const globalHooks = new Set(codexAideHookRegistrations(scopes[0]));
    const duplicates = [
      ...new Set(scopes[1] ? codexAideHookRegistrations(scopes[1]) : []),
    ].filter((hook) => globalHooks.has(hook));
    if (duplicates.length > 0) {
      console.log(
        `    Warning: duplicate aide hooks in Global and Project (${duplicates.length} registrations); inherited hooks can run twice.`,
      );
    }
  }
} catch (error) {
  console.error(`Codex: ${error instanceof Error ? error.message : error}`);
  process.exit(1);
}
