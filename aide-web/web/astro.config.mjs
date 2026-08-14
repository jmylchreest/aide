import { defineConfig } from "astro/config";
import react from "@astrojs/react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  integrations: [react()],
  outDir: "../internal/webdist/build",
  vite: {
    // Tailwind v4 ships as a Vite plugin; the @astrojs/tailwind integration
    // drove it through PostCSS, which v4 moved to a separate package.
    plugins: [tailwindcss()],
    build: {
      // Ensure assets use relative paths so Go embed serving works
      assetsDir: "_astro",
    },
  },
});
