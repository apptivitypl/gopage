package syntax

import (
	"strings"

	"github.com/sonquer/rill/internal/diag"
)

const fence = "---"

type mode uint8

const (
	modeText mode = iota
	modeInterp
	modeDirective
	modeTag
)

type Lexer struct {
	src     string
	pos     uint32
	mode    mode
	scanned bool
}

func NewLexer(src string) *Lexer {
	return &Lexer{src: src}
}

func (l *Lexer) Next() Token {
	if !l.scanned {
		l.scanned = true
		if token, ok := l.frontmatter(); ok {
			return token
		}
	}
	switch l.mode {
	case modeInterp:
		return l.inBraces(KindInterpEnd, "}}")
	case modeDirective:
		return l.inBraces(KindDirectiveEnd, "%}")
	case modeTag:
		return l.inTag()
	default:
		return l.text()
	}
}

func (l *Lexer) frontmatter() (Token, bool) {
	if !strings.HasPrefix(l.src, fence) {
		return Token{}, false
	}
	rest := l.src[len(fence):]
	if !strings.HasPrefix(rest, "\n") && !strings.HasPrefix(rest, "\r\n") {
		return Token{}, false
	}
	bodyStart := len(fence) + newlineWidth(rest)
	end := findClosingFence(l.src, bodyStart)
	if end < 0 {
		l.pos = uint32(len(l.src))
		return Token{
			Kind: KindUnexpected,
			Span: diag.Span{Start: 0, End: uint32(len(fence))},
			Text: l.src[bodyStart:],
		}, true
	}
	l.pos = uint32(end)
	l.skipClosingFence()
	return Token{
		Kind: KindFrontmatter,
		Span: diag.Span{Start: uint32(bodyStart), End: uint32(end)},
		Text: l.src[bodyStart:end],
	}, true
}

func findClosingFence(src string, from int) int {
	for offset := from; offset < len(src); {
		lineEnd := strings.IndexByte(src[offset:], '\n')
		var line string
		if lineEnd < 0 {
			line = src[offset:]
		} else {
			line = src[offset : offset+lineEnd]
		}
		if strings.TrimRight(line, "\r") == fence {
			return offset
		}
		if lineEnd < 0 {
			return -1
		}
		offset += lineEnd + 1
	}
	return -1
}

func newlineWidth(s string) int {
	if strings.HasPrefix(s, "\r\n") {
		return 2
	}
	return 1
}

func (l *Lexer) skipClosingFence() {
	l.pos += uint32(len(fence))
	for l.pos < uint32(len(l.src)) && (l.src[l.pos] == '\r' || l.src[l.pos] == '\n') {
		l.pos++
	}
}

func (l *Lexer) text() Token {
	start := l.pos
	for l.pos < uint32(len(l.src)) {
		if l.startsWith("{{") {
			if l.pos > start {
				return l.emit(KindText, start, l.pos)
			}
			l.mode = modeInterp
			l.pos += 2
			return l.emit(KindInterpStart, start, l.pos)
		}
		if l.startsWith("{%") {
			if l.pos > start {
				return l.emit(KindText, start, l.pos)
			}
			l.mode = modeDirective
			l.pos += 2
			return l.emit(KindDirectiveStart, start, l.pos)
		}
		if script, ok := l.clientScript(); ok {
			if l.pos > start {
				return l.emit(KindText, start, l.pos)
			}
			tagStart := l.pos
			l.pos = script.end
			return Token{
				Kind:  KindClientScript,
				Span:  diag.Span{Start: tagStart, End: script.end},
				Text:  l.src[script.open:script.close],
				Value: script.lang,
			}
		}
		if name, width, closing, ok := l.tag(); ok {
			if l.pos > start {
				return l.emit(KindText, start, l.pos)
			}
			tagStart := l.pos
			l.pos += width
			kind := kindOfTag(name, closing)
			if !closing {
				l.mode = modeTag
			}
			return Token{Kind: kind, Span: diag.Span{Start: tagStart, End: l.pos}, Text: name}
		}
		l.pos++
	}
	if l.pos > start {
		return l.emit(KindText, start, l.pos)
	}
	return Token{Kind: KindEOF, Span: diag.At(l.pos)}
}

const (
	clientOpen  = "<script"
	clientClose = "</script>"
)

type clientBlock struct {
	open  uint32
	close uint32
	end   uint32
	lang  string
}

func (l *Lexer) clientScript() (clientBlock, bool) {
	rest := l.src[l.pos:]
	if !strings.HasPrefix(rest, clientOpen) {
		return clientBlock{}, false
	}
	head := tagEnd(rest, len(clientOpen))
	if head < 0 {
		return clientBlock{}, false
	}
	lang, ok := clientAttributes(rest[len(clientOpen):head])
	if !ok {
		return clientBlock{}, false
	}
	body := strings.Index(rest[head:], clientClose)
	if body < 0 {
		return clientBlock{}, false
	}
	open := l.pos + uint32(head) + 1
	closes := l.pos + uint32(head) + uint32(body)
	return clientBlock{open: open, close: closes, end: closes + uint32(len(clientClose)), lang: lang}, true
}

func tagEnd(text string, from int) int {
	var quote byte
	for index := from; index < len(text); index++ {
		switch letter := text[index]; {
		case quote != 0:
			if letter == quote {
				quote = 0
			}
		case letter == '"', letter == '\'':
			quote = letter
		case letter == '>':
			return index
		}
	}
	return -1
}

func clientAttributes(attributes string) (string, bool) {
	lang, client := "", false
	for field := range strings.FieldsSeq(attributes) {
		name, value, valued := strings.Cut(field, "=")
		switch name {
		case "client":
			client = !valued
		case "lang":
			lang = strings.Trim(value, `"'`)
		}
	}
	return lang, client
}

func kindOfTag(name string, closing bool) Kind {
	if isComponentStart(name[0]) {
		if closing {
			return KindComponentClose
		}
		return KindComponentOpen
	}
	if closing {
		return KindElementClose
	}
	return KindElementOpen
}

func (l *Lexer) tag() (string, uint32, bool, bool) {
	rest := l.src[l.pos:]
	if !strings.HasPrefix(rest, "<") {
		return "", 0, false, false
	}
	offset := 1
	closing := strings.HasPrefix(rest[offset:], "/")
	if closing {
		offset++
	}
	if offset >= len(rest) || !isTagStart(rest[offset]) {
		return "", 0, false, false
	}
	end := offset
	for end < len(rest) && (isIdentPart(rest[end]) || rest[end] == '-') {
		end++
	}
	name := rest[offset:end]
	if closing {
		if end >= len(rest) || rest[end] != '>' {
			return "", 0, false, false
		}
		return name, uint32(end + 1), true, true
	}
	return name, uint32(end), false, true
}

func isComponentStart(c byte) bool {
	return c >= 'A' && c <= 'Z'
}

func isTagStart(c byte) bool {
	return isComponentStart(c) || (c >= 'a' && c <= 'z')
}

func (l *Lexer) inTag() Token {
	l.skipSpace()
	if l.pos >= uint32(len(l.src)) {
		return Token{Kind: KindEOF, Span: diag.At(l.pos)}
	}
	start := l.pos
	switch {
	case l.startsWith("/>"):
		l.pos += 2
		l.mode = modeText
		return l.emit(KindTagSelfClose, start, l.pos)
	case l.src[l.pos] == '>':
		l.pos++
		l.mode = modeText
		return l.emit(KindTagEnd, start, l.pos)
	case isIdentStart(l.src[l.pos]):
		for l.pos < uint32(len(l.src)) && (isIdentPart(l.src[l.pos]) || l.src[l.pos] == '-') {
			l.pos++
		}
		return l.emit(KindIdent, start, l.pos)
	case l.src[l.pos] == '\'' || l.src[l.pos] == '"':
		return l.stringLiteral(start, l.src[l.pos])
	case l.src[l.pos] == ':':
		l.pos++
		return l.emit(KindColon, start, l.pos)
	case l.src[l.pos] == '#':
		l.pos++
		return l.emit(KindHash, start, l.pos)
	case l.src[l.pos] == '?':
		l.pos++
		return l.emit(KindQuestion, start, l.pos)
	case l.src[l.pos] == '=':
		l.pos++
		return l.emit(KindAssign, start, l.pos)
	default:
		l.pos++
		return l.emit(KindUnexpected, start, l.pos)
	}
}

func (l *Lexer) inBraces(closing Kind, closingText string) Token {
	l.skipSpace()
	if l.pos >= uint32(len(l.src)) {
		return Token{Kind: KindEOF, Span: diag.At(l.pos)}
	}
	start := l.pos
	if l.startsWith(closingText) {
		l.pos += uint32(len(closingText))
		l.mode = modeText
		return l.emit(closing, start, l.pos)
	}
	switch c := l.src[l.pos]; {
	case isIdentStart(c):
		for l.pos < uint32(len(l.src)) && isIdentPart(l.src[l.pos]) {
			l.pos++
		}
		return l.emit(KindIdent, start, l.pos)
	case isDigit(c):
		return l.number(start)
	case c == '\'' || c == '"':
		return l.stringLiteral(start, c)
	}
	for _, operator := range operators {
		if l.startsWith(operator.text) {
			l.pos += uint32(len(operator.text))
			return l.emit(operator.kind, start, l.pos)
		}
	}
	l.pos++
	return l.emit(KindUnexpected, start, l.pos)
}

func (l *Lexer) number(start uint32) Token {
	for l.pos < uint32(len(l.src)) && isDigit(l.src[l.pos]) {
		l.pos++
	}
	if l.pos+1 < uint32(len(l.src)) && l.src[l.pos] == '.' && isDigit(l.src[l.pos+1]) {
		l.pos++
		for l.pos < uint32(len(l.src)) && isDigit(l.src[l.pos]) {
			l.pos++
		}
	}
	token := l.emit(KindNumber, start, l.pos)
	token.Value = token.Text
	return token
}

func (l *Lexer) stringLiteral(start uint32, quote byte) Token {
	l.pos++
	valueStart := l.pos
	for l.pos < uint32(len(l.src)) && l.src[l.pos] != quote {
		l.pos++
	}
	if l.pos >= uint32(len(l.src)) {
		return l.emit(KindUnexpected, start, l.pos)
	}
	value := l.src[valueStart:l.pos]
	l.pos++
	token := l.emit(KindString, start, l.pos)
	token.Value = value
	return token
}

func (l *Lexer) skipSpace() {
	for l.pos < uint32(len(l.src)) && isSpace(l.src[l.pos]) {
		l.pos++
	}
}

func (l *Lexer) startsWith(prefix string) bool {
	return strings.HasPrefix(l.src[l.pos:], prefix)
}

func (l *Lexer) emit(kind Kind, start, end uint32) Token {
	return Token{Kind: kind, Span: diag.Span{Start: start, End: end}, Text: l.src[start:end]}
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || isDigit(c)
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}
