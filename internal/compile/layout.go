package compile

import (
	"sort"
	"strings"

	"github.com/apptivitypl/gopage/internal/ir"
)

const layoutPrefix = "layout"

func LayoutName(file string) string {
	dir := strings.TrimSuffix(strings.TrimSuffix(file, LayoutFile), "/")
	dir = strings.TrimPrefix(strings.TrimPrefix(dir, AppDir), "/")
	if dir == "" {
		return layoutPrefix
	}
	parts := []string{layoutPrefix}
	for segment := range strings.SplitSeq(dir, "/") {
		if trimmed := strings.Trim(segment, "()[].,"); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, ".")
}

func (s *state) loadingLayouts() []ir.Layout {
	var layouts []ir.Layout
	for file, template := range s.templates {
		if !template.IsLayout || !template.HasLoader() {
			continue
		}
		index, ok := s.planOf[file]
		if !ok {
			continue
		}
		layouts = append(layouts, ir.Layout{Name: LayoutName(file), Plan: index})
	}
	sort.Slice(layouts, func(i, j int) bool { return layouts[i].Name < layouts[j].Name })
	return layouts
}
