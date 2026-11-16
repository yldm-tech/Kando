// Package browserext is Kando's side of its browser extension (resources/browser-extension): the native messaging host the browser starts, which is Kando's own executable started with the browser's arguments (IsHost) and relays the extension's requests to the running app over its socket (package remote), and Settings › 通用 › 浏览器扩展, which registers that host with the browsers on the computer (Connect) and gives out the extension's folder (Install). It imports no area of the window.
package browserext

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// The native messaging protocol, as Chrome (https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#native-messaging-host-protocol) and Firefox (https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/Native_messaging#app_side) speak it on the host's standard input and output: each message is JSON in UTF-8, after its length in bytes as an unsigned 32-bit integer in the machine's byte order.

// MaxMessage is the longest message the host reads or writes: what the browsers take from a host at most, 1 MB. The extension's requests are far shorter, as they carry at most MaxAsk links of at most MaxVideoURL bytes (browse), so a longer one is not the extension's.
const MaxMessage = 1 << 20

// ErrTooLong is the error of a message longer than MaxMessage.
var ErrTooLong = errors.New("native message longer than 1 MB")

// ReadMessage reads one message from r. It returns io.EOF when r ends before a message starts, as when the browser closes the port, and io.ErrUnexpectedEOF when it ends inside one.
func ReadMessage(r io.Reader) ([]byte, error) {
	var size [4]byte
	if _, err := io.ReadFull(r, size[:]); err != nil {
		return nil, err
	}
	n := binary.NativeEndian.Uint32(size[:])
	if n > MaxMessage {
		return nil, fmt.Errorf("%w: %d bytes", ErrTooLong, n)
	}
	data := make([]byte, n)
	if _, err := io.ReadFull(r, data); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	return data, nil
}

// WriteMessage writes data, one JSON message, to w with its length before it, in one write.
func WriteMessage(w io.Writer, data []byte) error {
	if len(data) > MaxMessage {
		return fmt.Errorf("%w: %d bytes", ErrTooLong, len(data))
	}
	out := make([]byte, 4+len(data))
	binary.NativeEndian.PutUint32(out, uint32(len(data)))
	copy(out[4:], data)
	_, err := w.Write(out)
	return err
}
