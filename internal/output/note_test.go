package output

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/CameronBrooks11/zotgo/internal/zotero"
)

func testNote(html string) zotero.Note {
	return zotero.Note{
		Key:          "NOTE0001",
		ParentKey:    "PARENT01",
		DateAdded:    "2026-01-01T00:00:00Z",
		DateModified: "2026-02-01T00:00:00Z",
		Tags:         []zotero.Tag{{Tag: "review", Type: 0}, {Tag: "imported", Type: 1}},
		HTML:         html,
	}
}

func TestNewNotesOmitHTML(t *testing.T) {
	records := NewNotes([]zotero.Note{testNote(`<div><p>PRIVATE BODY</p></div>`)})
	if len(records) != 1 {
		t.Fatalf("records length = %d", len(records))
	}
	record := records[0]
	if record.HTML != nil || !record.HasContent {
		t.Fatalf("record body state = html:%v content:%v", record.HTML, record.HasContent)
	}
	if len(record.Tags) != 2 || record.Tags[0].Name != "review" || record.Tags[1].Name != "imported" || !record.Tags[1].Automatic {
		t.Fatalf("tags = %#v", record.Tags)
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(encoded), "PRIVATE BODY") || strings.Contains(string(encoded), `"html"`) {
		t.Fatalf("list record leaked HTML: %s", encoded)
	}
}

func TestNewNoteGetIncludesHTMLEvenWhenEmpty(t *testing.T) {
	for _, html := range []string{"", `<div><p>Body</p></div>`} {
		record := NewNote(testNote(html), true)
		if record.HTML == nil || *record.HTML != html {
			t.Fatalf("html %q encoded as %#v", html, record.HTML)
		}
		if record.HasContent != (html != "") {
			t.Fatalf("html %q hasContent = %v", html, record.HasContent)
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		var fields map[string]any
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		for _, field := range []string{"key", "parentKey", "dateAdded", "dateModified", "tags", "hasContent", "html"} {
			if _, ok := fields[field]; !ok {
				t.Errorf("get note omitted %q: %s", field, encoded)
			}
		}
		if len(fields) != 7 {
			t.Errorf("get note fields = %v, want exactly 7", fields)
		}
	}
}

func TestNewNotesReturnsEmptyArray(t *testing.T) {
	if records := NewNotes(nil); records == nil || len(records) != 0 {
		t.Fatalf("records = %#v, want non-nil empty slice", records)
	}
}
