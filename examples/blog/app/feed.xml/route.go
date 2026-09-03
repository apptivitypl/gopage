package route

import (
	"strings"

	"github.com/apptivitypl/rill"

	"github.com/apptivitypl/rill/examples/blog/content"
)

const feedType = "application/rss+xml; charset=utf-8"

func GET(ctx *rill.Ctx, params rill.Params) (rill.Response, error) {
	posts, err := content.All()
	if err != nil {
		return nil, err
	}
	origin := "https://" + ctx.Request().Host

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<rss version="2.0"><channel>` + "\n")
	b.WriteString("<title>posts</title>\n")
	b.WriteString("<link>" + origin + "/</link>\n")
	b.WriteString("<description>posts, newest first</description>\n")
	for _, post := range posts {
		b.WriteString("<item>\n")
		b.WriteString("<title>" + escape(post.Title) + "</title>\n")
		b.WriteString("<link>" + origin + "/posts/" + post.Slug + "</link>\n")
		b.WriteString("<guid isPermaLink=\"false\">" + post.Slug + "</guid>\n")
		b.WriteString("<description>" + escape(post.Summary) + "</description>\n")
		b.WriteString("</item>\n")
	}
	b.WriteString("</channel></rss>\n")
	return rill.Content(feedType, []byte(b.String())), nil
}

var escaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func escape(value string) string {
	return escaper.Replace(value)
}
