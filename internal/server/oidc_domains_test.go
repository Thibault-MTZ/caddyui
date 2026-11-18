// SPDX-License-Identifier: Apache-2.0

package server

import (
	"reflect"
	"testing"
)

// v2.67.0: SSO sign-in can be restricted to named email domains.
//
// The motivating case is a public identity provider. With a private IdP,
// "anyone my provider authenticates" means your own staff; point CaddyUI at
// Google and it means every account in the world, which with auto-create on
// is a CaddyUI account for anyone who finds the login page. These tests pin
// down the refusals rather than just the happy path.

func TestAllowedDomainsParsing(t *testing.T) {
	got := parseOIDCAllowedDomains("Example.com\n@example.org, \n  EXAMPLE.NET  \n# a comment\n\n")
	want := []string{"example.com", "example.org", "example.net"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsed = %#v, want %#v — lowercased, '@' tolerated, comments and blanks dropped", got, want)
	}
}

// An empty list must not restrict anything, or upgrading would lock out every
// existing SSO user.
func TestEmptyListAllowsEveryone(t *testing.T) {
	cfg := oidcConfig{}
	for _, e := range []string{"anyone@gmail.com", "staff@example.com", "odd@sub.example.co.uk"} {
		if !cfg.emailDomainAllowed(e) {
			t.Errorf("%s was refused with no domain list configured", e)
		}
	}
}

func TestOnlyListedDomainsAreAdmitted(t *testing.T) {
	cfg := oidcConfig{AllowedDomains: parseOIDCAllowedDomains("example.com, example.org")}
	for e, want := range map[string]bool{
		"staff@example.com":      true,
		"other@example.org":      true,
		"Staff@Example.Com":      true,  // address casing must not matter
		"attacker@gmail.com":     false, // the whole point
		"staff@notexample.com":   false,
		"staff@example.com.evil": false, // suffix games must not pass
	} {
		if got := cfg.emailDomainAllowed(e); got != want {
			t.Errorf("emailDomainAllowed(%q) = %v, want %v", e, got, want)
		}
	}
}

// Listing a domain must not silently admit its subdomains — mail.example.com
// can be a different tenant entirely.
func TestSubdomainsAreNotImplied(t *testing.T) {
	cfg := oidcConfig{AllowedDomains: []string{"example.com"}}
	if cfg.emailDomainAllowed("user@mail.example.com") {
		t.Error("a subdomain was admitted by listing the parent domain")
	}
	cfg2 := oidcConfig{AllowedDomains: []string{"example.com", "mail.example.com"}}
	if !cfg2.emailDomainAllowed("user@mail.example.com") {
		t.Error("an explicitly listed subdomain was refused")
	}
}

// A malformed address must fail closed rather than slipping through.
func TestMalformedAddressesAreRefused(t *testing.T) {
	cfg := oidcConfig{AllowedDomains: []string{"example.com"}}
	for _, e := range []string{"", "no-at-sign", "trailing@", "@example.com "} {
		if cfg.emailDomainAllowed(e) {
			t.Errorf("malformed address %q was admitted", e)
		}
	}
	// "@example.com" with no local part has a valid domain; it is the
	// provider's job to not assert that, and ours not to crash on it.
	_ = cfg.emailDomainAllowed("@example.com")
}

// An address whose domain differs only by case from the configured one must
// still match — IdPs are inconsistent about casing.
func TestConfiguredDomainCasingIsNormalised(t *testing.T) {
	cfg := oidcConfig{AllowedDomains: parseOIDCAllowedDomains("ExAmPlE.CoM")}
	if !cfg.emailDomainAllowed("user@example.com") {
		t.Error("configured domain casing should be normalised at parse time")
	}
}
