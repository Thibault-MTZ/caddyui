// SPDX-License-Identifier: Apache-2.0

package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/X4Applegate/caddyui/internal/models"
)

// v2.68.3 (issue #150): a copy the target refused was written only to the
// Activity Log. The save redirected as if it had worked, and the target kept
// serving its old version of the host, protection settings included. A save
// that did not reach every target now says which one and why.

func deployErrorOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return loc.Query().Get("deploy_error")
}

func (f followFleet) createHostRec(t *testing.T, domain string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{
		"domains": {domain}, "forward_scheme": {"http"},
		"forward_host": {"203.0.113.10"}, "forward_port": {"8080"}, "enabled": {"on"},
	}
	rec := httptest.NewRecorder()
	f.s.createProxyHost(rec, cookieReq(t, "/proxy-hosts", form, f.source))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create %s: want 303, got %d: %s", domain, rec.Code, rec.Body.String())
	}
	return rec
}

func TestSaveThatReachesEveryTargetReportsNothing(t *testing.T) {
	f := newFollowFleet(t)
	rec := f.createHostRec(t, "app.example.com")
	if msg := deployErrorOf(t, rec); msg != "" {
		t.Fatalf("a copy that landed reported a problem: %s", msg)
	}
	if rec.Header().Get("Location") != "/proxy-hosts" {
		t.Errorf("Location = %q, want the plain list", rec.Header().Get("Location"))
	}
}

func TestSaveSaysWhenTheTargetAlreadyClaimsTheDomain(t *testing.T) {
	f := newFollowFleet(t)
	// The edge already serves the name as part of a different host, e.g. one
	// imported there before the source was made its configuration source.
	if _, err := models.CreateProxyHost(f.db, f.edge, 0, &models.ProxyHost{
		Domains: "app.example.com, www.app.example.com", ForwardScheme: "http",
		ForwardHost: "198.51.100.7", ForwardPort: 80, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	msg := deployErrorOf(t, f.createHostRec(t, "app.example.com"))
	if !strings.Contains(msg, "Edge") || !strings.Contains(msg, "already claimed") {
		t.Fatalf("deploy_error = %q, want it to name Edge and the domain conflict", msg)
	}
	if strings.Contains(msg, "Other") {
		t.Errorf("deploy_error names a server that is not a target: %q", msg)
	}
}

func TestSaveSaysWhenTheTargetIsMonitoringOnly(t *testing.T) {
	f := newFollowFleet(t)
	if _, err := f.db.Exec(`UPDATE caddy_servers SET type=? WHERE id=?`, models.CaddyServerTypeExternal, f.edge); err != nil {
		t.Fatal(err)
	}
	msg := deployErrorOf(t, f.createHostRec(t, "app.example.com"))
	if !strings.Contains(msg, "Edge") || !strings.Contains(msg, "monitoring-only") {
		t.Fatalf("deploy_error = %q, want it to name Edge as monitoring-only", msg)
	}
}

func TestDeployProblemsAreRecordedWithTheirReason(t *testing.T) {
	f := newFollowFleet(t)
	if _, err := models.CreateProxyHost(f.db, f.edge, 0, &models.ProxyHost{
		Domains: "app.example.com, www.app.example.com", ForwardScheme: "http",
		ForwardHost: "198.51.100.7", ForwardPort: 80, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	f.createHostRec(t, "app.example.com")
	var detail string
	if err := f.db.QueryRow(`SELECT detail FROM activity_log WHERE action='proxy_cross_deploy' AND success=0 ORDER BY id DESC LIMIT 1`).Scan(&detail); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detail, "already claimed") {
		t.Errorf("activity detail = %q, want the reason, not only the domains", detail)
	}
}

func TestWithDeployProblems(t *testing.T) {
	if got := withDeployProblems("/proxy-hosts", nil); got != "/proxy-hosts" {
		t.Errorf("no problems: %q", got)
	}
	got := withDeployProblems("/raw-routes/4/deploying", []string{"Edge: down", "Lab: refused"})
	u, err := url.Parse(got)
	if err != nil || u.Path != "/raw-routes/4/deploying" {
		t.Fatalf("got %q (%v)", got, err)
	}
	if msg := u.Query().Get("deploy_error"); !strings.Contains(msg, "Edge: down; Lab: refused") {
		t.Errorf("deploy_error = %q", msg)
	}
	if got := withDeployProblems("/x?a=1", []string{"E: x"}); !strings.HasPrefix(got, "/x?a=1&deploy_error=") {
		t.Errorf("existing query not kept: %q", got)
	}
}

func TestListPagesShowTheDeployProblemEscaped(t *testing.T) {
	f := newFollowFleet(t)
	for path, handler := range map[string]http.HandlerFunc{
		"/proxy-hosts":       f.s.listProxyHosts,
		"/redirection-hosts": f.s.listRedirectionHosts,
		"/raw-routes":        f.s.listRawRoutes,
	} {
		req := cookieReq(t, path+"?deploy_error="+url.QueryEscape("Saved, but not deployed. Edge: <b>refused</b>"), nil, f.source)
		req.Method = http.MethodGet
		rec := httptest.NewRecorder()
		handler(rec, req)
		body := rec.Body.String()
		if !strings.Contains(body, "Edge: &lt;b&gt;refused&lt;/b&gt;") {
			t.Errorf("%s: the notice is missing or not escaped (status %d)", path, rec.Code)
		}
		if strings.Contains(body, "<b>refused</b>") {
			t.Errorf("%s: the notice was rendered as HTML", path)
		}
	}
}
