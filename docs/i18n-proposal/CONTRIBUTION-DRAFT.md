# Draft discussion message — not submitted

## Extending interface localization and French coverage

Hello, and thank you for CaddyUI.

I'm French and English is not my strongest language. I use language-assistance
tools when writing in English, so please let me know if any wording is unclear.
AI assistance was also used during development and language review. The code,
tests and limitations are included for review rather than presented as a claim
of complete production validation.

While testing the French interface, I found that many forms, Settings sections
and browser messages still contain hardcoded English. A catalog-only change
cannot cover those strings. I prepared a local prototype that keeps the existing
JSON catalogs and localization helpers, extracts the missing text, and makes
some fragmented messages easier to translate correctly.

The second language review was important. It caught real meaning errors, such
as confusing a health probe's request body with an expected response body, as
well as agreement issues around certificate and DNS explanations. I added
regression coverage for titles, states, retention and form submission contracts.

I know that the remaining Settings pages are already planned on your side, and
that translation PRs are expected to remain data-only. I do not want to duplicate
that work or submit a large overlapping change without agreement. Could you
advise which implementation pieces would be useful and how you would prefer to
split them from the French catalog update?

The local dossier includes the exact tested source versions, automated results,
screenshots, and areas that have not been tested. No production configuration
was changed during the localization tests. Please treat this as a proposal for
discussion rather than a request to merge the entire prototype as-is.
