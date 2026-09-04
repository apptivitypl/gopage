package syntax

import (
	"fmt"
	"slices"
	"strings"

	"github.com/apptivitypl/gopage/internal/diag"
)

type parser struct {
	lexer *Lexer
	file  string
	bag   *diag.Bag
	tok   Token
}

func ClientScriptOf(document *Document) (*ClientScript, bool) {
	for _, node := range document.Nodes {
		if script, ok := node.(*ClientScript); ok {
			return script, true
		}
	}
	return nil, false
}

func ClientScripts(document *Document) []*ClientScript {
	var scripts []*ClientScript
	for _, node := range document.Nodes {
		if script, ok := node.(*ClientScript); ok {
			scripts = append(scripts, script)
		}
	}
	return scripts
}

func Parse(file, source string, bag *diag.Bag) *Document {
	p := &parser{lexer: NewLexer(source), file: file, bag: bag}
	p.advance()
	return p.document()
}

func (p *parser) advance() {
	p.tok = p.lexer.Next()
}

func (p *parser) document() *Document {
	doc := &Document{}
	switch {
	case p.tok.Kind == KindFrontmatter:
		doc.Frontmatter = &Frontmatter{Span: p.tok.Span, Code: p.tok.Text}
		p.advance()
	case p.tok.Kind == KindUnexpected && p.tok.Span.Start == 0:
		p.report(diag.C001, p.tok.Span, "the frontmatter fence is never closed",
			"close the block with a line containing only ---")
		p.advance()
	}

	doc.Nodes, _, _ = p.block(nil)
	return doc
}

func (p *parser) block(stops []string) ([]Node, string, diag.Span) {
	var nodes []Node
	for {
		if p.tok.Kind == KindDirectiveStart {
			name, span, ok := p.directiveName()
			if !ok {
				continue
			}
			if slices.Contains(stops, name) {
				return nodes, name, span
			}
			if node := p.directive(name, span); node != nil {
				nodes = append(nodes, node)
			}
			continue
		}
		node, stop := p.bodyNode()
		if stop {
			return nodes, "", diag.At(p.tok.Span.Start)
		}
		if node != nil {
			nodes = append(nodes, node)
		}
	}
}

func (p *parser) bodyNode() (Node, bool) {
	switch p.tok.Kind {
	case KindEOF:
		return nil, true
	case KindText:
		node := &Text{Span: p.tok.Span, Value: p.tok.Text}
		p.advance()
		return node, false
	case KindInterpStart:
		return p.interpolation(), false
	case KindComponentOpen:
		name := p.tok.Text
		span := p.tok.Span
		p.advance()
		return p.component(name, span), false
	case KindClientScript:
		node := &ClientScript{Span: p.tok.Span, Code: p.tok.Text, Lang: p.tok.Value}
		p.advance()
		return node, false
	case KindElementOpen:
		name := p.tok.Text
		span := p.tok.Span
		p.advance()
		return p.element(name, span), false
	case KindElementClose:
		node := &Text{Span: p.tok.Span, Value: "</" + p.tok.Text + ">"}
		p.advance()
		return node, false
	case KindComponentClose:
		p.report(diag.C306, p.tok.Span,
			fmt.Sprintf("</%s> closes a tag that is not open here", p.tok.Text), "")
		p.advance()
		return nil, false
	case KindDirectiveStart:
		name, span, ok := p.directiveName()
		if !ok {
			return nil, false
		}
		return p.directive(name, span), false
	default:
		return nil, true
	}
}

func (p *parser) interpolation() Node {
	start := p.tok.Span.Start
	p.advance()

	if p.tok.Kind == KindEOF {
		p.report(diag.C002, diag.Span{Start: start, End: p.tok.Span.End},
			"the interpolation is never closed", "close it with }}")
		return nil
	}
	value, ok := p.expr()
	if !ok {
		return nil
	}
	if p.tok.Kind != KindInterpEnd {
		p.report(diag.C002, diag.Span{Start: start, End: p.tok.Span.End},
			"the interpolation is never closed", "close it with }}")
		p.recover()
		return nil
	}
	span := diag.Span{Start: start, End: p.tok.Span.End}
	p.advance()
	return &Interpolation{Span: span, Expr: value}
}

func (p *parser) directiveName() (string, diag.Span, bool) {
	start := p.tok.Span.Start
	p.advance()
	if p.tok.Kind != KindIdent {
		p.report(diag.C004, p.tok.Span,
			fmt.Sprintf("expected a directive name, found %s", p.tok.Kind),
			"available directives: "+strings.Join(knownDirectives, ", "))
		p.recover()
		return "", diag.Span{}, false
	}
	name := p.tok.Text
	span := diag.Span{Start: start, End: p.tok.Span.End}
	p.advance()
	return name, span, true
}

func (p *parser) endDirective(name string, span diag.Span) bool {
	if p.tok.Kind != KindDirectiveEnd {
		p.report(diag.C005, diag.Span{Start: span.Start, End: p.tok.Span.End},
			fmt.Sprintf("the %s directive is never closed", name), "close it with %}")
		p.recover()
		return false
	}
	p.advance()
	return true
}

var knownDirectives = []string{"outlet", "children", "slot", "meta", "assets", "if", "elif", "else", "endif",
	"for", "endfor", "let", "match", "when", "endmatch", "fragment", "placeholder", "endfragment"}

func (p *parser) directive(name string, span diag.Span) Node {
	switch name {
	case "outlet":
		if !p.endDirective(name, span) {
			return nil
		}
		return &Outlet{Span: span}
	case "children":
		if !p.endDirective(name, span) {
			return nil
		}
		return &Children{Span: span}
	case "meta":
		if !p.endDirective(name, span) {
			return nil
		}
		return &MetaBlock{Span: span}
	case "assets":
		if !p.endDirective(name, span) {
			return nil
		}
		return &AssetsBlock{Span: span}
	case "slot":
		return p.slotDirective(span)
	case "if":
		return p.ifDirective(span)
	case "for":
		return p.forDirective(span)
	case "let":
		return p.letDirective(span)
	case "match":
		return p.matchDirective(span)
	case "fragment":
		return p.fragmentDirective(span)
	case "endif", "endfor", "endmatch", "endfragment", "placeholder", "elif", "else", "when":
		p.skipToDirectiveEnd()
		p.report(diag.C006, span,
			fmt.Sprintf("{%% %s %%} has no matching opening directive", name),
			openerFor(name))
		return nil
	default:
		if !p.endDirective(name, span) {
			return nil
		}
		p.report(diag.C004, span, fmt.Sprintf("unknown directive %q", name), suggestDirective(name))
		return nil
	}
}

func (p *parser) ifDirective(span diag.Span) Node {
	node := &If{Span: span}
	stops := []string{"elif", "else", "endif"}

	for {
		cond, ok := p.expr()
		if !ok {
			return nil
		}
		if !p.endDirective("if", span) {
			return nil
		}
		body, stop, stopSpan := p.block(stops)
		node.Branches = append(node.Branches, Branch{Cond: cond, Body: body})

		switch stop {
		case "elif":
			span = stopSpan
			continue
		case "else":
			if !p.endDirective("else", stopSpan) {
				return nil
			}
			elseBody, endStop, endSpan := p.block([]string{"endif"})
			node.Branches = append(node.Branches, Branch{Body: elseBody})
			return p.closeBlock(node, "if", endStop, "endif", endSpan)
		default:
			return p.closeBlock(node, "if", stop, "endif", stopSpan)
		}
	}
}

func (p *parser) forDirective(span diag.Span) Node {
	if p.tok.Kind != KindIdent {
		p.report(diag.C201, p.tok.Span,
			fmt.Sprintf("expected a loop variable, found %s", p.tok.Kind),
			"the form is {% for item in items %}")
		p.recover()
		return nil
	}
	node := &For{Span: span, Var: p.tok.Text}
	p.advance()

	if p.tok.Kind != KindIdent || p.tok.Text != "in" {
		p.report(diag.C201, p.tok.Span,
			fmt.Sprintf("expected in, found %s", p.tok.Kind),
			"the form is {% for item in items %}")
		p.recover()
		return nil
	}
	p.advance()

	seq, ok := p.expr()
	if !ok {
		return nil
	}
	node.Seq = seq
	if !p.endDirective("for", span) {
		return nil
	}

	body, stop, stopSpan := p.block([]string{"else", "endfor"})
	node.Body = body
	if stop == "else" {
		if !p.endDirective("else", stopSpan) {
			return nil
		}
		empty, endStop, endSpan := p.block([]string{"endfor"})
		node.Empty = empty
		return p.closeBlock(node, "for", endStop, "endfor", endSpan)
	}
	return p.closeBlock(node, "for", stop, "endfor", stopSpan)
}

func (p *parser) matchDirective(span diag.Span) Node {
	subject, ok := p.expr()
	if !ok {
		return nil
	}
	if !p.endDirective("match", span) {
		return nil
	}
	node := &Match{Span: span, Subject: subject}

	leading, stop, stopSpan := p.block([]string{"when", "endmatch"})
	if hasContent(leading) {
		p.report(diag.C006, stopSpan, "text between {% match %} and the first {% when %}",
			"every branch of a match lives inside a {% when %}")
	}
	for stop == "when" {
		arm, ok := p.arm(stopSpan)
		if !ok {
			return nil
		}
		var body []Node
		body, stop, stopSpan = p.block([]string{"when", "endmatch"})
		arm.Body = body
		node.Arms = append(node.Arms, arm)
	}
	return p.closeBlock(node, "match", stop, "endmatch", stopSpan)
}

func (p *parser) arm(span diag.Span) (Arm, bool) {
	if p.tok.Kind != KindIdent {
		p.report(diag.C201, p.tok.Span,
			fmt.Sprintf("expected a case name, found %s", p.tok.Kind),
			"the form is {% when Active %}")
		p.recover()
		return Arm{}, false
	}
	arm := Arm{Span: span, Name: p.tok.Text}
	p.advance()
	if !p.endDirective("when", span) {
		return Arm{}, false
	}
	return arm, true
}

func hasContent(nodes []Node) bool {
	for _, node := range nodes {
		text, ok := node.(*Text)
		if !ok || strings.TrimSpace(text.Value) != "" {
			return true
		}
	}
	return false
}

func (p *parser) slotDirective(span diag.Span) Node {
	if p.tok.Kind != KindString {
		p.report(diag.C306, p.tok.Span,
			fmt.Sprintf("expected a quoted slot name, found %s", p.tok.Kind),
			`the form is {% slot "footer" %}`)
		p.recover()
		return nil
	}
	node := &SlotOutlet{Span: span, Name: p.tok.Value}
	p.advance()
	if !p.endDirective("slot", span) {
		return nil
	}
	return node
}

const deferSetting = "defer"

func (p *parser) fragmentDirective(span diag.Span) Node {
	if p.tok.Kind != KindString {
		p.report(diag.C201, p.tok.Span, "a fragment needs a name in quotes",
			`the form is {% fragment "reviews" cache="5m" %}`)
		p.recover()
		return nil
	}
	node := &Fragment{Span: span, Name: p.tok.Value, NameSpan: p.tok.Span}
	p.advance()

	for p.tok.Kind == KindIdent {
		name := p.tok.Text
		nameSpan := p.tok.Span
		p.advance()
		if name == deferSetting && p.tok.Kind != KindAssign {
			node.Defer = true
			continue
		}
		if p.tok.Kind != KindAssign {
			p.report(diag.C201, nameSpan, fmt.Sprintf("%s needs a value", name),
				`write cache="5m", stale="1h" or defer`)
			p.recover()
			return nil
		}
		p.advance()
		if p.tok.Kind != KindString {
			p.report(diag.C201, p.tok.Span, fmt.Sprintf("the value of %s must be quoted", name),
				`write cache="5m", stale="1h" or defer="visible"`)
			p.recover()
			return nil
		}
		switch name {
		case deferSetting:
			node.Defer = true
			node.Strategy = p.tok.Value
		case "cache":
			node.Cache = p.tok.Value
		case "stale":
			node.Stale = p.tok.Value
		default:
			p.report(diag.C201, nameSpan, fmt.Sprintf("a fragment has no %s setting", name),
				"a fragment takes cache, stale and defer")
			p.recover()
			return nil
		}
		p.advance()
	}
	if !p.endDirective("fragment", span) {
		return nil
	}
	body, stop, stopSpan := p.block([]string{"endfragment", "placeholder"})
	node.Body = body
	if stop == "placeholder" {
		if !p.endDirective("placeholder", stopSpan) {
			return nil
		}
		node.HoldSpan = stopSpan
		node.Placeholder, stop, stopSpan = p.block([]string{"endfragment"})
	}
	return p.closeBlock(node, "fragment", stop, "endfragment", stopSpan)
}

func (p *parser) letDirective(span diag.Span) Node {
	if p.tok.Kind != KindIdent {
		p.report(diag.C201, p.tok.Span,
			fmt.Sprintf("expected a name, found %s", p.tok.Kind),
			"the form is {% let name = expression %}")
		p.recover()
		return nil
	}
	node := &Let{Span: span, Name: p.tok.Text}
	p.advance()

	if p.tok.Kind != KindAssign {
		p.report(diag.C201, p.tok.Span,
			fmt.Sprintf("expected = after the name, found %s", p.tok.Kind),
			"the form is {% let name = expression %}")
		p.recover()
		return nil
	}
	p.advance()

	value, ok := p.expr()
	if !ok {
		return nil
	}
	node.Value = value
	if !p.endDirective("let", span) {
		return nil
	}
	return node
}

func (p *parser) closeBlock(node Node, opener, stop, want string, span diag.Span) Node {
	if stop != want {
		p.report(diag.C006, span,
			fmt.Sprintf("{%% %s %%} is never closed", opener),
			fmt.Sprintf("close it with {%% %s %%}", want))
		return nil
	}
	return p.finish(node, want, span)
}

func (p *parser) finish(node Node, name string, span diag.Span) Node {
	if !p.endDirective(name, span) {
		return nil
	}
	return node
}

func (p *parser) skipToDirectiveEnd() {
	for {
		switch p.tok.Kind {
		case KindEOF, KindText, KindDirectiveStart, KindInterpStart:
			return
		case KindDirectiveEnd:
			p.advance()
			return
		default:
			p.advance()
		}
	}
}

func (p *parser) recover() {
	for {
		switch p.tok.Kind {
		case KindEOF:
			return
		case KindInterpEnd, KindDirectiveEnd:
			p.advance()
			return
		case KindText, KindDirectiveStart, KindInterpStart:
			return
		default:
			p.advance()
		}
	}
}

func (p *parser) report(code diag.Code, span diag.Span, message, help string) {
	d := diag.New(code, p.file, span, message)
	if help != "" {
		d = d.WithHelp(help)
	}
	p.bag.Add(d)
}

var openers = map[string]string{
	"endif":       "{% if %}",
	"elif":        "{% if %}",
	"else":        "{% if %} or {% for %}",
	"endfor":      "{% for %}",
	"when":        "{% match %}",
	"endmatch":    "{% match %}",
	"endfragment": "{% fragment %}",
	"placeholder": "{% fragment %}",
}

func openerFor(name string) string {
	return "this closes " + openers[name] + ", which is not open here"
}

func suggestDirective(name string) string {
	lowered := strings.ToLower(name)
	for _, known := range knownDirectives {
		if strings.HasPrefix(known, lowered) || strings.HasPrefix(lowered, known) {
			return fmt.Sprintf("did you mean {%% %s %%}?", known)
		}
	}
	return "available directives: " + strings.Join(knownDirectives, ", ")
}

func Directives() []string {
	return append([]string(nil), knownDirectives...)
}
