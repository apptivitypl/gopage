package image

import (
	"bytes"
	"errors"
	"image"
	stdcolor "image/color"
	"image/jpeg"
	"image/png"
	"io"
	"testing"
)

func photo(t *testing.T, width, height int) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			picture.SetRGBA(x, y, stdcolor.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 40, A: 255})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, picture, nil); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return out.Bytes()
}

func decode(t *testing.T, data []byte) image.Image {
	t.Helper()
	picture, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return picture
}

func TestAnImageIsScaledToTheAskedWidth(t *testing.T) {
	out, format, err := Transform(photo(t, 400, 200), Options{Width: 100, Quality: 70}, nil)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if format != FormatJPEG {
		t.Errorf("format = %q", format)
	}
	if bounds := decode(t, out).Bounds(); bounds.Dx() != 100 || bounds.Dy() != 50 {
		t.Errorf("bounds = %v, want the aspect ratio kept", bounds)
	}
}

func TestALowerQualityMakesASmallerFile(t *testing.T) {
	source := photo(t, 400, 200)
	high, _, err := Transform(source, Options{Width: 200, Quality: 95}, nil)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	low, _, err := Transform(source, Options{Width: 200, Quality: 30}, nil)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if len(low) >= len(high) {
		t.Errorf("low = %d bytes, high = %d bytes", len(low), len(high))
	}
}

func TestAnImageIsNeverScaledUp(t *testing.T) {
	out, _, err := Transform(photo(t, 100, 50), Options{Width: 400}, nil)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if bounds := decode(t, out).Bounds(); bounds.Dx() != 100 {
		t.Errorf("bounds = %v, want the source width", bounds)
	}
}

func TestTheFormatFollowsTheRequest(t *testing.T) {
	out, format, err := Transform(photo(t, 100, 50), Options{Width: 50, Format: FormatPNG}, nil)
	if err != nil || format != FormatPNG {
		t.Fatalf("format = %q, err = %v", format, err)
	}
	if _, err := png.Decode(bytes.NewReader(out)); err != nil {
		t.Errorf("png: %v", err)
	}
	if _, format, _ := Transform(photo(t, 100, 50), Options{Format: "tiff"}, nil); format != FormatJPEG {
		t.Errorf("an unknown format falls back to jpeg: %q", format)
	}
}

func TestAnAppMayPlugItsOwnEncoder(t *testing.T) {
	called := false
	extra := map[string]Encoder{"webp": func(w io.Writer, _ image.Image, _ int) error {
		called = true
		_, err := w.Write([]byte("RIFF"))
		return err
	}}
	out, format, err := Transform(photo(t, 100, 50), Options{Width: 50, Format: "webp"}, extra)
	if err != nil || !called || format != "webp" || string(out) != "RIFF" {
		t.Errorf("out = %q, format = %q, err = %v", out, format, err)
	}
}

func TestNonsenseIsRefused(t *testing.T) {
	if _, _, err := Transform([]byte("not an image"), Options{Width: 10}, nil); err == nil {
		t.Error("a broken source was accepted")
	}
	if _, _, err := Transform(make([]byte, MaxSourceSize+1), Options{}, nil); !errors.Is(err, ErrTooLarge) {
		t.Error("an oversized source was accepted")
	}
}

func TestTheFormatTableIsExported(t *testing.T) {
	if len(Formats()) != 3 || !Known(FormatGIF) || Known("tiff") {
		t.Errorf("formats = %v", Formats())
	}
	if ContentType(FormatPNG) != "image/png" {
		t.Errorf("content type = %q", ContentType(FormatPNG))
	}
}

func TestExtremeScalesStayInsideTheImage(t *testing.T) {
	out, _, err := Transform(photo(t, 300, 300), Options{Width: 1}, nil)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if bounds := decode(t, out).Bounds(); bounds.Dx() != 1 || bounds.Dy() != 1 {
		t.Errorf("bounds = %v", bounds)
	}
	wide, _, err := Transform(photo(t, 400, 3), Options{Width: 2}, nil)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if bounds := decode(t, wide).Bounds(); bounds.Dy() < 1 {
		t.Errorf("bounds = %v, want at least one row", bounds)
	}
}

func TestAnEmptySpanYieldsNothing(t *testing.T) {
	if got := average(image.NewRGBA(image.Rect(0, 0, 2, 2)), image.Point{}, 1, 1, 1, 1); got.A != 0 {
		t.Errorf("colour = %+v", got)
	}
	if start, end := span(3, 0.1, 1); start != 0 || end != 1 {
		t.Errorf("span = %d..%d", start, end)
	}
}

func TestAQualityOutsideTheRangeFallsBack(t *testing.T) {
	for _, quality := range []int{0, -5, 200} {
		if _, _, err := Transform(photo(t, 40, 20), Options{Width: 20, Quality: quality}, nil); err != nil {
			t.Errorf("quality %d: %v", quality, err)
		}
	}
}

func TestAnEncoderThatFailsIsReported(t *testing.T) {
	extra := map[string]Encoder{"webp": func(io.Writer, image.Image, int) error { return io.ErrClosedPipe }}
	if _, _, err := Transform(photo(t, 40, 20), Options{Format: "webp"}, extra); err == nil {
		t.Error("the encoder error was swallowed")
	}
}

func TestSupportAdaptsTheCodecForTheServer(t *testing.T) {
	held := Support{}
	source, width := held.Limits()
	if source != MaxSourceSize || width != MaxWidth {
		t.Errorf("limits = %d, %d", source, width)
	}
	if !held.Knows("png") || held.Knows("webp") {
		t.Error("only the built-in formats are known without an encoder")
	}
	plugged := Support{Encoders: map[string]Encoder{"webp": func(io.Writer, image.Image, int) error { return nil }}}
	if !plugged.Knows("webp") {
		t.Error("a plugged encoder makes its format known")
	}
	body, kind, err := held.Transform(photo(t, 200, 100), 100, 80, "png")
	if err != nil || kind != "image/png" || len(body) == 0 {
		t.Errorf("transform = %d bytes, %q, err = %v", len(body), kind, err)
	}
	if _, _, err := held.Transform([]byte("not an image"), 10, 80, ""); err == nil {
		t.Error("a source that is no image must fail")
	}
}
