# Analytics localization: local review notes

This is an **unpublished local prototype**, not an approved upstream contribution.
The maintainer already announced the other Settings pages, including Analytics,
in [PR #135](https://github.com/X4Applegate/caddyui/pull/135#issuecomment-6098497216).
Coordinate that overlap before proposing template changes. The existing French
catalog-only PR should remain separate.

## Goal and scope

Make Analytics labels translatable with the existing catalog mechanism instead
of adding a separate translation engine. Translators can then update JSON rather
than editing application code for every language.

- Settings → Analytics: ingest, exclusions, retention, storage and pipeline labels.
- Analytics overview and host, path, visitor and response-class drilldowns.
- English and French catalogs; other languages retain the normal English fallback.
- Visitor-block confirmation text uses the existing client-side translator.
- No form field names, routes, query values, permissions or Caddy payloads changed.

The catalogs now have 505 keys each, including 191 new keys. The earlier French
review corrections are preserved. This does **not** mean the whole application
is translated: other forms, server diagnostics and relative timestamps remain
outside this batch.

## Findings and real corrections

1. The Settings help text said “Retained 30 days” even when retention differed.
   It now uses the configured number of days and handles zero as indefinite.
2. Display-label comparisons in window selectors coupled their selected state
   to English wording. They now use the existing stable `WindowQuery` values.
3. A first test incorrectly expected an empty-chart message. The handler uses
   `fillBuckets`, which returns a dense zero-filled timeline even without events.
   The corrected test asserts that actual chart and the empty visitors table.
4. An intermediate extraction reused short labels across different grammatical
   contexts. The host summary now uses a complete parameterized sentence; card
   and pruning labels remain distinct.

## Validation recorded on 20 October 2026

Code revision: `2b78413` (the subsequent documentation commit does not change it).
Go 1.27.2, Linux, isolated environment. Commands:

```sh
go test -p 1 ./internal/i18n/ ./internal/server/ -run 'Translation|Catalog|Language|Locale|Analytics' -count=1
go test -p 1 ./...
```

Both passed. The full server package took 117.513 seconds in this constrained
test environment; this is a test-suite duration, not an application benchmark.
The new tests cover the six real routes, locale separation, English fallback,
window selection, escaped request data, markup-free catalog values and retention
of seven days or indefinitely in both languages.

The server package cannot currently compile on Windows because upstream storage
code uses Linux `syscall.Statfs` types. That unrelated portability issue was not
changed to make this localization prototype work.

Binary compilation and browser acceptance are recorded separately by the local
deployment workflow. Do not infer them solely from passing Go tests.

## Before any upstream proposal

Agree on the next batch with the maintainer, refresh against their current branch,
keep compatible keys, and remove duplicate work. Retain genuine test evidence and
review the French in the running UI. A clean contribution can select or squash
the useful local commits after review; no history dates need to be rewritten.

Prepared with AI assistance and local review. No GitHub publication performed.
