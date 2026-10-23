// SPDX-License-Identifier: Apache-2.0

package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/X4Applegate/caddyui/internal/models"
)

// v2.68.3 (issue #149): a catch-all Advanced route served on :80 — here the
// fallback a live-Caddy import had copied into Advanced routes — came before
// the HTTPS redirect, so every Force SSL host got a 404 over plain HTTP.

const importedFallbackJSON = `{"handle":[{"handler":"error","status_code":404}],"terminal":true}`

func routeJSON(t *testing.T, r any) string {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// indexOf returns the position of the first route whose JSON contains want.
func indexOf(t *testing.T, routes []any, want string) int {
	t.Helper()
	for i, r := range routes {
		if strings.Contains(routeJSON(t, r), want) {
			return i
		}
	}
	t.Fatalf("no route contains %s in %v", want, routes)
	return -1
}

func isRedirect(t *testing.T, r any) bool {
	t.Helper()
	return strings.Contains(routeJSON(t, r), `"status_code":308`)
}

func TestForceSSLRedirectRunsBeforeACatchAllAdvancedRouteOnPlainHTTP(t *testing.T) {
	s := newHTTPOnlyTestServer(t)
	raws := []models.RawRoute{{Label: "srv0 route[37] (no host match)", Enabled: true, JSONData: importedFallbackJSON}}
	routes := s.buildHTTPRoutes([]models.ProxyHost{proxyRow("secure.example.com", true)}, nil, raws)

	if len(routes) != 3 {
		t.Fatalf("routes = %d, want redirect + catch-all + fallback: %v", len(routes), routes)
	}
	if !isRedirect(t, routes[0]) {
		t.Fatalf("route 0 is not the HTTPS redirect, so Force SSL hosts get the catch-all: %s", routeJSON(t, routes[0]))
	}
	if routeJSON(t, routes[1]) != importedFallbackJSON {
		t.Errorf("route 1 = %s, want the catch-all Advanced route after the redirect", routeJSON(t, routes[1]))
	}
	if last := lastRouteJSON(t, routes); !strings.Contains(last, `"handler":"error"`) || strings.Contains(last, `"match"`) {
		t.Errorf("the fallback is not last: %s", last)
	}
}

// Hosts served over plain HTTP keep running before the redirect; only the
// catch-all that follows them moves behind it.
func TestRedirectSitsAfterTheLastHostRouteAndBeforeTrailingCatchAlls(t *testing.T) {
	s := newHTTPOnlyTestServer(t)
	proxies := []models.ProxyHost{proxyRow("open.example.com", false), proxyRow("secure.example.com", true)}
	proxies[1].ID = 2
	raws := []models.RawRoute{{Enabled: true, JSONData: importedFallbackJSON}}
	routes := s.buildHTTPRoutes(proxies, nil, raws)

	open, redirect, catchAll := indexOf(t, routes, "open.example.com"), indexOf(t, routes, "secure.example.com"), indexOf(t, routes, `"status_code":404}],"terminal":true}`)
	if !isRedirect(t, routes[redirect]) {
		t.Fatalf("secure.example.com is not redirected: %s", routeJSON(t, routes[redirect]))
	}
	if !(open < redirect && redirect < catchAll) {
		t.Errorf("order open=%d redirect=%d catch-all=%d, want open < redirect < catch-all", open, redirect, catchAll)
	}
}

// A path-scoped route for a forced host (an HTTP-only /.well-known path, say)
// is more specific than the host's redirect and still has to win (issue #129).
func TestPathScopedHTTPRouteForAForcedHostStillBeatsItsRedirect(t *testing.T) {
	s := newHTTPOnlyTestServer(t)
	raws := []models.RawRoute{
		{Enabled: true, JSONData: importedFallbackJSON},
		{Enabled: true, JSONData: `{"match":[{"host":["secure.example.com"],"path":["/.well-known/*"]}],"handle":[{"handler":"static_response","status_code":200}]}`},
	}
	routes := s.buildHTTPRoutes([]models.ProxyHost{proxyRow("secure.example.com", true)}, nil, raws)

	path := indexOf(t, routes, "/.well-known/*")
	redirect := -1
	for i, r := range routes {
		if isRedirect(t, r) {
			redirect = i
		}
	}
	if redirect < 0 || path > redirect {
		t.Errorf("path route at %d, redirect at %d: the redirect would shadow the HTTP path", path, redirect)
	}
}

// An exact forced host must still be redirected ahead of a wildcard host that
// is served on :80 (issue #129), with a catch-all Advanced route in the list.
func TestExactForcedHostStillBeatsAPlainHTTPWildcardWithACatchAll(t *testing.T) {
	s := newHTTPOnlyTestServer(t)
	raws := []models.RawRoute{
		{Enabled: true, JSONData: `{"match":[{"host":["*.example.com"]}],"handle":[{"handler":"static_response","status_code":200}]}`},
		{Enabled: true, JSONData: importedFallbackJSON},
	}
	routes := s.buildHTTPRoutes([]models.ProxyHost{proxyRow("secure.example.com", true)}, nil, raws)

	wildcard, catchAll := indexOf(t, routes, "*.example.com"), indexOf(t, routes, `"status_code":404}],"terminal":true}`)
	redirect := -1
	for i, r := range routes {
		if isRedirect(t, r) {
			redirect = i
		}
	}
	if !(redirect >= 0 && redirect < wildcard && redirect < catchAll) {
		t.Errorf("redirect=%d wildcard=%d catch-all=%d, want the redirect first", redirect, wildcard, catchAll)
	}
}
