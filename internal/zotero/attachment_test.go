package zotero

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func int64Pointer(value int64) *int64 { return &value }

func TestEnvelopeAttachment(t *testing.T) {
	data := json.RawMessage(`{
		"itemType":"attachment",
		"parentItem":"PARENT01",
		"title":"Full Text PDF",
		"linkMode":"imported_file",
		"contentType":"application/pdf",
		"charset":"",
		"filename":"paper.pdf",
		"url":"https://example.com/paper.pdf",
		"accessDate":"2026-01-01T00:00:00Z",
		"dateAdded":"2026-01-02T00:00:00Z",
		"dateModified":"2026-01-03T00:00:00Z",
		"tags":[{"tag":"review","type":0}],
		"md5":"0123456789abcdef",
		"mtime":1700000000000
	}`)
	envelope := Envelope{
		Key:  "ATTACH01",
		Data: data,
		Links: map[string]Link{
			"enclosure": {Href: "http://127.0.0.1/file", Type: "application/pdf", Title: "paper.pdf", Length: int64Pointer(1234)},
		},
	}
	attachment, err := envelope.Attachment()
	if err != nil {
		t.Fatalf("Attachment: %v", err)
	}
	if attachment.Key != "ATTACH01" || attachment.ParentKey != "PARENT01" || attachment.LinkMode != "imported_file" || attachment.Filename != "paper.pdf" {
		t.Fatalf("attachment = %#v", attachment)
	}
	if attachment.MD5 == nil || *attachment.MD5 != "0123456789abcdef" || attachment.MTime == nil || *attachment.MTime != 1700000000000 {
		t.Fatalf("storage metadata = md5:%v mtime:%v", attachment.MD5, attachment.MTime)
	}
	if attachment.Enclosure == nil || attachment.Enclosure.Length == nil || *attachment.Enclosure.Length != 1234 {
		t.Fatalf("enclosure = %#v", attachment.Enclosure)
	}
	if !reflect.DeepEqual(attachment.Tags, []Tag{{Tag: "review", Type: 0}}) {
		t.Fatalf("tags = %#v", attachment.Tags)
	}
}

func TestAttachmentNullableAndAlternateShapes(t *testing.T) {
	for _, tt := range []struct {
		name   string
		parent string
		mtime  string
		want   *int64
	}{
		{name: "standalone false and string mtime", parent: "false", mtime: `"1700000000000"`, want: int64Pointer(1700000000000)},
		{name: "standalone null", parent: "null", mtime: "null", want: nil},
		{name: "omitted parent", parent: "", mtime: "null", want: nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			parent := ""
			if tt.parent != "" {
				parent = `,"parentItem":` + tt.parent
			}
			data := json.RawMessage(`{"itemType":"attachment","linkMode":"linked_url","tags":[],"md5":null,"mtime":` + tt.mtime + parent + `}`)
			attachment, err := (Envelope{Key: "ATTACH01", Data: data}).Attachment()
			if err != nil {
				t.Fatalf("Attachment: %v", err)
			}
			if attachment.ParentKey != "" || attachment.MD5 != nil {
				t.Fatalf("nullable fields = parent:%q md5:%v", attachment.ParentKey, attachment.MD5)
			}
			if !reflect.DeepEqual(attachment.MTime, tt.want) {
				t.Fatalf("mtime = %v, want %v", attachment.MTime, tt.want)
			}
			if attachment.Tags == nil {
				t.Fatal("tags is nil")
			}
		})
	}
}

func TestAttachmentFileStatus(t *testing.T) {
	length := int64(42)
	tests := []struct {
		name       string
		attachment Attachment
		state      string
	}{
		{name: "linked URL", attachment: Attachment{LinkMode: "linked_url"}, state: "not-applicable"},
		{name: "linked file", attachment: Attachment{LinkMode: "linked_file"}, state: "linked-unverified"},
		{name: "imported unavailable", attachment: Attachment{LinkMode: "imported_file"}, state: "unavailable"},
		{name: "imported location", attachment: Attachment{LinkMode: "imported_url", Enclosure: &Link{Href: "http://local/file"}}, state: "location-advertised"},
		{name: "imported metadata", attachment: Attachment{LinkMode: "imported_file", Enclosure: &Link{Href: "http://local/file", Length: &length}}, state: "metadata-available"},
		{name: "unknown mode", attachment: Attachment{LinkMode: "future_mode"}, state: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := tt.attachment.FileStatus()
			if status.State != tt.state || status.Reason == "" {
				t.Fatalf("status = %#v, want state %q and reason", status, tt.state)
			}
		})
	}
}

func TestAttachmentRejectsMalformedItems(t *testing.T) {
	tests := []struct {
		name string
		item Envelope
		want string
	}{
		{name: "missing key", item: Envelope{Data: json.RawMessage(`{"itemType":"attachment"}`)}, want: "missing attachment key"},
		{name: "missing data", item: Envelope{Key: "ATTACH01"}, want: "has no data"},
		{name: "invalid data", item: Envelope{Key: "ATTACH01", Data: json.RawMessage(`{`)}, want: "unexpected end"},
		{name: "wrong type", item: Envelope{Key: "ATTACH01", Data: json.RawMessage(`{"itemType":"book"}`)}, want: `type "book"`},
		{name: "bad parent", item: Envelope{Key: "ATTACH01", Data: json.RawMessage(`{"itemType":"attachment","parentItem":{}}`)}, want: "parentItem"},
		{name: "bad md5", item: Envelope{Key: "ATTACH01", Data: json.RawMessage(`{"itemType":"attachment","md5":7}`)}, want: "md5"},
		{name: "bad mtime", item: Envelope{Key: "ATTACH01", Data: json.RawMessage(`{"itemType":"attachment","mtime":"later"}`)}, want: "mtime"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.item.Attachment()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Attachment error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestRawItemPreservesAttachmentEnvelope(t *testing.T) {
	const fixture = `{"key":"ATTACH01","version":"","futureTop":{"kept":true},"links":{"enclosure":{"href":"http://local/file","type":"application/pdf","title":"paper.pdf","length":1234,"futureLink":true}},"data":{"itemType":"attachment","filename":"paper.pdf","futureData":7}}`
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/0/items/ATTACH01", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(fixture))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	raw, err := New(srv.URL).RawItem(context.Background(), UserLibrary(), "ATTACH01")
	if err != nil {
		t.Fatalf("RawItem: %v", err)
	}
	var got, want any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	if err := json.Unmarshal([]byte(fixture), &want); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("raw changed envelope: got %#v, want %#v", got, want)
	}
}
