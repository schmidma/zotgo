package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/CameronBrooks11/zotgo/internal/zotero"
)

func TestAnnotations(t *testing.T) {
	var out bytes.Buffer
	Annotations(&out, "ATTACH01", []zotero.Annotation{
		{Key: "ANN00001", Type: "highlight", PageLabel: "12", SortIndex: "00012|00001", Color: "#ffd400", HasText: true},
		{Key: "ANN00002", Type: "note", PageLabel: "13", SortIndex: "00013|00002", Color: "#2ea8e5", HasComment: true},
	})
	text := out.String()
	for _, want := range []string{
		"KEY", "PAGE", "SORT INDEX", "TYPE", "TEXT", "COMMENT", "COLOR",
		"ANN00001", "12", "00012|00001", "highlight", "yes", "#ffd400",
		"ANN00002", "13", "00013|00002", "note", "#2ea8e5",
		"2 annotations",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}
}

func TestAnnotationsEmpty(t *testing.T) {
	var out bytes.Buffer
	Annotations(&out, "ATTACH01", nil)
	if got, want := out.String(), "No annotations for attachment ATTACH01.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
