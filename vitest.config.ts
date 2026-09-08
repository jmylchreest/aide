import { configDefaults, defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    // The dashboard and isolated Bun evaluation fixtures have separate runners.
    exclude: [
      ...configDefaults.exclude,
      "dist/**",
      "aide-web/**",
      ".aide/**",
      "scripts/retrieval-quality/outcomes-v1/**",
    ],
  },
});
