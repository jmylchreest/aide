import { themes as prismThemes } from "prism-react-renderer";
import type { Config } from "@docusaurus/types";
import type * as Preset from "@docusaurus/preset-classic";

// Check if we have any released versions
const versions: string[] = require("./versions.json");
const hasReleasedVersions = versions.length > 0;

const config: Config = {
  title: "AIDE",
  tagline:
    "Multi-agent orchestration, persistent memory, and intelligent workflows for AI coding assistants",
  favicon: "img/favicon.svg",

  // GitHub Pages deployment
  url: "https://jmylchreest.github.io",
  baseUrl: "/aide/",
  organizationName: "jmylchreest",
  projectName: "aide",
  deploymentBranch: "gh-pages",
  trailingSlash: false,

  onBrokenLinks: "throw",

  markdown: {
    hooks: {
      onBrokenMarkdownLinks: "warn",
    },
  },

  i18n: {
    defaultLocale: "en",
    locales: ["en"],
  },

  presets: [
    [
      "classic",
      {
        docs: {
          sidebarPath: "./sidebars.ts",
          editUrl: "https://github.com/jmylchreest/aide/tree/main/docs/",
          includeCurrentVersion: true,
          ...(hasReleasedVersions
            ? {
                versions: {
                  current: {
                    label: "main",
                    path: "next",
                    banner: "unreleased",
                  },
                },
                lastVersion: versions[0],
              }
            : {}),
        },
        blog: false,
        theme: {
          customCss: "./src/css/custom.css",
        },
      } satisfies Preset.Options,
    ],
  ],

  // Local search plugin (no external service dependency)
  plugins: [
    [
      "@cmfcmf/docusaurus-search-local",
      {
        indexDocs: true,
        indexBlog: false,
        indexPages: true,
        language: "en",
        maxSearchResults: 8,
      },
    ],
  ],

  themeConfig: {
    colorMode: {
      defaultMode: "dark",
      disableSwitch: false,
      respectPrefersColorScheme: true,
    },

    navbar: {
      title: "AIDE",
      logo: {
        alt: "AIDE Logo",
        src: "img/logo.svg",
      },
      items: [
        {
          type: "docSidebar",
          sidebarId: "docs",
          position: "left",
          label: "Documentation",
        },
        ...(hasReleasedVersions
          ? [
              {
                type: "docsVersionDropdown" as const,
                position: "right" as const,
                dropdownActiveClassDisabled: true,
              },
            ]
          : []),
        {
          href: "https://github.com/jmylchreest/aide",
          label: "GitHub",
          position: "right",
        },
      ],
    },

    footer: {
      style: "dark",
      links: [
        {
          title: "Documentation",
          items: [
            { label: "Getting Started", to: "/docs/getting-started/" },
            { label: "Features", to: "/docs/features/memory" },
            { label: "Skills", to: "/docs/skills/" },
          ],
        },
        {
          title: "Community",
          items: [
            { label: "GitHub", href: "https://github.com/jmylchreest/aide" },
            {
              label: "Issues",
              href: "https://github.com/jmylchreest/aide/issues",
            },
          ],
        },
        {
          title: "Related Projects",
          items: [
            {
              label: "Claude Code",
              href: "https://docs.anthropic.com/en/docs/agents-and-tools/claude-code/overview",
            },
            { label: "OpenCode", href: "https://opencode.ai" },
          ],
        },
      ],
      copyright: `Copyright ${new Date().getFullYear()} AIDE. Built with Docusaurus.`,
    },

    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: [
        "bash",
        "toml",
        "json",
        "go",
        "typescript",
        "markdown",
      ],
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
