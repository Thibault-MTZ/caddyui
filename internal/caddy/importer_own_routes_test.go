// SPDX-License-Identifier: Apache-2.0

package caddy

import (
	"encoding/json"
	"strings"
	"testing"
)

// v2.68.3 (issue #149): importing the live config of a Caddy that CaddyUI
// already manages stored CaddyUI's own fallback as an Advanced route
// ("srv0 route[37] (no host match)"), which was then served on :80 in front
// of the Force SSL redirects.
func TestImportSkipsCaddyUIsOwnHTTPServerAndFallback(t *testing.T) {
	var cfg map[string]any
	if err := json.Unmarshal([]byte(`{"apps":{"http":{"servers":{
	  "srv0":{"listen":[":443"],"routes":[
	    {"match":[{"host":["web.example.com"]}],"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"web:80"}]}]},
	    {"match":[{"host":["raw.example.com"]}],"handle":[{"handler":"subroute","routes":[{"match":[{"path":["/api/*"]}],"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"api:80"}]}]}]}]},
	    {"handle":[{"handler":"error","status_code":404}],"terminal":true}]},
	  "caddyui_http":{"listen":[":80"],"routes":[
	    {"match":[{"host":["web.example.com","raw.example.com"]}],"handle":[{"handler":"static_response","status_code":308,"headers":{"Location":["https://{http.request.host}{http.request.uri}"]}}],"terminal":true},
	    {"handle":[{"handler":"static_response","status_code":404,"headers":{"Content-Type":["text/html; charset=utf-8"]},"body":"<h1>Not here</h1>"}],"terminal":true}]}
	}}}}`), &cfg); err != nil {
		t.Fatal(err)
	}

	r := ClassifyConfig(cfg)
	if len(r.Redirect) != 0 {
		t.Errorf("the Force SSL redirect on caddyui_http became Redirection hosts: %+v", r.Redirect)
	}
	if len(r.Proxies) != 1 || r.Proxies[0].Domains != "web.example.com" {
		t.Errorf("proxies = %+v, want just web.example.com", r.Proxies)
	}
	if len(r.Passthrough) != 1 || r.Passthrough[0].Label != "raw.example.com" {
		t.Errorf("passthrough = %+v, want only the raw.example.com route, no fallback", r.Passthrough)
	}
	if len(r.Skipped) != 1 || !strings.Contains(r.Skipped[0], "srv0 route[2]") || !strings.Contains(r.Skipped[0], "fallback") {
		t.Errorf("skipped = %q, want the srv0 fallback reported", r.Skipped)
	}
}

func TestIsCaddyUIFallback(t *testing.T) {
	tests := []struct {
		name  string
		route string
		want  bool
	}{
		{"default error 404", `{"handle":[{"handler":"error","status_code":404}],"terminal":true}`, true},
		{"configured status", `{"handle":[{"handler":"error","status_code":410}],"terminal":true}`, true},
		{"custom page", `{"handle":[{"handler":"static_response","status_code":404,"headers":{"Content-Type":["text/html; charset=utf-8"]},"body":"<h1>x</h1>"}],"terminal":true}`, true},
		{"behind CrowdSec", `{"handle":[{"handler":"crowdsec"},{"handler":"error","status_code":404}],"terminal":true}`, true},
		{"behind CrowdSec with excluded paths", `{"handle":[{"handler":"subroute","routes":[{"match":[{"path":["/health"]}],"terminal":true},{"handle":[{"handler":"crowdsec"}]}]},{"handler":"error","status_code":404}],"terminal":true}`, true},
		{"has a matcher", `{"match":[{"host":["a.example.com"]}],"handle":[{"handler":"error","status_code":404}],"terminal":true}`, false},
		{"not terminal", `{"handle":[{"handler":"error","status_code":404}]}`, false},
		{"user catch-all proxy", `{"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"app:80"}]}],"terminal":true}`, false},
		{"user respond with other headers", `{"handle":[{"handler":"static_response","status_code":404,"headers":{"X-Why":["gone"]},"body":"x"}],"terminal":true}`, false},
		{"error with message", `{"handle":[{"handler":"error","status_code":404,"error":"nope"}],"terminal":true}`, false},
		{"two real handlers", `{"handle":[{"handler":"headers"},{"handler":"error","status_code":404}],"terminal":true}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var route map[string]any
			if err := json.Unmarshal([]byte(tt.route), &route); err != nil {
				t.Fatal(err)
			}
			if got := isCaddyUIFallback(route); got != tt.want {
				t.Errorf("isCaddyUIFallback = %v, want %v", got, tt.want)
			}
		})
	}
}
