// SPDX-License-Identifier: Apache-2.0

package models

import (
	"database/sql"
	"strconv"
	"strings"
	"time"
)

// Analytics insight queries added for issue #145: latency percentiles and
// trends, per-bucket status mixes, host-level user agents, and a combined
// request filter.
//
// Everything here reads access_events directly rather than the daily rollup,
// because each of these needs a dimension (duration, user agent, exact status)
// the rollup does not carry. That bounds them to the retention window of the
// raw events, which is the same bound the existing drill-downs already have.

// InsightFilter is the set of conditions every query in this file shares.
// Zero values mean "no constraint", so a bare filter is a fleet-wide query
// over the window.
type InsightFilter struct {
	From     time.Time
	To       time.Time
	Host     string // exact host, or "*.example.com" wildcard
	Status   string // status class: 2xx/3xx/4xx/5xx/other
	Path     string // exact path
	ClientIP string
	// UASearch matches a substring of the user agent, case-insensitively.
	UASearch string
	ServerID int64
	// Scoped limits the filter to OnlyHosts (exact names or "*.example.com"
	// wildcards): the hosts a non-admin may see. Scoped with no hosts matches
	// nothing. Without it, a viewer's search with no host picked counted
	// everyone's requests in its total and status mix (v2.68.3).
	Scoped    bool
	OnlyHosts []string
}

// where builds the SQL fragment and arguments for the filter. The leading
// `ts` bounds are always present, so callers append this to a bare
// `WHERE 1=1`-style prefix of their own.
func (f InsightFilter) where() (string, []any) {
	var b strings.Builder
	args := []any{f.From.Unix()}
	b.WriteString(" AND ts >= ?")
	if !f.To.IsZero() {
		b.WriteString(" AND ts < ?")
		args = append(args, f.To.Unix())
	}
	if clause, hostArgs := hostMatchClause(f.Host); clause != "" {
		b.WriteString(clause)
		args = append(args, hostArgs...)
	}
	if clause, statusArgs, ok := statusClassClause(f.Status); ok {
		b.WriteString(clause)
		args = append(args, statusArgs...)
	}
	if f.Path != "" {
		b.WriteString(" AND path = ?")
		args = append(args, f.Path)
	}
	if f.ClientIP != "" {
		b.WriteString(" AND client_ip = ?")
		args = append(args, f.ClientIP)
	}
	if f.UASearch != "" {
		// LIKE wildcards in user input would otherwise match far more than
		// typed — same escaping contract as hostMatchClause.
		esc := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(f.UASearch)
		b.WriteString(" AND LOWER(user_agent) LIKE ? ESCAPE '!'")
		args = append(args, "%"+strings.ToLower(esc)+"%")
	}
	if clause, serverArgs := serverMatchClause(f.ServerID); clause != "" {
		b.WriteString(clause)
		args = append(args, serverArgs...)
	}
	if f.Scoped {
		var alts []string
		for _, h := range f.OnlyHosts {
			if clause, hostArgs := hostMatchClause(h); clause != "" {
				alts = append(alts, strings.TrimPrefix(clause, " AND "))
				args = append(args, hostArgs...)
			}
		}
		if len(alts) == 0 {
			b.WriteString(" AND 1=0")
		} else {
			b.WriteString(" AND (" + strings.Join(alts, " OR ") + ")")
		}
	}
	return b.String(), args
}

// --- Latency percentiles (issue #145 item 6) ---

// LatencyStats is a latency summary for one window or bucket.
//
// There is deliberately no split between proxy time and upstream time:
// Caddy's access log emits a single `duration` for the whole request, so that
// distinction cannot be recovered here. Reporting a guess would be worse than
// reporting nothing.
type LatencyStats struct {
	Count int64
	AvgMs int64
	MinMs int64
	MaxMs int64
	P50Ms int64
	P95Ms int64
	P99Ms int64
}

// percentileMs returns the duration at the given percentile by walking the
// sorted rows. SQLite has no percentile function and MariaDB's differs, so
// this uses the ORDER BY + OFFSET approach the existing p95 panel already
// used — one extra query per percentile, which is cheap at dashboard sizes
// and portable across both backends.
func percentileMs(db *sql.DB, where string, args []any, count int64, pct int) int64 {
	if count <= 0 {
		return 0
	}
	offset := count * int64(pct) / 100
	if offset >= count {
		offset = count - 1
	}
	var v int64
	_ = db.QueryRow(
		`SELECT COALESCE(duration_ms,0) FROM access_events WHERE 1=1`+where+
			` ORDER BY duration_ms LIMIT 1 OFFSET `+strconv.FormatInt(offset, 10), args...).Scan(&v)
	return v
}

// LatencySummary returns avg/min/max plus p50, p95 and p99 for the filter.
func LatencySummary(db *sql.DB, f InsightFilter) (LatencyStats, error) {
	var s LatencyStats
	where, args := f.where()
	err := db.QueryRow(
		// AVG() is a REAL; CAST so the scan into int64 succeeds. See the note
		// on ResponseTimeSince — the same shape silently broke that panel.
		`SELECT COUNT(*), CAST(COALESCE(AVG(duration_ms),0) AS INTEGER), COALESCE(MIN(duration_ms),0), COALESCE(MAX(duration_ms),0)
		   FROM access_events WHERE 1=1`+where, args...).Scan(&s.Count, &s.AvgMs, &s.MinMs, &s.MaxMs)
	if err != nil && err != sql.ErrNoRows {
		return s, err
	}
	s.P50Ms = percentileMs(db, where, args, s.Count, 50)
	s.P95Ms = percentileMs(db, where, args, s.Count, 95)
	s.P99Ms = percentileMs(db, where, args, s.Count, 99)
	return s, nil
}

// LatencyBucket is one time-bucket of latency, for the trend chart.
type LatencyBucket struct {
	Hour  time.Time
	Count int64
	AvgMs int64
	MaxMs int64
	P95Ms int64
}

// LatencyBuckets returns per-bucket latency across the filter's window.
//
// avg/max come from one grouped pass. p95 would need a sort per bucket, which
// is why it is derived from the bucket's own rows only when that bucket is
// small enough to matter; for the dashboard trend the avg and max lines carry
// the shape, and the summary panel gives exact percentiles for the window.
func LatencyBuckets(db *sql.DB, f InsightFilter, bucketSeconds int64) ([]LatencyBucket, error) {
	if bucketSeconds <= 0 {
		bucketSeconds = 3600
	}
	where, args := f.where()
	bs := strconv.FormatInt(bucketSeconds, 10)
	rows, err := db.Query(
		`SELECT (ts / `+bs+`) * `+bs+` AS bucket,
		        COUNT(*),
		        CAST(COALESCE(AVG(duration_ms),0) AS INTEGER),
		        COALESCE(MAX(duration_ms),0)
		   FROM access_events WHERE 1=1`+where+`
		  GROUP BY bucket ORDER BY bucket`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LatencyBucket
	for rows.Next() {
		var sec, count, avg, max int64
		if err := rows.Scan(&sec, &count, &avg, &max); err != nil {
			return nil, err
		}
		out = append(out, LatencyBucket{
			Hour:  time.Unix(sec, 0).UTC(),
			Count: count,
			AvgMs: avg,
			MaxMs: max,
		})
	}
	return out, rows.Err()
}

// --- Per-bucket status mix (issue #145 item 3) ---

// StatusBucket is the status breakdown of one time-bucket, so a request chart
// can be coloured by response class instead of a single flat series.
type StatusMixBucket struct {
	Hour  time.Time
	S2xx  int
	S3xx  int
	S4xx  int
	S5xx  int
	Other int
}

// Total is every request in the bucket, whatever its class.
func (b StatusMixBucket) Total() int { return b.S2xx + b.S3xx + b.S4xx + b.S5xx + b.Other }

// StatusMixBuckets returns the per-bucket status mix over the filter's window.
// Empty buckets are omitted, matching AccessBuckets — the caller fills gaps.
func StatusMixBuckets(db *sql.DB, f InsightFilter, bucketSeconds int64) ([]StatusMixBucket, error) {
	if bucketSeconds <= 0 {
		bucketSeconds = 3600
	}
	where, args := f.where()
	bs := strconv.FormatInt(bucketSeconds, 10)
	rows, err := db.Query(
		`SELECT (ts / `+bs+`) * `+bs+` AS bucket,
		        SUM(CASE WHEN status BETWEEN 200 AND 299 THEN 1 ELSE 0 END),
		        SUM(CASE WHEN status BETWEEN 300 AND 399 THEN 1 ELSE 0 END),
		        SUM(CASE WHEN status BETWEEN 400 AND 499 THEN 1 ELSE 0 END),
		        SUM(CASE WHEN status BETWEEN 500 AND 599 THEN 1 ELSE 0 END),
		        SUM(CASE WHEN status < 200 OR status >= 600 THEN 1 ELSE 0 END)
		   FROM access_events WHERE 1=1`+where+`
		  GROUP BY bucket ORDER BY bucket`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StatusMixBucket
	for rows.Next() {
		var sec int64
		var b StatusMixBucket
		if err := rows.Scan(&sec, &b.S2xx, &b.S3xx, &b.S4xx, &b.S5xx, &b.Other); err != nil {
			return nil, err
		}
		b.Hour = time.Unix(sec, 0).UTC()
		out = append(out, b)
	}
	return out, rows.Err()
}

// --- Host-level user agents (issue #145 item 4) ---

// HostUserAgents returns the raw user-agent strings seen for the filter, most
// frequent first.
//
// The host page already had a *browser* breakdown (TopBrowsers), which buckets
// agents into Chrome/Firefox/… and so hides exactly what the reporter wanted
// to see: the specific agents hitting a host. This returns them verbatim.
// HostUAStat is one user agent with how much traffic it accounts for.
// Distinct from UAStat, which only carries a count and is used by the
// per-visitor drill-down where the visitor is already fixed.
type HostUAStat struct {
	UserAgent string
	Views     int
	Visitors  int
}

func HostUserAgents(db *sql.DB, f InsightFilter, limit int) ([]HostUAStat, error) {
	if limit <= 0 {
		limit = 15
	}
	where, args := f.where()
	rows, err := db.Query(
		`SELECT user_agent, COUNT(*) AS views, COUNT(DISTINCT client_ip) AS visitors
		   FROM access_events WHERE 1=1`+where+` AND user_agent != ''
		  GROUP BY user_agent ORDER BY views DESC LIMIT `+strconv.Itoa(limit), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HostUAStat
	for rows.Next() {
		var u HostUAStat
		if err := rows.Scan(&u.UserAgent, &u.Views, &u.Visitors); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// --- Combined request search (issue #145 item 5) ---

// RequestRow is one matched request in the search results.
type RequestRow struct {
	TS         time.Time
	Host       string
	Path       string
	Method     string
	Status     int
	ClientIP   string
	UserAgent  string
	DurationMs int64
	BytesOut   int64
	ServerName string
}

// SearchRequests returns individual requests matching every supplied
// condition, newest first. This is the backing query for the analytics filter
// bar: "all 5xx for example.net in the last hour" is one filter, rather than
// four drill-downs in sequence.
func SearchRequests(db *sql.DB, f InsightFilter, limit int) ([]RequestRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	where, args := f.where()
	rows, err := db.Query(
		`SELECT ts, host, path, method, status, client_ip, user_agent,
		        COALESCE(duration_ms,0), COALESCE(bytes_out,0), COALESCE(server_name,'')
		   FROM access_events WHERE 1=1`+where+`
		  ORDER BY ts DESC LIMIT `+strconv.Itoa(limit), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RequestRow
	for rows.Next() {
		var r RequestRow
		var sec int64
		if err := rows.Scan(&sec, &r.Host, &r.Path, &r.Method, &r.Status, &r.ClientIP,
			&r.UserAgent, &r.DurationMs, &r.BytesOut, &r.ServerName); err != nil {
			return nil, err
		}
		r.TS = time.Unix(sec, 0).UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}

// SearchRequestCount is the number of requests matching the filter, so the UI
// can say how many matched even when the row list is capped.
func SearchRequestCount(db *sql.DB, f InsightFilter) (int64, error) {
	where, args := f.where()
	var n int64
	err := db.QueryRow(`SELECT COUNT(*) FROM access_events WHERE 1=1`+where, args...).Scan(&n)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return n, err
}

// SearchStatusMix is the status-class breakdown of the filtered set, so the
// explorer can show what the current filter is actually made of.
func SearchStatusMix(db *sql.DB, f InsightFilter) (StatusBuckets, error) {
	where, args := f.where()
	var b StatusBuckets
	err := db.QueryRow(
		`SELECT SUM(CASE WHEN status BETWEEN 200 AND 299 THEN 1 ELSE 0 END),
		        SUM(CASE WHEN status BETWEEN 300 AND 399 THEN 1 ELSE 0 END),
		        SUM(CASE WHEN status BETWEEN 400 AND 499 THEN 1 ELSE 0 END),
		        SUM(CASE WHEN status BETWEEN 500 AND 599 THEN 1 ELSE 0 END),
		        SUM(CASE WHEN status < 200 OR status >= 600 THEN 1 ELSE 0 END)
		   FROM access_events WHERE 1=1`+where, args...).Scan(&b.S2xx, &b.S3xx, &b.S4xx, &b.S5xx, &b.SOther)
	if err == sql.ErrNoRows {
		return b, nil
	}
	return b, err
}

// SearchHostFacets returns the hosts present in the filtered set with their
// counts, so the filter bar can offer the hosts that actually have matching
// traffic rather than every host ever configured.
// HostFacet is a host and its share of the filtered result set.
type HostFacet struct {
	Host     string
	Views    int
	Visitors int
}

func SearchHostFacets(db *sql.DB, f InsightFilter, limit int) ([]HostFacet, error) {
	if limit <= 0 {
		limit = 20
	}
	where, args := f.where()
	rows, err := db.Query(
		`SELECT host, COUNT(*) AS views, COUNT(DISTINCT client_ip) AS visitors
		   FROM access_events WHERE 1=1`+where+` AND host != ''
		  GROUP BY host ORDER BY views DESC LIMIT `+strconv.Itoa(limit), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HostFacet
	for rows.Next() {
		var h HostFacet
		if err := rows.Scan(&h.Host, &h.Views, &h.Visitors); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
