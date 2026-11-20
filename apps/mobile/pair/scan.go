package pair

import (
	"bytes"
	"errors"
	"image"
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
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", err
	}
	// TRY_HARDER looks past a photo's glare, tilt and the screen's own pixels, which a code printed flat on paper does not need.
	result, err := qrcode.NewQRCodeReader().Decode(bmp, map[gozxing.DecodeHintType]interface{}{gozxing.DecodeHintType_TRY_HARDER: true})
	if err != nil {
		return "", ErrNoCode
	}
	return result.GetText(), nil
}
