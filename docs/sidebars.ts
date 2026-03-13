import type { SidebarsConfig } from "@docusaurus/plugin-content-docs";

const sidebars: SidebarsConfig = {
  docs: [
    "intro",
    {
      type: "category",
      label: "Getting Started",
      link: {
        type: "doc",
        id: "getting-started/index",
      },
      items: [
        "getting-started/claude-code",
        "getting-started/opencode",
        "getting-started/from-source",
        "getting-started/configuration",
      ],
    },
    {
      type: "category",
      label: "Features",
      items: [
        "features/memory",
        "features/decisions",
        "features/code-indexing",
        "features/static-analysis",
        "features/status-dashboard",
      ],
    },
    {
      type: "category",
      label: "Skills",
      link: {
        type: "doc",
        id: "skills/index",
      },
      items: ["skills/built-in", "skills/custom"],
    },
    {
      type: "category",
      label: "Modes",
      link: {
        type: "doc",
        id: "modes/index",
      },
      items: ["modes/swarm", "modes/ralph"],
    },
    {
      type: "category",
      label: "Integrations",
      items: ["integrations/index"],
    },
    {
      type: "category",
      label: "Reference",
      link: {
        type: "doc",
        id: "reference/index",
      },
      items: [
        "reference/architecture",
        "reference/mcp-tools",
        "reference/cli",
        "reference/storage",
        "reference/hooks",
        "reference/platform-comparison",
      ],
    },
  ],
};

export default sidebars;
