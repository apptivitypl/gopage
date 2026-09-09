# Contributing

Open an issue before a large change. A pull request that arrives without one may be refused on
grounds that have nothing to do with its quality.

## The rules CI enforces

These are not style preferences. Every one of them fails a build.

1. **Engine code carries no explanatory comments.** If a piece of code needs a paragraph to be
   understood, the names are wrong. Comments belong in `docs/errors`, in this file, and in commit
   messages, where they can be as long as they need to be.
2. **90% statement coverage, ratcheted.** A package that sits above its locked figure may not drop
   below it. A package may sit below the global gate only with a written justification in
   `dev.jsonc`; the parser refuses an exemption that is not signed.
3. **Tests need nothing installed.** No network, no database, no Docker. A test that reaches the
   internet is a test that fails on someone else's machine.
4. **No Makefile and no shell scripts.** The tooling is Go, in `cmd/gopagetool`. A contributor who
   has Go has the whole toolchain.
5. **A new diagnostic code needs a page and a test.** Every code in the registry must have
   `docs/errors/<code>.md`, an entry in that directory's index, and a test that actually produces
   it. `gopagetool diag` fails otherwise.
6. **The config schema and the Go struct move together.** `schema/gopage.schema.json` is checked
   against `internal/config` by reflection; a field added to one and not the other fails the build.
7. **The editor grammar and the compiler move together.** Every directive, filter, activation
   strategy and built-in component the compiler knows must appear in
   `editors/vscode/syntaxes/gopage.tmLanguage.json`, and nothing else may. `gopagetool vscode check`
   reads the four alternations out of the grammar and compares them with `syntax.Directives()`,
   `runtime.FilterNames()`, `compile.Strategies()` and `compile.BuiltinNames()`. Adding a directive
   to the parser and not to the grammar fails the build.
8. **The committed examples are the templates' output.** `gopagetool example` regenerates
   `examples/hello-world`, `examples/blog` and `examples/catalog`, and fails on any difference. Fix
   one by changing the template and running `gopagetool example --update`, never by editing the
   example. They require a published gopage, so to build one against your checkout write a workspace
   first: `gopagetool example --workspace`. It names the version the example's own `go.mod` pins,
   and `GOWORK=off` runs the tool while the workspace is broken. `gopagetool example --verify`
   builds them with the published gopage they pin rather than with this checkout, because that is
   what someone outside the repository has; a branch that changes generated code therefore does not
   fail it, and the examples are re-pinned after the release that publishes the change.
9. **The version lives in the tag, not in the tree.** See Releases below.
10. **A regression is a bug until it is explained.** `gopagetool bench --check` compares against
    the figures in `dev.lock.json`. If a change makes something slower or larger, either fix it or
    say in the pull request why the cost buys something worth more.

## Before you push

```bash
go run ./cmd/gopagetool ci
```

That runs the same gates CI does, in the same order: gofmt, `go vet`, golangci-lint, the tests, the
diagnostic registry, the config schema, the editor grammar and the coverage gate. It needs
`golangci-lint` on your PATH; the version CI pins is in `.github/workflows/ci.yml`.

Three gates it does not run, because they are slower:

```bash
go run ./cmd/gopagetool bench --check
```

```bash
PATH="$PWD/node_modules/.bin:$PATH" go run ./cmd/gopagetool smoke --reference
```

```bash
pnpm --filter gopage test:grammar && pnpm --filter gopage test:unit && pnpm --filter gopage test:integration
```

The second builds the reference application for both targets and checks that they answer with the
same documents. It needs `pnpm install` first, and so does the third, which downloads a Visual
Studio Code to run the extension in. On Linux the extension host needs a display server, so CI wraps
that last command in `xvfb-run -a`.

The extension's own diagnostics are checked against a real language server, so point
`GOPAGE_BINARY` at one to run them: `go build -o /tmp/gopage ./cmd/gopage` and then
`GOPAGE_BINARY=/tmp/gopage pnpm --filter gopage test:integration`. Without it those two tests skip
rather than fail, because a checkout has no binary until something builds one.

The grammar tests pass four stub grammars from `editors/vscode/test/grammar/stubs`. They are not
decoration. `vscode-textmate` drops a rule whose `include` names a grammar it cannot resolve, and
the test runner only ever loads the grammars this extension itself contributes, so without the
stubs the frontmatter and island rules vanish and their tests pass by matching nothing. The stubs
declare the four scopes the grammar delegates to and hold no patterns, which is enough to make the
delegation observable while leaving the assertions about our own scopes exact.

## Layout

```
cmd/gopage/            the CLI a user installs
cmd/gopagetool/        the gates; never shipped
internal/syntax/     lexer and parser for .gopage
internal/compile/    the compiler frontend, and every diagnostic
internal/ir/         the render plan and its codec
internal/runtime/    the plan interpreter
internal/server/     routing, caching, fragments, the HTTP surface
internal/cache/      the bounded response cache and the policy a loader records
internal/reply/      the status, headers and cookies a loader asks for
internal/vocab/      locale prefixes, and the segment each locale publishes
internal/seo/        the sitemap and robots.txt documents
internal/image/      decoding, scaling and re-encoding behind /_gopage/image
internal/og/         the open graph card
og/                  the public wrapper an app imports to draw one
images/              the public wrapper the generated code imports when images are on
internal/build/      the build pipeline and code generation
internal/paths/      where everything lands on disk, stated once
internal/scaffold/   the templates gopage new writes
editors/vscode/      the visual studio code extension
internal/devserver/  the process gopage dev supervises, and the proxy in front of it
internal/demo/       the node server the demo target ships
examples/            the templates' output, committed and checked
npm/                 the hand-written half of the npm packages
docs/errors/         one page per diagnostic code
```

`internal/paths` is the single statement of a project's on-disk shape. If you are about to write a
path literal anywhere else, put it there instead.

## Commits

Conventional Commits for the subject, then a paragraph explaining why. The subject says what
changed; the body says what was wrong before, and why this is the fix rather than another one. A
commit that only says what the diff already shows is a wasted commit message.

## Releases

Nothing in this repository records the version of the next release. A release is `publish.yml`
dispatched with a version, and that number is the only place it exists: the git tag is written from
it, goreleaser names the archives after that tag, and the npm packages are assembled at the same
number. Two versions can no longer disagree because there is only one.

`gopagetool release plan --version X.Y.Z` asks git and the npm registry whether that version is
already out and reports only what is missing, so dispatching the same version twice publishes
nothing the second time. That makes a half-finished release recoverable: dispatch it again and the
tool skips what already landed.

Nothing in CI checks whether main has changes waiting for a release, because a repository that
carries no version has nothing to compare against. The consequence is deliberate: `gopagetool ci`
neither reaches the network nor reads git history, so it answers the same question offline and on a
shallow clone as it does on a release runner.

For the Go module, goreleaser builds the archives, cosign signs the checksums against the workflow's
own identity, and the release carries an SBOM and a provenance attestation for every artifact.

The npm packages are assembled from those archives, never built separately:
`gopagetool release run @apptivitypl/gopage --from <archives>` writes `dist/npm`, generating every
`package.json` from the manifest so a version can never drift. With no package name it assembles
everything the plan says is missing. Without `--publish` it stops at the folder and prints the
`npm publish` line it would have run.

`publish.yml` also runs nightly, moving the `nightly` prerelease onto the tip of `main` when `main`
has moved. Snapshot builds skip goreleaser's signing pipe, so the workflow signs `checksums.txt`
itself. Nothing nightly reaches npm, because a published version can never be taken back.

Nothing in CI holds an npm token. Every package is configured on npmjs.com with a trusted publisher
pointing at `publish.yml`, so the workflow authenticates over OIDC. The publish job calls
`actions/setup-node` without `registry-url`: with it the action writes an empty `_authToken` that npm
reads as authentication, and the OIDC exchange never happens.

npm refuses to configure a trusted publisher for a name it has never seen, so a new package has to be
published once by hand and then passed to `gopagetool release trust`, which needs 2FA on the account
and a browser login. Weigh that before adding one.

### The editor extension

`editors/vscode` is released on its own schedule, because a fix to the grammar has nothing to do
with a release of the compiler. `vscode.yml` is dispatched with a version, the same way `publish.yml`
is, and that number is again the only place the version exists: the committed manifest carries
`0.0.0`, the workflow writes the dispatched version into it with `gopagetool vscode version --set`,
and the tag `gopage-vscode@X.Y.Z` is pushed once both registries have accepted the package. Rule 7
refuses a manifest that carries anything but `0.0.0`, so a stamped checkout can never be committed.

The same package is published to the Visual Studio Marketplace and to Open VSX, from one built
`.vsix`, so the two listings can never diverge. `--skip-duplicate` on both makes a re-dispatch safe
after a partial failure. The Marketplace authenticates over OIDC, so no token for it exists
anywhere; Open VSX has no equivalent yet and reads `OVSX_PAT` from the `vscode-marketplace`
environment.

The Marketplace has no prerelease versions, only channels, and it moves every user to the highest
version on offer. The channel is therefore carried by the minor: **an even minor is released, an odd
minor is pre-release**. `gopagetool vscode version` refuses a version whose minor disagrees with the
dispatched channel, because publishing a released `0.3.0` over a pre-release `0.3.x` would silently
pull every pre-release user back onto the stable channel.

## Licence

Contributions are licensed as MIT OR Apache-2.0, the same as the project, without any further
terms. See [LICENSE](LICENSE).
