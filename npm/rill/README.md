# @apptivitypl/rill

The [rill](https://github.com/apptivitypl/rill) command line, delivered as a prebuilt binary.

```bash
pnpm add -D @apptivitypl/rill
```

```bash
pnpm exec rill new my-site
```

Or without adding a dependency:

```bash
pnpm dlx @apptivitypl/rill new my-site
```

The package holds a small launcher; the binary for your platform arrives as an optional dependency,
so nothing is compiled and no install script runs. Six platforms are published as
`@apptivitypl/rill-<system>-<architecture>`: linux, macOS and Windows, on x64 and arm64. Each one also
carries its own command, so `pnpm dlx @apptivitypl/rill-linux-x64` works where optional dependencies are
turned off.

rill compiles your templates into Go and then calls `go build`. It uses the Go toolchain on your
PATH; when there is none, the first build fetches a pinned one into the rill cache, checks it against
a published sha256 and keeps it there. `RILL_GO` points at a toolchain you would rather it used.

Licensed MIT OR Apache-2.0.
