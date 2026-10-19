// SPDX-License-Identifier: Apache-2.0

// i18n-messages audits explicitly user-facing Go expressions. Review its output
// before -write; merge returned keys into en.json before compiling the server.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/X4Applegate/caddyui/internal/i18naudit"
	"os"
)

func main() {
	root := flag.String("root", ".", "repository root")
	write := flag.Bool("write", false, "replace reviewed user-facing literals with existing translator calls")
	flag.Parse()
	entries, err := i18naudit.ExtractMessages(*root, *write)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(entries); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
