/**
 * Tests for skill-matcher platform filtering
 *
 * Run with: npx vitest run src/test/skill-matcher.test.ts
 */

import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { mkdtempSync, rmSync, mkdirSync, writeFileSync } from "fs";
import { join } from "path";
import { tmpdir } from "os";
import {
  matchSkills,
  discoverSkills,
  parseSkillFrontmatter,
} from "../core/skill-matcher.js";
import type { Skill } from "../core/types.js";

// ─── parseSkillFrontmatter: platforms parsing ────────────────────────────

describe("parseSkillFrontmatter", () => {
  it("parses platforms array from frontmatter", () => {
    const content = `---
name: test
description: A test skill
triggers:
  - test
platforms:
  - opencode
---

# Test
`;
    const result = parseSkillFrontmatter(content);
    expect(result).not.toBeNull();
    expect(result!.meta.platforms).toEqual(["opencode"]);
  });

  it("parses multiple platforms", () => {
    const content = `---
name: test
description: A test skill
triggers:
  - test
platforms:
  - opencode
  - claude-code
---

# Test
`;
    const result = parseSkillFrontmatter(content);
    expect(result!.meta.platforms).toEqual(["opencode", "claude-code"]);
  });

  it("omits platforms when not specified", () => {
    const content = `---
name: test
description: A test skill
triggers:
  - test
---

# Test
`;
    const result = parseSkillFrontmatter(content);
    expect(result).not.toBeNull();
    expect(result!.meta.platforms).toBeUndefined();
  });
});

// ─── matchSkills: platform filtering ─────────────────────────────────────

describe("matchSkills platform filtering", () => {
  const makeSkill = (
    name: string,
    triggers: string[],
    platforms?: string[],
  ): Skill => ({
    name,
    path: `/fake/${name}.md`,
    triggers,
    platforms,
    content: `# ${name}`,
  });

  const universalSkill = makeSkill("deploy", ["deploy", "ship"]);
  const ocOnlySkill = makeSkill(
    "context-usage",
    ["context usage", "token usage"],
    ["opencode"],
  );
  const ccOnlySkill = makeSkill("cc-tool", ["cc special"], ["claude-code"]);
  const bothSkill = makeSkill(
    "both",
    ["both platforms"],
    ["opencode", "claude-code"],
  );

  const allSkills = [universalSkill, ocOnlySkill, ccOnlySkill, bothSkill];

  it("returns all matching skills when no platform specified (up to maxResults)", () => {
    const matches = matchSkills(
      "deploy context usage cc special both platforms",
      allSkills,
      10,
    );
    expect(matches).toHaveLength(4);
  });

  it("filters out claude-code-only skills on opencode", () => {
    const matches = matchSkills("cc special", allSkills, 3, "opencode");
    expect(matches).toHaveLength(0);
  });

  it("filters out opencode-only skills on claude-code", () => {
    const matches = matchSkills("context usage", allSkills, 3, "claude-code");
    expect(matches).toHaveLength(0);
  });

  it("includes universal skills (no platforms field) on any platform", () => {
    const ocMatches = matchSkills("deploy", allSkills, 3, "opencode");
    expect(ocMatches).toHaveLength(1);
    expect(ocMatches[0].name).toBe("deploy");

    const ccMatches = matchSkills("deploy", allSkills, 3, "claude-code");
    expect(ccMatches).toHaveLength(1);
    expect(ccMatches[0].name).toBe("deploy");
  });

  it("includes platform-specific skill on matching platform", () => {
    const matches = matchSkills("context usage", allSkills, 3, "opencode");
    expect(matches).toHaveLength(1);
    expect(matches[0].name).toBe("context-usage");
  });

  it("includes skills listed for both platforms", () => {
    const ocMatches = matchSkills("both platforms", allSkills, 3, "opencode");
    expect(ocMatches).toHaveLength(1);
    expect(ocMatches[0].name).toBe("both");

    const ccMatches = matchSkills(
      "both platforms",
      allSkills,
      3,
      "claude-code",
    );
    expect(ccMatches).toHaveLength(1);
    expect(ccMatches[0].name).toBe("both");
  });
});

// ─── discoverSkills + loadSkill: platforms field ─────────────────────────

describe("discoverSkills with platforms", () => {
  let projectDir: string;

  beforeEach(() => {
    projectDir = mkdtempSync(join(tmpdir(), "aide-skill-test-"));
    mkdirSync(join(projectDir, ".aide", "skills"), { recursive: true });
  });

  afterEach(() => {
    rmSync(projectDir, { recursive: true, force: true });
  });

  it("loads platforms from discovered skill files", () => {
    writeFileSync(
      join(projectDir, ".aide", "skills", "oc-only.md"),
      `---
name: oc-only
description: OpenCode only skill
triggers:
  - oc test
platforms:
  - opencode
---

# OC Only
`,
    );

    const skills = discoverSkills(projectDir);
    expect(skills).toHaveLength(1);
    expect(skills[0].platforms).toEqual(["opencode"]);
  });

  it("loads skill without platforms as undefined", () => {
    writeFileSync(
      join(projectDir, ".aide", "skills", "universal.md"),
      `---
name: universal
description: Universal skill
triggers:
  - universal test
---

# Universal
`,
    );

    const skills = discoverSkills(projectDir);
    expect(skills).toHaveLength(1);
    expect(skills[0].platforms).toBeUndefined();
  });
});
