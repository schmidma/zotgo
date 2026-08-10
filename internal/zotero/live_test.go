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

func TestLiveAnnotationsMatchRawChildren(t *testing.T) {
	c := liveClient(t)
	ctx := context.Background()

	items, _, err := c.Items(ctx, UserLibrary(), ItemsOptions{ItemType: "annotation", Limit: 1})
	if err != nil {
		t.Fatalf("find annotation: %v", err)
	}
	if len(items) == 0 {
		t.Skip("no annotations to exercise")
	}
	var seed struct {
		ParentItem string `json:"parentItem"`
	}
	if err := json.Unmarshal(items[0].Data, &seed); err != nil {
		t.Fatalf("decode seed annotation: %v", err)
	}
	if seed.ParentItem == "" {
		t.Fatal("seed annotation has no parent attachment")
	}

	opts := ChildrenOptions{ItemType: "annotation", Limit: 100}
	rawItems, err := c.AllRawChildItems(ctx, UserLibrary(), seed.ParentItem, opts)
	if err != nil {
		t.Fatalf("AllRawChildItems: %v", err)
	}
	typedItems, err := c.AllChildItems(ctx, UserLibrary(), seed.ParentItem, opts)
	if err != nil {
		t.Fatalf("AllChildItems: %v", err)
	}
	annotations, err := Annotations(typedItems)
	if err != nil {
		t.Fatalf("Annotations: %v", err)
	}
	if len(rawItems) != len(annotations) || len(annotations) == 0 {
		t.Fatalf("raw=%d typed=%d, want equal nonzero counts", len(rawItems), len(annotations))
	}

	type expectedAnnotation struct {
		ParentItem          string `json:"parentItem"`
		ItemType            string `json:"itemType"`
		AnnotationType      string `json:"annotationType"`
		AnnotationText      string `json:"annotationText"`
		AnnotationComment   string `json:"annotationComment"`
		AnnotationSortIndex string `json:"annotationSortIndex"`
	}
	expected := make(map[string]expectedAnnotation, len(rawItems))
	for _, rawItem := range rawItems {
		var envelope struct {
			Key  string             `json:"key"`
			Data expectedAnnotation `json:"data"`
		}
		if err := json.Unmarshal(rawItem, &envelope); err != nil {
			t.Fatalf("decode raw annotation envelope: %v", err)
		}
		if envelope.Key == "" || envelope.Data.ItemType != "annotation" || envelope.Data.ParentItem != seed.ParentItem {
			t.Fatalf("unexpected raw annotation identity: key=%q type=%q parent=%q", envelope.Key, envelope.Data.ItemType, envelope.Data.ParentItem)
		}
		expected[envelope.Key] = envelope.Data
	}
	for i, annotation := range annotations {
		want, ok := expected[annotation.Key]
		if !ok {
			t.Errorf("typed output invented annotation %q", annotation.Key)
			continue
		}
		if annotation.AttachmentKey != want.ParentItem || annotation.Type != want.AnnotationType || annotation.SortIndex != want.AnnotationSortIndex {
			t.Errorf("annotation %q metadata mismatch", annotation.Key)
		}
		if annotation.HasText != (want.AnnotationText != "") || annotation.HasComment != (want.AnnotationComment != "") {
			t.Errorf("annotation %q presence flags mismatch", annotation.Key)
		}
		if i > 0 {
			previous := annotations[i-1]
			if previous.SortIndex > annotation.SortIndex && annotation.SortIndex != "" {
				t.Errorf("annotations out of document order at %q and %q", previous.Key, annotation.Key)
			}
		}
	}
	t.Logf("checked %d annotations under one attachment", len(annotations))
}

func TestLiveNotFound(t *testing.T) {
	c := liveClient(t)
	if _, err := c.Item(context.Background(), UserLibrary(), "ZZZZZZZZ"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
