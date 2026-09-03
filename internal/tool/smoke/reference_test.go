package smoke

import (
	"github.com/apptivitypl/rill/internal/config"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSubdomainConfigSwitchesTheModeAndAddsHosts(t *testing.T) {
	got, err := SubdomainConfig(`{"i18n": {"mode": "path", "locales": ["en"]}}`)
	if err != nil {
		t.Fatalf("SubdomainConfig: %v", err)
	}
	if strings.Contains(got, `"mode": "path"`) {
		t.Error("path mode survived the switch")
	}
	if !strings.Contains(got, `"mode": "subdomain"`) {
		t.Error("subdomain mode is missing")
	}
	for _, host := range []string{DefaultHost, LocaleHost} {
		if !strings.Contains(got, host) {
			t.Errorf("host %q is missing from %q", host, got)
		}
	}
	if _, err := config.Parse(got); err != nil {
		t.Errorf("the switched config does not parse: %v", err)
	}
}

func TestSubdomainConfigRefusesAConfigThatDoesNotParse(t *testing.T) {
	if _, err := SubdomainConfig("{not json"); err == nil {
		t.Error("a broken config must be reported")
	}
}

func TestFingerprintSkipsGeneratedTreesAndTheConfig(t *testing.T) {
	tree := fstest.MapFS{
		"app/page.rill":            {Data: []byte("<h1>hello</h1>")},
		"rill.jsonc":               {Data: []byte("mode = \"path\"")},
		"dist/server":              {Data: []byte("binary")},
		"internal/gen/config.json": {Data: []byte("copy")},
		".wrangler/s.sqlite":       {Data: []byte("state")},
		"wrangler.log":             {Data: []byte("noise")},
	}
	files, err := Fingerprint(tree)
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("files = %v, want only the template", files)
	}
	if _, ok := files["app/page.rill"]; !ok {
		t.Errorf("files = %v", files)
	}
}

func TestChangedReportsEveryDifference(t *testing.T) {
	before := map[string]string{"a": "1", "b": "2", "gone": "3"}
	after := map[string]string{"a": "1", "b": "9", "new": "4"}
	got := Changed(before, after)
	want := []string{"b", "gone", "new"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Changed = %v, want %v", got, want)
	}
}

func TestChangedOnIdenticalTreesIsEmpty(t *testing.T) {
	tree := map[string]string{"a": "1"}
	if got := Changed(tree, tree); len(got) != 0 {
		t.Errorf("Changed = %v, want none", got)
	}
}

func TestRunSubdomainPassesWhenEveryHostAnswers(t *testing.T) {
	fetch := func(host, url string) (Response, error) {
		body, status := "", http.StatusTeapot
		for _, check := range SubdomainChecks() {
			if check.Host != host || url != check.Path {
				continue
			}
			body += check.Contains + " "
			status = check.Status
		}
		return Response{Status: status, Body: body}, nil
	}
	if err := RunSubdomain(fetch, ""); err != nil {
		t.Fatalf("RunSubdomain: %v", err)
	}
}

func TestRunSubdomainFailsWhenTheLocaleHostServesTheWrongLanguage(t *testing.T) {
	fetch := func(host, url string) (Response, error) {
		if host == LocaleHost {
			return Response{Status: http.StatusOK, Body: "4 listings"}, nil
		}
		return Response{Status: http.StatusOK, Body: "what is on offer 4 listings 14 354.00 PLN per m2"}, nil
	}
	err := RunSubdomain(fetch, "")
	if err == nil || !strings.Contains(err.Error(), LocaleHost) {
		t.Fatalf("err = %v, want a failure naming the locale host", err)
	}
}

func TestRunDevWantsTheReloadScriptAndTheGoRoutes(t *testing.T) {
	answers := map[string]Response{
		"/":           {Status: http.StatusOK, Body: "Rendered in Go. <rill-island " + ReloadPath, ContentType: "text/html"},
		"/about":      {Status: http.StatusOK, Body: "<title>what happened to this request</title>", ContentType: "text/html"},
		"/api/health": {Status: http.StatusOK, Body: `{"runtime":"go"}`, ContentType: "application/json"},
		"/nope":       {Status: http.StatusNotFound, Body: "<title>no route answers this address</title>"},
		"/assets/":    {Status: http.StatusNotFound},
	}
	fetch := func(url string) (Response, error) { return answers[url], nil }
	if err := RunDev(fetch, ""); err != nil {
		t.Fatalf("RunDev: %v", err)
	}

	answers["/"] = Response{Status: http.StatusOK, Body: "Rendered in Go. <rill-island", ContentType: "text/html"}
	if err := RunDev(fetch, ""); err == nil {
		t.Error("a page without the live reload script should fail the dev run")
	}
}

func TestVerifyRebuildLooksForTheEditedMarkup(t *testing.T) {
	fetch := func(string) (Response, error) { return Response{Body: "before " + DevMarker}, nil }
	if err := VerifyRebuild(fetch, ""); err != nil {
		t.Fatalf("VerifyRebuild: %v", err)
	}
	stale := func(string) (Response, error) { return Response{Body: "before"}, nil }
	if err := VerifyRebuild(stale, ""); err == nil {
		t.Error("an unchanged page should fail the rebuild check")
	}
}

func TestOverlaidRecognisesTheDevOverlay(t *testing.T) {
	overlay := Response{Status: http.StatusInternalServerError, Body: "rill: the project does not compile"}
	if !Overlaid(overlay) {
		t.Error("the overlay was not recognised")
	}
	if Overlaid(Response{Status: http.StatusOK, Body: "does not compile"}) {
		t.Error("a 200 is not an overlay")
	}
	if Overlaid(Response{Status: http.StatusInternalServerError, Body: "boom"}) {
		t.Error("an unrelated 500 is not an overlay")
	}
}
