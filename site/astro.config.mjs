import { defineConfig } from "astro/config"

// GitHub Pages serves the site under /ember; a custom domain would drop the base.
export default defineConfig({
  site: "https://tejas242.github.io",
  base: "/ember",
  compressHTML: true,
})
