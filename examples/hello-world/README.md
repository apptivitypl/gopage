# Rendered in Go, interactive in React

This is what `gopage new my-site` writes: one page, served as HTML the server rendered, with four small
components that hydrate on their own, a loader that fetches a list before the response is sent, and a
JSON route beside the page.

<p>
  <a href="https://codesandbox.io/p/devbox/github/apptivitypl/gopage/tree/main/examples/hello-world"><img alt="open in codesandbox" src="https://assets.codesandbox.io/github/button-edit-lime.svg" height="30"></a>
</p>

## What is on the page

The hero, the copy and the source panel are HTML the server wrote. No JavaScript is involved in
them, and none is sent for them. Four things are interactive, and each one ships only itself:

- **Ticker** is a counter, the smallest thing that has to run in the browser.
- **Stars** asks GitHub for the star count once the page is already readable, and remembers the
  answer for an hour.
- **Response** calls `/api/stories` and shows the status, the content type and how long it took.
- **Theme toggle** writes the choice to `localStorage`. The document reads it in a blocking script
  in `app/layout.gopage`, before first paint, so the page never flashes the wrong theme.

The Hacker News list is the opposite case: `HackerNews` in `app/page.gopage` fetches it while the
request is being handled, so it arrives as HTML with everything else.

## Where to look

| file | what it shows |
| --- | --- |
| `app/page.gopage` | frontmatter is Go: `Props`, `Meta`, and a loader that returns the stories |
| `app/layout.gopage` | the document every page renders into |
| `app/api/stories/route.go` | a route is a `GET` function returning `gopage.JSON` |
| `app/not-found.gopage`, `app/error.gopage` | the two pages you do not write until you need them |
| `components/Ticker.gopage` | markup, then `<script client>`: that attribute is the whole opt-in |
| `components/Response.gopage` | a React island, typed against `gopage:props/Response` |
| `server/hackernews/` | ordinary Go the loader calls, split by build tag for the worker |
| `gopage.jsonc` | languages, reserved prefixes, css engine, navigation mode |

## Running it

```bash
pnpm install
```

```bash
gopage dev
```

It watches the project, rebuilds what changed and reloads the browser. `pnpm install` is only for
React; a template without an interactive component needs nothing from npm.

## Deploying it

```bash
gopage build --target workers && wrangler deploy
```

```bash
gopage build --target native && ./dist/server
```

The worker build writes `wrangler.jsonc` beside the project and puts the assets where Static Assets
expects them. The native build is one binary with everything inside it.

## When the story list looks canned

The loader calls the Hacker News API. Where nothing can reach it, which covers a worker without
outbound access and the WebAssembly demo in a browser tab, `server/hackernews/edge.go` answers with
a built-in list instead, so the page still renders. That is the intended behaviour, not a failure.

---

Generated from the `hello-world` template. Change the template rather than this folder; see
[CONTRIBUTING](../../CONTRIBUTING.md).
