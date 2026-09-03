# Security

## Reporting

Report privately through
[a security advisory](https://github.com/apptivitypl/rill/security/advisories/new). Do not open a
public issue for a vulnerability.

Say what you did, what happened, and what you expected. A `.rill` file or a project that shows the
problem is worth more than a description of it.

Expect an acknowledgement within a few days. Until 1.0 there are no supported branches: fixes land
on `main`, and the fix is the release.

## What counts as a vulnerability here

rill compiles templates and serves the result, so the interesting failures are the ones where data
crosses a boundary it should not.

- **Escaping.** Anything that gets attacker-controlled text into rendered HTML without escaping, or
  into an attribute, URL or `<script>` context in a way the escaper did not account for. The
  compiler refuses an interpolation in the four contexts HTML escaping cannot hold — a `<script>`
  or `<style>` body, an `on*` handler, `srcdoc` — and filters the scheme of a URL attribute, so a
  value that reaches any of them anyway is a bug in that machinery.
- **Cache boundaries.** A response cached under one key that is served for another: a per-user
  value reaching a shared fragment, a locale bleeding across hosts, a cached page answering a
  request whose loader would have returned something else. The `I2` invariant test exists for
  exactly this class.
- **Path traversal.** A request that reads outside `public/`, or a build that writes outside the
  project directory.
- **Forms.** A submission accepted without its CSRF token, or a token that is valid across
  origins or across users. State-changing requests are also held to their origin through
  `Sec-Fetch-Site`, so a cross-site write that gets through is one of these too.
- **The compiler as an attack surface.** A `.rill` file that makes the compiler write outside the
  project, execute something, or loop forever. Templates are trusted input in most projects, so
  this is lower severity — but it is still a bug worth reporting.
- **Generated projects.** A default in a scaffolded project that is unsafe in production, such as
  a header, a cache directive, or a trusted-proxy setting that takes a header at face value.

## What does not count

- A denial of service from a request that is expensive by construction, such as a loader you wrote
  that is slow. rill bounds its cache, not your code.
- Anything that needs write access to the project's own source. A template can already run Go.
- Vulnerabilities in a dependency that rill does not reach. `govulncheck` runs daily and reports
  what is actually called; a finding in an unreached path is not one of ours to fix, though a
  report of it is still welcome.
- Missing hardening headers on the dev server. `rill dev` is not a production server and does not
  pretend to be.

## What ships with a release

Every release archive carries a checksum, and `checksums.txt` is signed with cosign against the
release workflow's own OIDC identity — there is no private key to steal. Each archive has an SBOM,
and the archives, SBOMs and checksums all carry a build provenance attestation. The install script
verifies the checksum always, and the signature when `cosign` is present; a signature that does not
verify stops the install. `--require-signature` turns an absent signature, or an absent `cosign`,
into a refusal too.
