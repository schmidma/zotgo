package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/CameronBrooks11/zotgo/internal/zotero"
)

func TestNotes(t *testing.T) {
	var out bytes.Buffer
	Notes(&out, "PARENT01", []zotero.Note{
		{Key: "NOTE0002", DateModified: "2026-02-02T00:00:00Z", HTML: `<div>PRIVATE BODY</div>`, Tags: []zotero.Tag{{Tag: "a"}}},
		{Key: "NOTE0001", DateModified: "2026-02-01T00:00:00Z"},
	})
	text := out.String()
	for _, want := range []string{"KEY", "MODIFIED", "CONTENT", "TAGS", "NOTE0002", "2026-02-02", "yes", "NOTE0001", "no", "2 notes"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "PRIVATE BODY") {
		t.Fatalf("note list leaked body:\n%s", text)
	}
}

func TestNotesEmpty(t *testing.T) {
	var out bytes.Buffer
	Notes(&out, "PARENT01", nil)
	if got, want := out.String(), "No notes for item PARENT01.\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestNote(t *testing.T) {
	var out bytes.Buffer
	Note(&out, zotero.Note{
		Key:          "NOTE0001",
		ParentKey:    "PARENT01",
		DateAdded:    "2026-01-01T00:00:00Z",
		DateModified: "2026-02-01T00:00:00Z",
		Tags:         []zotero.Tag{{Tag: "review"}},
		HTML:         `<div><p>Requested body</p></div>`,
	})
	text := out.String()
	for _, want := range []string{"Key", "NOTE0001", "Parent", "PARENT01", "Added", "Modified", "review", "HTML:", "<div><p>Requested body</p></div>"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}
}

func TestNoteEmpty(t *testing.T) {
	var out bytes.Buffer
	Note(&out, zotero.Note{Key: "NOTE0001"})
	if !strings.Contains(out.String(), "HTML:\n(empty)\n") {
		t.Fatalf("empty note output = %q", out.String())
	}
}
