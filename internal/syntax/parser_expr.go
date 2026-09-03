package syntax

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/apptivitypl/rill/internal/diag"
)

func (p *parser) expr() (Expr, bool) {
	value, ok := p.binary(1)
	if !ok {
		return nil, false
	}
	for p.tok.Kind == KindPipe {
		if value, ok = p.filterCall(value); !ok {
			return nil, false
		}
	}
	return value, true
}

func (p *parser) filterCall(input Expr) (Expr, bool) {
	p.advance()
	if p.tok.Kind != KindIdent {
		p.report(diag.C201, p.tok.Span, "a filter needs a name after |", `write {{ value | upper }}`)
		return nil, false
	}
	call := &FilterCall{
		Name:     p.tok.Text,
		NameSpan: p.tok.Span,
		Input:    input,
		Span:     diag.Span{Start: input.ExprSpan().Start, End: p.tok.Span.End},
	}
	p.advance()
	if p.tok.Kind != KindLParen {
		return call, true
	}
	p.advance()
	argument, ok := p.expr()
	if !ok {
		return nil, false
	}
	call.Argument = argument
	if p.tok.Kind != KindRParen {
		p.report(diag.C202, call.NameSpan, fmt.Sprintf("the argument of %s is never closed", call.Name),
			"close it with )")
		return nil, false
	}
	call.Span.End = p.tok.Span.End
	p.advance()
	return call, true
}

func (p *parser) binary(minPrecedence int) (Expr, bool) {
	left, ok := p.unary()
	if !ok {
		return nil, false
	}
	for {
		op, isBinary := binaryOps[p.tok.Kind]
		if !isBinary || Precedence(op) < minPrecedence {
			return left, true
		}
		p.advance()
		right, ok := p.binary(Precedence(op) + 1)
		if !ok {
			return nil, false
		}
		left = &Binary{
			Span:  diag.Span{Start: left.ExprSpan().Start, End: right.ExprSpan().End},
			Op:    op,
			Left:  left,
			Right: right,
		}
	}
}

func (p *parser) unary() (Expr, bool) {
	switch p.tok.Kind {
	case KindNot, KindMinus:
		op := OpNot
		if p.tok.Kind == KindMinus {
			op = OpNeg
		}
		start := p.tok.Span.Start
		p.advance()
		operand, ok := p.unary()
		if !ok {
			return nil, false
		}
		return &Unary{
			Span:    diag.Span{Start: start, End: operand.ExprSpan().End},
			Op:      op,
			Operand: operand,
		}, true
	default:
		return p.postfix()
	}
}

func (p *parser) postfix() (Expr, bool) {
	base, ok := p.primary()
	if !ok {
		return nil, false
	}
	for p.tok.Kind == KindLBracket {
		p.advance()
		index, ok := p.expr()
		if !ok {
			return nil, false
		}
		if p.tok.Kind != KindRBracket {
			p.report(diag.C202, p.tok.Span,
				fmt.Sprintf("expected ] to close the index, found %s", p.tok.Kind), "")
			p.recover()
			return nil, false
		}
		base = &Index{
			Span:  diag.Span{Start: base.ExprSpan().Start, End: p.tok.Span.End},
			Base:  base,
			Index: index,
		}
		p.advance()
	}
	return base, true
}

func (p *parser) primary() (Expr, bool) {
	switch p.tok.Kind {
	case KindIdent:
		return p.pathOrKeyword()
	case KindString:
		lit := &StringLit{Span: p.tok.Span, Value: p.tok.Value}
		p.advance()
		return lit, true
	case KindNumber:
		return p.number()
	case KindLParen:
		return p.grouped()
	default:
		p.report(diag.C201, p.tok.Span,
			fmt.Sprintf("expected a value, found %s", p.tok.Kind),
			"expressions read props fields, literals, and the operators listed in the reference")
		p.recover()
		return nil, false
	}
}

const MessageFunc = "t"

func (p *parser) pathOrKeyword() (Expr, bool) {
	if p.tok.Text == "true" || p.tok.Text == "false" {
		lit := &BoolLit{Span: p.tok.Span, Value: p.tok.Text == "true"}
		p.advance()
		return lit, true
	}
	start := p.tok.Span.Start
	var segments []string
	for {
		if p.tok.Kind != KindIdent {
			p.report(diag.C201, p.tok.Span,
				fmt.Sprintf("expected a field name, found %s", p.tok.Kind), "")
			p.recover()
			return nil, false
		}
		segments = append(segments, p.tok.Text)
		end := p.tok.Span.End
		p.advance()
		if p.tok.Kind == KindLParen && len(segments) == 1 {
			if segments[0] == MessageFunc {
				return p.messageCall(start)
			}
			return p.builtinCall(start, segments[0], diag.Span{Start: start, End: end})
		}
		if p.tok.Kind != KindDot {
			return &Path{Span: diag.Span{Start: start, End: end}, Segments: segments}, true
		}
		p.advance()
	}
}

func (p *parser) number() (Expr, bool) {
	text := p.tok.Value
	span := p.tok.Span
	p.advance()
	if strings.Contains(text, ".") {
		value, err := strconv.ParseFloat(text, 64)
		if err != nil {
			p.report(diag.C201, span, fmt.Sprintf("cannot read %q as a number", text), "")
			return nil, false
		}
		return &FloatLit{Span: span, Value: value}, true
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		p.report(diag.C201, span, fmt.Sprintf("cannot read %q as a whole number", text),
			"whole numbers must fit in 64 bits")
		return nil, false
	}
	return &IntLit{Span: span, Value: value}, true
}

func (p *parser) grouped() (Expr, bool) {
	p.advance()
	inner, ok := p.expr()
	if !ok {
		return nil, false
	}
	if p.tok.Kind != KindRParen {
		p.report(diag.C202, p.tok.Span,
			fmt.Sprintf("expected ) to close the group, found %s", p.tok.Kind), "")
		p.recover()
		return nil, false
	}
	p.advance()
	return inner, true
}

func (p *parser) messageCall(start uint32) (Expr, bool) {
	p.advance()
	if p.tok.Kind != KindString {
		p.report(diag.C201, p.tok.Span, "a message needs a key in quotes",
			`the form is t("listing.title") or t("reviews.count", count = len(Reviews))`)
		p.recover()
		return nil, false
	}
	call := &MessageCall{Key: p.tok.Value, KeySpan: p.tok.Span}
	p.advance()

	if p.tok.Kind == KindComma {
		p.advance()
		if p.tok.Kind != KindIdent || p.tok.Text != "count" {
			p.report(diag.C201, p.tok.Span, "a message takes one argument named count",
				`write t("reviews.count", count = len(Reviews))`)
			p.recover()
			return nil, false
		}
		p.advance()
		if p.tok.Kind != KindAssign {
			p.report(diag.C201, p.tok.Span, "count needs a value",
				`write t("reviews.count", count = len(Reviews))`)
			p.recover()
			return nil, false
		}
		p.advance()
		count, ok := p.expr()
		if !ok {
			return nil, false
		}
		call.Count = count
	}
	if p.tok.Kind != KindRParen {
		p.report(diag.C202, call.KeySpan, "the message call is never closed", "close it with )")
		p.recover()
		return nil, false
	}
	call.Span = diag.Span{Start: start, End: p.tok.Span.End}
	p.advance()
	return call, true
}

func (p *parser) builtinCall(start uint32, name string, nameSpan diag.Span) (Expr, bool) {
	p.advance()
	argument, ok := p.expr()
	if !ok {
		return nil, false
	}
	if p.tok.Kind != KindRParen {
		p.report(diag.C202, nameSpan, fmt.Sprintf("the call to %s is never closed", name), "close it with )")
		p.recover()
		return nil, false
	}
	call := &FilterCall{
		Span:     diag.Span{Start: start, End: p.tok.Span.End},
		Name:     name,
		NameSpan: nameSpan,
		Input:    argument,
	}
	p.advance()
	return call, true
}
