package runtime

import (
	"strconv"
	"unicode/utf8"
)

const maxJSONDepth = 16

func AppendJSON(dst []byte, value Value) []byte {
	return appendJSON(dst, value, 0)
}

func appendJSON(dst []byte, value Value, depth int) []byte {
	if depth > maxJSONDepth {
		return append(dst, "null"...)
	}
	switch value.Kind {
	case KindNil:
		return append(dst, "null"...)
	case KindString:
		return appendJSONString(dst, value.Str)
	case KindInt:
		return strconv.AppendInt(dst, value.Int(), 10)
	case KindFloat:
		return appendJSONFloat(dst, value.Float())
	case KindBool:
		return strconv.AppendBool(dst, value.Truthy())
	case KindSeq:
		return appendJSONSeq(dst, value.Sequence(), depth)
	case KindObject:
		return appendJSONObject(dst, value.Object(), depth)
	default:
		return append(dst, "null"...)
	}
}

func appendJSONFloat(dst []byte, number float64) []byte {
	if number != number || number > 1.7e308 || number < -1.7e308 {
		return append(dst, "null"...)
	}
	return strconv.AppendFloat(dst, number, 'g', -1, 64)
}

func appendJSONSeq(dst []byte, seq Sequence, depth int) []byte {
	if seq == nil {
		return append(dst, "null"...)
	}
	dst = append(dst, '[')
	for i := range seq.Len() {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendJSON(dst, seq.At(i), depth+1)
	}
	return append(dst, ']')
}

func appendJSONObject(dst []byte, object Accessible, depth int) []byte {
	fields, ok := object.(Fields)
	if !ok {
		return append(dst, "null"...)
	}
	dst = append(dst, '{')
	for i, name := range fields.Names() {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = appendJSONString(dst, name)
		dst = append(dst, ':')
		value, _ := object.Get([]string{name})
		dst = appendJSON(dst, value, depth+1)
	}
	return append(dst, '}')
}

type Fields interface {
	Names() []string
}

func appendJSONString(dst []byte, text string) []byte {
	dst = append(dst, '"')
	for i := 0; i < len(text); {
		c := text[i]
		if c < utf8.RuneSelf {
			dst = appendJSONByte(dst, c)
			i++
			continue
		}
		_, size := utf8.DecodeRuneInString(text[i:])
		dst = append(dst, text[i:i+size]...)
		i += size
	}
	return append(dst, '"')
}

func appendJSONByte(dst []byte, c byte) []byte {
	switch c {
	case '"':
		return append(dst, '\\', '"')
	case '\\':
		return append(dst, '\\', '\\')
	case '\n':
		return append(dst, '\\', 'n')
	case '\r':
		return append(dst, '\\', 'r')
	case '\t':
		return append(dst, '\\', 't')
	case '<', '>', '&':
		return append(dst, '\\', 'u', '0', '0', hex[c>>4], hex[c&0xf])
	}
	if c < 0x20 {
		return append(dst, '\\', 'u', '0', '0', hex[c>>4], hex[c&0xf])
	}
	return append(dst, c)
}

const hex = "0123456789abcdef"
