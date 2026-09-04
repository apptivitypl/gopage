# Architecture

How a `.gopage` file becomes bytes on a wire, and which package owns each step.

## The shape of the idea

Most template engines walk a tree on every request. gopage walks it once, at build time, and writes
down a flat list of instructions. What remains at request time is an interpreter over a byte slice.

That has a second consequence, which is the real point: once the page is a plan, the server can
execute *part* of it. A fragment. A shell without its body. A page whose layout the browser already
holds. The plan is what makes "send less" a lookup rather than a special case.

## The build

`internal/build.Run` is the whole pipeline. In order:

1. **Load the config.** `internal/config` reads `gopage.jsonc` through `internal/jsonc`, which strips
   comments and trailing commas by overwriting them with spaces — every byte offset survives, so a
   decode error still names the line the reader is looking at. Decoding is strict: an unknown key
   is an error that names itself.

2. **Compile.** `internal/compile` walks `app/` and `components/`. `internal/syntax` lexes and
   parses each file against a grammar; the Go block at the top of a template is parsed as Go, and
   the types declared there become the component's props. Every rejection is a `diag.Diagnostic`
   with a code, a file, a position and a help line. `internal/diag` owns the registry.

3. **Bundle.** `internal/bundle` drives esbuild — the Go port, not the Node one — over the islands
   the compiler found. Islands are written as intermediate `.tsx` under `.gopage/cache/islands` and
   bundled from there. `internal/css` runs the Tailwind standalone binary when the project asks for
   it, over a class inventory the compiler wrote, because Tailwind cannot read `.gopage`.

4. **Lower.** `internal/compile/lower.go` turns each template into `internal/ir` operations. Runs
   of static markup collapse into a single `OpStatic` pointing at a range of one shared blob. A
   page is mostly `copy`.

5. **Generate and write.** `internal/build/generate.go` emits the Go that a project embeds:
   one package per route, a registry, and `app.go` with the `go:embed` directives. Everything
   embedded has to sit inside `internal/gen`, because `go:embed` cannot reach outside its own
   package directory or into a dot-directory. That single constraint decides the whole layout,
   which `internal/paths` states once for everyone.

## The request

`internal/server` owns the HTTP surface. A request is matched against the manifest, then:

- **Reserved and static first.** Assets, `public/` files and redirects are answered before any
  render is considered.
- **The cache.** `internal/cache` is bounded by bytes, not entries, and evicts by least recent use.
  A cache key carries everything that changes the answer — route, locale, host, and the loader's
  own declared inputs. What may not enter a key is the point of the `I2` invariant test.
- **Partial navigation.** When the browser sends the partial header, the server compares the chain
  of layouts it holds against the one this route needs and sends only the suffix that differs. A
  missing or malformed header is not an error; it answers with the whole document.
- **Fragments.** A deferred fragment is either inlined, flushed in the tail of the same response,
  or fetched by the browser on its own, depending on `fragments.deferred`. The shell is cacheable
  even when the body is not, which is why the mode exists.
- **Render.** `internal/runtime` interprets the plan. It writes into a pooled buffer and escapes on
  the way out; there is no intermediate string.

## Where a value is allowed to land

HTML escaping is correct inside markup and inside a quoted attribute, and wrong everywhere a second
grammar reads the same bytes. A `<script>` body is a JavaScript program, a `<style>` body is a
stylesheet, an `on*` attribute is a program the tokenizer decodes *before* it compiles, and `srcdoc`
is a whole document. One escaper cannot serve all of them.

Because the plan is built at build time, the compiler already knows which context an interpolation
lands in, so this costs nothing per request:

- A URL attribute — `href`, `src`, `action`, `formaction` and their kin — lowers to `OpURL` instead
  of `OpText`. It filters the scheme the way a browser reads it, stripping the control characters
  that would otherwise hide `java\tscript:`, and writes nothing when the scheme is not one a link
  may carry.
- The four contexts an escaper cannot rescue are refused at compile time, with a code that says
  which one and what to do instead: [C321](docs/errors/C321.md) for a script body,
  [C322](docs/errors/C322.md) for css, [C323](docs/errors/C323.md) for an event handler and
  [C324](docs/errors/C324.md) for `srcdoc`.

There is deliberately no `raw`, `unsafe` or `html` filter to reach for. The filter table is closed
and an unknown name is a hard error, so there is no escape hatch to audit.

## Two targets, one project

`gopage build --target native` compiles `cmd/server` into `dist/server` with everything embedded.
`--target workers` compiles `cmd/worker` to `js/wasm`, generates the JS shim and writes
`wrangler.jsonc` with the assets pointed at `dist/assets`.

The library builds for `js/wasm`; the CLI does not, and does not need to — the configurator uses a
terminal UI that has no meaning in a worker.

Keeping the two honest is a CI job rather than a claim: the reference application is built both
ways, both are served, and the same paths are fetched from each and compared after normalising the
origin. A difference fails the build.

## The gates

`cmd/gopagetool` is a second binary that never ships. It owns the rules: coverage with a ratchet,
the diagnostic registry, the config schema, the performance log, and the deploy smoke test.
`go run ./cmd/gopagetool ci` is what CI runs, so a green local run means a green remote one.

`dev.jsonc` holds the thresholds a human sets. `dev.lock.json` holds the two ratchets a machine
writes — coverage per package, and the benchmark and bundle-size baseline.

## Testing

`internal/invariant` holds the four properties that must not break, stated as tests rather than
prose: the cache stays inside its budget, a private value cannot enter a shared fragment, a render
from any root rebuilds the same document, and a broken partial header still answers a whole chain.

Fuzzing runs against the two places that read untrusted bytes: the template parser and the IR
decoder.
