/** Evidence at the host hook boundary. Never a claim about provider tokens or
 * about calls that were not observed. Unsupported shell syntax stays unknown. */
import { createHash } from "crypto";
import {
  closeSync,
  fstatSync,
  openSync,
  readSync,
  realpathSync,
  statSync,
} from "fs";
import { isAbsolute, relative, resolve, sep } from "path";

const hash = (text: string | Buffer) =>
  createHash("sha256").update(text).digest("hex");
const object = (value: unknown): Record<string, unknown> | undefined =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;

export function aideRetrievalTool(name: string): string | undefined {
  return /^(?:mcp__(?:aide|plugin_aide_aide)__|aide_)(code_outline|code_read_symbol)$/.exec(
    name,
  )?.[1];
}

/** Deliberately a recogniser, not a shell interpreter. No expansion, redirects,
 * pipelines, options that transform cat output, or multiple file operands. */
function shellTarget(command: unknown):
  | {
      file?: string;
      method: string;
      start?: number;
      end?: number;
      search?: boolean;
    }
  | undefined {
  if (
    typeof command !== "string" ||
    command.length > 8192 ||
    /[\r\n$`|;&<>*?{}()[\]~#\\]/.test(command)
  )
    return;
  const words: string[] = [];
  const tokens = /\s*(?:'([^']*)'|"([^"]*)"|([^\s'"]+))/gy;
  let at = 0;
  while (at < command.length) {
    if (!command.slice(at).trim()) break;
    tokens.lastIndex = at;
    const match = tokens.exec(command);
    if (!match) return;
    words.push(match[1] ?? match[2] ?? match[3]);
    at = tokens.lastIndex;
    if (at < command.length && !/\s/.test(command[at])) return;
  }
  if (words[0] === "rtk") {
    words.splice(0, words[1] === "proxy" ? 2 : 1);
  }
  const program = words.shift();
  if (program === "cat") {
    if (words[0] === "--") words.shift();
    if (words.length === 1 && words[0] && !words[0].startsWith("-"))
      return { file: words[0], method: "shell_cat" };
  }
  if (program === "sed" && words.length === 3 && words[0] === "-n") {
    const range = /^(\d+)(?:,(\d+))?p$/.exec(words[1]);
    if (range && !words[2].startsWith("-")) {
      const start = Number(range[1]),
        end = Number(range[2] ?? range[1]);
      if (
        Number.isSafeInteger(start) &&
        Number.isSafeInteger(end) &&
        start > 0 &&
        end >= start
      )
        return { file: words[2], method: "shell_sed", start, end };
    }
  }
  if (program === "rg" || program === "grep") {
    // A search was requested even when its file scope cannot be established.
    // Only this small option subset permits an explicit single-file target.
    while (words.length && /^-(?:[niIFw]+)$/.test(words[0])) words.shift();
    if (words[0] === "--") words.shift();
    const file =
      words.length === 2 && words.every((word) => word && !word.startsWith("-"))
        ? words[1]
        : undefined;
    return { method: "shell_search", search: true, file };
  }
}

/** Read only bounded, regular project files, including after symlink resolution.
 * This is observation-only: never execute a recognised command. */
function snapshot(
  cwd: string,
  file: string,
): { file: string; bytes: Buffer } | undefined {
  let fd: number | undefined;
  try {
    const root = realpathSync(cwd);
    const path = realpathSync(resolve(cwd, file));
    const rel = relative(root, path);
    if (!rel || rel === ".." || rel.startsWith(`..${sep}`) || isAbsolute(rel))
      return;
    if (!statSync(path).isFile()) return;
    fd = openSync(path, "r");
    const stat = fstatSync(fd);
    if (!stat.isFile() || stat.size > 8 * 1024 * 1024) return;
    // Bound allocation/read even if the file grows after the stat.
    const buffer = Buffer.alloc(stat.size + 1);
    let size = 0;
    while (size < buffer.length) {
      const n = readSync(fd, buffer, size, buffer.length - size, null);
      if (!n) break;
      size += n;
    }
    if (size !== stat.size) return;
    const bytes = buffer.subarray(0, size);
    return { file: rel, bytes };
  } catch {
    return;
  } finally {
    if (fd !== undefined) closeSync(fd);
  }
}

function validShellWorkdir(cwd: string, workdir: unknown): workdir is string {
  if (typeof workdir !== "string" || !workdir.trim() || workdir.includes("\0"))
    return false;
  try {
    // Check the supplied path before normalising away components such as a
    // nonexistent directory followed by "..".
    return statSync(
      isAbsolute(workdir) ? workdir : `${cwd}${sep}${workdir}`,
    ).isDirectory();
  } catch {
    return false;
  }
}

function receiptEvidence(
  name: string,
  text: string | undefined,
  response: unknown,
): Record<string, string> | undefined {
  const receipt = object(object(object(response)?._meta)?.["aide/retrieval"]);
  if (
    !receipt ||
    receipt.version !== 1 ||
    receipt.tool !== name ||
    text === undefined ||
    receipt.text_sha256 !== hash(text) ||
    typeof receipt.id !== "string" ||
    !receipt.id ||
    receipt.id.length > 128 ||
    !Array.isArray(receipt.references) ||
    !receipt.references.length ||
    receipt.references.length > 10
  )
    return;
  const files = new Set<string>();
  const references = [];
  for (const value of receipt.references) {
    const ref = object(value);
    if (
      !ref ||
      typeof ref.file !== "string" ||
      !ref.file ||
      ref.file.length > 4096 ||
      files.has(ref.file) ||
      typeof ref.sha256 !== "string" ||
      !/^[a-f0-9]{64}$/.test(ref.sha256) ||
      typeof ref.bytes !== "number" ||
      !Number.isSafeInteger(ref.bytes) ||
      ref.bytes < 0
    )
      return;
    files.add(ref.file);
    references.push({ file: ref.file, sha256: ref.sha256, bytes: ref.bytes });
  }
  return {
    retrieval_id: receipt.id,
    source_verification: "server_receipt",
    source_references: JSON.stringify(references),
    reference_kind: "full_file",
  };
}

export function retrievalEvidence(
  cwd: string,
  name: string,
  args: Record<string, unknown>,
  text: string | undefined,
  response: unknown,
  failed: boolean,
  hookExitCode?: unknown,
  options: { requireShellWorkdir?: boolean } = {},
): Record<string, string> {
  const result = object(response);
  const shell =
    name === "Bash" ? shellTarget(args.command ?? args.cmd) : undefined;
  const exitCodes = [hookExitCode, result?.exit_code, result?.exitCode].filter(
    (value) => value !== undefined,
  );
  // Do not coerce malformed values or choose a successful value over conflicting
  // host evidence. Missing exit codes are allowed for hosts that omit them.
  const invalidExit =
    exitCodes.some(
      (value) => typeof value !== "number" || !Number.isSafeInteger(value),
    ) || new Set(exitCodes).size > 1;
  failed ||=
    result?.interrupted === true ||
    result?.timed_out === true ||
    exitCodes.some(
      (value) =>
        typeof value === "number" &&
        Number.isSafeInteger(value) &&
        value !== 0 &&
        // grep/rg status 1 means no selected matches, not a retrieval error.
        // Explicit host failures and interruption markers above still win.
        !(shell?.search && value === 1),
    );
  const pending =
    name === "Bash" &&
    result?.session_id !== undefined &&
    exitCodes.every((value) => value === null);
  if (name === "code_outline" || name === "code_read_symbol") {
    const receipt = receiptEvidence(name, text, response);
    return {
      retrieval_method: name,
      ...(typeof args.file === "string" && args.file
        ? { retrieval_target: relative(cwd, resolve(cwd, args.file)) }
        : {}),
      retrieval_status: failed
        ? "failed"
        : receipt && !invalidExit
          ? "referenced"
          : "unverified",
      ...receipt,
    };
  }
  const path = args.file_path ?? args.filePath;
  const target =
    name === "Read" && typeof path === "string"
      ? {
          file: path,
          method: "native_read",
          start: args.offset,
          end: undefined as number | undefined,
          search: false,
        }
      : name === "Bash"
        ? shell
        : name === "Grep"
          ? {
              method: "native_search",
              search: true,
              file: typeof args.path === "string" ? args.path : undefined,
            }
          : undefined;
  if (!target)
    return name === "Bash" ? { retrieval_status: "unclassified_shell" } : {};
  const attrs: Record<string, string> = {
    retrieval_method: target.method,
    retrieval_status: failed
      ? "failed"
      : pending
        ? "pending"
        : invalidExit
          ? "unverified"
          : target.search
            ? "search"
            : "unverified",
  };
  if (name === "Bash" && target.file) {
    const workdir = args.workdir ?? args.cwd;
    // Some hosts omit the shell's execution directory from hook input. A
    // matching project-root file cannot establish where a relative read ran.
    if (
      options.requireShellWorkdir &&
      !isAbsolute(target.file) &&
      !validShellWorkdir(cwd, workdir)
    ) {
      attrs.retrieval_reason = "unknown_shell_workdir";
      return attrs;
    }
    if (!(options.requireShellWorkdir && isAbsolute(target.file))) {
      if (workdir !== undefined && typeof workdir !== "string")
        return { retrieval_status: "unclassified_shell" };
      target.file = resolve(cwd, workdir ?? ".", target.file);
    }
  }
  if (target.file)
    attrs.retrieval_target = relative(cwd, resolve(cwd, target.file));
  if (
    failed ||
    invalidExit ||
    pending ||
    target.search ||
    !target.file ||
    text === undefined
  )
    return attrs;
  const current = snapshot(cwd, target.file);
  if (!current) return attrs;
  const delivered = Buffer.from(text, "utf8");
  if (current.bytes.equals(delivered)) {
    attrs.retrieval_status = "full_file";
    attrs.source_verification = "current_file_match";
  } else {
    const start = target.start ?? 1;
    const end =
      target.end ??
      (typeof args.limit === "number" && typeof start === "number"
        ? start + args.limit - 1
        : undefined);
    if (
      typeof start !== "number" ||
      !Number.isSafeInteger(start) ||
      start < 1 ||
      end === undefined ||
      !Number.isSafeInteger(end) ||
      end < start
    )
      return attrs;
    const source = current.bytes.toString("utf8");
    if (!current.bytes.equals(Buffer.from(source))) return attrs;
    const lines = source.match(/[^\n]*\n|[^\n]+$/g) ?? [];
    const selected = lines.slice(start - 1, end).join("");
    if (!selected || selected !== text) return attrs;
    attrs.retrieval_status = "range";
    // Only these lines matched a current snapshot. This does not prove the
    // undisplayed part had this version when the host originally read it.
    attrs.source_verification = "current_range_match";
    attrs.delivered_start_line = String(start);
    attrs.delivered_end_line = String(Math.min(end, lines.length));
  }
  attrs.source_references = JSON.stringify([
    {
      file: current.file,
      sha256: hash(current.bytes),
      bytes: current.bytes.length,
    },
  ]);
  attrs.reference_kind = "full_file";
  return attrs;
}
