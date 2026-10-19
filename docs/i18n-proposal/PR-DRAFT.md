# Draft only — discuss scope before opening a PR

## What & why

Extend the existing JSON localization mechanism so hardcoded interface prose can
be translated, and expand the reviewed French catalog. Keep technical identifiers,
submitted values and configuration examples unchanged. See [the dossier](README.md)
for implementation details and screenshots.

## Related issue

Historical context: #128; existing French catalog contribution: #144.
Neither reference is a claim of approval for this broader implementation.
Discuss overlap with the planned Settings work before submission.

## Type of change

- [x] Bug fix
- [x] Documentation
- [x] Refactor / internal
- [x] Build / CI / tooling

## Checklist

- [ ] Attach the completed build/test results from [VALIDATION.md](VALIDATION.md).
- [ ] Confirm the final agreed scope and split into focused implementation/catalog PRs.
- [ ] Confirm the clean proposal commit and existing code style.
- [x] Add an Unreleased changelog entry and translation documentation.
- [ ] Agree the release version bump with the maintainer; no official release number invented locally.
- [ ] Contributor to confirm agreement to the project's Apache License 2.0 before submission.

## Notes for reviewers

I'm French and English is not my strongest language. I use language-assistance
tools when writing in English. AI tools also assisted development and review.
Please point out unclear wording. Exact versions, actual checks and untested
areas are documented in the validation record. The private demo is read-only
towards production Caddy. Screenshots must be recaptured with public fixture
identifiers before any public submission.
