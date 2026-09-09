<p align="center">
  <img alt="the gopage mark" src="https://raw.githubusercontent.com/apptivitypl/gopage/main/editors/vscode/images/icon.png" width="120" height="120">
</p>

<h1 align="center">GoPage for Visual Studio Code</h1>

<p align="center">
  <b>A web framework in Go that sends HTML first.</b><br>
  <i>Compile the whole page. Execute the smallest part of it.</i>
</p>

Language support for [GoPage](https://github.com/apptivitypl/gopage) templates. A `.gopage` file is
Go frontmatter followed by markup, and this extension understands both halves.

## What it does

- **Highlighting that follows the file.** The frontmatter between `---` fences highlights as Go, the
  body as HTML, `<script client>` islands as TypeScript, and `{{ … }}`, `{% … %}` and
  `<Component :Prop="…" />` as the template language they are.
- **Diagnostics as you type.** Every `GOPAGE-C…` the compiler would report appears in the Problems
  panel, with the same message and the same fix.
- **Completion and hover.** Directives inside `{% %}`, filters after `|`, the fields of your `Props`
  and the built-in components, each with what it does.
- **Snippets** for every directive, for the frontmatter skeleton, and for an island.
- **Validation for `gopage.jsonc`**, driven by the schema the compiler itself is checked against.

Diagnostics, completion and hover come from `gopage lsp`, which ships inside the `gopage` command.
Highlighting and snippets need nothing installed.

## Installing gopage

The extension looks for the binary in the `gopage.path` setting, then in the workspace's
`node_modules/.bin`, then on `PATH`.

```bash
curl -fsSL https://raw.githubusercontent.com/apptivitypl/gopage/main/install.sh | sh
```

Or as a project dependency:

```bash
pnpm add -D @apptivitypl/gopage
```

Without a binary the extension still highlights and completes snippets; it says so once and stays
out of the way.

## Settings

| Setting | Default | What it does |
| --- | --- | --- |
| `gopage.path` | `""` | Where the `gopage` binary is. Empty searches the workspace, then `PATH`. |
| `gopage.languageServer.enabled` | `true` | Run `gopage lsp` for diagnostics, completion and hover. |
| `gopage.trace.server` | `off` | Trace the messages exchanged with the language server. |

## Commands

| Command | What it does |
| --- | --- |
| **GoPage: Build the project** | Runs `gopage build` as a task. |
| **GoPage: Start the development server** | Runs `gopage dev` as a task. |
| **GoPage: Restart the language server** | Restarts `gopage lsp`, after installing a new binary. |

> [!WARNING]
> **The GoPage API is not stable.** Templates, configuration and the Go API can change between
> releases, sometimes in ways that need edits in your project. Pin a version, and read the release
> notes before you raise it.

## Licence

MIT OR Apache-2.0, the same as GoPage itself.
