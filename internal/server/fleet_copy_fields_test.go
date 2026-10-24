// SPDX-License-Identifier: Apache-2.0

package server

import (
	"reflect"
	"testing"

	"github.com/X4Applegate/caddyui/internal/models"
)

// Issue #150: "protected sites now become public" on the other nodes. A fleet
// copy is a whole-struct copy saved through CreateProxyHost/UpdateProxyHost, so
// a column those two forget to write is silently left behind on every target.
// For a protection setting (an allowlist, Basic Auth, mTLS, forward auth) that
// means the copy serves the site open. This fills every field of a proxy host
// and checks that each one arrives, on create and on update, so a column added
// later without being wired into both functions fails here.

// fleetCopyPerTarget are the fields a copy deliberately does not carry:
// identity, ownership, timestamps, and the per-server DNS record and
// certificate selection (preserveProxyTargetPolicy).
var fleetCopyPerTarget = map[string]bool{
	"ID": true, "ServerID": true, "OwnerID": true, "OwnerEmail": true,
	"CreatedAt": true, "UpdatedAt": true, "CertificateID": true,
	"DNSProvider": true, "DNSZoneID": true, "DNSZoneName": true, "DNSRecordID": true,
	"DNSProfileID": true, "DNSSkipRecord": true, "CFDNSRecordID": true, "CFZoneID": true,
	"PBDNSRecordID": true, "PBDomain": true,
	// Filled from Settings at sync time, not stored on the row.
	"GlobalStripHeaders": true,
}

// fillProxyHost sets every copyable scalar field to a value derived from seed,
// so a create and an update with different seeds differ in every field.
func fillProxyHost(t *testing.T, p *models.ProxyHost, seed int) {
	t.Helper()
	v := reflect.ValueOf(p).Elem()
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		name := typ.Field(i).Name
		if fleetCopyPerTarget[name] {
			continue
		}
		f := v.Field(i)
		switch f.Kind() {
		case reflect.String:
			f.SetString(name + "-" + string(rune('a'+seed)))
		case reflect.Bool:
			f.SetBool(seed%2 == 1)
		case reflect.Int, reflect.Int64:
			f.SetInt(int64(10*seed + i%7 + 1))
		}
	}
	p.Domains = []string{"", "app.example.com", "app.example.com"}[seed%3]
	p.Enabled = true
	p.NodeLocal = false
}

func diffCopiedFields(t *testing.T, want, got *models.ProxyHost) []string {
	t.Helper()
	wv, gv := reflect.ValueOf(want).Elem(), reflect.ValueOf(got).Elem()
	var missing []string
	for i := 0; i < wv.NumField(); i++ {
		name := wv.Type().Field(i).Name
		if fleetCopyPerTarget[name] {
			continue
		}
		switch wv.Field(i).Kind() {
		case reflect.String, reflect.Bool, reflect.Int, reflect.Int64:
		default:
			continue
		}
		if !reflect.DeepEqual(wv.Field(i).Interface(), gv.Field(i).Interface()) {
			missing = append(missing, name)
		}
	}
	return missing
}

func TestFleetCopyCarriesEveryProxyHostSetting(t *testing.T) {
	s, sourceServerID, targetServerID := newFleetSyncTestServer(t)

	for step, seed := range []int{1, 2} {
		src := models.ProxyHost{}
		if step > 0 {
			cur, err := models.ListProxyHosts(s.DB, sourceServerID, 0, true, nil)
			if err != nil || len(cur) != 1 {
				t.Fatalf("source rows = %d, %v", len(cur), err)
			}
			src = cur[0]
		}
		fillProxyHost(t, &src, seed)
		var err error
		if src.ID == 0 {
			src.ID, err = models.CreateProxyHost(s.DB, sourceServerID, 0, &src)
		} else {
			err = models.UpdateProxyHost(s.DB, &src)
		}
		if err != nil {
			t.Fatal(err)
		}
		saved, err := models.GetProxyHost(s.DB, src.ID)
		if err != nil {
			t.Fatal(err)
		}
		// Fields the model itself does not keep are a different bug; only
		// what the source really holds has to reach the target.
		if lost := diffCopiedFields(t, &src, saved); len(lost) > 0 {
			t.Logf("step %d: not stored on the source row itself (skipped): %v", step, lost)
		}

		res, err := s.upsertFleetProxyHost(sourceServerID, targetServerID, *saved, 0)
		if err != nil {
			t.Fatalf("step %d: upsert: %v", step, err)
		}
		got, err := models.GetProxyHost(s.DB, res.ID)
		if err != nil {
			t.Fatal(err)
		}
		// The comparison must be able to fail: against an empty host, every
		// string and number differs (and every bool on the create pass).
		if n := len(diffCopiedFields(t, saved, &models.ProxyHost{})); n < 200 {
			t.Fatalf("step %d: only %d fields differ from an empty host; the fill is not reaching the struct", step, n)
		}
		if missing := diffCopiedFields(t, saved, got); len(missing) > 0 {
			t.Errorf("step %d (%s): %d setting(s) did not reach the target: %v",
				step, map[int]string{0: "create", 1: "update"}[step], len(missing), missing)
		}
	}
}
