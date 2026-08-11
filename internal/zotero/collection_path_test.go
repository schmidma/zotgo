package zotero

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func collectionEnvelope(key, name, parent string) Envelope {
	return Envelope{
		Key:  key,
		Data: json.RawMessage(`{"key":` + quoted(key) + `,"name":` + quoted(name) + `,"parentCollection":` + parent + `}`),
	}
}

func quoted(value string) string {
	return strconv.Quote(value)
}

func TestResolveCollectionPaths(t *testing.T) {
	collections := []Envelope{
		collectionEnvelope("LEAF0001", "Methods", `"MID00001"`),
		collectionEnvelope("ROOT0001", "Research", "false"),
		collectionEnvelope("MID00001", "Projects", `"ROOT0001"`),
	}
	paths, err := ResolveCollectionPaths(collections, []string{"LEAF0001", "ROOT0001", "LEAF0001"})
	if err != nil {
		t.Fatalf("ResolveCollectionPaths: %v", err)
	}
	if len(paths) != 3 || paths[0].Key != "LEAF0001" || paths[1].Key != "ROOT0001" || paths[2].Key != "LEAF0001" {
		t.Fatalf("request order/duplicates not preserved: %#v", paths)
	}
	want := CollectionPath{
		Key:       "LEAF0001",
		Name:      "Methods",
		ParentKey: "MID00001",
		Segments: []CollectionPathSegment{
			{Key: "ROOT0001", Name: "Research"},
			{Key: "MID00001", Name: "Projects"},
			{Key: "LEAF0001", Name: "Methods"},
		},
	}
	if !reflect.DeepEqual(paths[0], want) {
		t.Fatalf("leaf path = %#v, want %#v", paths[0], want)
	}
	if !reflect.DeepEqual(paths[1], CollectionPath{
		Key: "ROOT0001", Name: "Research", Segments: []CollectionPathSegment{{Key: "ROOT0001", Name: "Research"}},
	}) {
		t.Fatalf("root path = %#v", paths[1])
	}
}

func TestResolveCollectionPathsAcceptsFalseAsRoot(t *testing.T) {
	paths, err := ResolveCollectionPaths(
		[]Envelope{collectionEnvelope("ROOT0001", "Root", "false")},
		[]string{"ROOT0001"},
	)
	if err != nil {
		t.Fatalf("ResolveCollectionPaths: %v", err)
	}
	if len(paths) != 1 || paths[0].ParentKey != "" || len(paths[0].Segments) != 1 {
		t.Fatalf("root path = %#v", paths)
	}
}

func TestResolveCollectionPathsRejectsInvalidGraphs(t *testing.T) {
	tests := []struct {
		name        string
		collections []Envelope
		keys        []string
		want        string
	}{
		{name: "empty response key", collections: []Envelope{{Data: json.RawMessage(`{"name":"No key"}`)}}, keys: []string{"ANY00001"}, want: "response has no key"},
		{name: "duplicate response key", collections: []Envelope{collectionEnvelope("DUPL0001", "One", "false"), collectionEnvelope("DUPL0001", "Two", "false")}, keys: []string{"DUPL0001"}, want: `duplicate key "DUPL0001"`},
		{name: "empty requested key", collections: nil, keys: []string{""}, want: "must not be empty"},
		{name: "missing requested collection", collections: nil, keys: []string{"LOST0001"}, want: `no collection with key "LOST0001"`},
		{name: "missing parent", collections: []Envelope{collectionEnvelope("LEAF0001", "Leaf", `"LOST0001"`)}, keys: []string{"LEAF0001"}, want: `references missing parent "LOST0001"`},
		{name: "cycle", collections: []Envelope{collectionEnvelope("ONE00001", "One", `"TWO00002"`), collectionEnvelope("TWO00002", "Two", `"ONE00001"`)}, keys: []string{"ONE00001"}, want: `contains a cycle at "ONE00001"`},
		{name: "missing collection data", collections: []Envelope{{Key: "NODATA01"}}, keys: []string{"NODATA01"}, want: `expected a data object`},
		{name: "null collection data", collections: []Envelope{{Key: "NULLDATA", Data: json.RawMessage(`null`)}}, keys: []string{"NULLDATA"}, want: `expected a data object`},
		{name: "malformed collection", collections: []Envelope{{Key: "BADJSON1", Data: json.RawMessage(`{`)}}, keys: []string{"BADJSON1"}, want: `decode collection "BADJSON1"`},
		{name: "missing data key", collections: []Envelope{{Key: "NOKEY001", Data: json.RawMessage(`{"name":"No Key","parentCollection":false}`)}}, keys: []string{"NOKEY001"}, want: `data has no key`},
		{name: "missing data name", collections: []Envelope{{Key: "NONAME01", Data: json.RawMessage(`{"key":"NONAME01","parentCollection":false}`)}}, keys: []string{"NONAME01"}, want: `data has no name`},
		{name: "mismatched data key", collections: []Envelope{{Key: "OTHER001", Data: json.RawMessage(`{"key":"TOP00001","name":"Mismatch","parentCollection":false}`)}}, keys: []string{"OTHER001"}, want: `collection "OTHER001" data has key "TOP00001"`},
		{name: "missing parent", collections: []Envelope{{Key: "BADPAR01", Data: json.RawMessage(`{"key":"BADPAR01","name":"Bad"}`)}}, keys: []string{"BADPAR01"}, want: "expected a non-empty collection key or false"},
		{name: "null parent", collections: []Envelope{collectionEnvelope("BADPAR01", "Bad", "null")}, keys: []string{"BADPAR01"}, want: "expected a non-empty collection key or false"},
		{name: "empty parent", collections: []Envelope{collectionEnvelope("BADPAR01", "Bad", `""`)}, keys: []string{"BADPAR01"}, want: "expected a non-empty collection key or false"},
		{name: "boolean true parent", collections: []Envelope{collectionEnvelope("BADPAR01", "Bad", "true")}, keys: []string{"BADPAR01"}, want: "expected a non-empty collection key or false"},
		{name: "numeric parent", collections: []Envelope{collectionEnvelope("BADPAR01", "Bad", "42")}, keys: []string{"BADPAR01"}, want: "expected a non-empty collection key or false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveCollectionPaths(tt.collections, tt.keys)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestResolveCollectionPathsIgnoresUnrelatedMalformedParent(t *testing.T) {
	collections := []Envelope{
		collectionEnvelope("ROOT0001", "Root", "false"),
		collectionEnvelope("BROKEN01", "Broken", "true"),
	}
	paths, err := ResolveCollectionPaths(collections, []string{"ROOT0001"})
	if err != nil {
		t.Fatalf("ResolveCollectionPaths: %v", err)
	}
	if len(paths) != 1 || paths[0].Key != "ROOT0001" {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestAllRawCollectionsPreservesEnvelopesAndBackoff(t *testing.T) {
	const first = `{"key":"ROOT0001","version":"","futureTop":{"kept":true},"data":{"key":"ROOT0001","name":"Root","parentCollection":false,"futureData":1}}`
	const second = `{"key":"LEAF0001","version":7,"data":{"key":"LEAF0001","name":"Leaf","parentCollection":"ROOT0001"}}`
	var baseURL string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/0/collections", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("start") {
		case "":
			w.Header().Set("Link", `<`+baseURL+`/api/users/0/collections?start=1>; rel="next"`)
			w.Header().Set("Backoff", "2")
			_, _ = w.Write([]byte(`[` + first + `]`))
		case "1":
			_, _ = w.Write([]byte(`[` + second + `]`))
		default:
			t.Fatalf("unexpected start %q", r.URL.Query().Get("start"))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	baseURL = srv.URL

	recorder := &sleepRecorder{}
	client := New(srv.URL)
	client.sleep = recorder.sleep
	raw, err := client.AllRawCollections(context.Background(), UserLibrary(), CollectionsOptions{})
	if err != nil {
		t.Fatalf("AllRawCollections: %v", err)
	}
	if len(raw) != 2 || len(recorder.waits) != 1 || recorder.waits[0] != 2*time.Second {
		t.Fatalf("raw=%d waits=%v", len(raw), recorder.waits)
	}
	for i, fixture := range []string{first, second} {
		var got, want any
		if err := json.Unmarshal(raw[i], &got); err != nil {
			t.Fatalf("decode raw %d: %v", i, err)
		}
		if err := json.Unmarshal([]byte(fixture), &want); err != nil {
			t.Fatalf("decode fixture %d: %v", i, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("raw %d changed envelope: got %#v, want %#v", i, got, want)
		}
	}
	paths, err := ResolveRawCollectionPaths(raw, []string{"LEAF0001"})
	if err != nil {
		t.Fatalf("ResolveRawCollectionPaths: %v", err)
	}
	if len(paths) != 1 || len(paths[0].Segments) != 2 || paths[0].Segments[0].Key != "ROOT0001" {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestAllRawCollectionsRejectsInvalidCollectionResponses(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want string
	}{
		{name: "empty", body: "", want: "empty response body"},
		{name: "null", body: "null", want: "expected a JSON array"},
		{name: "object", body: `{}`, want: "expected a JSON array"},
		{name: "malformed array", body: `[`, want: "unexpected end"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			_, err := New(srv.URL).AllRawCollections(context.Background(), UserLibrary(), CollectionsOptions{})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestResolveRawCollectionPathsRejectsMalformedEnvelope(t *testing.T) {
	_, err := ResolveRawCollectionPaths([]json.RawMessage{json.RawMessage(`{`)}, []string{"BAD00001"})
	if err == nil || !strings.Contains(err.Error(), "decode collection envelope 0") {
		t.Fatalf("error = %v", err)
	}
}
