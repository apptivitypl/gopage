package syntax

import "testing"

func lex(src string) []Token {
	l := NewLexer(src)
	var tokens []Token
	for {
		tok := l.Next()
		tokens = append(tokens, tok)
		if tok.Kind == KindEOF {
			return tokens
		}
	}
}

func kinds(tokens []Token) []Kind {
	out := make([]Kind, len(tokens))
	for i, tok := range tokens {
		out[i] = tok.Kind
	}
	return out
}

func assertKinds(t *testing.T, src string, want ...Kind) []Token {
	t.Helper()
	tokens := lex(src)
	got := kinds(tokens)
	if len(got) != len(want) {
		t.Fatalf("lex(%q) = %v, want %v", src, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("lex(%q) = %v, want %v", src, got, want)
		}
	}
	return tokens
}

func TestPlainTextIsOneToken(t *testing.T) {
	tokens := assertKinds(t, "hello there", KindText, KindEOF)
	if tokens[0].Text != "hello there" {
		t.Errorf("text = %q", tokens[0].Text)
	}
}

func TestElementTagsAreLexed(t *testing.T) {
	tokens := assertKinds(t, "<h1>hello</h1>",
		KindElementOpen, KindTagEnd, KindText, KindElementClose, KindEOF)
	if tokens[0].Text != "h1" || tokens[3].Text != "h1" {
		t.Errorf("tag names = %q, %q", tokens[0].Text, tokens[3].Text)
	}
}

func TestElementAttributesAreLexed(t *testing.T) {
	assertKinds(t, `<div class="x" :class="y" hidden?="z" data-id="1">`,
		KindElementOpen,
		KindIdent, KindAssign, KindString,
		KindColon, KindIdent, KindAssign, KindString,
		KindIdent, KindQuestion, KindAssign, KindString,
		KindIdent, KindAssign, KindString,
		KindTagEnd, KindEOF)
}

func TestDoctypeAndCommentsStayText(t *testing.T) {
	assertKinds(t, "<!doctype html>\n<!-- note -->", KindText, KindEOF)
}

func TestEmptySourceIsJustEOF(t *testing.T) {
	assertKinds(t, "", KindEOF)
}

func TestInterpolationSplitsTheStream(t *testing.T) {
	tokens := assertKinds(t, "a{{ x }}b",
		KindText, KindInterpStart, KindIdent, KindInterpEnd, KindText, KindEOF)
	if tokens[2].Text != "x" {
		t.Errorf("identifier = %q", tokens[2].Text)
	}
}

func TestDottedPathInsideInterpolation(t *testing.T) {
	assertKinds(t, "{{ a.b }}",
		KindInterpStart, KindIdent, KindDot, KindIdent, KindInterpEnd, KindEOF)
}

func TestDirectiveTokens(t *testing.T) {
	assertKinds(t, "{% outlet %}",
		KindDirectiveStart, KindIdent, KindDirectiveEnd, KindEOF)
}

func TestUnexpectedByteInsideBraces(t *testing.T) {
	tokens := assertKinds(t, "{{ @ }}",
		KindInterpStart, KindUnexpected, KindInterpEnd, KindEOF)
	if tokens[1].Text != "@" {
		t.Errorf("unexpected token text = %q", tokens[1].Text)
	}
}

func TestUnterminatedInterpolationReachesEOF(t *testing.T) {
	assertKinds(t, "{{ x", KindInterpStart, KindIdent, KindEOF)
}

func TestFrontmatterIsLexedAsOneToken(t *testing.T) {
	tokens := assertKinds(t, "---\ntype Props struct{}\n---\nbody",
		KindFrontmatter, KindText, KindEOF)
	if tokens[0].Text != "type Props struct{}\n" {
		t.Errorf("frontmatter = %q", tokens[0].Text)
	}
	if tokens[1].Text != "body" {
		t.Errorf("body = %q", tokens[1].Text)
	}
}

func TestFrontmatterAcceptsCarriageReturns(t *testing.T) {
	tokens := assertKinds(t, "---\r\nvar x int\r\n---\r\nbody", KindFrontmatter, KindText, KindEOF)
	if tokens[1].Text != "body" {
		t.Errorf("body = %q", tokens[1].Text)
	}
}

func TestUnterminatedFrontmatterIsFlagged(t *testing.T) {
	assertKinds(t, "---\nvar x int\n", KindUnexpected, KindEOF)
}

func TestFenceLaterInTheBodyIsPlainText(t *testing.T) {
	tokens := assertKinds(t, "text\n---\n", KindText, KindEOF)
	if tokens[0].Text != "text\n---\n" {
		t.Errorf("text = %q", tokens[0].Text)
	}
}

func TestFenceWithoutNewlineIsPlainText(t *testing.T) {
	assertKinds(t, "--- not a fence", KindText, KindEOF)
}

func TestEmptyFrontmatter(t *testing.T) {
	tokens := assertKinds(t, "---\n---\nbody", KindFrontmatter, KindText, KindEOF)
	if tokens[0].Text != "" {
		t.Errorf("frontmatter = %q, want empty", tokens[0].Text)
	}
}

func TestSpansCoverTheSource(t *testing.T) {
	src := "ab{{ c }}"
	tokens := lex(src)
	if tokens[0].Span.Start != 0 || tokens[0].Span.End != 2 {
		t.Errorf("text span = %+v", tokens[0].Span)
	}
	last := tokens[len(tokens)-2]
	if last.Span.End != uint32(len(src)) {
		t.Errorf("last span ends at %d, want %d", last.Span.End, len(src))
	}
}

func TestKindNames(t *testing.T) {
	if KindInterpStart.String() != "{{" {
		t.Errorf("KindInterpStart = %q", KindInterpStart)
	}
	if Kind(200).String() != "unknown token" {
		t.Errorf("unknown kind = %q", Kind(200))
	}
}
