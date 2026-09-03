package devserver

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/sonquer/rill/internal/diag"
)

const OverlayTitle = "rill: the project does not compile"

func OverlayFailure(w http.ResponseWriter, message string) {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	fmt.Fprintf(&b, "<title>%s</title>\n<style>%s</style>\n</head>\n<body>\n", OverlayTitle, overlayCSS)
	fmt.Fprintf(&b, "<h1>%s</h1>\n", OverlayTitle)
	fmt.Fprintf(&b, "<pre>%s</pre>\n", escape(message))
	b.WriteString("<p class=\"hint\">the page returns as soon as the build passes</p>\n")
	b.WriteString(ReloadScript)
	b.WriteString("\n</body>\n</html>\n")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte(b.String()))
}

func Overlay(w http.ResponseWriter, diagnostics []diag.Diagnostic, sources map[string]string) {
	body := renderOverlay(diagnostics, sources)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte(body))
}

func renderOverlay(diagnostics []diag.Diagnostic, sources map[string]string) string {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	fmt.Fprintf(&b, "<title>%s</title>\n<style>%s</style>\n</head>\n<body>\n", OverlayTitle, overlayCSS)
	fmt.Fprintf(&b, "<h1>%s</h1>\n", OverlayTitle)
	if len(diagnostics) == 0 {
		b.WriteString("<p>nothing has been built yet</p>\n")
	}
	for _, item := range diagnostics {
		if item.Severity != diag.Error {
			continue
		}
		fmt.Fprintf(&b, "<pre>%s</pre>\n", escape(diag.Render(item, sources[item.File])))
	}
	b.WriteString("<p class=\"hint\">the page returns as soon as the build passes</p>\n")
	b.WriteString(ReloadScript)
	b.WriteString("\n</body>\n</html>\n")
	return b.String()
}

const overlayCSS = `
:root { color-scheme: light dark; }
body { font: 14px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace; margin: 0; padding: 2rem; }
h1 { font-size: 1rem; margin: 0 0 1.5rem; }
pre { background: rgba(127,127,127,.12); border-radius: 8px; margin: 0 0 1rem; overflow-x: auto; padding: 1rem; }
.hint { opacity: .6; }
`

var overlayEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
)

func escape(value string) string {
	return overlayEscaper.Replace(value)
}
