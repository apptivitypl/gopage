package redirect

import "testing"

func TestARootedPathIsTheOnlyPathAccepted(t *testing.T) {
	cases := map[string]bool{
		"/":                true,
		"/listings/1":      true,
		"/a?b=c#d":         true,
		"//evil.com":       false,
		"/\\evil.com":      false,
		"/\t/evil.com":     false,
		"/\r\n/evil.com":   false,
		"https://evil.com": false,
		"evil.com":         false,
		"":                 false,
		"\\\\evil.com":     false,
		"/ok\x7f":          false,
	}
	for target, want := range cases {
		if _, got := Path(target); got != want {
			t.Errorf("Path(%q) = %v, want %v", target, got, want)
		}
	}
}

func TestALocationTakesAnAbsoluteHttpUrlButNeverASchemeRelativeOne(t *testing.T) {
	cases := map[string]bool{
		"/thanks":                    true,
		"https://payments.test/x":    true,
		"http://payments.test/x":     true,
		"//evil.com":                 false,
		"/\\evil.com":                false,
		"javascript:alert(1)":        false,
		"data:text/html,<script>":    false,
		"mailto:someone@example.com": false,
		"https://":                   false,
		"https://evil.com\n":         false,
	}
	for target, want := range cases {
		if _, got := Location(target); got != want {
			t.Errorf("Location(%q) = %v, want %v", target, got, want)
		}
	}
}

func TestAnAcceptedTargetComesBackUnchanged(t *testing.T) {
	for _, target := range []string{"/listings/1?page=2", "https://payments.test/checkout"} {
		got, ok := Location(target)
		if !ok || got != target {
			t.Errorf("Location(%q) = %q, %v", target, got, ok)
		}
	}
}

func TestSafeTakesAPathWithoutAnAllowlist(t *testing.T) {
	for _, target := range []string{"/", "/a/b", "/a?x=1#y"} {
		if got, ok := Safe(target, nil); !ok || got != target {
			t.Errorf("Safe(%q) = %q, %v", target, got, ok)
		}
	}
}

func TestSafeRefusesAnotherSiteUnlessNamed(t *testing.T) {
	for _, target := range []string{"https://evil.test/x", "http://evil.test", "//evil.test"} {
		if _, ok := Safe(target, nil); ok {
			t.Errorf("Safe(%q) was allowed with no allowlist", target)
		}
		if _, ok := Safe(target, []string{"payments.test"}); ok {
			t.Errorf("Safe(%q) was allowed against a different host", target)
		}
	}
}

func TestSafeMatchesAHostWithItsPort(t *testing.T) {
	if _, ok := Safe("https://payments.test:8443/x", []string{"payments.test"}); ok {
		t.Error("a host with a port must not match the bare host")
	}
	if _, ok := Safe("https://payments.test:8443/x", []string{"payments.test:8443"}); !ok {
		t.Error("the port must be allowed to be part of the entry")
	}
}

func TestSafeStillRefusesAnUnparsableTarget(t *testing.T) {
	if _, ok := Safe("javascript:alert(1)", []string{"evil.test"}); ok {
		t.Error("a scheme that runs must never be allowed")
	}
	if _, ok := Safe("https://\x7f/x", []string{"evil.test"}); ok {
		t.Error("a control character must be refused")
	}
}
