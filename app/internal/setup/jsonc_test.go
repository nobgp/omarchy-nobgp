package setup

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

var trailingComma = regexp.MustCompile(`,(\s*[}\]])`)

// mustParse checks the file is valid JSONC: valid JSON once // comments and
// trailing commas are removed (no entry here contains "//" in a string).
func mustParse(t *testing.T, s string) {
	t.Helper()
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if i := strings.Index(l, "//"); i >= 0 {
			l = l[:i]
		}
		lines = append(lines, l)
	}
	clean := trailingComma.ReplaceAllString(strings.Join(lines, "\n"), "$1")
	var v map[string]any
	if err := json.Unmarshal([]byte(clean), &v); err != nil {
		t.Fatalf("not valid JSONC: %v\n%s", err, s)
	}
}
