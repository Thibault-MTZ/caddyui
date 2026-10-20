# Draft: i18n: make Security settings translatable

Security settings still contain hardcoded English labels and help text. This
extracts that section into the existing English catalog so other languages can
translate it without changing templates again.

This is the first focused batch discussed in #144. It uses stable
`settings.security.*` keys and reuses existing shared keys. Submitted field names,
provider and role values, technical examples and the CAPTCHA switching script
stay unchanged. No new translation framework or runtime dependency is added.

French translations are prepared on a separate branch. Russian and Chinese
remain valid partial catalogs and fall back to English for the new keys.

Baseline: upstream `e96a090`, after v2.66.1. Targeted localization and settings
tests and `go build -p 1 ./...` passed on Go1.27.2/Linux. The added regression
tests cover partial-catalog fallback, form values and stored-value escaping.

Please review and test before merging. I have a broader local development
version, but not every workflow has been tested. Live external OIDC providers,
CAPTCHA verification and actual security-policy changes have not been exercised
by this batch. Rich-text help still has separate translated spans around its
existing markup; feedback on that shape is welcome before applying it elsewhere.

AI assistance was used for preparation; the diff and test results were reviewed.
This file is a preparation note on my personal branch, not an opened PR.
