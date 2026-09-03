package bundle

import (
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
)

const CSSType = "text/css"

var styles = newStyleMinifier()

func newStyleMinifier() *minify.M {
	engine := minify.New()
	engine.AddFunc(CSSType, css.Minify)
	return engine
}

func MinifyCSS(source []byte) ([]byte, error) {
	return styles.Bytes(CSSType, source)
}
