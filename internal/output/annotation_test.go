package output

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/CameronBrooks11/zotgo/internal/zotero"
)

func TestNewAnnotations(t *testing.T) {
	records := NewAnnotations([]zotero.Annotation{
		{
			Key:           "ANN00001",
			AttachmentKey: "ATTACH01",
			Type:          "highlight",
			PageLabel:     "12",
			Color:         "#ffd400",
			SortIndex:     "00012|00001",
			HasText:       true,
			HasComment:    false,
		},
	})
	if len(records) != 1 {
		t.Fatalf("records length = %d", len(records))
	}
	got := records[0]
	if got.Key != "ANN00001" || got.AttachmentKey != "ATTACH01" || got.Type != "highlight" {
		t.Fatalf("record = %#v", got)
	}
	if !got.HasText || got.HasComment {
		t.Fatalf("presence flags = text:%v comment:%v", got.HasText, got.HasComment)
	}

	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	text := string(encoded)
	for _, forbidden := range []string{"annotationText", "annotationComment", "annotationPosition", "version"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("stable annotation leaked %q: %s", forbidden, text)
		}
	}
}

func TestAnnotationFieldsRemainPresentWhenEmpty(t *testing.T) {
	record := NewAnnotations([]zotero.Annotation{{
		Key:           "ANN00002",
		AttachmentKey: "ATTACH01",
		Type:          "ink",
	}})[0]
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, field := range []string{
		"key", "attachmentKey", "type", "pageLabel", "color", "sortIndex", "hasText", "hasComment",
	} {
		if _, ok := fields[field]; !ok {
			t.Errorf("empty annotation omitted %q: %s", field, encoded)
		}
	}
	if len(fields) != 8 {
		t.Errorf("annotation fields = %v, want exactly 8 stable fields", fields)
	}
}

func TestNewAnnotationsReturnsEmptyArray(t *testing.T) {
	if records := NewAnnotations(nil); records == nil || len(records) != 0 {
		t.Fatalf("records = %#v, want non-nil empty slice", records)
	}
}
