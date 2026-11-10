import tailwindcss from "@tailwindcss/vite";
import { tanstackStart } from "@tanstack/react-start/plugin/vite";
import viteReact from "@vitejs/plugin-react";
import { defineConfig } from "vite-plus";
import { kandoPostBuild, kandoSite, prerenderPages } from "./buildtime/plugin";

// outDir is where the site is built; the tests build into a temporary folder through KANDO_SITE_OUT, which the prerender's preview server sees too when it loads this file again.
const outDir = process.env["KANDO_SITE_OUT"] ?? "dist";
// publicDir is where the build writes the files it collects from the repository; a test build of its own gets another through KANDO_SITE_PUBLIC, so that it leaves the main build's alone.
const publicDir = process.env["KANDO_SITE_PUBLIC"] ?? ".generated/public";
// contentDir holds the content files; a language has pages only when it has one. The tests point it at a copy through KANDO_SITE_CONTENT to build without a language.
const contentDir = process.env["KANDO_SITE_CONTENT"] ?? "content";
const ignore = ["dist/**", ".generated/**", "src/routeTree.gen.ts", "static/**", "testdata/**"];

export default defineConfig({
  publicDir,
  build: { outDir, emptyOutDir: true, assetsInlineLimit: 0 },
  fmt: { ignorePatterns: ignore, printWidth: 160 },
  lint: { ignorePatterns: ignore, options: { typeAware: true, typeCheck: true } },
  test: {
    include: ["test/**/*.test.ts"],
    environment: "node",
    globalSetup: ["test/global-setup.ts"],
    testTimeout: 60_000,
    hookTimeout: 300_000,
  },
  plugins: [
    kandoSite({ repoRoot: "../..", outDir, publicDir, contentDir }),
    tailwindcss(),
    tanstackStart({
      pages: prerenderPages(contentDir),
      prerender: { enabled: true, crawlLinks: false, autoStaticPathsDiscovery: false, failOnError: true },
      sitemap: { enabled: false },
      server: { build: { inlineCss: false } },
    }),
    viteReact(),
    kandoPostBuild({ outDir }),
  ],
});
