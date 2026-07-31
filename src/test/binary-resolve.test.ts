/**
 * Tests for locating the npm-bundled aide binary.
 *
 * The regression these pin down: package managers HOIST optionalDependencies
 * to the install root, so the per-arch package is a sibling of the plugin.
 * A lookup under <plugin-root>/node_modules/ finds nothing in any real
 * install, which silently sends every OpenCode/Codex user down the download
 * path the bundling was meant to replace.
 *
 * Run with: npx vitest run src/test/binary-resolve.test.ts
 */

import { describe, it, expect, afterEach } from "vitest";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";
import {
  bundledBinaryCandidates,
  getArchPackageSuffix,
  getBinaryFileName,
  getBinaryPackageName,
  resolveBundledBinary,
} from "../lib/binary-resolve.js";

const LINUX = { platform: "linux", arch: "x64" } as const;

describe("platform mapping", () => {
  it("maps node platform/arch to the published package names", () => {
    expect(getArchPackageSuffix({ platform: "linux", arch: "x64" })).toBe("linux-amd64");
    expect(getArchPackageSuffix({ platform: "darwin", arch: "arm64" })).toBe("darwin-arm64");
    expect(getArchPackageSuffix({ platform: "win32", arch: "x64" })).toBe("windows-amd64");
    expect(getBinaryPackageName({ platform: "win32", arch: "arm64" })).toBe(
      "@jmylchreest/aide-binary-windows-arm64",
    );
  });

  it("returns null for platforms we publish no binary for", () => {
    expect(getArchPackageSuffix({ platform: "freebsd", arch: "x64" })).toBeNull();
    expect(getBinaryPackageName({ platform: "linux", arch: "ia32" })).toBeNull();
  });

  it("keeps the .exe suffix on Windows only", () => {
    expect(getBinaryFileName({ platform: "win32" })).toBe("aide.exe");
    expect(getBinaryFileName({ platform: "linux" })).toBe("aide");
  });
});

describe("resolveBundledBinary", () => {
  let tempDir: string;

  afterEach(() => {
    if (tempDir) rmSync(tempDir, { recursive: true, force: true });
  });

  function makeTree(binaryAt: string | null): string {
    tempDir = mkdtempSync(join(tmpdir(), "aide-resolve-"));
    const pluginRoot = join(
      tempDir,
      "node_modules",
      "@jmylchreest",
      "aide-plugin",
    );
    mkdirSync(pluginRoot, { recursive: true });
    if (binaryAt) {
      const full = join(tempDir, binaryAt);
      mkdirSync(join(full, ".."), { recursive: true });
      writeFileSync(full, "#!/bin/sh\n");
    }
    return pluginRoot;
  }

  const HOISTED =
    "node_modules/@jmylchreest/aide-binary-linux-amd64/bin/aide";
  const NESTED =
    "node_modules/@jmylchreest/aide-plugin/node_modules/@jmylchreest/aide-binary-linux-amd64/bin/aide";

  // `exists` keeps these on the synthetic tree: without it, node resolution
  // runs first and would answer from whatever this checkout has installed.
  it("finds the hoisted sibling package (what npm/bun actually produce)", () => {
    const pluginRoot = makeTree(HOISTED);
    const found = resolveBundledBinary({ pluginRoot, exists: existsSync, ...LINUX });
    expect(found).toBe(join(tempDir, HOISTED));
  });

  it("finds a nested copy when a version conflict forced npm to nest", () => {
    const pluginRoot = makeTree(NESTED);
    const found = resolveBundledBinary({ pluginRoot, exists: existsSync, ...LINUX });
    expect(found).toBe(join(tempDir, NESTED));
  });

  it("returns null when the optional dependency was not installed", () => {
    const pluginRoot = makeTree(null);
    expect(resolveBundledBinary({ pluginRoot, exists: existsSync, ...LINUX })).toBeNull();
  });

  it("returns null on platforms with no published binary", () => {
    const pluginRoot = makeTree(HOISTED);
    expect(
      resolveBundledBinary({
        pluginRoot,
        exists: existsSync,
        platform: "freebsd",
        arch: "x64",
      }),
    ).toBeNull();
  });

  it("looks for aide.exe on Windows", () => {
    const candidates = bundledBinaryCandidates("/x/node_modules/@jmylchreest/aide-plugin", {
      platform: "win32",
      arch: "x64",
    });
    expect(candidates.every((c) => c.endsWith("aide.exe"))).toBe(true);
    expect(
      candidates.some((c) =>
        c.includes(join("node_modules", "@jmylchreest", "aide-binary-windows-amd64")),
      ),
    ).toBe(true);
  });

  it("checks the hoist location before giving up", () => {
    const pluginRoot = join("/x", "node_modules", "@jmylchreest", "aide-plugin");
    const candidates = bundledBinaryCandidates(pluginRoot, LINUX);
    expect(candidates).toContain(
      join("/x", "node_modules", "@jmylchreest", "aide-binary-linux-amd64", "bin", "aide"),
    );
  });
});

describe("wrapper parity", () => {
  // bin/aide-wrapper.ts duplicates this logic because it runs before
  // node_modules is guaranteed to exist. Keep the duplication honest.
  const wrapper = readFileSync(
    join(import.meta.dirname, "..", "..", "bin", "aide-wrapper.ts"),
    "utf-8",
  );

  it("uses node resolution, not a plugin-root-only lookup", () => {
    expect(wrapper).toContain("createRequire");
    expect(wrapper).toMatch(/resolve\(\s*`\$\{BINARY_PKG\}\/bin\/aide\$\{EXT\}`/);
  });

  it("walks up node_modules for the hoisted layout", () => {
    expect(wrapper).toContain('dir.endsWith(`${sep}node_modules`)');
  });

  it("knows the same six platforms", () => {
    for (const key of [
      "linux-x64",
      "linux-arm64",
      "darwin-x64",
      "darwin-arm64",
      "win32-x64",
      "win32-arm64",
    ]) {
      expect(wrapper).toContain(`"${key}"`);
    }
  });

  it("verifies the bundled binary version instead of trusting it blindly", () => {
    expect(wrapper).toContain("versionGte(bundledVersion.split(\"-\")[0], pluginVersion)");
  });
});
