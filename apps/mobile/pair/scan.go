package pair

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	_ "image/jpeg"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

// ErrNoCode is ReadCode's error for a photo with no QR code it can read.
var ErrNoCode = errors.New("no QR code in the photo")

// photoSize is the longer side a photo of the computer's screen is scaled to before it is read: the pairing code's modules are still several pixels across at it, and a phone's full-size photo would take far longer to read.
const photoSize = 1600

// ReadCode is the text of the QR code in photo, a JPEG as the camera took it, such as the kando://pair link the computer's Settings › Phone Sync shows.
func ReadCode(photo []byte) (string, error) {
	img, _, err := image.Decode(bytes.NewReader(photo))
	if err != nil {
		return "", err
	}
	// A code the photo shows clearly is still missed now and then, about one in a hundred, where the JPEG's noise breaks a finder pattern's edges; at half the size, each pixel the average of four, the noise is gone and the code is read.
	for _, try := range []image.Image{img, halved(img)} {
		bmp, err := gozxing.NewBinaryBitmapFromImage(try)
		if err != nil {
			return "", err
		}
		// TRY_HARDER looks past a photo's glare, tilt and the screen's own pixels, which a code printed flat on paper does not need.
		if result, err := qrcode.NewQRCodeReader().Decode(bmp, map[gozxing.DecodeHintType]interface{}{gozxing.DecodeHintType_TRY_HARDER: true}); err == nil {
			return result.GetText(), nil
		}
	}
	return "", ErrNoCode
}

// halved is img at half its width and height in grey, each pixel the average of the four it stands for.
func halved(img image.Image) image.Image {
	b := img.Bounds()
	out := image.NewGray(image.Rect(0, 0, b.Dx()/2, b.Dy()/2))
	for y := 0; y < out.Rect.Dy(); y++ {
		for x := 0; x < out.Rect.Dx(); x++ {
			sum := 0
			for _, d := range [4][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
				sum += int(color.GrayModel.Convert(img.At(b.Min.X+2*x+d[0], b.Min.Y+2*y+d[1])).(color.Gray).Y)
			}
			out.Pix[y*out.Stride+x] = uint8(sum / 4)
		}
	}
	return out
}
