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

const AlternatesField = "Alternates"

type Meta struct {
	Title       string
	Description string
	Canonical   string
	Image       string
	Robots      string
	Alternates  Alternates
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
	case AlternatesField:
		return Seq(m.Alternates), true
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
