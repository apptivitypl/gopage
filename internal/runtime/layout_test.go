package runtime

import "testing"

func TestLayeredWrapsOnlyWhereThereAreProps(t *testing.T) {
	props := Map{"Title": String("home")}
	if got := layered(props, 0, &Options{}); got.(Map) == nil {
		t.Error("no layout props means the page props pass through")
	}
	opts := &Options{Layouts: []Accessible{Map{"Home": String("/")}, nil}}
	wrapped := layered(props, 0, opts)
	if value, ok := wrapped.Get([]string{LayoutRoot, "Home"}); !ok || value.Str != "/" {
		t.Errorf("layout.Home = %+v, ok = %v", value, ok)
	}
	if value, ok := wrapped.Get([]string{"Title"}); !ok || value.Str != "home" {
		t.Errorf("Title = %+v, ok = %v", value, ok)
	}
	if got := layered(props, 1, opts); got.(Map) == nil {
		t.Error("a nil entry leaves the props alone")
	}
	if got := layered(props, 9, opts); got.(Map) == nil {
		t.Error("an index past the chain leaves the props alone")
	}
}
