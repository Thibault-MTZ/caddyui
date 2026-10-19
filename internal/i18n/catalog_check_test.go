// SPDX-License-Identifier: Apache-2.0
package i18n

import (
	"strings"
	"testing"
)

func TestCatalogCheckAllowsFallbackAndRejectsBrokenVariables(t *testing.T) {
	en := map[string]string{"x": "Hello {name}", "message.error": "Host %s: %d errors (100%%)", "_meta.name": "English"}
	if got := CheckCatalog(en, map[string]string{"x": ""}, false); len(got) != 0 {
		t.Fatal(got)
	}
	if got := CheckCatalog(en, map[string]string{"x": "Bonjour {person}", "message.error": "Hôte %d : %s erreurs (100%%)", "extra": "X"}, true); len(got) != 4 {
		t.Fatalf("want variable/printf/extra/metadata errors: %v", got)
	}
	if got := CheckCatalog(en, map[string]string{"x": "Bonjour {name}", "message.error": "Hôte %s : %d erreurs (100%%)", "_meta.name": "Français"}, true); len(got) != 0 {
		t.Fatal(strings.Join(got, "\n"))
	}
}
