package og

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

func decode(t *testing.T, data []byte) image.Image {
	t.Helper()
	picture, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return picture
}

func TestACardIsAPngOfTheAskedSize(t *testing.T) {
	data, err := Render(Card{Title: "Praca w Warszawie", Subtitle: "12 922 oferty", Badge: "hexjobs"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	bounds := decode(t, data).Bounds()
	if bounds.Dx() != DefaultWidth || bounds.Dy() != DefaultHeight {
		t.Errorf("bounds = %v", bounds)
	}
}

func TestACardHonoursItsOwnSizeAndColours(t *testing.T) {
	data, err := Render(Card{
		Title:      "x",
		Width:      600,
		Height:     300,
		Background: color.RGBA{R: 255, A: 255},
		Foreground: color.White,
		Accent:     color.RGBA{B: 255, A: 255},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	picture := decode(t, data)
	if picture.Bounds().Dx() != 600 || picture.Bounds().Dy() != 300 {
		t.Errorf("bounds = %v", picture.Bounds())
	}
	red, _, _, _ := picture.At(10, 200).RGBA()
	if red>>8 != 255 {
		t.Errorf("background = %v", picture.At(10, 200))
	}
	_, _, blue, _ := picture.At(10, 4).RGBA()
	if blue>>8 != 255 {
		t.Errorf("accent = %v", picture.At(10, 4))
	}
}

func TestALongTitleIsWrappedAndCapped(t *testing.T) {
	chosen, err := face(goregular.TTF, 64)
	if err != nil {
		t.Fatalf("face: %v", err)
	}
	defer func() { _ = chosen.Close() }()
	lines := wrap(strings.Repeat("warszawa ", 40), chosen, 1040)
	if len(lines) != maxTitleLines {
		t.Errorf("lines = %d, want the cap", len(lines))
	}
	if wrap("   ", chosen, 100) != nil {
		t.Error("nothing to wrap")
	}
	if got := wrap("one two", chosen, 10_000); len(got) != 1 {
		t.Errorf("lines = %v", got)
	}
}

func TestABrokenFontIsReported(t *testing.T) {
	if _, err := face([]byte("not a font"), 12); err == nil {
		t.Error("a broken font was accepted")
	}
}

func TestTheTitleIsActuallyDrawn(t *testing.T) {
	blank, err := Render(Card{Title: ""})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	written, err := Render(Card{Title: "gopage"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if bytes.Equal(blank, written) {
		t.Error("the title changed nothing")
	}
}
