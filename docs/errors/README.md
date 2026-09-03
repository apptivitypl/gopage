# rill diagnostic codes

One page per code. The file name is the code: `C305.md`, `W703.md`. `rilltool diag` refuses to pass
unless every code in the registry has a page here **and** a test that produces it, so this list
cannot drift from what the compiler actually reports.

| Range | Area |
|---|---|
| C0xx | template syntax |
| C1xx | routes, links, route handlers |
| C2xx | expressions |
| C3xx | props, components, islands, images, filters, fragments |
| C5xx | cache and what may enter it |
| C6xx | catalogs and plurals |
| Wxxx | warnings |

Every page says what the message looks like, why the compiler refuses it rather than letting it
through, and how to fix it — including the escape hatch where one exists.

| Code | Title |
|---|---|
| [C001](C001.md) | unterminated frontmatter fence |
| [C002](C002.md) | unterminated interpolation |
| [C004](C004.md) | unknown directive |
| [C005](C005.md) | unterminated directive |
| [C006](C006.md) | unbalanced block directive |
| [C101](C101.md) | conflicting routes |
| [C102](C102.md) | page under the api namespace |
| [C103](C103.md) | outlet outside a layout |
| [C104](C104.md) | route handler without a method |
| [C105](C105.md) | malformed route handler |
| [C111](C111.md) | link points at no route |
| [C201](C201.md) | malformed expression |
| [C202](C202.md) | unclosed group |
| [C301](C301.md) | unreadable go block |
| [C302](C302.md) | unsupported props type |
| [C303](C303.md) | unexported props field |
| [C304](C304.md) | unknown struct in props |
| [C305](C305.md) | unknown field in template |
| [C306](C306.md) | malformed component tag |
| [C307](C307.md) | unknown component |
| [C308](C308.md) | component argument mismatch |
| [C309](C309.md) | match is not exhaustive |
| [C310](C310.md) | malformed element attribute |
| [C311](C311.md) | malformed built-in component |
| [C312](C312.md) | malformed submit handler |
| [C313](C313.md) | unknown filter |
| [C314](C314.md) | malformed fragment |
| [C315](C315.md) | malformed island |
| [C316](C316.md) | malformed image |
| [C317](C317.md) | malformed client script |
| [C318](C318.md) | malformed deferred fragment |
| [C319](C319.md) | malformed fragment placeholder |
| [C320](C320.md) | private value on an island |
| [C321](C321.md) | value interpolated into a script |
| [C322](C322.md) | value interpolated into css |
| [C323](C323.md) | value interpolated into an event handler |
| [C324](C324.md) | value interpolated into srcdoc |
| [C503](C503.md) | private value in a cached fragment |
| [C601](C601.md) | missing translation |
| [C602](C602.md) | plural form mismatch |
| [C603](C603.md) | catalog in the old toml format |
| [W703](W703.md) | class built at request time |
