package og

import (
	"github.com/apptivitypl/gopage/internal/og"
)

type Card = og.Card

func Render(card Card) ([]byte, error) {
	return og.Render(card)
}
