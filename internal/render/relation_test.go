package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/CameronBrooks11/zotgo/internal/zotero"
)

func TestRelationsTable(t *testing.T) {
	var buf bytes.Buffer
	Relations(&buf, "SOURCE01", []zotero.Relation{
		{Predicate: "dc:relation", TargetKey: "TARGET01", Target: "http://zotero.org/users/1/items/TARGET01"},
		{Predicate: "owl:sameAs", Target: "https://example.com/work/1"},
	})
	out := buf.String()
	for _, want := range []string{"PREDICATE", "TARGET KEY", "TARGET", "dc:relation", "TARGET01", "owl:sameAs", "https://example.com/work/1"} {
		if !strings.Contains(out, want) {
			t.Errorf("relations table missing %q:\n%s", want, out)
		}
	}
}

func TestRelationsEmpty(t *testing.T) {
	var buf bytes.Buffer
	Relations(&buf, "SOURCE01", nil)
	if got := buf.String(); got != "No relations for SOURCE01.\n" {
		t.Fatalf("output = %q", got)
	}
}
