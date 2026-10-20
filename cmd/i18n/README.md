# Experimental template audit — separate proposal

This branch isolates the existing personal-fork tool for review. It does not
change application templates or catalogs and is separate from the Security /
login translation contribution. Audit is the default, read-only mode; use that
mode when checking untranslated text. The optional extraction mode still needs
manual review and must not be run blindly over the whole application.


This local development tool inventories the HTML templates without executing Go
template actions or JavaScript. It uses the existing flat JSON catalogs and the
existing `t $` / `caddyuiT` helpers. It does not translate text or modify catalogs.

```sh
go test ./cmd/i18n
go run ./cmd/i18n -mode audit -report /tmp/template-inventory.json
go run ./cmd/i18n -mode extract -entries /tmp/template-entries.json -report /tmp/template-extraction.json
go run ./cmd/i18n -mode check -report /tmp/template-remaining.json
```

`audit` only reads sources. `extract` first scans every template, writes proposed
English entries to the separate file, then changes template source spans. Existing
entries files are merged; conflicting values abort. Review both the Git diff and
the inventory before merging proposed entries into `web/i18n/en.json` and supplying
translations. Until that merge, new keys deliberately fail the catalog checks.
`check` exits with status 1 when extractable or manual-review literals remain.

Keys have a stable file prefix, short text slug and content hash. Repeated text in
the same context shares a key; punctuation differences cannot collide silently.
Extraction is deterministic and idempotent, with no HTML markup in catalog values.
Literal whitespace is retained. HTML entities become their plain-text characters
in JSON and are contextually escaped by `html/template` on render; equivalent
entity spellings may therefore be normalized.

Protected content includes Go actions/comments, HTML comments, code/pre/textarea/
kbd bodies, CSS, technical placeholders, field names/values, URLs and protocol
tokens. Text labels and title/placeholder/aria-label/alt attributes are eligible.
Markup stays in the template, so rich-text sentences can initially have several
text keys; human review should combine them into complete parameterized sentences
where grammar requires it.

JavaScript extraction is deliberately conservative: alerts/confirms/prompts and
literal textContent/innerText/title output use `caddyuiT`. Simple interpolated Go
fields become named placeholders with Go's JavaScript-context escaping. Regexes,
comparisons, CSS selectors, unknown literals and complex interpolation stay
unchanged and appear in the inventory for manual review. Complete HTML string
literals keep their markup in JavaScript and escape each translated text/label
attribute using `caddyuiEscapeHTML`; incomplete attribute fragments need review.
`-review cmd/i18n/template-review.json` enables the explicitly reviewed display
literals in helper arguments, status label functions and workflow definitions.
The JavaScript scanner is a source lexer, not a semantic JS parser; an inventory
finding does not automatically mean that a string is translatable. Extraction is
an initial reviewed batch, not a claim of complete interface coverage.
