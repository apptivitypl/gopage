package bundle

import (
	"bytes"
	"strings"
	"testing"
)

func TestCSSIsMinified(t *testing.T) {
	source := []byte(`
:root {
    /* a comment */
    --ink: #16181d;
}

body {
    margin: 0;
    color: var(--ink);
}
`)
	out, err := MinifyCSS(source)
	if err != nil {
		t.Fatalf("MinifyCSS: %v", err)
	}
	got := string(out)
	if strings.Contains(got, "a comment") || strings.Contains(got, "\n") {
		t.Errorf("css = %q", got)
	}
	if !strings.Contains(got, "--ink:#16181d") || !strings.Contains(got, "margin:0") {
		t.Errorf("css = %q", got)
	}
	if len(out) >= len(source) {
		t.Errorf("minified %d bytes into %d", len(source), len(out))
	}
}

func TestMinifyingTwiceChangesNothing(t *testing.T) {
	once, err := MinifyCSS([]byte("body { margin: 0; }"))
	if err != nil {
		t.Fatal(err)
	}
	twice, err := MinifyCSS(once)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(once, twice) {
		t.Errorf("first = %q, second = %q", once, twice)
	}
}
