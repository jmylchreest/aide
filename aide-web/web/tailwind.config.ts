import type { Config } from "tailwindcss";

const config: Config = {
  darkMode: "class",
  content: ["./src/**/*.{astro,html,js,jsx,ts,tsx}"],
  theme: {
    extend: {
      colors: {
        aide: {
          bg: "#0a0a0a",
          surface: "#141414",
          "surface-hover": "#1a1a1a",
          border: "#262626",
          "border-light": "#333333",
          text: "#e5e5e5",
          "text-muted": "#a3a3a3",
          "text-dim": "#737373",
          accent: "#22d3ee",
          "accent-dark": "#06b6d4",
          "accent-light": "#67e8f9",
          "accent-dim": "#0e7490",
          green: "#34d399",
          red: "#f87171",
          yellow: "#fbbf24",
        },
      },
      fontFamily: {
        mono: [
          "JetBrains Mono",
          "Fira Code",
          "Consolas",
          "monospace",
        ],
        sans: ["Inter", "system-ui", "sans-serif"],
      },
      fontSize: {
        xs: ["0.7rem", { lineHeight: "1rem" }],
        sm: ["0.8rem", { lineHeight: "1.25rem" }],
        base: ["0.85rem", { lineHeight: "1.4" }],
      },
      borderRadius: {
        DEFAULT: "0.5rem",
        sm: "0.375rem",
      },
    },
  },
  plugins: [],
};

export default config;
