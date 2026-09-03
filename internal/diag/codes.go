package diag

type Code string

const (
	C001 Code = "C001"
	C002 Code = "C002"
	C004 Code = "C004"
	C005 Code = "C005"
	C006 Code = "C006"
	C201 Code = "C201"
	C202 Code = "C202"
	C301 Code = "C301"
	C302 Code = "C302"
	C303 Code = "C303"
	C304 Code = "C304"
	C305 Code = "C305"
	C306 Code = "C306"
	C307 Code = "C307"
	C308 Code = "C308"
	C309 Code = "C309"
	C310 Code = "C310"
	C101 Code = "C101"
	C102 Code = "C102"
	C103 Code = "C103"
	C104 Code = "C104"
	C105 Code = "C105"
	C311 Code = "C311"
	C312 Code = "C312"
	C313 Code = "C313"
	C314 Code = "C314"
	C315 Code = "C315"
	C316 Code = "C316"
	C503 Code = "C503"
	W703 Code = "W703"
	C601 Code = "C601"
	C602 Code = "C602"
	C111 Code = "C111"
	C603 Code = "C603"
	C317 Code = "C317"
	C318 Code = "C318"
	C319 Code = "C319"
	C320 Code = "C320"
	C321 Code = "C321"
	C322 Code = "C322"
	C323 Code = "C323"
	C324 Code = "C324"
)

var titles = map[Code]string{
	C001: "unterminated frontmatter fence",
	C002: "unterminated interpolation",
	C004: "unknown directive",
	C005: "unterminated directive",
	C006: "unbalanced block directive",
	C301: "unreadable go block",
	C302: "unsupported props type",
	C303: "unexported props field",
	C304: "unknown struct in props",
	C305: "unknown field in template",
	C306: "malformed component tag",
	C307: "unknown component",
	C308: "component argument mismatch",
	C309: "match is not exhaustive",
	C310: "malformed element attribute",
	C311: "malformed built-in component",
	C312: "malformed submit handler",
	C313: "unknown filter",
	C314: "malformed fragment",
	C315: "malformed island",
	C316: "malformed image",
	C317: "malformed client script",
	C318: "malformed deferred fragment",
	C319: "malformed fragment placeholder",
	C320: "private value on an island",
	C321: "value interpolated into a script",
	C322: "value interpolated into css",
	C323: "value interpolated into an event handler",
	C324: "value interpolated into srcdoc",
	C503: "private value in a cached fragment",
	W703: "class built at request time",
	C601: "missing translation",
	C602: "plural form mismatch",
	C201: "malformed expression",
	C202: "unclosed group",
	C101: "conflicting routes",
	C102: "page under the api namespace",
	C103: "outlet outside a layout",
	C104: "route handler without a method",
	C105: "malformed route handler",
	C111: "link points at no route",
	C603: "catalog in the old toml format",
}

func (c Code) Title() string {
	if title, ok := titles[c]; ok {
		return title
	}
	return "unknown diagnostic"
}

func (c Code) Known() bool {
	_, ok := titles[c]
	return ok
}

func Codes() []Code {
	codes := make([]Code, 0, len(titles))
	for code := range titles {
		codes = append(codes, code)
	}
	return codes
}
