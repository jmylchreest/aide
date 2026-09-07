import { configDefaults, defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    // The dashboard has its own dependencies and test job.
    exclude: [...configDefaults.exclude, "dist/**", "aide-web/**"],
  },
});
