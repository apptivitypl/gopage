package jsonc

import "fmt"

type SyntaxError struct {
	Offset int
	Msg    string
}

func (e *SyntaxError) Error() string {
	return e.Msg
}

func ToJSON(src []byte) ([]byte, error) {
	out := make([]byte, len(src))
	copy(out, src)
	comma := -1
	for i := 0; i < len(out); {
		switch {
		case out[i] == '"':
			end, err := endOfString(out, i)
			if err != nil {
				return nil, err
			}
			comma, i = -1, end
		case out[i] == '/' && i+1 < len(out) && out[i+1] == '/':
			end := i + 2
			for end < len(out) && out[end] != '\n' {
				end++
			}
			blank(out, i, end)
			i = end
		case out[i] == '/' && i+1 < len(out) && out[i+1] == '*':
			end := i + 2
			for end+1 < len(out) && (out[end] != '*' || out[end+1] != '/') {
				end++
			}
			if end+1 >= len(out) {
				return nil, &SyntaxError{Offset: i, Msg: "unterminated block comment"}
			}
			blank(out, i, end+2)
			i = end + 2
		case out[i] == ',':
			comma, i = i, i+1
		case out[i] == '}' || out[i] == ']':
			if comma >= 0 {
				out[comma] = ' '
			}
			comma, i = -1, i+1
		case isSpace(out[i]):
			i++
		default:
			comma, i = -1, i+1
		}
	}
	return out, nil
}

func blank(buf []byte, from, to int) {
	for i := from; i < to; i++ {
		if buf[i] != '\n' {
			buf[i] = ' '
		}
	}
}

func endOfString(buf []byte, start int) (int, error) {
	for i := start + 1; i < len(buf); i++ {
		switch buf[i] {
		case '\\':
			i++
		case '"':
			return i + 1, nil
		}
	}
	return 0, &SyntaxError{Offset: start, Msg: "unterminated string"}
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func Position(src []byte, offset int) (line, column int) {
	line, column = 1, 1
	for i := 0; i < offset && i < len(src); i++ {
		if src[i] == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return line, column
}

func Locate(src []byte, offset int, err error) error {
	line, column := Position(src, offset)
	return fmt.Errorf("line %d, column %d: %w", line, column, err)
}
