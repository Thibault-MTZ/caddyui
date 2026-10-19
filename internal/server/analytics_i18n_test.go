// SPDX-License-Identifier: Apache-2.0

package server

import (
	"bytes"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/X4Applegate/caddyui/internal/models"
)

func TestAnalyticsSettingsDescriptionShowsConfiguredRetention(t *testing.T) {
	e := newSecEnv(t)
	for _, tc := range []struct{ lang, days, want string }{
		{"en", "7", "Raw events retained for 7 days."},
		{"en", "1", "Raw events retained for 1 day."},
		{"fr", "1", "Événements bruts conservés pendant 1 jour."},
		{"fr", "7", "Événements bruts conservés pendant 7 jours."},
		{"en", "0", "Raw events retained forever."},
		{"fr", "0", "Événements bruts conservés indéfiniment."},
	} {
		t.Run(tc.lang+tc.days, func(t *testing.T) {
			if err := models.SetSetting(e.db, settingAnalyticsRetentionDays, tc.days); err != nil {
				t.Fatal(err)
			}
			e.do(t, "admin", http.MethodPost, "/profile", url.Values{"action": {"update_locale"}, "locale": {tc.lang}})
			body := e.do(t, "admin", http.MethodGet, "/settings/analytics", nil).Body.String()
			if !strings.Contains(body, tc.want) {
				t.Errorf("missing configured retention %q", tc.want)
			}
			if strings.Contains(body, "Retained 30 days.") || strings.Contains(body, "Conservation : 30 jours.") {
				t.Error("stale hardcoded retention remains")
			}
		})
	}
}

func TestAnalyticsTranslationEscapesInterpolatedDataAndFallsBack(t *testing.T) {
	view := template.Must(template.New("label").Funcs(template.FuncMap{"t": tFunc}).Parse(`{{t $ "analytics.shown" "count" .Value}}`))
	for _, lang := range []string{"fr", "zh-CN"} {
		var out bytes.Buffer
		if err := view.Execute(&out, map[string]any{"Lang": lang, "Value": `<img src=x onerror=alert(1)>`}); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), "<img") || !strings.Contains(out.String(), "&lt;img") {
			t.Errorf("%s: interpolated data was not escaped: %s", lang, out.String())
		}
		want := "shown" // zh-CN has no Analytics keys: fall back to English.
		if lang == "fr" {
			want = "affichés"
		}
		if !strings.Contains(out.String(), want) {
			t.Errorf("%s: missing %q in %s", lang, want, out.String())
		}
	}
}

// Exercise the real router and templates, including the empty-state branches.
// A translated window label must not change which filter button is selected.
func TestAnalyticsPagesFollowLocaleAndKeepWindowFilters(t *testing.T) {
	e := newSecEnv(t)
	e.do(t, "admin", http.MethodPost, "/profile", url.Values{"action": {"update_locale"}, "locale": {"fr"}})
	pages := map[string][]string{
		"/settings/analytics":   {"Statistiques de fréquentation", "Cible de collecte", "Récupérer l&#39;espace", `name="analytics_soft_start"`, `formaction="/settings/analytics/prune"`},
		"/analytics?server=all": {"Aucun trafic pour le moment", "ensemble du parc", "Exporter en CSV"},
		// fillBuckets supplies zero-valued chart buckets even with no stored
		// events, so this route renders the chart and empty visitors table.
		"/analytics/example.test?window=30d&server=all":                     {"30 derniers jours", "Visiteurs : 0 · Pages vues : 0 · Via Tous les serveurs", "Aucun visiteur sur cette période.", "Évolution des requêtes", `preserveAspectRatio="none" class="w-full h-32`, `font-medium">30 jours</a>`},
		"/analytics/example.test/path?p=%3Cscript%3E&window=1d&server=all":  {"&lt;script&gt;", "dernières 24 heures", "Aucun visiteur pour ce chemin", `font-medium">24 heures</a>`},
		"/analytics/example.test/visitor?ip=192.0.2.1&window=7d&server=all": {"Visiteur", "7 derniers jours", `data-ip="192.0.2.1"`, `js.analytics.block_confirm`, "Partout (tout le parc)"},
		"/analytics/example.test/status?class=4xx&window=30d&server=all":    {"4xx Erreur client", "30 derniers jours", "Aucune réponse dans cette classe"},
	}
	for path, wants := range pages {
		t.Run(path, func(t *testing.T) {
			rec := e.do(t, "admin", http.MethodGet, path, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s: status %d: %s", path, rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			for _, want := range wants {
				if !strings.Contains(body, want) {
					t.Errorf("missing %q", want)
				}
			}
			if strings.Contains(body, ">analytics.") || strings.Contains(body, "<h1><script>") {
				t.Error("raw translation key or unescaped request data in page")
			}
		})
	}
	// One person's locale must not affect another person's pages.
	body := e.do(t, "alice", http.MethodGet, "/analytics", nil).Body.String()
	if !strings.Contains(body, "You're seeing the hosts you own.") && !strings.Contains(body, "You&#39;re seeing the hosts you own.") {
		t.Error("another user's English owned-host empty state changed")
	}
}

func TestAnalyticsCatalogKeepsMarkupOutOfTranslations(t *testing.T) {
	for _, lang := range []string{"en", "fr"} {
		for key, value := range translations().Catalog(lang) {
			if strings.HasPrefix(key, "analytics.") || strings.HasPrefix(key, "js.analytics.") {
				if strings.ContainsAny(value, "<>") {
					t.Errorf("%s %s contains HTML: %q", lang, key, value)
				}
				if tFunc(lang, key) == key {
					t.Errorf("%s %s did not resolve", lang, key)
				}
			}
		}
	}
}
