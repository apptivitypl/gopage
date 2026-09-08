package redirect

import (
	"fmt"
	"net/http"
)

type Error struct {
	Status   int
	Location string
}

func (e *Error) Error() string {
	return fmt.Sprintf("redirect %d to %s", e.Status, e.Location)
}

func Fail(status int, location string) error {
	if status < http.StatusMultipleChoices || status > http.StatusPermanentRedirect {
		status = http.StatusFound
	}
	return &Error{Status: status, Location: location}
}
