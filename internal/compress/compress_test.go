package compress

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
)

func TestCompressibleTypes(t *testing.T) {
	big := MinCompressible + 1
	yes := []string{"text/css; charset=utf-8", "text/html", "application/javascript", "application/json", "image/svg+xml"}
	for _, kind := range yes {
		if !Compressible(kind, big) {
			t.Errorf("%s must be compressed", kind)
		}
	}
	no := []string{"image/png", "font/woff2", "application/octet-stream"}
	for _, kind := range no {
		if Compressible(kind, big) {
			t.Errorf("%s must be left alone", kind)
		}
	}
	if Compressible("text/css", MinCompressible-1) {
		t.Error("a small file is not worth compressing")
	}
}

func TestBrotliRoundTrip(t *testing.T) {
	source := []byte(strings.Repeat("body{margin:0}", 100))
	var out bytes.Buffer
	if err := Brotli(&out, source); err != nil {
		t.Fatalf("Brotli: %v", err)
	}
	packed := out.Bytes()
	if len(packed) >= len(source) {
		t.Errorf("compressed %d bytes into %d", len(source), len(packed))
	}
	back, err := io.ReadAll(brotli.NewReader(bytes.NewReader(packed)))
	if err != nil || !bytes.Equal(back, source) {
		t.Errorf("round trip failed: %v", err)
	}
}

func TestGzipRoundTrip(t *testing.T) {
	source := []byte(strings.Repeat("body{margin:0}", 100))
	var out bytes.Buffer
	if err := Gzip(&out, source); err != nil {
		t.Fatalf("Gzip: %v", err)
	}
	packed := out.Bytes()
	if len(packed) >= len(source) {
		t.Errorf("compressed %d bytes into %d", len(source), len(packed))
	}
	reader, err := gzip.NewReader(bytes.NewReader(packed))
	if err != nil {
		t.Fatal(err)
	}
	back, err := io.ReadAll(reader)
	if err != nil || !bytes.Equal(back, source) {
		t.Errorf("round trip failed: %v", err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestCompressorsReportAFailingSink(t *testing.T) {
	source := []byte(strings.Repeat("body{margin:0}", 200000))
	if err := Brotli(failingWriter{}, source); err == nil {
		t.Error("brotli must report a sink that cannot take the bytes")
	}
	if err := Gzip(failingWriter{}, source); err == nil {
		t.Error("gzip must report a sink that cannot take the bytes")
	}
}

func TestFastCompressesWhatBrotliCanRead(t *testing.T) {
	source := bytes.Repeat([]byte("rill renders pages from a compiled plan. "), 40)
	var packed bytes.Buffer
	if err := Fast(&packed, source); err != nil {
		t.Fatalf("Fast: %v", err)
	}
	if packed.Len() >= len(source) {
		t.Errorf("packed %d bytes from %d, want it smaller", packed.Len(), len(source))
	}
	read, err := io.ReadAll(brotli.NewReader(&packed))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(read, source) {
		t.Error("the bytes did not survive the round trip")
	}
}

func benchPage() []byte {
	var b strings.Builder
	b.WriteString("<!doctype html><html><body>")
	for i := range 200 {
		fmt.Fprintf(&b, `<p class="row row-%d">a paragraph of body text number %d</p>`, i, i)
	}
	b.WriteString("</body></html>")
	return []byte(b.String())
}

func BenchmarkFast(b *testing.B) {
	page := benchPage()
	b.ReportAllocs()
	b.SetBytes(int64(len(page)))
	for b.Loop() {
		if err := Fast(io.Discard, page); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGzip(b *testing.B) {
	page := benchPage()
	b.ReportAllocs()
	b.SetBytes(int64(len(page)))
	for b.Loop() {
		if err := Gzip(io.Discard, page); err != nil {
			b.Fatal(err)
		}
	}
}

func TestStreamableRefusesEventStreams(t *testing.T) {
	if Streamable(EventType) || Streamable(EventType+"; charset=utf-8") {
		t.Error("server-sent events must never be compressed")
	}
	if !Streamable("text/html; charset=utf-8") || !Streamable("application/json") {
		t.Error("a streamed document must still compress")
	}
	if Streamable("image/png") {
		t.Error("an already compressed type must be left alone")
	}
}

func TestAStreamRoundTripsThroughEachCoding(t *testing.T) {
	for _, coding := range []string{CodingBrotli, CodingGzip} {
		var sink bytes.Buffer
		stream := Open(coding, &sink)
		if _, err := stream.Write([]byte("first ")); err != nil {
			t.Fatalf("%s write: %v", coding, err)
		}
		if err := stream.Flush(); err != nil {
			t.Fatalf("%s flush: %v", coding, err)
		}
		if sink.Len() == 0 {
			t.Errorf("%s: flush emitted nothing", coding)
		}
		if _, err := stream.Write([]byte("second")); err != nil {
			t.Fatalf("%s write: %v", coding, err)
		}
		if err := stream.Close(); err != nil {
			t.Fatalf("%s close: %v", coding, err)
		}
		if got := unpack(t, coding, sink.Bytes()); got != "first second" {
			t.Errorf("%s round trip = %q", coding, got)
		}
	}
}

func TestAPooledStreamIsCleanForTheNextUse(t *testing.T) {
	for range 3 {
		var sink bytes.Buffer
		stream := Open(CodingBrotli, &sink)
		if _, err := stream.Write([]byte("only this")); err != nil {
			t.Fatal(err)
		}
		if err := stream.Close(); err != nil {
			t.Fatal(err)
		}
		if got := unpack(t, CodingBrotli, sink.Bytes()); got != "only this" {
			t.Fatalf("round trip = %q, want no bytes carried over", got)
		}
	}
}

func unpack(t *testing.T, coding string, packed []byte) string {
	t.Helper()
	if coding == CodingGzip {
		reader, err := gzip.NewReader(bytes.NewReader(packed))
		if err != nil {
			t.Fatalf("gzip reader: %v", err)
		}
		out, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("gzip read: %v", err)
		}
		return string(out)
	}
	out, err := io.ReadAll(brotli.NewReader(bytes.NewReader(packed)))
	if err != nil {
		t.Fatalf("brotli read: %v", err)
	}
	return string(out)
}
