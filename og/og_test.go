package og_test

import (
	"testing"

	"github.com/apptivitypl/gopage/og"
)

func TestACardIsRendered(t *testing.T) {
	data, err := og.Render(og.Card{Title: "Praca w Warszawie", Subtitle: "12 922 oferty"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(data) < 1000 || string(data[1:4]) != "PNG" {
		t.Errorf("card = %d bytes, header = %q", len(data), data[:8])
	}
}
