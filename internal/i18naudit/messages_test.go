// SPDX-License-Identifier: Apache-2.0
package i18naudit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMessagesKeepTechnicalValuesAndExtractOnlyRequestViews(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "internal", "server")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "form.go")
	source := `package server
func (s *Server) form(w http.ResponseWriter,r *http.Request){
 mode:="external";_ = mode
 renderErr("Name is required")
 renderErr("Remote service failed: " + err.Error())
 renderErr(fmt.Sprintf("Host %s failed (%d)", host, status))
 s.render(w,r,"form.html",map[string]any{"Error":"Passwords do not match","Method":"POST"})
 renderErr(s.tr(r,"existing.key"))
}
func noRequest(){renderErr("Keep this diagnostic unchanged")}
`
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	entries, err := ExtractMessages(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("got %d entries: %+v", len(entries), entries)
	}
	before, _ := os.ReadFile(path)
	if string(before) != source {
		t.Fatal("audit modified source")
	}
	if _, err = ExtractMessages(root, true); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	for _, want := range []string{`mode:="external"`, `"Method":"POST"`, `err.Error()`, `fmt.Sprintf(s.tr(r,`, `s.tr(r,"existing.key")`, `renderErr("Keep this diagnostic unchanged")`} {
		if !strings.Contains(string(after), want) {
			t.Errorf("lost %s", want)
		}
	}
	again, err := ExtractMessages(root, false)
	if err != nil || len(again) != 0 {
		t.Fatalf("not idempotent: %+v, %v", again, err)
	}
}
