// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"fmt"
	"html"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type finding struct {
	File        string `json:"file"`
	Line        int    `json:"line"`
	Kind        string `json:"kind"`
	Text        string `json:"text"`
	Key         string `json:"key,omitempty"`
	Disposition string `json:"disposition"`
	Reason      string `json:"reason,omitempty"`
}
type edit struct {
	start, end  int
	replacement string
}
type element struct {
	name      string
	technical bool
}
type scanner struct {
	source, file   string
	known, entries map[string]string
	findings       []finding
	edits          []edit
	stack          []element
	rawEnd         int
	reviewed       map[string]bool
	jsHTML         bool
	err            error
}

// actionEnd honors strings and comments, so }} or > inside a Go expression
// can never accidentally terminate an HTML text/attribute span.
func actionEnd(s string, start int) (int, error) {
	i := start + 2
	if i < len(s) && s[i] == '-' {
		i++
	}
	for i < len(s) && isSpace(s[i]) {
		i++
	}
	if strings.HasPrefix(s[i:], "/*") {
		end := strings.Index(s[i+2:], "*/")
		if end < 0 {
			return 0, fmt.Errorf("unterminated Go template comment")
		}
		i += 2 + end + 2
		for i < len(s) && isSpace(s[i]) {
			i++
		}
		if i < len(s) && s[i] == '-' {
			i++
		}
		if !strings.HasPrefix(s[i:], "}}") {
			return 0, fmt.Errorf("invalid Go template comment terminator")
		}
		return i + 2, nil
	}
	var quote byte
	for i < len(s) {
		c := s[i]
		if quote != 0 {
			if c == '\\' && quote != '`' {
				i += 2
				continue
			}
			if c == quote {
				quote = 0
			}
			i++
			continue
		}
		if c == '"' || c == '\'' || c == '`' {
			quote = c
			i++
			continue
		}
		if strings.HasPrefix(s[i:], "}}") {
			return i + 2, nil
		}
		i++
	}
	return 0, fmt.Errorf("unterminated Go template action")
}

func (s *scanner) scan() {
	for i := 0; i < len(s.source) && s.err == nil; {
		if strings.HasPrefix(s.source[i:], "{{") {
			end, err := actionEnd(s.source, i)
			if err != nil {
				s.err = err
				return
			}
			i = end
			continue
		}
		if strings.HasPrefix(s.source[i:], "<!--") {
			end := strings.Index(s.source[i+4:], "-->")
			if end < 0 {
				s.err = fmt.Errorf("unterminated HTML comment")
				return
			}
			i += 4 + end + 3
			continue
		}
		if s.source[i] == '<' && i+1 < len(s.source) && (asciiLetter(s.source[i+1]) || s.source[i+1] == '/' || s.source[i+1] == '!') {
			end := s.tagEnd(i)
			if s.err != nil {
				return
			}
			s.tag(i, end)
			i = end
			if s.rawEnd > i {
				i = s.rawEnd
				s.rawEnd = 0
			}
			continue
		}
		end := i + 1
		for end < len(s.source) && s.source[end] != '<' && !strings.HasPrefix(s.source[end:], "{{") {
			end++
		}
		technical := ""
		for _, el := range s.stack {
			if el.technical {
				technical = "technical element <" + el.name + ">"
				break
			}
		}
		if technical == "" {
			s.literal(i, end, "text", true, 0)
		} else {
			s.record(i, "text", s.source[i:end], "preserve", technical, "")
		}
		i = end
	}
}

func (s *scanner) tagEnd(start int) int {
	var quote byte
	for i := start + 1; i < len(s.source); i++ {
		if strings.HasPrefix(s.source[i:], "{{") {
			end, err := actionEnd(s.source, i)
			if err != nil {
				s.err = err
				return len(s.source)
			}
			i = end - 1
			continue
		}
		c := s.source[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if c == '>' {
			return i + 1
		}
	}
	s.err = fmt.Errorf("unterminated HTML tag at line %d", s.line(start))
	return len(s.source)
}

func (s *scanner) tag(start, end int) {
	i := start + 1
	closing := false
	if s.source[i] == '/' {
		closing = true
		i++
	}
	nameStart := i
	for i < end && (asciiLetter(s.source[i]) || (s.source[i] >= '0' && s.source[i] <= '9') || s.source[i] == '-') {
		i++
	}
	name := strings.ToLower(s.source[nameStart:i])
	if name == "" {
		return
	}
	if closing {
		for n := len(s.stack) - 1; n >= 0; n-- {
			if s.stack[n].name == name {
				s.stack = s.stack[:n]
				break
			}
		}
		return
	}
	if name == "script" || name == "style" {
		// Raw elements are handled as one span. Fake HTML inside JavaScript
		// strings or CSS selectors cannot affect our HTML stack.
		closeStart := indexFold(s.source[end:], "</"+name)
		if closeStart < 0 {
			s.err = fmt.Errorf("unclosed <%s>", name)
			return
		}
		if name == "script" {
			s.javascript(end, end+closeStart, 0)
		} else {
			s.record(end, "style", s.source[end:end+closeStart], "preserve", "CSS", "")
		}
		// Mark body bytes for scan() to skip, without rewriting their contents.
		s.rawEnd = end + closeStart
	}
	for i < end-1 {
		if isSpace(s.source[i]) || s.source[i] == '/' {
			i++
			continue
		}
		if strings.HasPrefix(s.source[i:], "{{") {
			j, err := actionEnd(s.source, i)
			if err != nil {
				s.err = err
				return
			}
			i = j
			continue
		}
		a := i
		for i < end-1 && !isSpace(s.source[i]) && s.source[i] != '=' && s.source[i] != '>' {
			i++
		}
		attr := strings.ToLower(s.source[a:i])
		for i < end-1 && isSpace(s.source[i]) {
			i++
		}
		if i >= end-1 || s.source[i] != '=' {
			continue
		}
		i++
		for i < end-1 && isSpace(s.source[i]) {
			i++
		}
		if i >= end-1 {
			break
		}
		quote := byte(0)
		if s.source[i] == '\'' || s.source[i] == '"' {
			quote = s.source[i]
			i++
		}
		valueStart := i
		for i < end-1 {
			if strings.HasPrefix(s.source[i:], "{{") {
				j, err := actionEnd(s.source, i)
				if err != nil {
					s.err = err
					return
				}
				i = j
				continue
			}
			if (quote != 0 && s.source[i] == quote) || (quote == 0 && (isSpace(s.source[i]) || s.source[i] == '>')) {
				break
			}
			i++
		}
		valueEnd := i
		if quote != 0 {
			i++
		}
		if attr == "title" || attr == "placeholder" || attr == "aria-label" || attr == "alt" {
			if quote == 0 {
				s.record(valueStart, "attribute", s.source[valueStart:valueEnd], "manual", "unquoted attribute", "")
				continue
			}
			s.literalSegments(valueStart, valueEnd, "attribute."+attr)
		} else if strings.HasPrefix(attr, "on") {
			s.javascript(valueStart, valueEnd, quote)
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(s.source[start:end-1]), "/") && !voidElement(name) {
		s.stack = append(s.stack, element{name, name == "code" || name == "pre" || name == "textarea" || name == "kbd"})
	}
}

func (s *scanner) literalSegments(start, end int, kind string) {
	for i := start; i < end; {
		if strings.HasPrefix(s.source[i:], "{{") {
			j, err := actionEnd(s.source, i)
			if err != nil {
				s.err = err
				return
			}
			i = j
			continue
		}
		j := i
		for j < end && !strings.HasPrefix(s.source[j:], "{{") {
			j++
		}
		s.literal(i, j, kind, true, 0)
		i = j
	}
}

func (s *scanner) literal(start, end int, kind string, entity bool, quote byte) {
	raw := s.source[start:end]
	text := strings.TrimSpace(raw)
	if text == "" {
		return
	}
	left := strings.Index(raw, text)
	right := left + len(text)
	if entity {
		text = html.UnescapeString(text)
	}
	if kind == "attribute.placeholder" && technicalExample(text) {
		s.record(start, kind, text, "preserve", "technical placeholder example", "")
		return
	}
	if strings.ContainsAny(text, "<>") {
		s.record(start, kind, text, "manual", "text includes a technical angle-bracket example", "")
		return
	}
	if !humanText(text) {
		s.record(start, kind, text, "preserve", "technical token or punctuation", "")
		return
	}
	key := s.key(kind, text)
	if s.err != nil {
		return
	}
	s.record(start, kind, text, "extract", "", key)
	replacement := `{{t $ ` + strconv.Quote(key) + `}}`
	if s.jsHTML {
		replacement = "caddyuiEscapeHTML(caddyuiT('" + key + "'))"
	}
	s.edits = append(s.edits, edit{start + left, start + right, replacement})
}

func (s *scanner) key(kind, text string) string {
	prefix := "page." + slug(strings.TrimSuffix(s.file, filepath.Ext(s.file))) + "."
	if strings.HasPrefix(kind, "js") || s.jsHTML {
		prefix = "js." + slug(strings.TrimSuffix(s.file, filepath.Ext(s.file))) + "."
	} else if strings.HasPrefix(kind, "attribute.") {
		prefix += strings.TrimPrefix(kind, "attribute.") + "."
	}
	h := sha256.Sum256([]byte(text))
	key := fmt.Sprintf("%s%s_%x", prefix, slug(text), h[:4])
	for _, m := range []map[string]string{s.known, s.entries} {
		if old, ok := m[key]; ok && old != text {
			s.err = fmt.Errorf("key collision %s: %q != %q", key, old, text)
			return ""
		}
	}
	if _, ok := s.known[key]; !ok {
		s.entries[key] = text
	}
	return key
}

func (s *scanner) record(start int, kind, text, disposition, reason, key string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	// Technical blocks are summarized in the inventory, not copied in full.
	if disposition == "preserve" && len(text) > 160 {
		text = text[:160] + "…"
	}
	s.findings = append(s.findings, finding{s.file, s.line(start), kind, text, key, disposition, reason})
}
func (s *scanner) line(at int) int { return 1 + strings.Count(s.source[:at], "\n") }
func (s *scanner) apply() string {
	sort.Slice(s.edits, func(i, j int) bool { return s.edits[i].start < s.edits[j].start })
	var out strings.Builder
	last := 0
	for _, e := range s.edits {
		if e.start < last {
			s.err = fmt.Errorf("overlapping edits")
			return s.source
		}
		out.WriteString(s.source[last:e.start])
		out.WriteString(e.replacement)
		last = e.end
	}
	out.WriteString(s.source[last:])
	return out.String()
}
func slug(text string) string {
	var b strings.Builder
	sep := false
	for _, r := range strings.ToLower(text) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if sep && b.Len() > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r)
			sep = false
		} else {
			sep = true
		}
		if b.Len() >= 48 {
			break
		}
	}
	if b.Len() == 0 {
		return "label"
	}
	return b.String()
}
func humanText(text string) bool {
	letters := 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	if letters < 2 {
		return false
	}
	trim := strings.TrimSpace(text)
	labelToken := strings.TrimSuffix(trim, ":")
	if len(labelToken) == 3 && labelToken[0] >= '1' && labelToken[0] <= '5' && labelToken[1:] == "xx" {
		return false
	}
	for _, header := range []string{"Strict-Transport-Security", "Cross-Origin-Opener-Policy", "Cross-Origin-Embedder-Policy", "Cross-Origin-Resource-Policy", "Content-Security-Policy", "Permissions-Policy", "Referrer-Policy", "X-Frame-Options", "X-Content-Type-Options", "X-XSS-Protection", "Cache-Control", "Content-Type", "Alt-Svc", "Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers", "Access-Control-Expose-Headers", "Access-Control-Allow-Credentials", "Access-Control-Max-Age", "X-Request-ID", "X-Request-Start", "X-Real-IP", "X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host", "X-Accel-Buffering", "Set-Cookie", "WWW-Authenticate", "Retry-After", "User-Agent"} {
		if labelToken == header {
			return false
		}
	}
	for _, token := range []string{"CaddyUI", "Caddy", "HTTP", "HTTPS", "HTTP/1.1", "HTTP/2", "HTTP/3", "TLS", "SSL", "TCP", "UDP", "QUIC", "DNS", "ACME", "JSON", "YAML", "PEM", "DER", "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS", "CONNECT", "TRACE", "h2", "h2c", "h3", "tcp", "udp", "gzip", "zstd", "br", "sse", "utf-8", "auto_https", "reverse_proxy"} {
		if labelToken == token {
			return false
		}
	}
	if strings.HasPrefix(trim, "/") || strings.HasPrefix(trim, "http://") || strings.HasPrefix(trim, "https://") || strings.HasPrefix(trim, "*. ") {
		return false
	}
	// Sentence punctuation does not turn a visible word into a file name or
	// protocol identifier ("image." versus "config.json").
	identifier := strings.TrimRight(trim, ".!?;,:")
	if !strings.ContainsAny(trim, " \t\n") && strings.ContainsAny(identifier, "_./:@=\\{}") {
		return false
	}
	return true
}
func asciiLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isSpace(c byte) bool     { return c == ' ' || c == '\n' || c == '\r' || c == '\t' || c == '\f' }
func voidElement(name string) bool {
	switch name {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
		return true
	}
	return false
}
func indexFold(s, sub string) int { return strings.Index(strings.ToLower(s), strings.ToLower(sub)) }

func technicalExample(text string) bool {
	trim := strings.TrimSpace(text)
	for _, sample := range []string{"admin", "secret", "homelab, edge, dmz", "Edge · DMZ · xCave", "production, api, wordpress", "groups", "Richard Applegate", "strict-origin-when-cross-origin", "DENY", "SAMEORIGIN", "camera=(), microphone=()", "geolocation=(), microphone=()", "noindex, nofollow", "GET, POST, PUT, DELETE, PATCH, OPTIONS", "GET,POST,PUT,DELETE", "TRACE,CONNECT", "application/x-www-form-urlencoded, multipart/form-data", ".php, .asp, .aspx", "x25519, p256, p384", "TLS_AES_128_GCM_SHA256, TLS_CHACHA20_POLY1305_SHA256", "sqlmap, nikto, scrapy", "(?i)(curl|python-requests|Go-http-client)", "GNU Terry Pratchett", "6Lc…", "0x4AAAAAAA…"} {
		if trim == sample {
			return true
		}
	}
	for _, prefix := range []string{"default-src ", "SecRuleRemoveById ", "X-", "Content-Type,", "CF-Connecting-IP,", "Remote-User,", "Accept-Encoding,", "Contact: mailto:", "User-agent: *", "HeaderName|", "session_hint=", "no-store  ·"} {
		if strings.HasPrefix(trim, prefix) {
			return true
		}
	}
	if strings.Contains(trim, "\n") && strings.Contains(trim, "{") && strings.Contains(trim, "}") {
		return true
	}
	if strings.HasPrefix(trim, "{") || strings.HasPrefix(trim, "[") || strings.HasPrefix(trim, "-----BEGIN") || strings.HasPrefix(trim, "example.") || strings.HasPrefix(trim, "*.") {
		return true
	}
	if strings.Contains(trim, "\n") && (strings.Contains(trim, "reverse_proxy") || strings.Contains(trim, "file_server") || strings.Contains(trim, "header_up")) {
		return true
	}
	return !strings.ContainsAny(trim, " \t\n") && strings.ContainsAny(trim, ".-:/@\\")
}
