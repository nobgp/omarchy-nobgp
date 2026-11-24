package setup

import (
	"strings"
	"testing"
)

const entries = `  "nobgp": {"icon":"x","label":"noBGP"},
  "nobgp.open": {"icon":"x","label":"Open","action":"a"},
`

func countKeys(s string) int { return strings.Count(s, `"nobgp`) }

func TestMergeIntoMissingFile(t *testing.T) {
	out, err := MergeMenu("", entries)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "{\n") || !strings.HasSuffix(out, "}\n") || countKeys(out) != 2 {
		t.Fatalf("%s", out)
	}
	mustParse(t, out)
}

// Omarchy's sample extension is only comments inside the braces.
func TestMergeIntoSampleExtension(t *testing.T) {
	sample := "{\n  // Extend the menu.\n  // \"about\": {\"icon\":\"\",\"label\":\"About\"},\n}\n"
	out, err := MergeMenu(sample, entries)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "// Extend the menu.") || countKeys(out) != 2 {
		t.Fatalf("%s", out)
	}
	mustParse(t, out)
}

func TestMergeAddsCommaAfterUserEntry(t *testing.T) {
	user := "{\n  \"personal\": {\"icon\":\"\",\"label\":\"Personal {not a brace}\"} // mine\n}\n"
	out, err := MergeMenu(user, entries)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"Personal {not a brace}"},`) {
		t.Fatalf("missing comma after the user's entry:\n%s", out)
	}
	mustParse(t, out)
}

func TestMergeIsIdempotentAndReplaces(t *testing.T) {
	user := "{\n  \"personal\": {\"label\":\"P\"},\n}\n"
	once, _ := MergeMenu(user, entries)
	twice, _ := MergeMenu(once, entries)
	if once != twice {
		t.Fatalf("second merge changed the file:\n%s\n---\n%s", once, twice)
	}
	newer, _ := MergeMenu(once, `  "nobgp": {"label":"noBGP v2"},`+"\n")
	if countKeys(newer) != 1 || !strings.Contains(newer, "noBGP v2") || !strings.Contains(newer, `"personal"`) {
		t.Fatalf("%s", newer)
	}
	back, changed := RemoveMenu(newer)
	if !changed || strings.Contains(back, "nobgp") || !strings.Contains(back, `"personal"`) {
		t.Fatalf("remove: %s", back)
	}
	mustParse(t, back)
}

func TestMergeRefusesBrokenFile(t *testing.T) {
	if _, err := MergeMenu("{\n  \"a\": \"unterminated\n", entries); err == nil {
		t.Fatal("want error")
	}
	if _, err := MergeMenu("// nothing here\n", entries); err == nil {
		t.Fatal("want error for a file without an object")
	}
}
