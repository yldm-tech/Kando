// TestBuildRefusesForeignFolder: the build will not empty a folder that holds something other than a previous build, so a mistyped output folder cannot wipe the repository.
import { existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, it } from "vite-plus/test";
import { checkOutDir } from "../buildtime/plugin";

it("refuses a folder that holds no previous build", () => {
  const out = mkdtempSync(join(tmpdir(), "kando-out-"));
  try {
    writeFileSync(join(out, "keep.txt"), "mine");
    expect(() => checkOutDir(out)).toThrow(/no previous build/);
    expect(existsSync(join(out, "keep.txt"))).toBe(true);
    mkdirSync(join(out, "client"));
    writeFileSync(join(out, "client", "CNAME"), "kando.yldm.ai\n");
    expect(() => checkOutDir(out)).not.toThrow();
    expect(() => checkOutDir(join(out, "missing"))).not.toThrow();
  } finally {
    rmSync(out, { recursive: true, force: true });
  }
});
