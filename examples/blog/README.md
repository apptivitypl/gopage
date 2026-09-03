# blog

What `rill new my-site --template blog` writes, committed so you can read it without running
anything. Markdown posts, a post page under a dynamic route, and a feed.

**Generated, not hand-written.** `rilltool example` regenerates it from
[internal/scaffold/templates/blog](../../internal/scaffold/templates/blog) and CI fails on any
difference, so a change belongs in the template. Then:

```bash
go run ./cmd/rilltool example --update
```

## Running it

```bash
rill dev
```

There are no browser packages here: the template ships no interactive component, so nothing from npm
is needed. `go.mod` requires a published rill rather than replacing it with a path, so the folder
stands on its own. Working on rill itself instead:

```bash
go run ./cmd/rilltool example --workspace
```

## Try it online

<p>
  <a href="https://stackblitz.com/github/apptivitypl/rill/tree/main/examples/blog"><img alt="open in stackblitz" src="https://developer.stackblitz.com/img/open_in_stackblitz.svg" height="30"></a>
  <a href="https://codesandbox.io/p/devbox/github/apptivitypl/rill/tree/main/examples/blog"><img alt="open in codesandbox" src="https://assets.codesandbox.io/github/button-edit-lime.svg" height="30"></a>
</p>

Both buttons open this folder. CodeSandbox reads `.devcontainer` and `.codesandbox`, boots a machine
with Go on it and runs `rill dev`, so editing a `.rill` file rebuilds and reloads. StackBlitz has no
Go toolchain, so `.stackblitzrc` starts the published [@apptivitypl/rill-demo-blog](https://www.npmjs.com/package/@apptivitypl/rill-demo-blog)
instead — this same code compiled to WebAssembly, answering every request, but not recompiling a
template.
