// SPDX-License-Identifier: Apache-2.0

package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	appdb "github.com/X4Applegate/caddyui/internal/db"
	"github.com/X4Applegate/caddyui/internal/models"
)

// v2.65.0 (issue #140): the admin IP allowlist used to compare r.RemoteAddr,
// which behind any reverse proxy is the proxy. Two symmetrical failures came
// out of that: allowlisting your real address locked you out, and the obvious
// workaround — allowlisting the proxy — admitted everyone who arrived through
// it while the UI still read as "restricted".

func allowlistTestServer(t *testing.T, allowlist string) *Server {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "caddyui.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := models.SetSetting(conn, settingAdminAllowlist, allowlist); err != nil {
		t.Fatal(err)
	}
	return &Server{DB: conn}
}

// gateStatus runs a request through adminIPGate and reports the status.
func gateStatus(t *testing.T, s *Server, remoteAddr string, headers map[string]string) int {
	t.Helper()
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = remoteAddr
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.adminIPGate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, r)
	return rec.Code
}

// The reporter's lockout: the visitor's real address is on the list, but the
// request reaches CaddyUI through a proxy.
func TestAllowlistedVisitorBehindAProxyGetsIn(t *testing.T) {
	s := allowlistTestServer(t, "203.0.113.9")
	// 127.0.0.1 is a trusted proxy by default (loopback/private), so the
	// forwarded address is believed.
	if got := gateStatus(t, s, "127.0.0.1:5555", map[string]string{"X-Forwarded-For": "203.0.113.9"}); got != http.StatusOK {
		t.Errorf("allowlisted visitor got %d, want 200 — this is the lockout from #140", got)
	}
}

func TestNonAllowlistedVisitorBehindAProxyIsRefused(t *testing.T) {
	s := allowlistTestServer(t, "203.0.113.9")
	if got := gateStatus(t, s, "127.0.0.1:5555", map[string]string{"X-Forwarded-For": "198.51.100.66"}); got != http.StatusForbidden {
		t.Errorf("unlisted visitor got %d, want 403", got)
	}
}

// The security half: a visitor cannot prepend an allowlisted address to
// X-Forwarded-For and be believed. Only the right-most entry — the one the
// trusted proxy appended — counts.
func TestForwardedForCannotBeSpoofed(t *testing.T) {
	s := allowlistTestServer(t, "203.0.113.9")
	if got := gateStatus(t, s, "127.0.0.1:5555", map[string]string{"X-Forwarded-For": "203.0.113.9, 198.51.100.66"}); got != http.StatusForbidden {
		t.Errorf("a spoofed left-most entry was believed (%d); only the right-most entry is the proxy's", got)
	}
}

// An untrusted peer's headers are ignored entirely, so a direct caller cannot
// claim an allowlisted address.
func TestUntrustedPeerHeadersAreIgnored(t *testing.T) {
	s := allowlistTestServer(t, "203.0.113.9")
	if got := gateStatus(t, s, "198.51.100.66:5555", map[string]string{"X-Forwarded-For": "203.0.113.9"}); got != http.StatusForbidden {
		t.Errorf("headers from an untrusted public peer were believed (%d)", got)
	}
}

func TestDirectConnectionFromAnAllowlistedAddressGetsIn(t *testing.T) {
	s := allowlistTestServer(t, "198.51.100.66")
	if got := gateStatus(t, s, "198.51.100.66:5555", nil); got != http.StatusOK {
		t.Errorf("direct allowlisted connection got %d, want 200", got)
	}
}

func TestEmptyAllowlistAllowsEveryone(t *testing.T) {
	s := allowlistTestServer(t, "")
	if got := gateStatus(t, s, "198.51.100.66:5555", nil); got != http.StatusOK {
		t.Errorf("an unset allowlist must not gate anything, got %d", got)
	}
}

func TestCIDREntriesMatch(t *testing.T) {
	s := allowlistTestServer(t, "10.0.0.0/8\n203.0.113.0/24")
	if got := gateStatus(t, s, "127.0.0.1:5555", map[string]string{"X-Forwarded-For": "203.0.113.77"}); got != http.StatusOK {
		t.Errorf("address inside an allowlisted CIDR got %d, want 200", got)
	}
	if got := gateStatus(t, s, "127.0.0.1:5555", map[string]string{"X-Forwarded-For": "203.0.114.1"}); got != http.StatusForbidden {
		t.Errorf("address outside every CIDR got %d, want 403", got)
	}
}

// The lockout escape hatch. The stored list must be left alone so removing
// the variable restores it.
func TestEnvOverrideReplacesTheStoredAllowlist(t *testing.T) {
	s := allowlistTestServer(t, "203.0.113.9")
	t.Setenv(envAdminAllowlist, "198.51.100.66")
	if got := s.adminAllowlist(); got != "198.51.100.66" {
		t.Errorf("effective allowlist = %q, want the environment value", got)
	}
	if !s.adminAllowlistFromEnv() {
		t.Error("the UI needs to know the stored list is not in force")
	}
	stored, _ := models.GetSetting(s.DB, settingAdminAllowlist)
	if stored != "203.0.113.9" {
		t.Errorf("stored setting was modified (%q); removing the variable must restore it", stored)
	}
}

func TestEnvOverrideOffDisablesTheGate(t *testing.T) {
	s := allowlistTestServer(t, "203.0.113.9")
	for _, v := range []string{"off", "OFF", "none", "any", "*"} {
		t.Setenv(envAdminAllowlist, v)
		if got := s.adminAllowlist(); got != "" {
			t.Errorf("%q should disable the gate, got %q", v, got)
		}
		if got := gateStatus(t, s, "198.51.100.66:5555", nil); got != http.StatusOK {
			t.Errorf("%q should let a locked-out admin back in, got %d", v, got)
		}
	}
}

// An empty variable is "not set", not "allow nobody" — otherwise a blank
// value in a compose file would be an accidental lockout of its own.
func TestEmptyEnvFallsBackToTheStoredList(t *testing.T) {
	s := allowlistTestServer(t, "203.0.113.9")
	t.Setenv(envAdminAllowlist, "")
	if got := s.adminAllowlist(); got != "203.0.113.9" {
		t.Errorf("effective allowlist = %q, want the stored value", got)
	}
	if s.adminAllowlistFromEnv() {
		t.Error("an empty variable is not an override")
	}
}
