package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// testPNG returns bytes that start with the PNG signature, which is all the
// content sniffing needs to recognise them as an image.
func testPNG(body string) []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), body...)
}

// writeTestTaggedMP3 writes an ID3v2.3 tag with a title and an optional front
// cover. The audio payload is not valid MPEG data.
func writeTestTaggedMP3(t *testing.T, path, title string, picture []byte) {
	t.Helper()
	frame := func(id string, payload []byte) []byte {
		header := make([]byte, 10, 10+len(payload))
		copy(header, id)
		binary.BigEndian.PutUint32(header[4:8], uint32(len(payload)))
		return append(header, payload...)
	}
	frames := frame("TIT2", append([]byte{0}, title...))
	if picture != nil {
		apic := append([]byte{0}, "image/png"...)
		apic = append(apic, 0, 3, 0)
		apic = append(apic, picture...)
		frames = append(frames, frame("APIC", apic)...)
	}
	size := len(frames)
	data := []byte{'I', 'D', '3', 3, 0, 0, byte(size >> 21 & 0x7f), byte(size >> 14 & 0x7f), byte(size >> 7 & 0x7f), byte(size & 0x7f)}
	data = append(data, frames...)
	data = append(data, "not mpeg audio"...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
