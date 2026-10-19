// SPDX-License-Identifier: Apache-2.0

package main

import "strings"

// These lexical helpers skip JavaScript code without evaluating it. Their
// only job is keeping quotes in regexes/nested template literals out of the
// plain-string extractor; complex templates remain manual-review findings.
func regexStart(source string, start, at int) bool {
	prefix := strings.TrimSpace(source[start:at])
	if prefix == "" {
		return true
	}
	last := prefix[len(prefix)-1]
	return strings.ContainsRune("(=,:![{;?&|", rune(last)) || strings.HasSuffix(prefix, "return") || strings.HasSuffix(prefix, "=>")
}
func regexEnd(source string, start, end int) (int, bool) {
	inClass := false
	for i := start + 1; i < end; i++ {
		if source[i] == '\\' {
			i++
			continue
		}
		if source[i] == '[' {
			inClass = true
		}
		if source[i] == ']' {
			inClass = false
		}
		if source[i] == '\n' || source[i] == '\r' {
			return 0, false
		}
		if source[i] == '/' && !inClass {
			i++
			for i < end && asciiLetter(source[i]) {
				i++
			}
			return i, true
		}
	}
	return 0, false
}
func templateLiteralEnd(source string, start, end int) (int, bool) {
	for i := start + 1; i < end; i++ {
		if source[i] == '\\' {
			i++
			continue
		}
		if strings.HasPrefix(source[i:], "{{") {
			j, err := actionEnd(source, i)
			if err != nil {
				return 0, false
			}
			i = j - 1
			continue
		}
		if source[i] == '`' {
			return i + 1, true
		}
		if strings.HasPrefix(source[i:], "${") {
			j, ok := jsExpressionEnd(source, i+2, end)
			if !ok {
				return 0, false
			}
			i = j - 1
		}
	}
	return 0, false
}
func jsExpressionEnd(source string, start, end int) (int, bool) {
	depth := 1
	for i := start; i < end; i++ {
		if strings.HasPrefix(source[i:], "{{") {
			j, err := actionEnd(source, i)
			if err != nil {
				return 0, false
			}
			i = j - 1
			continue
		}
		if strings.HasPrefix(source[i:], "//") {
			j := strings.IndexByte(source[i:end], '\n')
			if j < 0 {
				return 0, false
			}
			i += j
			continue
		}
		if strings.HasPrefix(source[i:], "/*") {
			j := strings.Index(source[i+2:end], "*/")
			if j < 0 {
				return 0, false
			}
			i += 2 + j + 1
			continue
		}
		if source[i] == '`' {
			j, ok := templateLiteralEnd(source, i, end)
			if !ok {
				return 0, false
			}
			i = j - 1
			continue
		}
		if source[i] == '\'' || source[i] == '"' {
			quote := source[i]
			i++
			for i < end {
				if source[i] == '\\' {
					i += 2
					continue
				}
				if source[i] == quote {
					break
				}
				i++
			}
			if i >= end {
				return 0, false
			}
			continue
		}
		if source[i] == '/' && regexStart(source, start, i) {
			if j, ok := regexEnd(source, i, end); ok {
				i = j - 1
				continue
			}
		}
		if source[i] == '{' {
			depth++
		}
		if source[i] == '}' {
			depth--
			if depth == 0 {
				return i + 1, true
			}
		}
	}
	return 0, false
}
