package jsonc

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func clean(t *testing.T, text string) string {
	t.Helper()
	out, err := ToJSON([]byte(text))
	if err != nil {
		t.Fatalf("ToJSON(%q): %v", text, err)
	}
	if len(out) != len(text) {
		t.Fatalf("length changed: %d bytes in, %d out", len(text), len(out))
	}
	return string(out)
}

func TestPlainJSONIsUnchanged(t *testing.T) {
	text := "{\n  \"a\": 1,\n  \"b\": [2, 3]\n}\n"
	if got := clean(t, text); got != text {
		t.Errorf("got %q", got)
	}
}

func TestLineCommentsBecomeSpaces(t *testing.T) {
	got := clean(t, "{\n  // a note\n  \"a\": 1 // trailing\n}")
	if strings.Contains(got, "note") || strings.Contains(got, "trailing") {
		t.Errorf("comment survived: %q", got)
	}
	var out map[string]int
	if err := json.Unmarshal([]byte(got), &out); err != nil || out["a"] != 1 {
		t.Errorf("decode = %v, %v", out, err)
	}
}

func TestBlockCommentsBecomeSpacesAndKeepNewlines(t *testing.T) {
	got := clean(t, "{\n  /* one\n     two */\n  \"a\": 1\n}")
	if strings.Count(got, "\n") != 4 {
		t.Errorf("newlines lost: %q", got)
	}
	var out map[string]int
	if err := json.Unmarshal([]byte(got), &out); err != nil || out["a"] != 1 {
		t.Errorf("decode = %v, %v", out, err)
	}
}

func TestCommentMarkersInsideStringsSurvive(t *testing.T) {
	text := `{"url": "https://gopage.dev/a//b", "block": "/* not a comment */"}`
	got := clean(t, text)
	if got != text {
		t.Errorf("got %q", got)
	}
}

func TestEscapedQuotesDoNotEndTheString(t *testing.T) {
	text := `{"a": "she said \"// hi\"", "b": 1}`
	if got := clean(t, text); got != text {
		t.Errorf("got %q", got)
	}
}

func TestABackslashBeforeTheClosingQuoteIsEscaped(t *testing.T) {
	text := `{"a": "back\\", "b": "// kept"}`
	if got := clean(t, text); got != text {
		t.Errorf("got %q", got)
	}
}

func TestTrailingCommasAreDropped(t *testing.T) {
	got := clean(t, "{\n  \"a\": [1, 2,],\n  \"b\": 3,\n}")
	var out struct {
		A []int
		B int
	}
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.A) != 2 || out.B != 3 {
		t.Errorf("out = %+v", out)
	}
}

func TestATrailingCommaBeforeAComentIsDropped(t *testing.T) {
	got := clean(t, "{\n  \"a\": 1, // note\n}")
	var out map[string]int
	if err := json.Unmarshal([]byte(got), &out); err != nil || out["a"] != 1 {
		t.Errorf("decode = %v, %v", out, err)
	}
}

func TestCommasInsideStringsAreKept(t *testing.T) {
	text := `{"a": "one, two", "b": ["x,", "y"]}`
	if got := clean(t, text); got != text {
		t.Errorf("got %q", got)
	}
}

func TestAnUnterminatedBlockCommentIsReported(t *testing.T) {
	_, err := ToJSON([]byte("{\n  /* forever\n}"))
	var syntax *SyntaxError
	if err == nil || !asSyntax(err, &syntax) {
		t.Fatalf("err = %v, want a syntax error", err)
	}
	if syntax.Offset != 4 {
		t.Errorf("offset = %d, want 4", syntax.Offset)
	}
}

func TestAnUnterminatedStringIsReported(t *testing.T) {
	if _, err := ToJSON([]byte(`{"a": "open`)); err == nil {
		t.Error("an unterminated string must be reported")
	}
}

func TestACommentAtTheEndWithoutANewline(t *testing.T) {
	got := clean(t, "{\"a\": 1} // done")
	var out map[string]int
	if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &out); err != nil {
		t.Errorf("decode: %v", err)
	}
}

func TestCarriageReturnsAreLeftAlone(t *testing.T) {
	got := clean(t, "{\r\n  // note\r\n  \"a\": 1\r\n}")
	var out map[string]int
	if err := json.Unmarshal([]byte(got), &out); err != nil || out["a"] != 1 {
		t.Errorf("decode = %v, %v", out, err)
	}
}

func TestOffsetsStillPointAtTheOriginalSource(t *testing.T) {
	text := "{\n  // a comment that is quite long\n  \"a\": nope\n}"
	out, err := ToJSON([]byte(text))
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	var target map[string]int
	err = json.Unmarshal(out, &target)
	var syntax *json.SyntaxError
	if !asJSONSyntax(err, &syntax) {
		t.Fatalf("err = %v, want a json.SyntaxError", err)
	}
	line, column := Position([]byte(text), int(syntax.Offset))
	if line != 3 {
		t.Errorf("line = %d, column = %d, want line 3", line, column)
	}
}

func TestPositionCountsColumnsFromOne(t *testing.T) {
	line, column := Position([]byte("ab\ncd"), 3)
	if line != 2 || column != 1 {
		t.Errorf("line = %d, column = %d", line, column)
	}
}

func TestPositionClampsPastTheEnd(t *testing.T) {
	if line, _ := Position([]byte("ab"), 99); line != 1 {
		t.Errorf("line = %d", line)
	}
}

func TestLocateNamesTheLineAndKeepsTheCause(t *testing.T) {
	cause := &SyntaxError{Msg: "boom"}
	err := Locate([]byte("a\nbc"), 3, cause)
	if !strings.Contains(err.Error(), "line 2, column 2") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v", err)
	}
}

func TestSyntaxErrorReportsItsMessage(t *testing.T) {
	if (&SyntaxError{Msg: "x"}).Error() != "x" {
		t.Error("Error must report the message")
	}
}

func asSyntax(err error, out **SyntaxError) bool {
	return errors.As(err, out)
}

func asJSONSyntax(err error, out **json.SyntaxError) bool {
	return errors.As(err, out)
}
