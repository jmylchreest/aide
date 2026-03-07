import type { ReactNode } from "react";
import Link from "@docusaurus/Link";
import useDocusaurusContext from "@docusaurus/useDocusaurusContext";
import Layout from "@theme/Layout";

import styles from "./index.module.css";

function HeroSection() {
  return (
    <section className={styles.hero}>
      <div className={styles.heroInner}>
        <h1 className={styles.title}>
          <span className={styles.titleAccent}>AIDE</span>
        </h1>
        <p className={styles.subtitle}>AI Development Environment</p>
        <p className={styles.tagline}>
          Multi-agent orchestration, persistent memory, and intelligent
          workflows for AI coding assistants.
        </p>

        <div className={styles.buttons}>
          <Link className={styles.primaryBtn} to="/docs">
            Get Started
          </Link>
          <Link
            className={styles.secondaryBtn}
            to="https://github.com/jmylchreest/aide"
          >
            View on GitHub
          </Link>
        </div>

        <div className={styles.codePreview}>
          <div className={styles.codeHeader}>
            <span className={`${styles.codeDot} ${styles.codeDotRed}`}></span>
            <span
              className={`${styles.codeDot} ${styles.codeDotYellow}`}
            ></span>
            <span className={`${styles.codeDot} ${styles.codeDotGreen}`}></span>
            <span className={styles.codeTitle}>terminal</span>
          </div>
          <div className={styles.codeContent}>
            <div className={styles.codeLine}>
              <span className={styles.codePrompt}>$</span>
              <span className={styles.codeCommand}>
                swarm 3 implement the dashboard
              </span>
            </div>
            <div className={styles.codeOutput}>
              <div>Spawning 3 parallel agents...</div>
              <div>Agent 1: DESIGN &rarr; TEST &rarr; DEV &rarr; VERIFY</div>
              <div>Agent 2: DESIGN &rarr; TEST &rarr; DEV &rarr; VERIFY</div>
              <div>Agent 3: DESIGN &rarr; TEST &rarr; DEV &rarr; VERIFY</div>
            </div>
            <div className={styles.codeLine} style={{ marginTop: "1rem" }}>
              <span className={styles.codePrompt}>$</span>
              <span className={styles.codeCommand}>
                remember that I prefer vitest for testing
              </span>
            </div>
            <div className={styles.codeOutput}>
              <div>Stored. Will auto-inject in future sessions.</div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

type FeatureItem = {
  icon: string;
  title: string;
  description: string;
};

const features: FeatureItem[] = [
  {
    icon: "\u{1F9E0}",
    title: "Persistent Memory",
    description:
      "Memories and decisions persist across sessions and auto-inject at startup. Never repeat yourself to your AI assistant again.",
  },
  {
    icon: "\u{1F41D}",
    title: "Swarm Mode",
    description:
      "Spawn parallel agents with full SDLC pipelines per story. Each agent designs, tests, implements, and verifies independently.",
  },
  {
    icon: "\u{1F50D}",
    title: "Code Indexing",
    description:
      "Fast symbol search using tree-sitter. Find functions, classes, and references across your entire codebase instantly.",
  },
  {
    icon: "\u{1F6E1}",
    title: "Static Analysis",
    description:
      "4 built-in analysers detect complexity, coupling, secrets, and code clones without any external tools.",
  },
  {
    icon: "\u{26A1}",
    title: "Skills System",
    description:
      "Markdown-based skills activate by keyword with fuzzy matching. 20+ built-in skills for testing, debugging, reviewing, and more.",
  },
  {
    icon: "\u{1F517}",
    title: "Multi-Platform",
    description:
      "Supports Claude Code and OpenCode through a shared core with platform-specific adapters. Skills and MCP tools work everywhere.",
  },
];

function FeaturesSection() {
  return (
    <section className={styles.features}>
      <div className={styles.featuresInner}>
        <h2 className={styles.featuresTitle}>
          Supercharge Your AI Coding Assistant
        </h2>
        <div className={styles.featuresGrid}>
          {features.map((feature, idx) => (
            <div key={idx} className={styles.featureCard}>
              <div className={styles.featureIcon}>{feature.icon}</div>
              <h3 className={styles.featureTitle}>{feature.title}</h3>
              <p className={styles.featureDesc}>{feature.description}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

function ComparisonSection() {
  return (
    <section className={styles.comparison}>
      <div className={styles.comparisonInner}>
        <h2 className={styles.comparisonTitle}>Before &amp; After</h2>
        <div className={styles.comparisonGrid}>
          <div className={styles.comparisonCol}>
            <h3 className={styles.comparisonColTitle}>Without AIDE</h3>
            <ul className={styles.comparisonList}>
              <li>Context lost between sessions</li>
              <li>Manual task coordination</li>
              <li>Repeated setup instructions</li>
              <li>No code search</li>
              <li>No code quality analysis</li>
              <li>Decisions forgotten</li>
            </ul>
          </div>
          <div
            className={`${styles.comparisonCol} ${styles.comparisonColHighlight}`}
          >
            <h3 className={styles.comparisonColTitle}>With AIDE</h3>
            <ul className={styles.comparisonList}>
              <li>Memories persist and auto-inject</li>
              <li>Swarm mode with parallel agents</li>
              <li>Skills activate by keyword</li>
              <li>Fast symbol search across codebase</li>
              <li>Static analysis with 4 analysers</li>
              <li>Decisions recorded and enforced</li>
            </ul>
          </div>
        </div>
      </div>
    </section>
  );
}

function InstallSection() {
  return (
    <section className={styles.install}>
      <div className={styles.installInner}>
        <h2 className={styles.installTitle}>Quick Install</h2>
        <div className={styles.installCards}>
          <div className={styles.installCard}>
            <h3>Claude Code</h3>
            <div className={styles.installCode}>
              <span>
                <span className={styles.installPrompt}>$ </span>
                claude plugin marketplace add jmylchreest/aide
              </span>
            </div>
          </div>
          <div className={styles.installCard}>
            <h3>OpenCode</h3>
            <div className={styles.installCode}>
              <span>
                <span className={styles.installPrompt}>$ </span>
                bunx @jmylchreest/aide-plugin install
              </span>
            </div>
          </div>
        </div>
        <p style={{ marginTop: "1.5rem", color: "#666", fontSize: "0.875rem" }}>
          See <Link to="/docs/getting-started">Installation Guide</Link> for all
          methods including source builds
        </p>
      </div>
    </section>
  );
}

export default function Home(): ReactNode {
  const { siteConfig } = useDocusaurusContext();
  return (
    <Layout title="Home" description={siteConfig.tagline}>
      <HeroSection />
      <FeaturesSection />
      <ComparisonSection />
      <InstallSection />
    </Layout>
  );
}
