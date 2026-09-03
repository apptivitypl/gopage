package smoke

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
)

const (
	FormPath   = "/items/2"
	tokenField = "__csrf"
	FlashText  = "enquiry received"
)

var tokenPattern = regexp.MustCompile(`name="` + tokenField + `" value="([^"]+)"`)

func RunForm(client *http.Client, base string) error {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return err
	}
	client.Jar = jar

	page, err := body(client.Get(base + FormPath))
	if err != nil {
		return err
	}
	match := tokenPattern.FindStringSubmatch(page)
	if match == nil {
		return fmt.Errorf("%s: the form carries no %s field", FormPath, tokenField)
	}
	token := match[1]

	if err := rejectsBadInput(client, base, token); err != nil {
		return err
	}
	if err := acceptsGoodInput(client, base, token); err != nil {
		return err
	}
	return rejectsAMissingToken(client, base)
}

func rejectsBadInput(client *http.Client, base, token string) error {
	response, err := submit(client, base, url.Values{
		tokenField: {token},
		"Name":     {"A"},
		"Email":    {"nope"},
	})
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusUnprocessableEntity {
		return fmt.Errorf("%s: a rejected form answered %d, want 422", FormPath, response.StatusCode)
	}
	text, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if !strings.Contains(string(text), "field-error") || !strings.Contains(string(text), `value="A"`) {
		return fmt.Errorf("%s: the rerendered form lost its errors or its values", FormPath)
	}
	return nil
}

func acceptsGoodInput(client *http.Client, base, token string) error {
	response, err := submit(client, base, url.Values{
		tokenField: {token},
		"Name":     {"Ada"},
		"Email":    {"ada@example.com"},
		"Consent":  {"on"},
	})
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusSeeOther {
		return fmt.Errorf("%s: an accepted form answered %d, want 303", FormPath, response.StatusCode)
	}
	page, err := body(client.Get(base + FormPath))
	if err != nil {
		return err
	}
	if !strings.Contains(page, FlashText) {
		return fmt.Errorf("%s: the flash did not survive the redirect", FormPath)
	}
	again, err := body(client.Get(base + FormPath))
	if err != nil {
		return err
	}
	if strings.Contains(again, FlashText) {
		return fmt.Errorf("%s: the flash was shown twice", FormPath)
	}
	return nil
}

func rejectsAMissingToken(client *http.Client, base string) error {
	response, err := submit(client, base, url.Values{"Name": {"Ada"}})
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusForbidden {
		return fmt.Errorf("%s: a submission without a token answered %d, want 403", FormPath, response.StatusCode)
	}
	return nil
}

func submit(client *http.Client, base string, values url.Values) (*http.Response, error) {
	request, err := http.NewRequest(http.MethodPost, base+FormPath, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return client.Do(request)
}

func body(response *http.Response, err error) (string, error) {
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	text, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	return string(text), nil
}
