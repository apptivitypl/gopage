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

**rill still needs Go.** It compiles your templates into Go and then calls `go build`, so a Go
toolchain has to be on PATH. This package only saves you from installing rill itself.

Licensed MIT OR Apache-2.0.
