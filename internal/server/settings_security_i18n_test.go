// SPDX-License-Identifier: Apache-2.0

package server

import (
	"html"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/X4Applegate/caddyui/internal/models"
)

// A partial catalog must retain usable security controls through English
// fallback. Changing display language must never translate submitted values.
func TestI18nSecuritySettingsFallbackAndControls(t *testing.T) {
	e := newSecEnv(t)
	for _, lang := range []string{"en", "fr", "ru", "zh-CN"} {
		t.Run(lang, func(t *testing.T) {
			e.do(t, "admin", http.MethodPost, "/profile", url.Values{"action": {"update_locale"}, "locale": {lang}})
			rec := e.do(t, "admin", http.MethodGet, "/settings/security", nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("security settings: HTTP %d", rec.Code)
			}
			body := rec.Body.String()
			for _, key := range []string{"settings.security.captcha.title", "settings.security.allowlist.label", "settings.security.oidc.label", "settings.security.totp.require"} {
				want := html.EscapeString(translations().T(lang, key))
				if !strings.Contains(body, want) {
					t.Errorf("missing localized security text %q", want)
				}
			}
			for _, contract := range []string{`name="captcha_provider" value="off"`, `name="captcha_provider" value="turnstile"`, `name="captcha_provider" value="recaptcha"`, `data-captcha-panel="turnstile"`, `data-captcha-panel="recaptcha"`, `name="admin_allowlist"`, `name="global_ip_blocklist"`, `name="oidc_default_role"`, `value="view"`, `value="user"`, `value="admin"`, `name="session_duration_days"`, `name="recaptcha_min_score"`, `CADDYUI_ADMIN_ALLOWLIST=off`, `group|role`} {
				if !strings.Contains(body, contract) {
					t.Errorf("security form contract lost: %s", contract)
				}
			}
			if strings.Contains(body, ">settings.security.") || strings.Contains(body, `title="settings.security.`) {
				t.Error("raw translation key rendered")
			}
			if lang == "ru" || lang == "zh-CN" {
				if !strings.Contains(body, "CAPTCHA protection") || !strings.Contains(body, "Security policy") {
					t.Error("partial catalog did not fall back to English")
				}
			}
		})
	}
}

func TestI18nSecuritySettingsEscapesStoredValues(t *testing.T) {
	e := newSecEnv(t)
	const unsafe = `<img src=x onerror=alert(1)>`
	if err := models.SetSetting(e.db, "oidc_issuer", unsafe); err != nil {
		t.Fatal(err)
	}
	body := e.do(t, "admin", http.MethodGet, "/settings/security", nil).Body.String()
	if strings.Contains(body, unsafe) || !strings.Contains(body, html.EscapeString(unsafe)) {
		t.Error("stored issuer must remain escaped in the translated form")
	}
}
