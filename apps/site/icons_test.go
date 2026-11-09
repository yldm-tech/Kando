package main

import (
	"image"
	"image/color"
	"testing"
)

// shrink averages the pixels each output pixel covers, opacity included.
func TestShrink(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			if x < 2 {
				src.SetRGBA(x, y, color.RGBA{200, 0, 0, 255})
			}
		}
	}
	got := shrink(src, 2)
	if c := got.RGBAAt(0, 0); c != (color.RGBA{200, 0, 0, 255}) {
		t.Errorf("left half: %v, want opaque red", c)
	}
	if c := got.RGBAAt(1, 1); c != (color.RGBA{}) {
		t.Errorf("right half: %v, want transparent", c)
	}
	half := shrink(src, 1).RGBAAt(0, 0)
	if half != (color.RGBA{100, 0, 0, 128}) {
		t.Errorf("whole: %v, want red at half opacity, premultiplied", half)
	}
}

// opaqueSquare drops the transparent margin around the app icon's rounded square.
func TestOpaqueSquare(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 2; y < 8; y++ {
		for x := 2; x < 8; x++ {
			src.SetRGBA(x, y, color.RGBA{0, 0, 255, 255})
		}
	}
	got := opaqueSquare(src)
	if b := got.Bounds(); b.Dx() != 6 || b.Dy() != 6 {
		t.Fatalf("bounds %v, want 6×6", b)
	}
	if c := got.RGBAAt(0, 0); c.A != 255 {
		t.Errorf("corner %v, want opaque", c)
	}
}
