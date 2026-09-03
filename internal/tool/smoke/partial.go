package smoke

import (
	"fmt"
	"net/http"
	"strings"
)

const (
	PartialHeader = "RILL-Partial"
	LevelHeader   = "RILL-Level"
	PartialType   = "text/vnd.rill-partial"
	PartialFrom   = "/"
	PartialTo     = "/items/1"
)

func RunPartial(client *http.Client, base string) error {
	request, err := http.NewRequest(http.MethodGet, base+PartialTo, nil)
	if err != nil {
		return err
	}
	request.Header.Set(PartialHeader, PartialFrom)
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	if got := response.Header.Get("Content-Type"); !strings.HasPrefix(got, PartialType) {
		return fmt.Errorf("%s: content type %q, want %q", PartialTo, got, PartialType)
	}
	if got := response.Header.Get(LevelHeader); got != "1" {
		return fmt.Errorf("%s: level %q, want the shared layout kept", PartialTo, got)
	}
	text, err := body(response, nil)
	if err != nil {
		return err
	}
	if strings.Contains(text, "<!doctype") || strings.Contains(text, "<html") {
		return fmt.Errorf("%s: the partial carries the whole document", PartialTo)
	}
	if !strings.Contains(text, "<h1") {
		return fmt.Errorf("%s: the partial carries no page", PartialTo)
	}
	return nil
}
