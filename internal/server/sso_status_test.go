// SPDX-License-Identifier: Apache-2.0

package server

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	appdb "github.com/X4Applegate/caddyui/internal/db"
	"github.com/X4Applegate/caddyui/internal/models"
)

// v2.68.0: the Users page shows how each account signs in.

func TestSSOProviderLabel(t *testing.T) {
	for in, want := range map[string]string{
		"https://accounts.google.com":        "Google",
		"https://sso.example.com":            "Example",
		"https://auth.authentik.io/app/o/x/": "Authentik",
		"":                                   "SSO", // never configured
		"not a url":                          "SSO",
		"https://localhost":                  "SSO", // single label, nothing to shorten
	} {
		if got := ssoProviderLabel(in); got != want {
			t.Errorf("ssoProviderLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

// An SSO-provisioned account is given a random string rather than a hash, so
// no password can ever match it. The Users page leans on that to warn that
// the account depends entirely on the provider, so it has to hold.
func TestCanPasswordLoginDistinguishesRealHashes(t *testing.T) {
	for name, tc := range map[string]struct {
		hash string
		want bool
	}{
		"bcrypt 2a":           {"$2a$10$abcdefghijklmnopqrstuv", true},
		"bcrypt 2b":           {"$2b$12$abcdefghijklmnopqrstuv", true},
		"bcrypt 2y":           {"$2y$10$abcdefghijklmnopqrstuv", true},
		"oidcRandom() output": {oidcRandom(), false},
		"empty":               {"", false},
		"looks almost right":  {"2a$10$notreallyahash", false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := (models.User{PasswordHash: tc.hash}).CanPasswordLogin(); got != tc.want {
				t.Errorf("CanPasswordLogin(%q) = %v, want %v", tc.hash, got, tc.want)
			}
		})
	}
}

func TestUsesSSOOnlyWhenRecorded(t *testing.T) {
	if (models.User{}).UsesSSO() {
		t.Error("an account that has never signed in via SSO must not report that it has")
	}
	u := models.User{SSOLastLogin: sql.NullTime{Time: time.Now(), Valid: true}}
	if !u.UsesSSO() {
		t.Error("a recorded SSO login was not reported")
	}
}

// Recording is what makes the badge truthful: it must persist, and it must
// apply to an account created locally that later uses SSO — not only to
// accounts the provider created.
func TestRecordSSOLoginPersistsAndAppliesToLocalAccounts(t *testing.T) {
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "caddyui.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// A perfectly ordinary local account with a real password hash.
	id, err := models.CreateUser(conn, "local@example.com", "$2a$10$abcdefghijklmnopqrstuv", "Local", models.RoleView)
	if err != nil {
		t.Fatal(err)
	}
	before, err := models.GetUserByEmail(conn, "local@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if before.UsesSSO() {
		t.Fatal("a freshly created local account must not look like an SSO user")
	}
	if !before.CanPasswordLogin() {
		t.Fatal("a real bcrypt hash should permit password login")
	}

	if err := models.RecordSSOLogin(conn, id, "https://accounts.google.com"); err != nil {
		t.Fatal(err)
	}

	after, err := models.GetUserByEmail(conn, "local@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !after.UsesSSO() {
		t.Error("the recorded SSO login did not survive a round trip")
	}
	if after.SSOIssuer != "https://accounts.google.com" {
		t.Errorf("issuer = %q, want the provider that was used", after.SSOIssuer)
	}
	// Both routes remain open for this account, and the page should say so.
	if !after.CanPasswordLogin() {
		t.Error("using SSO must not take away an existing password")
	}
	if ssoProviderLabel(after.SSOIssuer) != "Google" {
		t.Errorf("badge label = %q, want Google", ssoProviderLabel(after.SSOIssuer))
	}
}

// An account the provider created has no usable password, which is exactly
// the case the Users page flags as depending on SSO.
func TestProvisionedAccountIsFlaggedAsSSOOnly(t *testing.T) {
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "caddyui.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Mirrors what the OIDC callback does.
	id, err := models.CreateUser(conn, "joe@example.com", oidcRandom(), "Joe", models.RoleView)
	if err != nil {
		t.Fatal(err)
	}
	if err := models.RecordSSOLogin(conn, id, "https://accounts.google.com"); err != nil {
		t.Fatal(err)
	}
	u, err := models.GetUserByEmail(conn, "joe@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !u.UsesSSO() {
		t.Error("a provisioned account should report SSO use")
	}
	if u.CanPasswordLogin() {
		t.Error("a provisioned account has no usable password and must not claim otherwise")
	}
}
