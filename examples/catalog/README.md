# Filters, two languages, and one island

This is what `gopage new my-site --template catalog` writes: a listing that filters without a page
reload, in English and Polish, with a single interactive component. It is the widest of the three
examples, and the one CI builds twice — once as a native binary, once as a Cloudflare Worker — to
prove both answer with the same document.

<p>
  <a href="https://codesandbox.io/p/devbox/github/apptivitypl/gopage/tree/main/examples/catalog"><img alt="open in codesandbox" src="https://assets.codesandbox.io/github/button-edit-lime.svg" height="30"></a>
</p>

## What it does

The listing narrows by query string: `/?city=krakow` returns the two flats in Kraków. The filter
control is the only thing that runs in the browser; the list it filters is rendered on the server
and swapped in, so the page keeps working with JavaScript disabled.

Both languages are routes, not a runtime lookup: `/` is English and `/pl` is Polish, declared in
`gopage.jsonc` under `i18n` with `prefixDefault` off so the default language keeps the bare path.

## Where to look

| file | what it shows |
| --- | --- |
| `app/page.gopage` | the listing: a cached loader, an island, and a deferred fragment in one page |
| `app/items/[id]/page.gopage` | a dynamic route whose loader tags what it cached |
| `app/api/feed/route.go` | server-sent events: the route pushes each item as it goes |
| `components/Filter/` | the island: `client.ts` beside `props.go`, so the browser gets types the server wrote |
| `components/Badge/`, `components/ItemCard/` | components with no browser code at all |
| `locales/en.json`, `locales/pl.json` | the message catalogs the compiler checks against the templates |
| `catalog/items.go` | the data, embedded; there is no database here |

## Running it

```bash
pnpm install && pnpm dev
```

The island needs its browser packages once. With the CLI already on your PATH, `gopage dev` does
the same thing.

## Deploying it

```bash
gopage build --target workers && wrangler deploy
```

```bash
gopage build --target native && ./dist/server
```

---

Generated from the `catalog` template. Change the template rather than this folder; see
[CONTRIBUTING](../../CONTRIBUTING.md).
