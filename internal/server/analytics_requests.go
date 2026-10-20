// SPDX-License-Identifier: Apache-2.0

package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/X4Applegate/caddyui/internal/models"
)

// The request explorer (issue #145).
//
// The reporter's complaint was not that the data was missing but that getting
// to it took "a lot of back and forth": status mix on one page, visitors on
// another, user agents only under an individual visitor. Each drill-down
// answered one question and then dead-ended.
//
// This is one page that takes every dimension at once — host, status class,
// user agent, path, client IP and an explicit time window — and lists the
// matching requests. The existing per-host drill-downs still exist and now
// link here, so "4xx for this host, last hour, from this bot" is one view
// rather than four.

// requestFilterForm is the parsed filter, kept as strings so the template can
// render the form back without reformatting.
type requestFilterForm struct {
	Host     string
	Status   string
	UA       string
	Path     string
	ClientIP string
	Window   string
	From     string // RFC3339-ish, from chart drill-through
	To       string
}

// any reports whether the operator has narrowed anything at all. An unfiltered
// explorer is just a firehose, so the page says so rather than rendering the
// most recent 200 requests as if they were a result.
func (f requestFilterForm) any() bool {
	return f.Host != "" || f.Status != "" || f.UA != "" || f.Path != "" ||
		f.ClientIP != "" || f.From != ""
}

// parseRequestFilter reads the query string into a filter form.
func parseRequestFilter(r *http.Request) requestFilterForm {
	q := r.URL.Query()
	get := func(k string) string { return strings.TrimSpace(q.Get(k)) }
	f := requestFilterForm{
		Host:     strings.ToLower(get("host")),
		Status:   strings.ToLower(get("status")),
		UA:       get("ua"),
		Path:     get("path"),
		ClientIP: get("ip"),
		Window:   strings.ToLower(get("window")),
		From:     get("from"),
		To:       get("to"),
	}
	switch f.Status {
	case "2xx", "3xx", "4xx", "5xx", "other":
	default:
		f.Status = ""
	}
	return f
}

// windowDuration maps the window token to a duration, defaulting to 7 days.
func windowDuration(token string) time.Duration {
	switch token {
	case "1h":
		return time.Hour
	case "6h":
		return 6 * time.Hour
	case "1d", "24h", "day":
		return 24 * time.Hour
	case "30d":
		return 30 * 24 * time.Hour
	default:
		return 7 * 24 * time.Hour
	}
}

// parseUnixParam reads a Unix-seconds query parameter. The chart links pass
// bucket boundaries this way, which keeps the drill-through exact rather than
// re-deriving a window from a label.
func parseUnixParam(v string) (time.Time, bool) {
	if v == "" {
		return time.Time{}, false
	}
	sec, err := strconv.ParseInt(v, 10, 64)
	if err != nil || sec <= 0 {
		return time.Time{}, false
	}
	return time.Unix(sec, 0).UTC(), true
}

// getAnalyticsRequests renders the request explorer.
func (s *Server) getAnalyticsRequests(w http.ResponseWriter, r *http.Request) {
	u := s.currentUser(r)
	if u == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	isAdmin := u.Role == models.RoleAdmin
	serverScopeID := s.analyticsServerScope(r)
	f := parseRequestFilter(r)

	// Non-admins may only see hosts they own. The filter is intersected with
	// that set rather than trusted, so a hand-edited ?host= cannot reach
	// another tenant's traffic.
	var allowedHosts []string
	if !isAdmin {
		var err error
		allowedHosts, err = s.scopedHostsForAnalytics(u, serverScopeID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if f.Host != "" {
			permitted := false
			for _, h := range allowedHosts {
				if h == f.Host {
					permitted = true
					break
				}
			}
			if !permitted {
				// 404 rather than 403, so the set of hosts stays unguessable.
				http.NotFound(w, r)
				return
			}
		}
	}

	now := time.Now()
	window := windowDuration(f.Window)
	since := now.Add(-window)
	var until time.Time
	// An explicit from/to (a chart drill-through) overrides the window.
	if from, ok := parseUnixParam(f.From); ok {
		since = from
		if to, ok2 := parseUnixParam(f.To); ok2 {
			until = to
		}
	}

	filter := models.InsightFilter{
		From:     since,
		To:       until,
		Host:     f.Host,
		Status:   f.Status,
		Path:     f.Path,
		ClientIP: f.ClientIP,
		UASearch: f.UA,
		ServerID: serverScopeID,
	}

	var (
		rows   []models.RequestRow
		total  int64
		facets []models.HostFacet
		mix    models.StatusBuckets
		errMsg string
	)
	if f.any() {
		var err error
		if rows, err = models.SearchRequests(s.DB, filter, 200); err != nil {
			errMsg = "Could not read requests: " + err.Error()
		}
		total, _ = models.SearchRequestCount(s.DB, filter)
		facets, _ = models.SearchHostFacets(s.DB, filter, 12)
		mix, _ = models.SearchStatusMix(s.DB, filter)
		// A viewer's unscoped search must not list other tenants' hosts.
		if !isAdmin && f.Host == "" {
			rows = filterRowsToHosts(rows, allowedHosts)
			facets = filterFacetsToHosts(facets, allowedHosts)
		}
	}

	allServers, _ := models.ListCaddyServers(s.DB)
	s.render(w, r, "analytics_requests.html", map[string]any{
		"User":             u,
		"IsAdmin":          isAdmin,
		"Filter":           f,
		"HasFilter":        f.any(),
		"Rows":             rows,
		"RowCount":         len(rows),
		"Total":            total,
		"Truncated":        total > int64(len(rows)),
		"Facets":           facets,
		"StatusMix":        mix,
		"Error":            errMsg,
		"WindowLabel":      windowLabelFor(f.Window),
		"ExplicitRange":    f.From != "",
		"RangeFrom":        since,
		"RangeTo":          until,
		"AllServers":       allServers,
		"SelectedServerID": serverScopeID,
		"Section":          "analytics",
	})
}

// windowLabelFor is the human label for a window token.
func windowLabelFor(token string) string {
	switch token {
	case "1h":
		return "last hour"
	case "6h":
		return "last 6 hours"
	case "1d", "24h", "day":
		return "last 24 hours"
	case "30d":
		return "last 30 days"
	default:
		return "last 7 days"
	}
}

// filterRowsToHosts drops rows for hosts the caller may not see. Belt and
// braces: the SQL is already scoped by host when one is given, but an
// unscoped search has no host to scope by.
func filterRowsToHosts(rows []models.RequestRow, allowed []string) []models.RequestRow {
	if allowed == nil {
		return rows
	}
	set := make(map[string]bool, len(allowed))
	for _, h := range allowed {
		set[h] = true
	}
	out := rows[:0]
	for _, r := range rows {
		if set[r.Host] {
			out = append(out, r)
		}
	}
	return out
}

func filterFacetsToHosts(facets []models.HostFacet, allowed []string) []models.HostFacet {
	if allowed == nil {
		return facets
	}
	set := make(map[string]bool, len(allowed))
	for _, h := range allowed {
		set[h] = true
	}
	out := facets[:0]
	for _, f := range facets {
		if set[f.Host] {
			out = append(out, f)
		}
	}
	return out
}
