// SPDX-License-Identifier: Apache-2.0
package server

import (
	"strings"
	"testing"
	"time"
)

func TestRelativeTimeTranslationIsLocalAndKeepsBoundaries(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		lang  string
		value time.Time
		want  string
	}{
		{"en", time.Time{}, "never"}, {"fr", time.Time{}, "jamais"},
		{"en", now.Add(-59 * time.Second), "59s ago"}, {"fr", now.Add(-59 * time.Second), "il y a 59 s"},
		{"en", now.Add(-time.Minute), "1m ago"}, {"fr", now.Add(-time.Minute), "il y a 1 min"},
		{"en", now.Add(-2 * time.Hour), "2h ago"}, {"fr", now.Add(2 * time.Hour), "dans 2 h"},
		{"fr", now.Add(-24 * time.Hour), "il y a 1 j"},
		{"zh-CN", now.Add(-time.Minute), "1m ago"},
	} {
		if got := formatRelativeTime(map[string]any{"Lang": tc.lang}, tc.value, now); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.lang, got, tc.want)
		}
	}
	if got := formatRelativeTime("fr", now.Add(-40*24*time.Hour), now); strings.Contains(got, "time.") {
		t.Error(got)
	}
	if got := humanDurationLocalized(time.Hour, "fr"); got != "1 heure" {
		t.Error(got)
	}
	if got := humanDurationLocalized(48*time.Hour, "en"); got != "2 days" {
		t.Error(got)
	}
}
