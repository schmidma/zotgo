package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/CameronBrooks11/zotgo/internal/zotero"
)

func TestAttachment(t *testing.T) {
	md5 := "0123456789abcdef"
	mtime := int64(1700000000000)
	length := int64(1234)
	var out bytes.Buffer
	Attachment(&out, zotero.Attachment{
		Key:          "ATTACH01",
		ParentKey:    "PARENT01",
		Title:        "Full Text PDF",
		LinkMode:     "imported_file",
		ContentType:  "application/pdf",
		Filename:     "paper.pdf",
		URL:          "https://example.com/paper.pdf",
		DateAdded:    "2026-01-01T00:00:00Z",
		DateModified: "2026-01-02T00:00:00Z",
		Tags:         []zotero.Tag{{Tag: "review"}},
		MD5:          &md5,
		MTime:        &mtime,
		Enclosure:    &zotero.Link{Href: "http://local/file", Length: &length},
	})
	text := out.String()
	for _, want := range []string{
		"Key", "ATTACH01", "Parent", "PARENT01", "Full Text PDF", "imported_file",
		"application/pdf", "paper.pdf", "review", md5, "1700000000000",
		"http://local/file", "1234", "metadata-available", "advertised a file location and size metadata",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}
}

func TestAttachmentLinkedURLStatus(t *testing.T) {
	var out bytes.Buffer
	Attachment(&out, zotero.Attachment{Key: "ATTACH01", LinkMode: "linked_url", URL: "https://example.com"})
	text := out.String()
	if !strings.Contains(text, "not-applicable") || !strings.Contains(text, "no managed file") {
		t.Fatalf("linked URL output = %s", text)
	}
}
