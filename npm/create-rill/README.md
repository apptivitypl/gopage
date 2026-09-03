# @apptivitypl/create-rill

Scaffolds a [rill](https://github.com/apptivitypl/rill) project.

```bash
pnpm create @apptivitypl/rill my-site
```

Everything after the name is passed to `rill new`, so `--template blog`, `--locales en,pl` and the
rest work the same way. Without `--yes` it asks for the module path, template, languages, navigation
mode, css engine and theme.

It installs [@apptivitypl/rill](https://www.npmjs.com/package/@apptivitypl/rill) to do the work.

rill compiles your templates into Go and then calls `go build`. It uses the Go toolchain on your
PATH, and fetches a pinned one into its own cache when there is none.

Licensed MIT OR Apache-2.0.
