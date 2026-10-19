// SPDX-License-Identifier: Apache-2.0
package web

import (
	"encoding/json"
	"regexp"
	"testing"
)

var sourceTranslationCall = regexp.MustCompile(`\{\{t \$ "([a-z0-9_.]+)"\}\}|caddyuiT\(['"]([a-z0-9_.]+)['"]\)`)

// englishTemplateSource resolves literal catalog references for source-level
// regression checks. It does not execute templates or JavaScript: structural
// markers and control flow remain exactly as authored. Runtime rendering and
// escaping are covered separately by internal/server's HTTP tests.
func englishTemplateSource(t *testing.T, source []byte) string {
	t.Helper()
	raw, err := FS.ReadFile("i18n/en.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog map[string]string
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	return sourceTranslationCall.ReplaceAllStringFunc(string(source), func(call string) string {
		match := sourceTranslationCall.FindStringSubmatch(call)
		key := match[1] + match[2]
		text, ok := catalog[key]
		if !ok {
			t.Fatalf("source uses missing English translation %q", key)
		}
		return text
	})
}
