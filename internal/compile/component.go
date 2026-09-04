package compile

import (
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/apptivitypl/gopage/internal/diag"
	"github.com/apptivitypl/gopage/internal/paths"
	"github.com/apptivitypl/gopage/internal/schema"
	"github.com/apptivitypl/gopage/internal/syntax"
)

const (
	ComponentsDir  = paths.ComponentsDir
	TemplateFile   = "template.gopage"
	PropsFile      = "props.go"
	TemplateSuffix = ".gopage"
)

type Component struct {
	Name     string
	File     string
	Props    string
	Document *syntax.Document
	Schema   *schema.Schema
	Source   string
}

func (c Component) Fields() []schema.Field {
	if c.Schema == nil {
		return nil
	}
	props, ok := c.Schema.Props()
	if !ok {
		return nil
	}
	return props.Fields
}

func ComponentNames(components map[string]Component) []string {
	names := make([]string, 0, len(components))
	for name := range components {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func LoadComponent(fsys fs.FS, name, file string, bag *diag.Bag) (Component, bool) {
	template, ok := ReadTemplate(fsys, file, bag)
	if !ok {
		return Component{}, false
	}
	component := Component{
		Name:     name,
		File:     file,
		Document: template.Document,
		Source:   template.Source,
	}
	sources := template.Sources()
	if props, err := fs.ReadFile(fsys, path.Join(path.Dir(file), PropsFile)); err == nil {
		component.Props = string(props)
		sources = append(sources, schema.Source{File: path.Join(path.Dir(file), PropsFile), Code: stripPackage(string(props))})
	}
	if len(sources) > 0 {
		component.Schema = schema.Parse(sources, bag)
	}
	return component, true
}

func stripPackage(code string) string {
	var kept []string
	for line := range strings.SplitSeq(code, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "package ") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
