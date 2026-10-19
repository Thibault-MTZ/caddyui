// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strconv"
	"strings"
)

// A complete HTML literal can keep its markup in JavaScript while its plain
// text/label attributes become escaped caddyuiT calls. Incomplete concatenated
// attribute fragments remain manual: guessing their context is unsafe.
func (s *scanner) htmlJSFragment(start, end int, text string) bool {
	if !strings.Contains(text, "<") {
		return false
	}
	known := map[string]string{}
	for k, v := range s.known {
		known[k] = v
	}
	for k, v := range s.entries {
		known[k] = v
	}
	nested := scanner{source: text, file: s.file, known: known, entries: map[string]string{}, jsHTML: true}
	nested.scan()
	if nested.err != nil || len(nested.edits) == 0 {
		return false
	}
	for _, finding := range nested.findings {
		if finding.Kind == "js" && finding.Disposition == "extract" {
			return false
		}
	}
	var chunks []string
	last := 0
	for _, e := range nested.edits {
		chunks = append(chunks, strconv.Quote(text[last:e.start]), e.replacement)
		last = e.end
	}
	chunks = append(chunks, strconv.Quote(text[last:]))
	for k, v := range nested.entries {
		s.entries[k] = v
	}
	s.edits = append(s.edits, edit{start, end, "(" + strings.Join(chunks, " + ") + ")"})
	for _, f := range nested.findings {
		if f.Disposition == "extract" {
			f.Line = s.line(start)
			f.Kind = "js.html." + f.Kind
			f.Reason = "plain text escaped before HTML insertion"
			s.findings = append(s.findings, f)
		}
	}
	return true
}
