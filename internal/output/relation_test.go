package output

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/CameronBrooks11/zotgo/internal/zotero"
)

func TestNewRelationsShapesStableRecords(t *testing.T) {
	item := zotero.Envelope{
		Key:     "SOURCE01",
		Version: 99,
		Data: json.RawMessage(`{
			"relations": {
				"owl:sameAs": ["https://example.com/work/1"],
				"dc:relation": ["http://zotero.org/users/123/items/TARGET01"]
			}
		}`),
	}

	got, err := NewRelations(item)
	if err != nil {
		t.Fatalf("NewRelations: %v", err)
	}
	want := []Relation{
		{ItemKey: "SOURCE01", Predicate: "dc:relation", Target: "http://zotero.org/users/123/items/TARGET01", TargetKey: "TARGET01"},
		{ItemKey: "SOURCE01", Predicate: "owl:sameAs", Target: "https://example.com/work/1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("relations = %#v, want %#v", got, want)
	}

	blob, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var records []map[string]any
	if err := json.Unmarshal(blob, &records); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := records[0]["version"]; ok {
		t.Fatalf("relation exposes endpoint-scoped version: %s", blob)
	}
	if _, ok := records[1]["targetKey"]; ok {
		t.Fatalf("external relation has targetKey: %s", blob)
	}
}

func TestNewRelationsEmptyIsAnEmptySlice(t *testing.T) {
	got, err := NewRelations(zotero.Envelope{Key: "SOURCE01", Data: json.RawMessage(`{"relations":{}}`)})
	if err != nil {
		t.Fatalf("NewRelations: %v", err)
	}
	blob, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(blob) != "[]" {
		t.Fatalf("empty relations = %s, want []", blob)
	}
}
