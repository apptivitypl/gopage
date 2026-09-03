package schema

import (
	"testing"

	"github.com/apptivitypl/rill/internal/diag"
)

func deferredSchema(t *testing.T, code string) *Schema {
	t.Helper()
	var bag diag.Bag
	return Parse([]Source{{File: "app/page.rill", Code: code}}, &bag)
}

func TestALoaderBecomesADeferredField(t *testing.T) {
	model := deferredSchema(t, `
type Review struct {
	Author string
}

type Props struct {
	Heading string
}

func Reviews(ctx *rill.Ctx) ([]Review, error) {
	return nil, nil
}
`)
	props, ok := model.Props()
	if !ok {
		t.Fatal("props are missing")
	}
	field, ok := props.Field("Reviews")
	if !ok {
		t.Fatalf("fields = %+v, want Reviews", props.Fields)
	}
	if !field.Deferred || field.Type.Kind != KindSlice {
		t.Errorf("field = %+v, want a deferred slice", field)
	}
	if names := Deferred(model); len(names) != 1 || names[0] != "Reviews" {
		t.Errorf("Deferred = %v", names)
	}
}

func TestTheReservedLoadersAreNotDeferredFields(t *testing.T) {
	model := deferredSchema(t, `
type Props struct {
	Heading string
}

func Load(ctx *rill.Ctx) (Props, error) {
	return Props{}, nil
}

func Submit(ctx *rill.Ctx) (Props, error) {
	return Props{}, nil
}
`)
	if names := Deferred(model); len(names) != 0 {
		t.Errorf("Deferred = %v, want none", names)
	}
}

func TestOnlyTheLoaderShapeCounts(t *testing.T) {
	model := deferredSchema(t, `
type Props struct {
	Heading string
}

func NoContext() ([]string, error) {
	return nil, nil
}

func TwoParams(ctx *rill.Ctx, extra int) ([]string, error) {
	return nil, nil
}

func NotAPointer(ctx rill.Ctx) ([]string, error) {
	return nil, nil
}

func OneResult(ctx *rill.Ctx) []string {
	return nil
}

func WrongSecond(ctx *rill.Ctx) ([]string, string) {
	return nil, ""
}

func (p Props) Method(ctx *rill.Ctx) ([]string, error) {
	return nil, nil
}

func lower(ctx *rill.Ctx) ([]string, error) {
	return nil, nil
}

func Good(ctx *rill.Ctx) (string, error) {
	return "", nil
}
`)
	if names := Deferred(model); len(names) != 1 || names[0] != "Good" {
		t.Errorf("Deferred = %v, want only Good", names)
	}
}

func TestADeferredFieldDoesNotShadowARealOne(t *testing.T) {
	model := deferredSchema(t, `
type Props struct {
	Reviews string
}

func Reviews(ctx *rill.Ctx) ([]string, error) {
	return nil, nil
}
`)
	props, _ := model.Props()
	field, _ := props.Field("Reviews")
	if field.Deferred || field.Type.Kind != KindString {
		t.Errorf("field = %+v, want the declared field kept", field)
	}
}

func TestAProjectWithoutPropsHasNoDeferredFields(t *testing.T) {
	model := deferredSchema(t, `
func Reviews(ctx *rill.Ctx) ([]string, error) {
	return nil, nil
}
`)
	if names := Deferred(model); len(names) != 0 {
		t.Errorf("Deferred = %v, want none without props", names)
	}
	if names := Deferred(nil); names != nil {
		t.Errorf("Deferred(nil) = %v", names)
	}
}
