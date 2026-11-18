// SPDX-License-Identifier: Apache-2.0

package server

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	appdb "github.com/X4Applegate/caddyui/internal/db"
	"github.com/X4Applegate/caddyui/internal/models"
)

// v2.68.3 (issue #150): "fleet settings are not being propagated". Checked on
// a real v2.68.2 with two Caddy nodes: a new host and its middleware profile
// did reach the automatic target, but
//   - a Settings save (e.g. the fleet-wide IP blocklist) re-synced only the
//     selected server, so the other nodes kept serving blocked addresses;
//   - disabling a host, or putting it in maintenance, with the quick toggle or
//     a bulk action left its copy live on every target;
//   - ticking an automatic target copied nothing that already existed, with
//     no word that "Sync from current" is needed for that.

type followFleet struct {
	s                    *Server
	db                   *sql.DB
	source, edge, other  int64
	sourceFake, edgeFake *layer4FakeAdmin
}

func newFollowFleet(t *testing.T) followFleet {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "caddyui.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	mk := func(name string) (int64, *layer4FakeAdmin) {
		admin, fake := newLayer4FakeAdmin(t, `{"apps":{"http":{"servers":{"srv0":{"listen":[":443"],"routes":[]}}}}}`)
		id, err := models.CreateCaddyServer(conn, &models.CaddyServer{Name: name, AdminURL: admin.URL, Type: models.CaddyServerTypeManaged})
		if err != nil {
			t.Fatal(err)
		}
		return id, fake
	}
	f := followFleet{s: newRenderingTestServer(t, conn), db: conn}
	f.source, f.sourceFake = mk("Source")
	f.edge, f.edgeFake = mk("Edge")
	f.other, _ = mk("Other") // managed, but not a deployment target
	setDefaultTargets(t, conn, f.source, f.edge)
	return f
}

func (f followFleet) createHost(t *testing.T, domain string, extra url.Values) *models.ProxyHost {
	t.Helper()
	form := url.Values{
		"domains": {domain}, "forward_scheme": {"http"},
		"forward_host": {"203.0.113.10"}, "forward_port": {"8080"}, "enabled": {"on"},
	}
	for k, v := range extra {
		form[k] = v
	}
	rec := httptest.NewRecorder()
	f.s.createProxyHost(rec, cookieReq(t, "/proxy-hosts", form, f.source))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create %s: want 303, got %d: %s", domain, rec.Code, rec.Body.String())
	}
	return f.hostOn(t, f.source, domain)
}

func (f followFleet) hostOn(t *testing.T, serverID int64, domain string) *models.ProxyHost {
	t.Helper()
	hosts, err := models.ListProxyHosts(f.db, serverID, 0, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := range hosts {
		if hosts[i].Domains == domain {
			return &hosts[i]
		}
	}
	return nil
}

// state is a host's enabled/maintenance flags for failure messages.
func state(p *models.ProxyHost) string {
	if p == nil {
		return "no copy"
	}
	return fmt.Sprintf("enabled=%v maintenance=%v", p.Enabled, p.MaintenanceMode)
}

func (fa *layer4FakeAdmin) loads() int {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	return fa.loadCalls
}

func TestQuickTogglesFollowAutomaticDeploymentTargets(t *testing.T) {
	f := newFollowFleet(t)
	src := f.createHost(t, "app.example.com", nil)
	if c := f.hostOn(t, f.edge, "app.example.com"); c == nil || !c.Enabled {
		t.Fatalf("the automatic target did not get an enabled copy: %s", state(c))
	}

	idParam := strconv.FormatInt(src.ID, 10)
	rec := httptest.NewRecorder()
	f.s.toggleProxyHost(rec, withChiURLParam(cookieReq(t, "/proxy-hosts/x/toggle", nil, f.source), "id", idParam))
	if c := f.hostOn(t, f.edge, "app.example.com"); c == nil || c.Enabled {
		t.Fatalf("disabled on the source, but the copy on the target is %s", state(c))
	}

	rec = httptest.NewRecorder()
	f.s.toggleMaintenanceMode(rec, withChiURLParam(cookieReq(t, "/proxy-hosts/x/maintenance", nil, f.source), "id", idParam))
	if c := f.hostOn(t, f.edge, "app.example.com"); c == nil || !c.MaintenanceMode {
		t.Fatalf("maintenance mode did not reach the copy: %s", state(c))
	}
	if f.hostOn(t, f.other, "app.example.com") != nil {
		t.Error("a server that is not a deployment target received a copy")
	}
}

// A bulk action moves every copy too, and syncs each target once, not once
// per host.
func TestBulkActionsFollowTargetsWithOneSyncPerTarget(t *testing.T) {
	f := newFollowFleet(t)
	a := f.createHost(t, "a.example.com", nil)
	b := f.createHost(t, "b.example.com", nil)
	before := f.edgeFake.loads()

	form := url.Values{"action": {"disable"}, "ids[]": {strconv.FormatInt(a.ID, 10), strconv.FormatInt(b.ID, 10)}}
	rec := httptest.NewRecorder()
	f.s.bulkToggleProxyHosts(rec, cookieReq(t, "/proxy-hosts/bulk-toggle", form, f.source))
	for _, d := range []string{"a.example.com", "b.example.com"} {
		if c := f.hostOn(t, f.edge, d); c == nil || c.Enabled {
			t.Errorf("bulk disable: copy of %s on the target is %s, want disabled", d, state(c))
		}
	}
	if n := f.edgeFake.loads() - before; n != 1 {
		t.Errorf("the target was synced %d times for one bulk action of 2 hosts, want 1", n)
	}

	form.Set("action", "enable")
	rec = httptest.NewRecorder()
	f.s.bulkMaintenanceProxyHosts(rec, cookieReq(t, "/proxy-hosts/bulk-maintenance", form, f.source))
	for _, d := range []string{"a.example.com", "b.example.com"} {
		if c := f.hostOn(t, f.edge, d); c == nil || !c.MaintenanceMode {
			t.Errorf("bulk maintenance: copy of %s on the target is %s, want maintenance on", d, state(c))
		}
	}
}

func TestRedirectionAndAdvancedRouteTogglesFollowTargets(t *testing.T) {
	f := newFollowFleet(t)
	rec := httptest.NewRecorder()
	f.s.createRedirectionHost(rec, cookieReq(t, "/redirection-hosts", url.Values{
		"domains": {"old.example.com"}, "forward_scheme": {"https"},
		"forward_domain": {"new.example.com"}, "forward_http_code": {"301"}, "enabled": {"on"},
	}, f.source))
	redirOn := func(serverID int64) *models.RedirectionHost {
		rows, _ := models.ListRedirectionHosts(f.db, serverID, 0, true, nil)
		if len(rows) != 1 {
			t.Fatalf("server %d has %d redirections, want 1", serverID, len(rows))
		}
		return &rows[0]
	}
	src := redirOn(f.source)
	rec = httptest.NewRecorder()
	f.s.toggleRedirectionHost(rec, withChiURLParam(cookieReq(t, "/redirection-hosts/x/toggle", nil, f.source), "id", strconv.FormatInt(src.ID, 10)))
	if redirOn(f.edge).Enabled {
		t.Error("redirection disabled on the source stays enabled on the target")
	}

	raw := models.RawRoute{Label: "raw", Enabled: true, JSONData: `{"match":[{"host":["raw.example.com"]}],"handle":[{"handler":"static_response","status_code":200}]}`}
	id, err := models.CreateRawRoute(f.db, f.source, 0, &raw)
	if err != nil {
		t.Fatal(err)
	}
	raw.ID = id
	f.s.crossDeployRawRoute("test", f.source, &raw, []int64{f.edge})
	rec = httptest.NewRecorder()
	f.s.bulkToggleRawRoutes(rec, cookieReq(t, "/raw-routes/bulk-toggle", url.Values{"action": {"disable"}, "ids[]": {strconv.FormatInt(id, 10)}}, f.source))
	rows, _ := models.ListRawRoutes(f.db, f.edge, 0, true, nil)
	if len(rows) != 1 || rows[0].Enabled {
		t.Errorf("advanced route disabled on the source: %d copies on the target, first enabled=%v; want one, disabled", len(rows), len(rows) > 0 && rows[0].Enabled)
	}
}

func TestNodeLocalHostTogglesStayOffTargets(t *testing.T) {
	f := newFollowFleet(t)
	src := f.createHost(t, "db.internal.example.com", url.Values{"node_local": {"on"}})
	rec := httptest.NewRecorder()
	f.s.toggleProxyHost(rec, withChiURLParam(cookieReq(t, "/proxy-hosts/x/toggle", nil, f.source), "id", strconv.FormatInt(src.ID, 10)))
	if f.hostOn(t, f.edge, "db.internal.example.com") != nil {
		t.Error("toggling a node-local host deployed it to the target")
	}
}

func TestSettingsSavesResyncEveryManagedServer(t *testing.T) {
	f := newFollowFleet(t)
	if _, err := models.CreateCaddyServer(f.db, &models.CaddyServer{Name: "Monitor", AdminURL: "http://mon:2019", Type: models.CaddyServerTypeExternal}); err != nil {
		t.Fatal(err)
	}
	if got, want := f.s.settingsSyncServerIDs(f.edge), []int64{f.edge, f.source, f.other}; !reflect.DeepEqual(got, want) {
		t.Fatalf("settings sync order = %v, want %v (selected first, every managed server, no external)", got, want)
	}

	// End to end: the fleet-wide blocklist saved on Settings -> Security,
	// with Source selected, must reach the Edge Caddy too.
	f.createHost(t, "app.example.com", nil)
	rec := httptest.NewRecorder()
	f.s.postSettings(rec, cookieReq(t, "/settings", url.Values{
		"settings_section": {"security"}, "global_ip_blocklist": {"198.51.100.0/24"},
	}, f.source))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("settings save: want 303, got %d: %s", rec.Code, rec.Body.String())
	}
	reached := func(fa *layer4FakeAdmin) bool {
		fa.mu.Lock()
		defer fa.mu.Unlock()
		for _, w := range fa.writes {
			if strings.Contains(w, "198.51.100.0/24") {
				return true
			}
		}
		return false
	}
	deadline := time.Now().Add(10 * time.Second)
	for !(reached(f.sourceFake) && reached(f.edgeFake)) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !reached(f.sourceFake) || !reached(f.edgeFake) {
		t.Errorf("fleet-wide blocklist pushed to source=%v edge=%v, want both", reached(f.sourceFake), reached(f.edgeFake))
	}
}

func TestAddingAnAutomaticTargetSaysExistingResourcesNeedASync(t *testing.T) {
	f := newFollowFleet(t)
	setDefaultTargets(t, f.db, f.source) // none yet
	f.createHost(t, "app.example.com", nil)
	srv, _ := models.GetCaddyServer(f.db, f.source)

	update := func(targets ...int64) string {
		form := url.Values{"name": {"Source"}, "admin_url": {srv.AdminURL}, "type": {"managed"}}
		for _, id := range targets {
			form.Add("default_deploy_targets", strconv.FormatInt(id, 10))
		}
		rec := httptest.NewRecorder()
		f.s.updateServer(rec, withChiURLParam(postForm(t, "/servers/x", form), "id", strconv.FormatInt(f.source, 10)))
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("update: want 303, got %d: %s", rec.Code, rec.Body.String())
		}
		loc, _ := url.QueryUnescape(rec.Header().Get("Location"))
		return loc
	}
	if loc := update(f.edge); !strings.Contains(loc, "The 1 that already exist on Source were not copied") || !strings.Contains(loc, "Sync from current on Edge") {
		t.Errorf("adding a target: redirect = %q, want the not-copied notice", loc)
	}
	if loc := update(f.edge); loc != "/servers" {
		t.Errorf("saving again with the same target: redirect = %q, want no notice", loc)
	}
}
