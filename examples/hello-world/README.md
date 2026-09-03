# hello-world

What `rill new my-site` writes with the defaults, committed so you can read it without running
anything. One page with a live component, a fetched list and a JSON route.

**Generated, not hand-written.** `rilltool example` regenerates it from
[internal/scaffold/templates/hello-world](../../internal/scaffold/templates/hello-world) and CI fails
on any difference, so a change belongs in the template. Then:

```bash
go run ./cmd/rilltool example --update
```

## Running it

```bash
pnpm install && rill dev
```

`go.mod` requires a published rill rather than replacing it with a path, so the folder stands on its
own and an online editor can open it directly. Working on rill itself instead? Point the examples at
your checkout with a workspace, which is what the devcontainer does:

```bash
go run ./cmd/rilltool example --workspace
```

## Try it online

<p>
  <a href="https://stackblitz.com/github/apptivitypl/rill/tree/main/examples/hello-world"><img alt="open in stackblitz" src="https://developer.stackblitz.com/img/open_in_stackblitz.svg" height="30"></a>
  <a href="https://codesandbox.io/p/devbox/github/apptivitypl/rill/tree/main/examples/hello-world"><img alt="open in codesandbox" src="https://assets.codesandbox.io/github/button-edit-lime.svg" height="30"></a>
</p>

Both buttons open this folder. CodeSandbox reads `.devcontainer` and `.codesandbox`, boots a machine
with Go on it and runs `rill dev`, so editing a `.rill` file rebuilds and reloads. StackBlitz has no
Go toolchain, so `.stackblitzrc` starts the published [@apptivitypl/rill-demo-hello-world](https://www.npmjs.com/package/@apptivitypl/rill-demo-hello-world)
instead — this same code compiled to WebAssembly, answering every request, but not recompiling a
template.

## One thing to know about the story list

The loader fetches Hacker News. Where the network is not reachable — a worker without outbound
access, or a WebContainer — `server/hackernews/edge.go` answers with the built-in list instead, so
the page still renders. That is by design, not a failure.
