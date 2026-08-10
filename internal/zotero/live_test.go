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
	"net/http"
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

func TestLiveRelationsMatchRawItemData(t *testing.T) {
	c := liveClient(t)
	response, err := c.get(context.Background(), "/api/users/0/items?limit=100")
	if err != nil {
		t.Fatalf("GET items: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET items: %s", response.Status)
	}
	var items []struct {
		Key  string          `json:"key"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&items); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	for _, rawItem := range items {
		var raw struct {
			Relations map[string]any `json:"relations"`
		}
		if err := json.Unmarshal(rawItem.Data, &raw); err != nil {
			t.Fatalf("decode item %s: %v", rawItem.Key, err)
		}
		if len(raw.Relations) == 0 {
			continue
		}
		expected := make(map[string]int)
		var expectedCount int
		for predicate, value := range raw.Relations {
			switch targets := value.(type) {
			case string:
				expected[predicate+"\x00"+targets]++
				expectedCount++
			case []any:
				for _, target := range targets {
					target, ok := target.(string)
					if !ok {
						t.Fatalf("raw relation %s on %s has non-string target", predicate, rawItem.Key)
					}
					expected[predicate+"\x00"+target]++
					expectedCount++
				}
			default:
				t.Fatalf("raw relation %s on %s has shape %T", predicate, rawItem.Key, value)
			}
		}
		relations, err := (Envelope{Key: rawItem.Key, Data: rawItem.Data}).Relations()
		if err != nil {
			t.Fatalf("Relations(%s): %v", rawItem.Key, err)
		}
		if len(relations) != expectedCount {
			t.Fatalf("Relations(%s) returned %d edges, raw data has %d", rawItem.Key, len(relations), expectedCount)
		}
		for _, relation := range relations {
			key := relation.Predicate + "\x00" + relation.Target
			if expected[key] == 0 {
				t.Errorf("Relations(%s) invented edge %s -> %s", rawItem.Key, relation.Predicate, relation.Target)
				continue
			}
			expected[key]--
		}
		t.Logf("item %s: checked %d relation edges", rawItem.Key, len(relations))
		return
	}
	t.Skip("no item relations among first 100 items")
}

func TestLiveNotFound(t *testing.T) {
	c := liveClient(t)
	if _, err := c.Item(context.Background(), UserLibrary(), "ZZZZZZZZ"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
