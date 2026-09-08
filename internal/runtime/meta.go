package runtime

const MetaRoot = "meta"

type Alternate struct {
	Lang string
	Href string
}

func (a Alternate) Get(path []string) (Value, bool) {
	if len(path) != 1 {
		return Nil(), false
	}
	switch path[0] {
	case "Lang":
		return String(a.Lang), true
	case "Href":
		return String(a.Href), true
	default:
		return Nil(), false
	}
}

type Alternates []Alternate

func (a Alternates) Len() int { return len(a) }

func (a Alternates) At(index int) Value {
	if index < 0 || index >= len(a) {
		return Nil()
	}
	return Object(a[index])
}

type Locales []string

func (l Locales) Len() int { return len(l) }

func (l Locales) At(index int) Value {
	if index < 0 || index >= len(l) {
		return Nil()
	}
	return String(l[index])
}

const AlternatesField = "Alternates"

const LocaleAlternatesField = "LocaleAlternates"

const HeadField = "Head"

type Meta struct {
	Title            string
	Description      string
	Canonical        string
	Image            string
	Robots           string
	Head             string
	Type             string
	URL              string
	Locale           string
	Card             string
	Site             string
	Alternates       Alternates
	LocaleAlternates Locales
}

func (m Meta) Get(path []string) (Value, bool) {
	if len(path) != 1 {
		return Nil(), false
	}
	switch path[0] {
	case "Title":
		return String(m.Title), true
	case "Description":
		return String(m.Description), true
	case "Canonical":
		return String(m.Canonical), true
	case "Image":
		return String(m.Image), true
	case "Robots":
		return String(m.Robots), true
	case "Type":
		return String(m.Type), true
	case "URL":
		return String(m.URL), true
	case "Locale":
		return String(m.Locale), true
	case "Card":
		return String(m.Card), true
	case "Site":
		return String(m.Site), true
	case HeadField:
		return String(m.Head), true
	case AlternatesField:
		return Seq(m.Alternates), true
	case LocaleAlternatesField:
		return Seq(m.LocaleAlternates), true
	default:
		return Nil(), false
	}
}

type rooted struct {
	props Accessible
	name  string
	root  Accessible
}

func WithRoot(props Accessible, name string, root Accessible) Accessible {
	return rooted{props: props, name: name, root: root}
}

func WithMeta(props Accessible, meta Meta) Accessible {
	return WithRoot(props, MetaRoot, meta)
}

func (r rooted) Get(path []string) (Value, bool) {
	if len(path) > 0 && path[0] == r.name {
		return r.root.Get(path[1:])
	}
	if r.props == nil {
		return Nil(), false
	}
	return r.props.Get(path)
}

type Leaf string

func (l Leaf) Get(path []string) (Value, bool) {
	if len(path) != 0 {
		return Nil(), false
	}
	return String(string(l)), true
}
