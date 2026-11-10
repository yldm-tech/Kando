// TestIcons, TestShrink and TestOpaqueSquare: the icons made from the app icon, the web app manifest's among them, have their sizes, and the resizing averages premultiplied colors as Go's image.RGBA does.
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, it } from "vite-plus/test";
import { logoSize, newRGBA, opaqueSquare, pngSize, shrink } from "../buildtime/icons";
import { dist } from "./helpers";

it("writes the icons in their sizes", () => {
  for (const [file, size] of [
    ["logo.png", logoSize],
    ["apple-touch-icon.png", 180],
    ["favicon-32.png", 32],
    ["favicon-16.png", 16],
    ["icon-192.png", 192],
    ["icon-512.png", 512],
  ] as const) {
    expect(pngSize(join(dist(), file)), file).toEqual({ w: size, h: size });
  }
  const ico = readFileSync(join(dist(), "favicon.ico"));
  expect(ico.readUInt16LE(2)).toBe(1);
  expect(ico.readUInt16LE(4)).toBe(2);
  expect([ico[6], ico[22]]).toEqual([16, 32]);
});

it("shrinks by averaging the pixels each output pixel covers, opacity included", () => {
  const src = newRGBA(4, 4);
  for (let y = 0; y < 4; y++) {
    for (let x = 0; x < 2; x++) {
      src.pix.set([200, 0, 0, 255], (y * 4 + x) * 4);
    }
  }
  const half = shrink(src, 2);
  expect([...half.pix.subarray(0, 4)]).toEqual([200, 0, 0, 255]);
  expect([...half.pix.subarray(12, 16)]).toEqual([0, 0, 0, 0]);
  // Red at half opacity, premultiplied.
  expect([...shrink(src, 1).pix]).toEqual([100, 0, 0, 128]);
});

it("drops the transparent margin around the app icon's square", () => {
  const src = newRGBA(10, 10);
  for (let y = 2; y < 8; y++) {
    for (let x = 2; x < 8; x++) {
      src.pix.set([0, 0, 255, 255], (y * 10 + x) * 4);
    }
  }
  const got = opaqueSquare(src);
  expect([got.w, got.h]).toEqual([6, 6]);
  expect(got.pix[3]).toBe(255);
});
