package main

import "testing"

func TestOnChannelHoldsTheEvenOddConvention(t *testing.T) {
	cases := []struct {
		version    string
		preRelease bool
		allowed    bool
	}{
		{"0.2.0", false, true},
		{"0.2.7", false, true},
		{"1.4.0", false, true},
		{"0.3.0", true, true},
		{"0.11.2", true, true},
		{"0.2.0", true, false},
		{"0.3.0", false, false},
	}
	for _, c := range cases {
		err := onChannel(c.version, c.preRelease)
		if (err == nil) != c.allowed {
			t.Errorf("onChannel(%q, %v) = %v, allowed = %v", c.version, c.preRelease, err, c.allowed)
		}
	}
}

func TestOnChannelRefusesAVersionWithoutAMinor(t *testing.T) {
	for _, version := range []string{"1", "1.x.0"} {
		if err := onChannel(version, false); err == nil {
			t.Errorf("onChannel(%q) accepted a version with no minor", version)
		}
	}
}

func TestStampVersionKeepsEveryOtherField(t *testing.T) {
	source := []byte("{\n  \"name\": \"gopage\",\n  \"version\": \"0.0.0\",\n  \"publisher\": \"apptivitypl\"\n}\n")
	stamped, err := stampVersion(source, "0.2.0")
	if err != nil {
		t.Fatalf("stampVersion: %v", err)
	}
	want := "{\n  \"name\": \"gopage\",\n  \"version\": \"0.2.0\",\n  \"publisher\": \"apptivitypl\"\n}\n"
	if string(stamped) != want {
		t.Errorf("stamped = %q, want %q", stamped, want)
	}
}

func TestStampVersionRefusesAManifestThatAlreadyCarriesOne(t *testing.T) {
	if _, err := stampVersion([]byte(`{"version": "0.2.0"}`), "0.4.0"); err == nil {
		t.Fatal("stampVersion overwrote a version that was already set")
	}
}

func TestVocabularyNamesEveryRuleTheGrammarMustCarry(t *testing.T) {
	want := []string{"builtin-component", "directive-name", "filter-name", "strategy-value"}
	vocabulary := vocabulary()
	if len(vocabulary) != len(want) {
		t.Fatalf("vocabulary has %d rules, want %d", len(vocabulary), len(want))
	}
	for _, name := range want {
		if len(vocabulary[name]) == 0 {
			t.Errorf("%s names nothing", name)
		}
	}
}
