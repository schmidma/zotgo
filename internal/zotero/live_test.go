//go:build live

// Live tests exercise the client against a real, running Zotero with the Local API
// enabled. They are excluded from normal builds and CI; run them locally with:
//
//	go test -tags live ./internal/zotero -run TestLive -v
//
// Override the target with ZOTGO_BASE_URL if Zotero is not on the default port.
package zotero

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"testing"
)

func liveClient(t *testing.T) *Client {
	t.Helper()
	c := New(os.Getenv("ZOTGO_BASE_URL"))
	if h := c.CheckHealth(context.Background()); !h.Ready() {
		t.Skipf("Zotero not ready for live tests: %+v", h)
	}
	return c
}

func TestLiveUserLibraryReads(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	items, page, err := c.Items(ctx, UserLibrary(), ItemsOptions{Top: true, Limit: 3})
	if err != nil {
		t.Fatalf("Items: %v", err)
	}
	if page.TotalResults == 0 || len(items) == 0 {
		t.Fatalf("expected items, got total=%d len=%d", page.TotalResults, len(items))
	}
	for _, it := range items {
		if it.Key == "" || it.ItemType() == "" {
			t.Errorf("item missing key/type: %+v", it)
		}
	}
	t.Logf("user library: %d items total, first=%q (%s)", page.TotalResults, items[0].Title(), items[0].ItemType())
}

func TestLiveGroupResolutionAndReads(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	groups, err := c.Groups(ctx)
	if err != nil {
		t.Fatalf("Groups: %v", err)
	}
	if len(groups) == 0 {
		t.Skip("no group libraries to exercise")
	}
	g := groups[0]

	byName, err := c.ResolveLibrary(ctx, g.Data.Name)
	if err != nil {
		t.Fatalf("ResolveLibrary(%q): %v", g.Data.Name, err)
	}
	if byName.Kind != LibraryKindGroup || byName.ID != g.ID {
		t.Fatalf("name resolution mismatch: %+v vs group %d", byName, g.ID)
	}
	// The routed read must land in the group library, not My Library.
	items, page, err := c.Items(ctx, byName, ItemsOptions{Top: true, Limit: 1})
	if err != nil {
		t.Fatalf("group Items: %v", err)
	}
	for _, it := range items {
		if it.Library.Type != LibraryKindGroup || it.Library.ID != g.ID {
			t.Fatalf("item routed to wrong library: %+v (want group %d)", it.Library, g.ID)
		}
	}
	t.Logf("group %q (id %d): %d items total", g.Data.Name, g.ID, page.TotalResults)
}

func TestLiveAttachmentMatchesRawItem(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	body, _, err := c.do(ctx, "/api/users/0/items", url.Values{"itemType": {"attachment"}, "limit": {"1"}})
	if err != nil {
		t.Fatalf("find attachment: %v", err)
	}
	var items []struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(body, &items); err != nil {
		t.Fatalf("decode attachment search: %v", err)
	}
	if len(items) == 0 {
		t.Skip("no attachments to exercise")
	}
	raw, err := c.RawItem(ctx, UserLibrary(), items[0].Key)
	if err != nil {
		t.Fatalf("RawItem: %v", err)
	}
	var envelope struct {
		Key   string          `json:"key"`
		Links map[string]Link `json:"links"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode raw attachment envelope: %v", err)
	}
	attachment, err := (Envelope{Key: envelope.Key, Links: envelope.Links, Data: envelope.Data}).Attachment()
	if err != nil {
		t.Fatalf("Attachment: %v", err)
	}
	var expected struct {
		ItemType    string          `json:"itemType"`
		ParentItem  json.RawMessage `json:"parentItem"`
		LinkMode    string          `json:"linkMode"`
		ContentType string          `json:"contentType"`
		Filename    string          `json:"filename"`
		MD5         json.RawMessage `json:"md5"`
		MTime       json.RawMessage `json:"mtime"`
	}
	if err := json.Unmarshal(envelope.Data, &expected); err != nil {
		t.Fatalf("decode raw attachment data: %v", err)
	}
	parentKey, err := attachmentParentKey(expected.ParentItem)
	if err != nil {
		t.Fatalf("decode raw attachment parent: %v", err)
	}
	md5, err := nullableString(expected.MD5)
	if err != nil {
		t.Fatalf("decode raw attachment md5: %v", err)
	}
	mtime, err := nullableInt64(expected.MTime)
	if err != nil {
		t.Fatalf("decode raw attachment mtime: %v", err)
	}
	if expected.ItemType != "attachment" || attachment.Key != envelope.Key || attachment.ParentKey != parentKey || attachment.LinkMode != expected.LinkMode || attachment.ContentType != expected.ContentType || attachment.Filename != expected.Filename {
		t.Fatal("typed attachment identity differs from raw attachment")
	}
	if (attachment.MD5 == nil) != (md5 == nil) || (attachment.MTime == nil) != (mtime == nil) {
		t.Fatal("typed attachment nullable metadata differs from raw attachment")
	}
	if md5 != nil && *attachment.MD5 != *md5 {
		t.Fatal("typed attachment md5 differs from raw attachment")
	}
	if mtime != nil && *attachment.MTime != *mtime {
		t.Fatal("typed attachment mtime differs from raw attachment")
	}
	status := attachment.FileStatus()
	if status.State == "" || status.Reason == "" {
		t.Fatalf("empty file status: %#v", status)
	}
	t.Logf("checked attachment mode %s with conservative status %s", attachment.LinkMode, status.State)
}

func TestLiveNotFound(t *testing.T) {
	c := liveClient(t)
	if _, err := c.Item(context.Background(), UserLibrary(), "ZZZZZZZZ"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
