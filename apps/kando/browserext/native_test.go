package browserext

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

// A message goes out as its length in the machine's byte order and its bytes, and reads back the same, up to the 1 MB the browsers take; a longer one is refused both ways, and a message cut short is an error rather than a message.
func TestNativeMessagesAreFramed(t *testing.T) {
	var buf bytes.Buffer
	msg := []byte(`{"id":1,"method":"browser.state","params":{"urls":["https://youtu.be/jNQXAC9IVRw"]}}`)
	if err := WriteMessage(&buf, msg); err != nil {
		t.Fatal(err)
	}
	if got := binary.NativeEndian.Uint32(buf.Bytes()[:4]); got != uint32(len(msg)) || !bytes.Equal(buf.Bytes()[4:], msg) {
		t.Fatalf("framed %q", buf.Bytes())
	}
	back, err := ReadMessage(&buf)
	if err != nil || !bytes.Equal(back, msg) {
		t.Fatalf("read %q, %v", back, err)
	}
	if _, err := ReadMessage(&buf); !errors.Is(err, io.EOF) {
		t.Fatalf("after the last message: %v, want EOF", err)
	}

	// A message of exactly MaxMessage bytes goes through.
	full := []byte(`"` + strings.Repeat("a", MaxMessage-2) + `"`)
	buf.Reset()
	if err := WriteMessage(&buf, full); err != nil {
		t.Fatal(err)
	}
	if back, err := ReadMessage(&buf); err != nil || len(back) != MaxMessage {
		t.Fatalf("a message at the limit: %d bytes, %v", len(back), err)
	}
	if err := WriteMessage(&buf, append(full, ' ')); !errors.Is(err, ErrTooLong) {
		t.Fatalf("writing one byte over the limit: %v", err)
	}

	// A length over the limit is refused before anything is read for it, as from a browser that is not one.
	var size [4]byte
	binary.NativeEndian.PutUint32(size[:], MaxMessage+1)
	if _, err := ReadMessage(bytes.NewReader(append(size[:], "{}"...))); !errors.Is(err, ErrTooLong) {
		t.Fatalf("a length over the limit: %v", err)
	}
	binary.NativeEndian.PutUint32(size[:], 10)
	if _, err := ReadMessage(bytes.NewReader(append(size[:], "{}"...))); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("a message cut short: %v", err)
	}
	if _, err := ReadMessage(bytes.NewReader(size[:2])); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("a length cut short: %v", err)
	}
}
