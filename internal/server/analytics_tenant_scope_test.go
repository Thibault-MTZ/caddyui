// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"database/sql"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/X4Applegate/caddyui/internal/auth"
	appdb "github.com/X4Applegate/caddyui/internal/db"
	"github.com/X4Applegate/caddyui/internal/models"
)

// v2.68.3: two ways a non-admin saw other tenants' traffic in Analytics.
//
//  1. A viewer who could see no hosts at all got the whole fleet on
//     /analytics and in the request explorer: scopedHostsForAnalytics
//     returned nil for them, and callers read nil as "admin, no filter".
//  2. In the request explorer, a search with no host picked — exactly what a
//     bar on the fleet chart opens since v2.68.2 — filtered the listed rows
//     but counted the total and the status mix across every host.

const otherTenantHost = "other-tenant.example.com"

type scopeFixture struct {
	s        *Server
	db       *sql.DB
	withHost *models.User // sees mine.example.com
	noHost   *models.User // sees nothing
	admin    *models.User
	now      time.Time
}

func newScopeFixture(t *testing.T) scopeFixture {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	sid, err := models.CreateCaddyServer(conn, &models.CaddyServer{Name: "Primary", AdminURL: "http://127.0.0.1:2019", Type: "caddy"})
	if err != nil {
		t.Fatal(err)
	}
	user := func(email, role string) *models.User {
		id, err := models.CreateUser(conn, email, "x", email, role)
		if err != nil {
			t.Fatal(err)
		}
		u, _ := models.GetUserByID(conn, id)
		return u
	}
	f := scopeFixture{
		s: newRenderingTestServer(t, conn), db: conn, now: time.Now(),
		withHost: user("tenant@example.com", models.RoleUser),
		noHost:   user("empty@example.com", models.RoleUser),
		admin:    user("admin@example.com", models.RoleAdmin),
	}
	p := models.ProxyHost{Domains: "mine.example.com", ForwardScheme: "http", ForwardHost: "10.0.0.5", ForwardPort: 80, Enabled: true}
	if _, err := models.CreateProxyHost(conn, sid, f.withHost.ID, &p); err != nil {
		t.Fatal(err)
	}
	add := func(host string, n int) {
		for i := 0; i < n; i++ {
			e := models.AccessEvent{TS: f.now.Add(-time.Duration(i) * time.Second), ServerID: sid, Host: host, Path: "/p", Method: "GET", Status: 500, ClientIP: "203.0.113.9"}
			if err := models.InsertAccessEvent(conn, e); err != nil {
				t.Fatal(err)
			}
		}
	}
	add("mine.example.com", 2)
	add(otherTenantHost, 37)
	return f
}

func (f scopeFixture) get(t *testing.T, u *models.User, path string) string {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.ContextUserKey, u))
	w := httptest.NewRecorder()
	if strings.HasPrefix(path, "/analytics/requests") {
		f.s.getAnalyticsRequests(w, req)
	} else {
		f.s.getAnalytics(w, req)
	}
	if w.Code != 200 {
		t.Fatalf("%s as %s: status %d", path, u.Email, w.Code)
	}
	return w.Body.String()
}

var explorerTotal = regexp.MustCompile(`text-2xl font-semibold text-ink-900">(\d+)<`)

func (f scopeFixture) explorerTotal(t *testing.T, u *models.User, path string) (string, string) {
	t.Helper()
	body := f.get(t, u, path)
	m := explorerTotal.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("%s: no total on the page", path)
	}
	return m[1], body
}

func TestAViewerWithNoHostsIsNeverGivenTheFleetView(t *testing.T) {
	f := newScopeFixture(t)
	hosts, err := f.s.scopedHostsForAnalytics(f.noHost, 0)
	if err != nil {
		t.Fatal(err)
	}
	if hosts == nil {
		t.Fatal("a non-admin got a nil host list, which every caller reads as admin / no filter")
	}
	for _, path := range []string{"/analytics", "/analytics?server=all", "/analytics?server=1"} {
		if body := f.get(t, f.noHost, path); strings.Contains(body, otherTenantHost) {
			t.Errorf("%s shows another tenant's host to a viewer with no hosts", path)
		}
	}
	if body := f.get(t, f.noHost, "/analytics/requests?status=5xx&window=24h"); strings.Contains(body, otherTenantHost) {
		t.Error("the request explorer lists another tenant's requests to a viewer with no hosts")
	}
}

func TestExplorerCountsOnlyTheViewersOwnHosts(t *testing.T) {
	f := newScopeFixture(t)
	drill := fmt.Sprintf("/analytics/requests?from=%d&to=%d&server=all", f.now.Add(-time.Hour).Unix(), f.now.Add(time.Minute).Unix())
	for _, path := range []string{"/analytics/requests?status=5xx&window=24h", drill} {
		if total, body := f.explorerTotal(t, f.withHost, path); total != "2" || strings.Contains(body, otherTenantHost) {
			t.Errorf("%s as a tenant: total %s (want 2, their own requests only), other host shown = %v", path, total, strings.Contains(body, otherTenantHost))
		}
		if total, _ := f.explorerTotal(t, f.noHost, path); total != "0" {
			t.Errorf("%s as a viewer with no hosts: total %s, want 0", path, total)
		}
		if total, _ := f.explorerTotal(t, f.admin, path); total != "39" {
			t.Errorf("%s as admin: total %s, want the whole fleet (39)", path, total)
		}
	}
}

func TestScopedInsightFilterMatchesWildcardsAndEmptyMatchesNothing(t *testing.T) {
	f := newScopeFixture(t)
	count := func(filter models.InsightFilter) int64 {
		filter.From = f.now.Add(-time.Hour)
		n, err := models.SearchRequestCount(f.db, filter)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(models.InsightFilter{}); n != 39 {
		t.Errorf("unscoped = %d, want 39", n)
	}
	if n := count(models.InsightFilter{Scoped: true}); n != 0 {
		t.Errorf("scoped to no hosts = %d, want 0", n)
	}
	if n := count(models.InsightFilter{Scoped: true, OnlyHosts: []string{"*.example.com"}}); n != 39 {
		t.Errorf("scoped to *.example.com = %d, want 39", n)
	}
	if n := count(models.InsightFilter{Scoped: true, OnlyHosts: []string{"mine.example.com", "*.%"}}); n != 2 {
		t.Errorf("scoped to mine + a LIKE-injection wildcard = %d, want 2", n)
	}
}
