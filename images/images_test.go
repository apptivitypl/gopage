package images_test

import (
	"image"
	"io"
	"testing"

	"github.com/apptivitypl/gopage/images"
)

func TestSupportAnswersItsLimitsAndFormats(t *testing.T) {
	held := images.Support(nil)
	source, width := held.Limits()
	if source <= 0 || width <= 0 {
		t.Errorf("limits = %d, %d", source, width)
	}
	if !held.Knows("png") || held.Knows("webp") {
		t.Error("only the built-in formats are known without an encoder")
	}
	plugged := images.Support(map[string]images.Encoder{
		"webp": func(io.Writer, image.Image, int) error { return nil },
	})
	if !plugged.Knows("webp") {
		t.Error("a plugged encoder makes its format known")
	}
}
