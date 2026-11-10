// The icons made from the app icon (apps/kando/resources/icon.png): logo.png, favicon-16.png and favicon-32.png, favicon.ico holding both, apple-touch-icon.png, and icon-192.png and icon-512.png, the web app manifest's (site.webmanifest). The app icon is a rounded square on a transparent margin; the logo and the favicons are that square without the margin, and the touch icon fills the square's corners with its background gradient too, because iOS rounds the corners itself and would show the transparent ones black.
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { PNG } from "pngjs";

// RGBA is an image of premultiplied 8-bit colors, row by row, as Go's image.RGBA holds them, so that averaging pixels weighs each one's color by its opacity.
export interface RGBA {
  w: number;
  h: number;
  pix: Uint8Array;
}

// Background gradient of the app icon's amber glass, from its top edge to its bottom one, sampled from its rim.
const gradientFrom = [0xf3, 0xb0, 0x5e] as const;
const gradientTo = [0xec, 0x7e, 0x2e] as const;

// logoSize is the side of logo.png, the mark the pages show beside the name and above the headline: twice the largest size a page draws it at, the 96 pixels of the hero.
export const logoSize = 192;

// manifestIconSizes are the sides of the web app manifest's icons, the two sizes browsers ask a manifest for.
export const manifestIconSizes = [192, 512] as const;

export function newRGBA(w: number, h: number): RGBA {
  return { w, h, pix: new Uint8Array(w * h * 4) };
}

// premultiply turns pngjs's straight colors into premultiplied ones.
export function premultiply(png: { width: number; height: number; data: Uint8Array }): RGBA {
  const img = newRGBA(png.width, png.height);
  for (let i = 0; i < png.data.length; i += 4) {
    const a = png.data[i + 3] ?? 0;
    for (let c = 0; c < 3; c++) {
      img.pix[i + c] = Math.round(((png.data[i + c] ?? 0) * a) / 255);
    }
    img.pix[i + 3] = a;
  }
  return img;
}

// encodePNG writes img as a PNG, its colors made straight again as PNG stores them.
export function encodePNG(img: RGBA): Buffer {
  const png = new PNG({ width: img.w, height: img.h });
  for (let i = 0; i < img.pix.length; i += 4) {
    const a = img.pix[i + 3] ?? 0;
    for (let c = 0; c < 3; c++) {
      png.data[i + c] = a === 0 ? 0 : Math.min(255, Math.round(((img.pix[i + c] ?? 0) * 255) / a));
    }
    png.data[i + 3] = a;
  }
  return PNG.sync.write(png, { colorType: 6, deflateLevel: 9 });
}

// opaqueSquare is the smallest square around the pixels of img that are not fully transparent, centered on them.
export function opaqueSquare(img: RGBA): RGBA {
  let minX = img.w;
  let minY = img.h;
  let maxX = 0;
  let maxY = 0;
  for (let y = 0; y < img.h; y++) {
    for (let x = 0; x < img.w; x++) {
      if ((img.pix[(y * img.w + x) * 4 + 3] ?? 0) !== 0) {
        minX = Math.min(minX, x);
        minY = Math.min(minY, y);
        maxX = Math.max(maxX, x + 1);
        maxY = Math.max(maxY, y + 1);
      }
    }
  }
  const side = Math.max(maxX - minX, maxY - minY);
  const x0 = Math.trunc((minX + maxX) / 2) - Math.trunc(side / 2);
  const y0 = Math.trunc((minY + maxY) / 2) - Math.trunc(side / 2);
  const dst = newRGBA(side, side);
  for (let y = 0; y < side; y++) {
    for (let x = 0; x < side; x++) {
      const sx = x0 + x;
      const sy = y0 + y;
      if (sx < 0 || sy < 0 || sx >= img.w || sy >= img.h) {
        continue;
      }
      const s = (sy * img.w + sx) * 4;
      dst.pix.set(img.pix.subarray(s, s + 4), (y * side + x) * 4);
    }
  }
  return dst;
}

// gradient is a size×size square of the icon's background gradient, from top to bottom.
export function gradient(size: number): RGBA {
  const img = newRGBA(size, size);
  const span = size - 1;
  const lerp = (a: number, b: number, t: number) => Math.trunc(a + (b - a) * t + 0.5);
  for (let y = 0; y < size; y++) {
    const t = y / span;
    for (let x = 0; x < size; x++) {
      const i = (y * size + x) * 4;
      img.pix[i] = lerp(gradientFrom[0], gradientTo[0], t);
      img.pix[i + 1] = lerp(gradientFrom[1], gradientTo[1], t);
      img.pix[i + 2] = lerp(gradientFrom[2], gradientTo[2], t);
      img.pix[i + 3] = 255;
    }
  }
  return img;
}

// over draws src over dst, both premultiplied and the same size, as Go's draw.Over does.
export function over(dst: RGBA, src: RGBA): void {
  for (let i = 0; i < dst.pix.length; i += 4) {
    const sa = src.pix[i + 3] ?? 0;
    for (let c = 0; c < 4; c++) {
      const d = dst.pix[i + c] ?? 0;
      dst.pix[i + c] = Math.round((src.pix[i + c] ?? 0) + (d * (255 - sa)) / 255);
    }
  }
}

// shrink scales the square src down to size×size with a box filter: each pixel is the average of the source pixels it covers.
export function shrink(src: RGBA, size: number): RGBA {
  const side = src.w;
  const dst = newRGBA(size, size);
  for (let dy = 0; dy < size; dy++) {
    const y0 = Math.trunc((dy * side) / size);
    const y1 = Math.trunc(((dy + 1) * side + size - 1) / size);
    for (let dx = 0; dx < size; dx++) {
      const x0 = Math.trunc((dx * side) / size);
      const x1 = Math.trunc(((dx + 1) * side + size - 1) / size);
      const sum = [0, 0, 0, 0];
      let n = 0;
      for (let y = y0; y < y1; y++) {
        for (let x = x0; x < x1; x++) {
          const i = (y * side + x) * 4;
          for (let c = 0; c < 4; c++) {
            sum[c] = (sum[c] ?? 0) + (src.pix[i + c] ?? 0);
          }
          n++;
        }
      }
      const o = (dy * size + dx) * 4;
      for (let c = 0; c < 4; c++) {
        dst.pix[o + c] = Math.trunc(((sum[c] ?? 0) + Math.trunc(n / 2)) / n);
      }
    }
  }
  return dst;
}

// encodeICO writes PNG images into an ICO file: a 6-byte header, a 16-byte directory entry per image, then the images.
export function encodeICO(images: { size: number; png: Buffer }[]): Buffer {
  const head = Buffer.alloc(6 + 16 * images.length);
  head.writeUInt16LE(0, 0);
  head.writeUInt16LE(1, 2);
  head.writeUInt16LE(images.length, 4);
  let offset = head.length;
  images.forEach((img, i) => {
    const e = 6 + 16 * i;
    const dim = img.size >= 256 ? 0 : img.size;
    head.writeUInt8(dim, e);
    head.writeUInt8(dim, e + 1);
    head.writeUInt16LE(1, e + 4);
    head.writeUInt16LE(32, e + 6);
    head.writeUInt32LE(img.png.length, e + 8);
    head.writeUInt32LE(offset, e + 12);
    offset += img.png.length;
  });
  return Buffer.concat([head, ...images.map((i) => i.png)]);
}

// writeIcons makes the icons from the app icon at iconPath into out.
export function writeIcons(iconPath: string, out: string): void {
  let decoded: PNG;
  try {
    decoded = PNG.sync.read(readFileSync(iconPath));
  } catch (err) {
    throw new Error(`${iconPath}: ${(err as Error).message}`, { cause: err });
  }
  const square = opaqueSquare(premultiply(decoded));
  writeFileSync(join(out, "logo.png"), encodePNG(shrink(square, logoSize)));
  const ico = [16, 32].map((size) => {
    const png = encodePNG(shrink(square, size));
    writeFileSync(join(out, `favicon-${size}.png`), png);
    return { size, png };
  });
  writeFileSync(join(out, "favicon.ico"), encodeICO(ico));
  for (const size of manifestIconSizes) {
    writeFileSync(join(out, `icon-${size}.png`), encodePNG(shrink(square, size)));
  }
  const filled = gradient(square.w);
  over(filled, square);
  writeFileSync(join(out, "apple-touch-icon.png"), encodePNG(shrink(filled, 180)));
}

// pngSize is the width and height of a PNG file, read from its header.
export function pngSize(path: string): { w: number; h: number } {
  const data = readFileSync(path);
  if (data.length < 24 || data.toString("latin1", 12, 16) !== "IHDR") {
    throw new Error(`${path} is not a PNG file`);
  }
  return { w: data.readUInt32BE(16), h: data.readUInt32BE(20) };
}
