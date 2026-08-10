package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

const sourceRelationEnvelope = `{
	"key":"SOURCE01",
	"version":17,
	"futureTop":{"kept":true},
	"data":{
		"key":"SOURCE01",
		"itemType":"journalArticle",
		"title":"Source item",
		"relations":{
			"owl:sameAs":["https://example.com/work/1"],
			"dc:relation":[
				"http://zotero.org/users/123/items/TARGET02",
				"http://zotero.org/groups/42/items/TARGET01"
			]
		},
		"futureData":"preserved"
	}
}`

func relationServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/0/items/{key}", func(w http.ResponseWriter, r *http.Request) {
		switch r.PathValue("key") {
		case "SOURCE01":
			_, _ = w.Write([]byte(sourceRelationEnvelope))
		case "EMPTY001":
			_, _ = w.Write([]byte(`{"key":"EMPTY001","version":4,"data":{"key":"EMPTY001","itemType":"book","relations":{}}}`))
		case "BROKEN01":
			_, _ = w.Write([]byte(`{"key":"BROKEN01","version":5,"data":{"key":"BROKEN01","relations":{"dc:relation":42}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	return httptest.NewServer(mux)
}

func TestRelationListHuman(t *testing.T) {
	srv := relationServer()
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "relation", "list", "SOURCE01")
	if err != nil {
		t.Fatalf("relation list: %v", err)
	}
	for _, want := range []string{"PREDICATE", "TARGET KEY", "dc:relation", "TARGET01", "TARGET02", "owl:sameAs", "https://example.com/work/1"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "TARGET01") > strings.Index(out, "TARGET02") {
		t.Errorf("targets are not stable-sorted:\n%s", out)
	}
}

func TestRelationListJSON(t *testing.T) {
	srv := relationServer()
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "--json", "relation", "list", "SOURCE01")
	if err != nil {
		t.Fatalf("relation list --json: %v", err)
	}
	var doc struct {
		Schema int    `json:"schema"`
		Kind   string `json:"kind"`
		Data   []struct {
			ItemKey   string `json:"itemKey"`
			Predicate string `json:"predicate"`
			Target    string `json:"target"`
			TargetKey string `json:"targetKey"`
		} `json:"data"`
		Meta struct {
			Shown int `json:"shown"`
			Total int `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if doc.Schema != 2 || doc.Kind != "relations" || doc.Meta.Shown != 3 || doc.Meta.Total != 3 {
		t.Fatalf("document = %+v", doc)
	}
	if len(doc.Data) != 3 || doc.Data[0].ItemKey != "SOURCE01" || doc.Data[0].TargetKey != "TARGET01" {
		t.Fatalf("relations = %+v", doc.Data)
	}
	if doc.Data[2].Predicate != "owl:sameAs" || doc.Data[2].TargetKey != "" {
		t.Fatalf("external relation = %+v", doc.Data[2])
	}
	if strings.Contains(out, `"version"`) {
		t.Fatalf("stable output exposes Zotero version:\n%s", out)
	}
}

func TestRelationListJSONL(t *testing.T) {
	srv := relationServer()
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "--jsonl", "relation", "list", "SOURCE01")
	if err != nil {
		t.Fatalf("relation list --jsonl: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3:\n%s", len(lines), out)
	}
	for i, line := range lines {
		var doc struct {
			Schema int    `json:"schema"`
			Kind   string `json:"kind"`
			Data   struct {
				ItemKey string `json:"itemKey"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(line), &doc); err != nil {
			t.Fatalf("line %d invalid: %v", i, err)
		}
		if doc.Schema != 2 || doc.Kind != "relation" || doc.Data.ItemKey != "SOURCE01" {
			t.Errorf("line %d = %+v", i, doc)
		}
	}
}

func TestRelationListRawPreservesEnvelope(t *testing.T) {
	srv := relationServer()
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "--raw", "relation", "list", "SOURCE01")
	if err != nil {
		t.Fatalf("relation list --raw: %v", err)
	}
	var got, want any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid raw JSON: %v\n%s", err, out)
	}
	if err := json.Unmarshal([]byte(sourceRelationEnvelope), &want); err != nil {
		t.Fatalf("invalid fixture: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("raw output changed Zotero's envelope:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestRelationListRawDoesNotShapeMalformedRelations(t *testing.T) {
	srv := relationServer()
	defer srv.Close()

	if _, _, err := runCLI(srv.URL, "--raw", "relation", "list", "BROKEN01"); err != nil {
		t.Fatalf("raw relation list rejected Zotero payload: %v", err)
	}
	if _, _, err := runCLI(srv.URL, "--json", "relation", "list", "BROKEN01"); err == nil || !strings.Contains(err.Error(), "shape relations") {
		t.Fatalf("json malformed relation error = %v", err)
	}
}

func TestRelationListEmptyAndNotFound(t *testing.T) {
	srv := relationServer()
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "relation", "list", "EMPTY001")
	if err != nil {
		t.Fatalf("empty relation list: %v", err)
	}
	if out != "No relations for EMPTY001.\n" {
		t.Fatalf("empty output = %q", out)
	}

	out, _, err = runCLI(srv.URL, "--json", "relation", "list", "EMPTY001")
	if err != nil {
		t.Fatalf("empty relation list --json: %v", err)
	}
	if !strings.Contains(out, `"data": []`) || !strings.Contains(out, `"shown": 0`) {
		t.Fatalf("empty JSON document:\n%s", out)
	}

	if _, _, err := runCLI(srv.URL, "relation", "list", "MISSING1"); err == nil || !strings.Contains(err.Error(), "no item with key") {
		t.Fatalf("missing relation list error = %v", err)
	}
}

func TestRelationListGroupLibrary(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users/0/groups", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":42,"data":{"id":42,"name":"Research Group"}}]`))
	})
	mux.HandleFunc("GET /api/groups/42/items/GROUPREL", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"key":"GROUPREL","data":{"key":"GROUPREL","relations":{"dc:relation":["http://zotero.org/groups/42/items/TARGET01"]}}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	out, _, err := runCLI(srv.URL, "-L", "Research Group", "--json", "relation", "list", "GROUPREL")
	if err != nil {
		t.Fatalf("group relation list: %v", err)
	}
	for _, want := range []string{`"type": "group"`, `"id": 42`, `"targetKey": "TARGET01"`} {
		if !strings.Contains(out, want) {
			t.Errorf("group output missing %q:\n%s", want, out)
		}
	}
}

func TestRelationListWebProfile(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /keys/current", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"userID":123,"username":"ada","access":{"user":{"library":true,"write":false}}}`))
	})
	mux.HandleFunc("GET /users/123/items/WEBREL01", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Zotero-API-Key") != "test-key" {
			t.Error("missing web API key")
		}
		_, _ = w.Write([]byte(`{"key":"WEBREL01","data":{"key":"WEBREL01","relations":{"dc:relation":["http://zotero.org/users/123/items/TARGET01"]}}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	t.Setenv("ZOTGO_API_KEY", "test-key")

	out, _, err := runCLI(srv.URL, "--web", "--json", "relation", "list", "WEBREL01")
	if err != nil {
		t.Fatalf("web relation list: %v", err)
	}
	for _, want := range []string{`"kind": "relations"`, `"id": 123`, `"targetKey": "TARGET01"`} {
		if !strings.Contains(out, want) {
			t.Errorf("web output missing %q:\n%s", want, out)
		}
	}
}

func TestRelationListHelpDocumentsOutput(t *testing.T) {
	out, _, err := runCLI("http://unused.invalid", "relation", "list", "--help")
	if err != nil {
		t.Fatalf("relation list --help: %v", err)
	}
	for _, want := range []string{"list one item's outgoing relations", "<item-key>", "predicate", "targetKey"} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing %q:\n%s", want, out)
		}
	}
}
