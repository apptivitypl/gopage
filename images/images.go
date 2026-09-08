package images

import (
	"github.com/apptivitypl/gopage"
	"github.com/apptivitypl/gopage/internal/image"
)

type Encoder = image.Encoder

func Support(encoders map[string]Encoder) gopage.ImageSupport {
	return image.Support{Encoders: encoders}
}
