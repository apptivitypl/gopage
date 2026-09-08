package compile

import (
	"strings"

	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/syntax"
)

func CheckStandalone(document *syntax.Document, file string, isLayout bool, bag *diag.Bag) {
	if !document.Standalone {
		return
	}
	if !isLayout {
		for _, span := range document.Standalones {
			bag.Add(diag.New(diag.C113, file, span, "{% standalone %} outside a layout").
				WithHelp("only a layout can start its own chain; move the directive to the layout above this file"))
		}
		return
	}
	missing := headless(document)
	if len(missing) == 0 {
		return
	}
	bag.Add(diag.Warn(diag.W705, file, document.Standalones[0],
		"a standalone layout carries no "+strings.Join(missing, " and ")).
		WithHelp("a layout that starts its own chain answers for the head; add the directive, " +
			"or drop {% standalone %} and let the layout above provide it"))
}

func headless(document *syntax.Document) []string {
	var missing []string
	if !holds[*syntax.MetaBlock](document) {
		missing = append(missing, "{% meta %}")
	}
	if !holds[*syntax.AssetsBlock](document) {
		missing = append(missing, "{% assets %}")
	}
	return missing
}

func holds[T syntax.Node](document *syntax.Document) bool {
	found := false
	syntax.Walk(document.Nodes, func(node syntax.Node) {
		if _, ok := node.(T); ok {
			found = true
		}
	})
	return found
}
