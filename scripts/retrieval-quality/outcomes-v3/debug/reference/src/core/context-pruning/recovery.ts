import { createHash } from "crypto";
import { mkdirSync, readFileSync, writeFileSync } from "fs";
import { join } from "path";
import type { PruneResult } from "./types.js";

/** Retain before shortening; failure leaves the original result intact. Files
 * are immutable and project-local, with no shell re-execution for recovery. */
export function recoverablePrune(
  cwd: string,
  scope: string,
  call: string,
  original: string,
) {
  return (candidate: PruneResult): PruneResult => {
    if (!candidate.modified || candidate.strategy === "supersede")
      return candidate;
    const unchanged = { output: original, modified: false, bytesSaved: 0 };
    if (!scope || !call) return unchanged;
    try {
      const dir = join(cwd, ".aide", "artifacts", "tool-output");
      const id = createHash("sha256")
        .update(JSON.stringify([scope, call, original]))
        .digest("hex");
      const path = join(dir, `${id}.txt`);
      const output = `${candidate.output}\n[aide:original] Read retained output at ${path}`;
      if (Buffer.byteLength(output) >= Buffer.byteLength(original))
        return unchanged;
      mkdirSync(dir, { recursive: true, mode: 0o700 });
      try {
        writeFileSync(path, original, {
          encoding: "utf8",
          mode: 0o600,
          flag: "wx",
        });
      } catch (err) {
        if (
          (err as NodeJS.ErrnoException).code !== "EEXIST" ||
          readFileSync(path, "utf8") !== original
        )
          return unchanged;
      }
      return {
        ...candidate,
        output,
        recoveryPath: path,
        bytesSaved: Buffer.byteLength(original) - Buffer.byteLength(output),
      };
    } catch {
      return unchanged;
    }
  };
}
