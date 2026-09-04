package form

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apptivitypl/gopage/internal/runtime"
)

type apply struct {
	Name    string  `validate:"required,len:2..80"`
	Email   string  `validate:"email"`
	Message *string `validate:"max:20"`
	Age     int     `validate:"min:18"`
	Score   float64
	Consent bool `validate:"accepted"`
	hidden  string
}

func post(t *testing.T, body string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/apply", strings.NewReader(body))
	request.Header.Set("Content-Type", urlencoded)
	return request
}

func decode(t *testing.T, body string) (apply, Result) {
	t.Helper()
	var target apply
	result, err := Decode(post(t, body), &target)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	return target, result
}

func TestDecodeFillsEveryKind(t *testing.T) {
	target, result := decode(t,
		"Name=Ada&Email=ada@example.com&Message=hi&Age=41&Score=1.5&Consent=on")
	if !result.Valid() {
		t.Fatalf("errors = %v", result.Errors)
	}
	if target.Name != "Ada" || target.Email != "ada@example.com" || target.Age != 41 {
		t.Errorf("target = %+v", target)
	}
	if target.Message == nil || *target.Message != "hi" {
		t.Errorf("message = %v", target.Message)
	}
	if target.Score != 1.5 || !target.Consent {
		t.Errorf("target = %+v", target)
	}
}

func TestUnexportedFieldsAreSkipped(t *testing.T) {
	target, result := decode(t, "Name=Ada&Email=ada@example.com&Age=41&Consent=on&hidden=x")
	if _, listed := result.Values["hidden"]; listed {
		t.Errorf("values = %v", result.Values)
	}
	if target.hidden != "" {
		t.Errorf("hidden = %q, want it untouched", target.hidden)
	}
}

func TestMissingOptionalPointerStaysNil(t *testing.T) {
	target, _ := decode(t, "Name=Ada&Email=ada@example.com&Age=41&Consent=on")
	if target.Message != nil {
		t.Errorf("message = %v, want nil", *target.Message)
	}
}

func TestAnEmptyPointerFieldStaysNil(t *testing.T) {
	target, _ := decode(t, "Name=Ada&Email=ada@example.com&Age=41&Consent=on&Message=")
	if target.Message != nil {
		t.Errorf("message = %q, want nil", *target.Message)
	}
}

func TestValidationFailuresAreCollected(t *testing.T) {
	_, result := decode(t, "Name=A&Email=nope&Age=12")
	if result.Valid() {
		t.Fatal("expected violations")
	}
	for _, field := range []string{"Name", "Email", "Age", "Consent"} {
		if len(result.Errors[field]) == 0 {
			t.Errorf("%s has no violation: %v", field, result.Errors)
		}
	}
}

func TestRawValuesAreKeptForRedisplay(t *testing.T) {
	_, result := decode(t, "Name=A&Email=nope")
	if result.Values["Name"] != "A" || result.Values["Email"] != "nope" {
		t.Errorf("values = %v", result.Values)
	}
}

func TestMalformedNumbersAreReported(t *testing.T) {
	_, result := decode(t, "Name=Ada&Email=ada@example.com&Age=old&Score=x&Consent=on")
	if !strings.Contains(strings.Join(result.Errors["Age"], " "), "whole number") {
		t.Errorf("age errors = %v", result.Errors["Age"])
	}
	if !strings.Contains(strings.Join(result.Errors["Score"], " "), "number") {
		t.Errorf("score errors = %v", result.Errors["Score"])
	}
}

func TestCheckboxSpellings(t *testing.T) {
	for _, raw := range []string{"on", "true", "1", "yes", "ON"} {
		target, _ := decode(t, "Name=Ada&Email=ada@example.com&Age=41&Consent="+raw)
		if !target.Consent {
			t.Errorf("%q did not tick the box", raw)
		}
	}
	target, _ := decode(t, "Name=Ada&Email=ada@example.com&Age=41&Consent=off")
	if target.Consent {
		t.Error("off must not tick the box")
	}
}

func TestMultipartBodiesAreRead(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range map[string]string{"Name": "Ada", "Email": "ada@example.com", "Age": "41", "Consent": "on"} {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/apply", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())

	var target apply
	result, err := Decode(request, &target)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !result.Valid() || target.Name != "Ada" {
		t.Errorf("target = %+v, errors = %v", target, result.Errors)
	}
}

func TestBrokenMultipartIsReported(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/apply", strings.NewReader("not multipart"))
	request.Header.Set("Content-Type", "multipart/form-data; boundary=zzz")
	var target apply
	if _, err := Decode(request, &target); err == nil {
		t.Error("a malformed multipart body must be reported")
	}
}

func TestASubmissionNeedsAFormEncoding(t *testing.T) {
	cases := map[string]string{"missing": "", "wrong": "application/json"}
	for name, kind := range cases {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/apply", strings.NewReader("Name=Ada"))
			if kind != "" {
				request.Header.Set("Content-Type", kind)
			}
			var target apply
			if _, err := Decode(request, &target); err == nil {
				t.Error("expected an error naming the encoding")
			}
		})
	}
}

func TestTargetMustBeAStructPointer(t *testing.T) {
	var target apply
	for _, bad := range []any{target, &[]string{}, (*apply)(nil), 7} {
		if _, err := Decode(post(t, ""), bad); err == nil {
			t.Errorf("%T was accepted", bad)
		}
	}
}

type unsupported struct {
	Tags []string
}

func TestUnsupportedFieldTypesAreReported(t *testing.T) {
	var target unsupported
	result, err := Decode(post(t, "Tags=a"), &target)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(result.Errors["Tags"]) == 0 {
		t.Errorf("errors = %v", result.Errors)
	}
}

type badTag struct {
	Name string `validate:"wobble"`
}

func TestAnUnknownRuleIsReportedAgainstTheField(t *testing.T) {
	var target badTag
	result, err := Decode(post(t, "Name=Ada"), &target)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(result.Errors["Name"]) == 0 {
		t.Errorf("errors = %v", result.Errors)
	}
}

func TestFieldsAndMessagesAreOrdered(t *testing.T) {
	_, result := decode(t, "Name=A&Email=nope&Age=12")
	if len(result.Fields()) != 6 || result.Fields()[0] != "Name" {
		t.Errorf("fields = %v", result.Fields())
	}
	if len(result.Messages()) < 3 {
		t.Errorf("messages = %v", result.Messages())
	}
}

func TestViewExposesTheResultToTemplates(t *testing.T) {
	_, result := decode(t, "Name=A&Email=nope&Age=12")
	view := NewView(result, "tok")

	failed, _ := view.Get([]string{"Failed"})
	if !failed.Truthy() {
		t.Error("a rejected submission is failed")
	}
	token, _ := view.Get([]string{"Token"})
	if token.Str != "tok" {
		t.Errorf("token = %+v", token)
	}
	value, _ := view.Get([]string{"Values", "Name"})
	if value.Str != "A" {
		t.Errorf("value = %+v", value)
	}
	message, _ := view.Get([]string{"Errors", "Email"})
	if message.Str == "" {
		t.Errorf("message = %+v", message)
	}
	empty, ok := view.Get([]string{"Errors", "Score"})
	if !ok || empty.Str != "" {
		t.Errorf("a field without a violation reads as empty: %+v", empty)
	}
}

func TestViewRejectsUnknownPaths(t *testing.T) {
	view := NewView(Result{Values: map[string]string{}, Errors: map[string][]string{}}, "")
	for _, path := range [][]string{nil, {"Wobble"}, {"Values"}, {"Errors"}, {"Values", "a", "b"}} {
		if _, ok := view.Get(path); ok {
			t.Errorf("%v was accepted", path)
		}
	}
}

func TestWithPutsTheFormBesideTheProps(t *testing.T) {
	_, result := decode(t, "Name=A")
	props := With(runtime.Empty{}, result, "tok")
	value, ok := props.Get([]string{Root, "Values", "Name"})
	if !ok || value.Str != "A" {
		t.Errorf("value = %+v, ok = %v", value, ok)
	}
}

func TestAMalformedUrlencodedBodyIsReported(t *testing.T) {
	var target apply
	if _, err := Decode(post(t, "Name=%zz"), &target); err == nil {
		t.Error("a body that is not valid urlencoding must be reported")
	}
}

type optionalNumber struct {
	Age *int
}

func TestAnOptionalNumberReportsItsOwnParseFailure(t *testing.T) {
	var target optionalNumber
	result, err := Decode(post(t, "Age=old"), &target)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(result.Errors["Age"]) == 0 {
		t.Errorf("errors = %v", result.Errors)
	}
	if target.Age != nil {
		t.Errorf("age = %v, want nil", *target.Age)
	}
}
