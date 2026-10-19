// SPDX-License-Identifier: Apache-2.0
package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/X4Applegate/caddyui/internal/i18n"
	"github.com/X4Applegate/caddyui/internal/models"
)

func TestServerMessageTranslationKeysExistAndKeepFormats(t *testing.T) {
	en := translations().Catalog("en")
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	used := 0
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".go") || strings.HasSuffix(file.Name(), "_test.go") {
			continue
		}
		tree, err := parser.ParseFile(token.NewFileSet(), file.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(tree, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "tr" {
				return true
			}
			literal, ok := call.Args[1].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			key, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			used++
			if _, ok := en[key]; !ok {
				t.Errorf("%s uses unknown key %s", file.Name(), key)
			}
			return true
		})
	}
	if used < 40 {
		t.Errorf("server scan missed expected message calls: %d", used)
	}
	for _, locale := range translations().Locales() {
		for _, issue := range i18n.CheckCatalog(en, translations().Catalog(locale.Code), false) {
			t.Errorf("%s: %s", locale.Code, issue)
		}
	}
}

func TestLoginErrorsFollowSiteLocaleWithoutChangingAuthentication(t *testing.T) {
	e := newSecEnv(t)
	for _, lang := range []string{"en", "fr"} {
		if err := models.SetSetting(e.db, settingDefaultLocale, lang); err != nil {
			t.Fatal(err)
		}
		rec := e.do(t, "", http.MethodPost, "/login", url.Values{"email": {"alice@t"}, "password": {"wrong-password"}})
		key := "message.server.postlogin.invalid_email_or_password_4d4ff2e3"
		if !strings.Contains(rec.Body.String(), translations().T(lang, key)) {
			t.Errorf("%s login error missing translated text", lang)
		}
		if rec.Header().Get("Location") == "/" {
			t.Error("incorrect password authenticated")
		}
	}
}

// "Disabled" is an adjective in a form and a past-tense verb in activity.
// Exercise real rendered pages so reuse of English text cannot silently apply
// the activity translation to a status or split a title at a gender boundary.
func TestInterfaceTranslationKeepsTitlesAndStatusContext(t *testing.T) {
	e := newSecEnv(t)
	e.do(t, "admin", http.MethodPost, "/profile", url.Values{"action": {"update_locale"}, "locale": {"fr"}})
	for _, tc := range []struct{ path, title string }{
		{"/proxy-hosts/new", "Nouvel hôte proxy"},
		{"/redirection-hosts/new", "Nouvelle redirection"},
		{"/raw-routes/new", "Nouvelle route avancée"},
	} {
		response := e.do(t, "admin", http.MethodGet, tc.path, nil)
		body := response.Body.String()
		if response.Code != http.StatusOK || !strings.Contains(body, "<title>"+tc.title+" · CaddyUI</title>") {
			t.Errorf("%s: status %d, missing grammatical title %q", tc.path, response.Code, tc.title)
		}
		if tc.path == "/proxy-hosts/new" {
			if !strings.Contains(body, ">Désactivé</option>") || strings.Contains(body, ">a désactivé</option>") {
				t.Error("form status inherited the activity verb instead of an adjective")
			}
			if !strings.Contains(body, "Corps de la requête de contrôle de santé") {
				t.Error("health probe request body mislabeled as an expected response body")
			}
		}
	}
}

func TestLocaleChangesKeepFormSubmissionContracts(t *testing.T) {
	e := newSecEnv(t)
	tags := regexp.MustCompile(`(?is)<(?:form|input|select|textarea|button|option)\b[^>]*>`)
	attributes := regexp.MustCompile(`(?:^|\s)(?:name|method|action|type|value)="[^"]*"`)
	scripts := regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>`)
	contract := func(body string) []string {
		var fields []string
		for _, tag := range tags.FindAllString(scripts.ReplaceAllString(body, ""), -1) {
			for _, attr := range attributes.FindAllString(tag, -1) {
				attr = strings.TrimSpace(attr)
				if strings.Contains(tag, `name="csrf_token"`) && strings.HasPrefix(attr, "value=") {
					continue // Session tokens are deliberately not stable page data.
				}
				fields = append(fields, attr)
			}
		}
		sort.Strings(fields)
		return fields
	}
	for _, path := range []string{
		"/proxy-hosts/new", "/redirection-hosts/new", "/raw-routes/new",
		"/layer4-proxies/new", "/middleware-profiles/new", "/certificates/new",
		"/users/new", "/groups/new", "/servers/new",
	} {
		var english []string
		for _, locale := range []string{"en", "fr"} {
			e.do(t, "admin", http.MethodPost, "/profile", url.Values{"action": {"update_locale"}, "locale": {locale}})
			response := e.do(t, "admin", http.MethodGet, path, nil)
			if response.Code != http.StatusOK {
				t.Fatalf("%s (%s): status %d", path, locale, response.Code)
			}
			fields := contract(response.Body.String())
			if len(fields) < 5 {
				t.Fatalf("%s: insufficient form controls found", path)
			}
			if locale == "en" {
				english = fields
			} else if !reflect.DeepEqual(english, fields) {
				t.Errorf("%s: localization changed field names, actions, types or submitted values", path)
			}
		}
	}
}
