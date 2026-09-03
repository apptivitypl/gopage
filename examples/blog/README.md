# Markdown posts, a feed, and no JavaScript

This is what `rill new my-site --template blog` writes: posts written in markdown, a page for each
one, and an RSS feed. Nothing runs in the browser — there is no interactive component here, so the
pages arrive as HTML and not a byte of JavaScript is sent.

<p>
  <a href="https://stackblitz.com/github/apptivitypl/rill/tree/main/examples/blog"><img alt="open in stackblitz" src="https://developer.stackblitz.com/img/open_in_stackblitz.svg" height="30"></a>
  <a href="https://codesandbox.io/p/devbox/github/apptivitypl/rill/tree/main/examples/blog"><img alt="open in codesandbox" src="https://assets.codesandbox.io/github/button-edit-lime.svg" height="30"></a>
</p>

## What it does

The posts are markdown files under `content/posts`, embedded into the binary with `go:embed`, so
there is no database and no build step that reads the disk at run time. The index lists them newest
first. Each post has its own page under a dynamic route, and the loader tags the result:

```go
ctx.Cache().TTL(time.Hour).Tag("posts", "post:" + post.Slug)
```

That tag is how a single post is dropped from the cache when it changes, without clearing the rest.

## Where to look

| file | what it shows |
| --- | --- |
| `app/page.rill` | the index — frontmatter is Go, `Load` returns what the markup renders |
| `app/posts/[slug]/page.rill` | a dynamic route; `params["slug"]` picks the post, and the loader sets a cache tag |
| `app/feed.xml/route.go` | a route that answers with something other than a page |
| `components/PostCard/` | a component big enough to want its own directory: `props.go` beside `template.rill` |
| `content/` | markdown, and the Go that reads it |
| `rill.jsonc` | plain css here, not Tailwind — the engine is a setting, not a rewrite |

## Adding a post

Drop a markdown file into `content/posts`. The front matter it expects is in `hello.md`.

## Running it

```bash
rill dev
```

No `pnpm install`: there is nothing from npm to fetch.

## Deploying it

```bash
rill build --target workers && wrangler deploy
```

```bash
rill build --target native && ./dist/server
```

---

Generated from the `blog` template. Change the template rather than this folder; see
[CONTRIBUTING](../../CONTRIBUTING.md).
