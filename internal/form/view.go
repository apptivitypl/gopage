package form

import "github.com/sonquer/rill/internal/runtime"

const (
	valuesField = "Values"
	errorsField = "Errors"
	failedField = "Failed"
	tokenField  = "Token"
)

type View struct {
	result Result
	token  string
}

func NewView(result Result, token string) View {
	return View{result: result, token: token}
}

func (v View) Get(path []string) (runtime.Value, bool) {
	if len(path) == 0 {
		return runtime.Nil(), false
	}
	switch path[0] {
	case failedField:
		return runtime.Bool(v.result.Submitted && !v.result.Valid()), true
	case tokenField:
		return runtime.String(v.token), true
	case valuesField:
		return lookupOne(v.result.Values, path[1:])
	case errorsField:
		return firstError(v.result.Errors, path[1:])
	default:
		return runtime.Nil(), false
	}
}

func lookupOne(values map[string]string, path []string) (runtime.Value, bool) {
	if len(path) != 1 {
		return runtime.Nil(), false
	}
	if values == nil {
		return runtime.String(""), true
	}
	return runtime.String(values[path[0]]), true
}

func firstError(errors map[string][]string, path []string) (runtime.Value, bool) {
	if len(path) != 1 {
		return runtime.Nil(), false
	}
	messages := errors[path[0]]
	if len(messages) == 0 {
		return runtime.String(""), true
	}
	return runtime.String(messages[0]), true
}

func With(props runtime.Accessible, result Result, token string) runtime.Accessible {
	return runtime.WithRoot(props, Root, NewView(result, token))
}
