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
