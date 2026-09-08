package compile

import (
	"fmt"
	"slices"
	"strings"

	"github.com/apptivitypl/gopage/internal/config"
	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/ir"
	"github.com/apptivitypl/gopage/internal/syntax"
)

func (s *state) varyOf(route Route) []ir.Vary {
	var dimensions []ir.Vary
	for _, file := range append(slices.Clone(s.standing(route.Layouts)), route.File) {
		template, ok := s.templates[file]
		if !ok {
			continue
		}
		for _, entry := range template.Document.Varies {
			if index := indexOfVary(dimensions, entry); index >= 0 {
				dimensions[index].Values = entry.Values
				continue
			}
			dimensions = append(dimensions, ir.Vary{
				Kind:   ir.VaryKind(entry.Kind),
				Name:   entry.Name,
				Values: entry.Values,
			})
		}
	}
	return dimensions
}

func indexOfVary(dimensions []ir.Vary, entry syntax.Vary) int {
	for index, held := range dimensions {
		if held.Kind == ir.VaryKind(entry.Kind) && strings.EqualFold(held.Name, entry.Name) {
			return index
		}
	}
	return -1
}

func CheckVary(document *syntax.Document, file string, settings config.Config, bag *diag.Bag) {
	answering := strings.HasSuffix(file, PageFile) || strings.HasSuffix(file, LayoutFile)
	for _, entry := range document.Varies {
		if !answering {
			bag.Add(diag.New(diag.C114, file, entry.Span,
				"{% vary %} outside a page or a layout").
				WithHelp("the cache key belongs to the route; move the directive to the page that answers, " +
					"or to the layout above it"))
			continue
		}
		if entry.Kind == syntax.VaryCookie && settings.Security.Personal(entry.Name) {
			bag.Add(diag.New(diag.C330, file, entry.Span,
				fmt.Sprintf("cookie %s is named in security.privateCookies, so the response is never cached", entry.Name)).
				WithHelp("drop the directive, or take the cookie out of security.privateCookies"))
		}
	}
}

func CheckVariants(name string, dimensions []ir.Vary, ceiling int, file string, bag *diag.Bag) {
	total := ir.Variants(dimensions)
	if ceiling <= 0 || total <= ceiling {
		return
	}
	bag.Add(diag.Warn(diag.W706, file, diag.Span{},
		fmt.Sprintf("route %s asks the cache for %d variants, and cache.variants allows %d", name, total, ceiling)).
		WithHelp("drop a vary directive, shorten one of its value lists, or raise cache.variants"))
}
