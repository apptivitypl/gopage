package main

import (
	"strings"
	"testing"
	"time"
)

func TestTheBannerCountsWhatItServes(t *testing.T) {
	var out strings.Builder
	p := &printer{writer: &out}
	about := summary{
		pages:   []string{"/", "/about"},
		api:     []string{"/api/health", "/api/stories"},
		islands: []string{"Stars", "Stories", "Ticker"},
		width:   120,
	}
	p.banner("http://localhost:3000/", "http://10.0.0.2:3000/", 1950*time.Millisecond, about)
	text := out.String()
	for _, want := range []string{
		"gopage  ready in 1.95s",
		"local   http://localhost:3000/",
		"network http://10.0.0.2:3000/",
		"pages   2  /  /about",
		"api     2  /api/health  /api/stories",
		"islands 3  Stars  Stories  Ticker",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("banner = %q, want %q", text, want)
		}
	}
	if strings.Contains(text, "\x1b[") {
		t.Error("no colour without a terminal")
	}
	var found bool
	for _, line := range taglines {
		found = found || strings.Contains(text, line)
	}
	if !found {
		t.Errorf("banner = %q, want one of the taglines", text)
	}
}

func TestAGroupWithNothingInItIsNotPrinted(t *testing.T) {
	var out strings.Builder
	p := &printer{writer: &out}
	p.banner("http://localhost:3000/", "", time.Second, summary{pages: []string{"/"}, width: 120})
	text := out.String()
	if !strings.Contains(text, "pages   1  /") {
		t.Errorf("banner = %q", text)
	}
	if strings.Contains(text, "api") || strings.Contains(text, "islands") {
		t.Errorf("banner = %q, want empty groups left out", text)
	}
}

func TestQuietDropsTheTaglineAndTheCounts(t *testing.T) {
	var out strings.Builder
	p := &printer{writer: &out}
	p.banner("http://localhost:3000/", "", time.Second, summary{pages: []string{"/"}, quiet: true})
	text := out.String()
	if strings.Contains(text, "pages") || strings.Contains(text, "network") {
		t.Errorf("banner = %q", text)
	}
	for _, line := range taglines {
		if strings.Contains(text, line) {
			t.Errorf("banner = %q, want no tagline when quiet", text)
		}
	}
}

func TestALongGroupIsCutToTheTerminal(t *testing.T) {
	about := summary{width: 40}
	long := []string{"/one/very/long/route/pattern", "/another/very/long/route/pattern"}
	if line := about.fit(long, 1); !strings.HasSuffix(line, "…") || len(line) > 40 {
		t.Errorf("line = %q", line)
	}
	if line := (summary{}).fit([]string{"/"}, 1); line != "/" {
		t.Errorf("line = %q, want a short list left alone at the default width", line)
	}
	if got := (summary{}).groups(); got != nil {
		t.Errorf("groups = %v, want nothing to print for an empty build", got)
	}
}

func TestEveryStartPicksATagline(t *testing.T) {
	for index := range taglines {
		at := time.Unix(0, int64(index)*int64(time.Millisecond))
		if got := tagline(at); got != taglines[index] {
			t.Errorf("tagline(%d) = %q", index, got)
		}
	}
	if strings.Contains(strings.Join(taglines, " "), "no node was started") {
		t.Error("that line was cut")
	}
}

func TestScaffoldStepsAndChildOutputAreTagged(t *testing.T) {
	var out strings.Builder
	p := &printer{writer: &out}
	p.step("created", "demo  from the hello-world template")
	p.step("installed", "the browser packages with pnpm")
	if _, err := p.tagged("pnpm").Write([]byte("Packages: +6\nDone in 784ms\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	p.hint("cd demo && gopage dev")
	text := out.String()
	for _, want := range []string{
		"• created  demo  from the hello-world template",
		"• installed  the browser packages with pnpm",
		"  pnpm  │ Packages: +6",
		"  pnpm  │ Done in 784ms",
		"next cd demo && gopage dev",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output = %q, want %q", text, want)
		}
	}
	if strings.Contains(text, "\x1b[") {
		t.Error("no colour without a terminal")
	}
	if got := pad("go"); got != "go   " {
		t.Errorf("pad = %q, want the tag column aligned", got)
	}
}

func TestBlankChildLinesAreNotPrefixed(t *testing.T) {
	var out strings.Builder
	p := &printer{writer: &out}
	if _, err := p.tagged("pnpm").Write([]byte("+ react 19.2.8\n\n   \n\x1b[0m\ndone\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	lines := strings.Count(out.String(), "\n")
	if lines != 2 {
		t.Errorf("output = %q, want only the two lines that carry text", out.String())
	}
	if !strings.Contains(out.String(), "+ react 19.2.8") || !strings.Contains(out.String(), "done") {
		t.Errorf("output = %q", out.String())
	}
}
