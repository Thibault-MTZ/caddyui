# Draft: i18n(fr): translate Security settings

Add French text for the Security settings keys introduced by the separate
extraction contribution. This branch depends on `i18n-settings-security`; its
own diff changes only `web/i18n/fr.json`.

The translation covers CAPTCHA configuration, trusted proxies, access lists,
certificate directories, OIDC, sessions and TOTP policy. Technical identifiers
and examples remain unchanged. French wording was reviewed in context, including
the distinction between a public site key and a server-side secret.

Before opening: update this branch onto the merged extraction, attach the actual
validation record, and request language review. Do not present local use or
automated rendering checks as exhaustive validation of authentication workflows.
