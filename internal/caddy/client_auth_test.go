// SPDX-License-Identifier: Apache-2.0

package caddy

import (
	"encoding/json"
	"testing"

	"github.com/X4Applegate/caddyui/internal/models"
)

// v2.64.0 (discussion #137): a `tls { client_auth ... }` block in a Caddyfile
// adapts to a client_authentication object inside the server's
// tls_connection_policies — never into the route. The expected JSON below is
// the verbatim policy Caddy 2.11 emitted for
//
//	ejbca.example.com {
//	    tls {
//	        client_auth {
//	            mode require_and_verify
//	            trust_pool file /etc/caddy/tls/ManagementCA.cacert.pem
//	        }
//	    }
//	    reverse_proxy srv1.example.com:8082
//	}
//
// If this test fails after a Caddy upgrade, re-run `caddy adapt` on that
// Caddyfile and compare — the key names here (pem_files, not
// trusted_ca_certs_pem_files) are Caddy's, not ours.
func TestClientAuthMatchesCaddysOwnAdaptedJSON(t *testing.T) {
	ph := models.ProxyHost{
		ID: 1, Domains: "ejbca.example.com", Enabled: true,
		ClientAuthMode:    "require_and_verify",
		ClientAuthCAFiles: "/etc/caddy/tls/ManagementCA.cacert.pem",
	}
	policies := BuildTLSConnectionPolicies([]models.ProxyHost{ph})
	if len(policies) != 2 {
		t.Fatalf("want the host policy plus the catch-all, got %d: %v", len(policies), policies)
	}
	got, err := json.Marshal(policies[0])
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"client_authentication":{"ca":{"pem_files":["/etc/caddy/tls/ManagementCA.cacert.pem"],"provider":"file"},"mode":"require_and_verify"},"match":{"sni":["ejbca.example.com"]}}`
	if string(got) != want {
		t.Errorf("policy does not match Caddy's own adapted JSON\n got: %s\nwant: %s", got, want)
	}
	// The catch-all has to stay last or Caddy serves TLS for nothing else.
	if last, _ := json.Marshal(policies[1]); string(last) != "{}" {
		t.Errorf("last policy should be the empty catch-all, got %s", last)
	}
}

// Client auth alone is enough to produce a policy. Before v2.64.0 the builder
// only looked at TLSMinVersion, so a host that set nothing else was skipped
// and its client_auth silently never reached Caddy.
func TestClientAuthWithoutMinVersionStillProducesAPolicy(t *testing.T) {
	ph := models.ProxyHost{ID: 1, Domains: "a.example.com", Enabled: true, ClientAuthMode: "require"}
	policies := BuildTLSConnectionPolicies([]models.ProxyHost{ph})
	if len(policies) != 2 {
		t.Fatalf("want a policy plus catch-all, got %d", len(policies))
	}
	p := policies[0].(map[string]any)
	if _, ok := p["protocol_min"]; ok {
		t.Error("no minimum version was set, so protocol_min should be absent")
	}
	ca := p["client_authentication"].(map[string]any)
	if ca["mode"] != "require" {
		t.Errorf("mode = %v, want require", ca["mode"])
	}
	// "require" does not verify the issuer, so no trust pool is emitted.
	if _, ok := ca["ca"]; ok {
		t.Errorf("no CA files were given, so no trust pool should be emitted: %v", ca)
	}
}

// Both settings share one policy for the host rather than producing two
// competing entries for the same SNI.
func TestMinVersionAndClientAuthShareOnePolicy(t *testing.T) {
	ph := models.ProxyHost{
		ID: 1, Domains: "a.example.com, b.example.com", Enabled: true,
		TLSMinVersion:     "1.3",
		ClientAuthMode:    "require_and_verify",
		ClientAuthCAFiles: "/certs/one.pem\n/certs/two.pem",
	}
	policies := BuildTLSConnectionPolicies([]models.ProxyHost{ph})
	if len(policies) != 2 {
		t.Fatalf("want one host policy plus catch-all, got %d", len(policies))
	}
	p := policies[0].(map[string]any)
	if p["protocol_min"] != "tls1.3" {
		t.Errorf("protocol_min = %v, want tls1.3", p["protocol_min"])
	}
	snis := p["match"].(map[string]any)["sni"].([]any)
	if len(snis) != 2 {
		t.Errorf("both domains should match this policy, got %v", snis)
	}
	files := p["client_authentication"].(map[string]any)["ca"].(map[string]any)["pem_files"].([]any)
	if len(files) != 2 || files[0] != "/certs/one.pem" || files[1] != "/certs/two.pem" {
		t.Errorf("both CA files should be listed, got %v", files)
	}
}

// A disabled host must not be able to demand client certificates.
func TestDisabledHostEmitsNoClientAuth(t *testing.T) {
	ph := models.ProxyHost{ID: 1, Domains: "a.example.com", Enabled: false,
		ClientAuthMode: "require_and_verify", ClientAuthCAFiles: "/certs/ca.pem"}
	if got := BuildTLSConnectionPolicies([]models.ProxyHost{ph}); got != nil {
		t.Errorf("disabled host produced policies: %v", got)
	}
}

// An unknown mode must never reach Caddy, which would reject the whole config
// and take every other site down with it.
func TestUnknownClientAuthModeIsIgnored(t *testing.T) {
	ph := models.ProxyHost{ID: 1, Domains: "a.example.com", Enabled: true,
		ClientAuthMode: "require_and_verify_and_hope", ClientAuthCAFiles: "/certs/ca.pem"}
	if got := BuildTLSConnectionPolicies([]models.ProxyHost{ph}); got != nil {
		t.Errorf("unknown mode produced policies: %v", got)
	}
}

func TestClientAuthCAFileListAcceptsCommasAndNewlines(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
	}{
		{"", 0},
		{"   ", 0},
		{"/a.pem", 1},
		{"/a.pem,/b.pem", 2},
		{"/a.pem\n/b.pem\n", 2},
		{"/a.pem,\n  /b.pem , /c.pem", 3},
	} {
		got := models.ProxyHost{ClientAuthCAFiles: tc.in}.ClientAuthCAFileList()
		if len(got) != tc.want {
			t.Errorf("%q → %v, want %d entries", tc.in, got, tc.want)
		}
		for _, f := range got {
			if f == "" || f != trimmed(f) {
				t.Errorf("%q produced an untrimmed entry %q", tc.in, f)
			}
		}
	}
}

func trimmed(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
