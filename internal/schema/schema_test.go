package schema

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/sonquer/rill/internal/diag"
)

func parse(t *testing.T, code string) (*Schema, *diag.Bag) {
	t.Helper()
	var bag diag.Bag
	return Parse([]Source{{File: "page.rill", Code: code}}, &bag), &bag
}

func parseClean(t *testing.T, code string) *Schema {
	t.Helper()
	schema, bag := parse(t, code)
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %+v", bag.Items())
	}
	return schema
}

func hasCode(bag *diag.Bag, want diag.Code) bool {
	for _, d := range bag.Items() {
		if d.Code == want {
			return true
		}
	}
	return false
}

func TestScalarFields(t *testing.T) {
	schema := parseClean(t, `
type Props struct {
	Title   string
	Count   int
	Ratio   float64
	Enabled bool
}`)
	props, ok := schema.Props()
	if !ok {
		t.Fatal("Props was not found")
	}
	want := map[string]Kind{
		"Title":   KindString,
		"Count":   KindInt,
		"Ratio":   KindFloat,
		"Enabled": KindBool,
	}
	for name, kind := range want {
		field, ok := props.Field(name)
		if !ok {
			t.Fatalf("field %s is missing", name)
		}
		if field.Type.Kind != kind {
			t.Errorf("%s has kind %s, want %s", name, field.Type.Kind, kind)
		}
	}
}

func TestEveryIntegerWidthIsAWholeNumber(t *testing.T) {
	schema := parseClean(t, `
type Props struct {
	A int8
	B int16
	C int32
	D int64
	E uint
	F uint64
	G rune
	H byte
}`)
	props, _ := schema.Props()
	for _, field := range props.Fields {
		if field.Type.Kind != KindInt {
			t.Errorf("%s has kind %s, want a whole number", field.Name, field.Type.Kind)
		}
	}
}

func TestSliceAndPointerFields(t *testing.T) {
	schema := parseClean(t, `
type Props struct {
	Tags  []string
	Badge *string
	Cards []Card
}
type Card struct {
	Title string
}`)
	props, _ := schema.Props()

	tags, _ := props.Field("Tags")
	if tags.Type.Kind != KindSlice || tags.Type.Elem.Kind != KindString {
		t.Errorf("Tags = %+v", tags.Type)
	}
	badge, _ := props.Field("Badge")
	if badge.Type.Kind != KindOptional || badge.Type.Elem.Kind != KindString {
		t.Errorf("Badge = %+v", badge.Type)
	}
	cards, _ := props.Field("Cards")
	if cards.Type.Kind != KindSlice || cards.Type.Elem.Kind != KindStruct {
		t.Errorf("Cards = %+v", cards.Type)
	}
}

func TestSeveralNamesOnOneLine(t *testing.T) {
	schema := parseClean(t, "type Props struct {\n\tA, B string\n}")
	props, _ := schema.Props()
	if len(props.Fields) != 2 {
		t.Errorf("fields = %v", props.FieldNames())
	}
}

func TestStructTags(t *testing.T) {
	schema := parseClean(t, "type Props struct {\n"+
		"\tVariant string `rill:\"default=secondary\"`\n"+
		"\tFooter  *Slot  `rill:\"slot\"`\n"+
		"\tAttrs   Attrs  `rill:\"rest\"`\n"+
		"}\ntype Slot struct{ X string }\ntype Attrs struct{ X string }")
	props, _ := schema.Props()

	variant, _ := props.Field("Variant")
	if variant.Default != "secondary" {
		t.Errorf("default = %q", variant.Default)
	}
	footer, _ := props.Field("Footer")
	if !footer.Slot {
		t.Error("the slot tag was not read")
	}
	attrs, _ := props.Field("Attrs")
	if !attrs.Rest {
		t.Error("the rest tag was not read")
	}
}

func TestUnknownTagOptionsAreIgnored(t *testing.T) {
	schema := parseClean(t, "type Props struct {\n\tA string `json:\"a\" rill:\"nonsense\"`\n}")
	props, _ := schema.Props()
	field, _ := props.Field("A")
	if field.Default != "" || field.Slot || field.Rest {
		t.Errorf("field = %+v", field)
	}
}

func TestSeveralStructsAreCollected(t *testing.T) {
	schema := parseClean(t, "type Props struct{ C Card }\ntype Card struct{ Title string }")
	if !schema.Has("Props") || !schema.Has("Card") {
		t.Errorf("structs = %v", schema.Order)
	}
	if !slices.Equal(schema.Order, []string{"Props", "Card"}) {
		t.Errorf("order = %v, want declaration order", schema.Order)
	}
}

func TestSeveralSourcesAreMerged(t *testing.T) {
	var bag diag.Bag
	schema := Parse([]Source{
		{File: "page.rill", Code: "type Props struct{ C Card }"},
		{File: "props.go", Code: "type Card struct{ Title string }"},
	}, &bag)
	if bag.HasErrors() {
		t.Fatalf("diagnostics: %+v", bag.Items())
	}
	if !schema.Has("Card") {
		t.Errorf("structs = %v", schema.Order)
	}
}

func TestNonStructDeclarationsAreSkipped(t *testing.T) {
	schema := parseClean(t, "type Alias = string\ntype Count int\nvar X = 1\nfunc F() {}\ntype Props struct{ A string }")
	if len(schema.Order) != 1 {
		t.Errorf("structs = %v, want only Props", schema.Order)
	}
}

func TestImportsAreAllowed(t *testing.T) {
	schema := parseClean(t, "import \"context\"\n\ntype Props struct{ A string }")
	if !schema.Has("Props") {
		t.Error("imports must not break parsing")
	}
}

func TestBrokenGoReportsC301(t *testing.T) {
	_, bag := parse(t, "type Props struct {")
	if !hasCode(bag, diag.C301) {
		t.Errorf("diagnostics = %+v, want C301", bag.Items())
	}
}

func TestUnsupportedTypesReportC302(t *testing.T) {
	cases := map[string]string{
		"map":       "type Props struct{ A map[string]int }",
		"interface": "type Props struct{ A interface{} }",
		"function":  "type Props struct{ A func() }",
		"channel":   "type Props struct{ A chan int }",
		"imported":  "type Props struct{ A time.Time }",
		"array":     "type Props struct{ A [3]int }",
		"embedded":  "type Props struct{ Card }",
	}
	for name, code := range cases {
		t.Run(name, func(t *testing.T) {
			_, bag := parse(t, code)
			if !hasCode(bag, diag.C302) {
				t.Errorf("diagnostics = %+v, want C302", bag.Items())
			}
		})
	}
}

func TestUnsupportedTypeIsNamedInTheMessage(t *testing.T) {
	_, bag := parse(t, "type Props struct{ A map[string]int }")
	if !strings.Contains(bag.Items()[0].Message, "a map") {
		t.Errorf("message = %q", bag.Items()[0].Message)
	}
	if bag.Items()[0].Help == "" {
		t.Error("the message must say what to do instead")
	}
}

func TestUnexportedFieldReportsC303(t *testing.T) {
	_, bag := parse(t, "type Props struct{ title string }")
	if !hasCode(bag, diag.C303) {
		t.Errorf("diagnostics = %+v, want C303", bag.Items())
	}
}

func TestUnknownStructReportsC304(t *testing.T) {
	_, bag := parse(t, "type Props struct{ C Card }")
	if !hasCode(bag, diag.C304) {
		t.Errorf("diagnostics = %+v, want C304", bag.Items())
	}
}

func TestUnknownStructInsideASliceIsReported(t *testing.T) {
	_, bag := parse(t, "type Props struct{ C []Card }")
	if !hasCode(bag, diag.C304) {
		t.Errorf("diagnostics = %+v, want C304", bag.Items())
	}
}

func TestUnknownStructSuggestsANearMiss(t *testing.T) {
	_, bag := parse(t, "type Props struct{ C Crad }\ntype Card struct{ A string }")
	if !strings.Contains(bag.Items()[0].Help, "Card") {
		t.Errorf("help = %q, want a suggestion", bag.Items()[0].Help)
	}
}

func TestResolveWalksNestedStructs(t *testing.T) {
	schema := parseClean(t, `
type Props struct {
	Listing Listing
	Cards   []Card
	Badge   *string
}
type Listing struct {
	Title string
	Owner Owner
}
type Owner struct {
	Name string
}
type Card struct {
	Title string
}`)
	cases := map[string]Kind{
		"Listing.Title":      KindString,
		"Listing.Owner.Name": KindString,
		"Cards.Title":        KindString,
	}
	for path, want := range cases {
		got, ok := schema.Resolve("Props", strings.Split(path, "."))
		if !ok {
			t.Fatalf("%s did not resolve", path)
		}
		if got.Kind != want {
			t.Errorf("%s resolved to %s, want %s", path, got.Kind, want)
		}
	}
}

func TestResolveRejectsUnknownPaths(t *testing.T) {
	schema := parseClean(t, "type Props struct{ Listing Listing }\ntype Listing struct{ Title string }")
	for _, path := range []string{"Missing", "Listing.Missing", "Listing.Title.Deeper"} {
		if _, ok := schema.Resolve("Props", strings.Split(path, ".")); ok {
			t.Errorf("%s must not resolve", path)
		}
	}
}

func TestResolveRejectsAnUnknownRoot(t *testing.T) {
	schema := parseClean(t, "type Props struct{ A string }")
	if _, ok := schema.Resolve("Nope", []string{"A"}); ok {
		t.Error("an unknown root must not resolve")
	}
}

func TestGoStringRoundTripsTypes(t *testing.T) {
	schema := parseClean(t, "type Props struct{ A []string\n B *Card\n C Card }\ntype Card struct{ X string }")
	props, _ := schema.Props()
	want := map[string]string{"A": "[]string", "B": "*Card", "C": "Card"}
	for name, text := range want {
		field, _ := props.Field(name)
		if got := field.Type.GoString(); got != text {
			t.Errorf("%s = %q, want %q", name, got, text)
		}
	}
}

func TestScalarReportsSimpleKinds(t *testing.T) {
	cases := map[Kind]bool{
		KindString: true, KindInt: true, KindFloat: true, KindBool: true,
		KindSlice: false, KindOptional: false, KindStruct: false, KindInvalid: false,
	}
	for kind, want := range cases {
		if got := (Type{Kind: kind}).Scalar(); got != want {
			t.Errorf("%s.Scalar() = %v, want %v", kind, got, want)
		}
	}
}

func TestKindNames(t *testing.T) {
	if KindSlice.String() != "slice" || Kind(99).String() != "unknown kind" {
		t.Error("kind names are wrong")
	}
}

func TestFieldLookupOnAMissingName(t *testing.T) {
	schema := parseClean(t, "type Props struct{ A string }")
	props, _ := schema.Props()
	if _, ok := props.Field("Nope"); ok {
		t.Error("Field must not invent members")
	}
	if !reflect.DeepEqual(props.FieldNames(), []string{"A"}) {
		t.Errorf("FieldNames = %v", props.FieldNames())
	}
}

func TestPropsIsOptional(t *testing.T) {
	schema := parseClean(t, "type Card struct{ A string }")
	if _, ok := schema.Props(); ok {
		t.Error("a block without Props must report none")
	}
}

func TestSuggestPicksTheNearestName(t *testing.T) {
	candidates := []string{"Listing", "Card", "Owner"}
	if got := Suggest("Crad", candidates); got != "Card" {
		t.Errorf("Suggest = %q, want Card", got)
	}
	if got := Suggest("zzzzzzzz", candidates); got != "" {
		t.Errorf("Suggest = %q, want no suggestion", got)
	}
	if got := Suggest("card", candidates); got != "Card" {
		t.Errorf("Suggest = %q, want a case-insensitive match", got)
	}
}

func TestEnumsAreCollected(t *testing.T) {
	schema := parseClean(t, `
type Status string

const (
	StatusActive Status = "active"
	StatusPaused Status = "paused"
	statusHidden Status = "hidden"
)

type Props struct {
	State Status
}`)
	enum, ok := schema.Enum("Status")
	if !ok {
		t.Fatalf("enums = %v", schema.Enums)
	}
	if !slices.Equal(enum.Members, []string{"StatusActive", "StatusPaused"}) {
		t.Errorf("members = %v, want the exported constants in order", enum.Members)
	}
}

func TestEnumFieldsAreMarked(t *testing.T) {
	schema := parseClean(t, "type Status int\nconst StatusA Status = 0\ntype Props struct{ State Status }")
	props, _ := schema.Props()
	field, _ := props.Field("State")
	if field.Type.Kind != KindEnum || field.Type.Name != "Status" {
		t.Errorf("field type = %+v, want an enum named Status", field.Type)
	}
}

func TestEnumWithoutConstantsIsStillAType(t *testing.T) {
	schema := parseClean(t, "type Status string\ntype Props struct{ State Status }")
	enum, ok := schema.Enum("Status")
	if !ok || len(enum.Members) != 0 {
		t.Errorf("enum = %+v, %v", enum, ok)
	}
}

func TestAliasesAreNotEnums(t *testing.T) {
	schema := parseClean(t, "type Name = string\ntype Props struct{ A string }")
	if _, ok := schema.Enum("Name"); ok {
		t.Error("an alias is not a named constant type")
	}
}

func TestNamedTypeOverAStructIsNotAnEnum(t *testing.T) {
	schema := parseClean(t, "type Card struct{ A string }\ntype Props struct{ C Card }")
	if _, ok := schema.Enum("Card"); ok {
		t.Error("a struct is not an enum")
	}
}

func TestConstantsWithoutATypeAreIgnored(t *testing.T) {
	schema := parseClean(t, "const Loose = 1\ntype Props struct{ A string }")
	if len(schema.Enums) != 0 {
		t.Errorf("enums = %v", schema.Enums)
	}
}

func TestEnumKindName(t *testing.T) {
	if KindEnum.String() != "enum" {
		t.Errorf("KindEnum = %q", KindEnum)
	}
}
