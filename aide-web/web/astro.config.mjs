import { defineConfig } from "astro/config";
import react from "@astrojs/react";
import tailwind from "@astrojs/tailwind";

export default defineConfig({
  integrations: [
    react(),
    tailwind({
      applyBaseStyles: false,
    }),
  ],
  outDir: "../internal/webdist/build",
  vite: {
    build: {
      // Ensure assets use relative paths so Go embed serving works
      assetsDir: "_astro",
    },
  },
});
