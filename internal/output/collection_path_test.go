package output

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/CameronBrooks11/zotgo/internal/zotero"
)

func TestNewCollectionPaths(t *testing.T) {
	paths := []zotero.CollectionPath{
		{
			Key:       "LEAF0001",
			Name:      "Methods",
			ParentKey: "MID00001",
			Segments: []zotero.CollectionPathSegment{
				{Key: "ROOT0001", Name: "Research"},
				{Key: "MID00001", Name: "Projects"},
				{Key: "LEAF0001", Name: "Methods"},
			},
		},
	}
	records := NewCollectionPaths(paths)
	want := []CollectionPath{
		{
			Key:       "LEAF0001",
			Name:      "Methods",
			ParentKey: "MID00001",
			Segments: []CollectionPathSegment{
				{Key: "ROOT0001", Name: "Research"},
				{Key: "MID00001", Name: "Projects"},
				{Key: "LEAF0001", Name: "Methods"},
			},
			DisplayPath: "Research / Projects / Methods",
		},
	}
	if !reflect.DeepEqual(records, want) {
		t.Fatalf("records = %#v, want %#v", records, want)
	}
}

func TestCollectionPathStableFields(t *testing.T) {
	records := NewCollectionPaths([]zotero.CollectionPath{{
		Key: "ROOT0001", Name: "Root", Segments: []zotero.CollectionPathSegment{{Key: "ROOT0001", Name: "Root"}},
	}})
	encoded, err := json.Marshal(records[0])
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, field := range []string{"key", "name", "parentKey", "segments", "displayPath"} {
		if _, ok := fields[field]; !ok {
			t.Errorf("collection path omitted %q: %s", field, encoded)
		}
	}
	if len(fields) != 5 || fields["parentKey"] != "" {
		t.Errorf("fields = %#v, want exactly five with empty parentKey", fields)
	}
}

func TestNewCollectionPathsEmptyIsArray(t *testing.T) {
	records := NewCollectionPaths(nil)
	if records == nil || len(records) != 0 {
		t.Fatalf("records = %#v, want non-nil empty slice", records)
	}
}
