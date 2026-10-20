// SPDX-License-Identifier: Apache-2.0

// i18n inventories and conservatively extracts user-visible template literals.
// It never edits a catalog: proposed English entries are a separate artifact.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	mode := flag.String("mode", "audit", "audit, check, or extract")
	dir := flag.String("templates", "web/templates", "template directory")
	catalog := flag.String("catalog", "web/i18n/en.json", "existing English catalog (read only)")
	entriesPath := flag.String("entries", "", "separate proposed English JSON file; required for extract")
	reportPath := flag.String("report", "", "inventory JSON file (otherwise stdout)")
	reviewPath := flag.String("review", "", "reviewed JavaScript display literals by file (JSON map of string arrays)")
	flag.Parse()
	if *mode != "audit" && *mode != "check" && *mode != "extract" {
		fail(fmt.Errorf("unknown mode %q", *mode))
	}
	if *mode == "extract" && *entriesPath == "" {
		fail(fmt.Errorf("extract requires -entries; the English catalog is never edited"))
	}
	if *entriesPath != "" {
		a, _ := filepath.Abs(*entriesPath)
		b, _ := filepath.Abs(*catalog)
		if a == b {
			fail(fmt.Errorf("entries must be separate from the existing catalog"))
		}
	}
	known := map[string]string{}
	if raw, err := os.ReadFile(*catalog); err != nil {
		fail(err)
	} else if err := json.Unmarshal(raw, &known); err != nil {
		fail(err)
	}
	entries := map[string]string{}
	review := map[string][]string{}
	if *reviewPath != "" {
		raw, err := os.ReadFile(*reviewPath)
		if err != nil {
			fail(err)
		}
		if err := json.Unmarshal(raw, &review); err != nil {
			fail(err)
		}
	}
	if *entriesPath != "" {
		if raw, err := os.ReadFile(*entriesPath); err == nil {
			if err := json.Unmarshal(raw, &entries); err != nil {
				fail(err)
			}
		} else if !os.IsNotExist(err) {
			fail(err)
		}
	}
	paths, err := filepath.Glob(filepath.Join(*dir, "*.html"))
	if err != nil {
		fail(err)
	}
	sort.Strings(paths)
	var inventory []finding
	type changedFile struct {
		path string
		data []byte
		mode os.FileMode
	}
	var changes []changedFile
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			fail(err)
		}
		approved := map[string]bool{}
		for _, text := range review[filepath.Base(path)] {
			approved[text] = true
		}
		s := scanner{source: string(raw), file: filepath.Base(path), known: known, entries: entries, reviewed: approved}
		s.scan()
		if s.err != nil {
			fail(fmt.Errorf("%s: %w", path, s.err))
		}
		inventory = append(inventory, s.findings...)
		if *mode == "extract" && len(s.edits) > 0 {
			info, err := os.Stat(path)
			if err != nil {
				fail(err)
			}
			changes = append(changes, changedFile{path, []byte(s.apply()), info.Mode()})
		}
	}
	// Complete every scan before writing anything: malformed templates must not
	// produce a partially edited tree. Use Git to review/revert extraction.
	if *mode == "extract" {
		writeJSON(*entriesPath, entries)
		for _, file := range changes {
			if err := os.WriteFile(file.path, file.data, file.mode); err != nil {
				fail(err)
			}
		}
	}
	if *reportPath != "" {
		writeJSON(*reportPath, inventory)
	} else {
		raw, err := json.MarshalIndent(inventory, "", "  ")
		if err != nil {
			fail(err)
		}
		fmt.Println(string(raw))
	}
	convertible, manual, technical := 0, 0, 0
	for _, item := range inventory {
		switch item.Disposition {
		case "extract":
			convertible++
		case "manual":
			manual++
		case "preserve":
			technical++
		}
	}
	fmt.Fprintf(os.Stderr, "%d template files; %d extractable literals; %d manual-review literals; %d preserved technical literals; %d proposed English entries\n", len(paths), convertible, manual, technical, len(entries))
	if *mode == "check" && (convertible > 0 || manual > 0) {
		os.Exit(1)
	}
}

func writeJSON(path string, value any) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0644); err != nil {
		fail(err)
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, "i18n:", err); os.Exit(2) }
