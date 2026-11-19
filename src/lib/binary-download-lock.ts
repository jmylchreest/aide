import {
  mkdirSync,
  readFileSync,
  readdirSync,
  rmdirSync,
  unlinkSync,
  writeFileSync,
} from "fs";
import { randomUUID } from "crypto";
import { join } from "path";
import { setTimeout as delay } from "timers/promises";

/** Serialize hooks and MCP downloads without ever evicting a live installer. */
export async function acquireBinaryDownloadLock(
  destDir: string,
  // Allow an owner to finish both 60-second URL attempts and validation.
  timeoutMs = 150000,
): Promise<() => void> {
  const lockPath = join(destDir, ".aide-download.lock");
  const owner = `owner-${randomUUID()}`;
  const removeOwner = (directory: string, name: string) => {
    try {
      unlinkSync(join(directory, name));
    } catch {
      // A successor may be between mkdir and writing its marker. Only the
      // waiter that removed this owner's marker may remove the directory.
      return;
    }
    try {
      rmdirSync(directory);
    } catch {
      /* a new owner has acquired it */
    }
  };

  const deadline = Date.now() + timeoutMs;
  for (;;) {
    let acquired = false;
    try {
      mkdirSync(lockPath);
      acquired = true;
    } catch (err) {
      if ((err as NodeJS.ErrnoException).code !== "EEXIST") throw err;
    }
    if (acquired) {
      try {
        writeFileSync(join(lockPath, owner), String(process.pid));
      } catch (err) {
        try {
          unlinkSync(join(lockPath, owner));
        } catch {
          /* write may not have created it */
        }
        try {
          rmdirSync(lockPath);
        } catch {
          /* retain the original write error */
        }
        throw err;
      }
      return () => removeOwner(lockPath, owner);
    }

    try {
      const names = readdirSync(lockPath);
      // A legacy wrapper may still be between mkdir and its PID write.
      // An empty or malformed lock is not proof that its owner is dead.
      if (
        names.length === 1 &&
        (names[0].startsWith("owner-") || names[0] === "pid")
      ) {
        const pid = Number(readFileSync(join(lockPath, names[0]), "utf8"));
        if (Number.isSafeInteger(pid) && pid > 0) {
          try {
            process.kill(pid, 0);
          } catch (err) {
            if ((err as NodeJS.ErrnoException).code === "ESRCH") {
              // Each owner has a unique filename. Two stale-lock cleaners
              // cannot delete a successor's marker or nonempty directory.
              removeOwner(lockPath, names[0]);
            }
          }
        }
      }
    } catch {
      /* the owner may have released it while we inspected it */
    }

    if (Date.now() >= deadline) {
      throw new Error(
        `Timed out waiting for ${lockPath}; another installer may still be running`,
      );
    }
    await delay(100);
  }
}
