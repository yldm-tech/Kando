package pair

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"net/netip"
	"testing"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"

	"kando/packages/sync"
)

// qrPhoto is a JPEG of text's QR code, as small on a grey page as a photo of a screen across a desk shows it.
func qrPhoto(t testing.TB, text string) []byte {
	t.Helper()
	code, err := qrcode.NewQRCodeWriter().Encode(text, gozxing.BarcodeFormat_QR_CODE, 300, 300, nil)
	if err != nil {
		t.Fatal(err)
	}
	photo := image.NewRGBA(image.Rect(0, 0, 1200, 900))
	draw.Draw(photo, photo.Bounds(), image.NewUniform(color.Gray{Y: 120}), image.Point{}, draw.Src)
	draw.Draw(photo, image.Rect(450, 300, 750, 600), code, image.Point{}, draw.Src)
	var b bytes.Buffer
	if err := jpeg.Encode(&b, photo, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestReadCodeReadsThePhotosQRCode(t *testing.T) {
	const link = "kando://pair?a=192.168.1.20:47123&c=123456&fp=AAAA&n=Studio"
	if got, err := ReadCode(qrPhoto(t, link)); err != nil || got != link {
		t.Fatalf("ReadCode = %q, %v, want %q", got, err, link)
	}
}

func TestReadCodeSaysWhenThereIsNoCode(t *testing.T) {
	var b bytes.Buffer
	blank := image.NewGray(image.Rect(0, 0, 640, 480))
	draw.Draw(blank, blank.Bounds(), image.NewUniform(color.Gray{Y: 200}), image.Point{}, draw.Src)
	if err := jpeg.Encode(&b, blank, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadCode(b.Bytes()); !errors.Is(err, ErrNoCode) {
		t.Fatalf("ReadCode of a blank photo: %v, want ErrNoCode", err)
	}
	if _, err := ReadCode([]byte("not a jpeg")); err == nil {
		t.Fatal("ReadCode read something that is no picture")
	}
}

// A pairing link whose code JPEG's noise hides at full size, one of about a hundred, is read at half the size.
func TestReadCodeReadsACodeTheNoiseHides(t *testing.T) {
	fp := make([]byte, 32)
	for i := range fp {
		fp[i] = byte(i*7 + 3*31)
	}
	link := sync.PairURL{Addrs: []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:40031")}, Code: "123456", Fingerprint: fp, Name: "Studio"}.String()
	if got, err := ReadCode(qrPhoto(t, link)); err != nil || got != link {
		t.Fatalf("ReadCode = %q, %v, want %q", got, err, link)
	}
}
