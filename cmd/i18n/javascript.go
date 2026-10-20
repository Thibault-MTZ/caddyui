// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"unicode/utf16"
)

// javascript extracts only literals in known text sinks. It does not pretend
// to be a JavaScript AST: unknown expressions are explicitly inventoried for
// human review, and HTML-producing strings are never put in plain catalogs.
func (s *scanner) javascript(start, end int, attributeQuote byte) {
	for i := start; i < end && s.err == nil; {
		if strings.HasPrefix(s.source[i:], "{{") {
			j, err := actionEnd(s.source, i)
			if err != nil {
				s.err = err
				return
			}
			i = j
			continue
		}
		if strings.HasPrefix(s.source[i:], "//") {
			j := strings.IndexByte(s.source[i:end], '\n')
			if j < 0 {
				return
			}
			i += j + 1
			continue
		}
		if strings.HasPrefix(s.source[i:], "/*") {
			j := strings.Index(s.source[i+2:end], "*/")
			if j < 0 {
				s.err = fmt.Errorf("unterminated JavaScript comment")
				return
			}
			i += 2 + j + 2
			continue
		}
		if s.source[i] == '/' && regexStart(s.source, start, i) {
			if j, ok := regexEnd(s.source, i, end); ok {
				i = j
				continue
			}
		}
		quote := s.source[i]
		if quote != '\'' && quote != '"' && quote != '`' {
			i++
			continue
		}
		literalStart := i
		if quote == '`' {
			j, ok := templateLiteralEnd(s.source, i, end)
			if !ok {
				s.record(i, "js", s.source[i:end], "manual", "unparsed JavaScript template literal", "")
				return
			}
			s.record(i, "js", s.source[i+1:j-1], "manual", "template literal requires expression and HTML review", "")
			i = j
			continue
		}
		i++
		for i < end {
			if strings.HasPrefix(s.source[i:], "{{") {
				j, err := actionEnd(s.source, i)
				if err != nil {
					s.err = err
					return
				}
				i = j
				continue
			}
			if s.source[i] == '\\' {
				i += 2
				continue
			}
			if s.source[i] == quote {
				break
			}
			i++
		}
		if i >= end {
			s.record(literalStart, "js", s.source[literalStart:end], "manual", "unparsed JavaScript literal", "")
			return
		}
		literalEnd := i + 1
		if literalEnd < end && (asciiLetter(s.source[literalEnd]) || s.source[literalEnd] == '_') {
			s.record(literalStart, "js", s.source[literalStart:end], "manual", "invalid or unsupported JavaScript quoting", "")
			return
		}
		raw := s.source[literalStart+1 : i]
		i = literalEnd
		text, ok := decodeJS(raw)
		if !ok {
			s.record(literalStart, "js", raw, "manual", "unsupported JavaScript escape", "")
			continue
		}
		if attributeQuote != 0 {
			text = html.UnescapeString(text)
		}
		if !humanText(text) {
			continue
		}
		prefix := strings.TrimSpace(s.source[start:literalStart])
		// Already translated calls must stay byte-for-byte unchanged.
		if strings.HasSuffix(prefix, "caddyuiT(") || strings.HasSuffix(prefix, "caddyuiT (") {
			continue
		}
		if quote == '`' {
			s.record(literalStart, "js", text, "manual", "template literal requires expression review", "")
			continue
		}
		if strings.Contains(text, "<") || strings.Contains(text, ">") {
			if attributeQuote == 0 && s.htmlJSFragment(literalStart, literalEnd, text) {
				continue
			}
			s.record(literalStart, "js", text, "manual", "HTML or comparison-like JavaScript string", "")
			continue
		}
		if !uiSink(prefix) && !s.reviewed[text] && !s.reviewed[strings.TrimSpace(text)] {
			s.record(literalStart, "js", text, "manual", "unclassified JavaScript literal (may be a protocol constant)", "")
			continue
		}
		if strings.Contains(text, "{{") {
			converted, args, ok := goPlaceholders(text)
			if !ok {
				s.record(literalStart, "js", text, "manual", "complex Go interpolation requires review", "")
				continue
			}
			key := s.key("js", converted)
			if s.err != nil {
				return
			}
			keyQuote := "'"
			if attributeQuote == '\'' {
				keyQuote = `"`
			}
			s.edits = append(s.edits, edit{literalStart, literalEnd, "caddyuiT(" + keyQuote + key + keyQuote + ", {" + strings.Join(args, ", ") + "})"})
			s.record(literalStart, "js", converted, "extract", "Go field interpolations become escaped JavaScript values", key)
			continue
		}
		key := s.key("js", text)
		if s.err != nil {
			return
		}
		keyQuote := "'"
		if attributeQuote == '\'' {
			keyQuote = `"`
		}
		s.edits = append(s.edits, edit{literalStart, literalEnd, "caddyuiT(" + keyQuote + key + keyQuote + ")"})
		s.record(literalStart, "js", text, "extract", "", key)
	}
}

func uiSink(prefix string) bool {
	for _, call := range []string{"alert(", "confirm(", "prompt(", "alert (", "confirm (", "prompt ("} {
		if strings.HasSuffix(prefix, call) {
			return true
		}
	}
	// An assignment's direct literal RHS, or a literal concatenated onto that
	// RHS, is text. Comparisons, replace() patterns and object keys are not.
	for _, field := range []string{".textContent", ".innerText", ".title"} {
		at := strings.LastIndex(prefix, field)
		if at < 0 {
			continue
		}
		tail := strings.TrimSpace(prefix[at+len(field):])
		if tail == "=" {
			return true
		}
		if strings.HasPrefix(tail, "=") && !strings.ContainsAny(tail, ";{}") && !strings.Contains(tail, "==") && !strings.Contains(tail, "!=") {
			// Restrict concatenations to a trailing plus and simple identifiers;
			// function argument strings can be patterns rather than UI copy.
			if strings.HasSuffix(tail, "+") && !strings.Contains(tail, "(") {
				return true
			}
		}
		if strings.HasPrefix(tail, "=") && !strings.ContainsAny(tail, ";{}") && (strings.HasSuffix(tail, "?") || strings.HasSuffix(tail, ":") || strings.HasSuffix(tail, "||")) {
			return true
		}
	}
	return false
}

func goPlaceholders(text string) (string, []string, bool) {
	var out strings.Builder
	var args []string
	for i := 0; i < len(text); {
		at := strings.Index(text[i:], "{{")
		if at < 0 {
			out.WriteString(text[i:])
			break
		}
		at += i
		out.WriteString(text[i:at])
		end, err := actionEnd(text, at)
		if err != nil {
			return "", nil, false
		}
		body := strings.TrimSpace(text[at+2 : end-2])
		if body == "" || (body[0] != '.' && body[0] != '$') {
			return "", nil, false
		}
		for _, r := range body {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune(".$_", r)) {
				return "", nil, false
			}
		}
		name := fmt.Sprintf("value%d", len(args)+1)
		out.WriteString("{" + name + "}")
		args = append(args, name+": {{"+body+"}}")
		i = end
	}
	return out.String(), args, true
}

func decodeJS(raw string) (string, bool) {
	var out strings.Builder
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' {
			out.WriteByte(raw[i])
			continue
		}
		i++
		if i >= len(raw) {
			return "", false
		}
		switch raw[i] {
		case '\'', '"', '\\', '/':
			out.WriteByte(raw[i])
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case 'b':
			out.WriteByte('\b')
		case 'f':
			out.WriteByte('\f')
		case '\n': // JavaScript line continuation
		case '\r':
			if i+1 < len(raw) && raw[i+1] == '\n' {
				i++
			}
		case 'x', 'u':
			n := 2
			if raw[i] == 'u' {
				n = 4
			}
			if i+n >= len(raw) {
				return "", false
			}
			v, err := strconv.ParseUint(raw[i+1:i+1+n], 16, 32)
			if err != nil {
				return "", false
			}
			i += n
			r := rune(v)
			if utf16.IsSurrogate(r) {
				return "", false
			}
			out.WriteRune(r)
		default:
			return "", false
		}
	}
	return out.String(), true
}
