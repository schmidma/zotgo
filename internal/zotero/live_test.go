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

func TestLiveNotesMatchRawItems(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	body, _, err := c.do(ctx, "/api/users/0/items", url.Values{"itemType": {"note"}, "limit": {"100"}})
	if err != nil {
		t.Fatalf("find notes: %v", err)
	}
	var items []struct {
		Key  string          `json:"key"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &items); err != nil {
		t.Fatalf("decode note search: %v", err)
	}
	var seed Envelope
	var parentKey string
	for _, item := range items {
		var data struct {
			ParentItem json.RawMessage `json:"parentItem"`
		}
		if err := json.Unmarshal(item.Data, &data); err != nil {
			t.Fatalf("decode seed note: %v", err)
		}
		parent, err := noteParentKey(data.ParentItem)
		if err != nil {
			t.Fatalf("decode seed parent: %v", err)
		}
		if parent != "" {
			seed = Envelope{Key: item.Key, Data: item.Data}
			parentKey = parent
			break
		}
	}
	if seed.Key == "" {
		t.Skip("no child notes to exercise")
	}

	rawItem, err := c.RawItem(ctx, UserLibrary(), seed.Key)
	if err != nil {
		t.Fatalf("RawItem: %v", err)
	}
	var rawSeed struct {
		Key  string `json:"key"`
		Data struct {
			ItemType     string          `json:"itemType"`
			ParentItem   json.RawMessage `json:"parentItem"`
			DateAdded    string          `json:"dateAdded"`
			DateModified string          `json:"dateModified"`
			Tags         []Tag           `json:"tags"`
			Note         string          `json:"note"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rawItem, &rawSeed); err != nil {
		t.Fatalf("decode raw note: %v", err)
	}
	typedSeed, err := seed.Note()
	if err != nil {
		t.Fatalf("Note: %v", err)
	}
	if rawSeed.Key != typedSeed.Key || rawSeed.Data.ItemType != "note" || typedSeed.ParentKey != parentKey || rawSeed.Data.DateAdded != typedSeed.DateAdded || rawSeed.Data.DateModified != typedSeed.DateModified || rawSeed.Data.Note != typedSeed.HTML || len(rawSeed.Data.Tags) != len(typedSeed.Tags) {
		t.Fatal("typed note differs from independently decoded raw note")
	}

	opts := ChildrenOptions{ItemType: "note", Limit: 100}
	rawChildren, err := c.AllRawChildItems(ctx, UserLibrary(), parentKey, opts)
	if err != nil {
		t.Fatalf("AllRawChildItems: %v", err)
	}
	type expectedNote struct {
		ParentItem   json.RawMessage `json:"parentItem"`
		DateModified string          `json:"dateModified"`
		Note         string          `json:"note"`
	}
	expected := make(map[string]expectedNote, len(rawChildren))
	envelopes := make([]Envelope, 0, len(rawChildren))
	for _, rawChild := range rawChildren {
		var envelope struct {
			Key  string          `json:"key"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(rawChild, &envelope); err != nil {
			t.Fatalf("decode raw child note: %v", err)
		}
		var data expectedNote
		if err := json.Unmarshal(envelope.Data, &data); err != nil {
			t.Fatalf("decode raw child note data: %v", err)
		}
		expected[envelope.Key] = data
		envelopes = append(envelopes, Envelope{Key: envelope.Key, Data: envelope.Data})
	}
	notes, err := Notes(envelopes)
	if err != nil {
		t.Fatalf("Notes: %v", err)
	}
	if len(rawChildren) != len(notes) || len(notes) == 0 {
		t.Fatalf("raw=%d typed=%d, want equal nonzero counts", len(rawChildren), len(notes))
	}
	for i, note := range notes {
		want, ok := expected[note.Key]
		if !ok {
			t.Errorf("typed output invented note %q", note.Key)
			continue
		}
		parent, err := noteParentKey(want.ParentItem)
		if err != nil || parent != note.ParentKey || want.DateModified != note.DateModified || want.Note != note.HTML {
			t.Errorf("note %q metadata/body mismatch", note.Key)
		}
		if i > 0 && notes[i-1].DateModified < note.DateModified {
			t.Errorf("notes out of modified-descending order at %q and %q", notes[i-1].Key, note.Key)
		}
	}
	t.Logf("checked %d notes under one item", len(notes))
}

func TestLiveNotFound(t *testing.T) {
	c := liveClient(t)
	if _, err := c.Item(context.Background(), UserLibrary(), "ZZZZZZZZ"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
