# Interface localization: fork review proposal

This proposal extends CaddyUI's existing localization mechanism across the
interface and supplies a reviewed French catalog. It is discussion and test
material prepared for the contributor's personal fork, not submitted upstream.

The original problem is visible in the contributor's installation: navigation
can be French while forms, settings explanations and browser-generated messages
remain English. Adding entries to `fr.json` cannot translate text that is still
hardcoded in templates, JavaScript or request handlers.

## Approach

- Retain the existing Go `t` helper, browser `caddyuiT` helper and flat JSON
  catalogs. English remains authoritative. No gettext migration, translation
  service, runtime dependency or automatic prose translation is introduced.
- Move interface prose onto keys while retaining technical identifiers, form
  field names, submitted values, URLs and configuration examples.
- Use complete messages with named variables where grammar or word order needs
  flexibility. Do not assemble plurals by appending English suffixes.
- Keep catalog values plain text. Go templates escape values contextually;
  browser-generated HTML escapes translated text before insertion.
- Synchronize missing keys without overwriting translations, and validate named
  variables and printf formats. Partial community catalogs retain English fallback.
- Keep translated labels out of selectors and application identifiers.

For example, a DNS explanation previously assembled several fragments and a
literal English suffix. It now uses one message:

```js
detail.textContent = caddyuiT('js.proxy_host_form.dns_provider_requirements', {
  provider: providerLabel,
  plugin: providerID
});
```

```json
{
  "js.proxy_host_form.dns_provider_requirements": "Utilisera DNS-01 avec {provider} et les identifiants déjà définis dans Paramètres → Fournisseurs DNS. Caddy doit intégrer le module caddy-dns/{plugin} correspondant."
}
```

Form titles are complete messages as well. Translating `New` and `Proxy host`
independently produced the wrong French agreement; a dedicated title can say
`Nouvel hôte proxy` without changing the English title or the route.

## What the second language review corrected

The initial extraction made text translatable, but did not prove that its meaning
or assembled grammar was correct. Contextual review found and corrected:

- A health probe's outgoing request body described as an expected response body.
- Passive health thresholds described as failed-request counts rather than
  concurrent-request or latency thresholds.
- Activity verbs reused as status adjectives, such as `a désactivé` in a form.
- Broken sentences around TLS issuers, certificate file paths, DNS credentials,
  network bridge names and per-user TOTP settings.
- Shared fragments that needed separate complete messages for Content-Type
  prefixes and query-string pairs.
- An untranslated placeholder explanation and a literal `plugin.` suffix.
- Singular retention of one day and count labels that should not depend on
  English plural suffixes.

These are language and presentation changes. Provider policies or technical
advice inherited from the English documentation have not been independently
re-audited as part of this translation proposal.

## Screenshots

Seven genuine captures were recorded from the isolated test instance. They show
the reviewed build and its synthetic data. The private image files are kept
locally and excluded from this branch's published history.

| Capture | What it demonstrates |
|---|---|
| Guided publication | French workflow, titles and form states |
| Advanced search | Functional search, stable selectors and French count message |
| Settings | Explanations, controls and relative timestamps |
| Certificate form | Certificate terminology and private-key explanation |
| Embedded help | Assembled rich-text documentation |
| English comparison | Same build with English selected; not an upstream-before screenshot |
| Next guided step | The French wizard actually advances without saving a proxy |

Public screenshots remain pending: recapture the views with public demo
identifiers before adding images to the fork or an upstream proposal.

## Validation and reproducibility

The reviewed catalogs contain 4,691 English entries and 4,691 French entries.
The second review corrected 137 existing French values and introduced five
complete messages.

See [the validation record](VALIDATION.md) for exact source versions, environment,
automated checks, browser checks and untested areas. See
[`TRANSLATING.md`](../TRANSLATING.md) for the catalog workflow and
[`cmd/i18n/README.md`](../../cmd/i18n/README.md) for extraction limitations. The [discussion draft](CONTRIBUTION-DRAFT.md) and
[PR template draft](PR-DRAFT.md) are local preparation, not published messages.

## Contribution scope

The [contribution guide](../../CONTRIBUTING.md) asks for discussion before a
substantial change, focused PRs, a successful `go build ./...`, and clean commits.
The local work history is preserved separately. The fork branch
`interface-localization-review` contains two clean commits: implementation/tests,
then documentation. Dates and recorded failed checks reflect the actual work.

Existing work was checked on 20 October 2026:

- [Issue #128](https://github.com/X4Applegate/caddyui/issues/128) is the historical,
  now closed i18n request.
- [French PR #144](https://github.com/X4Applegate/caddyui/pull/144) remains a
  separate catalog-only contribution and is not modified by this proposal.
- The maintainer has [already announced work on the remaining Settings pages](https://github.com/X4Applegate/caddyui/pull/135#issuecomment-6098497216).

Discuss overlap and split the implementation into the maintainer's preferred
review batches before opening another PR. A code/English batch and a French
catalog batch are different concerns; the existing language-only instructions
must not be treated as permission to submit all infrastructure changes blindly.

The [draft contributor message](CONTRIBUTION-DRAFT.md) is for human review only.
