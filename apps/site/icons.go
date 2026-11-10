package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

// Background gradient of the app icon's amber glass, from its top edge to its bottom one, sampled from its rim.
var (
	gradientFrom = color.RGBA{0xF3, 0xB0, 0x5E, 0xFF}
	gradientTo   = color.RGBA{0xEC, 0x7E, 0x2E, 0xFF}
)

// logoSize is the side of logo.png, the mark the pages show beside the name: twice the largest size a page draws it at, the 72 pixels of 404.html.
const logoSize = 144

// writeIcons makes the PNG icons from the app icon at iconPath: logo.png, favicon-16.png and favicon-32.png, favicon.ico holding both, and apple-touch-icon.png. The app icon is a rounded square on a transparent margin; the logo and the favicons are that square without the margin, and the touch icon fills the square's corners with its background gradient too, because iOS rounds the corners itself and would show the transparent ones black.
func writeIcons(iconPath, out string) error {
	f, err := os.Open(iconPath)
	if err != nil {
		return err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return fmt.Errorf("%s: %w", iconPath, err)
	}
	square := opaqueSquare(src)

	logo, err := encodePNG(shrink(square, logoSize))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "logo.png"), logo, 0o644); err != nil {
		return err
	}

	var ico []icoImage
	for _, size := range []int{16, 32} {
		data, err := encodePNG(shrink(square, size))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, fmt.Sprintf("favicon-%d.png", size)), data, 0o644); err != nil {
			return err
		}
		ico = append(ico, icoImage{size: size, png: data})
	}
	if err := os.WriteFile(filepath.Join(out, "favicon.ico"), encodeICO(ico), 0o644); err != nil {
		return err
	}

	filled := gradient(square.Bounds().Dx())
	draw.Draw(filled, filled.Bounds(), square, square.Bounds().Min, draw.Over)
	data, err := encodePNG(shrink(filled, 180))
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "apple-touch-icon.png"), data, 0o644)
}

// opaqueSquare is the smallest square around the pixels of img that are not fully transparent, copied into an RGBA image whose origin is its top left corner.
func opaqueSquare(img image.Image) *image.RGBA {
	b := img.Bounds()
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0 {
				minX, minY = min(minX, x), min(minY, y)
				maxX, maxY = max(maxX, x+1), max(maxY, y+1)
			}
		}
	}
	side := max(maxX-minX, maxY-minY)
	cx, cy := (minX+maxX)/2, (minY+maxY)/2
	r := image.Rect(cx-side/2, cy-side/2, cx-side/2+side, cy-side/2+side)
	dst := image.NewRGBA(image.Rect(0, 0, side, side))
	draw.Draw(dst, dst.Bounds(), img, r.Min, draw.Src)
	return dst
}

// gradient is a size×size square of the icon's background gradient, from top to bottom.
func gradient(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	span := float64(size - 1)
	lerp := func(a, b uint8, t float64) uint8 { return uint8(float64(a) + (float64(b)-float64(a))*t + 0.5) }
	for y := range size {
		for x := range size {
			t := float64(y) / span
			img.SetRGBA(x, y, color.RGBA{lerp(gradientFrom.R, gradientTo.R, t), lerp(gradientFrom.G, gradientTo.G, t), lerp(gradientFrom.B, gradientTo.B, t), 0xFF})
		}
	}
	return img
}

// shrink scales the square src down to size×size with a box filter: each pixel is the average of the source pixels it covers. image.RGBA holds premultiplied colors, so averaging them weighs each pixel's color by its opacity.
func shrink(src *image.RGBA, size int) *image.RGBA {
	side := src.Bounds().Dx()
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	for dy := range size {
		y0, y1 := dy*side/size, ((dy+1)*side+size-1)/size
		for dx := range size {
			x0, x1 := dx*side/size, ((dx+1)*side+size-1)/size
			var r, g, b, a, n int
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					i := src.PixOffset(x, y)
					p := src.Pix[i : i+4 : i+4]
					r, g, b, a = r+int(p[0]), g+int(p[1]), b+int(p[2]), a+int(p[3])
					n++
				}
			}
			i := dst.PixOffset(dx, dy)
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = uint8((r+n/2)/n), uint8((g+n/2)/n), uint8((b+n/2)/n), uint8((a+n/2)/n)
		}
	}
	return dst
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// icoImage is one size in an ICO file, stored as PNG, which every browser reads.
type icoImage struct {
	size int
	png  []byte
}

// encodeICO writes images into an ICO file: a 6-byte header, a 16-byte directory entry per image, then the images.
func encodeICO(images []icoImage) []byte {
	var buf bytes.Buffer
	le := binary.LittleEndian
	buf.Write(le.AppendUint16(nil, 0))
	buf.Write(le.AppendUint16(nil, 1))
	buf.Write(le.AppendUint16(nil, uint16(len(images))))
	offset := 6 + 16*len(images)
	for _, img := range images {
		dim := byte(img.size)
		if img.size >= 256 {
			dim = 0
		}
		buf.Write([]byte{dim, dim, 0, 0})
		buf.Write(le.AppendUint16(nil, 1))
		buf.Write(le.AppendUint16(nil, 32))
		buf.Write(le.AppendUint32(nil, uint32(len(img.png))))
		buf.Write(le.AppendUint32(nil, uint32(offset)))
		offset += len(img.png)
	}
	for _, img := range images {
		buf.Write(img.png)
	}
	return buf.Bytes()
}
