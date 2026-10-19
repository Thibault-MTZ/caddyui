# Translating CaddyUI

CaddyUI's interface can be shown in other languages (since v2.62.0, issue #128). English is the source; every other language is a JSON file of translations. **Adding or updating a language is a data-only change — no Go, template or JavaScript edits.**

## Available languages

| Code | Language | Maintainer |
|---|---|---|
| `en` | English (source) | — |
| `fr` | Français (French) | Local catalog based on the reviewed French translation in PR #144 |
| `zh-CN` | 简体中文 (Simplified Chinese) | @chongfenglaosiji (since v2.62.1) |

## The catalog files

Catalogs live in [`web/i18n/`](../web/i18n/), one file per language, named by its [BCP 47](https://www.rfc-editor.org/info/bcp47) code:

```
web/i18n/en.json      ← English: the source of truth
web/i18n/zh-CN.json   ← Simplified Chinese
web/i18n/de.json      ← German
```

Each file is a flat JSON object of **key → text**:

```json
{
  "_meta.name": "简体中文",
  "nav.proxy_hosts": "代理主机",
  "page.proxy_hosts.heading": "代理主机",
  "profile.language.saved": "语言已保存。"
}
```

- **`_meta.name`** is the language's own name, shown in the language picker.
- **Keys** are stable dotted names. Copy them exactly from `en.json`; never invent new ones in a translation — the build test rejects keys English does not have.
- **Missing keys fall back to English**, so a catalog can be partial. An empty string also counts as missing. Translate what you can; the rest stays English until it is done.
- **Placeholders** look like `{count}` or `{name}`. Keep every placeholder English uses, spelled the same (you may move them within the sentence). The build test checks this.
- Keys starting with `js.` are also available to the browser-side scripts.
- Text is plain text — no HTML. It is escaped when rendered.

## Choosing the language

- Each person can pick a language under **Profile → Language** — that always wins.
- Otherwise, if an administrator set **Settings → General → Default language**, that applies.
- Otherwise the browser's `Accept-Language` decides (`zh-TW` or `zh` will use `zh-CN` if that is the closest available), otherwise English.

## Checking your work

```bash
go test ./internal/i18n/ ./internal/server/ -run 'Translation|Catalog|Language|Locale'
```

This loads every catalog and fails if a file is not valid JSON, uses a key English lacks, or changes a placeholder. To see it, run CaddyUI, pick the language on your profile and browse.

## What is translated so far

The plumbing landed in v2.62.0 with English for the navigation, the top bar and its menus, the user menu, the list-page titles, the profile language picker and the default-language setting.

v2.63.0 added about 250 keys:

| Prefix | What it covers |
|---|---|
| `dashboard.*` | the whole Dashboard page |
| `js.dashboard.*`, `js.health.*` | texts the Dashboard's script writes (health-dot tooltips, update notice) |
| `profile.*`, `profile.msg.*` | the whole Profile page and its saved/error messages |
| `settings.section.*` | the Settings navigation: each section's name and description |
| `settings.*` (others) | the Settings heading and save buttons, and the whole **General** page |
| `status.*` | shared status words — Online, Offline, Enabled, Disabled, … |

More of the interface is moved onto keys release by release — the other Settings pages, then the list pages and forms, the large proxy-host form last. When new keys appear in `en.json`, a translation simply shows English for them until it is updated.

## Local Analytics prototype (unpublished)

This checkout explores the next Analytics translation batch locally. The maintainer
has already announced work on the other Settings pages, including Analytics, in
PR #135; coordinate that overlap before proposing an upstream contribution.
This prototype does not imply upstream agreement or publication.

`analytics.*` covers the Analytics overview, host, visitor, path and status pages,
and Settings → Analytics (including storage and retention controls). English and
French are supplied; other catalogs fall back to English. The visitor block
confirmation uses `js.analytics.block_confirm`. Window selectors use the stable
`WindowQuery` value rather than translated display labels. Markup, request data,
form actions, CSV exports and filtering remain in the templates.

Technical values and server diagnostics (including relative timestamps, ingest
target warnings, storage errors and reasons a reclaim is blocked) retain their
existing server-provided formatting. They are outside this template batch.

Regression coverage:

```bash
go test ./internal/i18n/ ./internal/server/ -run 'Translation|Catalog|Language|Locale|Analytics'
go build ./...
```

The Analytics tests render all six routes in French, check empty states, filter
selection, escaped request data and locale separation, and keep catalog text free
of HTML. The initial French catalog and the subsequent template extraction are
separate commits so that the translation-only contribution remains reviewable.

## Local interface extraction (unpublished)

The `interface-localization` development branch extends the same JSON mechanism
across the templates, browser messages and selected server messages. It does not
add machine translation, gettext, a second runtime or an external service.
English remains authoritative; French is supplied separately for every current
catalog entry, including the embedded help. Catalog completeness is validated,
but fragmented rich-text sentences still need visual and grammatical review.
Extraction coverage and translation coverage are different measurements: a
complete catalog does not imply that external diagnostics or every future UI
message is translated.

To create or update the translation work file without losing existing work:

```sh
go run ./cmd/i18n-catalog -lang fr -sync
go run ./cmd/i18n-catalog -lang fr
# Only require completeness when the language is actually finished:
go run ./cmd/i18n-catalog -lang fr -require-complete
```

Synchronization adds empty values for missing English keys. Empty values fall
back to English. It never invents translated prose or overwrites filled values.
Named placeholders and server printf directives are validated before deployment.

The reviewed template extraction workflow is documented in
[`cmd/i18n/README.md`](../cmd/i18n/README.md). `cmd/i18n-messages` separately audits
recognized Go message contexts. These conservative scanners are review aids;
zero automatically extractable literals does not prove that every dynamic
message is localized. Technical examples, user data, external service errors and
protocol identifiers must not be translated as interface prose.

Use complete phrases with named variables for new text. Keep HTML in templates,
escape translated text at its insertion context, and never use translated labels
as selectors, form values or API identifiers. Existing rich-text fragments still
need grammatical review before an upstream proposal. Relative-time displays and
dashboard recommendations now select the request's locale rather than changing
global process state.

The intended upstream split is code/tooling plus English first, then a French
catalog-only update. Both need coordination with the maintainer's ongoing i18n
work; neither is published by this local branch.

## Sending a translation

Open a pull request that only adds or edits `web/i18n/<code>.json`. If you maintain a language, mention it in the PR so you can be pinged when new keys appear.
