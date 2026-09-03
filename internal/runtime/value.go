package runtime

import (
	"math"
	"strconv"
)

type ValueKind uint8

const (
	KindNil ValueKind = iota
	KindString
	KindInt
	KindFloat
	KindBool
	KindSeq
	KindObject
)

type Sequence interface {
	Len() int
	At(index int) Value
}

type Accessible interface {
	Get(path []string) (Value, bool)
}

type Value struct {
	Kind ValueKind
	Str  string
	num  uint64
	ref  any
}

func (v Value) Int() int64 {
	return int64(v.num)
}

func (v Value) Float() float64 {
	return math.Float64frombits(v.num)
}

func (v Value) Sequence() Sequence {
	seq, _ := v.ref.(Sequence)
	return seq
}

func (v Value) Object() Accessible {
	object, _ := v.ref.(Accessible)
	return object
}

func String(s string) Value {
	return Value{Kind: KindString, Str: s}
}

func Int(n int64) Value {
	return Value{Kind: KindInt, num: uint64(n)}
}

func Float(f float64) Value {
	return Value{Kind: KindFloat, num: math.Float64bits(f)}
}

func Bool(b bool) Value {
	var n uint64
	if b {
		n = 1
	}
	return Value{Kind: KindBool, num: n}
}

func Nil() Value {
	return Value{}
}

func Seq(s Sequence) Value {
	return Value{Kind: KindSeq, ref: s}
}

func Object(a Accessible) Value {
	return Value{Kind: KindObject, ref: a}
}

func (v Value) Text() string {
	switch v.Kind {
	case KindString:
		return v.Str
	case KindInt:
		return strconv.FormatInt(v.Int(), 10)
	case KindFloat:
		return strconv.FormatFloat(v.Float(), 'g', -1, 64)
	case KindBool:
		if v.num != 0 {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func (v Value) Truthy() bool {
	switch v.Kind {
	case KindString:
		return v.Str != ""
	case KindInt, KindBool:
		return v.num != 0
	case KindFloat:
		return v.Float() != 0
	case KindSeq:
		seq := v.Sequence()
		return seq != nil && seq.Len() > 0
	case KindObject:
		return v.Object() != nil
	default:
		return false
	}
}

func (v Value) Number() (float64, bool) {
	switch v.Kind {
	case KindInt:
		return float64(v.Int()), true
	case KindFloat:
		return v.Float(), true
	default:
		return 0, false
	}
}

func (v Value) IsInt() bool {
	return v.Kind == KindInt
}

func Equal(a, b Value) bool {
	if left, ok := a.Number(); ok {
		if right, ok := b.Number(); ok {
			return left == right
		}
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case KindString:
		return a.Str == b.Str
	case KindBool:
		return a.num == b.num
	case KindNil:
		return true
	default:
		return false
	}
}

type Map map[string]Value

func (m Map) Get(path []string) (Value, bool) {
	if len(path) == 0 {
		return Nil(), false
	}
	value, ok := m[path[0]]
	if !ok {
		return Nil(), false
	}
	if len(path) == 1 {
		return value, true
	}
	object := value.Object()
	if value.Kind != KindObject || object == nil {
		return Nil(), false
	}
	return object.Get(path[1:])
}

type Empty struct{}

func (Empty) Get([]string) (Value, bool) {
	return Nil(), false
}

type Values []Value

func (v Values) Len() int {
	return len(v)
}

func (v Values) At(index int) Value {
	if index < 0 || index >= len(v) {
		return Nil()
	}
	return v[index]
}
