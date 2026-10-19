// SPDX-License-Identifier: Apache-2.0
package i18n

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

// PrintfVerbs preserves the formatting contract for server messages that keep
// their existing fmt.Sprintf calls. Named placeholders remain preferred for new
// messages. %% is included so an escaped percent cannot become a formatting verb.
func PrintfVerbs(text string) []string {
	re := regexp.MustCompile(`%(?:\[[0-9]+\])?[+#\- 0]*(?:[0-9]+|\*)?(?:\.(?:[0-9]+|\*))?(?:\[[0-9]+\])?[a-zA-Z%]`)
	return re.FindAllString(text, -1)
}

// CheckCatalog validates data-only contributions using the same contract as the
// runtime: English is authoritative, empty/missing values fall back to English.
// complete is an optional release gate for one maintained language, never a
// requirement imposed on every partial community catalog.
func CheckCatalog(english, locale map[string]string, complete bool) []string {
	var issues []string
	for key, value := range locale {
		if strings.HasPrefix(key, "_meta.") {
			continue
		}
		source, ok := english[key]
		if !ok {
			issues = append(issues, fmt.Sprintf("unknown key %s", key))
			continue
		}
		if strings.TrimSpace(value) == "" {
			continue
		}
		if !reflect.DeepEqual(Placeholders(source), Placeholders(value)) {
			issues = append(issues, fmt.Sprintf("placeholder mismatch %s", key))
		}
		if strings.HasPrefix(key, "message.") && !reflect.DeepEqual(PrintfVerbs(source), PrintfVerbs(value)) {
			issues = append(issues, fmt.Sprintf("printf format mismatch %s", key))
		}
	}
	if complete {
		for key := range english {
			if strings.TrimSpace(locale[key]) == "" {
				issues = append(issues, fmt.Sprintf("missing translation %s", key))
			}
		}
	}
	sort.Strings(issues)
	return issues
}
