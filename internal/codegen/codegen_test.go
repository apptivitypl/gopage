package codegen

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/sonquer/rill/internal/diag"
	"github.com/sonquer/rill/internal/schema"
)

func build(t *testing.T, code string) *schema.Schema {
	t.Helper()
	var bag diag.Bag
	parsed := schema.Parse([]schema.Source{{File: "page.rill", Code: code}}, &bag)
	if bag.HasErrors() {
		t.Fatalf("schema: %+v", bag.Items())
	}
	return parsed
}

func render(t *testing.T, file File) string {
	t.Helper()
	if file.Package == "" {
		file.Package = "page"
	}
	out, err := Render(file)
	if err != nil {
		t.Fatalf("Render: %v\n%s", err, out)
	}
	return string(out)
}

func mustParse(t *testing.T, source string) {
	t.Helper()
	if _, err := parser.ParseFile(token.NewFileSet(), "gen.go", source, parser.SkipObjectResolution); err != nil {
		t.Fatalf("generated code does not parse: %v\n%s", err, source)
	}
}

func TestGeneratedFileParses(t *testing.T) {
	source := render(t, File{Schema: build(t, `
type Props struct {
	Title   string
	Count   int
	Ratio   float64
	Enabled bool
	Tags    []string
	Badge   *string
	Listing Listing
	Cards   []Card
	Owner   *Owner
}
type Listing struct{ Title string }
type Card struct{ Title string }
type Owner struct{ Name string }`)})
	mustParse(t, source)
}

func TestScalarAccessors(t *testing.T) {
	source := render(t, File{Schema: build(t, `
type Props struct {
	Title   string
	Count   int
	Ratio   float64
	Enabled bool
}`)})
	for _, want := range []string{
		`rill.String(string(v.Title))`,
		`rill.Int(int64(v.Count))`,
		`rill.Float(float64(v.Ratio))`,
		`rill.Bool(bool(v.Enabled))`,
	} {
		if !strings.Contains(source, want) {
			t.Errorf("generated code is missing %s:\n%s", want, source)
		}
	}
}

func TestNestedStructDelegates(t *testing.T) {
	source := render(t, File{Schema: build(t, "type Props struct{ Listing Listing }\ntype Listing struct{ Title string }")})
	if !strings.Contains(source, "return v.Listing.Get(path[1:])") {
		t.Errorf("nested struct does not delegate:\n%s", source)
	}
	if !strings.Contains(source, "rill.Object(v.Listing)") {
		t.Errorf("nested struct is not exposed as an object:\n%s", source)
	}
}

func TestSliceAccessors(t *testing.T) {
	source := render(t, File{Schema: build(t, `
type Props struct {
	Tags   []string
	Counts []int
	Ratios []float64
	Flags  []bool
	Cards  []Card
}
type Card struct{ Title string }`)})
	for _, want := range []string{
		"rill.Strings(v.Tags)",
		"rill.Ints[int](v.Counts)",
		"rill.Floats[float64](v.Ratios)",
		"rill.Bools(v.Flags)",
		"rill.Objects[Card](v.Cards)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("generated code is missing %s:\n%s", want, source)
		}
	}
}

func TestOptionalScalar(t *testing.T) {
	source := render(t, File{Schema: build(t, "type Props struct{ Badge *string }")})
	if !strings.Contains(source, "if v.Badge == nil") {
		t.Errorf("optional field is not nil-checked:\n%s", source)
	}
	if !strings.Contains(source, "rill.String(string((*v.Badge)))") {
		t.Errorf("optional field is not dereferenced:\n%s", source)
	}
}

func TestOptionalStructDelegates(t *testing.T) {
	source := render(t, File{Schema: build(t, "type Props struct{ Owner *Owner }\ntype Owner struct{ Name string }")})
	if !strings.Contains(source, "return (*v.Owner).Get(path[1:])") {
		t.Errorf("optional struct does not delegate:\n%s", source)
	}
}

func TestUnknownStructYieldsNothing(t *testing.T) {
	var bag diag.Bag
	parsed := schema.Parse([]schema.Source{{File: "p.rill", Code: "type Props struct{ C Card }"}}, &bag)
	source := render(t, File{Schema: parsed})
	mustParse(t, source)
	if !strings.Contains(source, "return rill.Nil(), false") {
		t.Errorf("an unresolvable field must resolve to nothing:\n%s", source)
	}
}

func TestFrontmatterIsCopiedWithALineDirective(t *testing.T) {
	source := render(t, File{
		SourceFile: "app/listings/[id]/page.rill",
		SourceLine: 2,
		Source:     "func Load() error { return nil }\n",
	})
	if !strings.Contains(source, "//line app/listings/[id]/page.rill:2") {
		t.Errorf("the line directive is missing:\n%s", source)
	}
	if !strings.Contains(source, "func Load() error") {
		t.Errorf("the frontmatter was not copied:\n%s", source)
	}
	mustParse(t, source)
}

func TestEmptyFrontmatterAddsNoDirective(t *testing.T) {
	source := render(t, File{Source: "   \n"})
	if strings.Contains(source, "//line") {
		t.Errorf("an empty block must not produce a directive:\n%s", source)
	}
}

func TestSourceWithoutTrailingNewlineIsClosed(t *testing.T) {
	source := render(t, File{SourceFile: "p.rill", SourceLine: 1, Source: "var X = 1"})
	mustParse(t, source)
}

func TestImportsAreSortedAndDeduplicated(t *testing.T) {
	source := render(t, File{Imports: []string{"example.com/z", "example.com/a", "example.com/a"}})
	position := func(needle string) int { return strings.Index(source, needle) }
	if position(`"example.com/a"`) > position(`"example.com/z"`) {
		t.Errorf("imports are not sorted:\n%s", source)
	}
	if strings.Count(source, `"example.com/a"`) != 1 {
		t.Errorf("duplicate import:\n%s", source)
	}
	if !strings.Contains(source, `"`+RillImport+`"`) {
		t.Errorf("the rill import is missing:\n%s", source)
	}
}

func TestPackageClause(t *testing.T) {
	source := render(t, File{Package: "listings_id"})
	if !strings.HasPrefix(source, "// Code generated by rill. DO NOT EDIT.\n\npackage listings_id") {
		t.Errorf("package clause is wrong:\n%s", source)
	}
}

func TestBrokenSourceIsReportedWithTheText(t *testing.T) {
	out, err := Render(File{Package: "page", SourceFile: "p.rill", SourceLine: 1, Source: "func ("})
	if err == nil {
		t.Fatal("expected an error for source that cannot be formatted")
	}
	if !strings.Contains(string(out), "func (") {
		t.Error("the unformatted text must come back so the caller can show it")
	}
}

func TestPackageNameIsAValidIdentifier(t *testing.T) {
	cases := map[string]string{
		"index":       "index",
		"listings.id": "listings_id",
		"api.health":  "api_health",
		"Docs.Slug":   "docs_slug",
		"":            "route_",
		"1st":         "route_1st",
	}
	for input, want := range cases {
		if got := PackageName(input); got != want {
			t.Errorf("PackageName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestEnumCaseMethodIsGenerated(t *testing.T) {
	source := render(t, File{Schema: build(t, `
type Status string

const (
	StatusActive Status = "active"
	StatusPaused Status = "paused"
)

type Props struct {
	State  Status
	States []Status
}`)})
	mustParse(t, source)
	for _, want := range []string{
		"func (v Status) rillCase() string",
		`case StatusActive:`,
		`return "StatusActive"`,
		"rill.String(v.State.rillCase())",
		"rill.Cases[Status](v.States)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("generated code is missing %s:\n%s", want, source)
		}
	}
}

func TestEnumWithoutConstantsGetsNoMethod(t *testing.T) {
	source := render(t, File{Schema: build(t, "type Status string\ntype Props struct{ State Status }")})
	if strings.Contains(source, "rillCase") {
		t.Errorf("an enum with no constants needs no mapping:\n%s", source)
	}
}

func TestEnumsAreGeneratedInASortedOrder(t *testing.T) {
	source := render(t, File{Schema: build(t, `
type Zeta string
type Alpha string

const (
	ZetaOne  Zeta  = "z"
	AlphaOne Alpha = "a"
)

type Props struct {
	A Alpha
	Z Zeta
}`)})
	if strings.Index(source, "func (v Alpha)") > strings.Index(source, "func (v Zeta)") {
		t.Errorf("enums are not sorted:\n%s", source)
	}
}

func TestFrontmatterImportsAreHoisted(t *testing.T) {
	source, err := Render(File{
		Package:    "page",
		SourceFile: "app/page.rill",
		SourceLine: 2,
		Source: `import "time"

type Props struct {
	At time.Time
}
`,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	text := string(source)
	if !strings.Contains(text, "\t\"time\"\n") {
		t.Errorf("generated = %q, want time in the import block", text)
	}
	if strings.Count(text, "import (") != 1 {
		t.Errorf("generated = %q, want a single import block", text)
	}
}

func TestGroupedAndAliasedImportsAreHoisted(t *testing.T) {
	source, err := Render(File{
		Package:    "page",
		SourceFile: "app/page.rill",
		SourceLine: 2,
		Source: `import (
	"time"
	clock "time"
)

type Props struct {
	At time.Time
	Second clock.Time
}
`,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(source), `clock "time"`) {
		t.Errorf("generated = %q", source)
	}
}

func TestLineDirectivesSurviveTheHoist(t *testing.T) {
	source, err := Render(File{
		Package:    "page",
		SourceFile: "app/page.rill",
		SourceLine: 2,
		Source: `import "time"

type Props struct {
	At time.Time
}
`,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(source), "//line app/page.rill:3\n\ntype Props struct {") {
		t.Errorf("generated = %q, want the blank line after the import to keep the mapping", source)
	}
}

func TestSplitImportsLeavesUnparsableSourceAlone(t *testing.T) {
	imports, parts := SplitImports("func Load( {")
	if imports != nil || len(parts) != 1 || parts[0].Line != 1 {
		t.Errorf("imports = %v, parts = %+v", imports, parts)
	}
}

func TestSplitImportsWithoutAnyImports(t *testing.T) {
	imports, parts := SplitImports("type Props struct{}\n")
	if imports != nil || len(parts) != 1 {
		t.Errorf("imports = %v, parts = %+v", imports, parts)
	}
}

func TestAnEmptyFrontmatterEmitsNoDirective(t *testing.T) {
	source, err := Render(File{Package: "page", SourceFile: "app/page.rill", SourceLine: 2, Source: "   \n"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(string(source), "//line") {
		t.Errorf("generated = %q", source)
	}
}

func TestAFrontmatterThatOpensWithABlankLineKeepsItsMapping(t *testing.T) {
	imports, parts := SplitImports("\nimport \"time\"\n\ntype Props struct {\n\tAt time.Time\n}\n")
	if len(imports) != 1 {
		t.Fatalf("imports = %v", imports)
	}
	if len(parts) != 2 || parts[0].Line != 1 || parts[1].Line != 3 {
		t.Errorf("parts = %+v, want the text before and after the import kept apart", parts)
	}
}

func TestComputedFieldsAreCalled(t *testing.T) {
	model := &schema.Schema{
		Structs: map[string]schema.Struct{
			"Props": {Name: "Props", Fields: []schema.Field{
				{Name: "Price", Type: schema.Type{Kind: schema.KindInt, Name: "int64"}},
				{Name: "PricePerM2", Type: schema.Type{Kind: schema.KindInt, Name: "int64"}, Computed: true},
			}},
		},
		Order: []string{"Props"},
	}
	source, err := Render(File{Package: "page", Schema: model})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	text := string(source)
	if !strings.Contains(text, "rill.Int(int64(v.PricePerM2()))") {
		t.Errorf("generated = %q, want the method called", text)
	}
	if !strings.Contains(text, "rill.Int(int64(v.Price))") {
		t.Errorf("generated = %q, want the stored field read directly", text)
	}
}

func TestTypeScriptMirrorsTheSchema(t *testing.T) {
	model := &schema.Schema{
		Structs: map[string]schema.Struct{
			"Props": {Name: "Props", Fields: []schema.Field{
				{Name: "Label", Type: schema.Type{Kind: schema.KindString}},
				{Name: "Count", Type: schema.Type{Kind: schema.KindInt}},
				{Name: "Ratio", Type: schema.Type{Kind: schema.KindFloat}},
				{Name: "Compact", Type: schema.Type{Kind: schema.KindBool}},
				{Name: "State", Type: schema.Type{Kind: schema.KindEnum, Name: "State"}},
				{Name: "Tags", Type: schema.Type{Kind: schema.KindSlice, Elem: &schema.Type{Kind: schema.KindString}}},
				{Name: "Note", Type: schema.Type{Kind: schema.KindOptional, Elem: &schema.Type{Kind: schema.KindString}}},
				{Name: "Card", Type: schema.Type{Kind: schema.KindStruct, Name: "Card"}},
				{Name: "Loose", Type: schema.Type{Kind: schema.KindStruct, Name: "Unknown"}},
			}},
			"Card": {Name: "Card", Fields: []schema.Field{{Name: "Title", Type: schema.Type{Kind: schema.KindString}}}},
		},
		Enums: map[string]schema.Enum{"State": {Name: "State", Members: []string{"StateReady", "StatePlanned"}}},
		Order: []string{"Props", "Card"},
	}
	got := TypeScript(model)
	for _, want := range []string{
		`export type State = "StateReady" | "StatePlanned";`,
		"export interface Props {",
		"\tLabel: string;",
		"\tCount: number;",
		"\tRatio: number;",
		"\tCompact: boolean;",
		"\tState: State;",
		"\tTags: string[];",
		"\tNote?: string | null;",
		"\tCard: Card;",
		"\tLoose: unknown;",
		"export interface Card {",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("typescript = %q, want %q", got, want)
		}
	}
}

func TestAnEnumWithoutMembersFallsBackToString(t *testing.T) {
	model := &schema.Schema{
		Enums: map[string]schema.Enum{"State": {Name: "State"}},
		Order: nil,
	}
	if got := TypeScript(model); !strings.Contains(got, "export type State = string;") {
		t.Errorf("typescript = %q", got)
	}
}

func TestTypeScriptWithoutASchema(t *testing.T) {
	if got := TypeScript(nil); got != "" {
		t.Errorf("typescript = %q", got)
	}
}

func TestTypeScriptSkipsAMissingStruct(t *testing.T) {
	model := &schema.Schema{Structs: map[string]schema.Struct{}, Order: []string{"Gone"}}
	if got := TypeScript(model); strings.Contains(got, "Gone") {
		t.Errorf("typescript = %q", got)
	}
}

func TestAnUnknownKindIsUnknown(t *testing.T) {
	model := &schema.Schema{
		Structs: map[string]schema.Struct{
			"Props": {Name: "Props", Fields: []schema.Field{{Name: "Odd", Type: schema.Type{Kind: 99}}}},
		},
		Order: []string{"Props"},
	}
	if got := TypeScript(model); !strings.Contains(got, "Odd: unknown;") {
		t.Errorf("typescript = %q", got)
	}
}

func TestDeferredFieldsGetTheirOwnAccessor(t *testing.T) {
	source := `
type Review struct {
	Author string
}

type Props struct {
	Heading string
}

func Reviews(ctx *rill.Ctx) ([]Review, error) {
	return nil, nil
}

func Count(ctx *rill.Ctx) (int, error) {
	return 0, nil
}

func One(ctx *rill.Ctx) (Review, error) {
	return Review{}, nil
}
`
	var bag diag.Bag
	model := schema.Parse([]schema.Source{{File: "app/page.rill", Code: source}}, &bag)
	out, err := Render(File{Package: "index", SourceFile: "app/page.rill", SourceLine: 2, Source: source, Schema: model})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	code := string(out)
	for _, want := range []string{
		"type deferredReviews struct",
		"rill.Seq(rill.Objects[Review](d.value))",
		"type deferredCount struct",
		"rill.Int(int64(d.value))",
		"type deferredOne struct",
		"return d.value.Get(path)",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("generated code is missing %q:\n%s", want, code)
		}
	}
	if strings.Contains(code, `case "Reviews":`) {
		t.Error("a deferred field must not be read from the props struct")
	}
	mustParse(t, code)
}

func TestDeferredWrappersCoverEveryKind(t *testing.T) {
	source := `
type Row struct {
	Name string
}

type Tone string

const (
	ToneOk Tone = "ok"
)

type Props struct {
	Heading string
}

func Text(ctx *rill.Ctx) (string, error) {
	return "", nil
}

func Ratio(ctx *rill.Ctx) (float64, error) {
	return 0, nil
}

func Ready(ctx *rill.Ctx) (bool, error) {
	return false, nil
}

func Names(ctx *rill.Ctx) ([]string, error) {
	return nil, nil
}

func Rows(ctx *rill.Ctx) ([]Row, error) {
	return nil, nil
}

func Maybe(ctx *rill.Ctx) (*Row, error) {
	return nil, nil
}
`
	var bag diag.Bag
	model := schema.Parse([]schema.Source{{File: "app/page.rill", Code: source}}, &bag)
	out, err := Render(File{Package: "index", SourceFile: "app/page.rill", SourceLine: 2, Source: source, Schema: model})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	code := string(out)
	for _, want := range []string{
		"type deferredText struct {\n\tvalue string",
		"type deferredRatio struct {\n\tvalue float64",
		"type deferredReady struct {\n\tvalue bool",
		"type deferredNames struct {\n\tvalue []string",
		"type deferredRows struct {\n\tvalue []Row",
		"type deferredMaybe struct {\n\tvalue *Row",
		"rill.Strings(d.value)",
		"rill.Objects[Row](d.value)",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("generated code is missing %q", want)
		}
	}
	mustParse(t, code)
}

func TestDeferredWrappersHandleNamedAndUnknownTypes(t *testing.T) {
	source := `
type Tone string

const (
	ToneOk Tone = "ok"
)

type Props struct {
	Heading string
}

func Badge(ctx *rill.Ctx) (Tone, error) {
	return ToneOk, nil
}

func Tones(ctx *rill.Ctx) ([]Tone, error) {
	return nil, nil
}
`
	var bag diag.Bag
	model := schema.Parse([]schema.Source{{File: "app/page.rill", Code: source}}, &bag)
	out, err := Render(File{Package: "index", SourceFile: "app/page.rill", SourceLine: 2, Source: source, Schema: model})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	code := string(out)
	for _, want := range []string{"type deferredBadge struct {\n\tvalue Tone", "rill.Cases[Tone](d.value)"} {
		if !strings.Contains(code, want) {
			t.Errorf("generated code is missing %q:\n%s", want, code)
		}
	}
	mustParse(t, code)
}

func TestADeferredTypeOutsideTheSchemaYieldsNothing(t *testing.T) {
	source := `
type Props struct {
	Heading string
}

func Client(ctx *rill.Ctx) (http.Client, error) {
	return http.Client{}, nil
}

func Blobs(ctx *rill.Ctx) ([]http.Client, error) {
	return nil, nil
}
`
	var bag diag.Bag
	model := schema.Parse([]schema.Source{{File: "app/page.rill", Code: source}}, &bag)
	out, err := Render(File{Package: "index", SourceFile: "app/page.rill", SourceLine: 2, Source: source, Schema: model})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(string(out), "rill.Nil()") {
		t.Errorf("generated code = %s, want a nil value for a type the schema does not know", out)
	}
}

func TestDeferredNamedScalarsKeepTheirType(t *testing.T) {
	source := `
type Count int

type Props struct {
	Heading string
}

func Total(ctx *rill.Ctx) (Count, error) {
	return 0, nil
}

func Totals(ctx *rill.Ctx) ([]Count, error) {
	return nil, nil
}
`
	var bag diag.Bag
	model := schema.Parse([]schema.Source{{File: "app/page.rill", Code: source}}, &bag)
	out, err := Render(File{Package: "index", SourceFile: "app/page.rill", SourceLine: 2, Source: source, Schema: model})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	code := string(out)
	if !strings.Contains(code, "type deferredTotal struct {\n\tvalue Count") {
		t.Errorf("generated code = %s, want the named type kept", code)
	}
	if !strings.Contains(code, "type deferredTotals struct {\n\tvalue []Count") {
		t.Errorf("generated code = %s, want the named element type kept", code)
	}
	mustParse(t, code)
}
