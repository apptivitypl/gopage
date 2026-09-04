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
7. **The committed examples are the templates' output.** `gopagetool example` regenerates
   `examples/hello-world` and `examples/blog` and fails on any difference. Fix one by changing the
   template and running `gopagetool example --update`, never by editing the example. They require a
   published gopage, so to build one against your checkout write a workspace first:
   `gopagetool example --workspace`. It names the version the example's own `go.mod` pins, and
   `GOWORK=off` runs the tool while the workspace is broken.
8. **The version lives in the tag, not in the tree.** See Releases below.
9. **A regression is a bug until it is explained.** `gopagetool bench --check` compares against the
   figures in `dev.lock.json`. If a change makes something slower or larger, either fix it or say
   in the pull request why the cost buys something worth more.

## Before you push

```bash
go run ./cmd/gopagetool ci
```

That runs the same gates CI does, in the same order: gofmt, `go vet`, golangci-lint, the tests, the
diagnostic registry, the config schema and the coverage gate. It needs `golangci-lint` on your
PATH; the version CI pins is in `.github/workflows/ci.yml`.

Two gates it does not run, because they are slower:

```bash
go run ./cmd/gopagetool bench --check
```

```bash
PATH="$PWD/node_modules/.bin:$PATH" go run ./cmd/gopagetool smoke --reference
```

The second builds the reference application for both targets and checks that they answer with the
same documents. It needs `pnpm install` first.

## Layout

```
cmd/gopage/            the CLI a user installs
cmd/gopagetool/        the gates; never shipped
internal/syntax/     lexer and parser for .gopage
internal/compile/    the compiler frontend, and every diagnostic
internal/ir/         the render plan and its codec
internal/runtime/    the plan interpreter
internal/server/     routing, caching, fragments, the HTTP surface
internal/build/      the build pipeline and code generation
internal/paths/      where everything lands on disk, stated once
internal/scaffold/   the templates gopage new writes
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
carries no version has nothing to compare against. The consequence is deliberate — `gopagetool ci`
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

## Licence

Contributions are licensed as MIT OR Apache-2.0, the same as the project, without any further
terms. See [LICENSE](LICENSE).
