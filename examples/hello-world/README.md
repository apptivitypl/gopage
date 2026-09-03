# Rendered in Go, interactive in React

This is what `rill new my-site` writes: one page, served as HTML the server rendered, with four small
components that hydrate on their own, a loader that fetches a list before the response is sent, and a
JSON route beside the page.

<p>
  <a href="https://stackblitz.com/github/apptivitypl/rill/tree/main/examples/hello-world"><img alt="open in stackblitz" src="https://developer.stackblitz.com/img/open_in_stackblitz.svg" height="30"></a>
  <a href="https://codesandbox.io/p/devbox/github/apptivitypl/rill/tree/main/examples/hello-world"><img alt="open in codesandbox" src="https://assets.codesandbox.io/github/button-edit-lime.svg" height="30"></a>
</p>

## What is on the page

The hero, the copy and the source panel are HTML the server wrote. No JavaScript is involved in
them, and none is sent for them. Four things are interactive, and each one ships only itself:

- **Ticker** — a counter, the smallest thing that has to run in the browser.
- **Stars** — asks GitHub for the star count once the page is already readable, and remembers the
  answer for an hour.
- **Response** — calls `/api/stories` and shows the status, the content type and how long it took.
- **Theme toggle** — writes the choice to `localStorage`. The document reads it in a blocking script
  in `app/layout.rill`, before first paint, so the page never flashes the wrong theme.

The Hacker News list is the opposite case: `HackerNews` in `app/page.rill` fetches it while the
request is being handled, so it arrives as HTML with everything else.

## Where to look

| file | what it shows |
| --- | --- |
| `app/page.rill` | frontmatter is Go — `Props`, `Meta`, and a loader that returns the stories |
| `app/layout.rill` | the document every page renders into |
| `app/api/stories/route.go` | a route is a `GET` function returning `rill.JSON` |
| `app/not-found.rill`, `app/error.rill` | the two pages you do not write until you need them |
| `components/Ticker.rill` | markup, then `<script client>`: that attribute is the whole opt-in |
| `components/Response.rill` | a React island, typed against `rill:props/Response` |
| `server/hackernews/` | ordinary Go the loader calls, split by build tag for the worker |
| `rill.jsonc` | languages, reserved prefixes, css engine, navigation mode |

## Running it

```bash
pnpm install
```

```bash
rill dev
```

It watches the project, rebuilds what changed and reloads the browser. `pnpm install` is only for
React; a template without an interactive component needs nothing from npm.

## Deploying it

```bash
rill build --target workers && wrangler deploy
```

```bash
rill build --target native && ./dist/server
```

The worker build writes `wrangler.jsonc` beside the project and puts the assets where Static Assets
expects them. The native build is one binary with everything inside it.

## When the story list looks canned

The loader calls the Hacker News API. Where nothing can reach it — a worker without outbound access,
or the WebAssembly demo in a browser tab — `server/hackernews/edge.go` answers with a built-in list
instead, so the page still renders. That is the intended behaviour, not a failure.

---

Generated from the `hello-world` template. Change the template rather than this folder; see
[CONTRIBUTING](../../CONTRIBUTING.md).
