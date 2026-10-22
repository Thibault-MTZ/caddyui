// SPDX-License-Identifier: Apache-2.0

package caddy

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/X4Applegate/caddyui/internal/models"
)

// v2.68.1 (issue #148): every refusal CaddyUI generates must carry a body and
// a Content-Type.
//
// A bodiless response with no content type is legal HTTP, and for a long time
// that is what the IP allowlist emitted. Browsers have nothing to render and
// nothing to go on; mobile Safari and Edge respond by offering the empty
// response as a `document.txt` download instead of showing a denial page.

func TestDenyResponseCarriesBodyAndContentType(t *testing.T) {
	h := denyForbidden()
	if h["status_code"] != 403 {
		t.Errorf("status = %v, want 403", h["status_code"])
	}
	body, _ := h["body"].(string)
	if !strings.Contains(body, "403") {
		t.Errorf("body = %q, want something a human can read", body)
	}
	headers, _ := h["headers"].(map[string]any)
	ct, _ := headers["Content-Type"].([]any)
	if len(ct) != 1 || !strings.HasPrefix(ct[0].(string), "text/plain") {
		t.Errorf("Content-Type = %v, want text/plain — its absence is the bug", ct)
	}
}

// Extra headers a caller needs (the Allow list on a 405) must survive, and
// must not displace the content type.
func TestDenyResponseMergesExtraHeaders(t *testing.T) {
	h := denyResponse(405, "405 Method Not Allowed", map[string]any{"Allow": []any{"GET, HEAD"}})
	headers := h["headers"].(map[string]any)
	if _, ok := headers["Allow"]; !ok {
		t.Error("the caller's Allow header was dropped")
	}
	if _, ok := headers["Content-Type"]; !ok {
		t.Error("merging extra headers lost the content type")
	}
	if h["status_code"] != 405 {
		t.Errorf("status = %v, want 405", h["status_code"])
	}
}

// The real guard: walk everything BuildProxyRoute emits for a host that
// refuses things several different ways, and assert no refusal is bodiless.
// This is what stops the bug coming back at a call site nobody thought of —
// there were sixteen of them when this was fixed.
func TestNoGeneratedRefusalIsBodiless(t *testing.T) {
	ph := models.ProxyHost{
		ID: 1, Domains: "deny.example.com", Enabled: true,
		ForwardScheme: "http", ForwardHost: "10.0.0.5", ForwardPort: 8080,
		AccessList:          "192.168.0.0/16", // allowlist → 403 for everyone else
		IPBlocklist:         "203.0.113.0/24", // blocklist → 403
		BlockCommonExploits: true,             // path blocks → 403
		BlockedMethods:      "TRACE,CONNECT",  // → 405
		AllowedMethods:      "GET,POST",       // → 405
		BlockPrivateIPs:     true,
		BlockedAgents:       "badbot",
		BlockEmptyUserAgent: true,
	}
	raw, err := json.Marshal(BuildProxyRoute(ph, nil))
	if err != nil {
		t.Fatal(err)
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatal(err)
	}

	var bad []string
	var walk func(any)
	walk = func(n any) {
		switch v := n.(type) {
		case map[string]any:
			if v["handler"] == "static_response" {
				code, _ := v["status_code"].(float64)
				// Redirects carry a Location and 204 means "no content" —
				// both are correctly bodiless. Only errors need a body.
				if code >= 400 {
					body, _ := v["body"].(string)
					headers, _ := v["headers"].(map[string]any)
					_, hasCT := headers["Content-Type"]
					if body == "" || !hasCT {
						bad = append(bad, jsonOf(v))
					}
				}
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(tree)

	if len(bad) > 0 {
		t.Errorf("%d generated refusal(s) have no body or no Content-Type — mobile browsers "+
			"offer these as a download instead of rendering them:\n%s", len(bad), strings.Join(bad, "\n"))
	}
}

// Redirects and 204s must NOT be given bodies — that would be the opposite
// mistake, and a 204 with a body is malformed.
func TestRedirectsAndNoContentStayBodiless(t *testing.T) {
	ph := models.ProxyHost{
		ID: 1, Domains: "r.example.com", Enabled: true,
		ForwardScheme: "http", ForwardHost: "10.0.0.5", ForwardPort: 8080,
		WWWRedirect: "add", CORSEnabled: true, CORSOrigins: "*",
	}
	raw, _ := json.Marshal(BuildProxyRoute(ph, nil))
	// A 204 carrying a body would show up as "status_code":204 near a "body".
	if regexp.MustCompile(`"body":"[^"]+"[^}]*"status_code":204`).Match(raw) ||
		regexp.MustCompile(`"status_code":204[^}]*"body":"[^"]+"`).Match(raw) {
		t.Error("a 204 response was given a body, which is malformed")
	}
}

func jsonOf(v any) string {
	b, _ := json.MarshalIndent(v, "  ", " ")
	return "  " + string(b)
}
