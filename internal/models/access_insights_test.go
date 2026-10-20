// SPDX-License-Identifier: Apache-2.0

package models

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// Analytics insight queries for issue #145.

func insightTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE access_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ts INTEGER NOT NULL,
		server_id INTEGER NOT NULL DEFAULT 0,
		server_name TEXT NOT NULL DEFAULT '',
		host TEXT NOT NULL DEFAULT '',
		path TEXT NOT NULL DEFAULT '',
		method TEXT NOT NULL DEFAULT '',
		status INTEGER NOT NULL DEFAULT 0,
		client_ip TEXT NOT NULL DEFAULT '',
		user_agent TEXT NOT NULL DEFAULT '',
		duration_ms INTEGER NOT NULL DEFAULT 0,
		bytes_out INTEGER NOT NULL DEFAULT 0)`); err != nil {
		t.Fatal(err)
	}
	return db
}

func insertEvent(t *testing.T, db *sql.DB, ts time.Time, host string, status int, dur int64, ip, ua string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO access_events (ts, host, path, method, status, client_ip, user_agent, duration_ms, bytes_out)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		ts.Unix(), host, "/", "GET", status, ip, ua, dur, 100); err != nil {
		t.Fatal(err)
	}
}

// Percentiles are the headline of item 6, so they have to be right rather
// than merely plausible. With 1..100 ms the p50/p95/p99 are known exactly.
func TestLatencyPercentiles(t *testing.T) {
	db := insightTestDB(t)
	base := time.Now().Add(-time.Hour)
	for i := 1; i <= 100; i++ {
		insertEvent(t, db, base, "a.example.com", 200, int64(i), "10.0.0.1", "curl")
	}
	got, err := LatencySummary(db, InsightFilter{From: base.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 100 {
		t.Fatalf("count = %d, want 100", got.Count)
	}
	// Index p*count/100 into a sorted 1..100 list: p50 -> index 50 -> 51ms.
	for _, c := range []struct {
		name      string
		got, want int64
	}{
		{"p50", got.P50Ms, 51},
		{"p95", got.P95Ms, 96},
		{"p99", got.P99Ms, 100},
		{"min", got.MinMs, 1},
		{"max", got.MaxMs, 100},
		{"avg", got.AvgMs, 50},
	} {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

// A percentile offset must never run past the last row.
func TestLatencyPercentilesWithOneEvent(t *testing.T) {
	db := insightTestDB(t)
	base := time.Now().Add(-time.Hour)
	insertEvent(t, db, base, "a.example.com", 200, 42, "10.0.0.1", "curl")
	got, err := LatencySummary(db, InsightFilter{From: base.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	for name, v := range map[string]int64{"p50": got.P50Ms, "p95": got.P95Ms, "p99": got.P99Ms} {
		if v != 42 {
			t.Errorf("%s = %d, want 42 — the only sample", name, v)
		}
	}
}

func TestLatencySummaryOnEmptySetIsZeroNotError(t *testing.T) {
	db := insightTestDB(t)
	got, err := LatencySummary(db, InsightFilter{From: time.Now().Add(-time.Hour)})
	if err != nil {
		t.Fatalf("empty window should not error: %v", err)
	}
	if got.Count != 0 || got.P95Ms != 0 {
		t.Errorf("want a zero summary, got %+v", got)
	}
}

func TestStatusMixBucketsSplitByClass(t *testing.T) {
	db := insightTestDB(t)
	base := time.Now().Truncate(time.Hour).Add(-2 * time.Hour)
	for i := 0; i < 3; i++ {
		insertEvent(t, db, base, "a.example.com", 200, 10, "10.0.0.1", "curl")
	}
	insertEvent(t, db, base, "a.example.com", 404, 10, "10.0.0.2", "curl")
	insertEvent(t, db, base, "a.example.com", 500, 10, "10.0.0.3", "curl")
	// A later bucket, so grouping is actually exercised.
	insertEvent(t, db, base.Add(time.Hour), "a.example.com", 301, 10, "10.0.0.4", "curl")

	got, err := StatusMixBuckets(db, InsightFilter{From: base.Add(-time.Minute)}, 3600)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 buckets, got %d: %+v", len(got), got)
	}
	if got[0].S2xx != 3 || got[0].S4xx != 1 || got[0].S5xx != 1 || got[0].Total() != 5 {
		t.Errorf("first bucket = %+v, want 3x2xx 1x4xx 1x5xx", got[0])
	}
	if got[1].S3xx != 1 || got[1].Total() != 1 {
		t.Errorf("second bucket = %+v, want a single 3xx", got[1])
	}
}

func TestHostUserAgentsRanksByViews(t *testing.T) {
	db := insightTestDB(t)
	base := time.Now().Add(-time.Hour)
	for i := 0; i < 5; i++ {
		insertEvent(t, db, base, "a.example.com", 200, 10, "10.0.0.1", "Googlebot/2.1")
	}
	insertEvent(t, db, base, "a.example.com", 200, 10, "10.0.0.2", "curl/8")
	insertEvent(t, db, base, "a.example.com", 200, 10, "10.0.0.3", "curl/8")
	insertEvent(t, db, base, "b.example.com", 200, 10, "10.0.0.9", "OtherHostAgent")

	got, err := HostUserAgents(db, InsightFilter{From: base.Add(-time.Minute), Host: "a.example.com"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 agents for this host, got %d: %+v", len(got), got)
	}
	if got[0].UserAgent != "Googlebot/2.1" || got[0].Views != 5 || got[0].Visitors != 1 {
		t.Errorf("top agent = %+v, want Googlebot 5 views 1 visitor", got[0])
	}
	if got[1].UserAgent != "curl/8" || got[1].Visitors != 2 {
		t.Errorf("second agent = %+v, want curl/8 with 2 distinct visitors", got[1])
	}
}

// The filter is what makes "all 5xx for one host in a window" a single query
// instead of a sequence of drill-downs, so the conditions have to compose.
func TestSearchRequestsCombinesEveryCondition(t *testing.T) {
	db := insightTestDB(t)
	base := time.Now().Truncate(time.Hour).Add(-3 * time.Hour)
	insertEvent(t, db, base, "a.example.com", 500, 10, "10.0.0.1", "curl/8")                  // wanted
	insertEvent(t, db, base, "a.example.com", 200, 10, "10.0.0.1", "curl/8")                  // wrong status
	insertEvent(t, db, base, "b.example.com", 500, 10, "10.0.0.1", "curl/8")                  // wrong host
	insertEvent(t, db, base, "a.example.com", 500, 10, "10.0.0.1", "Mozilla/5.0")             // wrong agent
	insertEvent(t, db, base.Add(5*time.Hour), "a.example.com", 500, 10, "10.0.0.1", "curl/8") // outside window

	f := InsightFilter{
		From:     base.Add(-time.Minute),
		To:       base.Add(time.Hour),
		Host:     "a.example.com",
		Status:   "5xx",
		UASearch: "curl",
	}
	rows, err := SearchRequests(db, f, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want exactly the one matching request, got %d: %+v", len(rows), rows)
	}
	if rows[0].Status != 500 || rows[0].Host != "a.example.com" {
		t.Errorf("wrong row matched: %+v", rows[0])
	}
	n, err := SearchRequestCount(db, f)
	if err != nil || n != 1 {
		t.Errorf("count = %d (err %v), want 1 — must agree with the rows", n, err)
	}
}

// A user agent is user input reaching a LIKE; wildcards in it must be literal.
func TestUASearchEscapesLikeWildcards(t *testing.T) {
	db := insightTestDB(t)
	base := time.Now().Add(-time.Hour)
	insertEvent(t, db, base, "a.example.com", 200, 10, "10.0.0.1", "curl/8")
	insertEvent(t, db, base, "a.example.com", 200, 10, "10.0.0.2", "100%bot")

	f := InsightFilter{From: base.Add(-time.Minute), UASearch: "%"}
	rows, err := SearchRequests(db, f, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].UserAgent != "100%bot" {
		t.Errorf("a literal %% should match only the agent containing one, got %d rows: %+v", len(rows), rows)
	}
}

func TestSearchHostFacetsCountDistinctVisitors(t *testing.T) {
	db := insightTestDB(t)
	base := time.Now().Add(-time.Hour)
	insertEvent(t, db, base, "a.example.com", 200, 10, "10.0.0.1", "curl")
	insertEvent(t, db, base, "a.example.com", 200, 10, "10.0.0.1", "curl")
	insertEvent(t, db, base, "a.example.com", 200, 10, "10.0.0.2", "curl")
	insertEvent(t, db, base, "b.example.com", 200, 10, "10.0.0.3", "curl")

	got, err := SearchHostFacets(db, InsightFilter{From: base.Add(-time.Minute)}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Host != "a.example.com" || got[0].Views != 3 || got[0].Visitors != 2 {
		t.Errorf("facets = %+v, want a.example.com first with 3 views / 2 visitors", got)
	}
}

func TestLatencyBucketsGroupByWindow(t *testing.T) {
	db := insightTestDB(t)
	base := time.Now().Truncate(time.Hour).Add(-2 * time.Hour)
	insertEvent(t, db, base, "a.example.com", 200, 10, "10.0.0.1", "curl")
	insertEvent(t, db, base, "a.example.com", 200, 30, "10.0.0.1", "curl")
	insertEvent(t, db, base.Add(time.Hour), "a.example.com", 200, 100, "10.0.0.1", "curl")

	got, err := LatencyBuckets(db, InsightFilter{From: base.Add(-time.Minute)}, 3600)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 buckets, got %d", len(got))
	}
	if got[0].AvgMs != 20 || got[0].MaxMs != 30 || got[0].Count != 2 {
		t.Errorf("first bucket = %+v, want avg 20 max 30 count 2", got[0])
	}
	if got[1].MaxMs != 100 {
		t.Errorf("second bucket max = %d, want 100", got[1].MaxMs)
	}
}
