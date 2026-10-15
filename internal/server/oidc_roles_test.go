// SPDX-License-Identifier: Apache-2.0

package server

import (
	"reflect"
	"testing"

	"github.com/X4Applegate/caddyui/internal/models"
)

// v2.65.0 (issue #139): auto-created SSO accounts were always read-only.
// They can now take a configured default role, or one mapped from a group
// claim. The risk in all of this is handing out more access than intended,
// so these pin down the refusals as much as the matches.

func TestOIDCRoleMappingsRejectUnknownRoles(t *testing.T) {
	got := parseOIDCRoleMappings(`
# comment line
caddyui-admins|admin
infra|user
readers|view
typo-group|administrator
missing-role|
|admin
malformed line with no pipe
spaced | Admin
`)
	want := []oidcRoleMapping{
		{Group: "caddyui-admins", Role: models.RoleAdmin},
		{Group: "infra", Role: models.RoleUser},
		{Group: "readers", Role: models.RoleView},
		{Group: "spaced", Role: models.RoleAdmin}, // role is case-insensitive
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parsed mappings =\n %#v\nwant\n %#v", got, want)
	}
}

// "administrator" is not a CaddyUI role. Storing it would create accounts
// whose role no permission check recognises; dropping the rule means the user
// falls through to the default instead.
func TestUnknownRoleNeverReachesAnAccount(t *testing.T) {
	cfg := oidcConfig{RoleMappings: parseOIDCRoleMappings("admins|administrator")}
	if role, ok := cfg.roleForGroups([]string{"admins"}); ok {
		t.Errorf("an unknown role was matched and would be assigned: %q", role)
	}
	if got := cfg.defaultRole(); got != models.RoleView {
		t.Errorf("fallback role = %q, want view", got)
	}
}

func TestRoleForGroupsFirstMatchWins(t *testing.T) {
	cfg := oidcConfig{RoleMappings: parseOIDCRoleMappings("admins|admin\nstaff|user")}
	// A user in both groups gets the rule listed first, not the last or the
	// least privileged — the UI promises "first match wins".
	role, ok := cfg.roleForGroups([]string{"staff", "admins"})
	if !ok || role != models.RoleAdmin {
		t.Errorf("role = %q ok=%v, want admin true", role, ok)
	}
}

func TestRoleForGroupsIsCaseInsensitive(t *testing.T) {
	cfg := oidcConfig{RoleMappings: parseOIDCRoleMappings("CaddyUI-Admins|admin")}
	if role, ok := cfg.roleForGroups([]string{"caddyui-admins"}); !ok || role != models.RoleAdmin {
		t.Errorf("role = %q ok=%v, want admin true — IdPs are inconsistent about casing", role, ok)
	}
}

func TestNoMatchingGroupFallsBackToDefault(t *testing.T) {
	cfg := oidcConfig{DefaultRole: models.RoleUser, RoleMappings: parseOIDCRoleMappings("admins|admin")}
	if _, ok := cfg.roleForGroups([]string{"nobody"}); ok {
		t.Error("an unrelated group should not match")
	}
	if got := cfg.defaultRole(); got != models.RoleUser {
		t.Errorf("default role = %q, want user", got)
	}
}

// Every release before this one gave auto-created accounts the view role.
// An install that never touches the new settings has to keep doing that.
func TestUnconfiguredBehaviourIsUnchanged(t *testing.T) {
	cfg := oidcConfig{}
	if got := cfg.defaultRole(); got != models.RoleView {
		t.Errorf("default role = %q, want view — upgrading must not widen access", got)
	}
	if _, ok := cfg.roleForGroups([]string{"admins"}); ok {
		t.Error("no mappings are configured, so nothing should match")
	}
}

// IdPs disagree about how a groups claim is shaped. An unexpected shape must
// yield no groups rather than an error, so sign-in still succeeds on the
// default role.
func TestGroupsClaimShapes(t *testing.T) {
	for name, tc := range map[string]struct {
		in   any
		want []string
	}{
		"array of strings": {[]any{"admins", "staff"}, []string{"admins", "staff"}},
		"single string":    {"admins", []string{"admins"}},
		"space separated":  {"admins staff", []string{"admins", "staff"}},
		"comma separated":  {"admins,staff", []string{"admins", "staff"}},
		"blank entries":    {[]any{"admins", "", "  "}, []string{"admins"}},
		"array of objects": {[]any{map[string]any{"name": "admins"}}, nil},
		"number":           {float64(42), nil},
		"absent":           {nil, nil},
	} {
		t.Run(name, func(t *testing.T) {
			if got := oidcStringValues(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("oidcStringValues(%#v) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}
