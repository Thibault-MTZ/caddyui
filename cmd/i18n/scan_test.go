// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"html"
	"html/template"
	"reflect"
	"strings"
	"testing"
)

func scanFixture(t *testing.T, source string) *scanner {
	t.Helper()
	s := &scanner{source: source, file: "example.html", known: map[string]string{}, entries: map[string]string{}}
	s.scan()
	if s.err != nil {
		t.Fatal(s.err)
	}
	return s
}

func TestScannerPreservesGoActionsCommentsAndTechnicalBlocks(t *testing.T) {
	source := `{{define "content"}}{{/* ignored <b>Delete everything</b> }} */}}
<!-- Do not translate this comment -->
<p>Click <strong>Save</strong> to continue.</p>
<p>{{printf "}}>" .Name}}</p>
<pre><code>reverse_proxy localhost:8080 { header_up Host example.org }</code></pre>
<textarea name="config">respond /health "OK"</textarea>
<input name="host" value="example.org" placeholder="example.org" title="Enter a host" data-url="/proxy-hosts" />
<script>const html = '<div>Technical HTML</div>'; const method = 'POST'; el.textContent = 'Copied!';</script>
<style>.example::after { content: 'Do not translate'; }</style>{{end}}`
	s := scanFixture(t, source)
	out := s.apply()
	for _, unchanged := range []string{`{{printf "}}>" .Name}}`, `{{/* ignored <b>Delete everything</b> }} */}}`, `<!-- Do not translate this comment -->`, `<strong>`, `</strong>`, `reverse_proxy localhost:8080 { header_up Host example.org }`, `respond /health "OK"`, `name="host" value="example.org" placeholder="example.org"`, `data-url="/proxy-hosts"`, `const method = 'POST'`, `content: 'Do not translate'`} {
		if !strings.Contains(out, unchanged) {
			t.Errorf("changed protected bytes %q in %s", unchanged, out)
		}
	}
	if !strings.Contains(out, "el.textContent = caddyuiT('js.example.") {
		t.Error("textContent literal was not extracted")
	}
	for _, v := range s.entries {
		if strings.ContainsAny(v, "<>") || strings.Contains(v, "reverse_proxy") || strings.Contains(v, "Delete everything") {
			t.Errorf("technical content entered catalog: %q", v)
		}
	}
}

func TestRichTextEntitiesWhitespaceAndEscapingRenderTheSame(t *testing.T) {
	source := `<p>  A &amp; B <strong title="Don&#39;t &quot;escape&quot; twice">Save</strong> to continue.
{{.Name}}  </p>`
	s := scanFixture(t, source)
	funcs := template.FuncMap{"t": func(_ any, key string) string { return s.entries[key] }}
	render := func(text string) string {
		tpl, err := template.New("fixture").Funcs(funcs).Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := tpl.Execute(&out, map[string]any{"Name": `<script>alert("x")</script>`}); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	got, want := render(s.apply()), render(source)
	// html/template may canonicalize equivalent entities, e.g. &quot; to
	// &#34;. Compare decoded characters without weakening the escape check.
	if html.UnescapeString(got) != html.UnescapeString(want) {
		t.Errorf("render changed\ngot:  %s\nwant: %s", got, want)
	}
	if !strings.Contains(got, "&lt;script&gt;") || strings.Contains(got, "<script>") {
		t.Error("interpolated data lost HTML escaping")
	}
	for _, v := range s.entries {
		if strings.Contains(v, "&amp;") || strings.Contains(v, "&#39;") {
			t.Errorf("double escaped catalog text: %s", v)
		}
	}
}

func TestExtractionIsDeterministicAndIdempotent(t *testing.T) {
	source := `<p>Save changes</p><p>Save changes!</p><p>Save changes</p><input title="A title {{.Name}}" />`
	a, b := scanFixture(t, source), scanFixture(t, source)
	if a.apply() != b.apply() || !reflect.DeepEqual(a.entries, b.entries) {
		t.Error("nondeterministic extraction")
	}
	// Punctuation changes are disambiguated, while repeated text shares a key.
	if len(a.entries) != 3 {
		t.Errorf("got %d entries, want 3", len(a.entries))
	}
	again := scanFixture(t, a.apply())
	if len(again.edits) != 0 || again.apply() != a.apply() {
		t.Error("second extraction changed the source")
	}
}

func TestJavaScriptDoesNotTranslateProtocolComparisonsOrUnsafeHTML(t *testing.T) {
	source := `<button onclick="if (state === 'running') alert('Ready'); el.innerHTML = '<b>Error</b>'; confirm('Remove {{.Name}}?');">Remove</button>
<script>// alert('A comment');
const state = 'running'; el.textContent = state === 'running' ? 'Running' : 'Stopped'; el.innerText = 'No results';
const html = '<a href="/path">Link</a>';</script>`
	s := scanFixture(t, source)
	out := s.apply()
	for _, want := range []string{`state === 'running'`, `value1: {{.Name}}`, `// alert('A comment');`, `el.innerHTML = '<b>Error</b>'`} {
		if !strings.Contains(out, want) {
			t.Errorf("changed technical/complex source %q", want)
		}
	}
	for _, v := range s.entries {
		if v == "running" || strings.Contains(v, "<b>") || strings.Contains(v, "{{") {
			t.Errorf("unsafe JS entry %q", v)
		}
	}
	for _, want := range []string{"Ready", "No results", "Remove"} {
		found := false
		for _, v := range s.entries {
			if v == want {
				found = true
			}
		}
		if !found {
			t.Errorf("missing UI literal %q", want)
		}
	}
	placeholder := false
	for _, f := range s.findings {
		if f.Disposition == "extract" && strings.Contains(f.Text, "Remove {value1}") {
			placeholder = true
		}
	}
	if !placeholder {
		t.Error("simple Go/JS interpolation was not extracted as a placeholder")
	}
}

func TestHTMLJavaScriptLabelsKeepMarkupAndUseEscaping(t *testing.T) {
	s := scanFixture(t, `<script>row.innerHTML = '<input name="header" placeholder="Value to send"><b>Not available</b>';</script>`)
	out := s.apply()
	for _, want := range []string{`name=\"header\"`, `caddyuiEscapeHTML(caddyuiT('js.example.`, `Not available`} {
		if want == "Not available" {
			if strings.Contains(out, want) {
				t.Error("HTML label was not extracted")
			}
			continue
		}
		if !strings.Contains(out, want) {
			t.Errorf("missing protected markup/escaping %q in %s", want, out)
		}
	}
	for _, value := range s.entries {
		if strings.ContainsAny(value, "<>") {
			t.Errorf("HTML entered catalog: %q", value)
		}
	}
}

func TestJavaScriptRegexAndNestedTemplatesDoNotDesynchronizeLexer(t *testing.T) {
	source := "<script>const esc = /[&<>\"']/g; const html = `<p>${x ? `nested ${name}` : 'Other'}</p>`; el.textContent = 'Ready';</script>"
	s := scanFixture(t, source)
	out := s.apply()
	if !strings.Contains(out, "/[&<>\"']/g") || !strings.Contains(out, "`<p>${x ? `nested ${name}` : 'Other'}</p>`") {
		t.Error("changed regex or nested HTML template")
	}
	if !strings.Contains(out, "el.textContent = caddyuiT(") {
		t.Error("lost text after regex/template literal")
	}
	for _, f := range s.findings {
		if strings.Contains(f.Reason, "unparsed") {
			t.Errorf("lexer lost synchronization: %#v", f)
		}
	}
}

func TestJavaScriptGoPlaceholderUsesContextualEscaping(t *testing.T) {
	s := scanFixture(t, `<form onsubmit="return confirm('Delete {{.Name}}?')"></form>`)
	tpl, err := template.New("fixture").Parse(s.apply())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := tpl.Execute(&out, map[string]string{"Name": `');alert(1);//<img src=x>`}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "<img") || strings.Contains(out.String(), "value1: &#39;);alert") {
		t.Errorf("unsafe interpolation: %s", out.String())
	}
	if !strings.Contains(out.String(), `value1:`) || !strings.Contains(out.String(), `\u003c`) {
		t.Errorf("expected JSON escaping in JS context: %s", out.String())
	}
}

func TestTechnicalPlaceholderExamplesStayUnchanged(t *testing.T) {
	for _, value := range []string{"example.com, *.example.com", "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----", "example.org {\n reverse_proxy localhost:8080\n}", "wildcard-example-com"} {
		source := `<textarea placeholder="` + value + `"></textarea>`
		s := scanFixture(t, source)
		if s.apply() != source || len(s.entries) > 0 {
			t.Errorf("changed technical example %q", value)
		}
	}
}

func TestMalformedInputFailsBeforeExtraction(t *testing.T) {
	for _, source := range []string{`<p>{{printf "unfinished"`, `<input title="unfinished>`, `<!-- unfinished`, `<script>const a = 1;`} {
		s := &scanner{source: source, file: "bad.html", known: map[string]string{}, entries: map[string]string{}}
		s.scan()
		if s.err == nil {
			t.Errorf("accepted malformed input %q", source)
		}
	}
}

func TestSentenceEndingWordIsNotAFileName(t *testing.T) {
	for _, text := range []string{"image.", "disabled.", "Warning:"} {
		if !humanText(text) {
			t.Errorf("visible word incorrectly excluded: %q", text)
		}
	}
	for _, text := range []string{"config.json", "proxy.example.com.", "localhost:2019"} {
		if humanText(text) {
			t.Errorf("technical identifier considered prose: %q", text)
		}
	}
}
