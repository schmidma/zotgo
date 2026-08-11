package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/CameronBrooks11/zotgo/internal/zotero"
)

func TestCollectionPaths(t *testing.T) {
	paths := []zotero.CollectionPath{
		{
			Key: "LEAF0001",
			Segments: []zotero.CollectionPathSegment{
				{Key: "ROOT0001", Name: "Research"},
				{Key: "MID00001", Name: "Projects"},
				{Key: "LEAF0001", Name: "Methods"},
			},
		},
		{Key: "ROOT0001", Segments: []zotero.CollectionPathSegment{{Key: "ROOT0001", Name: "Research"}}},
	}
	var out bytes.Buffer
	CollectionPaths(&out, paths)
	text := out.String()
	for _, want := range []string{"KEY", "PATH", "LEAF0001", "Research / Projects / Methods", "ROOT0001", "2 collection paths"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}
	if strings.Index(text, "LEAF0001") > strings.LastIndex(text, "ROOT0001") {
		t.Fatalf("request order not preserved:\n%s", text)
	}
}
