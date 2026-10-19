// SPDX-License-Identifier: Apache-2.0

// i18n-catalog checks existing JSON catalogs or synchronizes a language with
// English. Synchronization preserves translations and adds empty fallback values;
// it does not contact a translation service or generate translated prose.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/X4Applegate/caddyui/internal/i18n"
	"os"
	"path/filepath"
	"strings"
)

func read(path string) (map[string]string, error) {
	var m map[string]string
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	e = json.Unmarshal(b, &m)
	return m, e
}
func main() {
	dir := flag.String("dir", "web/i18n", "catalog directory")
	lang := flag.String("lang", "", "one language code (otherwise all)")
	sync := flag.Bool("sync", false, "add missing English keys with empty values; preserve existing translations")
	name := flag.String("name", "", "language's own display name for a newly synchronized catalog")
	complete := flag.Bool("require-complete", false, "fail on missing or empty translations")
	flag.Parse()
	fail := func(e error) { fmt.Fprintln(os.Stderr, e); os.Exit(1) }
	if *sync && (*lang == "" || *lang == "en") {
		fail(fmt.Errorf("-sync needs a non-English -lang"))
	}
	if *lang != "" && (filepath.Base(*lang) != *lang || strings.ContainsAny(*lang, "/\\.")) {
		fail(fmt.Errorf("invalid language code"))
	}
	en, err := read(filepath.Join(*dir, "en.json"))
	if err != nil {
		fail(err)
	}
	files, err := filepath.Glob(filepath.Join(*dir, "*.json"))
	if err != nil {
		fail(err)
	}
	if *lang != "" {
		files = []string{filepath.Join(*dir, *lang+".json")}
	}
	bad := false
	for _, file := range files {
		if filepath.Base(file) == "en.json" {
			continue
		}
		cat, e := read(file)
		if e != nil {
			if !*sync || !os.IsNotExist(e) {
				fail(e)
			}
			cat = map[string]string{}
		}
		if *sync {
			for key := range en {
				if _, ok := cat[key]; !ok {
					cat[key] = ""
				}
			}
			if *name != "" {
				cat["_meta.name"] = *name
			} else if cat["_meta.name"] == "" {
				cat["_meta.name"] = *lang
			}
			b, e := json.MarshalIndent(cat, "", "  ")
			if e != nil {
				fail(e)
			}
			if e = os.WriteFile(file, append(b, '\n'), 0644); e != nil {
				fail(e)
			}
		}
		issues := i18n.CheckCatalog(en, cat, *complete)
		filled := 0
		for key := range en {
			if strings.TrimSpace(cat[key]) != "" {
				filled++
			}
		}
		fmt.Printf("%s: %d/%d translated\n", filepath.Base(file), filled, len(en))
		for _, issue := range issues {
			fmt.Fprintln(os.Stderr, filepath.Base(file)+": "+issue)
			bad = true
		}
	}
	if bad {
		os.Exit(1)
	}
}
