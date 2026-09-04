package smoke

import (
	"fmt"
	"net/http"
	"strings"
)

const (
	FragmentHeader = "GOPAGE-Fragment"
	FragmentType   = "text/vnd.gopage-fragment"
	FragmentPath   = "/"
	FragmentName   = "Cheapest"
	FragmentBody   = "attic on the square"
)

func RunFragment(client *http.Client, base string) error {
	response, text, err := askFragment(client, base, FragmentName)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: fragment %s answered %d, want 200",
			FragmentPath, FragmentName, response.StatusCode)
	}
	if got := response.Header.Get("Content-Type"); !strings.HasPrefix(got, FragmentType) {
		return fmt.Errorf("%s: fragment content type %q, want %q", FragmentPath, got, FragmentType)
	}
	if got := response.Header.Get("Cache-Control"); !strings.Contains(got, "private") {
		return fmt.Errorf("%s: fragment cache control %q, want it kept out of shared caches",
			FragmentPath, got)
	}
	if strings.Contains(text, "<html") || strings.Contains(text, "gopage-slot") {
		return fmt.Errorf("%s: the fragment carries the document around it", FragmentPath)
	}
	if !strings.Contains(text, FragmentBody) {
		return fmt.Errorf("%s: the fragment carries no body", FragmentPath)
	}

	refused, _, err := askFragment(client, base, "Missing")
	if err != nil {
		return err
	}
	if refused.StatusCode != http.StatusNotFound {
		return fmt.Errorf("%s: a fragment the route does not hold answered %d, want 404",
			FragmentPath, refused.StatusCode)
	}
	return nil
}

func askFragment(client *http.Client, base, name string) (*http.Response, string, error) {
	request, err := http.NewRequest(http.MethodGet, base+FragmentPath, nil)
	if err != nil {
		return nil, "", err
	}
	request.Header.Set(FragmentHeader, name)
	response, err := client.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = response.Body.Close() }()
	text, err := body(response, nil)
	if err != nil {
		return nil, "", err
	}
	return response, text, nil
}
