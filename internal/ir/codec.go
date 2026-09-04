package ir

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

var magic = [4]byte{'R', 'I', 'L', 'L'}

var ErrBadMagic = errors.New("not a gopage manifest")

func Encode(m *Manifest) []byte {
	var w writer
	w.bytes(magic[:])
	w.u32(m.Version)
	w.u32(uint32(len(m.Plans)))
	for i := range m.Plans {
		encodePlan(&w, &m.Plans[i])
	}
	w.u32(uint32(len(m.Routes)))
	for _, route := range m.Routes {
		encodeRoute(&w, route)
	}
	w.u32(uint32(len(m.Messages)))
	for _, key := range m.Messages {
		w.str(key)
	}
	w.u32(uint32(len(m.Catalogs)))
	for _, catalog := range m.Catalogs {
		w.str(catalog.Locale)
		w.u32(uint32(len(catalog.Texts)))
		for _, forms := range catalog.Texts {
			for _, text := range forms {
				w.str(text)
			}
		}
	}
	w.u32(uint32(len(m.Fallbacks)))
	for _, fallback := range m.Fallbacks {
		w.str(fallback.Prefix)
		w.str(fallback.Name)
		w.u8(uint8(fallback.Kind))
		w.u32(fallback.Plan)
		w.u32(uint32(len(fallback.LayoutChain)))
		for _, index := range fallback.LayoutChain {
			w.u32(index)
		}
	}
	return w.buf
}

func encodePlan(w *writer, p *Plan) {
	w.u32(uint32(len(p.Ops)))
	for _, op := range p.Ops {
		w.u8(uint8(op.Kind))
		w.u32(op.A)
		w.u32(op.B)
		w.u32(op.C)
	}
	w.u32(uint32(len(p.Exprs)))
	for _, expr := range p.Exprs {
		w.u8(uint8(expr.Kind))
		w.u8(expr.Op)
		w.u32(expr.A)
		w.u32(expr.B)
	}
	w.u32(uint32(len(p.Consts)))
	for _, value := range p.Consts {
		w.u8(uint8(value.Kind))
		w.str(value.Str)
		w.u64(uint64(value.Int))
		w.f64(value.Float)
	}
	w.u32(p.Locals)
	w.u32(uint32(len(p.Blob)))
	w.bytes(p.Blob)
	w.u32(uint32(len(p.Paths)))
	for _, path := range p.Paths {
		w.u32(uint32(len(path)))
		for _, segment := range path {
			w.str(segment)
		}
	}
	w.u32(uint32(len(p.Messages)))
	for _, key := range p.Messages {
		w.str(key)
	}
	w.u32(uint32(len(p.Islands)))
	for _, island := range p.Islands {
		w.str(island.Name)
		w.str(island.Strategy)
	}
	w.u32(uint32(len(p.Fragments)))
	for _, fragment := range p.Fragments {
		w.str(fragment.Name)
		w.u64(uint64(fragment.TTL))
		w.u64(uint64(fragment.Stale))
		w.u8(deferredFlag(fragment.Deferred))
		w.str(fragment.Strategy)
		w.u32(fragment.BodyEnd)
		w.u32(fragment.Hold)
		w.u32(fragment.HoldEnd)
		w.u32(uint32(len(fragment.Paths)))
		for _, index := range fragment.Paths {
			w.u32(index)
		}
	}
	w.u32(p.Capacity)
}

func encodeRoute(w *writer, r Route) {
	w.str(r.Pattern)
	w.str(r.Name)
	w.u32(r.Plan)
	w.u32(uint32(len(r.LayoutChain)))
	for _, index := range r.LayoutChain {
		w.u32(index)
	}
	w.u8(uint8(r.Class))
}

func Decode(data []byte) (*Manifest, error) {
	r := &reader{buf: data}
	head, err := r.bytes(len(magic))
	if err != nil || string(head) != string(magic[:]) {
		return nil, ErrBadMagic
	}
	m := &Manifest{}
	if m.Version, err = r.u32(); err != nil {
		return nil, err
	}
	if m.Version != Version {
		return nil, fmt.Errorf("manifest version %d, this binary reads version %d", m.Version, Version)
	}
	planCount, err := r.count(minPlanSize)
	if err != nil {
		return nil, err
	}
	m.Plans = makeSlice[Plan](planCount)
	for i := range m.Plans {
		if m.Plans[i], err = decodePlan(r); err != nil {
			return nil, err
		}
	}
	routeCount, err := r.count(minRouteSize)
	if err != nil {
		return nil, err
	}
	m.Routes = makeSlice[Route](routeCount)
	for i := range m.Routes {
		if m.Routes[i], err = decodeRoute(r); err != nil {
			return nil, err
		}
	}
	messageCount, err := r.count(lengthSize)
	if err != nil {
		return nil, err
	}
	m.Messages = makeSlice[string](messageCount)
	for i := range m.Messages {
		if m.Messages[i], err = r.str(); err != nil {
			return nil, err
		}
	}
	catalogCount, err := r.count(minCatalogSize)
	if err != nil {
		return nil, err
	}
	m.Catalogs = makeSlice[Catalog](catalogCount)
	for i := range m.Catalogs {
		if m.Catalogs[i], err = decodeCatalog(r); err != nil {
			return nil, err
		}
	}
	fallbackCount, err := r.count(minFallbackSize)
	if err != nil {
		return nil, err
	}
	m.Fallbacks = makeSlice[Fallback](fallbackCount)
	for i := range m.Fallbacks {
		if m.Fallbacks[i], err = decodeFallback(r); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func decodeOp(r *reader) (Op, error) {
	record, err := r.bytes(opSize)
	if err != nil {
		return Op{}, err
	}
	return Op{
		Kind: OpKind(record[0]),
		A:    binary.LittleEndian.Uint32(record[1:]),
		B:    binary.LittleEndian.Uint32(record[5:]),
		C:    binary.LittleEndian.Uint32(record[9:]),
	}, nil
}

func decodeExpr(r *reader) (ExprNode, error) {
	record, err := r.bytes(exprSize)
	if err != nil {
		return ExprNode{}, err
	}
	return ExprNode{
		Kind: ExprKind(record[0]),
		Op:   record[1],
		A:    binary.LittleEndian.Uint32(record[2:]),
		B:    binary.LittleEndian.Uint32(record[6:]),
	}, nil
}

func decodeConst(r *reader) (Const, error) {
	kind, err := r.bytes(1)
	if err != nil {
		return Const{}, err
	}
	str, err := r.str()
	if err != nil {
		return Const{}, err
	}
	tail, err := r.bytes(16)
	if err != nil {
		return Const{}, err
	}
	return Const{
		Kind:  ConstKind(kind[0]),
		Str:   str,
		Int:   int64(binary.LittleEndian.Uint64(tail)),
		Float: math.Float64frombits(binary.LittleEndian.Uint64(tail[8:])),
	}, nil
}

func decodeCatalog(r *reader) (Catalog, error) {
	var catalog Catalog
	var err error
	if catalog.Locale, err = r.str(); err != nil {
		return catalog, err
	}
	textCount, err := r.count(PluralForms * lengthSize)
	if err != nil {
		return catalog, err
	}
	catalog.Texts = makeSlice[[PluralForms]string](textCount)
	for i := range catalog.Texts {
		for form := range PluralForms {
			if catalog.Texts[i][form], err = r.str(); err != nil {
				return catalog, err
			}
		}
	}
	return catalog, nil
}

func decodeFallback(r *reader) (Fallback, error) {
	var fallback Fallback
	var err error
	if fallback.Prefix, err = r.str(); err != nil {
		return fallback, err
	}
	if fallback.Name, err = r.str(); err != nil {
		return fallback, err
	}
	kind, err := r.bytes(1)
	if err != nil {
		return fallback, err
	}
	fallback.Kind = FallbackKind(kind[0])
	if fallback.Plan, err = r.u32(); err != nil {
		return fallback, err
	}
	chainLen, err := r.count(lengthSize)
	if err != nil {
		return fallback, err
	}
	fallback.LayoutChain = makeSlice[uint32](chainLen)
	for i := range fallback.LayoutChain {
		if fallback.LayoutChain[i], err = r.u32(); err != nil {
			return fallback, err
		}
	}
	return fallback, nil
}

func deferredFlag(deferred bool) uint8 {
	if deferred {
		return 1
	}
	return 0
}

func decodeFragment(r *reader) (Fragment, error) {
	var fragment Fragment
	var err error
	if fragment.Name, err = r.str(); err != nil {
		return fragment, err
	}
	window, err := r.bytes(16)
	if err != nil {
		return fragment, err
	}
	fragment.TTL = int64(binary.LittleEndian.Uint64(window))
	fragment.Stale = int64(binary.LittleEndian.Uint64(window[8:]))
	flag, err := r.u8()
	if err != nil {
		return fragment, err
	}
	fragment.Deferred = flag == 1
	if fragment.Strategy, err = r.str(); err != nil {
		return fragment, err
	}
	if fragment.BodyEnd, err = r.u32(); err != nil {
		return fragment, err
	}
	if fragment.Hold, err = r.u32(); err != nil {
		return fragment, err
	}
	if fragment.HoldEnd, err = r.u32(); err != nil {
		return fragment, err
	}
	pathCount, err := r.count(lengthSize)
	if err != nil {
		return fragment, err
	}
	fragment.Paths = makeSlice[uint32](pathCount)
	for i := range fragment.Paths {
		if fragment.Paths[i], err = r.u32(); err != nil {
			return fragment, err
		}
	}
	return fragment, nil
}

func decodePlan(r *reader) (Plan, error) {
	var p Plan
	opCount, err := r.count(opSize)
	if err != nil {
		return p, err
	}
	p.Ops = makeSlice[Op](opCount)
	for i := range p.Ops {
		if p.Ops[i], err = decodeOp(r); err != nil {
			return p, err
		}
	}
	exprCount, err := r.count(exprSize)
	if err != nil {
		return p, err
	}
	p.Exprs = makeSlice[ExprNode](exprCount)
	for i := range p.Exprs {
		if p.Exprs[i], err = decodeExpr(r); err != nil {
			return p, err
		}
	}
	constCount, err := r.count(minConstSize)
	if err != nil {
		return p, err
	}
	p.Consts = makeSlice[Const](constCount)
	for i := range p.Consts {
		if p.Consts[i], err = decodeConst(r); err != nil {
			return p, err
		}
	}
	if p.Locals, err = r.u32(); err != nil {
		return p, err
	}
	blobLen, err := r.count(1)
	if err != nil {
		return p, err
	}
	if p.Blob, err = r.blob(int(blobLen)); err != nil {
		return p, err
	}
	pathCount, err := r.count(lengthSize)
	if err != nil {
		return p, err
	}
	p.Paths = makeSlice[[]string](pathCount)
	for i := range p.Paths {
		segmentCount, err := r.count(lengthSize)
		if err != nil {
			return p, err
		}
		path := makeSlice[string](segmentCount)
		for j := range path {
			if path[j], err = r.str(); err != nil {
				return p, err
			}
		}
		p.Paths[i] = path
	}
	planMessages, err := r.count(lengthSize)
	if err != nil {
		return p, err
	}
	p.Messages = makeSlice[string](planMessages)
	for i := range p.Messages {
		if p.Messages[i], err = r.str(); err != nil {
			return p, err
		}
	}
	islandCount, err := r.count(minIslandSize)
	if err != nil {
		return p, err
	}
	p.Islands = makeSlice[IslandUse](islandCount)
	for i := range p.Islands {
		if p.Islands[i].Name, err = r.str(); err != nil {
			return p, err
		}
		if p.Islands[i].Strategy, err = r.str(); err != nil {
			return p, err
		}
	}
	fragmentCount, err := r.count(minFragmentSize)
	if err != nil {
		return p, err
	}
	p.Fragments = makeSlice[Fragment](fragmentCount)
	for i := range p.Fragments {
		if p.Fragments[i], err = decodeFragment(r); err != nil {
			return p, err
		}
	}
	if p.Capacity, err = r.u32(); err != nil {
		return p, err
	}
	return p, nil
}

func decodeRoute(r *reader) (Route, error) {
	var route Route
	var err error
	if route.Pattern, err = r.str(); err != nil {
		return route, err
	}
	if route.Name, err = r.str(); err != nil {
		return route, err
	}
	if route.Plan, err = r.u32(); err != nil {
		return route, err
	}
	chainLen, err := r.count(lengthSize)
	if err != nil {
		return route, err
	}
	route.LayoutChain = makeSlice[uint32](chainLen)
	for i := range route.LayoutChain {
		if route.LayoutChain[i], err = r.u32(); err != nil {
			return route, err
		}
	}
	class, err := r.bytes(1)
	if err != nil {
		return route, err
	}
	route.Class = RouteClass(class[0])
	return route, nil
}

const (
	lengthSize      = 4
	opSize          = 13
	exprSize        = 10
	minConstSize    = 21
	minPlanSize     = 28
	minRouteSize    = 17
	minFallbackSize = 17
	minFragmentSize = 24
	minIslandSize   = 8
	minCatalogSize  = 8
)

func makeSlice[T any](n uint32) []T {
	if n == 0 {
		return nil
	}
	return make([]T, n)
}

type writer struct {
	buf []byte
}

func (w *writer) u8(v uint8) {
	w.buf = append(w.buf, v)
}

func (w *writer) u32(v uint32) {
	w.buf = binary.LittleEndian.AppendUint32(w.buf, v)
}

func (w *writer) u64(v uint64) {
	w.buf = binary.LittleEndian.AppendUint64(w.buf, v)
}

func (w *writer) f64(v float64) {
	w.u64(math.Float64bits(v))
}

func (w *writer) bytes(b []byte) {
	w.buf = append(w.buf, b...)
}

func (w *writer) str(s string) {
	w.u32(uint32(len(s)))
	w.buf = append(w.buf, s...)
}

type reader struct {
	buf []byte
	pos int
}

func (r *reader) u8() (uint8, error) {
	window, err := r.bytes(1)
	if err != nil {
		return 0, err
	}
	return window[0], nil
}

func (r *reader) u32() (uint32, error) {
	if r.pos+4 > len(r.buf) {
		return 0, io.ErrUnexpectedEOF
	}
	v := binary.LittleEndian.Uint32(r.buf[r.pos:])
	r.pos += 4
	return v, nil
}

func (r *reader) bytes(n int) ([]byte, error) {
	if n < 0 || r.pos+n > len(r.buf) {
		return nil, io.ErrUnexpectedEOF
	}
	out := r.buf[r.pos : r.pos+n]
	r.pos += n
	return out, nil
}

func (r *reader) blob(n int) ([]byte, error) {
	if n == 0 {
		return nil, nil
	}
	return r.bytes(n)
}

func (r *reader) remaining() int {
	return len(r.buf) - r.pos
}

func (r *reader) count(minElementSize int) (uint32, error) {
	n, err := r.u32()
	if err != nil {
		return 0, err
	}
	if int64(n)*int64(minElementSize) > int64(r.remaining()) {
		return 0, fmt.Errorf("%w: declared %d elements but only %d bytes remain",
			io.ErrUnexpectedEOF, n, r.remaining())
	}
	return n, nil
}

func (r *reader) str() (string, error) {
	n, err := r.u32()
	if err != nil {
		return "", err
	}
	b, err := r.bytes(int(n))
	if err != nil {
		return "", err
	}
	return string(b), nil
}
