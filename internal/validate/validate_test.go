package validate

import (
	"slices"
	"strings"
	"testing"
)

func text(value string) Value {
	return Value{Kind: KindString, Str: value, Present: true}
}

func number(value float64) Value {
	return Value{Kind: KindFloat, Num: value, Present: true}
}

func check(t *testing.T, value Value, tag string) []Violation {
	t.Helper()
	constraints, err := Parse(tag)
	if err != nil {
		t.Fatalf("Parse(%q): %v", tag, err)
	}
	return Check("Field", value, constraints)
}

func TestParseReadsSeveralRules(t *testing.T) {
	constraints, err := Parse("required, len:2..80")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(constraints) != 2 || constraints[1].Rule != "len" || constraints[1].Arg != "2..80" {
		t.Errorf("constraints = %+v", constraints)
	}
}

func TestParseIgnoresEmptyTags(t *testing.T) {
	for _, tag := range []string{"", "   ", ",,"} {
		constraints, err := Parse(tag)
		if err != nil || constraints != nil {
			t.Errorf("Parse(%q) = %v, %v", tag, constraints, err)
		}
	}
}

func TestParseRejectsAnUnknownRule(t *testing.T) {
	if _, err := Parse("wobble"); err == nil || !strings.Contains(err.Error(), "wobble") {
		t.Errorf("err = %v", err)
	}
}

func TestKnownAndRules(t *testing.T) {
	if !Known("email") || Known("wobble") {
		t.Error("Known must answer for the registry only")
	}
	if !slices.Contains(Rules(), "accepted") {
		t.Errorf("rules = %v", Rules())
	}
}

func TestRequired(t *testing.T) {
	if got := check(t, Value{Kind: KindString}, "required"); len(got) != 1 || got[0].Rule != "required" {
		t.Errorf("violations = %+v", got)
	}
	if got := check(t, text("  "), "required"); len(got) != 1 {
		t.Errorf("blank text is missing: %+v", got)
	}
	if got := check(t, text("x"), "required"); got != nil {
		t.Errorf("violations = %+v", got)
	}
}

func TestLengthBounds(t *testing.T) {
	if got := check(t, text("a"), "len:2..80"); len(got) != 1 || !strings.Contains(got[0].Message, "at least 2") {
		t.Errorf("violations = %+v", got)
	}
	if got := check(t, text(strings.Repeat("a", 81)), "len:2..80"); len(got) != 1 {
		t.Errorf("violations = %+v", got)
	}
	if got := check(t, text("ab"), "len:2..80"); got != nil {
		t.Errorf("violations = %+v", got)
	}
	if got := check(t, text("ab"), "len:2.."); got != nil {
		t.Errorf("an open upper bound accepts anything longer: %+v", got)
	}
	if got := check(t, text("żółw"), "len:4..4"); got != nil {
		t.Errorf("length counts runes, not bytes: %+v", got)
	}
}

func TestLengthRejectsMalformedBounds(t *testing.T) {
	for _, tag := range []string{"len:2", "len:a..b", "len:2..c"} {
		if got := check(t, text("x"), tag); len(got) != 1 {
			t.Errorf("%q produced %+v", tag, got)
		}
	}
}

func TestMinimumAndMaximum(t *testing.T) {
	if got := check(t, number(3), "min:5"); len(got) != 1 || !strings.Contains(got[0].Message, "starts at 5") {
		t.Errorf("violations = %+v", got)
	}
	if got := check(t, number(9), "max:5"); len(got) != 1 || !strings.Contains(got[0].Message, "stops at 5") {
		t.Errorf("violations = %+v", got)
	}
	if got := check(t, number(5), "min:5,max:5"); got != nil {
		t.Errorf("violations = %+v", got)
	}
	if got := check(t, text("abcdef"), "max:5"); len(got) != 1 || !strings.Contains(got[0].Message, "characters") {
		t.Errorf("strings are measured in characters: %+v", got)
	}
	if got := check(t, text("ab"), "min:5"); len(got) != 1 || !strings.Contains(got[0].Message, "characters") {
		t.Errorf("violations = %+v", got)
	}
}

func TestMinimumAndMaximumNeedNumbers(t *testing.T) {
	for _, tag := range []string{"min:x", "max:x"} {
		if got := check(t, number(1), tag); len(got) != 1 || !strings.Contains(got[0].Message, "number") {
			t.Errorf("%q produced %+v", tag, got)
		}
	}
}

func TestEmail(t *testing.T) {
	valid := []string{"a@b.co", "first.last@example.co.uk", "  a@b.co  "}
	for _, address := range valid {
		if got := check(t, text(address), "email"); got != nil {
			t.Errorf("%q rejected: %+v", address, got)
		}
	}
	invalid := []string{"", "a", "a@", "@b.co", "a@b", "a b@c.co", "a@.co", "a@b."}
	for _, address := range invalid {
		if got := check(t, text(address), "email"); len(got) != 1 {
			t.Errorf("%q accepted", address)
		}
	}
}

func TestAccepted(t *testing.T) {
	if got := check(t, Value{Kind: KindBool, Present: true}, "accepted"); len(got) != 1 {
		t.Errorf("violations = %+v", got)
	}
	if got := check(t, Value{Kind: KindBool, Bool: true, Present: true}, "accepted"); got != nil {
		t.Errorf("violations = %+v", got)
	}
}

func TestOneOf(t *testing.T) {
	if got := check(t, text("blue"), "in:red|green"); len(got) != 1 {
		t.Errorf("violations = %+v", got)
	}
	if got := check(t, text("green"), "in:red|green"); got != nil {
		t.Errorf("violations = %+v", got)
	}
}

func TestAbsentValuesSkipEveryRuleButRequiredAndAccepted(t *testing.T) {
	absent := Value{Kind: KindString}
	if got := check(t, absent, "email,len:2..8"); got != nil {
		t.Errorf("an absent optional field is not checked: %+v", got)
	}
	if got := check(t, absent, "required,email"); len(got) != 1 || got[0].Rule != "required" {
		t.Errorf("violations = %+v", got)
	}
	if got := check(t, Value{Kind: KindBool}, "accepted"); len(got) != 1 {
		t.Errorf("an unticked box is still checked: %+v", got)
	}
}

func TestViolationsCarryTheFieldName(t *testing.T) {
	constraints, _ := Parse("required")
	got := Check("Email", Value{Kind: KindString}, constraints)
	if len(got) != 1 || got[0].Field != "Email" {
		t.Errorf("violations = %+v", got)
	}
}
