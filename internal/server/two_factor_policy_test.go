// SPDX-License-Identifier: Apache-2.0

package server

import (
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	appdb "github.com/X4Applegate/caddyui/internal/db"
	"github.com/X4Applegate/caddyui/internal/models"
)

// v2.64.1 (issue #138): CaddyUI shipped two settings for one policy —
// "Require 2FA for all users" and "Require TOTP for all users" — with nearly
// identical descriptions. They are now one toggle. The risk in collapsing
// them is an install that had only one of the pair switched on quietly
// losing two-factor enforcement on upgrade, so that is what these pin down.

func twoFactorTestServer(t *testing.T) *Server {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "caddyui.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &Server{DB: conn}
}

func TestEitherStoredKeyStillRequiresTwoFactor(t *testing.T) {
	for name, stored := range map[string]map[string]string{
		"only the deprecated require_2fa (pre-upgrade install)": {settingRequire2FA: "1", settingRequireTOTP: "0"},
		"only the canonical require_totp":                       {settingRequire2FA: "0", settingRequireTOTP: "1"},
		"both, as saving now writes them":                       {settingRequire2FA: "1", settingRequireTOTP: "1"},
	} {
		t.Run(name, func(t *testing.T) {
			s := twoFactorTestServer(t)
			for k, v := range stored {
				if err := models.SetSetting(s.DB, k, v); err != nil {
					t.Fatal(err)
				}
			}
			if !s.twoFactorRequired() {
				t.Error("two-factor enforcement was dropped; an admin who had it on would silently lose it")
			}
		})
	}
}

func TestTwoFactorIsOffWhenNeitherKeyIsSet(t *testing.T) {
	s := twoFactorTestServer(t)
	for _, k := range []string{settingRequire2FA, settingRequireTOTP} {
		if err := models.SetSetting(s.DB, k, "0"); err != nil {
			t.Fatal(err)
		}
	}
	if s.twoFactorRequired() {
		t.Error("nothing is set, so the policy must be off")
	}
}

// Saving must write both keys, so downgrading to a release that reads only
// the deprecated key still enforces the policy.
func TestSavingThePolicyWritesBothKeys(t *testing.T) {
	for _, tc := range []struct {
		name, submitted, want string
	}{
		{"checked, value=1 convention", "1", "1"},
		{"checked, bare on convention", "on", "1"},
		{"unchecked", "", "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			form := url.Values{}
			if tc.submitted != "" {
				form.Set("require_totp", tc.submitted)
			}
			r := httptest.NewRequest("POST", "/settings/security", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if got := requireTOTPFormValue(r); got != tc.want {
				t.Errorf("requireTOTPFormValue = %q, want %q", got, tc.want)
			}
		})
	}
}

// The old require_2fa block let /api/ through and the require_totp block did
// not, so whether a JSON caller got a redirect instead of a response depended
// on which of the two identical-looking toggles the admin had used. The
// surviving check keeps the wider exclusion list.
func TestEnforcementExemptPathsAreTheWiderSet(t *testing.T) {
	// Mirrors the condition in requireAuth; kept as a table so a change to
	// the exclusions has to be deliberate.
	exempt := func(path string) bool {
		return strings.HasPrefix(path, "/totp/") || strings.HasPrefix(path, "/static/") ||
			strings.HasPrefix(path, "/api/") || path == "/logout"
	}
	for path, want := range map[string]bool{
		"/totp/setup":                   true, // enrolment page
		"/totp/regenerate-backup-codes": true, // and its siblings
		"/static/app.css":               true,
		"/api/v1/proxy-hosts":           true, // a redirect here would break JSON callers
		"/logout":                       true,
		"/":                             false,
		"/proxy-hosts":                  false,
		"/settings/security":            false,
	} {
		if got := exempt(path); got != want {
			t.Errorf("exempt(%q) = %v, want %v", path, got, want)
		}
	}
}
