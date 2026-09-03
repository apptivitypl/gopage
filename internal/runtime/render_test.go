package runtime

import (
	"testing"

	"github.com/apptivitypl/rill/internal/ir"
)

func TestTheLocaleRootCarriesTagAndDirection(t *testing.T) {
	locale := Locale{Tag: "pl", Prefix: "/pl"}
	props := WithLocale(Empty{}, locale)
	cases := map[string]string{"Tag": "pl", "Dir": "ltr", "Prefix": "/pl"}
	for field, want := range cases {
		value, ok := props.Get([]string{"locale", field})
		if !ok || value.Text() != want {
			t.Errorf("locale.%s = %q (ok=%v), want %q", field, value.Text(), ok, want)
		}
	}
	if value, ok := props.Get([]string{"locale", "Default"}); !ok || value.Text() != "false" {
		t.Errorf("locale.Default = %q", value.Text())
	}
	if _, ok := props.Get([]string{"locale", "Unknown"}); ok {
		t.Error("an unknown locale field must not resolve")
	}
	if _, ok := props.Get([]string{"locale", "Tag", "Extra"}); ok {
		t.Error("the locale root is one level deep")
	}
}

func TestRightToLeftLocalesReportTheirDirection(t *testing.T) {
	cases := map[string]string{"ar": "rtl", "he-IL": "rtl", "fa_IR": "rtl", "en": "ltr", "pl-PL": "ltr"}
	for tag, want := range cases {
		if got := (Locale{Tag: tag}).Dir(); got != want {
			t.Errorf("Dir(%q) = %q, want %q", tag, got, want)
		}
	}
}

func TestThePreloadOpWritesWhatTheServerDecided(t *testing.T) {
	plan := &ir.Plan{
		Ops:  []ir.Op{{Kind: ir.OpStatic, A: 0, B: 6}, {Kind: ir.OpPreload}, {Kind: ir.OpStatic, A: 6, B: 7}},
		Blob: []byte("<head></head>"),
	}
	out := Acquire(64)
	defer Release(out)
	link := `<link rel="modulepreload" href="/assets/island.R.js">`
	if err := RenderOptions([]*ir.Plan{plan}, Empty{}, out, Options{Preload: link}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := out.String(); got != "<head>"+link+"</head>" {
		t.Errorf("html = %q", got)
	}
	out.Reset()
	if err := RenderOptions([]*ir.Plan{plan}, Empty{}, out, Options{}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := out.String(); got != "<head></head>" {
		t.Errorf("html = %q, want nothing written without a preload list", got)
	}
}
