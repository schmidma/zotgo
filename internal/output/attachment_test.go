package output

import (
	"encoding/json"
	"testing"

	"github.com/CameronBrooks11/zotgo/internal/zotero"
)

func TestNewAttachment(t *testing.T) {
	md5 := "0123456789abcdef"
	mtime := int64(1700000000000)
	length := int64(1234)
	record := NewAttachment(zotero.Attachment{
		Key:          "ATTACH01",
		ParentKey:    "PARENT01",
		Title:        "Full Text PDF",
		LinkMode:     "imported_file",
		ContentType:  "application/pdf",
		Filename:     "paper.pdf",
		DateAdded:    "2026-01-01T00:00:00Z",
		DateModified: "2026-01-02T00:00:00Z",
		Tags:         []zotero.Tag{{Tag: "review", Type: 0}},
		MD5:          &md5,
		MTime:        &mtime,
		Enclosure:    &zotero.Link{Href: "http://local/file", Type: "application/pdf", Title: "paper.pdf", Length: &length},
	})
	if record.Key != "ATTACH01" || record.ParentKey != "PARENT01" || record.FileStatus.State != "metadata-available" {
		t.Fatalf("record = %#v", record)
	}
	if record.Enclosure == nil || record.Enclosure.Length == nil || *record.Enclosure.Length != 1234 {
		t.Fatalf("enclosure = %#v", record.Enclosure)
	}
	if len(record.Tags) != 1 || record.Tags[0].Name != "review" {
		t.Fatalf("tags = %#v", record.Tags)
	}
}

func TestAttachmentStableFieldsRemainPresentWhenNullOrEmpty(t *testing.T) {
	record := NewAttachment(zotero.Attachment{Key: "ATTACH01", LinkMode: "linked_url"})
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, field := range []string{
		"key", "parentKey", "title", "linkMode", "contentType", "charset", "filename", "url",
		"accessDate", "dateAdded", "dateModified", "tags", "md5", "mtime", "enclosure", "fileStatus",
	} {
		if _, ok := fields[field]; !ok {
			t.Errorf("attachment omitted %q: %s", field, encoded)
		}
	}
	if len(fields) != 16 {
		t.Errorf("fields = %v, want exactly 16", fields)
	}
	if fields["md5"] != nil || fields["mtime"] != nil || fields["enclosure"] != nil {
		t.Errorf("nullable fields = md5:%v mtime:%v enclosure:%v", fields["md5"], fields["mtime"], fields["enclosure"])
	}
	if fields["tags"] == nil {
		t.Error("tags encoded as null")
	}
}
