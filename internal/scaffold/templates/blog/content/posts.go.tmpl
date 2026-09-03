package content

import (
	"embed"
	"errors"
	"io/fs"
	"sort"
	"strings"
)

//go:embed all:posts
var files embed.FS

type Post struct {
	Slug    string
	Title   string
	Date    string
	Summary string
	Body    string
}

var ErrNotFound = errors.New("no such post")

func All() ([]Post, error) {
	entries, err := fs.ReadDir(files, "posts")
	if err != nil {
		return nil, err
	}
	posts := make([]Post, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		post, err := read(strings.TrimSuffix(entry.Name(), ".md"))
		if err != nil {
			return nil, err
		}
		posts = append(posts, post)
	}
	sort.Slice(posts, func(i, j int) bool { return posts[i].Date > posts[j].Date })
	return posts, nil
}

func Get(slug string) (Post, error) {
	if strings.ContainsAny(slug, "./\\") {
		return Post{}, ErrNotFound
	}
	return read(slug)
}

func read(slug string) (Post, error) {
	data, err := fs.ReadFile(files, "posts/"+slug+".md")
	if err != nil {
		return Post{}, ErrNotFound
	}
	post := Post{Slug: slug}
	body := string(data)
	for line := range strings.SplitSeq(body, "\n") {
		name, value, found := strings.Cut(line, ": ")
		if !found {
			break
		}
		switch name {
		case "title":
			post.Title = value
		case "date":
			post.Date = value
		case "summary":
			post.Summary = value
		}
	}
	if cut := strings.Index(body, "\n\n"); cut >= 0 {
		post.Body = strings.TrimSpace(body[cut+2:])
	}
	if post.Title == "" {
		post.Title = slug
	}
	return post, nil
}
