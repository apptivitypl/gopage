package invariant

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

func Normalize(document string) (string, error) {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return "", fmt.Errorf("parsing html: %w", err)
	}
	var b strings.Builder
	writeNode(&b, root)
	return b.String(), nil
}

func writeNode(b *strings.Builder, node *html.Node) {
	switch node.Type {
	case html.TextNode:
		if text := strings.Join(strings.Fields(node.Data), " "); text != "" {
			b.WriteString(text)
			b.WriteString(" ")
		}
		return
	case html.CommentNode:
		return
	case html.ElementNode:
		b.WriteString("<" + node.Data)
		writeAttributes(b, node)
		b.WriteString(">")
	default:
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		writeNode(b, child)
	}
	if node.Type == html.ElementNode {
		b.WriteString("</" + node.Data + ">")
	}
}

func writeAttributes(b *strings.Builder, node *html.Node) {
	names := make([]string, 0, len(node.Attr))
	values := make(map[string]string, len(node.Attr))
	for _, attribute := range node.Attr {
		names = append(names, attribute.Key)
		values[attribute.Key] = attribute.Val
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(b, " %s=%q", name, values[name])
	}
}
