# Local validation record

## Versions and environment

| Item | Recorded value |
|---|---|
| Upstream baseline | `c43126a4a7dd4e934bb3df0245a3b88fefa2a49e` |
| Upstream HEAD check | Same commit, checked on 20 October 2026 |
| Reviewed implementation | `72692fc` on `interface-localization` |
| Fork review branch | `interface-localization-review`; implementation/tests then documentation |
| Go module requirement | Go `1.26.0`, toolchain `go1.27.2`, unchanged |
| Actual Go runtime | Go `1.27.2`, Linux amd64 |
| Build/test OS | Debian GNU/Linux 13, installed release 13.6 |
| Resource cap | One CPU, 1 GiB container RAM; build/test job capped at 850 MiB and 80% CPU |
| Connected Caddy | `v2.11.7`, accessed through a source-restricted read-only relay |
| Browser review | Codex in-app browser on Windows |
| Optional JS syntax checker | Node.js `v24.19.0` |

The deployed demo identifies itself as `v2.65.1-i18n-review-72692fc`. Its Linux
binary SHA-256 is
`b0ae6ba23c1df6aaabbef9c5d14e17097d4e5eb465f60263dfe9f6d13e623351`.

The distribution/proposal commit can include evidence and documentation added
after the reviewed source snapshot. Compare `cmd`, `internal`, `web`, `go.mod`
and `go.sum` to establish source identity; a new commit hash alone does not mean
that another runtime implementation was tested.

The fork's implementation commit is `523d9e5`. Its `cmd`, `internal`, `web`,
`tools`, `go.mod` and `go.sum` trees were compared with `72692fc` using
`git diff --exit-code`; there were no differences. Reorganizing commits did not
rerun or invent tests. This document records the actual runs on that source.

## Automated checks

Run the project checks with the Go toolchain selected by `go.mod`:

```sh
go test -p 1 ./...
go build -p 1 ./...
go run ./cmd/i18n-catalog -lang fr -require-complete
node --check web/static/app.js
```

The `-p 1` cap is specific to the small test machine, not a requirement imposed
on contributors. No external linter or Node-based application build is required.

Focused tests exercise:

- English fallback, locale precedence and catalog key/placeholder contracts.
- English and French retention descriptions for zero, one and several days.
- French form title agreement and status adjectives rather than activity verbs.
- The request-body label of the active health probe.
- Nine forms in English and French, comparing submitted field names, values,
  methods and actions while excluding session-specific CSRF values.
- Rendered Analytics routes, filtering, empty states and escaped request data.
- Conservative extraction, protected configuration examples and malformed input.

The baseline and candidate are tested from separate source directories against
the same toolchain and resource limits. The baseline test/build log ends with
`UPSTREAM_C43126A_PASS`; the candidate log ends with `CANDIDATE_72692FC_PASS`.
The final local operations record contains their actual completion results and
durations. Do not infer a pass merely from these marker names being documented.

### Actual review iterations

The first full candidate run (`dd636d7`) failed two Analytics assertions after
the French terminology changed from `flotte` to `parc`. The next run (`df697eb`)
still expected `tout le parc` where the rendered scope sentence uses
`l'ensemble du parc`. Those are genuine failed checks, not fabricated development
steps. The assertions were corrected against the actual template/catalog; no
product behavior was changed to hide a failure. The reviewed candidate is
`a22d9f6`, which passed the full suite and build. Browser review then corrected
one remaining French phrase (`le Public` to `l’indicateur Public`) in `72692fc`.
The isolated demo was not replaced by either failed candidate.

### Completed results — 20 October 2026

| Source/check | Actual result |
|---|---|
| Official baseline `c43126a`, complete Go suite | PASS; server package 102.354 s |
| Official baseline `c43126a`, `go build -p 1 ./...` | PASS |
| Reviewed candidate `a22d9f6`, complete suite and build | PASS; server package 112.925 s |
| Final reviewed candidate `72692fc`, targeted locale/form/Analytics tests | PASS; 3.280 s |
| Final reviewed candidate `72692fc`, complete Go suite | PASS; server 111.398 s, web 0.385 s |
| Final reviewed candidate `72692fc`, complete build and versioned Linux binary | PASS |
| Strict French catalog check | PASS; 4,691/4,691 entries |
| Static browser JS and optional checker syntax | PASS, `node --check` |
| Rendered English pages | 16 HTTP 200; 105 scripts, 186 handlers, zero syntax errors |
| Rendered French pages | 16 HTTP 200; 105 scripts, 186 handlers, zero syntax errors |
| Browser language switch | French → English → French; selected locale and translated toast verified |
| Guided publication | French step 1 → step 2; no creation submitted |
| Advanced form search | `HSTS` → `3 correspondances`; security section opens |
| Certificate/help review | Correct request/key/mount terminology and assembled sentences inspected |
| Production preservation | Caddy config SHA unchanged; same Caddy/Polder/X4 processes; relay GET 200 / dummy POST 405 |

Package times above are Go's reported durations, not an end-to-end benchmark.
Some unchanged packages used Go's test cache. This compares source snapshots on
one Linux toolchain, with browser review in one browser; it is not a matrix of
multiple Go releases, operating systems, Caddy versions or mobile browsers.

## Optional check of rendered JavaScript

Save authenticated HTML snapshots into a temporary directory outside Git, then:

```sh
node tools/check-rendered-js.mjs /path/to/rendered-html
```

This compiles classic inline scripts and decoded DOM event handlers without
executing them. Script bodies are excluded when looking for HTML event handlers,
so strings that will later generate markup are not mistaken for current DOM
handlers. It reports syntax errors and returns a nonzero status on failure.
It does not execute interactions, fetch assets or validate JavaScript modules.

The checker itself was exercised with two synthetic fixtures: valid JavaScript
and an entity-encoded handler passed, while a malformed handler returned exit
status 1. A string containing generated HTML inside a script was correctly
excluded from DOM-handler scanning.

Snapshots can contain session tokens and environment details. They are not
documentation assets and must not be committed; only the summarized result and
reviewed screenshots belong in this dossier.

## Browser review and limits

Check both English and French, restoring French after the comparison captures.
Review the guided publication flow, advanced field filtering, Settings,
certificates and assembled help text. Do not save a proxy or trigger a real DNS,
ACME, certificate-export or Caddy configuration change for a language test.

The isolated instance retains synthetic Analytics events and a snapshot of
configuration obtained through the read-only relay. GET is allowed; mutation
requests are rejected. This proves neither real DNS-provider compatibility nor
certificate issuance, writable Caddy deployment, browser/device coverage or the
correctness of provider policy statements inherited from upstream documentation.

English source text and translated technical prose remain subject to maintainer
and native-language review. Catalog completeness alone is not proof of a correct
sentence, complete extraction coverage, or production readiness.

Protected code examples retain technical text. Some code-styled references to
English UI labels in the embedded help (for example `Domains`) still need a
manual distinction between a field label and executable configuration before
broadening the conservative extractor. They are not covered by the 4,691-key
completeness claim.
