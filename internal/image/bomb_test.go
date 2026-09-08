package image

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"testing"
)

func header(width, height uint32) []byte {
	var out bytes.Buffer
	out.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	var chunk bytes.Buffer
	chunk.WriteString("IHDR")
	_ = binary.Write(&chunk, binary.BigEndian, width)
	_ = binary.Write(&chunk, binary.BigEndian, height)
	chunk.Write([]byte{8, 2, 0, 0, 0})
	_ = binary.Write(&out, binary.BigEndian, uint32(chunk.Len()-4))
	out.Write(chunk.Bytes())
	_ = binary.Write(&out, binary.BigEndian, crc32.ChecksumIEEE(chunk.Bytes()))
	return out.Bytes()
}

func TestAPictureTooBigIsRefusedBeforeItIsDecoded(t *testing.T) {
	_, _, err := Transform(header(30000, 30000), Options{Width: 100}, nil)
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, want the size refused from the header rather than after decoding", err)
	}
}
