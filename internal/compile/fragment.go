package compile

import (
	"fmt"
	"strings"
	"time"

	"github.com/apptivitypl/rill/internal/config"
	"github.com/apptivitypl/rill/internal/diag"
	"github.com/apptivitypl/rill/internal/ir"
	"github.com/apptivitypl/rill/internal/syntax"
)

const MaxFragments = 64

func (b *builder) fragment(node *syntax.Fragment) {
	if len(b.fragments) >= MaxFragments {
		b.report(diag.C314, node.NameSpan,
			fmt.Sprintf("a template holds at most %d fragments", MaxFragments),
			"merge neighbouring fragments, or move the repetition into a loop")
		b.nodes(node.Body)
		return
	}
	if b.named(node.Name) {
		b.report(diag.C314, node.NameSpan, fmt.Sprintf("two fragments are both named %s", node.Name),
			"a fragment name is its cache key; give them different names")
		b.nodes(node.Body)
		return
	}
	ttl, ok := b.window(node.Cache, "cache", node.NameSpan)
	if !ok {
		b.nodes(node.Body)
		return
	}
	stale, ok := b.window(node.Stale, "stale", node.NameSpan)
	if !ok {
		b.nodes(node.Body)
		return
	}

	if node.Defer && !b.deferrable(node) {
		b.nodes(node.Body)
		return
	}
	if node.Strategy != "" && !node.Defer {
		b.report(diag.C318, node.NameSpan, "a strategy only means something on a deferred fragment",
			`write {% fragment "`+node.Name+`" defer="visible" %}, or drop the strategy`)
		b.nodes(node.Body)
		return
	}
	if len(node.Placeholder) > 0 && !node.Defer {
		b.report(diag.C319, node.HoldSpan, "only a deferred fragment has a placeholder",
			"add defer to the fragment, or drop the placeholder section")
		b.nodes(node.Body)
		return
	}

	index := uint32(len(b.fragments))
	b.fragments = append(b.fragments, ir.Fragment{
		Name:     node.Name,
		TTL:      ttl,
		Stale:    stale,
		Deferred: node.Defer,
		Strategy: node.Strategy,
	})

	start := b.emit(ir.Op{Kind: ir.OpFragment, A: index})
	reads := []uint32{}
	outer := b.reads
	b.reads = &reads
	b.nodes(node.Body)
	b.reads = outer
	for _, read := range reads {
		b.record(read)
	}
	b.fragments[index].BodyEnd = b.here()
	b.hold(index, node)
	b.patch(start, b.here())
	b.fragments[index].Paths = reads
	b.mergeable = -1
}

func (b *builder) hold(index uint32, node *syntax.Fragment) {
	if len(node.Placeholder) == 0 {
		return
	}
	jump := b.emit(ir.Op{Kind: ir.OpJump})
	b.fragments[index].Hold = b.here()
	b.nodes(node.Placeholder)
	b.fragments[index].HoldEnd = b.here()
	b.patch(jump, b.here())
	b.mergeable = -1
}

func (b *builder) deferrable(node *syntax.Fragment) bool {
	if b.nested > 0 {
		b.report(diag.C318, node.NameSpan, "a deferred fragment cannot sit inside a loop, a branch or a component",
			"move it to the top level of the template, or drop defer")
		return false
	}
	if !b.deferred[node.Name] {
		b.report(diag.C318, node.NameSpan,
			fmt.Sprintf("%s is deferred but the go block has no func %s", node.Name, node.Name),
			fmt.Sprintf("write func %s(ctx *rill.Ctx) (T, error) in the --- block, or drop defer", node.Name))
		return false
	}
	if node.Strategy == "" {
		return true
	}
	if !config.KnownStrategy(node.Strategy) {
		b.report(diag.C318, node.NameSpan,
			fmt.Sprintf("%s is not a fragment strategy", node.Strategy),
			"write defer=\""+strings.Join(config.Strategies(), "\" or defer=\"")+"\", or plain defer")
		return false
	}
	if !b.fetches {
		b.report(diag.C318, node.NameSpan,
			fmt.Sprintf("defer=%q needs [fragments] deferred = %q", node.Strategy, config.DeferredFetch),
			"the other modes send the fragment inside the same response, so there is nothing left to wait for")
		return false
	}
	return true
}

func (b *builder) named(name string) bool {
	for _, fragment := range b.fragments {
		if fragment.Name == name {
			return true
		}
	}
	return false
}

func (b *builder) window(text, setting string, span diag.Span) (int64, bool) {
	if text == "" {
		return 0, true
	}
	value, err := time.ParseDuration(text)
	if err != nil || value < 0 {
		b.report(diag.C314, span, fmt.Sprintf("%s=%q is not a duration", setting, text),
			`write a Go duration such as "5m" or "1h30m"`)
		return 0, false
	}
	return int64(value), true
}
