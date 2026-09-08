package image

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
)

const (
	FormatJPEG     = "jpeg"
	FormatPNG      = "png"
	FormatGIF      = "gif"
	MaxWidth       = 4096
	MaxPixels      = 40 << 20
	MaxSourceSize  = 24 << 20
	DefaultQuality = 75
)

var (
	ErrTooLarge    = errors.New("the source image is larger than the decoder accepts")
	ErrUnsupported = errors.New("the source is not a jpeg, png or gif")
)

type Options struct {
	Width   int
	Quality int
	Format  string
}

type Encoder func(w io.Writer, picture image.Image, quality int) error

var encoders = map[string]Encoder{
	FormatJPEG: func(w io.Writer, picture image.Image, quality int) error {
		return jpeg.Encode(w, picture, &jpeg.Options{Quality: quality})
	},
	FormatPNG: func(w io.Writer, picture image.Image, _ int) error {
		return png.Encode(w, picture)
	},
	FormatGIF: func(w io.Writer, picture image.Image, _ int) error {
		return gif.Encode(w, picture, nil)
	},
}

func Formats() []string {
	return []string{FormatJPEG, FormatPNG, FormatGIF}
}

func Known(format string) bool {
	_, ok := encoders[format]
	return ok
}

func ContentType(format string) string {
	return "image/" + format
}

func Transform(source []byte, opts Options, extra map[string]Encoder) ([]byte, string, error) {
	if len(source) > MaxSourceSize {
		return nil, "", ErrTooLarge
	}
	decoded, kind, err := image.Decode(bytes.NewReader(source))
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrUnsupported, err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx()*bounds.Dy() > MaxPixels {
		return nil, "", ErrTooLarge
	}
	format := opts.Format
	if format == "" {
		format = kind
	}
	encode, ok := encoderFor(format, extra)
	if !ok {
		format = FormatJPEG
		encode = encoders[FormatJPEG]
	}
	quality := opts.Quality
	if quality <= 0 || quality > 100 {
		quality = DefaultQuality
	}
	var out bytes.Buffer
	if err := encode(&out, Resize(decoded, opts.Width), quality); err != nil {
		return nil, "", err
	}
	return out.Bytes(), format, nil
}

func encoderFor(format string, extra map[string]Encoder) (Encoder, bool) {
	if encode, ok := extra[format]; ok {
		return encode, true
	}
	encode, ok := encoders[format]
	return encode, ok
}

type Support struct {
	Encoders map[string]Encoder
}

func (s Support) Transform(source []byte, width, quality int, format string) ([]byte, string, error) {
	body, kind, err := Transform(source, Options{Width: width, Quality: quality, Format: format}, s.Encoders)
	if err != nil {
		return nil, "", err
	}
	return body, ContentType(kind), nil
}

func (s Support) Knows(format string) bool {
	return Known(format) || s.Encoders[format] != nil
}

func (s Support) Limits() (int64, int) {
	return MaxSourceSize, MaxWidth
}
