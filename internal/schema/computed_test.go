package schema

import (
	"slices"
	"testing"

	"github.com/apptivitypl/gopage/internal/diag"
)

func parseSchema(t *testing.T, code string) *Schema {
	t.Helper()
	var bag diag.Bag
	schema := Parse([]Source{{File: "app/page.gopage", Code: code}}, &bag)
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %v", bag.Sorted())
	}
	return schema
}

func TestMethodsBecomeComputedFields(t *testing.T) {
	schema := parseSchema(t, `
type Props struct {
	Price int64
	Area  int64
}

func (p Props) PricePerM2() int64 { return p.Price }

func (p *Props) Label() string { return "x" }
`)
	props, ok := schema.Get("Props")
	if !ok {
		t.Fatal("no Props")
	}
	if !slices.Contains(props.FieldNames(), "PricePerM2") || !slices.Contains(props.FieldNames(), "Label") {
		t.Fatalf("fields = %v", props.FieldNames())
	}
	field, _ := props.Field("PricePerM2")
	if !field.Computed || field.Type.Kind != KindInt {
		t.Errorf("field = %+v", field)
	}
	stored, _ := props.Field("Price")
	if stored.Computed {
		t.Errorf("a stored field is not computed: %+v", stored)
	}
}

func TestOnlyNiladicExportedMethodsCount(t *testing.T) {
	schema := parseSchema(t, `
type Props struct {
	Price int64
}

func (p Props) WithArgument(n int) int64 { return p.Price }

func (p Props) NoResult() {}

func (p Props) TwoResults() (int64, error) { return 0, nil }

func (p Props) unexported() int64 { return 0 }

func (p Props) Unsupported() chan int { return nil }

func Free() int64 { return 0 }
`)
	props, _ := schema.Get("Props")
	if len(props.FieldNames()) != 1 {
		t.Errorf("fields = %v, want only the stored one", props.FieldNames())
	}
}

func TestAMethodOnAnUnknownTypeIsIgnored(t *testing.T) {
	schema := parseSchema(t, `
type Props struct {
	Price int64
}

type other int

func (o other) Value() int64 { return 0 }
`)
	props, _ := schema.Get("Props")
	if len(props.FieldNames()) != 1 {
		t.Errorf("fields = %v", props.FieldNames())
	}
}

func TestAMethodNeverShadowsAStoredField(t *testing.T) {
	schema := parseSchema(t, `
type Props struct {
	Price int64
}

func (p Props) Price() int64 { return 0 }
`)
	props, _ := schema.Get("Props")
	field, _ := props.Field("Price")
	if field.Computed || len(props.FieldNames()) != 1 {
		t.Errorf("fields = %v, field = %+v", props.FieldNames(), field)
	}
}

func TestAMethodOnAMultiReceiverListIsIgnored(t *testing.T) {
	var bag diag.Bag
	schema := Parse([]Source{{File: "app/page.gopage", Code: `
type Props struct {
	Price int64
}

func (p Props) Ok() int64 { return 0 }
`}}, &bag)
	props, _ := schema.Get("Props")
	if !slices.Contains(props.FieldNames(), "Ok") {
		t.Errorf("fields = %v", props.FieldNames())
	}
}

func TestOddReceiversAreIgnored(t *testing.T) {
	schema := parseSchema(t, `
type Props struct {
	Price int64
}

type Box[T any] struct{}

func () Loose() int64 { return 0 }

func (b Box[int64]) Generic() int64 { return 0 }

func (b *Box[int64]) PointerGeneric() int64 { return 0 }
`)
	props, _ := schema.Get("Props")
	if len(props.FieldNames()) != 1 {
		t.Errorf("fields = %v, want only the stored one", props.FieldNames())
	}
}

func TestPrivateFieldsAreMarked(t *testing.T) {
	schema := parseSchema(t, "\n"+`type Props struct {
	Title   string
	Viewer  Viewer `+"`gopage:\"private\"`"+`
	Session Session
}

type Viewer struct {
	Email string
}

type Session struct {
	Email string `+"`gopage:\"private\"`"+`
	Theme string
}
`)
	cases := map[bool][][]string{
		true:  {{"Viewer"}, {"Viewer", "Email"}, {"Session", "Email"}},
		false: {{"Title"}, {"Session"}, {"Session", "Theme"}},
	}
	for want, paths := range cases {
		for _, path := range paths {
			if got := schema.Private(PropsName, path); got != want {
				t.Errorf("Private(%v) = %v, want %v", path, got, want)
			}
		}
	}
}

func TestPrivateStopsAtWhatItCannotResolve(t *testing.T) {
	schema := parseSchema(t, "\n"+`type Props struct {
	Title  string
	Tags   []Tag
	Nested Deep
}

type Tag struct {
	Name string `+"`gopage:\"private\"`"+`
}

type Deep struct {
	Value string
}
`)
	if !schema.Private(PropsName, []string{"Tags", "Name"}) {
		t.Error("a private field under a slice is still private")
	}
	for _, path := range [][]string{{"Missing"}, {"Title", "Deeper"}, {"Nested", "Missing"}, {"Nested", "Value", "More"}} {
		if schema.Private(PropsName, path) {
			t.Errorf("Private(%v) = true, want false", path)
		}
	}
	if schema.Private("Nothing", []string{"x"}) {
		t.Error("an unknown root is not private")
	}
}
