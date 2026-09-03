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
4. **No Makefile and no shell scripts.** The tooling is Go, in `cmd/rilltool`. A contributor who
   has Go has the whole toolchain.
5. **A new diagnostic code needs a page and a test.** Every code in the registry must have
   `docs/errors/<code>.md`, an entry in that directory's index, and a test that actually produces
   it. `rilltool diag` fails otherwise.
6. **The config schema and the Go struct move together.** `schema/rill.schema.json` is checked
   against `internal/config` by reflection; a field added to one and not the other fails the build.
7. **A regression is a bug until it is explained.** `rilltool bench --check` compares against the
   figures in `dev.lock.json`. If a change makes something slower or larger, either fix it or say
   in the pull request why the cost buys something worth more.

## Before you push

```bash
go run ./cmd/rilltool ci
```

That runs the same gates CI does, in the same order: gofmt, `go vet`, golangci-lint, the tests, the
diagnostic registry, the config schema and the coverage gate. It needs `golangci-lint` on your
PATH; the version CI pins is in `.github/workflows/ci.yml`.

Two gates it does not run, because they are slower:

```bash
go run ./cmd/rilltool bench --check
```

```bash
PATH="$PWD/node_modules/.bin:$PATH" go run ./cmd/rilltool smoke --reference
```

The second builds the reference application for both targets and checks that they answer with the
same documents. It needs `pnpm install` first.

## Layout

```
cmd/rill/            the CLI a user installs
cmd/rilltool/        the gates; never shipped
internal/syntax/     lexer and parser for .rill
internal/compile/    the compiler frontend, and every diagnostic
internal/ir/         the render plan and its codec
internal/runtime/    the plan interpreter
internal/server/     routing, caching, fragments, the HTTP surface
internal/build/      the build pipeline and code generation
internal/paths/      where everything lands on disk, stated once
internal/scaffold/   the templates rill new writes
docs/errors/         one page per diagnostic code
```

`internal/paths` is the single statement of a project's on-disk shape. If you are about to write a
path literal anywhere else, put it there instead.

## Commits

Conventional Commits for the subject, then a paragraph explaining why. The subject says what
changed; the body says what was wrong before, and why this is the fix rather than another one. A
commit that only says what the diff already shows is a wasted commit message.

## Releases

The version lives in `VERSION` and nowhere else. A release is a tag that matches it: CI refuses to
publish when the tag and the file disagree. goreleaser builds the archives, cosign signs the
checksums against the workflow's own identity, and the release carries an SBOM and a provenance
attestation for every artifact.

## Licence

Contributions are licensed as MIT OR Apache-2.0, the same as the project, without any further
terms. See [LICENSE](LICENSE).
