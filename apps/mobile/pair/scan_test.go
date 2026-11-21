package pair

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"testing"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
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

// These pairing links' codes, flawless as they are, gozxing reads only a quarter turn round, which made a test that pairs from a photo of a computer's code with a new port and certificate each run fail about one run in a hundred.
func TestReadCodeReadsACodeOnlyReadTurned(t *testing.T) {
	for _, link := range []string{
		"kando://pair?a=127.0.0.1%3A23850&c=123456&fp=y0Bsnkg3alalTVCEV1ubZeD2oOAB_uvT-Fjg0BpHYFk&n=Studio",
		"kando://pair?a=127.0.0.1%3A14263&c=123456&fp=35xwYCHbRpeyXc5bINvqDq320RH8ne_42-3z4xQ1ax4&n=Studio",
		"kando://pair?a=127.0.0.1%3A43324&c=123456&fp=d5sDdUG-cMMHEnwo3A1cjtwRm9HfpoMLsz9OCYjklkk&n=Studio",
		"kando://pair?a=127.0.0.1%3A52533&c=123456&fp=hGb7qSZtyvs_40omjuPBtk5vbDsDy1sZcp3MtqPJSbw&n=Studio",
		"kando://pair?a=127.0.0.1%3A15066&c=123456&fp=dnHzqjMrewH0JYmKKexnL3K7f7BqPxVg4C_eFepsCcw&n=Studio",
	} {
		if got, err := ReadCode(qrPhoto(t, link)); err != nil || got != link {
			t.Errorf("ReadCode = %q, %v, want %q", got, err, link)
		}
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
