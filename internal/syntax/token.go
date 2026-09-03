package syntax

import "github.com/apptivitypl/rill/internal/diag"

type Kind uint8

const (
	KindEOF Kind = iota
	KindText
	KindFrontmatter
	KindInterpStart
	KindInterpEnd
	KindDirectiveStart
	KindDirectiveEnd
	KindIdent
	KindString
	KindNumber
	KindDot
	KindComma
	KindAssign
	KindPipe
	KindLParen
	KindRParen
	KindLBracket
	KindRBracket
	KindOr
	KindAnd
	KindNot
	KindEq
	KindNe
	KindLt
	KindLe
	KindGt
	KindGe
	KindTilde
	KindPlus
	KindMinus
	KindStar
	KindSlash
	KindPercent
	KindColon
	KindHash
	KindComponentOpen
	KindComponentClose
	KindElementOpen
	KindElementClose
	KindQuestion
	KindTagEnd
	KindTagSelfClose
	KindClientScript
	KindUnexpected
)

var kindNames = map[Kind]string{
	KindEOF:            "end of file",
	KindText:           "text",
	KindFrontmatter:    "frontmatter",
	KindInterpStart:    "{{",
	KindInterpEnd:      "}}",
	KindDirectiveStart: "{%",
	KindDirectiveEnd:   "%}",
	KindIdent:          "identifier",
	KindString:         "string",
	KindNumber:         "number",
	KindDot:            ".",
	KindComma:          ",",
	KindAssign:         "=",
	KindPipe:           "|",
	KindLParen:         "(",
	KindRParen:         ")",
	KindLBracket:       "[",
	KindRBracket:       "]",
	KindOr:             "||",
	KindAnd:            "&&",
	KindNot:            "!",
	KindEq:             "==",
	KindNe:             "!=",
	KindLt:             "<",
	KindLe:             "<=",
	KindGt:             ">",
	KindGe:             ">=",
	KindTilde:          "~",
	KindPlus:           "+",
	KindMinus:          "-",
	KindStar:           "*",
	KindSlash:          "/",
	KindPercent:        "%",
	KindColon:          ":",
	KindHash:           "#",
	KindComponentOpen:  "component tag",
	KindComponentClose: "closing component tag",
	KindElementOpen:    "element tag",
	KindElementClose:   "closing element tag",
	KindQuestion:       "?",
	KindTagEnd:         ">",
	KindTagSelfClose:   "/>",
	KindClientScript:   "client script",
	KindUnexpected:     "unexpected input",
}

func (k Kind) String() string {
	if name, ok := kindNames[k]; ok {
		return name
	}
	return "unknown token"
}

type Token struct {
	Kind  Kind
	Span  diag.Span
	Text  string
	Value string
}

var operators = []struct {
	text string
	kind Kind
}{
	{"||", KindOr},
	{"&&", KindAnd},
	{"==", KindEq},
	{"!=", KindNe},
	{"<=", KindLe},
	{">=", KindGe},
	{"|", KindPipe},
	{"!", KindNot},
	{"<", KindLt},
	{">", KindGt},
	{"~", KindTilde},
	{"+", KindPlus},
	{"-", KindMinus},
	{"*", KindStar},
	{"/", KindSlash},
	{"%", KindPercent},
	{"(", KindLParen},
	{")", KindRParen},
	{"[", KindLBracket},
	{"]", KindRBracket},
	{",", KindComma},
	{"=", KindAssign},
	{":", KindColon},
	{"#", KindHash},
	{"?", KindQuestion},
	{".", KindDot},
}
