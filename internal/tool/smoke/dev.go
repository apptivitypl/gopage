package smoke

import (
	"fmt"
	"net/http"
	"strings"
)

const DevMarker = "<p data-smoke=\"dev\">the dev server rebuilt this</p>"

func DevChecks() []Check {
	return []Check{
		{Path: "/", Status: http.StatusOK, Contains: "Rendered in Go.", ContentType: "text/html"},
		{Path: "/", Status: http.StatusOK, Contains: ReloadPath},
		{Path: "/", Status: http.StatusOK, Contains: `<gopage-island`},
		{Path: "/api/health", Status: http.StatusOK, Contains: `"runtime":"go"`, ContentType: "application/json"},
		{Path: "/nope", Status: http.StatusNotFound, Contains: "<title>no route answers this address</title>"},
		{Path: "/assets/", Status: http.StatusNotFound},
	}
}

const ReloadPath = "/_gopage/reload"

func RunDev(fetch Fetcher, base string) error {
	for _, check := range DevChecks() {
		response, err := fetch(base + check.Path)
		if err != nil {
			return fmt.Errorf("%s: %w", check.Path, err)
		}
		if err := Verify(check, response); err != nil {
			return err
		}
	}
	return nil
}

func VerifyRebuild(fetch Fetcher, base string) error {
	response, err := fetch(base + "/")
	if err != nil {
		return err
	}
	if !strings.Contains(response.Body, DevMarker) {
		return fmt.Errorf("the dev server did not pick up the edited template")
	}
	return nil
}

func Overlaid(response Response) bool {
	return response.Status == http.StatusInternalServerError &&
		strings.Contains(response.Body, "does not compile")
}
