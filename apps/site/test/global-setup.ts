// Builds the site once for the whole test run, into a temporary folder under .generated, as the Go tests' TestMain did; or uses the build in KANDO_SITE_DIST, as CI does after vp build.
import { spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync } from "node:fs";
import { join, resolve } from "node:path";
import type { TestProject } from "vite-plus/test/node";

declare module "vite-plus/test" {
  export interface ProvidedContext {
    // dist is the built site's folder of pages, <outDir>/client.
    dist: string;
  }
}

export default function setup(project: TestProject): () => void {
  const given = process.env["KANDO_SITE_DIST"];
  if (given) {
    project.provide("dist", join(resolve(given), "client"));
    return () => {};
  }
  // The build goes into the site's folder, not the system's temporary one: the prerender runs the server bundle, which finds React in the site's node_modules.
  mkdirSync(".generated", { recursive: true });
  const out = mkdtempSync(join(resolve(".generated"), "test-build-"));
  const build = spawnSync("vp", ["build"], {
    env: { ...process.env, KANDO_SITE_OUT: out },
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
    timeout: 280_000,
  });
  if (build.status !== 0) {
    rmSync(out, { recursive: true, force: true });
    throw new Error(`vp build failed (${build.status ?? build.signal}):\n${build.stdout}\n${build.stderr}`);
  }
  project.provide("dist", join(out, "client"));
  return () => rmSync(out, { recursive: true, force: true });
}
