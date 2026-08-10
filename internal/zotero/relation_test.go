package zotero

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestEnvelopeRelationsNormalizesAndSortsTargets(t *testing.T) {
	item := Envelope{Data: json.RawMessage(`{
		"relations": {
			"owl:sameAs": "https://example.com/work/1",
			"dc:relation": [
				"http://zotero.org/users/123/items/BBBB2222",
				"http://zotero.org/groups/42/items/AAAA1111"
			]
		}
	}`)}

	got, err := item.Relations()
	if err != nil {
		t.Fatalf("Relations: %v", err)
	}
	want := []Relation{
		{Predicate: "dc:relation", Target: "http://zotero.org/groups/42/items/AAAA1111", TargetKey: "AAAA1111"},
		{Predicate: "dc:relation", Target: "http://zotero.org/users/123/items/BBBB2222", TargetKey: "BBBB2222"},
		{Predicate: "owl:sameAs", Target: "https://example.com/work/1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("relations = %#v, want %#v", got, want)
	}
}

func TestEnvelopeRelationsRecognizesOnlyZoteroItemURIs(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   string
	}{
		{name: "user", target: "http://zotero.org/users/123/items/ABCD1234", want: "ABCD1234"},
		{name: "group", target: "https://www.zotero.org/groups/42/items/EFGH5678", want: "EFGH5678"},
		{name: "external host", target: "https://example.com/users/123/items/ABCD1234"},
		{name: "not item route", target: "https://zotero.org/users/123/collections/ABCD1234"},
		{name: "extra path", target: "https://zotero.org/users/123/items/ABCD1234/child"},
		{name: "relative", target: "/users/123/items/ABCD1234"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := relationTargetKey(tt.target); got != tt.want {
				t.Fatalf("relationTargetKey(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}

func TestEnvelopeRelationsRejectsMalformedTargetsWithoutBreakingItemData(t *testing.T) {
	item := Envelope{Data: json.RawMessage(`{"itemType":"book","title":"Still readable","relations":{"dc:relation":42}}`)}
	if relations, err := item.Relations(); err == nil {
		t.Fatalf("Relations succeeded for numeric target: %#v", relations)
	}
	data, err := item.ItemData()
	if err != nil {
		t.Fatalf("ItemData failed because of an unrelated relation shape: %v", err)
	}
	if data.Title != "Still readable" {
		t.Fatalf("title = %q", data.Title)
	}
}

func TestEnvelopeRelationsEmptyIsAnEmptySlice(t *testing.T) {
	item := Envelope{Data: json.RawMessage(`{"relations":{}}`)}
	got, err := item.Relations()
	if err != nil {
		t.Fatalf("Relations: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("relations = %#v, want non-nil empty slice", got)
	}
}
