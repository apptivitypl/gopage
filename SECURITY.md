# Security

## Reporting

Report privately through
[a security advisory](https://github.com/apptivitypl/gopage/security/advisories/new). Do not open a
public issue for a vulnerability.

Say what you did, what happened, and what you expected. A `.gopage` file or a project that shows the
problem is worth more than a description of it.

Expect an acknowledgement within a few days. Until 1.0 there are no supported branches: fixes land
on `main`, and the fix is the release.

## What counts as a vulnerability here

gopage compiles templates and serves the result, so the interesting failures are the ones where data
crosses a boundary it should not.

- **Escaping.** Anything that gets attacker-controlled text into rendered HTML without escaping, or
  into an attribute, URL or `<script>` context in a way the escaper did not account for. The
  compiler refuses an interpolation in the four contexts HTML escaping cannot hold, which are a
  `<script>` or `<style>` body, an `on*` handler and `srcdoc`, and it filters the scheme of a URL
  attribute, so a value that reaches any of them anyway is a bug in that machinery.
- **Cache boundaries.** A response cached under one key that is served for another: a per-user
  value reaching a shared fragment, a locale bleeding across hosts, a cached page answering a
  request whose loader would have returned something else. The `I2` invariant test exists for
  exactly this class.
- **Path traversal.** A request that reads outside `public/`, or a build that writes outside the
  project directory.
- **Forms.** A submission accepted without its CSRF token, or a token that is valid across
  origins or across users. State-changing requests are also held to their origin through
  `Sec-Fetch-Site`, so a cross-site write that gets through is one of these too.
- **The compiler as an attack surface.** A `.gopage` file that makes the compiler write outside the
  project, execute something, or loop forever. Templates are trusted input in most projects, so
  this is lower severity, but it is still a bug worth reporting.
- **Generated projects.** A default in a scaffolded project that is unsafe in production, such as
  a header, a cache directive, or a trusted-proxy setting that takes a header at face value.

## What does not count

- A denial of service from a request that is expensive by construction, such as a loader you wrote
  that is slow. gopage bounds its cache, not your code.
- Anything that needs write access to the project's own source. A template can already run Go.
- Vulnerabilities in a dependency that gopage does not reach. `govulncheck` runs daily and reports
  what is actually called; a finding in an unreached path is not one of ours to fix, though a
  report of it is still welcome.
- Missing hardening headers on the dev server. `gopage dev` is not a production server and does not
  pretend to be.

## What ships with a release

Every release archive carries a checksum, and `checksums.txt` is signed with cosign against the
release workflow's own OIDC identity, so there is no private key to steal. Each archive has an SBOM,
and the archives, SBOMs and checksums all carry a build provenance attestation. The install script
verifies the checksum always, and the signature when `cosign` is present; a signature that does not
verify stops the install. `--require-signature` turns an absent signature, or an absent `cosign`,
into a refusal too.

## What the automation here can reach

Every action used by a workflow in this repository is pinned to a commit hash, and a zizmor audit
runs on each pull request to keep it that way.

There is one deliberate exception. The review bot in `.github/workflows/review.yml` uses the
opencode action, which is pinned like the rest but which fetches and runs an installer of its own
when the job starts. Pinning the action fixes the instruction, not the bytes it downloads, and the
zizmor audit cannot see inside a third-party action, so nothing here would report it. That is the
reason it is written down instead.

The bot was given the smallest surface that still lets it work. Its job holds `contents: read` and
`pull-requests: write` and nothing else, so the worst it can do is write a comment on the pull
request that invoked it. It cannot push, tag, publish, edit a workflow or reach a release. It reads
its instructions from the default branch rather than from the branch under review, and project
configuration is disabled in its environment, so a pull request cannot rewrite what it was told to
do. On a pull request from a fork it does not run at all until someone with write access asks for
it, because a fork's diff is text written by a stranger and the model reads it.

Setting the repository variable `REVIEW_BOT` to `off` stops it without a commit.
