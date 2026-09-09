# Changelog

## Unreleased

### Added

- Syntax highlighting for `.gopage`: Go frontmatter, HTML body, `{{ … }}` interpolations,
  `{% … %}` directives, `<Component :Prop="…" />` tags and `<script client>` islands.
- Diagnostics, completion and hover through `gopage lsp`, found in the `gopage.path` setting, the
  workspace's `node_modules/.bin`, or on `PATH`.
- Snippets for every directive, the frontmatter skeleton, a component and an island.
- Schema validation for `gopage.jsonc`.
- **GoPage: Build the project**, **GoPage: Start the development server** and
  **GoPage: Restart the language server**.
