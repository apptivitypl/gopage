package redirect

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestAFailureCarriesTheStatusAndLocation(t *testing.T) {
	var failure *Error
	if !errors.As(Fail(http.StatusMovedPermanently, "/jobs"), &failure) {
		t.Fatal("Fail answers with a redirect error")
	}
	if failure.Status != http.StatusMovedPermanently || failure.Location != "/jobs" {
		t.Errorf("failure = %+v", failure)
	}
	if !strings.Contains(failure.Error(), "/jobs") {
		t.Errorf("message = %q", failure.Error())
	}
}

func TestAStatusOutsideTheRedirectRangeBecomesFound(t *testing.T) {
	for _, status := range []int{0, http.StatusOK, http.StatusNotFound, 999} {
		var failure *Error
		if !errors.As(Fail(status, "/"), &failure) || failure.Status != http.StatusFound {
			t.Errorf("status %d = %+v", status, failure)
		}
	}
}
