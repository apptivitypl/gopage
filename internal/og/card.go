package og

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	DefaultWidth  = 1200
	DefaultHeight = 630
	margin        = 80
	titleSize     = 64
	subtitleSize  = 32
	badgeSize     = 26
	maxTitleLines = 4
)

type Card struct {
	Title      string
	Subtitle   string
	Badge      string
	Width      int
	Height     int
	Background color.Color
	Foreground color.Color
	Accent     color.Color
}

func Render(card Card) ([]byte, error) {
	card = filled(card)
	canvas := image.NewRGBA(image.Rect(0, 0, card.Width, card.Height))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(card.Background), image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(0, 0, card.Width, 12), image.NewUniform(card.Accent), image.Point{}, draw.Src)

	faces, err := typefaces()
	if err != nil {
		return nil, err
	}
	if card.Badge != "" {
		write(canvas, faces.small, card.Accent, margin, margin+badgeSize, card.Badge)
	}
	baseline := margin + 2*titleSize
	for _, line := range wrap(card.Title, faces.bold, card.Width-2*margin) {
		write(canvas, faces.bold, card.Foreground, margin, baseline, line)
		baseline += titleSize + 16
	}
	if card.Subtitle != "" {
		write(canvas, faces.plain, card.Foreground, margin, card.Height-margin, card.Subtitle)
	}
	var out bytes.Buffer
	if err := png.Encode(&out, canvas); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

type faces struct {
	bold  font.Face
	plain font.Face
	small font.Face
}

var typefaces = sync.OnceValues(func() (faces, error) {
	bold, err := face(gobold.TTF, titleSize)
	if err != nil {
		return faces{}, err
	}
	plain, err := face(goregular.TTF, subtitleSize)
	if err != nil {
		return faces{}, err
	}
	small, err := face(goregular.TTF, badgeSize)
	if err != nil {
		return faces{}, err
	}
	return faces{bold: bold, plain: plain, small: small}, nil
})

func filled(card Card) Card {
	if card.Width <= 0 {
		card.Width = DefaultWidth
	}
	if card.Height <= 0 {
		card.Height = DefaultHeight
	}
	if card.Background == nil {
		card.Background = color.RGBA{R: 12, G: 14, B: 20, A: 255}
	}
	if card.Foreground == nil {
		card.Foreground = color.RGBA{R: 245, G: 246, B: 250, A: 255}
	}
	if card.Accent == nil {
		card.Accent = color.RGBA{R: 99, G: 102, B: 241, A: 255}
	}
	return card
}

func face(data []byte, size float64) (font.Face, error) {
	parsed, err := opentype.Parse(data)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(parsed, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
}

func write(canvas draw.Image, chosen font.Face, ink color.Color, x, y int, text string) {
	drawer := font.Drawer{
		Dst:  canvas,
		Src:  image.NewUniform(ink),
		Face: chosen,
		Dot:  fixed.P(x, y),
	}
	drawer.DrawString(text)
}

func wrap(text string, chosen font.Face, width int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	line := words[0]
	for _, word := range words[1:] {
		candidate := line + " " + word
		if font.MeasureString(chosen, candidate).Ceil() > width {
			lines = append(lines, line)
			if len(lines) == maxTitleLines {
				return lines
			}
			line = word
			continue
		}
		line = candidate
	}
	return append(lines, line)
}
