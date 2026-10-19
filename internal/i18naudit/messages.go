// SPDX-License-Identifier: Apache-2.0

// Package i18naudit inventories source text without changing protocol values,
// logs, external errors or configuration examples.
package i18naudit

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Message struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Key     string `json:"key"`
	Text    string `json:"text"`
	Context string `json:"context"`
}

var words = regexp.MustCompile(`[A-Za-z][A-Za-z0-9]*`)

func messageKey(file, function, text string) string {
	w := words.FindAllString(text, 6)
	for i := range w {
		w[i] = strings.ToLower(w[i])
	}
	sum := sha256.Sum256([]byte(text))
	return "message." + strings.ToLower(strings.TrimSuffix(filepath.Base(file), ".go")) + "." + strings.ToLower(function) + "." + strings.Join(w, "_") + fmt.Sprintf("_%x", sum[:4])
}

func humanText(s string) bool {
	if len(words.FindAllString(s, -1)) == 0 || strings.TrimSpace(s) == "" {
		return false
	}
	// JSON/HTML fragments, keys, identifiers, addresses and protocol constants
	// require explicit review, not a mechanical replacement.
	if strings.ContainsAny(s, "<>{}") || strings.Contains(s, "://") && !strings.Contains(s, " ") {
		return false
	}
	if !strings.ContainsAny(s, " \t\n") && s == strings.ToLower(s) {
		return false
	}
	return true
}

type replacement struct {
	start, end int
	value      string
}

// ExtractMessages converts only explicitly user-facing expressions in request
// handlers. Unknown expressions stay untouched and can be audited separately.
// The caller owns merging entries into en.json before building the application.
func ExtractMessages(root string, write bool) ([]Message, error) {
	var all []Message
	err := filepath.WalkDir(filepath.Join(root, "internal", "server"), func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		set := token.NewFileSet()
		f, err := parser.ParseFile(set, path, raw, parser.ParseComments)
		if err != nil {
			return err
		}
		var edits []replacement
		seen := map[token.Pos]bool{}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			recommendationBuilder := fn.Name.Name == "buildDashboardRecommendations"
			if fn.Recv == nil && !recommendationBuilder {
				continue
			}
			request, server := "", ""
			for _, field := range fn.Type.Params.List {
				star, ok := field.Type.(*ast.StarExpr)
				if !ok {
					continue
				}
				sel, ok := star.X.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Request" {
					continue
				}
				if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "http" && len(field.Names) == 1 {
					request = field.Names[0].Name
				}
			}
			var receivers []*ast.Field
			if fn.Recv != nil {
				receivers = fn.Recv.List
			}
			for _, field := range receivers {
				star, ok := field.Type.(*ast.StarExpr)
				if !ok {
					continue
				}
				if typ, ok := star.X.(*ast.Ident); ok && typ.Name == "Server" && len(field.Names) == 1 {
					server = field.Names[0].Name
				}
			}
			if (request == "" || server == "") && !recommendationBuilder {
				continue
			}
			prefix := server + ".tr(" + request + ", "
			if recommendationBuilder {
				prefix = "translate("
			}
			var collect func(ast.Expr, string)
			collect = func(expr ast.Expr, context string) {
				if expr == nil {
					return
				}
				switch n := expr.(type) {
				case *ast.BasicLit:
					if n.Kind != token.STRING || seen[n.Pos()] {
						return
					}
					text, e := strconv.Unquote(n.Value)
					if e != nil || !humanText(text) {
						return
					}
					key := messageKey(path, fn.Name.Name, text)
					seen[n.Pos()] = true
					pos := set.Position(n.Pos())
					rel, _ := filepath.Rel(root, path)
					all = append(all, Message{filepath.ToSlash(rel), pos.Line, key, text, context})
					edits = append(edits, replacement{pos.Offset, set.Position(n.End()).Offset, prefix + strconv.Quote(key) + ")"})
				case *ast.BinaryExpr:
					if n.Op == token.ADD {
						collect(n.X, context)
						collect(n.Y, context)
					}
				case *ast.CallExpr:
					if sel, ok := n.Fun.(*ast.SelectorExpr); ok {
						if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "fmt" && sel.Sel.Name == "Sprintf" && len(n.Args) > 0 {
							collect(n.Args[0], context+":printf")
						}
					}
				}
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.KeyValueExpr:
					if k, ok := x.Key.(*ast.BasicLit); ok && k.Kind == token.STRING {
						key, _ := strconv.Unquote(k.Value)
						switch key {
						case "Error", "Flash", "Success", "Message", "FlashError":
							collect(x.Value, "view:"+key)
						}
					}
				case *ast.CallExpr:
					if recommendationBuilder {
						if id, ok := x.Fun.(*ast.Ident); ok && (id.Name == "add" || id.Name == "addDismissible") && len(x.Args) >= 5 {
							for _, i := range []int{1, 2, 4} {
								collect(x.Args[i], "dashboard-recommendation")
							}
						}
					}
					if id, ok := x.Fun.(*ast.Ident); ok && len(x.Args) > 0 {
						switch id.Name {
						case "renderErr", "renderLoginErr", "renderTOTPErr", "renderError":
							collect(x.Args[0], "form-error")
						}
					}
					if sel, ok := x.Fun.(*ast.SelectorExpr); ok && len(x.Args) > 0 {
						if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "url" && sel.Sel.Name == "QueryEscape" {
							collect(x.Args[0], "redirect-message")
						}
					}
				case *ast.AssignStmt:
					for i, lhs := range x.Lhs {
						if id, ok := lhs.(*ast.Ident); ok && i < len(x.Rhs) {
							switch id.Name {
							case "errMsg", "errorMsg", "successMsg", "flashMsg":
								collect(x.Rhs[i], "message-variable")
							}
						}
					}
				}
				return true
			})
		}
		if write && len(edits) > 0 {
			sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
			for _, e := range edits {
				raw = append(append(append([]byte{}, raw[:e.start]...), []byte(e.value)...), raw[e.end:]...)
			}
			if err = os.WriteFile(path, raw, 0644); err != nil {
				return err
			}
		}
		return nil
	})
	return all, err
}
