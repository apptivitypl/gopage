package form

import (
	"fmt"
	"mime"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/apptivitypl/rill/internal/validate"
)

const (
	Root          = "form"
	TokenField    = "Token"
	MaxMemory     = 8 << 20
	urlencoded    = "application/x-www-form-urlencoded"
	multipartType = "multipart/form-data"
)

type Result struct {
	Values     map[string]string
	Errors     map[string][]string
	Submitted  bool
	fieldOrder []string
}

func (r Result) Valid() bool {
	return len(r.Errors) == 0
}

func (r Result) Fields() []string {
	return r.fieldOrder
}

func Decode(request *http.Request, target any) (Result, error) {
	result := Result{Values: map[string]string{}, Errors: map[string][]string{}, Submitted: true}
	values, err := read(request)
	if err != nil {
		return result, err
	}
	pointer := reflect.ValueOf(target)
	if pointer.Kind() != reflect.Pointer || pointer.IsNil() || pointer.Elem().Kind() != reflect.Struct {
		return result, fmt.Errorf("form: target must be a pointer to a struct, got %T", target)
	}
	structure := pointer.Elem()
	fields := structure.Type()
	for i := range fields.NumField() {
		field := fields.Field(i)
		if !field.IsExported() {
			continue
		}
		result.fieldOrder = append(result.fieldOrder, field.Name)
		raw, present := lookup(values, field)
		result.Values[field.Name] = raw
		if err := assign(structure.Field(i), raw, present); err != nil {
			result.add(field.Name, err.Error())
			continue
		}
		result.checkRules(field, raw, present, structure.Field(i))
	}
	return result, nil
}

func (r *Result) checkRules(field reflect.StructField, raw string, present bool, target reflect.Value) {
	constraints, err := validate.Parse(field.Tag.Get(validate.Tag))
	if err != nil {
		r.add(field.Name, err.Error())
		return
	}
	for _, violation := range validate.Check(field.Name, valueOf(target, raw, present), constraints) {
		r.add(field.Name, violation.Message)
	}
}

func (r *Result) add(field, message string) {
	r.Errors[field] = append(r.Errors[field], message)
}

func read(request *http.Request) (map[string][]string, error) {
	kind, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil {
		return nil, fmt.Errorf("form: a submission needs a content type of %s or %s", urlencoded, multipartType)
	}
	switch kind {
	case multipartType:
		if err := request.ParseMultipartForm(MaxMemory); err != nil {
			return nil, err
		}
		return request.MultipartForm.Value, nil
	case urlencoded:
		if err := request.ParseForm(); err != nil {
			return nil, err
		}
		return request.PostForm, nil
	default:
		return nil, fmt.Errorf("form: %s is not a form encoding", kind)
	}
}

func lookup(values map[string][]string, field reflect.StructField) (string, bool) {
	list, ok := values[field.Name]
	if !ok || len(list) == 0 {
		return "", false
	}
	return list[0], true
}

func valueOf(target reflect.Value, raw string, present bool) validate.Value {
	value := validate.Value{Str: raw, Present: present}
	element := target
	if element.Kind() == reflect.Pointer {
		if element.IsNil() {
			value.Present = false
			return value
		}
		element = element.Elem()
	}
	switch element.Kind() {
	case reflect.Bool:
		value.Kind = validate.KindBool
		value.Bool = element.Bool()
		value.Present = true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value.Kind = validate.KindInt
		value.Num = float64(element.Int())
	case reflect.Float32, reflect.Float64:
		value.Kind = validate.KindFloat
		value.Num = element.Float()
	default:
		value.Kind = validate.KindString
	}
	return value
}

func assign(target reflect.Value, raw string, present bool) error {
	if target.Kind() == reflect.Pointer {
		if !present || raw == "" {
			target.SetZero()
			return nil
		}
		element := reflect.New(target.Type().Elem())
		if err := assign(element.Elem(), raw, present); err != nil {
			return err
		}
		target.Set(element)
		return nil
	}
	switch target.Kind() {
	case reflect.String:
		target.SetString(raw)
		return nil
	case reflect.Bool:
		target.SetBool(checked(raw))
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return setInt(target, raw, present)
	case reflect.Float32, reflect.Float64:
		return setFloat(target, raw, present)
	default:
		return fmt.Errorf("this field has a type a form cannot carry")
	}
}

func setInt(target reflect.Value, raw string, present bool) error {
	if !present || raw == "" {
		target.SetInt(0)
		return nil
	}
	number, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fmt.Errorf("this field takes a whole number")
	}
	target.SetInt(number)
	return nil
}

func setFloat(target reflect.Value, raw string, present bool) error {
	if !present || raw == "" {
		target.SetFloat(0)
		return nil
	}
	number, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fmt.Errorf("this field takes a number")
	}
	target.SetFloat(number)
	return nil
}

func checked(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "on", "true", "1", "yes":
		return true
	default:
		return false
	}
}

func (r Result) Messages() []string {
	names := make([]string, 0, len(r.Errors))
	for name := range r.Errors {
		names = append(names, name)
	}
	sort.Strings(names)
	var messages []string
	for _, name := range names {
		messages = append(messages, r.Errors[name]...)
	}
	return messages
}
