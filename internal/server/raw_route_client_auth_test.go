// SPDX-License-Identifier: Apache-2.0

package server

import (
	"encoding/json"
	"strings"
	"testing"
)

// v2.64.0 (discussion #137): `tls { client_auth … }` in a pasted block adapts
// to a tls_connection_policies entry, which an Advanced route cannot store —
// it keeps routes only. It used to disappear without a word, leaving a site
// that looked locked down and let everyone in. The JSON below is what Caddy
// 2.11 produced for the reporter's block.
func adaptedClientAuthFixture(t *testing.T, raw string) map[string]any {
	t.Helper()
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

const clientAuthAdapted = `{"apps":{"http":{"servers":{"srv0":{
  "listen":[":443"],
  "routes":[{"match":[{"host":["ejbca.example.com"]}],"handle":[{"handler":"subroute","routes":[{"handle":[{"handler":"reverse_proxy","upstreams":[{"dial":"srv1.example.com:8082"}]}]}]}],"terminal":true}],
  "tls_connection_policies":[
    {"match":{"sni":["ejbca.example.com"]},"client_authentication":{"ca":{"pem_files":["/etc/caddy/tls/ManagementCA.cacert.pem"],"provider":"file"},"mode":"require_and_verify"}},
    {}
  ]}}}}}`

func TestAdaptedClientAuthSNIsNamesEveryAffectedHost(t *testing.T) {
	got := adaptedClientAuthSNIs(adaptedClientAuthFixture(t, clientAuthAdapted))
	if len(got) != 1 || got[0] != "ejbca.example.com" {
		t.Fatalf("got %v, want [ejbca.example.com]", got)
	}
}

// The empty catch-all policy carries no client_authentication and must not be
// mistaken for one, or every import would be refused.
func TestAdaptedClientAuthSNIsIgnoresPlainPolicies(t *testing.T) {
	const plain = `{"apps":{"http":{"servers":{"srv0":{
	  "tls_connection_policies":[{"match":{"sni":["a.example.com"]},"protocol_min":"tls1.3"},{}]}}}}}`
	if got := adaptedClientAuthSNIs(adaptedClientAuthFixture(t, plain)); len(got) != 0 {
		t.Errorf("a min-version-only policy was reported as client auth: %v", got)
	}
}

// A policy with no SNI match applies to everything the server serves; it still
// has to be reported, with a name the message can print.
func TestAdaptedClientAuthSNIsReportsUnmatchedPolicy(t *testing.T) {
	const noMatch = `{"apps":{"http":{"servers":{"srv0":{
	  "tls_connection_policies":[{"client_authentication":{"mode":"require"}}]}}}}}`
	got := adaptedClientAuthSNIs(adaptedClientAuthFixture(t, noMatch))
	if len(got) != 1 || !strings.Contains(got[0], "all hostnames") {
		t.Errorf("got %v, want a single (all hostnames) entry", got)
	}
}

func TestAdaptedClientAuthSNIsIsEmptyForOrdinaryConfig(t *testing.T) {
	if got := adaptedClientAuthSNIs(adaptedListenFixture(t)); len(got) != 0 {
		t.Errorf("ordinary config reported client auth: %v", got)
	}
}
